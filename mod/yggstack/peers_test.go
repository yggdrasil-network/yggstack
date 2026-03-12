package yggstack

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yggdrasil-network/yggdrasil-go/src/config"
	"github.com/yggdrasil-network/yggdrasil-go/src/core"
)

// // // // // // // // // //

// mockPeerCallbackObj is a test callback for peerMonitor.
type mockPeerCallbackObj struct {
	calls     atomic.Int64
	lastConn  atomic.Int64
	lastTotal atomic.Int64
}

func (m *mockPeerCallbackObj) OnPeerCountChanged(connected int64, total int64) {
	m.calls.Add(1)
	m.lastConn.Store(connected)
	m.lastTotal.Store(total)
}

// //

func TestPeerMonitorObj_PollDetectsChanges(t *testing.T) {
	// Create a real Core with empty config — no peers
	cfg := config.GenerateConfig()
	c, err := core.New(cfg.Certificate, noopLoggerObj{})
	if err != nil {
		t.Fatalf("core.New: %s", err)
	}
	defer c.Stop()

	cb := &mockPeerCallbackObj{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := &peerMonitorObj{
		core:     c,
		callback: cb,
		ctx:      ctx,
		cancel:   cancel,
	}

	// Initial poll — 0 peers
	m.poll()
	// Callback is not fired on 0->0 (lastConn/lastTotal default to 0)
	// So the first poll with 0/0 does not trigger the callback
	if cb.calls.Load() != 0 {
		t.Fatalf("expected 0 calls for 0/0 initial state, got %d", cb.calls.Load())
	}
}

func TestPeerMonitorObj_RunExitsOnCancel(t *testing.T) {
	cfg := config.GenerateConfig()
	c, err := core.New(cfg.Certificate, noopLoggerObj{})
	if err != nil {
		t.Fatalf("core.New: %s", err)
	}
	defer c.Stop()

	cb := &mockPeerCallbackObj{}
	ctx, cancel := context.WithCancel(context.Background())

	m := &peerMonitorObj{
		core:     c,
		callback: cb,
		ctx:      ctx,
		cancel:   cancel,
	}

	done := make(chan struct{})
	go func() {
		m.run()
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// ok, goroutine exited
	case <-time.After(2 * time.Second):
		t.Fatal("peerMonitor.run() did not exit after cancel")
	}
}

func TestPeerMonitorObj_PollCtxCancelled(t *testing.T) {
	cfg := config.GenerateConfig()
	c, err := core.New(cfg.Certificate, noopLoggerObj{})
	if err != nil {
		t.Fatalf("core.New: %s", err)
	}
	defer c.Stop()

	cb := &mockPeerCallbackObj{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	m := &peerMonitorObj{
		core:     c,
		callback: cb,
		ctx:      ctx,
		cancel:   cancel,
	}

	// poll() with cancelled context must not panic
	m.poll()
	// Callback not called — poll() does early return
	if cb.calls.Load() != 0 {
		t.Fatalf("expected 0 calls after cancelled context, got %d", cb.calls.Load())
	}
}

func TestPeerMonitorObj_AdaptivePolling(t *testing.T) {
	cfg := config.GenerateConfig()
	c, err := core.New(cfg.Certificate, noopLoggerObj{})
	if err != nil {
		t.Fatalf("core.New: %s", err)
	}
	defer c.Stop()

	cb := &mockPeerCallbackObj{}
	counter := &connectionCounterObj{}
	ctx, cancel := context.WithCancel(context.Background())

	m := &peerMonitorObj{
		core:        c,
		callback:    cb,
		connCounter: counter,
		ctx:         ctx,
		cancel:      cancel,
	}

	// Simulate an active connection — polling switches to peerPollFast
	counter.increment()

	// Set fake lastConn/lastTotal so the first poll detects a "change"
	m.lastConn = 999
	m.lastTotal = 999

	done := make(chan struct{})
	go func() {
		m.run()
		close(done)
	}()

	// Wait long enough for initial poll + at least 1 tick (with 0 real peers
	// callback fires only when detecting the 999->0 change)
	time.Sleep(peerPollFast*2 + 200*time.Millisecond)
	cancel()
	<-done

	// Verify that polling with connCounter > 0 invoked the callback at least once
	if cb.calls.Load() < 1 {
		t.Fatalf("expected >= 1 poll call during adaptive polling, got %d", cb.calls.Load())
	}
}
