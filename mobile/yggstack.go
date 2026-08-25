// Package mobile provides Android/iOS bindings for Yggstack
package mobile

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gologme/log"
	"github.com/hjson/hjson-go/v4"
	"github.com/things-go/go-socks5"

	"github.com/yggdrasil-network/yggdrasil-go/src/address"
	"github.com/yggdrasil-network/yggdrasil-go/src/admin"
	"github.com/yggdrasil-network/yggdrasil-go/src/config"
	"github.com/yggdrasil-network/yggdrasil-go/src/core"
	"github.com/yggdrasil-network/yggdrasil-go/src/multicast"

	"github.com/yggdrasil-network/yggstack/src/netstack"
	"github.com/yggdrasil-network/yggstack/src/types"
)

// Yggstack is the main mobile binding object
type Yggstack struct {
	core         *core.Core
	multicast    *multicast.Multicast
	admin        *admin.AdminSocket
	netstack     *netstack.YggdrasilNetstack
	socks5Server *socks5.Server
	socks5Tcp    net.Listener
	logger       *log.Logger
	config       *config.NodeConfig
	ctx          context.Context
	cancel       context.CancelFunc

	// Port mappings
	localTCPMappings  []types.TCPMapping
	localUDPMappings  []types.UDPMapping
	remoteTCPMappings []types.TCPMapping
	remoteUDPMappings []types.UDPMapping

	// Active connections tracking for cleanup
	activeConns   []net.Conn
	activeConnsMu sync.Mutex

	// Active listeners tracking for cleanup (TCP listeners and UDP conns)
	activeListeners   []io.Closer
	activeListenersMu sync.Mutex

	// Per-mapping context cancellation and listener references for on-the-fly enable/disable
	mappingCancels   sync.Map // string key → context.CancelFunc
	mappingListeners sync.Map // string key → io.Closer

	// State
	isRunning  bool
	handlersWg sync.WaitGroup // Wait group for handler goroutines
	mu         sync.RWMutex
}

// LogWriter implements io.Writer for Android logging
type LogWriter struct {
	callback LogCallback
}

// LogCallback is called when logs are generated
type LogCallback interface {
	OnLog(message string)
}

func (w *LogWriter) Write(p []byte) (n int, err error) {
	if w.callback != nil {
		w.callback.OnLog(string(p))
	}
	return len(p), nil
}

// NewYggstack creates a new Yggstack instance
func NewYggstack() *Yggstack {
	return &Yggstack{
		isRunning: false,
		logger:    log.New(os.Stdout, "", log.Flags()),
	}
}

// Mapping key helpers (used for per-mapping cancel / listener tracking)
func localTCPMappingKey(listenAddr, mappedAddr string) string {
	return "ltcp:" + listenAddr + "->" + mappedAddr
}
func localUDPMappingKey(listenAddr, mappedAddr string) string {
	return "ludp:" + listenAddr + "->" + mappedAddr
}
func remoteTCPMappingKey(remotePort int, localAddr string) string {
	return fmt.Sprintf("rtcp:%d->%s", remotePort, localAddr)
}
func remoteUDPMappingKey(remotePort int, localAddr string) string {
	return fmt.Sprintf("rudp:%d->%s", remotePort, localAddr)
}

// SetLogCallback sets custom log callback for Android
func (y *Yggstack) SetLogCallback(callback LogCallback) {
	if callback != nil {
		writer := &LogWriter{callback: callback}
		y.logger = log.New(writer, "", log.Flags())
	}
}

// SetLogLevel sets the logging level (info, warn, error, debug)
func (y *Yggstack) SetLogLevel(level string) {
	switch strings.ToLower(level) {
	case "error":
		y.logger.EnableLevel("error")
	case "warn":
		y.logger.EnableLevel("error")
		y.logger.EnableLevel("warn")
	case "info":
		y.logger.EnableLevel("error")
		y.logger.EnableLevel("warn")
		y.logger.EnableLevel("info")
	case "debug":
		y.logger.EnableLevel("error")
		y.logger.EnableLevel("warn")
		y.logger.EnableLevel("info")
		y.logger.EnableLevel("debug")
		y.logger.EnableLevel("trace")
	}
}

// GenerateConfig generates a new random configuration and returns it as JSON string
func GenerateConfig() (string, error) {
	cfg := config.GenerateConfig()
	cfg.AdminListen = "none"

	bs, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal config: %w", err)
	}
	return string(bs), nil
}

// LoadConfigJSON loads configuration from JSON string
func (y *Yggstack) LoadConfigJSON(configJSON string) error {
	y.mu.Lock()
	defer y.mu.Unlock()

	if y.isRunning {
		return fmt.Errorf("cannot load config while Yggstack is running")
	}

	cfg := &config.NodeConfig{}
	if err := json.Unmarshal([]byte(configJSON), cfg); err != nil {
		// Try HJSON format
		if err := hjson.Unmarshal([]byte(configJSON), cfg); err != nil {
			return fmt.Errorf("failed to parse config: %w", err)
		}
	}

	cfg.AdminListen = "none"
	y.config = cfg
	return nil
}

