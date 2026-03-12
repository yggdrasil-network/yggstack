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
// Delegates to *Obj fields.
type NodeMappingObj struct {
	node *Obj
}

// //

func (a *NodeMappingObj) GetNetstack() *netstack.YggdrasilNetstack { return a.node.Netstack }
func (a *NodeMappingObj) GetCoreMTU() uint64                       { return a.node.Core.MTU() }
func (a *NodeMappingObj) GetLogger() core.Logger                   { return a.node.logger }
func (a *NodeMappingObj) GetActivityCallback() activity.CallbackInterface {
	return a.node.activityCallback
}
func (a *NodeMappingObj) GetConnCounter() *activity.CounterObj { return &a.node.connCounter }
func (a *NodeMappingObj) GetComponentsCtx() context.Context    { return a.node.componentsCtx }
func (a *NodeMappingObj) GetComponentsWg() *sync.WaitGroup     { return &a.node.componentsWg }
func (a *NodeMappingObj) AddCloser(c io.Closer)                { a.node.addCloser(c) }
