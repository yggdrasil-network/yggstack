package lowpower

import (
	"net"
	"os"
	"sync"
	"time"

	"github.com/yggdrasil-network/yggstack/src/types"
)

// // // // // // // // // //

// wakeTriggerObj listens on the SOCKS port while the node sleeps.
// On incoming connection, wakes the node and proxies the connection through SOCKS.
type wakeTriggerObj struct {
	listener net.Listener
	mu       sync.Mutex
	wg       sync.WaitGroup
}

// //

const (
	wakeSOCKSWaitTimeout = 10 * time.Second
	wakeSOCKSDialTimeout = 5 * time.Second
)

// //

func (m *ManagerObj) startWakeTrigger(addr string) {
	network := "tcp"
	if m.node.SocksIsUnix() {
		network = "unix"
		// Remove stale socket left over from SOCKS
		_ = os.Remove(addr)
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

// handleWakeConnection wakes the node, waits for SOCKS readiness and proxies the connection.
func (m *ManagerObj) handleWakeConnection(conn net.Conn) {
	defer m.wakeTrigger.wg.Done()
	defer conn.Close()

	m.TransitionToFullPower()

	// Wait for SOCKS listener to become ready via channel
	network := "tcp"
	if m.node.SocksIsUnix() {
		network = "unix"
	}

	select {
	case <-m.node.SocksReadyCh():
		// SOCKS is ready
	case <-time.After(m.socksWaitTimeout):
		m.logger.Errorf("Low power mode: SOCKS not ready after %s, dropping wake connection", m.socksWaitTimeout)
		return
	case <-m.ctx.Done():
		return
	}

	socksConn, err := net.DialTimeout(network, m.node.SocksAddr(), wakeSOCKSDialTimeout)
	if err != nil {
		m.logger.Errorf("Low power mode: failed to connect to SOCKS %s: %s", m.node.SocksAddr(), err)
		return
	}
	defer socksConn.Close()

	types.ProxyTCP(conn, socksConn)
}

// closeWakeListener closes the listener without waiting for goroutines.
// Used on wake — no new connections accepted,
// but in-flight handleWakeConnection goroutines continue.
func (m *ManagerObj) closeWakeListener() {
	m.wakeTrigger.mu.Lock()
	if m.wakeTrigger.listener != nil {
		_ = m.wakeTrigger.listener.Close()
		if m.node.SocksIsUnix() {
			_ = os.Remove(m.node.SocksAddr())
		}
		m.wakeTrigger.listener = nil
	}
	m.wakeTrigger.mu.Unlock()
}

// stopWakeTrigger closes the listener and waits for all handleWakeConnection goroutines to finish.
func (m *ManagerObj) stopWakeTrigger() {
	m.closeWakeListener()
	m.wakeTrigger.wg.Wait()
}
