package udpforward

import (
	"net"
	"sync"
	"time"

	"github.com/yggdrasil-network/yggstack/src/types"
)

const (
	// DefaultUDPForwardQueueLen is the capacity of the datagram queue between
	// the local socket read and the netstack write. The queue is bounded
	// because the transport is datagram-based: when it fills up, datagrams are
	// dropped rather than buffered, which would otherwise turn a congested
	// path into unbounded memory growth.
	DefaultUDPForwardQueueLen = 1024
	// DefaultUDPForwardWriteTimeout is the deadline for a single netstack
	// write. A write without a deadline can never return: gonet.UDPConn.Write
	// waits for a writable event on the send buffer, and that event does not
	// arrive until the underlying path drains.
	DefaultUDPForwardWriteTimeout = 2 * time.Second
)

// UDPWriteConn is the netstack connection that local datagrams are written
// into. It is implemented by gonet.UDPConn.
type UDPWriteConn interface {
	net.Conn
	SetWriteDeadline(t time.Time) error
}

// UDPForwardLogger is the minimal set of logger methods the mapping needs.
// It is implemented by *log.Logger from github.com/gologme/log.
type UDPForwardLogger interface {
	Debugf(format string, v ...interface{})
	Errorf(format string, v ...interface{})
}

type udpForwardItem struct {
	conn UDPWriteConn
	key  string
	data []byte
}

// ForwardLocalUDP handles a local UDP mapping: it reads datagrams from the
// local socket and sends them into the Yggdrasil netstack toward the mapped
// address, which the caller provides via the dial function.
//
// The socket read and the netstack write are deliberately placed in separate
// goroutines. Writing into the netstack blocks and may never return until the
// underlying path drains; if it were done in the reading goroutine, a wedged
// path (congested channel, stuck peer, full send buffer) would stop the local
// socket from being drained. The socket would fill up, the kernel would drop
// datagrams, and the mapping would stay dead until a process restart -
// including the reverse direction, which is served by a separate ReverseProxyUDP
// goroutine. Separating the two turns a wedged path into dropped datagrams
// rather than a wedged mapping.
func ForwardLocalUDP(
	listen *net.UDPConn,
	mtu int,
	dial func() (UDPWriteConn, error),
	logger UDPForwardLogger,
	writeTimeout time.Duration,
	queueLen int,
) {
	sessions := new(sync.Map)
	queue := make(chan udpForwardItem, queueLen)

	go func() {
		for item := range queue {
			if err := item.conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
				logger.Debugf("local udp forward: cannot set write deadline: %q", err)
			}
			if _, err := item.conn.Write(item.data); err != nil {
				// The session is wedged or closed: free it and drop it from
				// accounting so the next datagram opens a fresh one.
				logger.Debugf("local udp forward: cannot write to yggdrasil: %q", err)
				_ = item.conn.Close()
				sessions.Delete(item.key)
			}
		}
	}()

	buf := make([]byte, mtu)
	for {
		n, remoteAddr, err := listen.ReadFrom(buf)
		if err != nil {
			logger.Debugf("local udp forward: readFrom error: %v", err)
			continue
		}
		if n == 0 {
			continue
		}

		key := remoteAddr.String()

		var conn UDPWriteConn
		if value, ok := sessions.Load(key); ok {
			conn = value.(UDPWriteConn)
		} else {
			logger.Debugf("Creating new session for %s", key)
			newConn, err := dial()
			if err != nil {
				logger.Errorf("Failed to connect to yggdrasil: %s", err)
				continue
			}
			conn = newConn
			sessions.Store(key, conn)
			go types.ReverseProxyUDP(uint64(mtu), listen, remoteAddr, conn)
		}

		data := make([]byte, n)
		copy(data, buf[:n])
		select {
		case queue <- udpForwardItem{conn: conn, key: key, data: data}:
		default:
			logger.Debugf("local udp forward: queue full, dropping datagram from %s", key)
		}
	}
}
