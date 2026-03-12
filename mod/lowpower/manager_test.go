package lowpower

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

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

type mockNodeControlObj struct {
	connCounter  activity.CounterObj
	socksAddr    string
	socksIsUnix  bool
	socksReadyCh chan struct{}
	stopCalled   atomic.Int32
	startCalled  atomic.Int32
	startErr     error
}

func (m *mockNodeControlObj) StopComponents() { m.stopCalled.Add(1) }
func (m *mockNodeControlObj) StartComponents(interface{}) error {
	m.startCalled.Add(1)
	return m.startErr
}
func (m *mockNodeControlObj) ConnCounter() *activity.CounterObj { return &m.connCounter }
func (m *mockNodeControlObj) SocksAddr() string                 { return m.socksAddr }
func (m *mockNodeControlObj) SocksIsUnix() bool                 { return m.socksIsUnix }
func (m *mockNodeControlObj) SocksReadyCh() <-chan struct{}     { return m.socksReadyCh }

// //

func newTestLPM(t *testing.T, idleTimeout time.Duration) (*ManagerObj, *mockNodeControlObj, context.CancelFunc) {
	t.Helper()
	node := &mockNodeControlObj{socksReadyCh: make(chan struct{})}
	cfg := ConfigObj{IdleTimeout: idleTimeout}
	ctx, cancel := context.WithCancel(context.Background())
	m := NewManager(node, cfg, ctx, noopLoggerObj{})
	return m, node, cancel
}

// //

func TestManagerObj_InitialState(t *testing.T) {
	m, _, cancel := newTestLPM(t, 60*time.Second)
	defer cancel()

	if m.state.Load() != StateFullPower {
		t.Fatalf("expected StateFullPower, got %d", m.state.Load())
	}
}

func TestManagerObj_TouchActivity(t *testing.T) {
	m, _, cancel := newTestLPM(t, 60*time.Second)
	defer cancel()

	// Set lastActivity to a known past timestamp
	past := time.Now().Add(-10 * time.Second).Unix()
	m.lastActivityAt.Store(past)
	m.touchActivity()
	after := m.lastActivityAt.Load()
	if after <= past {
		t.Fatalf("touchActivity should advance timestamp: past=%d, after=%d", past, after)
	}
}

func TestManagerObj_IdleSeconds(t *testing.T) {
	m, _, cancel := newTestLPM(t, 60*time.Second)
	defer cancel()

	// Right after creation idle should be ~0
	idle := m.idleSeconds()
	if idle > 2 {
		t.Fatalf("expected idle ~0, got %d", idle)
	}

	// Shift lastActivity into the past
	m.lastActivityAt.Store(time.Now().Add(-30 * time.Second).Unix())
	idle = m.idleSeconds()
	if idle < 28 || idle > 32 {
		t.Fatalf("expected idle ~30, got %d", idle)
	}
}

// //

func TestManagerObj_TransitionToLowPower(t *testing.T) {
	m, node, cancel := newTestLPM(t, 1*time.Second)
	defer cancel()

	// Without socksAddr — transitions directly to StateLowPower
	m.transitionToLowPower()
	if m.state.Load() != StateLowPower {
		t.Fatalf("expected StateLowPower, got %d", m.state.Load())
	}
	if node.stopCalled.Load() != 1 {
		t.Fatalf("expected StopComponents called once, got %d", node.stopCalled.Load())
	}
}

func TestManagerObj_TransitionToLowPower_Idempotent(t *testing.T) {
	m, _, cancel := newTestLPM(t, 1*time.Second)
	defer cancel()

	m.transitionToLowPower()
	// Repeated call must not panic
	m.transitionToLowPower()
	if m.state.Load() != StateLowPower {
		t.Fatalf("expected StateLowPower, got %d", m.state.Load())
	}
}

func TestManagerObj_TransitionToLowPower_OnlyFromFullPower(t *testing.T) {
	m, _, cancel := newTestLPM(t, 1*time.Second)
	defer cancel()

	// Set state to Starting — transitionToLowPower must be a no-op
	m.state.Store(StateStarting)
	m.transitionToLowPower()
	if m.state.Load() != StateStarting {
		t.Fatalf("state should remain StateStarting")
	}
}

// //

func TestManagerObj_Stop(t *testing.T) {
	m, _, cancel := newTestLPM(t, 60*time.Second)
	_ = cancel

	m.Stop()

	select {
	case <-m.ctx.Done():
		// ok
	default:
		t.Fatal("context should be cancelled after stop")
	}
}

// //

func TestManagerObj_IsLowPower(t *testing.T) {
	m, _, cancel := newTestLPM(t, 60*time.Second)
	defer cancel()

	m.state.Store(StateFullPower)
	if m.IsLowPower() {
		t.Fatal("StateFullPower should not be low power")
	}

	m.state.Store(StateStopping)
	if !m.IsLowPower() {
		t.Fatal("StateStopping should be low power")
	}

	m.state.Store(StateLowPower)
	if !m.IsLowPower() {
		t.Fatal("StateLowPower should be low power")
	}

	m.state.Store(StateStarting)
	if m.IsLowPower() {
		t.Fatal("StateStarting should not be low power")
	}
}

