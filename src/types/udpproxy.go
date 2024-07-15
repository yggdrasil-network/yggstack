package types

import (
	"net"
)

func ReverseProxyUDP(mtu uint64, dst net.PacketConn, dstAddr net.Addr, src net.UDPConn) error {
	buf := make([]byte, mtu)
	for {
		n, err := src.Read(buf[:])
		if err != nil {
			return err
		}
		if n > 0 {
			n, err = dst.WriteTo(buf[:n], dstAddr)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
