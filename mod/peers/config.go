package peers

import (
	"fmt"
	"net/url"
)

// // // // // // // // // //

// AddPeer adds a persistent peer at runtime. URI: "tcp://host:port", "quic://host:port", etc.
func AddPeer(c CoreInterface, uri string) error {
	if c == nil {
		return fmt.Errorf("node is not running")
	}
	u, err := url.Parse(uri)
	if err != nil {
		return fmt.Errorf("url.Parse: %w", err)
	}
	return c.AddPeer(u, "")
}

// RemovePeer removes a persistent peer at runtime.
func RemovePeer(c CoreInterface, uri string) error {
	if c == nil {
		return fmt.Errorf("node is not running")
	}
	u, err := url.Parse(uri)
	if err != nil {
		return fmt.Errorf("url.Parse: %w", err)
	}
	return c.RemovePeer(u, "")
}

// RetryPeersNow forces an immediate reconnection attempt to all disconnected peers.
func RetryPeersNow(c CoreInterface) {
	if c == nil {
		return
	}
	c.RetryPeersNow()
}
