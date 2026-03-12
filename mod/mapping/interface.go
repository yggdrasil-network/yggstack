package mapping

import (
	"context"
	"io"
	"sync"

	"github.com/yggdrasil-network/yggdrasil-go/src/core"

	"github.com/yggdrasil-network/yggstack/mod/activity"
	"github.com/yggdrasil-network/yggstack/src/netstack"
)

// // // // // // // // // //

// NodeInterface defines node dependencies required by mapping functions.
type NodeInterface interface {
	GetNetstack() *netstack.YggdrasilNetstack
	GetCoreMTU() uint64
	GetLogger() core.Logger
	GetActivityCallback() activity.CallbackInterface
	GetConnCounter() *activity.CounterObj
	GetComponentsCtx() context.Context
	GetComponentsWg() *sync.WaitGroup
	AddCloser(io.Closer)
}