// GetAddress returns the IPv6 address for this node
func (y *Yggstack) GetAddress() (string, error) {
	y.mu.RLock()
	defer y.mu.RUnlock()

	if y.config == nil {
		return "", fmt.Errorf("config not loaded")
	}

	privateKey := ed25519.PrivateKey(y.config.PrivateKey)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	addr := address.AddrForKey(publicKey)
	ip := net.IP(addr[:])
	return ip.String(), nil
}

// GetSubnet returns the IPv6 subnet for this node
func (y *Yggstack) GetSubnet() (string, error) {
	y.mu.RLock()
	defer y.mu.RUnlock()

	if y.config == nil {
		return "", fmt.Errorf("config not loaded")
	}

	privateKey := ed25519.PrivateKey(y.config.PrivateKey)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	snet := address.SubnetForKey(publicKey)
	ipnet := net.IPNet{
		IP:   append(snet[:], 0, 0, 0, 0, 0, 0, 0, 0),
		Mask: net.CIDRMask(len(snet)*8, 128),
	}
	return ipnet.String(), nil
}

// GetPublicKey returns the public key for this node
func (y *Yggstack) GetPublicKey() (string, error) {
	y.mu.RLock()
	defer y.mu.RUnlock()

	if y.config == nil {
		return "", fmt.Errorf("config not loaded")
	}

	privateKey := ed25519.PrivateKey(y.config.PrivateKey)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	return hex.EncodeToString(publicKey), nil
}

// AddPeer adds a peer to the configuration
func (y *Yggstack) AddPeer(peerURI string) error {
	y.mu.Lock()
	defer y.mu.Unlock()

	if y.config == nil {
		return fmt.Errorf("config not loaded")
	}

	// Check if peer already exists
	for _, peer := range y.config.Peers {
		if peer == peerURI {
			return nil // Already exists
		}
	}

	y.config.Peers = append(y.config.Peers, peerURI)
	return nil
}

// RemovePeer removes a peer from the configuration
func (y *Yggstack) RemovePeer(peerURI string) error {
	y.mu.Lock()
	defer y.mu.Unlock()

	if y.config == nil {
		return fmt.Errorf("config not loaded")
	}

	newPeers := []string{}
	for _, peer := range y.config.Peers {
		if peer != peerURI {
			newPeers = append(newPeers, peer)
		}
	}
	y.config.Peers = newPeers
	return nil
}

// GetPeers returns the list of configured peers as JSON string
func (y *Yggstack) GetPeers() (string, error) {
	y.mu.RLock()
	defer y.mu.RUnlock()

	if y.config == nil {
		return "", fmt.Errorf("config not loaded")
	}

	bs, err := json.Marshal(y.config.Peers)
	if err != nil {
		return "", fmt.Errorf("failed to marshal peers: %w", err)
	}
	return string(bs), nil
}

// GetPeersJSON returns detailed information about connected peers as JSON
func (y *Yggstack) GetPeersJSON() (string, error) {
	y.mu.RLock()
	defer y.mu.RUnlock()

	if y.core == nil {
		return "[]", fmt.Errorf("core not initialized")
	}

	peersInfo := y.core.GetPeers()
	bs, err := json.Marshal(peersInfo)
	if err != nil {
		return "[]", fmt.Errorf("failed to marshal peers info: %w", err)
	}
	return string(bs), nil
}

// AddLivePeer adds a peer to the running yggdrasil core without restarting.
// peerURI should be a full URI, e.g. "tcp://host:port?maxbackoff=5s".
// Returns an error if the node is not currently running.
func (y *Yggstack) AddLivePeer(peerURI string) error {
	y.mu.RLock()
	defer y.mu.RUnlock()

	if !y.isRunning || y.core == nil {
		return fmt.Errorf("Yggstack is not running")
	}

	u, err := url.Parse(peerURI)
	if err != nil {
		return fmt.Errorf("invalid peer URI %q: %w", peerURI, err)
	}
	return y.core.AddPeer(u, "")
}

// RemoveLivePeer removes a peer from the running yggdrasil core without restarting.
// peerURI should be a full URI matching what was passed when the peer was added,
// e.g. "tcp://host:port?maxbackoff=5s".
// Returns an error if the node is not currently running.
func (y *Yggstack) RemoveLivePeer(peerURI string) error {
	y.mu.RLock()
	defer y.mu.RUnlock()

	if !y.isRunning || y.core == nil {
		return fmt.Errorf("Yggstack is not running")
	}

	u, err := url.Parse(peerURI)
	if err != nil {
		return fmt.Errorf("invalid peer URI %q: %w", peerURI, err)
	}
	return y.core.RemovePeer(u, "")
}

// RetryPeersNow forces immediate reconnection attempt for all peers
// This should be called when network connectivity changes (e.g., WiFi <-> Cellular)
func (y *Yggstack) RetryPeersNow() error {
	y.mu.RLock()
	defer y.mu.RUnlock()

	if !y.isRunning {
		return fmt.Errorf("Yggstack is not running")
	}

	if y.core == nil {
		return fmt.Errorf("core not initialized")
	}

	y.logger.Infof("Forcing immediate peer retry...")
	y.core.RetryPeersNow()
	y.logger.Infof("RetryPeersNow() completed - peers should reconnect immediately")
	return nil
}

