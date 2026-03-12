package mapping

import (
	"context"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// // // // // // // // // //

type noopLoggerObj struct{}

func (noopLoggerObj) Printf(string, ...interface{}) {}
func (noopLoggerObj) Println(...interface{})        {}
func (noopLoggerObj) Infof(string, ...interface{})  {}
func (noopLoggerObj) Infoln(...interface{})         {}
func (noopLoggerObj) Warnf(string, ...interface{})  {}
func (noopLoggerObj) Warnln(...interface{})         {}
func (noopLoggerObj) Errorf(string, ...interface{}) {}
func (noopLoggerObj) Errorln(...interface{})        {}
func (noopLoggerObj) Debugf(string, ...interface{}) {}
func (noopLoggerObj) Debugln(...interface{})        {}
func (noopLoggerObj) Traceln(...interface{})        {}

// //

type mockConnObj struct {
	closed atomic.Int32
}

func (m *mockConnObj) Read([]byte) (int, error)         { return 0, io.EOF }
func (m *mockConnObj) Write([]byte) (int, error)        { return 0, nil }
func (m *mockConnObj) Close() error                     { m.closed.Add(1); return nil }
func (m *mockConnObj) LocalAddr() net.Addr              { return nil }
func (m *mockConnObj) RemoteAddr() net.Addr             { return nil }
func (m *mockConnObj) SetDeadline(time.Time) error      { return nil }
func (m *mockConnObj) SetReadDeadline(time.Time) error  { return nil }
func (m *mockConnObj) SetWriteDeadline(time.Time) error { return nil }

// //

func TestUDPSessionLastActivity(t *testing.T) {
	session := &UDPSessionObj{
		Conn: &mockConnObj{},
	}

	if v := session.LastActivity.Load(); v != 0 {
		t.Errorf("initial LastActivity = %d, want 0", v)
	}

	now := time.Now().Unix()
	session.LastActivity.Store(now)
	if v := session.LastActivity.Load(); v != now {
		t.Errorf("LastActivity = %d, want %d", v, now)
	}

	// Concurrent Store/Load must not cause a data race
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		ts := int64(i)
		go func() {
			defer wg.Done()
			session.LastActivity.Store(ts)
		}()
		go func() {
			defer wg.Done()
			_ = session.LastActivity.Load()
		}()
	}
	wg.Wait()
}

func TestCleanupUDPSessions(t *testing.T) {
	timeout := 100 * time.Millisecond
	now := time.Now().Unix()

	expiredConn := &mockConnObj{}
	activeConn := &mockConnObj{}

	expired := &UDPSessionObj{Conn: expiredConn}
	expired.LastActivity.Store(now - 10)

	active := &UDPSessionObj{Conn: activeConn}
	active.LastActivity.Store(now + 60)

	sessions := NewUDPSessionMap()
	sessions.Store("expired", expired)
	sessions.Store("active", active)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go CleanupUDPSessions(ctx, sessions, timeout, noopLoggerObj{})

	// Wait for at least one cleanup tick (timeout/4 = 25ms) + margin
	time.Sleep(timeout/2 + 20*time.Millisecond)

	if _, ok := sessions.Load("expired"); ok {
		t.Error("expired session was not cleaned up")
	}
	if expiredConn.closed.Load() == 0 {
		t.Error("expired session conn was not closed")
	}

	if _, ok := sessions.Load("active"); !ok {
		t.Error("active session was incorrectly removed")
	}
	if activeConn.closed.Load() != 0 {
		t.Error("active session conn was unexpectedly closed")
	}
}
