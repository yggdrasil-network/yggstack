package yggstack

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
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