// Start starts the Yggstack node with optional SOCKS listener and nameserver
func (y *Yggstack) Start(socksAddress string, nameserver string) error {
	y.mu.Lock()
	defer y.mu.Unlock()

	if y.isRunning {
		return fmt.Errorf("Yggstack is already running")
	}

	if y.config == nil {
		return fmt.Errorf("config not loaded, call LoadConfigJSON first")
	}

	y.ctx, y.cancel = context.WithCancel(context.Background())

	// Generate self-signed certificate if not already present
	if y.config.Certificate == nil {
		if err := y.config.GenerateSelfSignedCertificate(); err != nil {
			return fmt.Errorf("failed to generate certificate: %w", err)
		}
	}

	// Setup the Yggdrasil core
	var err error
	privateKey := ed25519.PrivateKey(y.config.PrivateKey)
	options := []core.SetupOption{
		core.NodeInfo(y.config.NodeInfo),
		core.NodeInfoPrivacy(y.config.NodeInfoPrivacy),
	}

	for _, addr := range y.config.Listen {
		options = append(options, core.ListenAddress(addr))
	}

	for _, peer := range y.config.Peers {
		options = append(options, core.Peer{URI: peer})
	}

	for intf, peers := range y.config.InterfacePeers {
		for _, peer := range peers {
			options = append(options, core.Peer{URI: peer, SourceInterface: intf})
		}
	}

	for _, allowed := range y.config.AllowedPublicKeys {
		k, err := hex.DecodeString(allowed)
		if err != nil {
			return fmt.Errorf("invalid allowed public key: %w", err)
		}
		options = append(options, core.AllowedPublicKey(k[:]))
	}

	if y.config.GroupPassword != "" {
		options = append(options, core.GroupPassword(y.config.GroupPassword))
	}

	if y.core, err = core.New(y.config.Certificate, y.logger, options...); err != nil {
		return fmt.Errorf("failed to create core: %w", err)
	}

	address, subnet := y.core.Address(), y.core.Subnet()
	publicKey := privateKey.Public().(ed25519.PublicKey)
	publicstr := hex.EncodeToString(publicKey)
	y.logger.Infof("Your public key is %s", publicstr)
	y.logger.Infof("Your IPv6 address is %s", address.String())
	y.logger.Infof("Your IPv6 subnet is %s", subnet.String())
	y.logger.Infof("Your Yggstack resolver name is %s%s", publicstr, types.NameMappingSuffix)

	// Setup the admin socket (disabled for mobile)
	adminOptions := []admin.SetupOption{
		admin.ListenAddress("none"),
	}
	if y.admin, err = admin.New(y.core, y.logger, adminOptions...); err != nil {
		return fmt.Errorf("failed to create admin: %w", err)
	}

	// Setup the multicast module
	multicastOptions := []multicast.SetupOption{}
	for _, intf := range y.config.MulticastInterfaces {
		multicastOptions = append(multicastOptions, multicast.MulticastInterface{
			Regex:    regexp.MustCompile(intf.Regex),
			Beacon:   intf.Beacon,
			Listen:   intf.Listen,
			Port:     intf.Port,
			Priority: uint8(intf.Priority),
			Password: intf.Password,
		})
	}

	if y.multicast, err = multicast.New(y.core, y.logger, multicastOptions...); err != nil {
		return fmt.Errorf("failed to create multicast: %w", err)
	}

	// Setup Yggdrasil netstack
	if y.netstack, err = netstack.CreateYggdrasilNetstack(y.core); err != nil {
		return fmt.Errorf("failed to create netstack: %w", err)
	}

	// Start SOCKS server if requested
	if socksAddress != "" {
		socksOptions := []socks5.Option{
			socks5.WithDial(y.netstack.DialContext),
		}

		if nameserver != "" {
			resolver := types.NewNameResolver(y.netstack, nameserver)
			socksOptions = append(socksOptions, socks5.WithResolver(resolver))
			y.logger.Infof("Using DNS nameserver: %s", nameserver)
		} else {
			y.logger.Infof("DNS nameserver is not set!")
			y.logger.Infof("SOCKS server will not be able to resolve hostnames other than .pk.ygg!")
		}

		y.socks5Server = socks5.NewServer(socksOptions...)
		y.logger.Infof("Starting SOCKS server on %s", socksAddress)

		if y.socks5Tcp, err = net.Listen("tcp", socksAddress); err != nil {
			y.Stop()
			return fmt.Errorf("failed to start SOCKS listener: %w", err)
		}

		go func() {
			if err := y.socks5Server.Serve(y.socks5Tcp); err != nil {
				y.logger.Errorf("SOCKS server error: %s", err)
			}
		}()
	}

	// Setup local TCP mappings
	for _, mapping := range y.localTCPMappings {
		key := localTCPMappingKey(mapping.Listen.String(), mapping.Mapped.String())
		childCtx, cancel := context.WithCancel(y.ctx)
		y.mappingCancels.Store(key, cancel)
		y.handlersWg.Add(1)
		go y.handleLocalTCPMappingCtx(childCtx, key, mapping)
	}

	// Setup local UDP mappings
	for _, mapping := range y.localUDPMappings {
		key := localUDPMappingKey(mapping.Listen.String(), mapping.Mapped.String())
		childCtx, cancel := context.WithCancel(y.ctx)
		y.mappingCancels.Store(key, cancel)
		y.handlersWg.Add(1)
		go y.handleLocalUDPMappingCtx(childCtx, key, mapping)
	}

	// Setup remote TCP mappings
	for _, mapping := range y.remoteTCPMappings {
		key := remoteTCPMappingKey(mapping.Listen.Port, mapping.Mapped.String())
		childCtx, cancel := context.WithCancel(y.ctx)
		y.mappingCancels.Store(key, cancel)
		y.handlersWg.Add(1)
		go y.handleRemoteTCPMappingCtx(childCtx, key, mapping)
	}

	// Setup remote UDP mappings
	for _, mapping := range y.remoteUDPMappings {
		key := remoteUDPMappingKey(mapping.Listen.Port, mapping.Mapped.String())
		childCtx, cancel := context.WithCancel(y.ctx)
		y.mappingCancels.Store(key, cancel)
		y.handlersWg.Add(1)
		go y.handleRemoteUDPMappingCtx(childCtx, key, mapping)
	}

	y.isRunning = true
	y.logger.Infof("Yggstack started successfully")
	return nil
}

