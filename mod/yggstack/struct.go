package yggstack

import (
	"context"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yggdrasil-network/yggdrasil-go/src/admin"
	"github.com/yggdrasil-network/yggdrasil-go/src/config"
	"github.com/yggdrasil-network/yggdrasil-go/src/core"
	"github.com/yggdrasil-network/yggdrasil-go/src/multicast"

	"github.com/yggdrasil-network/yggstack/mod/activity"
	"github.com/yggdrasil-network/yggstack/mod/lowpower"
	"github.com/yggdrasil-network/yggstack/mod/mapping"
	"github.com/yggdrasil-network/yggstack/mod/peers"
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

	// Netstack -- userspace gVisor TCP/UDP stack over Yggdrasil.
	// WARNING: when LowPower != nil this field becomes nil during node sleep.
	// Direct access (obj.Netstack.DialContext) will cause nil dereference.
	// Use Obj methods: DialContext, DialTCP, DialUDP, ListenTCP, ListenUDP --
	// they handle LPM correctly via netstackPtr atomic load.
	// Unsafe: calling Netstack.Close() directly bypasses Close() cleanup sequence.
	Netstack *netstack.YggdrasilNetstack

	ctx              context.Context
	netstackPtr      atomic.Pointer[netstack.YggdrasilNetstack]
	socksListener    net.Listener
	socksReadyCh     chan struct{} // closed when SOCKS listener is ready after wake
	socksAddr        string
	socksIsUnix      bool
	coreStopTimeout  time.Duration
	logger           core.Logger
	cancel           context.CancelFunc
	closeOnce        sync.Once
	closers          []io.Closer
	closersMu        sync.Mutex
	activityCallback activity.CallbackInterface
	connCounter      activity.CounterObj
	peerMonitor      peers.MonitorInterface
	nodeConfig       *config.NodeConfig
	lowPower         lowpower.ManagerInterface
	nodeMapping      mapping.NodeInterface
	nodeControl      lowpower.NodeControlInterface
	componentsMu     sync.RWMutex
	componentsCtx    context.Context
	componentsCancel context.CancelFunc
	componentsWg     sync.WaitGroup
}

func (o *Obj) addCloser(c io.Closer) {
	o.closersMu.Lock()
	o.closers = append(o.closers, c)
	o.closersMu.Unlock()
}
