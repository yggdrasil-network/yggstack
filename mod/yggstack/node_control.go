package yggstack

import (
	"fmt"

	"github.com/yggdrasil-network/yggstack/mod/activity"
)

// // // // // // // // // //

// NodeControlObj is the default lowpower.NodeControlInterface adapter.
type NodeControlObj struct {
	node *Obj
}

// //

func (a *NodeControlObj) StopComponents() { a.node.stopComponents() }
func (a *NodeControlObj) StartComponents(origCfg interface{}) error {
	cfg, ok := origCfg.(ConfigObj)
	if !ok {
		return fmt.Errorf("StartComponents: expected ConfigObj, got %T", origCfg)
	}
	return a.node.startComponents(cfg)
}
func (a *NodeControlObj) ConnCounter() *activity.CounterObj { return &a.node.connCounter }
func (a *NodeControlObj) SocksAddr() string                 { return a.node.socksAddr }
func (a *NodeControlObj) SocksIsUnix() bool                 { return a.node.socksIsUnix }
func (a *NodeControlObj) SocksReadyCh() <-chan struct{}     { return a.node.socksReadyCh }
