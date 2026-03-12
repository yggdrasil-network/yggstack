package yggstack

import (
	"context"
	"time"

	golog "github.com/gologme/log"

	"github.com/yggdrasil-network/yggdrasil-go/src/config"
	"github.com/yggdrasil-network/yggdrasil-go/src/core"

	"github.com/yggdrasil-network/yggstack/src/types"
)

// // // // // // // // // //

type ConfigObj struct {
	// Ctx is the parent context for the node lifecycle.
	// When cancelled, the node shuts down gracefully.
	// If nil, context.Background() is used.
	Ctx context.Context

	// Config is the Yggdrasil node configuration (keys, peers, listen addresses, etc.).
	// Generated via config.GenerateConfig(). Peers are specified in URI format:
	// tcp://host:port, tls://host:port, quic://host:port, ws://host:port, wss://host:port.
	// If nil, a new config with random keys and AdminListen="none" is generated.
	Config *config.NodeConfig

	// Logger receives all node log output (peer connections, errors, etc.).
	// Must implement yggdrasil-go core.Logger interface (Println, Infof, Warnf, Errorf, etc.).
	// If nil, all log output is discarded.
	Logger core.Logger

	// MulticastLogger enables mDNS peer discovery on local network.
	// Requires *log.Logger (not core.Logger) due to upstream multicast.New() signature.
	// If nil, multicast is disabled entirely.
	// TODO: switch to core.Logger when multicast.New() accepts an interface instead of *log.Logger
	MulticastLogger *golog.Logger

	// SocksAddr starts a SOCKS5 proxy server on the given address.
	// TCP format: "127.0.0.1:1080" or ":1080".
	// UNIX socket format: "/tmp/yggstack.sock" (no colon in the string).
	// Supports CONNECT (TCP) and UDP ASSOCIATE commands.
	// Resolves <publickey>.pk.ygg domains to Yggdrasil IPv6 addresses automatically.
	// If empty, SOCKS5 proxy is not started.
	SocksAddr string

	// Nameserver is a Yggdrasil-accessible DNS server for resolving .ygg domains via SOCKS5.
	// Format: "[ipv6]:port", e.g. "[324:71e:281a:9ed3::53]:53".
	// Without this, only <publickey>.pk.ygg names resolve; other .ygg domains will fail.
	// Only used when SocksAddr is set.
	Nameserver string

	// SocksVerbose enables detailed SOCKS5 connection logging.
	// Only used when SocksAddr is set.
	SocksVerbose bool

	// Mapping holds all port forwarding rules (local and remote, TCP and UDP).
	Mapping MappingConfigObj

	// UDPSessionTimeout is the inactivity timeout for UDP forwarding sessions.
	// After this duration without traffic, the session is closed and resources are freed.
	// Default: 120s.
	UDPSessionTimeout time.Duration

	// ActivityCallback receives notifications on connection lifecycle (create/transfer/close).
	// When nil, connections are not wrapped — zero overhead.
	ActivityCallback ActivityCallbackInterface

	// PeerChangeCallback receives notifications when the number of connected peers changes.
	// Uses adaptive polling: 500ms with active connections, 5s when idle.
	// When nil, peer monitoring is disabled.
	PeerChangeCallback PeerChangeCallbackInterface

	// CoreStopTimeout limits the time spent waiting for core.Stop() to complete.
	// When the timeout is exceeded, shutdown continues without waiting.
	// Relevant when switching networks (WiFi → LTE), where peer closure can hang indefinitely.
	// If 0 — waits forever (default, backward-compatible behavior).
	CoreStopTimeout time.Duration

	// LowPower enables power saving: when no active connections exist for longer than
	// IdleTimeout, the node stops. On incoming connection — restarts automatically.
	// nil = disabled. Requires ActivityCallback != nil.
	LowPower *LowPowerConfigObj
}

// //

// MappingConfigObj holds all port forwarding rules.
type MappingConfigObj struct {
	// LocalTCP forwards a local TCP port to a remote Yggdrasil address (like ssh -L).
	// Each entry maps Listen (local host:port) -> Mapped (remote Yggdrasil IPv6:port).
	// Example: listen 127.0.0.1:8080 -> forward to [ygg-ipv6]:8080.
	// CLI equivalent: -local-tcp 127.0.0.1:8080:<remote-yggdrasil-ipv6>:8080
	LocalTCP []types.TCPMapping

	// LocalUDP forwards a local UDP port to a remote Yggdrasil address (like ssh -L for UDP).
	// Each entry maps Listen (local host:port) -> Mapped (remote Yggdrasil IPv6:port).
	// Example: listen 127.0.0.1:5353 -> forward to [ygg-ipv6]:53.
	// CLI equivalent: -local-udp 127.0.0.1:5353:<remote-yggdrasil-ipv6>:53
	LocalUDP []types.UDPMapping

	// RemoteTCP exposes a local TCP service to the Yggdrasil network (like ssh -R).
	// Each entry maps Listen (Yggdrasil-side port) -> Mapped (local host:port).
	// Listen address is always the node's own Yggdrasil IPv6; only the port is specified.
	// Mapped defaults to [::1] (IPv6 loopback) if address is omitted.
	// Example: ygg-port 80 -> forward to 127.0.0.1:8080.
	// CLI equivalent: -remote-tcp 80:127.0.0.1:8080
	RemoteTCP []types.TCPMapping

	// RemoteUDP exposes a local UDP service to the Yggdrasil network (like ssh -R for UDP).
	// Each entry maps Listen (Yggdrasil-side port) -> Mapped (local host:port).
	// Listen address is always the node's own Yggdrasil IPv6; only the port is specified.
	// Mapped defaults to [::1] (IPv6 loopback) if address is omitted.
	// Example: ygg-port 53 -> forward to 127.0.0.1:53.
	// CLI equivalent: -remote-udp 53:127.0.0.1:53
	RemoteUDP []types.UDPMapping
}

// LowPowerConfigObj holds low power mode parameters.
type LowPowerConfigObj struct {
	// IdleTimeout is the idle duration before entering sleep. Default: 60s.
	IdleTimeout time.Duration
}
