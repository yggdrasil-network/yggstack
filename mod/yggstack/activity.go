package yggstack

import (
	"net"
	"strconv"
	"sync"
	"sync/atomic"
)

// // // // // // // // // //

// ActivityCallbackInterface receives connection lifecycle notifications.
// Implementation must be non-blocking.
type ActivityCallbackInterface interface {
	OnConnectionCreated(connId string, protocol string)
	OnConnectionClosed(connId string)
}

// //

// connectionCounterObj is an atomic counter of active tracked connections.
type connectionCounterObj struct {
	active atomic.Int64
}

func (c *connectionCounterObj) increment()   { c.active.Add(1) }
func (c *connectionCounterObj) decrement()   { c.active.Add(-1) }
func (c *connectionCounterObj) count() int64 { return c.active.Load() }

// //

// trackedConnObj wraps net.Conn, notifying callback on data transfer and close.
type trackedConnObj struct {
	net.Conn
	connId   string
	callback ActivityCallbackInterface
	counter  *connectionCounterObj
	closed   sync.Once
}

func (t *trackedConnObj) Close() error {
	var innerErr error
	t.closed.Do(func() {
		t.callback.OnConnectionClosed(t.connId)
		t.counter.decrement()
		innerErr = t.Conn.Close()
	})
	return innerErr
}

// //

var connIdCounter atomic.Uint64

// Atomic counter guarantees uniqueness under burst load
func generateConnId(prefix string, addr string) string {
	id := connIdCounter.Add(1)
	buf := make([]byte, 0, len(prefix)+1+len(addr)+1+20)
	buf = append(buf, prefix...)
	buf = append(buf, '-')
	buf = append(buf, addr...)
	buf = append(buf, '-')
	buf = strconv.AppendUint(buf, id, 10)
	return string(buf)
}
