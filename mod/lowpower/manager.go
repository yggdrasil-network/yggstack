package lowpower

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yggdrasil-network/yggdrasil-go/src/core"
)

// // // // // // // // // //

// ManagerObj manages automatic sleep/wake transitions of the node.
// All state transitions are driven by Run() — the single owning goroutine.
// External callers request wake via TransitionToFullPower which sends to wakeCh.
type ManagerObj struct {
	node   NodeControlInterface
	cfg    ConfigObj
	state  atomic.Int32
	ctx    context.Context
	cancel context.CancelFunc
	logger core.Logger

	wakeCh chan struct{} // buffered(1): request wake from any goroutine without blocking

	origCfgMu sync.RWMutex
	origCfg   interface{} // opaque config passed back to StartComponents on wake

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
		wakeCh:           make(chan struct{}, 1),
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

// Run is the sole owner of state transitions.
// Must be run in a dedicated goroutine.
func (m *ManagerObj) Run() {
	ticker := time.NewTicker(idleCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return

		case <-m.wakeCh:
			// Wake request from TransitionToFullPower — process regardless of current state.
			if m.state.Load() != StateFullPower {
				m.doTransitionToFullPower()
			}

		case <-ticker.C:
			if m.state.Load() != StateFullPower {
				continue
			}
			// Refresh lastActivity while there are active connections.
			if m.node.ConnCounter().Count() > 0 {
				m.touchActivity()
				continue
			}
			if time.Duration(m.idleSeconds())*time.Second >= m.cfg.IdleTimeout {
				m.transitionToLowPower()
				// Process any wake signal that arrived while we were stopping.
				select {
				case <-m.wakeCh:
					m.doTransitionToFullPower()
				default:
				}
			}
		}
	}
}

// //

// transitionToLowPower stops components and enters sleep.
// Must be called only from Run(). No-op if not in StateFullPower.
func (m *ManagerObj) transitionToLowPower() {
	if m.state.Load() != StateFullPower {
		return
	}
	m.state.Store(StateStopping)
	m.logger.Infof("Low power mode: entering sleep after %s idle", m.cfg.IdleTimeout)

	socksAddr := m.node.SocksAddr()

	m.node.StopComponents()

	// Wake trigger on SOCKS port: wake the node when a client connects.
	if socksAddr != "" {
		m.startWakeTrigger(socksAddr)
	}

	m.state.Store(StateLowPower)
	m.logger.Infof("Low power mode: sleeping")
}

// doTransitionToFullPower wakes the node.
// Must be called only from Run().
func (m *ManagerObj) doTransitionToFullPower() {
	m.state.Store(StateStarting)
	m.logger.Infof("Low power mode: waking up")

	// Close listener — no new connections accepted,
	// but in-flight handleWakeConnection goroutines continue and wait for SOCKS.
	m.closeWakeListener()

	if err := m.node.StartComponents(m.getOrigCfg()); err != nil {
		m.logger.Errorf("Low power mode: failed to restart components: %s", err)
		m.state.Store(StateLowPower)
		return
	}

	m.touchActivity()
	m.state.Store(StateFullPower)
	m.logger.Infof("Low power mode: fully awake")
}

// //

// TransitionToFullPower signals Run() to wake the node.
// Non-blocking: if a wake is already queued, the call is a no-op.
// Safe to call from any goroutine.
func (m *ManagerObj) TransitionToFullPower() {
	select {
	case m.wakeCh <- struct{}{}:
	default:
	}
}

// //

// IsLowPower returns true if the node is sleeping or entering sleep.
func (m *ManagerObj) IsLowPower() bool {
	s := m.state.Load()
	return s == StateLowPower || s == StateStopping
}

// SetOrigConfig stores the configuration for restart on wake.
// Safe to call from any goroutine.
func (m *ManagerObj) SetOrigConfig(cfg interface{}) {
	m.origCfgMu.Lock()
	m.origCfg = cfg
	m.origCfgMu.Unlock()
}

// OrigConfig returns the stored restart configuration.
func (m *ManagerObj) OrigConfig() interface{} {
	return m.getOrigCfg()
}

func (m *ManagerObj) getOrigCfg() interface{} {
	m.origCfgMu.RLock()
	defer m.origCfgMu.RUnlock()
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
