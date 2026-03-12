package yggstack

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/things-go/go-socks5"

	"github.com/yggdrasil-network/yggstack/src/types"
)

// // // // // // // // // //

func (o *Obj) startSocks(cfg ConfigObj) error {
	dialFn := o.Netstack.DialContext
	if o.activityCallback != nil {
		originalDial := dialFn
		dialFn = func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := originalDial(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			connId := generateConnId("socks", addr)
			o.activityCallback.OnConnectionCreated(connId, "socks")
			o.connCounter.increment()
			return &trackedConnObj{
				Conn: conn, connId: connId,
				callback: o.activityCallback, counter: &o.connCounter,
			}, nil
		}
	}
	socksOptions := []socks5.Option{
		socks5.WithDial(dialFn),
	}
	if cfg.Nameserver == "" {
		o.logger.Infof("DNS nameserver is not set!")
		o.logger.Infof("SOCKS server will not be able to resolve hostnames other than .pk.ygg !")
	}
	resolver := types.NewNameResolver(o.Netstack, cfg.Nameserver)
	socksOptions = append(socksOptions, socks5.WithResolver(resolver))
	if cfg.SocksVerbose {
		socksOptions = append(socksOptions, socks5.WithLogger(o.logger))
	}
	server := socks5.NewServer(socksOptions...)

	if strings.Contains(cfg.SocksAddr, ":") {
		o.socksIsUnix = false
		o.logger.Infof("Starting SOCKS server on %s", cfg.SocksAddr)
		var err error
		o.socksListener, err = net.Listen("tcp", cfg.SocksAddr)
		if err != nil {
			return fmt.Errorf("net.Listen tcp %s: %w", cfg.SocksAddr, err)
		}
	} else {
		o.socksIsUnix = true
		o.logger.Infof("Starting SOCKS server with socket file %s", cfg.SocksAddr)
		var err error
		o.socksListener, err = net.Listen("unix", cfg.SocksAddr)
		if err != nil {
			if isErrorAddressAlreadyInUse(err) {
				_, dialErr := net.Dial("unix", cfg.SocksAddr)
				if dialErr != nil {
					// Unlink dead socket if not connected
					if rmErr := os.RemoveAll(cfg.SocksAddr); rmErr != nil {
						return fmt.Errorf("os.RemoveAll %s: %w", cfg.SocksAddr, rmErr)
					}
					o.socksListener, err = net.Listen("unix", cfg.SocksAddr)
					if err != nil {
						return fmt.Errorf("net.Listen unix %s: %w", cfg.SocksAddr, err)
					}
				} else {
					return fmt.Errorf("another instance is listening on socket '%s'", cfg.SocksAddr)
				}
			} else {
				return fmt.Errorf("net.Listen unix %s: %w", cfg.SocksAddr, err)
			}
		}
	}

	// Signal handleWakeConnection: SOCKS is ready
	if o.socksReadyCh != nil {
		close(o.socksReadyCh)
	}

	o.componentsWg.Add(1)
	go func() {
		defer o.componentsWg.Done()
		if err := server.Serve(o.socksListener); err != nil {
			if o.componentsCtx.Err() == nil {
				o.logger.Errorf("SOCKS5 server error: %s", err)
			}
		}
	}()

	return nil
}

func (o *Obj) startLocalTCP(mappings []types.TCPMapping) {
	for _, mapping := range mappings {
		o.componentsWg.Add(1)
		go func(m types.TCPMapping) {
			defer o.componentsWg.Done()
			listener, err := net.ListenTCP("tcp", m.Listen)
			if err != nil {
				o.logger.Errorf("Failed to listen on local TCP %s: %s", m.Listen, err)
				return
			}
			o.addCloser(listener)
			o.logger.Infof("Mapping local TCP port %d to Yggdrasil %s", m.Listen.Port, m.Mapped)
			for {
				c, err := listener.Accept()
				if err != nil {
					if o.componentsCtx.Err() != nil {
						return
					}
					o.logger.Errorf("Local TCP accept error: %s", err)
					return
				}
				r, err := o.Netstack.DialTCP(m.Mapped)
				if err != nil {
					o.logger.Errorf("Failed to connect to %s: %s", m.Mapped, err)
					_ = c.Close()
					continue
				}
				var remote net.Conn = r
				if o.activityCallback != nil {
					connId := generateConnId("tcp", m.Mapped.String())
					o.activityCallback.OnConnectionCreated(connId, "tcp")
					o.connCounter.increment()
					remote = &trackedConnObj{
						Conn: r, connId: connId,
						callback: o.activityCallback, counter: &o.connCounter,
					}
				}
				go types.ProxyTCP(c, remote)
			}
		}(mapping)
	}
}

