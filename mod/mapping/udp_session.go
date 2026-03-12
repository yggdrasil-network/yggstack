package mapping

import (
	"net"
	"sync"
	"sync/atomic"

	"github.com/yggdrasil-network/yggstack/mod/activity"
)

// // // // // // // // // //

// UDPSessionObj holds the state of a single UDP session.
type UDPSessionObj struct {
	Conn         net.Conn
	RemoteAddr   net.Addr
	LastActivity atomic.Int64
	ConnId       string                     // "" when callback is nil
	Callback     activity.CallbackInterface // nil when tracking is disabled
	Counter      *activity.CounterObj       // nil when tracking is disabled
	CloseOnce    sync.Once
}

// UDPSessionMapObj is a typed concurrent map of UDP sessions.
type UDPSessionMapObj struct {
	mu   sync.RWMutex
	data map[string]*UDPSessionObj
}

// NewUDPSessionMap creates an empty session map.
func NewUDPSessionMap() *UDPSessionMapObj {
	return &UDPSessionMapObj{data: make(map[string]*UDPSessionObj)}
}
