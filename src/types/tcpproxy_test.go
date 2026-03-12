package types

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"
)

// // // // // // // // // //

func TestProxyTCP_DataFlowsBothDirections(t *testing.T) {
	// left1 <-> right1 — ProxyTCP bridge — left2 <-> right2
	left1, right1 := net.Pipe()
	left2, right2 := net.Pipe()
	defer left1.Close()
	defer left2.Close()

	go ProxyTCP(right1, right2)

	want := []byte("hello from side A")

	// A → B
	go left1.Write(want)
	buf := make([]byte, len(want))
	if _, err := io.ReadFull(left2, buf); err != nil {
		t.Fatalf("A→B read: %v", err)
	}
	if !bytes.Equal(buf, want) {
		t.Errorf("A→B: got %q, want %q", buf, want)
	}

	// B → A
	want2 := []byte("reply from side B")
	go left2.Write(want2)
	buf2 := make([]byte, len(want2))
	if _, err := io.ReadFull(left1, buf2); err != nil {
		t.Fatalf("B→A read: %v", err)
	}
	if !bytes.Equal(buf2, want2) {
		t.Errorf("B→A: got %q, want %q", buf2, want2)
	}
}

func TestProxyTCP_ExitsOnClose(t *testing.T) {
	left1, right1 := net.Pipe()
	left2, right2 := net.Pipe()

	done := make(chan struct{})
	go func() {
		ProxyTCP(right1, right2)
		close(done)
	}()

	left1.Close()
	left2.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ProxyTCP did not return after connections were closed")
	}
}

func TestProxyTCP_HalfCloseUnblocks(t *testing.T) {
	left1, right1 := net.Pipe()
	left2, right2 := net.Pipe()

	done := make(chan struct{})
	go func() {
		ProxyTCP(right1, right2)
		close(done)
	}()

	// Closing one side should make ProxyTCP close both and return.
	left1.Close()

	select {
	case <-done:
	case <-time.After(proxyTCPCloseTimeout + time.Second):
		t.Fatal("ProxyTCP did not return after one-sided close")
	}

	// After ProxyTCP returns, both right1/right2 are closed, so left2 should get EOF.
	buf := make([]byte, 1)
	left2.SetDeadline(time.Now().Add(500 * time.Millisecond))
	_, err := left2.Read(buf)
	if err == nil {
		t.Error("expected left2 to be closed after ProxyTCP exits")
	}
}

// //

func BenchmarkProxyTCP(b *testing.B) {
	payload := bytes.Repeat([]byte("x"), 4096)

	left1, right1 := net.Pipe()
	left2, right2 := net.Pipe()
	defer left1.Close()
	defer left2.Close()

	go ProxyTCP(right1, right2)

	buf := make([]byte, len(payload))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		go left1.Write(payload)
		io.ReadFull(left2, buf)
	}
}
