package mapping

import (
	"context"
	"net"
	"time"

	"github.com/yggdrasil-network/yggdrasil-go/src/core"

	"github.com/yggdrasil-network/yggstack/mod/activity"
	"github.com/yggdrasil-network/yggstack/src/types"
)

// // // // // // // // // //

// StartLocalUDP forwards a local UDP port to a remote Yggdrasil address.
func StartLocalUDP(node NodeInterface, mappings []types.UDPMapping, sessionTimeout time.Duration) {
	log := node.GetLogger()
	ns := node.GetNetstack()
	cb := node.GetActivityCallback()
	counter := node.GetConnCounter()
	wg := node.GetComponentsWg()
	ctx := node.GetComponentsCtx()
	mtu := node.GetCoreMTU()

	for _, mapping := range mappings {
		wg.Add(1)
		go func(m types.UDPMapping) {
			defer wg.Done()
			udpListenConn, err := net.ListenUDP("udp", m.Listen)
			if err != nil {
				log.Errorf("Failed to listen on local UDP %s: %s", m.Listen, err)
				return
			}
			node.AddCloser(udpListenConn)
			log.Infof("Mapping local UDP port %d to Yggdrasil %s", m.Listen.Port, m.Mapped)
			connections := NewUDPSessionMap()

			wg.Add(1)
			go func() {
				defer wg.Done()
				CleanupUDPSessions(ctx, connections, sessionTimeout, log)
			}()

			buf := make([]byte, mtu)
			for {
				n, remoteAddr, err := udpListenConn.ReadFrom(buf)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					if n == 0 {
						continue
					}
				}

				key := remoteAddr.String()
				session, ok := connections.Load(key)

				if !ok {
					log.Debugf("Creating new session for %s", key)
					fwdConn, err := ns.DialUDP(m.Mapped)
					if err != nil {
						log.Errorf("Failed to connect to %s: %s", m.Mapped, err)
						continue
					}
					session = &UDPSessionObj{
						Conn:       fwdConn,
						RemoteAddr: remoteAddr,
					}
					if cb != nil {
						session.ConnId = activity.GenerateConnId("udp", key)
						session.Callback = cb
						session.Counter = counter
						cb.OnConnectionCreated(session.ConnId, "udp")
						counter.Increment()
					}
					session.LastActivity.Store(time.Now().Unix())
					connections.Store(key, session)
					go types.ReverseProxyUDP(mtu, udpListenConn, remoteAddr, fwdConn)
				}

				session.LastActivity.Store(time.Now().Unix())
				_, err = session.Conn.Write(buf[:n])
				if err != nil {
					log.Debugf("Cannot write from yggdrasil to udp listener: %q", err)
					CloseUDPSession(session, log)
					connections.Delete(key)
					continue
				}
			}
		}(mapping)
	}
}

// StartRemoteUDP exposes a local UDP service to the Yggdrasil network.
func StartRemoteUDP(node NodeInterface, mappings []types.UDPMapping, sessionTimeout time.Duration) {
	log := node.GetLogger()
	ns := node.GetNetstack()
	cb := node.GetActivityCallback()
	counter := node.GetConnCounter()
	wg := node.GetComponentsWg()
	ctx := node.GetComponentsCtx()
	mtu := node.GetCoreMTU()

	for _, mapping := range mappings {
		wg.Add(1)
		go func(m types.UDPMapping) {
			defer wg.Done()
			udpListenConn, err := ns.ListenUDP(m.Listen)
			if err != nil {
				log.Errorf("Failed to listen on Yggdrasil UDP %s: %s", m.Listen, err)
				return
			}
			node.AddCloser(udpListenConn)
			log.Infof("Mapping Yggdrasil UDP port %d to %s", m.Listen.Port, m.Mapped)
			connections := NewUDPSessionMap()

			wg.Add(1)
			go func() {
				defer wg.Done()
				CleanupUDPSessions(ctx, connections, sessionTimeout, log)
			}()

			buf := make([]byte, mtu)
			for {
				n, remoteAddr, err := udpListenConn.ReadFrom(buf)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					log.Debugf("udp readFrom error: %v", err)
				}
				if n == 0 {
					continue
				}

				key := remoteAddr.String()
				session, ok := connections.Load(key)

				if !ok {
					log.Debugf("Creating new session for %s", key)
					fwdConn, err := net.DialUDP("udp", nil, m.Mapped)
					if err != nil {
						log.Errorf("Failed to connect to %s: %s", m.Mapped, err)
						continue
					}
					session = &UDPSessionObj{
						Conn:       fwdConn,
						RemoteAddr: remoteAddr,
					}
					if cb != nil {
						session.ConnId = activity.GenerateConnId("udp-remote", key)
						session.Callback = cb
						session.Counter = counter
						cb.OnConnectionCreated(session.ConnId, "udp")
						counter.Increment()
					}
					session.LastActivity.Store(time.Now().Unix())
					connections.Store(key, session)
					go types.ReverseProxyUDP(mtu, udpListenConn, remoteAddr, fwdConn)
				}

				session.LastActivity.Store(time.Now().Unix())
				_, err = session.Conn.Write(buf[:n])
				if err != nil {
					log.Debugf("Cannot write from yggdrasil to udp listener: %q", err)
					CloseUDPSession(session, log)
					connections.Delete(key)
					continue
				}
			}
		}(mapping)
	}
}

// //

// CloseUDPSession closes a UDP session and notifies the callback.
func CloseUDPSession(session *UDPSessionObj, logger core.Logger) {
	session.CloseOnce.Do(func() {
		_ = session.Conn.Close()
		if session.Callback != nil && session.ConnId != "" {
			session.Callback.OnConnectionClosed(session.ConnId)
			session.Counter.Decrement()
		}
	})
}

// CleanupUDPSessions periodically removes inactive UDP sessions.
func CleanupUDPSessions(ctx context.Context, connections *UDPSessionMapObj, timeout time.Duration, logger core.Logger) {
	ticker := time.NewTicker(timeout / 4)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now().Unix()
			connections.Range(func(key string, session *UDPSessionObj) bool {
				if now-session.LastActivity.Load() > int64(timeout.Seconds()) {
					logger.Debugf("Cleaning up inactive UDP session %s", key)
					CloseUDPSession(session, logger)
					connections.Delete(key)
				}
				return true
			})
		}
	}
}
