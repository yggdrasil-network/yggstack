package yggstack

import (
	"net"
	"sync"
	"sync/atomic"
)

// // // // // // // // // //

type udpSessionObj struct {
	conn         net.Conn
	remoteAddr   net.Addr
	lastActivity atomic.Int64
	connId       string                    // "" when callback is nil
	callback     ActivityCallbackInterface // nil when tracking is disabled
	counter      *connectionCounterObj     // nil when tracking is disabled
	closeOnce    sync.Once
}

// udpSessionMapObj is a typed concurrent map for UDP sessions.
type udpSessionMapObj struct {
	mu   sync.RWMutex
	data map[string]*udpSessionObj
}
