package yggstack

import (
	"context"
	"io"
	"sync"

	"github.com/yggdrasil-network/yggdrasil-go/src/core"

	"github.com/yggdrasil-network/yggstack/mod/activity"
	"github.com/yggdrasil-network/yggstack/src/netstack"
)

// // // // // // // // // //

// NodeMappingObj is the default mapping.NodeInterface adapter.
type NodeMappingObj struct {
	node *Obj
}

// //

func (a *NodeMappingObj) GetNetstack() *netstack.YggdrasilNetstack { return a.node.netstackPtr.Load() }

// GetCoreMTU returns the Core MTU.
// Called only during component initialization (from goroutines spawned in initNetworking).
// Happens-before is guaranteed by the go statement in StartLocalUDP/StartRemoteUDP.
// No lock needed: value is read once and cached locally by callers.
func (a *NodeMappingObj) GetCoreMTU() uint64 {
	if a.node.Core == nil {
		return 0
	}
	return a.node.Core.MTU()
}

func (a *NodeMappingObj) GetLogger() core.Logger { return a.node.logger }
func (a *NodeMappingObj) GetActivityCallback() activity.CallbackInterface {
	return a.node.activityCallback
}
func (a *NodeMappingObj) GetConnCounter() *activity.CounterObj { return &a.node.connCounter }

// GetComponentsCtx returns the component-generation context.
// Called synchronously from initNetworking (same goroutine as startComponents which holds
// the write lock — RLock would deadlock) and from goroutines spawned during init
// (happens-before via go statement). No lock needed.
func (a *NodeMappingObj) GetComponentsCtx() context.Context {
	return a.node.componentsCtx
}

func (a *NodeMappingObj) GetComponentsWg() *sync.WaitGroup { return &a.node.componentsWg }
func (a *NodeMappingObj) AddCloser(c io.Closer)            { a.node.addCloser(c) }
