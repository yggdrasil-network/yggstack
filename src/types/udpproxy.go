package types

import (
	"context"
	"net"
	"time"
)

// // // // // // // // // //

// ReverseProxyUDP reads packets from src and writes them to dst/dstAddr.
// Exits on first I/O error, on ctx cancellation, or when src is closed.
// ctx cancellation sets a past read deadline on src to unblock the blocked Read.
func ReverseProxyUDP(ctx context.Context, mtu uint64, dst net.PacketConn, dstAddr net.Addr, src net.Conn) error {
	// Watcher: unblocks Read when context is cancelled.
	// Closed via defer to avoid leaking it when the loop exits normally.
	watchDone := make(chan struct{})
	defer close(watchDone)
	go func() {
		select {
		case <-ctx.Done():
			_ = src.SetReadDeadline(time.Now())
		case <-watchDone:
		}
	}()

	buf := make([]byte, mtu)
	for {
		n, err := src.Read(buf[:])
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		if n > 0 {
			_, err = dst.WriteTo(buf[:n], dstAddr)
			if err != nil {
				return err
			}
		}
	}
}
