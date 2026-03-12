package lowpower

import (
	"github.com/yggdrasil-network/yggstack/mod/activity"
)

// // // // // // // // // //

// NodeControlInterface decouples the low power manager from the node implementation.
type NodeControlInterface interface {
	StopComponents()
	StartComponents(origCfg interface{}) error
	ConnCounter() *activity.CounterObj
	SocksAddr() string
	SocksIsUnix() bool
	SocksReadyCh() <-chan struct{}
}

// ManagerInterface is the contract for a low power manager component.
type ManagerInterface interface {
	Run()
	Stop()
	IsLowPower() bool
	TransitionToFullPower()
	SetOrigConfig(cfg interface{})
	OrigConfig() interface{}
	GetState() int32
}
