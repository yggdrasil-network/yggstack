package yggstack

import (
	"context"
	"time"

	"github.com/yggdrasil-network/yggdrasil-go/src/core"
)

// // // // // // // // // //

// PeerChangeCallbackInterface is notified when the connected peer count changes.
type PeerChangeCallbackInterface interface {
	OnPeerCountChanged(connected int64, total int64)
}

// //

const (
	peerPollFast = 500 * time.Millisecond
	peerPollSlow = 5 * time.Second
)

// //

// peerMonitorObj polls core.GetPeers() with adaptive frequency.
type peerMonitorObj struct {
	core        *core.Core
	callback    PeerChangeCallbackInterface
	connCounter *connectionCounterObj
	ctx         context.Context
	cancel      context.CancelFunc
	lastConn    int64
	lastTotal   int64
}

func (m *peerMonitorObj) run() {
	ticker := time.NewTicker(peerPollSlow)
	defer ticker.Stop()

	// Initial snapshot
	m.poll()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.poll()

			if m.connCounter != nil && m.connCounter.count() > 0 {
				ticker.Reset(peerPollFast)
			} else {
				ticker.Reset(peerPollSlow)
			}
		}
	}
}

func (m *peerMonitorObj) poll() {
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
