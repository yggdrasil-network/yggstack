package yggstack

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/yggdrasil-network/yggdrasil-go/src/config"
	"github.com/yggdrasil-network/yggdrasil-go/src/core"
)

// // // // // // // // // //

// mockCloserObj tracks Close() calls
type mockCloserObj struct {
	closed atomic.Int32
}

func (m *mockCloserObj) Close() error {
	m.closed.Add(1)
	return nil
}

// //

// mockConnObj implements net.Conn with Close() tracking
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

// Compile-time check: noopLoggerObj must satisfy core.Logger
var _ core.Logger = noopLoggerObj{}

// //

func TestNoopLoggerObj(t *testing.T) {
	noop := noopLoggerObj{}

	// All methods must not panic with any arguments
	noop.Printf("format %s %d", "str", 42)
	noop.Printf("")
	noop.Println("a", "b", 1)
	noop.Println()
	noop.Infof("info %v", nil)
	noop.Infoln("info")
	noop.Warnf("warn %d", 1)
	noop.Warnln("warn")
	noop.Errorf("error %v", fmt.Errorf("test"))
	noop.Errorln("error")
	noop.Debugf("debug %s", "x")
	noop.Debugln("debug")
	noop.Traceln("trace")
}

func TestIsErrorAddressAlreadyInUse(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "io.EOF",
			err:  io.EOF,
			want: false,
		},
		{
			name: "random error",
			err:  fmt.Errorf("something failed"),
			want: false,
		},
		{
			name: "EADDRINUSE",
			err:  &os.SyscallError{Syscall: "bind", Err: syscall.EADDRINUSE},
			want: true,
		},
		{
			name: "ECONNREFUSED",
			err:  &os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSED},
			want: false,
		},
		{
			name: "wrapped EADDRINUSE",
			err:  fmt.Errorf("listen: %w", &os.SyscallError{Syscall: "bind", Err: syscall.EADDRINUSE}),
			want: true,
		},
		{
			name: "wrapped non-EADDRINUSE",
			err:  fmt.Errorf("listen: %w", &os.SyscallError{Syscall: "bind", Err: syscall.EPERM}),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isErrorAddressAlreadyInUse(tt.err)
			if got != tt.want {
				t.Errorf("isErrorAddressAlreadyInUse(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestAddCloser(t *testing.T) {
	t.Run("sequential", func(t *testing.T) {
		obj := &Obj{logger: noopLoggerObj{}}
		c1 := &mockCloserObj{}
		c2 := &mockCloserObj{}
		c3 := &mockCloserObj{}

		obj.addCloser(c1)
		obj.addCloser(c2)
		obj.addCloser(c3)

		if len(obj.closers) != 3 {
			t.Fatalf("len(closers) = %d, want 3", len(obj.closers))
		}
	})

	t.Run("concurrent", func(t *testing.T) {
		obj := &Obj{logger: noopLoggerObj{}}
		const n = 100
		var wg sync.WaitGroup
		wg.Add(n)
		for i := 0; i < n; i++ {
			go func() {
				defer wg.Done()
				obj.addCloser(&mockCloserObj{})
			}()
		}
		wg.Wait()

		if len(obj.closers) != n {
			t.Fatalf("len(closers) = %d, want %d", len(obj.closers), n)
		}
	})
}

func TestCloseIdempotency(t *testing.T) {
	cfg := config.GenerateConfig()
	c, err := core.New(cfg.Certificate, noopLoggerObj{})
	if err != nil {
		t.Fatalf("core.New: %v", err)
	}

	var cancelCount atomic.Int32
	closer1 := &mockCloserObj{}
	closer2 := &mockCloserObj{}

	obj := &Obj{
		Core:    c,
		ctx:     context.Background(),
		logger:  noopLoggerObj{},
		cancel:  func() { cancelCount.Add(1) },
		closers: []io.Closer{closer1, closer2},
	}

	// First call executes cleanup
	if err := obj.Close(); err != nil {
		t.Fatalf("first Close() returned error: %v", err)
	}
	if cancelCount.Load() != 1 {
		t.Errorf("cancel called %d times after first Close(), want 1", cancelCount.Load())
	}
	if closer1.closed.Load() != 1 {
		t.Errorf("closer1 closed %d times, want 1", closer1.closed.Load())
	}
	if closer2.closed.Load() != 1 {
		t.Errorf("closer2 closed %d times, want 1", closer2.closed.Load())
	}

	// Second and third calls do nothing
	_ = obj.Close()
	_ = obj.Close()

	if cancelCount.Load() != 1 {
		t.Errorf("cancel called %d times after 3 Close() calls, want 1", cancelCount.Load())
	}
	if closer1.closed.Load() != 1 {
		t.Errorf("closer1 closed %d times after 3 Close() calls, want 1", closer1.closed.Load())
	}
}

func TestCloseNilFields(t *testing.T) {
	// Close() must not panic when optional fields are nil
	cfg := config.GenerateConfig()
	c, err := core.New(cfg.Certificate, noopLoggerObj{})
	if err != nil {
		t.Fatalf("core.New: %v", err)
	}

	obj := &Obj{
		Core:      c,
		Admin:     nil,
		Multicast: nil,
		Netstack:  nil,
		ctx:       context.Background(),
		logger:    noopLoggerObj{},
		cancel:    func() {},
	}

	// Must not panic
	if err := obj.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}
}

func TestUDPSessionLastActivity(t *testing.T) {
	session := &udpSessionObj{
		conn: &mockConnObj{},
	}

	// Initial value is zero
	if v := session.lastActivity.Load(); v != 0 {
		t.Errorf("initial lastActivity = %d, want 0", v)
	}

	// Store and load
	now := time.Now().Unix()
	session.lastActivity.Store(now)
	if v := session.lastActivity.Load(); v != now {
		t.Errorf("lastActivity = %d, want %d", v, now)
	}

	// Concurrent Store/Load must not race
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		ts := int64(i)
		go func() {
			defer wg.Done()
			session.lastActivity.Store(ts)
		}()
		go func() {
			defer wg.Done()
			_ = session.lastActivity.Load()
		}()
	}
	wg.Wait()
}

