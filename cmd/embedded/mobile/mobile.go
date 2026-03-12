// Package mobile provides gomobile bindings for Yggstack.
//
// Build for Android: gomobile bind -target=android -o yggstack.aar .
// Build for iOS:     gomobile bind -target=ios -o Yggstack.xcframework .
package mobile

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/yggdrasil-network/yggdrasil-go/src/config"
	"github.com/yggdrasil-network/yggstack/mod/yggstack"
	"github.com/yggdrasil-network/yggstack/src/types"
)

// // // // // // // // // //

// Yggstack is the main mobile binding type. Create with NewYggstack().
type Yggstack struct {
	mu         sync.Mutex
	node       *yggstack.Obj
	nodeCfg    *config.NodeConfig
	logBridge  *logBridgeObj
	peerBridge *peerBridgeObj

	// Options accumulated before Start()
	udpTimeoutMs int64
	coreStopMs   int64
	multicast    bool

	// Mappings accumulated before Start()
	localTCPs  []types.TCPMapping
	localUDPs  []types.UDPMapping
	remoteTCPs []types.TCPMapping
	remoteUDPs []types.UDPMapping
}

// NewYggstack creates a new Yggstack instance.
func NewYggstack() *Yggstack {
	return &Yggstack{
		logBridge:  newLogBridge(),
		peerBridge: newPeerBridge(),
	}
}

// // // // // // // // // //

// LoadConfigJSON parses a JSON configuration string and stores it for use on Start().
// The config must be valid JSON in Yggdrasil NodeConfig format (see GenerateConfig).
// Returns an error if the node is already running or the JSON is malformed.
func (y *Yggstack) LoadConfigJSON(jsonStr string) error {
	y.mu.Lock()
	defer y.mu.Unlock()
	if y.node != nil {
		return fmt.Errorf("cannot change config while running; call Stop() first")
	}
	// Use GenerateConfig as base to ensure all fields have sane defaults.
	// User-supplied JSON fields overwrite the defaults — including PrivateKey.
	base := config.GenerateConfig()
	base.AdminListen = "none"
	if err := json.Unmarshal([]byte(jsonStr), base); err != nil {
		return fmt.Errorf("config parse: %w", err)
	}
	base.AdminListen = "none" // enforce even if user JSON tried to set it
	y.nodeCfg = base
	return nil
}

// SetLogCallback registers a callback for log output.
// May be called at any time, including while running.
func (y *Yggstack) SetLogCallback(cb LogCallback) {
	y.logBridge.setCallback(cb)
}

// SetLogLevel sets the minimum log level to forward to LogCallback.
// Accepted values: "trace", "debug", "info" (default), "warn", "error".
func (y *Yggstack) SetLogLevel(level string) {
	y.logBridge.setLevel(level)
}

// SetPeerChangeCallback registers a callback for peer count change events.
// May be called at any time, including while running.
func (y *Yggstack) SetPeerChangeCallback(cb PeerChangeCallback) {
	y.peerBridge.setCallback(cb)
}

// SetCoreStopTimeout sets the maximum time in milliseconds to wait for the Yggdrasil
// core to stop on Close(). Useful on mobile where network switches can cause hangs.
// Zero means wait forever (default). Must be called before Start().
func (y *Yggstack) SetCoreStopTimeout(ms int64) {
	y.mu.Lock()
	y.coreStopMs = ms
	y.mu.Unlock()
}

// SetSessionTimeout sets the UDP session inactivity timeout in milliseconds.
// After this duration without traffic, the UDP session is closed. Default: 120000 (120s).
// Must be called before Start().
func (y *Yggstack) SetSessionTimeout(ms int64) {
	y.mu.Lock()
	y.udpTimeoutMs = ms
	y.mu.Unlock()
}

// SetMulticastEnabled enables or disables mDNS peer discovery on the local network.
// Must be called before Start().
func (y *Yggstack) SetMulticastEnabled(enabled bool) {
	y.mu.Lock()
	y.multicast = enabled
	y.mu.Unlock()
}

// // // // // // // // // //

// AddPeer adds a peer by URI. Supported schemes: tcp, tls, quic, ws, wss.
// When called before Start(): stored in config and applied on Start().
// When called while running: connects immediately.
func (y *Yggstack) AddPeer(uri string) error {
	y.mu.Lock()
	defer y.mu.Unlock()
	if y.node != nil {
		return y.node.AddPeer(uri)
	}
	if y.nodeCfg == nil {
		y.nodeCfg = config.GenerateConfig()
		y.nodeCfg.AdminListen = "none"
	}
	// Avoid duplicates
	for _, p := range y.nodeCfg.Peers {
		if p == uri {
			return nil
		}
	}
	y.nodeCfg.Peers = append(y.nodeCfg.Peers, uri)
	return nil
}

