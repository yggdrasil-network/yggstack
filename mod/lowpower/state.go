package lowpower

import (
	"time"
)

// // // // // // // // // //

const (
	StateFullPower = int32(iota)
	StateStopping
	StateLowPower
	StateStarting
)

const (
	idleCheckInterval = 10 * time.Second
)
