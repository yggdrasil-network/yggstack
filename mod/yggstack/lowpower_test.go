package yggstack

import (
	"context"
	"net"
	"testing"
	"time"
)

// // // // // // // // // //

func newTestLPM(t *testing.T, idleTimeout time.Duration) (*lowPowerManagerObj, context.CancelFunc) {
	t.Helper()
	obj := &Obj{}
	cfg := LowPowerConfigObj{
		IdleTimeout: idleTimeout,
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := newLowPowerManager(obj, cfg, ctx, cancel, noopLoggerObj{})
	return m, cancel
}

// //

func TestLowPowerStateObj_InitialState(t *testing.T) {
	m, cancel := newTestLPM(t, 60*time.Second)
	defer cancel()

	if lowPowerStateObj(m.state.Load()) != stateFullPower {
		t.Fatalf("expected stateFullPower, got %d", m.state.Load())
	}
}

func TestLowPowerManagerObj_TouchActivity(t *testing.T) {
	m, cancel := newTestLPM(t, 60*time.Second)
	defer cancel()

	before := m.lastActivityAt.Load()
	time.Sleep(10 * time.Millisecond)
	m.touchActivity()
	after := m.lastActivityAt.Load()
	if after < before {
		t.Fatalf("touchActivity should advance timestamp: before=%d, after=%d", before, after)
	}
}

func TestLowPowerManagerObj_IdleSeconds(t *testing.T) {
	m, cancel := newTestLPM(t, 60*time.Second)
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

func TestLowPowerManagerObj_TransitionToLowPower(t *testing.T) {
	m, cancel := newTestLPM(t, 1*time.Second)
	defer cancel()

	// Without socksAddr — transitions directly to stateLowPower
	m.transitionToLowPower()
	state := lowPowerStateObj(m.state.Load())
	if state != stateLowPower {
		t.Fatalf("expected stateLowPower, got %d", state)
	}
}

func TestLowPowerManagerObj_TransitionToLowPower_Idempotent(t *testing.T) {
	m, cancel := newTestLPM(t, 1*time.Second)
	defer cancel()

	m.transitionToLowPower()
	// Repeated call must not panic
	m.transitionToLowPower()
	state := lowPowerStateObj(m.state.Load())
	if state != stateLowPower {
		t.Fatalf("expected stateLowPower, got %d", state)
	}
}

func TestLowPowerManagerObj_TransitionToLowPower_OnlyFromFullPower(t *testing.T) {
	m, cancel := newTestLPM(t, 1*time.Second)
	defer cancel()

	// Set state to Starting — transitionToLowPower must be a no-op
	m.state.Store(int32(stateStarting))
	m.transitionToLowPower()
	if lowPowerStateObj(m.state.Load()) != stateStarting {
		t.Fatalf("state should remain stateStarting")
	}
}

// //

func TestLowPowerManagerObj_Stop(t *testing.T) {
	m, cancel := newTestLPM(t, 60*time.Second)
	_ = cancel

	m.stop()

	select {
	case <-m.ctx.Done():
		// ok
	default:
		t.Fatal("context should be cancelled after stop")
	}
}

// //

func TestIsLowPower_NilManager(t *testing.T) {
	obj := &Obj{}
	if obj.IsLowPower() {
		t.Fatal("should return false when lowPower is nil")
	}
}

func TestIsLowPower_States(t *testing.T) {
	m, cancel := newTestLPM(t, 60*time.Second)
	defer cancel()

	obj := &Obj{lowPower: m}

	m.state.Store(int32(stateFullPower))
	if obj.IsLowPower() {
		t.Fatal("stateFullPower should not be low power")
	}

	m.state.Store(int32(stateStopping))
	if !obj.IsLowPower() {
		t.Fatal("stateStopping should be low power")
	}

	m.state.Store(int32(stateLowPower))
	if !obj.IsLowPower() {
		t.Fatal("stateLowPower should be low power")
	}

	m.state.Store(int32(stateStarting))
	if obj.IsLowPower() {
		t.Fatal("stateStarting should not be low power")
	}
}

func TestActiveConnections(t *testing.T) {
	obj := &Obj{}
	if obj.ActiveConnections() != 0 {
		t.Fatal("should be 0 initially")
	}
	obj.connCounter.increment()
	obj.connCounter.increment()
	if obj.ActiveConnections() != 2 {
		t.Fatalf("expected 2, got %d", obj.ActiveConnections())
	}
}

func TestWakeLowPower_NilManager(t *testing.T) {
	obj := &Obj{}
	err := obj.WakeLowPower()
	if err == nil {
		t.Fatal("should return error when lowPower is nil")
	}
}

func TestWakeLowPower_NotSleeping(t *testing.T) {
	m, cancel := newTestLPM(t, 60*time.Second)
	defer cancel()

	obj := &Obj{lowPower: m}
	// In FullPower state — WakeLowPower is a no-op
	err := obj.WakeLowPower()
	if err != nil {
		t.Fatalf("should not return error: %s", err)
	}
}

// //

func TestLowPowerManagerObj_RunIdleDetection(t *testing.T) {
	obj := &Obj{}
	cfg := LowPowerConfigObj{
		IdleTimeout: 1 * time.Millisecond,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := newLowPowerManager(obj, cfg, ctx, cancel, noopLoggerObj{})
	// Shift lastActivity far into the past
	m.lastActivityAt.Store(time.Now().Add(-1 * time.Hour).Unix())

	// Call transitionToLowPower directly (run() uses 10s ticker)
	m.transitionToLowPower()
	if lowPowerStateObj(m.state.Load()) != stateLowPower {
		t.Fatalf("expected stateLowPower after idle, got %d", m.state.Load())
	}
}

func TestLowPowerManagerObj_ActivityPreventsIdle(t *testing.T) {
	m, cancel := newTestLPM(t, 60*time.Second)
	defer cancel()

	// Activity just happened
	m.touchActivity()
	idle := m.idleSeconds()
	if idle >= int64(m.cfg.IdleTimeout.Seconds()) {
		t.Fatal("should not exceed idle timeout right after activity")
	}
}

// //

func TestLowPowerManagerObj_WakeTrigger_AcceptsConnection(t *testing.T) {
	m, cancel := newTestLPM(t, 60*time.Second)
	defer cancel()
	m.socksWaitTimeout = 200 * time.Millisecond

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %s", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	m.startWakeTrigger(addr)
	defer m.stopWakeTrigger()

	// Wake trigger should accept the connection
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("should connect to wake trigger: %s", err)
	}
	conn.Close()
}

func TestLowPowerManagerObj_WakeTrigger_ProxiesViaSocks(t *testing.T) {
	obj := &Obj{logger: noopLoggerObj{}}
	cfg := LowPowerConfigObj{IdleTimeout: 60 * time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := newLowPowerManager(obj, cfg, ctx, cancel, noopLoggerObj{})

	// Emulate SOCKS: echo server on a random port
	socksLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %s", err)
	}
	defer socksLn.Close()
	socksAddr := socksLn.Addr().String()
	obj.socksAddr = socksAddr

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

	// Signal via channel that SOCKS is ready
	obj.socksReadyCh = make(chan struct{})
	close(obj.socksReadyCh)

	// Wake trigger on a separate port
	triggerLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %s", err)
	}
	triggerAddr := triggerLn.Addr().String()
	triggerLn.Close()

	m.startWakeTrigger(triggerAddr)
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

func TestLowPowerManagerObj_WakeTrigger_UnixSocket(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := tmpDir + "/test.sock"

	obj := &Obj{
		logger:      noopLoggerObj{},
		socksIsUnix: true,
		socksAddr:   sockPath,
	}
	cfg := LowPowerConfigObj{IdleTimeout: 60 * time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := newLowPowerManager(obj, cfg, ctx, cancel, noopLoggerObj{})
	m.socksWaitTimeout = 200 * time.Millisecond

	m.startWakeTrigger(sockPath)
	defer m.stopWakeTrigger()

	conn, err := net.DialTimeout("unix", sockPath, 2*time.Second)
	if err != nil {
		t.Fatalf("should connect to UNIX wake trigger: %s", err)
	}
	conn.Close()
}

func TestLowPowerManagerObj_StopWakeTrigger(t *testing.T) {
	m, cancel := newTestLPM(t, 60*time.Second)
	defer cancel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %s", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	m.startWakeTrigger(addr)
	m.stopWakeTrigger()

	_, err = net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err == nil {
		t.Fatal("should not connect after wake trigger stopped")
	}
}

func TestLowPowerManagerObj_SetOrigConfig_NilsCtx(t *testing.T) {
	m, cancel := newTestLPM(t, 60*time.Second)
	defer cancel()

	cfg := ConfigObj{
		Ctx:       context.Background(),
		SocksAddr: "127.0.0.1:1080",
	}
	m.setOrigConfig(cfg)

	if m.origCfg.Ctx != nil {
		t.Fatal("setOrigConfig should nil out Ctx")
	}
	if m.origCfg.SocksAddr != "127.0.0.1:1080" {
		t.Fatal("setOrigConfig should preserve other fields")
	}
}