// RemovePeer removes a peer by URI.
// When called before Start(): removed from config.
// When called while running: disconnects immediately.
func (y *Yggstack) RemovePeer(uri string) error {
	y.mu.Lock()
	defer y.mu.Unlock()
	if y.node != nil {
		return y.node.RemovePeer(uri)
	}
	if y.nodeCfg == nil {
		return nil
	}
	filtered := y.nodeCfg.Peers[:0]
	for _, p := range y.nodeCfg.Peers {
		if p != uri {
			filtered = append(filtered, p)
		}
	}
	y.nodeCfg.Peers = filtered
	return nil
}

// // // // // // // // // //

// AddLocalTCPMapping adds a rule that forwards a local TCP port to a Yggdrasil address.
// local:  listen address, e.g. "127.0.0.1:8080"
// remote: Yggdrasil destination, e.g. "[200:1234::1]:80"
// Must be called before Start(); takes effect on next Start().
func (y *Yggstack) AddLocalTCPMapping(local, remote string) error {
	m, err := parseTCPMapping(local, remote)
	if err != nil {
		return err
	}
	y.mu.Lock()
	y.localTCPs = append(y.localTCPs, m)
	y.mu.Unlock()
	return nil
}

// AddLocalUDPMapping adds a rule that forwards a local UDP port to a Yggdrasil address.
// local:  listen address, e.g. "127.0.0.1:5353"
// remote: Yggdrasil destination, e.g. "[200:1234::1]:53"
// Must be called before Start(); takes effect on next Start().
func (y *Yggstack) AddLocalUDPMapping(local, remote string) error {
	m, err := parseUDPMapping(local, remote)
	if err != nil {
		return err
	}
	y.mu.Lock()
	y.localUDPs = append(y.localUDPs, m)
	y.mu.Unlock()
	return nil
}

// AddRemoteTCPMapping exposes a local TCP service on the Yggdrasil network.
// port:  the Yggdrasil-side listen port (1-65535)
// local: the local service to forward to, e.g. "127.0.0.1:80"
// Must be called before Start(); takes effect on next Start().
func (y *Yggstack) AddRemoteTCPMapping(port int, local string) error {
	m, err := parseRemoteTCPMapping(port, local)
	if err != nil {
		return err
	}
	y.mu.Lock()
	y.remoteTCPs = append(y.remoteTCPs, m)
	y.mu.Unlock()
	return nil
}

// AddRemoteUDPMapping exposes a local UDP service on the Yggdrasil network.
// port:  the Yggdrasil-side listen port (1-65535)
// local: the local service to forward to, e.g. "127.0.0.1:53"
// Must be called before Start(); takes effect on next Start().
func (y *Yggstack) AddRemoteUDPMapping(port int, local string) error {
	m, err := parseRemoteUDPMapping(port, local)
	if err != nil {
		return err
	}
	y.mu.Lock()
	y.remoteUDPs = append(y.remoteUDPs, m)
	y.mu.Unlock()
	return nil
}

// ClearLocalMappings removes all pending local TCP/UDP forwarding rules.
// Must be called before Start(); has no effect while running.
func (y *Yggstack) ClearLocalMappings() {
	y.mu.Lock()
	y.localTCPs = nil
	y.localUDPs = nil
	y.mu.Unlock()
}

// ClearRemoteMappings removes all pending remote TCP/UDP forwarding rules.
// Must be called before Start(); has no effect while running.
func (y *Yggstack) ClearRemoteMappings() {
	y.mu.Lock()
	y.remoteTCPs = nil
	y.remoteUDPs = nil
	y.mu.Unlock()
}

// // // // // // // // // //

