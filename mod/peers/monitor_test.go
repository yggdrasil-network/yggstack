package peers

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yggdrasil-network/yggdrasil-go/src/config"
	"github.com/yggdrasil-network/yggdrasil-go/src/core"

	"github.com/yggdrasil-network/yggstack/mod/activity"
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

// mockCallbackObj is a test callback for MonitorObj.
type mockCallbackObj struct {
	calls     atomic.Int64
	lastConn  atomic.Int64
	lastTotal atomic.Int64
}

func (m *mockCallbackObj) OnPeerCountChanged(connected int64, total int64) {
	m.calls.Add(1)
	m.lastConn.Store(connected)
	m.lastTotal.Store(total)
}

// //

func TestMonitorObj_PollDetectsChanges(t *testing.T) {
	cfg := config.GenerateConfig()
	c, err := core.New(cfg.Certificate, noopLoggerObj{})
	if err != nil {
		t.Fatalf("core.New: %s", err)
	}
	defer c.Stop()

	cb := &mockCallbackObj{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewMonitor(c, cb, nil, ctx)

	// 0 peers — callback is not fired (0/0 → 0/0)
	m.Poll()
	if cb.calls.Load() != 0 {
		t.Fatalf("expected 0 calls for 0/0 initial state, got %d", cb.calls.Load())
	}

	// Seed stale values so the next Poll() detects a change (999/999 → 0/0)
	m.lastConn.Store(999)
	m.lastTotal.Store(999)
	m.Poll()
	if cb.calls.Load() != 1 {
		t.Fatalf("expected 1 call after change, got %d", cb.calls.Load())
	}
	if cb.lastConn.Load() != 0 || cb.lastTotal.Load() != 0 {
		t.Fatalf("callback should report 0/0, got %d/%d", cb.lastConn.Load(), cb.lastTotal.Load())
	}
}

func TestMonitorObj_RunExitsOnCancel(t *testing.T) {
	cfg := config.GenerateConfig()
	c, err := core.New(cfg.Certificate, noopLoggerObj{})
	if err != nil {
		t.Fatalf("core.New: %s", err)
	}
	defer c.Stop()

	cb := &mockCallbackObj{}
	ctx, cancel := context.WithCancel(context.Background())

	m := NewMonitor(c, cb, nil, ctx)

	done := make(chan struct{})
	go func() {
		m.Run()
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("MonitorObj.Run() did not exit after cancel")
	}
}

func TestMonitorObj_PollCtxCancelled(t *testing.T) {
	cfg := config.GenerateConfig()
	c, err := core.New(cfg.Certificate, noopLoggerObj{})
	if err != nil {
		t.Fatalf("core.New: %s", err)
	}
	defer c.Stop()

	cb := &mockCallbackObj{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	m := NewMonitor(c, cb, nil, ctx)

	// Poll() with cancelled context must not panic
	m.Poll()
	if cb.calls.Load() != 0 {
		t.Fatalf("expected 0 calls after cancelled context, got %d", cb.calls.Load())
	}
}

func TestMonitorObj_AdaptivePolling(t *testing.T) {
	cfg := config.GenerateConfig()
	c, err := core.New(cfg.Certificate, noopLoggerObj{})
	if err != nil {
		t.Fatalf("core.New: %s", err)
	}
	defer c.Stop()

	cb := &mockCallbackObj{}
	counter := &activity.CounterObj{}
	ctx, cancel := context.WithCancel(context.Background())

	m := NewMonitor(c, cb, counter, ctx)

	// Simulate an active connection — polling switches to pollFast
	counter.Increment()

	// Set fake lastConn/lastTotal so the first poll detects a "change"
	m.lastConn.Store(999)
	m.lastTotal.Store(999)

	done := make(chan struct{})
	go func() {
		m.Run()
		close(done)
	}()

	time.Sleep(pollFast*2 + 200*time.Millisecond)
	cancel()
	<-done

	if cb.calls.Load() < 1 {
		t.Fatalf("expected >= 1 poll call during adaptive polling, got %d", cb.calls.Load())
	}
}