func (o *Obj) startLocalUDP(mappings []types.UDPMapping, sessionTimeout time.Duration) {
	for _, mapping := range mappings {
		o.componentsWg.Add(1)
		go func(m types.UDPMapping) {
			defer o.componentsWg.Done()
			mtu := o.Core.MTU()
			udpListenConn, err := net.ListenUDP("udp", m.Listen)
			if err != nil {
				o.logger.Errorf("Failed to listen on local UDP %s: %s", m.Listen, err)
				return
			}
			o.addCloser(udpListenConn)
			o.logger.Infof("Mapping local UDP port %d to Yggdrasil %s", m.Listen.Port, m.Mapped)
			connections := &udpSessionMapObj{data: make(map[string]*udpSessionObj)}

			o.componentsWg.Add(1)
			go func() {
				defer o.componentsWg.Done()
				o.cleanupUDPSessions(o.componentsCtx, connections, sessionTimeout)
			}()

			buf := make([]byte, mtu)
			for {
				n, remoteAddr, err := udpListenConn.ReadFrom(buf)
				if err != nil {
					if o.componentsCtx.Err() != nil {
						return
					}
					if n == 0 {
						continue
					}
				}

				key := remoteAddr.String()

				session, ok := connections.Load(key)

				if !ok {
					o.logger.Debugf("Creating new session for %s", key)
					fwdConn, err := o.Netstack.DialUDP(m.Mapped)
					if err != nil {
						o.logger.Errorf("Failed to connect to %s: %s", m.Mapped, err)
						continue
					}
					session = &udpSessionObj{
						conn:       fwdConn,
						remoteAddr: remoteAddr,
					}
					if o.activityCallback != nil {
						session.connId = generateConnId("udp", key)
						session.callback = o.activityCallback
						session.counter = &o.connCounter
						o.activityCallback.OnConnectionCreated(session.connId, "udp")
						o.connCounter.increment()
					}
					session.lastActivity.Store(time.Now().Unix())
					connections.Store(key, session)
					go types.ReverseProxyUDP(mtu, udpListenConn, remoteAddr, fwdConn)
				}

				session.lastActivity.Store(time.Now().Unix())
				_, err = session.conn.Write(buf[:n])
				if err != nil {
					o.logger.Debugf("Cannot write from yggdrasil to udp listener: %q", err)
					o.closeUDPSession(session)
					connections.Delete(key)
					continue
				}
			}
		}(mapping)
	}
}

func (o *Obj) startRemoteTCP(mappings []types.TCPMapping) {
	for _, mapping := range mappings {
		o.componentsWg.Add(1)
		go func(m types.TCPMapping) {
			defer o.componentsWg.Done()
			listener, err := o.Netstack.ListenTCP(m.Listen)
			if err != nil {
				o.logger.Errorf("Failed to listen on Yggdrasil TCP %s: %s", m.Listen, err)
				return
			}
			o.addCloser(listener)
			o.logger.Infof("Mapping Yggdrasil TCP port %d to %s", m.Listen.Port, m.Mapped)
			for {
				c, err := listener.Accept()
				if err != nil {
					if o.componentsCtx.Err() != nil {
						return
					}
					o.logger.Errorf("Remote TCP accept error: %s", err)
					return
				}
				r, err := net.DialTCP("tcp", nil, m.Mapped)
				if err != nil {
					o.logger.Errorf("Failed to connect to %s: %s", m.Mapped, err)
					_ = c.Close()
					continue
				}
				var incoming net.Conn = c
				if o.activityCallback != nil {
					connId := generateConnId("tcp-remote", c.RemoteAddr().String())
					o.activityCallback.OnConnectionCreated(connId, "tcp")
					o.connCounter.increment()
					incoming = &trackedConnObj{
						Conn: c, connId: connId,
						callback: o.activityCallback, counter: &o.connCounter,
					}
				}
				go types.ProxyTCP(incoming, r)
			}
		}(mapping)
	}
}

