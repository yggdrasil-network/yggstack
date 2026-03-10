package yggstack

import (
	"context"
	"net"
	"sync"

	"github.com/gologme/log"
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

	socksListener net.Listener
	socksAddr     string
	logger        *log.Logger
	cancel        context.CancelFunc
	closeOnce     sync.Once
}

// //

type udpSessionObj struct {
	conn       interface{}
	remoteAddr net.Addr
}
