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
	// Core is the underlying Yggdrasil node instance.
	// Unsafe: calling Core.Stop() directly bypasses Close() cleanup sequence
	// and will leak netstack, admin socket, and port forwarding resources.
	// Use Close() for proper shutdown.
	Core *core.Core

	// Admin is the management API socket. May be nil if AdminListen is set to "none".
	// Unsafe: calling Admin.Stop() directly bypasses Close() cleanup sequence.
	Admin *admin.AdminSocket

	// Multicast handles mDNS peer discovery on the local network.
	// Nil when ConfigObj.MulticastLogger was not provided.
	Multicast *multicast.Multicast

	// Netstack is the gVisor userspace network stack bridging TCP/UDP over Yggdrasil.
	// Prefer using DialContext, DialTCP, DialUDP, ListenTCP, ListenUDP methods
	// on Obj/Interface instead of accessing Netstack directly.
	// Unsafe: calling Netstack.Close() directly bypasses Close() cleanup sequence.
	Netstack *netstack.YggdrasilNetstack

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
