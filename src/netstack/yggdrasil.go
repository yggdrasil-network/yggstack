package netstack

import (
	"log"
	"net"

	"github.com/yggdrasil-network/yggdrasil-go/src/core"
	"github.com/yggdrasil-network/yggdrasil-go/src/ipv6rwc"

	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
)

// overlayConn is the part of *ipv6rwc.ReadWriteCloser that the NIC depends on.
// The NIC is written against this interface so that its behaviour can be
// exercised without a live Yggdrasil core.
type overlayConn interface {
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
	MTU() uint64
	MaxMTU() uint64
	SetMTU(mtu uint64)
}

// nicReaders is the number of goroutines that read the overlay at the same
// time.
//
// Reading is not a passive operation. Read sends from inside the call in two
// cases: an inbound packet larger than the overlay MTU is answered with an
// ICMPv6 PacketTooBig, and a packet that was buffered until the destination
// key was learned is flushed once that key becomes known. Such a send goes
// through the session layer, which waits for other actors of the library to
// make progress. That wait has no timeout and a single read call cannot be
// cancelled on its own: only closing the whole connection, which tears down
// the node, breaks it. A single reader would therefore stop draining the
// inbound queue for as long as one send is waiting, and the actors the send
// waits for are fed from that very queue, so the NIC would stop receiving for
// good. With a second reader the inbound queue keeps being drained while the
// first one is busy inside a send, so one stuck call no longer stops inbound
// delivery.
//
// Packets are delivered by whichever reader picks them up first, so their
// order is not guaranteed across readers. The overlay is a datagram network
// and both UDP and TCP tolerate reordering.
const nicReaders = 2

type YggdrasilNIC struct {
	stack       *YggdrasilNetstack
	ipv6rwc     overlayConn
	dispatcher  stack.NetworkDispatcher
	mtu         uint32
	rstPackets  chan *stack.PacketBuffer
	packetQueue chan []byte
}

// newNIC prepares a NIC over the given overlay connection.
//
// Two different limits are involved and they must not be confused.
//
// The MTU the local network stack is told about (nic.mtu) bounds the packets
// that stack builds, and it is the smaller of the two. The MTU kept by the
// overlay connection bounds the packets that may arrive from the overlay: a
// larger one is dropped and answered with an ICMPv6 PacketTooBig. That answer
// is a send performed on the reading goroutine (see nicReaders), so the
// overlay limit is raised to the largest packet the overlay can deliver at
// all, and no inbound packet can reach the answering path. Doing so also stops
// the NIC from discarding perfectly valid traffic: a local UDP mapping whose
// datagram is larger than the default limit would otherwise be answered with
// PacketTooBig on every packet and never reach its socket.
func newNIC(rwc overlayConn) *YggdrasilNIC {
	localMTU := rwc.MTU()
	rwc.SetMTU(rwc.MaxMTU())
	return &YggdrasilNIC{
		ipv6rwc:     rwc,
		mtu:         uint32(localMTU),
		rstPackets:  make(chan *stack.PacketBuffer, 100),
		packetQueue: make(chan []byte, 1000),
	}
}

func (s *YggdrasilNetstack) NewYggdrasilNIC(ygg *core.Core) tcpip.Error {
	return s.newYggdrasilNIC(newNIC(ipv6rwc.NewReadWriteCloser(ygg)), ygg)
}

func (s *YggdrasilNetstack) newYggdrasilNIC(nic *YggdrasilNIC, ygg *core.Core) tcpip.Error {
	nic.stack = s
	if err := s.stack.CreateNIC(1, nic); err != nil {
		return err
	}
	for i := 0; i < nicReaders; i++ {
		go nic.readLoop(i + 1)
	}
	go func() {
		for packet := range nic.packetQueue {
			pkb := stack.NewPacketBuffer(stack.PacketBufferOptions{
				Payload: buffer.MakeWithData(packet),
			})
			nic.dispatcher.DeliverNetworkPacket(ipv6.ProtocolNumber, pkb)
			pkb.DecRef()
		}
	}()
	go func() {
		for {
			pkt := <-nic.rstPackets
			if pkt == nil {
				continue
			}
			_ = nic.writePacket(pkt)
			pkt.DecRef()
		}
	}()
	_, snet, err := net.ParseCIDR("0200::/7")
	if err != nil {
		return &tcpip.ErrBadAddress{}
	}
	subnet, err := tcpip.NewSubnet(
		tcpip.AddrFromSlice(snet.IP.To16()),
		tcpip.MaskFrom(string(snet.Mask)),
	)
	if err != nil {
		return &tcpip.ErrBadAddress{}
	}
	s.stack.AddRoute(tcpip.Route{
		Destination: subnet,
		NIC:         1,
	})
	if s.stack.HandleLocal() {
		ip := ygg.Address()
		if err := s.stack.AddProtocolAddress(
			1,
			tcpip.ProtocolAddress{
				Protocol:          ipv6.ProtocolNumber,
				AddressWithPrefix: tcpip.AddrFromSlice(ip.To16()).WithPrefix(),
			},
			stack.AddressProperties{},
		); err != nil {
			return err
		}
	}
	return nil
}

