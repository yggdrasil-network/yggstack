package yggstack

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yggdrasil-network/yggdrasil-go/src/core"

	"github.com/yggdrasil-network/yggstack/src/types"
)

// // // // // // // // // //

type lowPowerStateObj int32

const (
	stateFullPower lowPowerStateObj = iota
	stateStopping
	stateLowPower
	stateStarting
)

const (
	lpmIdleCheckInterval = 10 * time.Second
)

// //

// wakeTriggerObj listens on the SOCKS port while the node sleeps.
// On incoming connection, wakes the node and proxies the connection through SOCKS.
type wakeTriggerObj struct {
	listener net.Listener
	mu       sync.Mutex
	wg sync.WaitGroup
}

// //

// lowPowerManagerObj manages automatic sleep/wake transitions of the node.
type lowPowerManagerObj struct {
	obj     *Obj
	cfg     LowPowerConfigObj
	origCfg ConfigObj // Configuration for restart
	state   atomic.Int32
	ctx     context.Context
	cancel  context.CancelFunc
	logger  core.Logger

	wakeTrigger      wakeTriggerObj
	socksWaitTimeout time.Duration // SOCKS readiness timeout on wake (default 10s)

	lastActivityAt atomic.Int64 // Unix timestamp of last activity
}

func newLowPowerManager(obj *Obj, cfg LowPowerConfigObj, ctx context.Context, cancel context.CancelFunc, logger core.Logger) *lowPowerManagerObj {
	m := &lowPowerManagerObj{
		obj:              obj,
		cfg:              cfg,
		ctx:              ctx,
		cancel:           cancel,
		logger:           logger,
		socksWaitTimeout: wakeSOCKSWaitTimeout,
	}
	m.state.Store(int32(stateFullPower))
	m.touchActivity()
	return m
}

func (m *lowPowerManagerObj) touchActivity() {
	m.lastActivityAt.Store(time.Now().Unix())
}

func (m *lowPowerManagerObj) idleSeconds() int64 {
	return time.Now().Unix() - m.lastActivityAt.Load()
}

// //

