package netstack

import (
	"net"
	"sync"

	"github.com/yggdrasil-network/yggdrasil-go/src/core"
	"github.com/yggdrasil-network/yggdrasil-go/src/ipv6rwc"

	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
)

// // // // // // // // // //

var writeBufPool = sync.Pool{
	New: func() interface{} { return make([]byte, 65535) },
}

type YggdrasilNIC struct {
	stack      *YggdrasilNetstack
	ipv6rwc    *ipv6rwc.ReadWriteCloser
	dispatcher stack.NetworkDispatcher
	readBuf    []byte
	rstPackets chan *stack.PacketBuffer
	done       chan struct{}
	closeOnce  sync.Once
	logger     core.Logger
}

func (s *YggdrasilNetstack) NewYggdrasilNIC(ygg *core.Core) (*YggdrasilNIC, tcpip.Error) {
	rwc := ipv6rwc.NewReadWriteCloser(ygg)
	mtu := rwc.MTU()
	nic := &YggdrasilNIC{
		stack:      s,
		ipv6rwc:    rwc,
		readBuf:    make([]byte, mtu),
		rstPackets: make(chan *stack.PacketBuffer, 100),
		done:       make(chan struct{}),
		logger:     s.logger,
	}
	if err := s.stack.CreateNIC(1, nic); err != nil {
		return nil, err
	}

	// Read packets from Yggdrasil and deliver to netstack
	go func() {
		var rx int
		var err error
		for {
			rx, err = nic.ipv6rwc.Read(nic.readBuf)
			if err != nil {
				select {
				case <-nic.done:
					// Normal shutdown
				default:
					nic.logger.Println(err)
				}
				return
			}
			pkb := stack.NewPacketBuffer(stack.PacketBufferOptions{
				Payload: buffer.MakeWithData(nic.readBuf[:rx]),
			})
			nic.dispatcher.DeliverNetworkPacket(ipv6.ProtocolNumber, pkb)
			pkb.DecRef()
		}
	}()

	// Deferred RST packet sending
	go func() {
		for {
			select {
			case <-nic.done:
				return
			case pkt := <-nic.rstPackets:
				if pkt == nil {
					continue
				}
				_ = nic.writePacket(pkt)
				pkt.DecRef()
			}
		}
	}()

	_, snet, err := net.ParseCIDR("0200::/7")
	if err != nil {
		return nil, &tcpip.ErrBadAddress{}
	}
	subnet, err := tcpip.NewSubnet(
		tcpip.AddrFromSlice(snet.IP.To16()),
		tcpip.MaskFrom(string(snet.Mask)),
	)
	if err != nil {
		return nil, &tcpip.ErrBadAddress{}
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
			return nil, err
		}
	}
	return nic, nil
}

// //

func (e *YggdrasilNIC) Attach(dispatcher stack.NetworkDispatcher) { e.dispatcher = dispatcher }

func (e *YggdrasilNIC) IsAttached() bool { return e.dispatcher != nil }

func (e *YggdrasilNIC) MTU() uint32 { return uint32(e.ipv6rwc.MTU()) }

func (e *YggdrasilNIC) SetMTU(uint32) {}

func (*YggdrasilNIC) Capabilities() stack.LinkEndpointCapabilities { return stack.CapabilityNone }

func (*YggdrasilNIC) MaxHeaderLength() uint16 { return 40 }

func (*YggdrasilNIC) LinkAddress() tcpip.LinkAddress { return "" }

func (*YggdrasilNIC) SetLinkAddress(tcpip.LinkAddress) {}

func (*YggdrasilNIC) Wait() {}

// //

func (e *YggdrasilNIC) writePacket(
	pkt *stack.PacketBuffer,
) tcpip.Error {
	// Recover: ToView() panics on packets without payload
	defer func() {
		if r := recover(); r != nil {
			e.logger.Println("writePacket panic:", r)
		}
	}()
	buf := writeBufPool.Get().([]byte)
	defer writeBufPool.Put(buf)
	vv := pkt.ToView()
	n, err := vv.Read(buf)
	if err != nil {
		return &tcpip.ErrAborted{}
	}
	_, err = e.ipv6rwc.Write(buf[:n])
	if err != nil {
		return &tcpip.ErrAborted{}
	}
	return nil
}

func (e *YggdrasilNIC) WritePackets(
	list stack.PacketBufferList,
) (int, tcpip.Error) {
	for i, pkt := range list.AsSlice() {
		if pkt.Data().Size() == 0 {
			if pkt.Network().TransportProtocol() == tcp.ProtocolNumber {
				tcpHeader := header.TCP(pkt.TransportHeader().Slice())
				if (tcpHeader.Flags() & header.TCPFlagRst) == header.TCPFlagRst {
					pkt.IncRef()
					select {
					case e.rstPackets <- pkt:
					default:
						pkt.DecRef()
					}
					continue
				}
			}
		}
		if err := e.writePacket(pkt); err != nil {
			e.logger.Println(err)
			return i, err
		}
	}

	return list.Len(), nil
}

func (e *YggdrasilNIC) WriteRawPacket(*stack.PacketBuffer) tcpip.Error {
	return &tcpip.ErrNotSupported{}
}

// //

func (*YggdrasilNIC) ARPHardwareType() header.ARPHardwareType {
	return header.ARPHardwareNone
}

func (e *YggdrasilNIC) AddHeader(*stack.PacketBuffer) {
}

func (e *YggdrasilNIC) ParseHeader(*stack.PacketBuffer) bool {
	return true
}

func (e *YggdrasilNIC) Close() {
	e.closeOnce.Do(func() {
		close(e.done)
		e.stack.stack.RemoveNIC(1)
		e.dispatcher = nil
	})
}

func (e *YggdrasilNIC) SetOnCloseAction(func()) {}