// Start launches the Yggdrasil node with SOCKS5 proxy on socksAddr and optional
// Yggdrasil-side DNS nameserver. Returns an error if already running or start fails.
//
// socksAddr:  TCP or UNIX socket address for the SOCKS5 proxy, e.g. "127.0.0.1:1080"
//
//	or "/tmp/yggstack.sock". Empty string disables SOCKS5.
//
// nameserver: Yggdrasil-accessible DNS server for .ygg domains, e.g. "[324:71e::]53]:53".
//
//	Empty string disables external .ygg DNS resolution.
func (y *Yggstack) Start(socksAddr, nameserver string) error {
	y.mu.Lock()
	defer y.mu.Unlock()
	if y.node != nil {
		return fmt.Errorf("already running; call Stop() first")
	}

	nodeCfg := y.nodeCfg
	if nodeCfg == nil {
		nodeCfg = config.GenerateConfig()
		nodeCfg.AdminListen = "none"
	}

	cfg := yggstack.ConfigObj{
		Config:             nodeCfg,
		Logger:             y.logBridge,
		SocksAddr:          socksAddr,
		Nameserver:         nameserver,
		PeerChangeCallback: y.peerBridge,
		Mapping: yggstack.MappingConfigObj{
			LocalTCP:  y.localTCPs,
			LocalUDP:  y.localUDPs,
			RemoteTCP: y.remoteTCPs,
			RemoteUDP: y.remoteUDPs,
		},
	}
	if y.udpTimeoutMs > 0 {
		cfg.UDPSessionTimeout = time.Duration(y.udpTimeoutMs) * time.Millisecond
	}
	if y.coreStopMs > 0 {
		cfg.CoreStopTimeout = time.Duration(y.coreStopMs) * time.Millisecond
	}

	node, err := yggstack.New(cfg)
	if err != nil {
		return fmt.Errorf("start node: %w", err)
	}
	y.node = node
	return nil
}

// Stop shuts down the node and all port forwarding. Safe to call if not running.
func (y *Yggstack) Stop() error {
	y.mu.Lock()
	defer y.mu.Unlock()
	if y.node == nil {
		return nil
	}
	err := y.node.Close()
	y.node = nil
	return err
}

// IsRunning returns true if the node is currently started.
func (y *Yggstack) IsRunning() bool {
	y.mu.Lock()
	running := y.node != nil
	y.mu.Unlock()
	return running
}

// // // // // // // // // //

// GetAddress returns the node's Yggdrasil IPv6 address, e.g. "200:1234::1".
// Returns empty string if not running.
func (y *Yggstack) GetAddress() string {
	y.mu.Lock()
	node := y.node
	y.mu.Unlock()
	if node == nil {
		return ""
	}
	return node.Address().String()
}

// GetSubnet returns the node's Yggdrasil IPv6 subnet, e.g. "300:1234::/64".
// Returns empty string if not running.
func (y *Yggstack) GetSubnet() string {
	y.mu.Lock()
	node := y.node
	y.mu.Unlock()
	if node == nil {
		return ""
	}
	s := node.Subnet()
	return s.String()
}

// GetPublicKey returns the node's Ed25519 public key as a hex string.
// Returns empty string if not running.
func (y *Yggstack) GetPublicKey() string {
	y.mu.Lock()
	node := y.node
	y.mu.Unlock()
	if node == nil {
		return ""
	}
	return hex.EncodeToString(node.PublicKey())
}

// GetPeers returns the list of configured peer URIs as a JSON array.
// Includes both connected and disconnected peers.
// Returns "[]" if not running.
func (y *Yggstack) GetPeers() string {
	y.mu.Lock()
	node := y.node
	y.mu.Unlock()
	if node == nil {
		return "[]"
	}
	uris := make([]string, 0)
	for _, p := range node.GetPeers() {
		uris = append(uris, p.URI)
	}
	b, err := json.Marshal(uris)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// GetPeersJSON returns detailed peer statistics as a JSON array.
// Each entry includes URI, connection state, traffic counters, latency, and uptime.
// Returns "[]" if not running or on error.
func (y *Yggstack) GetPeersJSON() string {
	y.mu.Lock()
	node := y.node
	y.mu.Unlock()
	if node == nil {
		return "[]"
	}
	b, err := node.GetPeersJSON()
	if err != nil || b == nil {
		return "[]"
	}
	return string(b)
}

// RetryPeersNow forces an immediate reconnection attempt to all disconnected peers.
// No-op if not running.
func (y *Yggstack) RetryPeersNow() {
	y.mu.Lock()
	node := y.node
	y.mu.Unlock()
	if node != nil {
		node.RetryPeersNow()
	}
}

// TriggerPeerUpdate fires the PeerChangeCallback with the current peer count.
// Useful for refreshing UI state after registering a callback while already running.
// No-op if not running or no callback is set.
func (y *Yggstack) TriggerPeerUpdate() {
	y.mu.Lock()
	node := y.node
	y.mu.Unlock()
	if node == nil {
		return
	}
	var connected, total int64
	for _, p := range node.GetPeers() {
		total++
		if p.Up {
			connected++
		}
	}
	y.peerBridge.OnPeerCountChanged(connected, total)
}

// ActiveConnections returns the number of currently tracked active connections.
// Returns 0 if not running.
func (y *Yggstack) ActiveConnections() int64 {
	y.mu.Lock()
	node := y.node
	y.mu.Unlock()
	if node == nil {
		return 0
	}
	return node.ActiveConnections()
}
