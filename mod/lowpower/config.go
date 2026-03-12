package lowpower

import (
	"time"
)

// // // // // // // // // //

type ConfigObj struct {
	// IdleTimeout is the idle duration before entering sleep. Default: 60s.
	IdleTimeout time.Duration
}
