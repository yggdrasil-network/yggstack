package yggstack

import (
	"context"
	"crypto/ed25519"
	"net"
)

// // // // // // // // // //

// Public contract for a Yggdrasil node
type Interface interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
	DialTCP(addr *net.TCPAddr) (net.Conn, error)
	DialUDP(addr *net.UDPAddr) (net.Conn, error)
	ListenTCP(addr *net.TCPAddr) (net.Listener, error)
	ListenUDP(addr *net.UDPAddr) (net.PacketConn, error)
	Address() net.IP
	Subnet() net.IPNet
	PublicKey() ed25519.PublicKey
	GetPeers() []PeerInfoObj
	GetPeersJSON() ([]byte, error)
	AddPeer(uri string) error
	RemovePeer(uri string) error
	RetryPeersNow()
	Close() error
}

// Compile-time check
var _ Interface = (*Obj)(nil)
