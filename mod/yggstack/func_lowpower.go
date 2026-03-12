package yggstack

import "fmt"

// // // // // // // // // //

// IsLowPower returns true if the node is in sleep mode.
func (o *Obj) IsLowPower() bool {
	if o.lowPower == nil {
		return false
	}
	return o.lowPower.IsLowPower()
}

// ActiveConnections returns the number of tracked active connections.
func (o *Obj) ActiveConnections() int64 {
	return o.connCounter.Count()
}

// WakeLowPower forces the node to wake up from sleep mode.
func (o *Obj) WakeLowPower() error {
	if o.lowPower == nil {
		return fmt.Errorf("low power mode is not enabled")
	}
	if o.lowPower.IsLowPower() {
		o.lowPower.TransitionToFullPower()
	}
	return nil
}
