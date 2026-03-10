package yggstack

import (
	"context"

	"github.com/gologme/log"
	"github.com/yggdrasil-network/yggdrasil-go/src/config"

	"github.com/yggdrasil-network/yggstack/src/types"
)

// // // // // // // // // //

type ConfigObj struct {
	Ctx      context.Context
	Config   *config.NodeConfig
	Logger   *log.Logger
	LogLevel string

	// SOCKS5: TCP address (":1080") or UNIX socket path ("/tmp/yggstack.sock")
	SocksAddr  string
	Nameserver string

	// Port forwarding
	LocalTCP  []types.TCPMapping
	LocalUDP  []types.UDPMapping
	RemoteTCP []types.TCPMapping
	RemoteUDP []types.UDPMapping
}
