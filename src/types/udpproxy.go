package types

import (
	"net"
)

func ReverseProxyUDPConn(mtu uint64, dst net.PacketConn, dstAddr net.Addr, src net.UDPConn) error {
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

func ReverseProxyUDPPacketConn(mtu uint64, dst net.PacketConn, dstAddr net.Addr, src net.PacketConn) error {
	buf := make([]byte, mtu)
	for {
		n, _, err := src.ReadFrom(buf[:])
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