func (e *YggdrasilNIC) Attach(dispatcher stack.NetworkDispatcher) { e.dispatcher = dispatcher }

func (e *YggdrasilNIC) IsAttached() bool { return e.dispatcher != nil }

// readLoop reads the overlay until the connection fails. Each reader owns its
// buffer, so the readers never share memory.
func (e *YggdrasilNIC) readLoop(id int) {
	buf := make([]byte, e.ipv6rwc.MaxMTU())
	for {
		rx, err := e.ipv6rwc.Read(buf)
		if err != nil {
			// Losing a reader narrows the protection described above the
			// constant: with none left, a single send performed inside Read
			// stops inbound delivery again. Say so instead of going quiet.
			log.Printf("netstack: overlay reader %d of %d stopped: %v", id, nicReaders, err)
			return
		}
		if rx == 0 {
			continue
		}
		packet := make([]byte, rx)
		copy(packet, buf[:rx])
		select {
		case e.packetQueue <- packet:
		default:
			log.Println("Packet queue full, dropping packet")
		}
	}
}

func (e *YggdrasilNIC) MTU() uint32 { return e.mtu }

func (e *YggdrasilNIC) SetMTU(uint32) {}

func (*YggdrasilNIC) Capabilities() stack.LinkEndpointCapabilities { return stack.CapabilityNone }

func (*YggdrasilNIC) MaxHeaderLength() uint16 { return 40 }

func (*YggdrasilNIC) LinkAddress() tcpip.LinkAddress { return "" }

func (*YggdrasilNIC) SetLinkAddress(tcpip.LinkAddress) {}

func (*YggdrasilNIC) Wait() {}

func (e *YggdrasilNIC) writePacket(
	pkt *stack.PacketBuffer,
) tcpip.Error {
	// We need to recover from panic() here because
	// parser in ToView() gets confused on some packets
	// without payload and panics
	defer func() {
		r := recover()
		if r != nil {
		}
	}()
	// ToView returns a copy owned by this call, so concurrent writers - the
	// local stack and the goroutine that retries reset packets - never share
	// a buffer here.
	vv := pkt.ToView()
	defer vv.Release()
	if _, err := e.ipv6rwc.Write(vv.AsSlice()); err != nil {
		return &tcpip.ErrAborted{}
	}
	return nil
}

func (e *YggdrasilNIC) WritePackets(
	list stack.PacketBufferList,
) (int, tcpip.Error) {
	var i int = 0
	var err tcpip.Error = nil
	for i, pkt := range list.AsSlice() {
		if pkt.Data().Size() == 0 {
			if pkt.Network().TransportProtocol() == tcp.ProtocolNumber {
				tcpHeader := header.TCP(pkt.TransportHeader().Slice())
				if (tcpHeader.Flags() & header.TCPFlagRst) == header.TCPFlagRst {
					pkt.IncRef()
					select {
					case e.rstPackets <- pkt:
						// Packet queued successfully
					default:
						// Channel full, drop packet and release ref
						pkt.DecRef()
					}
					continue
				}
			}
		}
		err = e.writePacket(pkt)
		if err != nil {
			log.Println(err)
			return i - 1, err
		}
	}

	return i, nil
}

func (e *YggdrasilNIC) WriteRawPacket(*stack.PacketBuffer) tcpip.Error {
	panic("not implemented")
}

func (*YggdrasilNIC) ARPHardwareType() header.ARPHardwareType {
	return header.ARPHardwareNone
}

func (e *YggdrasilNIC) AddHeader(*stack.PacketBuffer) {
}

func (e *YggdrasilNIC) ParseHeader(*stack.PacketBuffer) bool {
	return true
}

func (e *YggdrasilNIC) Close() {
	e.stack.stack.RemoveNIC(1)
	e.dispatcher = nil
}

func (e *YggdrasilNIC) SetOnCloseAction(func()) {}