// Stop stops the Yggstack node
func (y *Yggstack) Stop() error {
	y.mu.Lock()

	if !y.isRunning {
		y.mu.Unlock()
		return fmt.Errorf("Yggstack is not running")
	}

	if y.cancel != nil {
		y.cancel()
	}

	// Close SOCKS5 listener with deadline to prevent hanging
	if y.socks5Tcp != nil {
		// Set deadline before closing to unblock any Accept() calls
		if tcpListener, ok := y.socks5Tcp.(*net.TCPListener); ok {
			tcpListener.SetDeadline(time.Now())
		}
		y.socks5Tcp.Close()
		y.socks5Tcp = nil
		y.logger.Infof("SOCKS5 listener closed")
	}

	// Close all active listeners first to stop accepting new connections
	y.closeAllListeners()

	// Close all active proxy connections to unblock handlers
	// Run with timeout to prevent hanging on stuck connections
	closeConnsDone := make(chan struct{})
	go func() {
		y.closeAllConnections()
		close(closeConnsDone)
	}()

	select {
	case <-closeConnsDone:
		y.logger.Infof("Connections closed successfully")
	case <-time.After(2 * time.Second):
		y.logger.Warnf("WARNING: Timeout closing connections after 2 seconds - forcing continuation")
	}

	// Release lock before waiting for handlers
	y.mu.Unlock()

	// Wait for all handler goroutines to finish with timeout
	y.logger.Infof("Waiting for handlers to stop...")
	done := make(chan struct{})
	go func() {
		y.handlersWg.Wait()
		close(done)
	}()

	select {
	case <-done:
		y.logger.Infof("All handlers stopped")
	case <-time.After(3 * time.Second):
		y.logger.Warnf("Timeout waiting for handlers to stop, forcing shutdown")
	}

	// Reacquire lock for final cleanup
	y.mu.Lock()
	defer y.mu.Unlock()

	if y.admin != nil {
		y.admin.Stop()
		y.admin = nil
	}

	if y.multicast != nil {
		y.multicast.Stop()
		y.multicast = nil
	}

	if y.netstack != nil {
		// Netstack uses gvisor stack which doesn't need explicit cleanup
		// It will be garbage collected once no references remain
		y.netstack = nil
	}

	if y.core != nil {
		// core.Stop() can block for several seconds closing broken peer connections.
		// Run it in a goroutine with a hard timeout so a stuck core cannot prevent
		// the Android service from restarting cleanly after a network switch.
		coreStopped := make(chan struct{})
		core := y.core
		y.core = nil
		go func() {
			core.Stop()
			close(coreStopped)
		}()
		select {
		case <-coreStopped:
			y.logger.Infof("core stopped")
		case <-time.After(4 * time.Second):
			y.logger.Warnf("core.Stop() timed out after 4s - forcing continuation")
		}
	}

	y.socks5Server = nil

	// Clear per-mapping tracking maps
	y.mappingCancels.Range(func(k, v any) bool {
		y.mappingCancels.Delete(k)
		return true
	})
	y.mappingListeners.Range(func(k, v any) bool {
		y.mappingListeners.Delete(k)
		return true
	})

	y.isRunning = false
	y.logger.Infof("Yggstack stopped")
	return nil
}

// trackConnection adds a connection to the active connections list
func (y *Yggstack) trackConnection(conn net.Conn) {
	y.activeConnsMu.Lock()
	defer y.activeConnsMu.Unlock()
	y.activeConns = append(y.activeConns, conn)
}

func (y *Yggstack) trackListener(listener io.Closer) {
	y.activeListenersMu.Lock()
	defer y.activeListenersMu.Unlock()
	y.activeListeners = append(y.activeListeners, listener)
}

