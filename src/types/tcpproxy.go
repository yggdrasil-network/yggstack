package types

import (
	"io"
	"net"
	"time"
)

// // // // // // // // // //

// proxyTCPCloseTimeout is the maximum time to wait for the second io.Copy
// goroutine after connections have been closed.
// Connections are already closed so the goroutine should unblock immediately;
// the timeout is a safety net for implementations that ignore Close.
const proxyTCPCloseTimeout = 5 * time.Second

// ProxyTCP proxies data bidirectionally between c1 and c2.
// Returns when both directions have finished.
func ProxyTCP(c1, c2 net.Conn) {
	errCh := make(chan error, 2)
	go func() { _, err := io.Copy(c1, c2); errCh <- err }()
	go func() { _, err := io.Copy(c2, c1); errCh <- err }()

	// Wait for one direction to fail or finish, then close both ends.
	<-errCh
	_ = c1.Close()
	_ = c2.Close()

	// Wait for the second goroutine, bounded by timeout.
	select {
	case <-errCh:
	case <-time.After(proxyTCPCloseTimeout):
	}
}
