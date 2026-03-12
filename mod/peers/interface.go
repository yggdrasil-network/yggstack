package peers

import (
	"net/url"

	"github.com/yggdrasil-network/yggdrasil-go/src/core"
)

// // // // // // // // // //

// CoreInterface abstracts the Yggdrasil core methods used by the peers package.
// *core.Core satisfies this implicitly.
type CoreInterface interface {
	GetPeers() []core.PeerInfo
	AddPeer(u *url.URL, sintf string) error
	RemovePeer(u *url.URL, sintf string) error
	RetryPeersNow()
}

// ChangeCallbackInterface is notified when the connected peer count changes.
// Implementations must not block.
type ChangeCallbackInterface interface {
	OnPeerCountChanged(connected int64, total int64)
}

// MonitorInterface is the contract for a peer monitor component.
type MonitorInterface interface {
	Run()
	Cancel()
}
