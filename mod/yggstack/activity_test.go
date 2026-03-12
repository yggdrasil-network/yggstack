package yggstack

import (
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// // // // // // // // // //

type mockActivityCallbackObj struct {
	created []string
	closed  []string
	mu      sync.Mutex
}

func (m *mockActivityCallbackObj) OnConnectionCreated(connId string, protocol string) {
	m.mu.Lock()
	m.created = append(m.created, connId)
	m.mu.Unlock()
}

func (m *mockActivityCallbackObj) OnConnectionClosed(connId string) {
	m.mu.Lock()
	m.closed = append(m.closed, connId)
	m.mu.Unlock()
}

// //

func TestTrackedConnObj_Write(t *testing.T) {
	cb := &mockActivityCallbackObj{}
	counter := &connectionCounterObj{}
	counter.increment()

	inner := &mockWritableConnObj{}
	tracked := &trackedConnObj{
		Conn:     inner,
		connId:   "test-conn-1",
		callback: cb,
		counter:  counter,
	}

	payload := []byte("hello yggdrasil")
	n, err := tracked.Write(payload)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(payload) {
		t.Fatalf("expected %d bytes written, got %d", len(payload), n)
	}
}

func TestTrackedConnObj_Read(t *testing.T) {
	cb := &mockActivityCallbackObj{}
	counter := &connectionCounterObj{}
	counter.increment()

	pr, pw := io.Pipe()
	inner := &mockPipeConnObj{reader: pr, writer: pw}

	tracked := &trackedConnObj{
		Conn:     inner,
		connId:   "test-conn-2",
		callback: cb,
		counter:  counter,
	}

	payload := []byte("hello yggdrasil")
	go func() {
		_, _ = pw.Write(payload)
	}()

	buf := make([]byte, 64)
	n, err := tracked.Read(buf)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if n != len(payload) {
		t.Fatalf("expected %d bytes, got %d", len(payload), n)
	}
}

func TestTrackedConnObj_CloseIdempotent(t *testing.T) {
	cb := &mockActivityCallbackObj{}
	counter := &connectionCounterObj{}
	counter.increment()

	inner := &mockConnObj{}

	tracked := &trackedConnObj{
		Conn:     inner,
		connId:   "test-close-1",
		callback: cb,
		counter:  counter,
	}

	_ = tracked.Close()
	_ = tracked.Close()
	_ = tracked.Close()

	cb.mu.Lock()
	closedCount := len(cb.closed)
	cb.mu.Unlock()

	if closedCount != 1 {
		t.Errorf("expected OnConnectionClosed called once, got %d", closedCount)
	}

	if counter.count() != 0 {
		t.Errorf("expected counter=0 after close, got %d", counter.count())
	}

	if inner.closed.Load() != 1 {
		t.Errorf("expected inner Close called once, got %d", inner.closed.Load())
	}
}

func TestConnectionCounterObj_Concurrent(t *testing.T) {
	counter := &connectionCounterObj{}
	var wg sync.WaitGroup
	n := 1000

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			counter.increment()
		}()
	}
	wg.Wait()

	if counter.count() != int64(n) {
		t.Errorf("expected %d, got %d", n, counter.count())
	}

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			counter.decrement()
		}()
	}
	wg.Wait()

	if counter.count() != 0 {
		t.Errorf("expected 0, got %d", counter.count())
	}
}

func TestGenerateConnId(t *testing.T) {
	id1 := generateConnId("socks", "127.0.0.1:1080")
	id2 := generateConnId("socks", "127.0.0.1:1080")

	if !strings.HasPrefix(id1, "socks-127.0.0.1:1080-") {
		t.Errorf("unexpected id format: %s", id1)
	}
	if id1 == id2 {
		t.Errorf("expected unique ids, got %s == %s", id1, id2)
	}
}

func TestGenerateConnId_BurstUniqueness(t *testing.T) {
	const n = 100_000
	ids := make(map[string]struct{}, n)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := generateConnId("socks", "127.0.0.1:1080")
			mu.Lock()
			ids[id] = struct{}{}
			mu.Unlock()
		}()
	}
	wg.Wait()

	if len(ids) != n {
		t.Fatalf("expected %d unique IDs, got %d (collisions: %d)", n, len(ids), n-len(ids))
	}
}

// //

// mockWritableConnObj is a net.Conn that returns len(b) from Write.
type mockWritableConnObj struct {
	closed atomic.Int32
}

func (m *mockWritableConnObj) Read([]byte) (int, error)         { return 0, io.EOF }
func (m *mockWritableConnObj) Write(b []byte) (int, error)      { return len(b), nil }
func (m *mockWritableConnObj) Close() error                     { m.closed.Add(1); return nil }
func (m *mockWritableConnObj) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (m *mockWritableConnObj) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (m *mockWritableConnObj) SetDeadline(time.Time) error      { return nil }
func (m *mockWritableConnObj) SetReadDeadline(time.Time) error  { return nil }
func (m *mockWritableConnObj) SetWriteDeadline(time.Time) error { return nil }

// mockPipeConnObj is a net.Conn backed by io.Pipe for testing Read/Write.
type mockPipeConnObj struct {
	reader *io.PipeReader
	writer *io.PipeWriter
}

func (m *mockPipeConnObj) Read(b []byte) (int, error)       { return m.reader.Read(b) }
func (m *mockPipeConnObj) Write(b []byte) (int, error)      { return m.writer.Write(b) }
func (m *mockPipeConnObj) Close() error                     { _ = m.reader.Close(); return m.writer.Close() }
func (m *mockPipeConnObj) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (m *mockPipeConnObj) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (m *mockPipeConnObj) SetDeadline(time.Time) error      { return errors.ErrUnsupported }
func (m *mockPipeConnObj) SetReadDeadline(time.Time) error  { return errors.ErrUnsupported }
func (m *mockPipeConnObj) SetWriteDeadline(time.Time) error { return errors.ErrUnsupported }