// closeAllListeners forcefully closes all tracked listeners
func (y *Yggstack) closeAllListeners() {
	y.activeListenersMu.Lock()
	listeners := y.activeListeners
	y.activeListeners = nil
	y.activeListenersMu.Unlock()

	// Set deadlines on listeners that support it before closing
	deadline := time.Now()
	for _, listener := range listeners {
		// Try to set deadline if the listener type supports it
		if tcpListener, ok := listener.(*net.TCPListener); ok {
			tcpListener.SetDeadline(deadline)
		} else if udpConn, ok := listener.(*net.UDPConn); ok {
			udpConn.SetDeadline(deadline)
		}
	}

	// Now close all listeners
	for _, listener := range listeners {
		listener.Close()
	}
	y.logger.Infof("Closed %d active listeners", len(listeners))
}

// closeAllConnections forcefully closes all tracked connections
func (y *Yggstack) closeAllConnections() {
	y.activeConnsMu.Lock()
	conns := y.activeConns
	y.activeConns = nil
	y.activeConnsMu.Unlock()

	// Set aggressive deadlines on all connections BEFORE closing
	// This forces any blocked Read()/Write() operations to timeout immediately
	deadline := time.Now().Add(100 * time.Millisecond)
	for _, conn := range conns {
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			tcpConn.SetDeadline(deadline)
		} else if udpConn, ok := conn.(*net.UDPConn); ok {
			udpConn.SetDeadline(deadline)
		} else {
			// For other connection types, try setting deadline via generic interface
			conn.SetDeadline(deadline)
		}
	}

	// Small delay to allow deadlines to trigger
	time.Sleep(150 * time.Millisecond)

	// Now close all connections - they should close quickly
	for _, conn := range conns {
		conn.Close()
	}
	y.logger.Infof("Closed %d active connections", len(conns))
}

// IsRunning returns whether Yggstack is currently running
func (y *Yggstack) IsRunning() bool {
	y.mu.RLock()
	defer y.mu.RUnlock()
	return y.isRunning
}

// AddLocalTCPMapping adds a TCP mapping from local address to remote Yggdrasil address
// Format: localAddr="127.0.0.1:8080", remoteAddr="[200:1234::1]:8080"
func (y *Yggstack) AddLocalTCPMapping(localAddr, remoteAddr string) error {
	y.mu.Lock()
	defer y.mu.Unlock()

	localTCPAddr, err := net.ResolveTCPAddr("tcp", localAddr)
	if err != nil {
		return fmt.Errorf("invalid local TCP address %s: %w", localAddr, err)
	}

	remoteTCPAddr, err := net.ResolveTCPAddr("tcp", remoteAddr)
	if err != nil {
		return fmt.Errorf("invalid remote TCP address %s: %w", remoteAddr, err)
	}

	mapping := types.TCPMapping{
		Listen: localTCPAddr,
		Mapped: remoteTCPAddr,
	}

	y.localTCPMappings = append(y.localTCPMappings, mapping)

	// If already running, start the mapping handler
	if y.isRunning {
		key := localTCPMappingKey(localTCPAddr.String(), remoteTCPAddr.String())
		childCtx, cancel := context.WithCancel(y.ctx)
		y.mappingCancels.Store(key, cancel)
		y.handlersWg.Add(1)
		go y.handleLocalTCPMappingCtx(childCtx, key, mapping)
	}

	y.logger.Infof("Added local TCP mapping: %s -> %s", localAddr, remoteAddr)
	return nil
}

// AddLocalUDPMapping adds a UDP mapping from local address to remote Yggdrasil address
// Format: localAddr="127.0.0.1:5353", remoteAddr="[200:1234::1]:53"
func (y *Yggstack) AddLocalUDPMapping(localAddr, remoteAddr string) error {
	y.mu.Lock()
	defer y.mu.Unlock()

	localUDPAddr, err := net.ResolveUDPAddr("udp", localAddr)
	if err != nil {
		return fmt.Errorf("invalid local UDP address %s: %w", localAddr, err)
	}

	remoteUDPAddr, err := net.ResolveUDPAddr("udp", remoteAddr)
	if err != nil {
		return fmt.Errorf("invalid remote UDP address %s: %w", remoteAddr, err)
	}

	mapping := types.UDPMapping{
		Listen: localUDPAddr,
		Mapped: remoteUDPAddr,
	}

	y.localUDPMappings = append(y.localUDPMappings, mapping)

	// If already running, start the mapping handler
	if y.isRunning {
		key := localUDPMappingKey(localUDPAddr.String(), remoteUDPAddr.String())
		childCtx, cancel := context.WithCancel(y.ctx)
		y.mappingCancels.Store(key, cancel)
		y.handlersWg.Add(1)
		go y.handleLocalUDPMappingCtx(childCtx, key, mapping)
	}

	y.logger.Infof("Added local UDP mapping: %s -> %s", localAddr, remoteAddr)
	return nil
}