func TestStopCoreWithTimeout_ZeroMeansNoTimeout(t *testing.T) {
	cfg := config.GenerateConfig()
	c, err := core.New(cfg.Certificate, noopLoggerObj{})
	if err != nil {
		t.Fatalf("core.New: %v", err)
	}

	obj := &Obj{
		Core:            c,
		coreStopTimeout: 0, // infinite wait
		logger:          noopLoggerObj{},
	}

	obj.stopCoreWithTimeout()
	if obj.Core != nil {
		t.Fatal("Core must be nil after stopCoreWithTimeout()")
	}
}

func TestStopCoreWithTimeout_WithTimeout(t *testing.T) {
	cfg := config.GenerateConfig()
	c, err := core.New(cfg.Certificate, noopLoggerObj{})
	if err != nil {
		t.Fatalf("core.New: %v", err)
	}

	obj := &Obj{
		Core:            c,
		coreStopTimeout: 5 * time.Second, // timeout won't fire — Core stops fast
		logger:          noopLoggerObj{},
	}

	start := time.Now()
	obj.stopCoreWithTimeout()
	elapsed := time.Since(start)

	if obj.Core != nil {
		t.Fatal("Core must be nil after stopCoreWithTimeout()")
	}
	if elapsed >= 5*time.Second {
		t.Fatalf("expected fast stop, got %s", elapsed)
	}
}

func TestStopCoreWithTimeout_NilCore(t *testing.T) {
	obj := &Obj{
		Core:            nil,
		coreStopTimeout: 1 * time.Second,
		logger:          noopLoggerObj{},
	}

	// Must not panic
	obj.stopCoreWithTimeout()
	if obj.Core != nil {
		t.Fatal("Core must remain nil")
	}
}

func TestComponentsCtx_CancelStopsGoroutines(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	componentsCtx, componentsCancel := context.WithCancel(ctx)

	obj := &Obj{
		ctx:              ctx,
		cancel:           cancel,
		componentsCtx:    componentsCtx,
		componentsCancel: componentsCancel,
		logger:           noopLoggerObj{},
	}

	// Start a goroutine bound to componentsCtx
	done := make(chan struct{})
	obj.componentsWg.Add(1)
	go func() {
		defer obj.componentsWg.Done()
		<-obj.componentsCtx.Done()
		close(done)
	}()

	// Cancelling componentsCtx stops the goroutine
	componentsCancel()
	obj.componentsWg.Wait()

	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("goroutine should have stopped after componentsCancel()")
	}
}

func TestComponentsCtx_NewGeneration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	firstCtx, firstCancel := context.WithCancel(ctx)
	obj := &Obj{
		ctx:              ctx,
		cancel:           cancel,
		componentsCtx:    firstCtx,
		componentsCancel: firstCancel,
		logger:           noopLoggerObj{},
	}

	// First cancellation
	firstCancel()
	if obj.componentsCtx.Err() == nil {
		t.Fatal("first componentsCtx should be cancelled")
	}

	// New generation
	obj.componentsCtx, obj.componentsCancel = context.WithCancel(ctx)
	if obj.componentsCtx.Err() != nil {
		t.Fatal("new componentsCtx should not be cancelled")
	}

	// Parent context is not affected
	if ctx.Err() != nil {
		t.Fatal("parent ctx should not be cancelled")
	}

	obj.componentsCancel()
}

// //

func TestCleanupUDPSessions(t *testing.T) {
	obj := &Obj{logger: noopLoggerObj{}}

	timeout := 100 * time.Millisecond
	now := time.Now().Unix()

	expiredConn := &mockConnObj{}
	activeConn := &mockConnObj{}

	expired := &udpSessionObj{conn: expiredConn}
	expired.lastActivity.Store(now - 10) // Well past timeout

	active := &udpSessionObj{conn: activeConn}
	active.lastActivity.Store(now + 60) // Far in the future

	sessions := &udpSessionMapObj{data: make(map[string]*udpSessionObj)}
	sessions.Store("expired", expired)
	sessions.Store("active", active)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go obj.cleanupUDPSessions(ctx, sessions, timeout)

	// Wait for at least one cleanup tick (timeout/4 = 25ms) + margin
	time.Sleep(timeout/2 + 20*time.Millisecond)

	// Expired session must be removed and connection closed
	if _, ok := sessions.Load("expired"); ok {
		t.Error("expired session was not cleaned up")
	}
	if expiredConn.closed.Load() == 0 {
		t.Error("expired session conn was not closed")
	}

	// Active session must remain
	if _, ok := sessions.Load("active"); !ok {
		t.Error("active session was incorrectly removed")
	}
	if activeConn.closed.Load() != 0 {
		t.Error("active session conn was unexpectedly closed")
	}
}
