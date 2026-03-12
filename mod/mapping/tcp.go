package mapping

import (
	"errors"
	"net"
	"time"

	"github.com/yggdrasil-network/yggstack/mod/activity"
	"github.com/yggdrasil-network/yggstack/src/types"
)

// // // // // // // // // //

const (
	acceptRetryInitial = 5 * time.Millisecond
	acceptRetryMax     = 1 * time.Second
)

// isTransientAcceptError reports whether an Accept error is transient
// (e.g. ECONNABORTED, EMFILE) and the listener should retry rather than exit.
func isTransientAcceptError(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Temporary() //nolint:staticcheck
}

// //

// StartLocalTCP forwards a local TCP port to a remote Yggdrasil address.
func StartLocalTCP(node NodeInterface, mappings []types.TCPMapping) {
	log := node.GetLogger()
	ns := node.GetNetstack()
	cb := node.GetActivityCallback()
	counter := node.GetConnCounter()
	wg := node.GetComponentsWg()
	ctx := node.GetComponentsCtx()

	for _, mapping := range mappings {
		wg.Add(1)
		go func(m types.TCPMapping) {
			defer wg.Done()
			listener, err := net.ListenTCP("tcp", m.Listen)
			if err != nil {
				log.Errorf("Failed to listen on local TCP %s: %s", m.Listen, err)
				return
			}
			node.AddCloser(listener)
			log.Infof("Mapping local TCP port %d to Yggdrasil %s", m.Listen.Port, m.Mapped)

			var retryDelay time.Duration
			for {
				c, err := listener.Accept()
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					if isTransientAcceptError(err) {
						if retryDelay == 0 {
							retryDelay = acceptRetryInitial
						} else {
							retryDelay *= 2
							if retryDelay > acceptRetryMax {
								retryDelay = acceptRetryMax
							}
						}
						log.Warnf("Local TCP accept transient error (retry in %s): %s", retryDelay, err)
						select {
						case <-time.After(retryDelay):
						case <-ctx.Done():
							return
						}
						continue
					}
					log.Errorf("Local TCP accept error: %s", err)
					return
				}
				retryDelay = 0

				r, err := ns.DialTCP(m.Mapped)
				if err != nil {
					log.Errorf("Failed to connect to %s: %s", m.Mapped, err)
					_ = c.Close()
					continue
				}
				var remote net.Conn = r
				if cb != nil {
					connId := activity.GenerateConnId("tcp", m.Mapped.String())
					cb.OnConnectionCreated(connId, "tcp")
					counter.Increment()
					remote = &activity.TrackedConnObj{
						Conn: r, ConnId: connId,
						Callback: cb, Counter: counter,
					}
				}
				go types.ProxyTCP(c, remote)
			}
		}(mapping)
	}
}

// StartRemoteTCP exposes a local TCP service to the Yggdrasil network.
func StartRemoteTCP(node NodeInterface, mappings []types.TCPMapping) {
	log := node.GetLogger()
	ns := node.GetNetstack()
	cb := node.GetActivityCallback()
	counter := node.GetConnCounter()
	wg := node.GetComponentsWg()
	ctx := node.GetComponentsCtx()

	for _, mapping := range mappings {
		wg.Add(1)
		go func(m types.TCPMapping) {
			defer wg.Done()
			listener, err := ns.ListenTCP(m.Listen)
			if err != nil {
				log.Errorf("Failed to listen on Yggdrasil TCP %s: %s", m.Listen, err)
				return
			}
			node.AddCloser(listener)
			log.Infof("Mapping Yggdrasil TCP port %d to %s", m.Listen.Port, m.Mapped)

			var retryDelay time.Duration
			for {
				c, err := listener.Accept()
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					if isTransientAcceptError(err) {
						if retryDelay == 0 {
							retryDelay = acceptRetryInitial
						} else {
							retryDelay *= 2
							if retryDelay > acceptRetryMax {
								retryDelay = acceptRetryMax
							}
						}
						log.Warnf("Remote TCP accept transient error (retry in %s): %s", retryDelay, err)
						select {
						case <-time.After(retryDelay):
						case <-ctx.Done():
							return
						}
						continue
					}
					log.Errorf("Remote TCP accept error: %s", err)
					return
				}
				retryDelay = 0

				r, err := net.DialTCP("tcp", nil, m.Mapped)
				if err != nil {
					log.Errorf("Failed to connect to %s: %s", m.Mapped, err)
					_ = c.Close()
					continue
				}
				var incoming net.Conn = c
				if cb != nil {
					connId := activity.GenerateConnId("tcp-remote", c.RemoteAddr().String())
					cb.OnConnectionCreated(connId, "tcp")
					counter.Increment()
					incoming = &activity.TrackedConnObj{
						Conn: c, ConnId: connId,
						Callback: cb, Counter: counter,
					}
				}
				go types.ProxyTCP(incoming, r)
			}
		}(mapping)
	}
}
