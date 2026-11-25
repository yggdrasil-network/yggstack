package udpforward

import (
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeUDPLogger is a logger that prints nothing: the tests exercise the
// mapping's behavior, not its log output.
type fakeUDPLogger struct{}

func (fakeUDPLogger) Debugf(string, ...interface{}) {}
func (fakeUDPLogger) Errorf(string, ...interface{}) {}

// blockingUDPConn is a netstack connection whose Write does not return until
// the test releases it. This reproduces the state where the underlying path
// stopped accepting data (congested channel, stuck peer).
type blockingUDPConn struct {
	release chan struct{}
	writes  atomic.Int64
	closed  atomic.Bool
}

func (c *blockingUDPConn) Write(b []byte) (int, error) {
	<-c.release
	c.writes.Add(1)
	return len(b), nil
}

func (c *blockingUDPConn) Read([]byte) (int, error) {
	select {}
}

func (c *blockingUDPConn) Close() error {
	c.closed.Store(true)
	return nil
}

func (c *blockingUDPConn) LocalAddr() net.Addr              { return &net.UDPAddr{} }
func (c *blockingUDPConn) RemoteAddr() net.Addr             { return &net.UDPAddr{} }
func (c *blockingUDPConn) SetDeadline(time.Time) error      { return nil }
func (c *blockingUDPConn) SetReadDeadline(time.Time) error  { return nil }
func (c *blockingUDPConn) SetWriteDeadline(time.Time) error { return nil }

// recordingUDPConn is a connection that accepts everything and records the
// datagram sizes.
type recordingUDPConn struct {
	mu     sync.Mutex
	sizes  []int
	closed atomic.Bool
}

func (c *recordingUDPConn) Write(b []byte) (int, error) {
	c.mu.Lock()
	c.sizes = append(c.sizes, len(b))
	c.mu.Unlock()
	return len(b), nil
}

func (c *recordingUDPConn) Read([]byte) (int, error)         { select {} }
func (c *recordingUDPConn) Close() error                     { c.closed.Store(true); return nil }
func (c *recordingUDPConn) LocalAddr() net.Addr              { return &net.UDPAddr{} }
func (c *recordingUDPConn) RemoteAddr() net.Addr             { return &net.UDPAddr{} }
func (c *recordingUDPConn) SetDeadline(time.Time) error      { return nil }
func (c *recordingUDPConn) SetReadDeadline(time.Time) error  { return nil }
func (c *recordingUDPConn) SetWriteDeadline(time.Time) error { return nil }

func (c *recordingUDPConn) written() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.sizes)
}

// TestForwardLocalUDPKeepsDrainingWhenWriteBlocks checks the key property of
// the mapping: a blocked netstack write must not stop the local socket from
// draining. While the write is blocked, datagrams either sit in the bounded
// queue or are dropped; after the write is released, exactly what made it into
// the queue is written (one in flight + queueLen in the queue), not the whole
// backlog that accumulated during the block. With reading and writing in a
// single goroutine (the old behavior), everything would be written while the
// socket filled up.
func TestForwardLocalUDPKeepsDrainingWhenWriteBlocks(t *testing.T) {
	const queueLen = 2
	const sent = 20

	listen, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listen.Close()

	conn := &blockingUDPConn{release: make(chan struct{})}
	go ForwardLocalUDP(
		listen,
		1500,
		func() (UDPWriteConn, error) { return conn, nil },
		fakeUDPLogger{},
		time.Second,
		queueLen,
	)

	client, err := net.DialUDP("udp", nil, listen.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	payload := make([]byte, 100)
	for i := 0; i < sent; i++ {
		if _, err := client.Write(payload); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}

	// Let the reader pull all datagrams out of the socket while the write is blocked.
	time.Sleep(time.Second)

	close(conn.release)
	time.Sleep(time.Second)

	got := int(conn.writes.Load())
	want := queueLen + 1
	if got != want {
		t.Fatalf("wrote %d datagrams, expected %d: reading must not back up the stream while the write is blocked", got, want)
	}
}

// TestForwardLocalUDPWritesAllWhenTransportAccepts verifies that, with a working
// transport, the mapping forwards every datagram without loss.
func TestForwardLocalUDPWritesAllWhenTransportAccepts(t *testing.T) {
	const queueLen = 8
	const sent = 5

	listen, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listen.Close()

	conn := &recordingUDPConn{}
	go ForwardLocalUDP(
		listen,
		1500,
		func() (UDPWriteConn, error) { return conn, nil },
		fakeUDPLogger{},
		time.Second,
		queueLen,
	)

	client, err := net.DialUDP("udp", nil, listen.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	for i := 0; i < sent; i++ {
		if _, err := client.Write(make([]byte, 100+i)); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}

	deadline := time.Now().Add(2 * time.Second)
	for conn.written() < sent && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if got := conn.written(); got != sent {
		t.Fatalf("wrote %d datagrams, expected %d", got, sent)
	}
}
