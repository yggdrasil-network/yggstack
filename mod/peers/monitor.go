package peers

import (
	"context"
	"time"

	"github.com/yggdrasil-network/yggstack/mod/activity"
)

// // // // // // // // // //

const (
	pollFast = 500 * time.Millisecond
	pollSlow = 5 * time.Second
)

// //

// MonitorObj polls core.GetPeers() with adaptive frequency.
type MonitorObj struct {
	core        CoreInterface
	callback    ChangeCallbackInterface
	connCounter *activity.CounterObj
	ctx         context.Context
	cancel      context.CancelFunc
	lastConn    int64
	lastTotal   int64
}

// NewMonitor creates a new peer monitor.
func NewMonitor(core CoreInterface, callback ChangeCallbackInterface, connCounter *activity.CounterObj, ctx context.Context, cancel context.CancelFunc) *MonitorObj {
	return &MonitorObj{
		core:        core,
		callback:    callback,
		connCounter: connCounter,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Cancel stops the monitor.
func (m *MonitorObj) Cancel() {
	m.cancel()
}

func (m *MonitorObj) Run() {
	ticker := time.NewTicker(pollSlow)
	defer ticker.Stop()

	// Initial snapshot
	m.Poll()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.Poll()

			if m.connCounter != nil && m.connCounter.Count() > 0 {
				ticker.Reset(pollFast)
			} else {
				ticker.Reset(pollSlow)
			}
		}
	}
}

func (m *MonitorObj) Poll() {
	if m.ctx.Err() != nil {
		return
	}
	peers := m.core.GetPeers()
	var connected, total int64
	for _, p := range peers {
		total++
		if p.Up {
			connected++
		}
	}
	if connected != m.lastConn || total != m.lastTotal {
		m.lastConn = connected
		m.lastTotal = total
		m.callback.OnPeerCountChanged(connected, total)
	}
}
