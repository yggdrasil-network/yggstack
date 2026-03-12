package yggstack

import (
	"fmt"

	"github.com/yggdrasil-network/yggstack/mod/peers"
)

// // // // // // // // // //

// GetPeers returns a snapshot of all configured peers.
func (o *Obj) GetPeers() []peers.InfoObj {
	o.componentsMu.RLock()
	defer o.componentsMu.RUnlock()
	if o.Core == nil {
		return []peers.InfoObj{}
	}
	return peers.GetPeers(o.Core)
}

// GetPeersJSON returns peer statistics in JSON format.
func (o *Obj) GetPeersJSON() ([]byte, error) {
	o.componentsMu.RLock()
	defer o.componentsMu.RUnlock()
	if o.Core == nil {
		return nil, fmt.Errorf("node is not running")
	}
	return peers.GetPeersJSON(o.Core)
}

// AddPeer adds a persistent peer at runtime.
func (o *Obj) AddPeer(uri string) error {
	o.componentsMu.RLock()
	defer o.componentsMu.RUnlock()
	if o.Core == nil {
		return fmt.Errorf("node is not running")
	}
	return peers.AddPeer(o.Core, uri)
}

// RemovePeer removes a persistent peer at runtime.
func (o *Obj) RemovePeer(uri string) error {
	o.componentsMu.RLock()
	defer o.componentsMu.RUnlock()
	if o.Core == nil {
		return fmt.Errorf("node is not running")
	}
	return peers.RemovePeer(o.Core, uri)
}

// RetryPeersNow forces an immediate reconnection attempt to all disconnected peers.
func (o *Obj) RetryPeersNow() {
	o.componentsMu.RLock()
	defer o.componentsMu.RUnlock()
	if o.Core == nil {
		return
	}
	peers.RetryPeersNow(o.Core)
}
