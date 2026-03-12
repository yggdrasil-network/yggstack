package types

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"
)

// // // // // // // // // //

// newUDPPair returns two bound UDP conns that can exchange packets with each other.
func newUDPPair(t *testing.T) (a, b *net.UDPConn) {
	t.Helper()
	a, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("ListenUDP a: %v", err)
	}
	b, err = net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		a.Close()
		t.Fatalf("ListenUDP b: %v", err)
	}
	t.Cleanup(func() { a.Close(); b.Close() })
	return a, b
}

// //

func TestReverseProxyUDP_ForwardsData(t *testing.T) {
	dst, recv := newUDPPair(t)
	srcLeft, srcRight := net.Pipe()
	defer srcLeft.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- ReverseProxyUDP(ctx, 1500, dst, recv.LocalAddr(), srcRight)
	}()

	want := []byte("hello udp world")
	if _, err := srcLeft.Write(want); err != nil {
		t.Fatalf("Write to src: %v", err)
	}

	buf := make([]byte, 1500)
	recv.SetDeadline(time.Now().Add(time.Second))
	n, _, err := recv.ReadFrom(buf)
	if err != nil {
		t.Fatalf("ReadFrom recv: %v", err)
	}
	if !bytes.Equal(buf[:n], want) {
		t.Errorf("got %q, want %q", buf[:n], want)
	}

	// Teardown via src close
	srcLeft.Close()
	select {
	case err := <-errCh:
		if err == nil {
			t.Error("expected non-nil error after src closed")
		}
	case <-time.After(time.Second):
		t.Fatal("ReverseProxyUDP did not return after src closed")
	}
}

func TestReverseProxyUDP_ContextCancel(t *testing.T) {
	dst, _ := newUDPPair(t)
	srcLeft, srcRight := net.Pipe()
	defer srcLeft.Close()

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- ReverseProxyUDP(ctx, 1500, dst, dst.LocalAddr(), srcRight)
	}()

	cancel()

	select {
	case err := <-errCh:
		if err != context.Canceled {
			t.Errorf("want context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ReverseProxyUDP did not return after context cancel")
	}
}

func TestReverseProxyUDP_SrcClose(t *testing.T) {
	dst, _ := newUDPPair(t)
	srcLeft, srcRight := net.Pipe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- ReverseProxyUDP(ctx, 1500, dst, dst.LocalAddr(), srcRight)
	}()

	srcLeft.Close()

	select {
	case err := <-errCh:
		if err == nil {
			t.Error("expected non-nil error after src closed")
		}
	case <-time.After(time.Second):
		t.Fatal("ReverseProxyUDP did not return after src closed")
	}
}

// //

func BenchmarkReverseProxyUDP(b *testing.B) {
	dst, recv := newUDPPairB(b)
	srcLeft, srcRight := net.Pipe()
	defer srcLeft.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go ReverseProxyUDP(ctx, 1500, dst, recv.LocalAddr(), srcRight)

	payload := bytes.Repeat([]byte("x"), 512)
	buf := make([]byte, 1500)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		srcLeft.Write(payload)
		recv.SetDeadline(time.Now().Add(time.Second))
		recv.ReadFrom(buf)
	}
}

func newUDPPairB(b *testing.B) (a, recv *net.UDPConn) {
	b.Helper()
	a, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		b.Fatalf("ListenUDP a: %v", err)
	}
	recv, err = net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		a.Close()
		b.Fatalf("ListenUDP recv: %v", err)
	}
	b.Cleanup(func() { a.Close(); recv.Close() })
	return a, recv
}
