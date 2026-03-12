package lowpower

import (
	"fmt"
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

func (m *ManagerObj) startWakeTrigger(addr string) error {
	network := "tcp"
	if m.node.SocksIsUnix() {
		network = "unix"
		// Remove stale socket left over from SOCKS
		_ = os.Remove(addr)
	}
	listener, err := net.Listen(network, addr)
	if err != nil {
		return fmt.Errorf("wake trigger listen %s %s: %w", network, addr, err)
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
	return nil
}

// handleWakeConnection wakes the node, waits for SOCKS readiness and proxies the connection.
// ProxyTCP closes both connections, so early-return paths close conn explicitly.
func (m *ManagerObj) handleWakeConnection(conn net.Conn) {
	defer m.wakeTrigger.wg.Done()

	m.TransitionToFullPower()

	network := "tcp"
	if m.node.SocksIsUnix() {
		network = "unix"
	}

	select {
	case <-m.node.SocksReadyCh():
	case <-time.After(m.socksWaitTimeout):
		m.logger.Errorf("Low power mode: SOCKS not ready after %s, dropping wake connection", m.socksWaitTimeout)
		_ = conn.Close()
		return
	case <-m.ctx.Done():
		_ = conn.Close()
		return
	}

	socksConn, err := net.DialTimeout(network, m.node.SocksAddr(), wakeSOCKSDialTimeout)
	if err != nil {
		m.logger.Errorf("Low power mode: failed to connect to SOCKS %s: %s", m.node.SocksAddr(), err)
		_ = conn.Close()
		return
	}

	// Close connections on context cancellation so ProxyTCP unblocks.
	// Without this, Stop() would hang on wg.Wait() while io.Copy blocks.
	proxyDone := make(chan struct{})
	go func() {
		select {
		case <-m.ctx.Done():
			_ = conn.Close()
			_ = socksConn.Close()
		case <-proxyDone:
		}
	}()

	types.ProxyTCP(conn, socksConn)
	close(proxyDone)
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
