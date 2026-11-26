package netstack

import (
	"sync"
	"testing"
	"time"
)

// fakeOverlay stands in for *ipv6rwc.ReadWriteCloser. Reads take packets from a
// channel. A read call can be made to block forever, which imitates a call
// that is stuck inside a send the library performs on the reading goroutine.
type fakeOverlay struct {
	packets chan []byte
	// blockAt is the 1-based number of the read call that never returns.
	blockAt int
	maxMTU  uint64

	mu    sync.Mutex
	mtu   uint64
	reads int
}

func newFakeOverlay(mtu, maxMTU uint64) *fakeOverlay {
	return &fakeOverlay{
		packets: make(chan []byte, 8),
		maxMTU:  maxMTU,
		mtu:     mtu,
	}
}

func (f *fakeOverlay) Read(p []byte) (int, error) {
	f.mu.Lock()
	f.reads++
	block := f.blockAt > 0 && f.reads == f.blockAt
	f.mu.Unlock()
	if block {
		select {}
	}
	packet := <-f.packets
	return copy(p, packet), nil
}

func (f *fakeOverlay) Write(p []byte) (int, error) { return len(p), nil }

func (f *fakeOverlay) MTU() uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.mtu
}

func (f *fakeOverlay) MaxMTU() uint64 { return f.maxMTU }

func (f *fakeOverlay) SetMTU(mtu uint64) {
	f.mu.Lock()
	f.mtu = mtu
	f.mu.Unlock()
}

func (f *fakeOverlay) readCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reads
}

// TestNICAcceptsPacketAboveLocalMTU checks that the limit applied to packets
// arriving from the overlay is not the MTU reported to the local stack. An
// inbound packet larger than that MTU must be delivered as it is: answering it
// with an ICMPv6 PacketTooBig is a send performed inside the read call, and
// dropping it loses traffic the local stack can handle (for example a
// forwarded datagram larger than the default limit).
func TestNICAcceptsPacketAboveLocalMTU(t *testing.T) {
	const localMTU, overlayMTU = 1280, 65535
	rwc := newFakeOverlay(localMTU, overlayMTU)
	nic := newNIC(rwc)

	if got := nic.MTU(); got != localMTU {
		t.Fatalf("local stack is told MTU %d, want %d", got, localMTU)
	}
	if got := rwc.MTU(); got != overlayMTU {
		t.Fatalf("overlay accepts packets up to %d, want %d", got, overlayMTU)
	}

	go nic.readLoop(1)
	packet := make([]byte, localMTU+148)
	packet[0] = 0x60
	rwc.packets <- packet

	select {
	case got := <-nic.packetQueue:
		if len(got) != len(packet) {
			t.Fatalf("delivered %d bytes, want %d", len(got), len(packet))
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("packet of %d bytes was not delivered within 2s", len(packet))
	}
}

// TestNICKeepsDeliveringWhileReadIsBlocked checks that one read call stuck
// inside the library does not stop inbound delivery. The library performs
// sends inside Read, and such a send can wait for the session layer to make
// progress, which in turn needs the inbound queue to be drained: with a single
// reader the whole NIC would stop receiving for good.
func TestNICKeepsDeliveringWhileReadIsBlocked(t *testing.T) {
	rwc := newFakeOverlay(1280, 65535)
	rwc.blockAt = 1
	nic := newNIC(rwc)
	for i := 0; i < nicReaders; i++ {
		go nic.readLoop(1)
	}

	deadline := time.After(2 * time.Second)
	for rwc.readCalls() == 0 {
		select {
		case <-deadline:
			t.Fatal("no reader entered the overlay")
		case <-time.After(10 * time.Millisecond):
		}
	}

	rwc.packets <- []byte{0x60, 1, 2, 3}
	select {
	case got := <-nic.packetQueue:
		if len(got) != 4 {
			t.Fatalf("delivered %d bytes, want 4", len(got))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("inbound delivery stopped while one read call was blocked")
	}
}
