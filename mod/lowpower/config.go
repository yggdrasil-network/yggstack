package lowpower

import (
	"time"
)

// // // // // // // // // //

// ConfigObj holds low power mode parameters.
type ConfigObj struct {
	// IdleTimeout is the idle duration before entering sleep. Default: 60s.
	IdleTimeout time.Duration
}