func (o *Obj) startRemoteUDP(mappings []types.UDPMapping, sessionTimeout time.Duration) {
	for _, mapping := range mappings {
		o.componentsWg.Add(1)
		go func(m types.UDPMapping) {
			defer o.componentsWg.Done()
			mtu := o.Core.MTU()
			udpListenConn, err := o.Netstack.ListenUDP(m.Listen)
			if err != nil {
				o.logger.Errorf("Failed to listen on Yggdrasil UDP %s: %s", m.Listen, err)
				return
			}
			o.addCloser(udpListenConn)
			o.logger.Infof("Mapping Yggdrasil UDP port %d to %s", m.Listen.Port, m.Mapped)
			connections := &udpSessionMapObj{data: make(map[string]*udpSessionObj)}

			o.componentsWg.Add(1)
			go func() {
				defer o.componentsWg.Done()
				o.cleanupUDPSessions(o.componentsCtx, connections, sessionTimeout)
			}()

			buf := make([]byte, mtu)
			for {
				n, remoteAddr, err := udpListenConn.ReadFrom(buf)
				if err != nil {
					if o.componentsCtx.Err() != nil {
						return
					}
					o.logger.Debugf("udp readFrom error: %v", err)
				}
				if n == 0 {
					continue
				}

				key := remoteAddr.String()

				session, ok := connections.Load(key)

				if !ok {
					o.logger.Debugf("Creating new session for %s", key)
					fwdConn, err := net.DialUDP("udp", nil, m.Mapped)
					if err != nil {
						o.logger.Errorf("Failed to connect to %s: %s", m.Mapped, err)
						continue
					}
					session = &udpSessionObj{
						conn:       fwdConn,
						remoteAddr: remoteAddr,
					}
					if o.activityCallback != nil {
						session.connId = generateConnId("udp-remote", key)
						session.callback = o.activityCallback
						session.counter = &o.connCounter
						o.activityCallback.OnConnectionCreated(session.connId, "udp")
						o.connCounter.increment()
					}
					session.lastActivity.Store(time.Now().Unix())
					connections.Store(key, session)
					go types.ReverseProxyUDP(mtu, udpListenConn, remoteAddr, fwdConn)
				}

				session.lastActivity.Store(time.Now().Unix())
				_, err = session.conn.Write(buf[:n])
				if err != nil {
					o.logger.Debugf("Cannot write from yggdrasil to udp listener: %q", err)
					o.closeUDPSession(session)
					connections.Delete(key)
					continue
				}
			}
		}(mapping)
	}
}

// //

func (o *Obj) closeUDPSession(session *udpSessionObj) {
	session.closeOnce.Do(func() {
		_ = session.conn.Close()
		if session.callback != nil && session.connId != "" {
			session.callback.OnConnectionClosed(session.connId)
			session.counter.decrement()
		}
	})
}

func (o *Obj) cleanupUDPSessions(ctx context.Context, connections *udpSessionMapObj, timeout time.Duration) {
	ticker := time.NewTicker(timeout / 4)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now().Unix()
			connections.Range(func(key string, session *udpSessionObj) bool {
				if now-session.lastActivity.Load() > int64(timeout.Seconds()) {
					o.logger.Debugf("Cleaning up inactive UDP session %s", key)
					o.closeUDPSession(session)
					connections.Delete(key)
				}
				return true
			})
		}
	}
}