// AddRemoteTCPMapping adds a TCP mapping to expose local port on Yggdrasil network
// Format: remotePort=8080, localAddr="127.0.0.1:80"
func (y *Yggstack) AddRemoteTCPMapping(remotePort int, localAddr string) error {
	y.mu.Lock()
	defer y.mu.Unlock()

	if y.config == nil {
		return fmt.Errorf("config not loaded")
	}

	localTCPAddr, err := net.ResolveTCPAddr("tcp", localAddr)
	if err != nil {
		return fmt.Errorf("invalid local TCP address %s: %w", localAddr, err)
	}

	// Get our Yggdrasil address
	privateKey := ed25519.PrivateKey(y.config.PrivateKey)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	addr := address.AddrForKey(publicKey)
	ip := net.IP(addr[:])

	remoteTCPAddr := &net.TCPAddr{
		IP:   ip,
		Port: remotePort,
	}

	mapping := types.TCPMapping{
		Listen: remoteTCPAddr,
		Mapped: localTCPAddr,
	}

	y.remoteTCPMappings = append(y.remoteTCPMappings, mapping)

	// If already running, start the mapping handler
	if y.isRunning {
		key := remoteTCPMappingKey(mapping.Listen.Port, mapping.Mapped.String())
		childCtx, cancel := context.WithCancel(y.ctx)
		y.mappingCancels.Store(key, cancel)
		y.handlersWg.Add(1)
		go y.handleRemoteTCPMappingCtx(childCtx, key, mapping)
	}

	y.logger.Infof("Added remote TCP mapping: [%s]:%d -> %s", ip, remotePort, localAddr)
	return nil
}

// AddRemoteUDPMapping adds a UDP mapping to expose local port on Yggdrasil network
// Format: remotePort=53, localAddr="127.0.0.1:53"
func (y *Yggstack) AddRemoteUDPMapping(remotePort int, localAddr string) error {
	y.mu.Lock()
	defer y.mu.Unlock()

	if y.config == nil {
		return fmt.Errorf("config not loaded")
	}

	localUDPAddr, err := net.ResolveUDPAddr("udp", localAddr)
	if err != nil {
		return fmt.Errorf("invalid local UDP address %s: %w", localAddr, err)
	}

	// Get our Yggdrasil address
	privateKey := ed25519.PrivateKey(y.config.PrivateKey)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	addr := address.AddrForKey(publicKey)
	ip := net.IP(addr[:])

	remoteUDPAddr := &net.UDPAddr{
		IP:   ip,
		Port: remotePort,
	}

	mapping := types.UDPMapping{
		Listen: remoteUDPAddr,
		Mapped: localUDPAddr,
	}

	y.remoteUDPMappings = append(y.remoteUDPMappings, mapping)

	// If already running, start the mapping handler
	if y.isRunning {
		key := remoteUDPMappingKey(mapping.Listen.Port, mapping.Mapped.String())
		childCtx, cancel := context.WithCancel(y.ctx)
		y.mappingCancels.Store(key, cancel)
		y.handlersWg.Add(1)
		go y.handleRemoteUDPMappingCtx(childCtx, key, mapping)
	}

	y.logger.Infof("Added remote UDP mapping: [%s]:%d -> %s", ip, remotePort, localAddr)
	return nil
}

// ClearLocalMappings clears all local (forward) port mappings
func (y *Yggstack) ClearLocalMappings() error {
	y.mu.Lock()
	defer y.mu.Unlock()

	y.localTCPMappings = nil
	y.localUDPMappings = nil

	y.logger.Infof("Cleared all local port mappings")
	return nil
}

// ClearRemoteMappings clears all remote (expose) port mappings
func (y *Yggstack) ClearRemoteMappings() error {
	y.mu.Lock()
	defer y.mu.Unlock()

	y.remoteTCPMappings = nil
	y.remoteUDPMappings = nil

	y.logger.Infof("Cleared all remote port mappings")
	return nil
}

// RemoveLocalTCPMapping stops and removes a specific local TCP forward mapping.
// Cancels the handler goroutine and closes its listener immediately.
func (y *Yggstack) RemoveLocalTCPMapping(localAddr, remoteAddr string) error {
	y.mu.Lock()
	defer y.mu.Unlock()

	localTCPAddr, err := net.ResolveTCPAddr("tcp", localAddr)
	if err != nil {
		return fmt.Errorf("invalid local TCP address %s: %w", localAddr, err)
	}
	remoteTCPAddr, err := net.ResolveTCPAddr("tcp", remoteAddr)
	if err != nil {
		return fmt.Errorf("invalid remote TCP address %s: %w", remoteAddr, err)
	}

	key := localTCPMappingKey(localTCPAddr.String(), remoteTCPAddr.String())
	if v, ok := y.mappingCancels.LoadAndDelete(key); ok {
		v.(context.CancelFunc)()
	}
	if v, ok := y.mappingListeners.LoadAndDelete(key); ok {
		v.(io.Closer).Close()
	}

	newMappings := make([]types.TCPMapping, 0, len(y.localTCPMappings))
	for _, m := range y.localTCPMappings {
		if m.Listen.String() != localTCPAddr.String() || m.Mapped.String() != remoteTCPAddr.String() {
			newMappings = append(newMappings, m)
		}
	}
	y.localTCPMappings = newMappings

	y.logger.Infof("Removed local TCP mapping: %s -> %s", localAddr, remoteAddr)
	return nil
}

