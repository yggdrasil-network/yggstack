package yggstack

import (
	"sync/atomic"
	"testing"

	"github.com/yggdrasil-network/yggstack/mod/lowpower"
)

// // // // // // // // // //

// mockLPMObj implements lowpower.ManagerInterface for testing Obj wrappers.
type mockLPMObj struct {
	state      atomic.Int32
	origCfg    interface{}
	wakeCalled atomic.Int32
	stopCalled atomic.Int32
}

func (m *mockLPMObj) Run()  {}
func (m *mockLPMObj) Stop() { m.stopCalled.Add(1) }
func (m *mockLPMObj) IsLowPower() bool {
	s := m.state.Load()
	return s == lowpower.StateLowPower || s == lowpower.StateStopping
}
func (m *mockLPMObj) TransitionToFullPower()        { m.wakeCalled.Add(1) }
func (m *mockLPMObj) SetOrigConfig(cfg interface{}) { m.origCfg = cfg }
func (m *mockLPMObj) OrigConfig() interface{}       { return m.origCfg }
func (m *mockLPMObj) GetState() int32               { return m.state.Load() }

// //

func TestIsLowPower_NilManager(t *testing.T) {
	obj := &Obj{}
	if obj.IsLowPower() {
		t.Fatal("should return false when lowPower is nil")
	}
}

func TestIsLowPower_States(t *testing.T) {
	m := &mockLPMObj{}
	obj := &Obj{lowPower: m}

	m.state.Store(lowpower.StateFullPower)
	if obj.IsLowPower() {
		t.Fatal("StateFullPower should not be low power")
	}

	m.state.Store(lowpower.StateStopping)
	if !obj.IsLowPower() {
		t.Fatal("StateStopping should be low power")
	}

	m.state.Store(lowpower.StateLowPower)
	if !obj.IsLowPower() {
		t.Fatal("StateLowPower should be low power")
	}

	m.state.Store(lowpower.StateStarting)
	if obj.IsLowPower() {
		t.Fatal("StateStarting should not be low power")
	}
}

func TestActiveConnections(t *testing.T) {
	obj := &Obj{}
	if obj.ActiveConnections() != 0 {
		t.Fatal("should be 0 initially")
	}
	obj.connCounter.Increment()
	obj.connCounter.Increment()
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
	m := &mockLPMObj{}
	m.state.Store(lowpower.StateFullPower)

	obj := &Obj{lowPower: m}
	err := obj.WakeLowPower()
	if err != nil {
		t.Fatalf("should not return error: %s", err)
	}
	if m.wakeCalled.Load() != 0 {
		t.Fatal("TransitionToFullPower should not be called when not sleeping")
	}
}

func TestWakeLowPower_Sleeping(t *testing.T) {
	m := &mockLPMObj{}
	m.state.Store(lowpower.StateLowPower)

	obj := &Obj{lowPower: m}
	err := obj.WakeLowPower()
	if err != nil {
		t.Fatalf("should not return error: %s", err)
	}
	if m.wakeCalled.Load() != 1 {
		t.Fatal("TransitionToFullPower should be called when sleeping")
	}
}

// //

func TestSetOrigConfig_NilsCtx(t *testing.T) {
	m := &mockLPMObj{}

	cfg := ConfigObj{
		SocksAddr: "127.0.0.1:1080",
	}
	m.SetOrigConfig(cfg)

	stored := m.OrigConfig().(ConfigObj)
	if stored.SocksAddr != "127.0.0.1:1080" {
		t.Fatal("fields should be preserved")
	}
}
