package types

import (
	"io"
	"net"
)

// // // // // // // // // //

func ProxyTCP(c1, c2 net.Conn) {
	errCh := make(chan error, 2)
	go func() { _, err := io.Copy(c1, c2); errCh <- err }()
	go func() { _, err := io.Copy(c2, c1); errCh <- err }()

	// Wait for one direction to finish, close both
	<-errCh
	_ = c1.Close()
	_ = c2.Close()
	<-errCh
}