// RemoveLocalUDPMapping stops and removes a specific local UDP forward mapping.
func (y *Yggstack) RemoveLocalUDPMapping(localAddr, remoteAddr string) error {
	y.mu.Lock()
	defer y.mu.Unlock()

	localUDPAddr, err := net.ResolveUDPAddr("udp", localAddr)
	if err != nil {
		return fmt.Errorf("invalid local UDP address %s: %w", localAddr, err)
	}
	remoteUDPAddr, err := net.ResolveUDPAddr("udp", remoteAddr)
	if err != nil {
		return fmt.Errorf("invalid remote UDP address %s: %w", remoteAddr, err)
	}

	key := localUDPMappingKey(localUDPAddr.String(), remoteUDPAddr.String())
	if v, ok := y.mappingCancels.LoadAndDelete(key); ok {
		v.(context.CancelFunc)()
	}
	if v, ok := y.mappingListeners.LoadAndDelete(key); ok {
		v.(io.Closer).Close()
	}

	newMappings := make([]types.UDPMapping, 0, len(y.localUDPMappings))
	for _, m := range y.localUDPMappings {
		if m.Listen.String() != localUDPAddr.String() || m.Mapped.String() != remoteUDPAddr.String() {
			newMappings = append(newMappings, m)
		}
	}
	y.localUDPMappings = newMappings

	y.logger.Infof("Removed local UDP mapping: %s -> %s", localAddr, remoteAddr)
	return nil
}

// RemoveRemoteTCPMapping stops and removes a specific remote TCP expose mapping.
func (y *Yggstack) RemoveRemoteTCPMapping(remotePort int, localAddr string) error {
	y.mu.Lock()
	defer y.mu.Unlock()

	localTCPAddr, err := net.ResolveTCPAddr("tcp", localAddr)
	if err != nil {
		return fmt.Errorf("invalid local TCP address %s: %w", localAddr, err)
	}

	key := remoteTCPMappingKey(remotePort, localTCPAddr.String())
	if v, ok := y.mappingCancels.LoadAndDelete(key); ok {
		v.(context.CancelFunc)()
	}
	if v, ok := y.mappingListeners.LoadAndDelete(key); ok {
		v.(io.Closer).Close()
	}

	newMappings := make([]types.TCPMapping, 0, len(y.remoteTCPMappings))
	for _, m := range y.remoteTCPMappings {
		if m.Listen.Port != remotePort || m.Mapped.String() != localTCPAddr.String() {
			newMappings = append(newMappings, m)
		}
	}
	y.remoteTCPMappings = newMappings

	y.logger.Infof("Removed remote TCP mapping: port %d -> %s", remotePort, localAddr)
	return nil
}

// RemoveRemoteUDPMapping stops and removes a specific remote UDP expose mapping.
func (y *Yggstack) RemoveRemoteUDPMapping(remotePort int, localAddr string) error {
	y.mu.Lock()
	defer y.mu.Unlock()

	localUDPAddr, err := net.ResolveUDPAddr("udp", localAddr)
	if err != nil {
		return fmt.Errorf("invalid local UDP address %s: %w", localAddr, err)
	}

	key := remoteUDPMappingKey(remotePort, localUDPAddr.String())
	if v, ok := y.mappingCancels.LoadAndDelete(key); ok {
		v.(context.CancelFunc)()
	}
	if v, ok := y.mappingListeners.LoadAndDelete(key); ok {
		v.(io.Closer).Close()
	}

	newMappings := make([]types.UDPMapping, 0, len(y.remoteUDPMappings))
	for _, m := range y.remoteUDPMappings {
		if m.Listen.Port != remotePort || m.Mapped.String() != localUDPAddr.String() {
			newMappings = append(newMappings, m)
		}
	}
	y.remoteUDPMappings = newMappings

	y.logger.Infof("Removed remote UDP mapping: port %d -> %s", remotePort, localAddr)
	return nil
}

// Helper functions for port mapping handlers
func (y *Yggstack) handleLocalTCPMappingCtx(ctx context.Context, key string, mapping types.TCPMapping) {
	defer y.handlersWg.Done()
	defer y.mappingListeners.Delete(key)
	defer y.mappingCancels.Delete(key)

	select {
	case <-ctx.Done():
		return
	default:
	}

	listener, err := net.ListenTCP("tcp", mapping.Listen)
	if err != nil {
		y.logger.Errorf("Failed to listen on local TCP %s: %s", mapping.Listen, err)
		return
	}
	defer listener.Close()

	y.trackListener(listener)
	y.mappingListeners.Store(key, listener)

	y.logger.Infof("Mapping local TCP port %d to Yggdrasil %s", mapping.Listen.Port, mapping.Mapped)

	for {
		listener.SetDeadline(time.Now().Add(100 * time.Millisecond))

		select {
		case <-ctx.Done():
			y.logger.Infof("Stopping TCP mapping handler for port %d", mapping.Listen.Port)
			return
		default:
			c, err := listener.Accept()
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				select {
				case <-ctx.Done():
					return
				default:
					continue
				}
			}

			r, err := y.netstack.DialTCP(mapping.Mapped)
			if err != nil {
				y.logger.Errorf("Failed to connect to %s: %s", mapping.Mapped, err)
				c.Close()
				continue
			}

			y.trackConnection(c)
			y.trackConnection(r)

			go types.ProxyTCP(y.core.MTU(), c, r)
		}
	}
}

