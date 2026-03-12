package activity

import (
	"sync/atomic"
)

// // // // // // // // // //

// CounterObj is an atomic counter for active tracked connections.
type CounterObj struct {
	active atomic.Int64
}

func (c *CounterObj) Increment()   { c.active.Add(1) }
func (c *CounterObj) Decrement()   { c.active.Add(-1) }
func (c *CounterObj) Count() int64 { return c.active.Load() }
