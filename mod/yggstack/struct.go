package yggstack

import (
	"context"
	"io"
	"net"
	"sync"
	"sync/atomic"

	"github.com/yggdrasil-network/yggdrasil-go/src/admin"
	"github.com/yggdrasil-network/yggdrasil-go/src/core"
	"github.com/yggdrasil-network/yggdrasil-go/src/multicast"

	"github.com/yggdrasil-network/yggstack/src/netstack"
)

// // // // // // // // // //

type Obj struct {
	Core      *core.Core
	Admin     *admin.AdminSocket
	Multicast *multicast.Multicast
	Netstack  *netstack.YggdrasilNetstack

	ctx           context.Context
	socksListener net.Listener
	socksAddr     string
	logger        core.Logger
	cancel        context.CancelFunc
	closeOnce     sync.Once
	closers       []io.Closer
	closersMu     sync.Mutex
}

func (o *Obj) addCloser(c io.Closer) {
	o.closersMu.Lock()
	o.closers = append(o.closers, c)
	o.closersMu.Unlock()
}

// //

type udpSessionObj struct {
	conn         net.Conn
	remoteAddr   net.Addr
	lastActivity atomic.Int64
}