func (y *Yggstack) handleLocalUDPMappingCtx(ctx context.Context, key string, mapping types.UDPMapping) {
	defer y.handlersWg.Done()
	defer y.mappingListeners.Delete(key)
	defer y.mappingCancels.Delete(key)

	select {
	case <-ctx.Done():
		return
	default:
	}

	mtu := y.core.MTU()
	udpListenConn, err := net.ListenUDP("udp", mapping.Listen)
	if err != nil {
		y.logger.Errorf("Failed to listen on local UDP %s: %s", mapping.Listen, err)
		return
	}
	defer udpListenConn.Close()

	y.trackListener(udpListenConn)
	y.mappingListeners.Store(key, udpListenConn)

	y.logger.Infof("Mapping local UDP port %d to Yggdrasil %s", mapping.Listen.Port, mapping.Mapped)

	localUdpConnections := new(sync.Map)
	udpBuffer := make([]byte, mtu)

	for {
		udpListenConn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

		select {
		case <-ctx.Done():
			y.logger.Infof("Stopping UDP mapping handler for port %d", mapping.Listen.Port)
			return
		default:
			bytesRead, remoteUdpAddr, err := udpListenConn.ReadFrom(udpBuffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				select {
				case <-ctx.Done():
					return
				default:
					continue
				}
			}

			connKey := remoteUdpAddr.String()
			var yggdrasilConn interface{}

			if conn, ok := localUdpConnections.Load(connKey); ok {
				yggdrasilConn = conn
			} else {
				yggdrasilConn, err = y.netstack.DialUDP(mapping.Mapped)
				if err != nil {
					y.logger.Errorf("Failed to dial UDP %s: %s", mapping.Mapped, err)
					continue
				}
				localUdpConnections.Store(connKey, yggdrasilConn)

				y.trackConnection(yggdrasilConn.(net.Conn))

				go types.ReverseProxyUDP(mtu, udpListenConn, remoteUdpAddr, yggdrasilConn.(net.Conn))
			}

			if _, err := yggdrasilConn.(net.Conn).Write(udpBuffer[:bytesRead]); err != nil {
				y.logger.Errorf("Failed to write to UDP connection: %s", err)
			}
		}
	}
}

func (y *Yggstack) handleRemoteTCPMappingCtx(ctx context.Context, key string, mapping types.TCPMapping) {
	defer y.handlersWg.Done()
	defer y.mappingListeners.Delete(key)
	defer y.mappingCancels.Delete(key)

	select {
	case <-ctx.Done():
		return
	default:
	}

	listener, err := y.netstack.ListenTCP(mapping.Listen)
	if err != nil {
		y.logger.Errorf("Failed to listen on remote TCP %s: %s", mapping.Listen, err)
		return
	}
	defer listener.Close()

	y.trackListener(listener)
	y.mappingListeners.Store(key, listener)

	y.logger.Infof("Exposing local TCP %s on Yggdrasil port %d", mapping.Mapped, mapping.Listen.Port)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			c, err := listener.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					continue
				}
			}

			r, err := net.DialTCP("tcp", nil, mapping.Mapped)
			if err != nil {
				y.logger.Errorf("Failed to connect to local %s: %s", mapping.Mapped, err)
				c.Close()
				continue
			}

			y.trackConnection(c)
			y.trackConnection(r)

			go types.ProxyTCP(y.core.MTU(), c, r)
		}
	}
}

func (y *Yggstack) handleRemoteUDPMappingCtx(ctx context.Context, key string, mapping types.UDPMapping) {
	defer y.handlersWg.Done()
	defer y.mappingListeners.Delete(key)
	defer y.mappingCancels.Delete(key)

	select {
	case <-ctx.Done():
		return
	default:
	}

	mtu := y.core.MTU()
	udpListenConn, err := y.netstack.ListenUDP(mapping.Listen)
	if err != nil {
		y.logger.Errorf("Failed to listen on remote UDP %s: %s", mapping.Listen, err)
		return
	}
	defer udpListenConn.Close()

	y.trackListener(udpListenConn)
	y.mappingListeners.Store(key, udpListenConn)

	y.logger.Infof("Exposing local UDP %s on Yggdrasil port %d", mapping.Mapped, mapping.Listen.Port)

	localUdpConnections := new(sync.Map)
	udpBuffer := make([]byte, mtu)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			bytesRead, remoteUdpAddr, err := udpListenConn.ReadFrom(udpBuffer)
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					continue
				}
			}

			connKey := remoteUdpAddr.String()
			var localConn *net.UDPConn

			if conn, ok := localUdpConnections.Load(connKey); ok {
				localConn = conn.(*net.UDPConn)
			} else {
				localConn, err = net.DialUDP("udp", nil, mapping.Mapped)
				if err != nil {
					y.logger.Errorf("Failed to dial local UDP %s: %s", mapping.Mapped, err)
					continue
				}
				localUdpConnections.Store(connKey, localConn)

				y.trackConnection(localConn)

				go types.ReverseProxyUDP(mtu, udpListenConn, remoteUdpAddr, localConn)
			}

			if _, err := localConn.Write(udpBuffer[:bytesRead]); err != nil {
				y.logger.Errorf("Failed to write to local UDP connection: %s", err)
			}
		}
	}
}
