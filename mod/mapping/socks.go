package mapping

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/things-go/go-socks5"

	"github.com/yggdrasil-network/yggstack/mod/activity"
	"github.com/yggdrasil-network/yggstack/src/types"
)

// // // // // // // // // //

// SocksConfigObj holds SOCKS5 server parameters.
type SocksConfigObj struct {
	Addr       string
	Nameserver string
	Verbose    bool
}

// SocksResultObj holds the result of starting the SOCKS5 server.
type SocksResultObj struct {
	Listener net.Listener
	IsUnix   bool
}

// //

// StartSocks starts a SOCKS5 server and returns the listener.
// readyCh is closed when the listener is ready to accept connections.
func StartSocks(node NodeInterface, cfg SocksConfigObj, readyCh chan struct{}) (*SocksResultObj, error) {
	ns := node.GetNetstack()
	log := node.GetLogger()
	cb := node.GetActivityCallback()
	counter := node.GetConnCounter()

	dialFn := ns.DialContext
	if cb != nil {
		originalDial := dialFn
		dialFn = func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := originalDial(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			connId := activity.GenerateConnId("socks", addr)
			cb.OnConnectionCreated(connId, "socks")
			counter.Increment()
			return &activity.TrackedConnObj{
				Conn: conn, ConnId: connId,
				Callback: cb, Counter: counter,
			}, nil
		}
	}

	socksOptions := []socks5.Option{
		socks5.WithDial(dialFn),
	}
	if cfg.Nameserver == "" {
		log.Infof("DNS nameserver is not set!")
		log.Infof("SOCKS server will not be able to resolve hostnames other than .pk.ygg !")
	}
	resolver := types.NewNameResolver(ns, cfg.Nameserver)
	socksOptions = append(socksOptions, socks5.WithResolver(resolver))
	if cfg.Verbose {
		socksOptions = append(socksOptions, socks5.WithLogger(log))
	}
	server := socks5.NewServer(socksOptions...)

	result := &SocksResultObj{}

	if strings.Contains(cfg.Addr, ":") {
		result.IsUnix = false
		log.Infof("Starting SOCKS server on %s", cfg.Addr)
		var err error
		result.Listener, err = net.Listen("tcp", cfg.Addr)
		if err != nil {
			return nil, fmt.Errorf("net.Listen tcp %s: %w", cfg.Addr, err)
		}
	} else {
		result.IsUnix = true
		log.Infof("Starting SOCKS server with socket file %s", cfg.Addr)
		var err error
		result.Listener, err = net.Listen("unix", cfg.Addr)
		if err != nil {
			if isErrorAddressAlreadyInUse(err) {
				_, dialErr := net.Dial("unix", cfg.Addr)
				if dialErr != nil {
					if rmErr := os.RemoveAll(cfg.Addr); rmErr != nil {
						return nil, fmt.Errorf("os.RemoveAll %s: %w", cfg.Addr, rmErr)
					}
					result.Listener, err = net.Listen("unix", cfg.Addr)
					if err != nil {
						return nil, fmt.Errorf("net.Listen unix %s: %w", cfg.Addr, err)
					}
				} else {
					return nil, fmt.Errorf("another instance is listening on socket '%s'", cfg.Addr)
				}
			} else {
				return nil, fmt.Errorf("net.Listen unix %s: %w", cfg.Addr, err)
			}
		}
	}

	// Signal handleWakeConnection: SOCKS is ready
	if readyCh != nil {
		close(readyCh)
	}

	wg := node.GetComponentsWg()
	ctx := node.GetComponentsCtx()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := server.Serve(result.Listener); err != nil {
			if ctx.Err() == nil {
				log.Errorf("SOCKS5 server error: %s", err)
			}
		}
	}()

	return result, nil
}
