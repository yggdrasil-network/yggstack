package types

import (
	"net"
)

func tcpProxyFunc(mtu uint64, dst, src net.Conn) error {
	buf := make([]byte, mtu)
	for {
		n, err := src.Read(buf[:])
		if err != nil {
			return err
		}
		if n > 0 {
			n, err = dst.Write(buf[:n])
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func ProxyTCP(mtu uint64, c1, c2 net.Conn) error {
	// Start proxying
	errCh := make(chan error, 2)
	c2.Write([]byte(c1.RemoteAddr().String()))
	go func() { errCh <- tcpProxyFunc(mtu, c1, c2) }()
	go func() { errCh <- tcpProxyFunc(mtu, c2, c1) }()

	// Wait
	for i := 0; i < 2; i++ {
		e := <-errCh
		if e != nil {
			// Close connections and return
			c1.Close()
			c2.Close()
			return e
		}
	}

	return nil
}
