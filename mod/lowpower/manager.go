package lowpower

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/yggdrasil-network/yggdrasil-go/src/core"
)

// // // // // // // // // //

// ManagerObj manages automatic sleep/wake transitions of the node.
type ManagerObj struct {
	node    NodeControlInterface
	cfg     ConfigObj
	origCfg interface{} // opaque config passed back to StartComponents on wake
	state   atomic.Int32
	ctx     context.Context
	cancel  context.CancelFunc
	logger  core.Logger

	wakeTrigger      wakeTriggerObj
	socksWaitTimeout time.Duration // SOCKS readiness timeout on wake (default 10s)

	lastActivityAt atomic.Int64 // Unix timestamp of last activity
}

// //

func NewManager(node NodeControlInterface, cfg ConfigObj, ctx context.Context, cancel context.CancelFunc, logger core.Logger) *ManagerObj {
	m := &ManagerObj{
		node:             node,
		cfg:              cfg,
		ctx:              ctx,
		cancel:           cancel,
		logger:           logger,
		socksWaitTimeout: wakeSOCKSWaitTimeout,
	}
	m.state.Store(StateFullPower)
	m.touchActivity()
	return m
}

func (m *ManagerObj) touchActivity() {
	m.lastActivityAt.Store(time.Now().Unix())
}

func (m *ManagerObj) idleSeconds() int64 {
	return time.Now().Unix() - m.lastActivityAt.Load()
}

// //

func (m *ManagerObj) Run() {
	ticker := time.NewTicker(idleCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			if m.state.Load() != StateFullPower {
				continue
			}
			// Refresh lastActivity while there are active connections
			if m.node.ConnCounter().Count() > 0 {
				m.touchActivity()
				continue
			}
			if time.Duration(m.idleSeconds())*time.Second >= m.cfg.IdleTimeout {
				m.transitionToLowPower()
			}
		}
	}
}

// //

func (m *ManagerObj) transitionToLowPower() {
	if !m.state.CompareAndSwap(StateFullPower, StateStopping) {
		return
	}
	m.logger.Infof("Low power mode: entering sleep after %s idle", m.cfg.IdleTimeout)

	socksAddr := m.node.SocksAddr()

	m.node.StopComponents()

	// Wake trigger on SOCKS port: wake the node when a client connects
	if socksAddr != "" {
		m.startWakeTrigger(socksAddr)
	}

	m.state.Store(StateLowPower)
	m.logger.Infof("Low power mode: sleeping")
}

func (m *ManagerObj) TransitionToFullPower() {
	if !m.state.CompareAndSwap(StateLowPower, StateStarting) {
		return
	}
	m.logger.Infof("Low power mode: waking up")

	// Close listener — no new connections accepted,
	// but in-flight handleWakeConnection goroutines continue and wait for SOCKS
	m.closeWakeListener()

	if err := m.node.StartComponents(m.origCfg); err != nil {
		m.logger.Errorf("Low power mode: failed to restart components: %s", err)
		m.state.Store(StateLowPower)
		return
	}

	m.touchActivity()
	m.state.Store(StateFullPower)
	m.logger.Infof("Low power mode: fully awake")
}

// //

// IsLowPower returns true if the node is in sleep or stopping state.
func (m *ManagerObj) IsLowPower() bool {
	s := m.state.Load()
	return s == StateLowPower || s == StateStopping
}

// SetOrigConfig stores the configuration for restart on wake.
func (m *ManagerObj) SetOrigConfig(cfg interface{}) {
	m.origCfg = cfg
}

// OrigConfig returns the stored restart configuration.
func (m *ManagerObj) OrigConfig() interface{} {
	return m.origCfg
}

// GetState returns the current low power state as int32.
func (m *ManagerObj) GetState() int32 {
	return m.state.Load()
}

func (m *ManagerObj) Stop() {
	m.cancel()
	m.stopWakeTrigger()
}
