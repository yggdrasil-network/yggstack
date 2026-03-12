package yggstack

import (
	"fmt"
	"net/url"
)

// // // // // // // // // //

// AddPeer adds a persistent peer at runtime. URI: "tcp://host:port", "quic://host:port", etc.
func (o *Obj) AddPeer(uri string) error {
	if o.Core == nil {
		return fmt.Errorf("node is not running")
	}
	u, err := url.Parse(uri)
	if err != nil {
		return fmt.Errorf("url.Parse: %w", err)
	}
	return o.Core.AddPeer(u, "")
}

// RemovePeer removes a persistent peer at runtime.
func (o *Obj) RemovePeer(uri string) error {
	if o.Core == nil {
		return fmt.Errorf("node is not running")
	}
	u, err := url.Parse(uri)
	if err != nil {
		return fmt.Errorf("url.Parse: %w", err)
	}
	return o.Core.RemovePeer(u, "")
}

// RetryPeersNow forces an immediate reconnection attempt to all disconnected peers.
func (o *Obj) RetryPeersNow() {
	if o.Core == nil {
		return
	}
	o.Core.RetryPeersNow()
}