func (m *lowPowerManagerObj) run() {
	ticker := time.NewTicker(lpmIdleCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			if lowPowerStateObj(m.state.Load()) != stateFullPower {
				continue
			}
			// Refresh lastActivity while there are active connections
			if m.obj.connCounter.count() > 0 {
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

func (m *lowPowerManagerObj) transitionToLowPower() {
	if !m.state.CompareAndSwap(int32(stateFullPower), int32(stateStopping)) {
		return
	}
	m.logger.Infof("Low power mode: entering sleep after %s idle", m.cfg.IdleTimeout)

	socksAddr := m.obj.socksAddr

	m.obj.stopComponents()

	// Wake trigger on SOCKS port: wake the node when a client connects
	if socksAddr != "" {
		m.startWakeTrigger(socksAddr)
	}

	m.state.Store(int32(stateLowPower))
	m.logger.Infof("Low power mode: sleeping")
}

func (m *lowPowerManagerObj) transitionToFullPower() {
	if !m.state.CompareAndSwap(int32(stateLowPower), int32(stateStarting)) {
		return
	}
	m.logger.Infof("Low power mode: waking up")

	// Close listener — no new connections accepted,
	// but in-flight handleWakeConnection goroutines continue and wait for SOCKS
	m.closeWakeListener()

	if err := m.obj.startComponents(m.origCfg); err != nil {
		m.logger.Errorf("Low power mode: failed to restart components: %s", err)
		m.state.Store(int32(stateLowPower))
		return
	}

	m.touchActivity()
	m.state.Store(int32(stateFullPower))
	m.logger.Infof("Low power mode: fully awake")
}

// //

func (m *lowPowerManagerObj) startWakeTrigger(addr string) {
	network := "tcp"
	if m.obj.socksIsUnix {
		network = "unix"
		// Remove stale socket left over from SOCKS
		_ = os.RemoveAll(addr)
	}
	listener, err := net.Listen(network, addr)
	if err != nil {
		m.logger.Errorf("Low power mode: failed to start wake trigger on %s %s: %s", network, addr, err)
		return
	}
	m.wakeTrigger.mu.Lock()
	m.wakeTrigger.listener = listener
	m.wakeTrigger.mu.Unlock()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			// Check under lock that listener is still alive — prevents wg.Add racing with wg.Wait
			m.wakeTrigger.mu.Lock()
			if m.wakeTrigger.listener == nil {
				m.wakeTrigger.mu.Unlock()
				conn.Close()
				return
			}
			m.wakeTrigger.wg.Add(1)
			m.wakeTrigger.mu.Unlock()
			go m.handleWakeConnection(conn)
		}
	}()
}

const (
	wakeSOCKSWaitTimeout = 10 * time.Second
	wakeSOCKSDialTimeout = 5 * time.Second
)

// handleWakeConnection wakes the node, waits for SOCKS readiness and proxies the connection.
func (m *lowPowerManagerObj) handleWakeConnection(conn net.Conn) {
	defer m.wakeTrigger.wg.Done()
	defer conn.Close()

	m.transitionToFullPower()

	// Wait for SOCKS listener to become ready via channel
	network := "tcp"
	if m.obj.socksIsUnix {
		network = "unix"
	}

	select {
	case <-m.obj.socksReadyCh:
		// SOCKS is ready
	case <-time.After(m.socksWaitTimeout):
		m.logger.Errorf("Low power mode: SOCKS not ready after %s, dropping wake connection", m.socksWaitTimeout)
		return
	case <-m.ctx.Done():
		return
	}

	socksConn, err := net.DialTimeout(network, m.obj.socksAddr, wakeSOCKSDialTimeout)
	if err != nil {
		m.logger.Errorf("Low power mode: failed to connect to SOCKS %s: %s", m.obj.socksAddr, err)
		return
	}
	defer socksConn.Close()

	types.ProxyTCP(conn, socksConn)
}

// closeWakeListener closes the listener without waiting for goroutines.
// Used on wake — no new connections accepted,
// but in-flight handleWakeConnection goroutines continue.
func (m *lowPowerManagerObj) closeWakeListener() {
	m.wakeTrigger.mu.Lock()
	if m.wakeTrigger.listener != nil {
		_ = m.wakeTrigger.listener.Close()
		if m.obj.socksIsUnix {
			_ = os.RemoveAll(m.obj.socksAddr)
		}
		m.wakeTrigger.listener = nil
	}
	m.wakeTrigger.mu.Unlock()
}

// stopWakeTrigger closes the listener and waits for all handleWakeConnection goroutines to finish.
func (m *lowPowerManagerObj) stopWakeTrigger() {
	m.closeWakeListener()
	m.wakeTrigger.wg.Wait()
}

func (m *lowPowerManagerObj) stop() {
	m.cancel()
	m.stopWakeTrigger()
}

// setOrigConfig stores the configuration for restart.
// Ctx is set to nil — on wake o.ctx is used instead.
func (m *lowPowerManagerObj) setOrigConfig(cfg ConfigObj) {
	cfg.Ctx = nil
	m.origCfg = cfg
}

// //

// IsLowPower returns true if the node is in sleep mode.
func (o *Obj) IsLowPower() bool {
	if o.lowPower == nil {
		return false
	}
	state := lowPowerStateObj(o.lowPower.state.Load())
	return state == stateLowPower || state == stateStopping
}

// ActiveConnections returns the number of tracked active connections.
func (o *Obj) ActiveConnections() int64 {
	return o.connCounter.count()
}

// WakeLowPower forces the node to wake up from sleep mode.
func (o *Obj) WakeLowPower() error {
	if o.lowPower == nil {
		return fmt.Errorf("low power mode is not enabled")
	}
	if lowPowerStateObj(o.lowPower.state.Load()) == stateLowPower {
		o.lowPower.transitionToFullPower()
		return nil
	}
	return nil
}