// //

func TestManagerObj_TransitionOnStaleActivity(t *testing.T) {
	m, _, cancel := newTestLPM(t, 1*time.Millisecond)
	defer cancel()

	// Shift lastActivity far into the past so idle exceeds timeout
	m.lastActivityAt.Store(time.Now().Add(-1 * time.Hour).Unix())

	m.transitionToLowPower()
	if m.state.Load() != StateLowPower {
		t.Fatalf("expected StateLowPower after idle, got %d", m.state.Load())
	}
}

func TestManagerObj_ActivityPreventsIdle(t *testing.T) {
	m, _, cancel := newTestLPM(t, 60*time.Second)
	defer cancel()

	// Activity just happened
	m.touchActivity()
	idle := m.idleSeconds()
	if idle >= int64(m.cfg.IdleTimeout.Seconds()) {
		t.Fatal("should not exceed idle timeout right after activity")
	}
}

// //

func TestManagerObj_SetOrigConfig(t *testing.T) {
	m, _, cancel := newTestLPM(t, 60*time.Second)
	defer cancel()

	cfg := map[string]string{"key": "value"}
	m.SetOrigConfig(cfg)

	stored, ok := m.OrigConfig().(map[string]string)
	if !ok {
		t.Fatal("SetOrigConfig should store the value as-is")
	}
	if stored["key"] != "value" {
		t.Fatal("SetOrigConfig should preserve the stored value")
	}
}

// //

func TestManagerObj_WakeTrigger_AcceptsConnection(t *testing.T) {
	m, _, cancel := newTestLPM(t, 60*time.Second)
	defer cancel()
	m.socksWaitTimeout = 200 * time.Millisecond

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %s", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	if err := m.startWakeTrigger(addr); err != nil {
		t.Fatalf("startWakeTrigger: %s", err)
	}
	defer m.stopWakeTrigger()

	// Wake trigger should accept the connection
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("should connect to wake trigger: %s", err)
	}
	conn.Close()
}

func TestManagerObj_WakeTrigger_ProxiesViaSocks(t *testing.T) {
	// Emulate SOCKS: echo server on a random port
	socksLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %s", err)
	}
	defer socksLn.Close()
	socksAddr := socksLn.Addr().String()

	readyCh := make(chan struct{})
	close(readyCh)

	node := &mockNodeControlObj{
		socksAddr:    socksAddr,
		socksReadyCh: readyCh,
	}
	cfg := ConfigObj{IdleTimeout: 60 * time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewManager(node, cfg, ctx, noopLoggerObj{})

	go func() {
		for {
			c, err := socksLn.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 256)
				n, err := c.Read(buf)
				if err != nil {
					return
				}
				c.Write(buf[:n])
			}(c)
		}
	}()

	// Wake trigger on a separate port
	triggerLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %s", err)
	}
	triggerAddr := triggerLn.Addr().String()
	triggerLn.Close()

	if err := m.startWakeTrigger(triggerAddr); err != nil {
		t.Fatalf("startWakeTrigger: %s", err)
	}
	defer m.stopWakeTrigger()

	// Connect to wake trigger — data should be proxied through SOCKS
	conn, err := net.DialTimeout("tcp", triggerAddr, 2*time.Second)
	if err != nil {
		t.Fatalf("should connect to wake trigger: %s", err)
	}
	defer conn.Close()

	testMsg := []byte("hello-proxy")
	_, err = conn.Write(testMsg)
	if err != nil {
		t.Fatalf("write: %s", err)
	}

	buf := make([]byte, 256)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read: %s", err)
	}
	if string(buf[:n]) != "hello-proxy" {
		t.Fatalf("expected echo, got %q", string(buf[:n]))
	}
}

func TestManagerObj_WakeTrigger_UnixSocket(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := tmpDir + "/test.sock"

	node := &mockNodeControlObj{
		socksIsUnix:  true,
		socksAddr:    sockPath,
		socksReadyCh: make(chan struct{}),
	}
	cfg := ConfigObj{IdleTimeout: 60 * time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewManager(node, cfg, ctx, noopLoggerObj{})
	m.socksWaitTimeout = 200 * time.Millisecond

	if err := m.startWakeTrigger(sockPath); err != nil {
		t.Fatalf("startWakeTrigger: %s", err)
	}
	defer m.stopWakeTrigger()

	conn, err := net.DialTimeout("unix", sockPath, 2*time.Second)
	if err != nil {
		t.Fatalf("should connect to UNIX wake trigger: %s", err)
	}
	conn.Close()
}

func TestManagerObj_StopWakeTrigger(t *testing.T) {
	m, _, cancel := newTestLPM(t, 60*time.Second)
	defer cancel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %s", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	if err := m.startWakeTrigger(addr); err != nil {
		t.Fatalf("startWakeTrigger: %s", err)
	}
	m.stopWakeTrigger()

	_, err = net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err == nil {
		t.Fatal("should not connect after wake trigger stopped")
	}
}
