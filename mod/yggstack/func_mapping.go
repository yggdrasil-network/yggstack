package yggstack

import (
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/things-go/go-socks5"
	"github.com/yggdrasil-network/yggstack/src/types"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
)

// // // // // // // // // //

func (o *Obj) startSocks(cfg ConfigObj) error {
	socksOptions := []socks5.Option{
		socks5.WithDial(o.Netstack.DialContext),
	}
	if cfg.Nameserver == "" {
		o.logger.Infof("DNS nameserver is not set!")
		o.logger.Infof("SOCKS server will not be able to resolve hostnames other than .pk.ygg !")
	}
	resolver := types.NewNameResolver(o.Netstack, cfg.Nameserver)
	socksOptions = append(socksOptions, socks5.WithResolver(resolver))
	if o.logger.GetLevel("debug") {
		socksOptions = append(socksOptions, socks5.WithLogger(o.logger))
	}
	server := socks5.NewServer(socksOptions...)

	if strings.Contains(cfg.SocksAddr, ":") {
		o.logger.Infof("Starting SOCKS server on %s", cfg.SocksAddr)
		var err error
		o.socksListener, err = net.Listen("tcp", cfg.SocksAddr)
		if err != nil {
			return fmt.Errorf("net.Listen tcp %s: %w", cfg.SocksAddr, err)
		}
	} else {
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

	go func() {
		if err := server.Serve(o.socksListener); err != nil {
			o.logger.Errorf("SOCKS5 server error: %s", err)
		}
	}()

	return nil
}

//

func (o *Obj) startLocalTCP(mappings []types.TCPMapping) {
	for _, mapping := range mappings {
		go func(m types.TCPMapping) {
			listener, err := net.ListenTCP("tcp", m.Listen)
			if err != nil {
				o.logger.Errorf("Failed to listen on local TCP %s: %s", m.Listen, err)
				return
			}
			o.logger.Infof("Mapping local TCP port %d to Yggdrasil %s", m.Listen.Port, m.Mapped)
			for {
				c, err := listener.Accept()
				if err != nil {
					o.logger.Errorf("Local TCP accept error: %s", err)
					return
				}
				r, err := o.Netstack.DialTCP(m.Mapped)
				if err != nil {
					o.logger.Errorf("Failed to connect to %s: %s", m.Mapped, err)
					_ = c.Close()
					continue
				}
				go types.ProxyTCP(o.Core.MTU(), c, r)
			}
		}(mapping)
	}
}

func (o *Obj) startLocalUDP(mappings []types.UDPMapping) {
	for _, mapping := range mappings {
		go func(m types.UDPMapping) {
			mtu := o.Core.MTU()
			udpListenConn, err := net.ListenUDP("udp", m.Listen)
			if err != nil {
				o.logger.Errorf("Failed to listen on local UDP %s: %s", m.Listen, err)
				return
			}
			o.logger.Infof("Mapping local UDP port %d to Yggdrasil %s", m.Listen.Port, m.Mapped)
			connections := new(sync.Map)
			buf := make([]byte, mtu)
			for {
				n, remoteAddr, err := udpListenConn.ReadFrom(buf)
				if err != nil {
					if n == 0 {
						continue
					}
				}

				key := remoteAddr.String()
				connVal, ok := connections.Load(key)

				if !ok {
					o.logger.Debugf("Creating new session for %s", key)
					fwdConn, err := o.Netstack.DialUDP(m.Mapped)
					if err != nil {
						o.logger.Errorf("Failed to connect to %s: %s", m.Mapped, err)
						continue
					}
					session := &udpSessionObj{
						conn:       fwdConn,
						remoteAddr: remoteAddr,
					}
					connections.Store(key, session)
					go types.ReverseProxyUDP(mtu, udpListenConn, remoteAddr, fwdConn)
				}

				session, ok := connVal.(*udpSessionObj)
				if !ok {
					continue
				}

				fwdConnPtr := session.conn.(*gonet.UDPConn)
				fwdConn := *fwdConnPtr

				_, err = fwdConn.Write(buf[:n])
				if err != nil {
					o.logger.Debugf("Cannot write from yggdrasil to udp listener: %q", err)
					fwdConn.Close()
					connections.Delete(key)
					continue
				}
			}
		}(mapping)
	}
}

func (o *Obj) startRemoteTCP(mappings []types.TCPMapping) {
	for _, mapping := range mappings {
		go func(m types.TCPMapping) {
			listener, err := o.Netstack.ListenTCP(m.Listen)
			if err != nil {
				o.logger.Errorf("Failed to listen on Yggdrasil TCP %s: %s", m.Listen, err)
				return
			}
			o.logger.Infof("Mapping Yggdrasil TCP port %d to %s", m.Listen.Port, m.Mapped)
			for {
				c, err := listener.Accept()
				if err != nil {
					o.logger.Errorf("Remote TCP accept error: %s", err)
					return
				}
				r, err := net.DialTCP("tcp", nil, m.Mapped)
				if err != nil {
					o.logger.Errorf("Failed to connect to %s: %s", m.Mapped, err)
					_ = c.Close()
					continue
				}
				go types.ProxyTCP(o.Core.MTU(), c, r)
			}
		}(mapping)
	}
}

func (o *Obj) startRemoteUDP(mappings []types.UDPMapping) {
	for _, mapping := range mappings {
		go func(m types.UDPMapping) {
			mtu := o.Core.MTU()
			udpListenConn, err := o.Netstack.ListenUDP(m.Listen)
			if err != nil {
				o.logger.Errorf("Failed to listen on Yggdrasil UDP %s: %s", m.Listen, err)
				return
			}
			o.logger.Infof("Mapping Yggdrasil UDP port %d to %s", m.Listen.Port, m.Mapped)
			connections := new(sync.Map)
			buf := make([]byte, mtu)
			for {
				n, remoteAddr, err := udpListenConn.ReadFrom(buf)
				if err != nil {
					o.logger.Debugf("udp readFrom error: %v", err)
				}
				if n == 0 {
					continue
				}

				key := remoteAddr.String()
				var session *udpSessionObj

				connVal, ok := connections.Load(key)

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
					connections.Store(key, session)
					go types.ReverseProxyUDP(mtu, udpListenConn, remoteAddr, fwdConn)
				} else {
					session, ok = connVal.(*udpSessionObj)
					if !ok {
						continue
					}
				}

				fwdConnPtr := session.conn.(*net.UDPConn)
				fwdConn := *fwdConnPtr

				_, err = fwdConn.Write(buf[:n])
				if err != nil {
					o.logger.Debugf("Cannot write from yggdrasil to udp listener: %q", err)
					fwdConn.Close()
					connections.Delete(key)
					continue
				}
			}
		}(mapping)
	}
}
