package yggstack

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"regexp"
	"time"

	"github.com/yggdrasil-network/yggdrasil-go/src/admin"
	"github.com/yggdrasil-network/yggdrasil-go/src/config"
	"github.com/yggdrasil-network/yggdrasil-go/src/core"
	"github.com/yggdrasil-network/yggdrasil-go/src/multicast"

	"github.com/yggdrasil-network/yggstack/src/netstack"
	"github.com/yggdrasil-network/yggstack/src/types"
)

// // // // // // // // // //

func New(cfg ConfigObj) (_ *Obj, retErr error) {
	if cfg.Ctx == nil {
		cfg.Ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(cfg.Ctx)

	nodeCfg := cfg.Config
	if nodeCfg == nil {
		nodeCfg = config.GenerateConfig()
		nodeCfg.AdminListen = "none"
	}

	log := cfg.Logger
	if log == nil {
		log = noopLoggerObj{}
	}

	if cfg.UDPSessionTimeout == 0 {
		cfg.UDPSessionTimeout = 120 * time.Second
	}

	componentsCtx, componentsCancel := context.WithCancel(ctx)

	obj := &Obj{
		ctx:              ctx,
		cancel:           cancel,
		componentsCtx:    componentsCtx,
		componentsCancel: componentsCancel,
		socksAddr:        cfg.SocksAddr,
		coreStopTimeout:  cfg.CoreStopTimeout,
		logger:           log,
		activityCallback: cfg.ActivityCallback,
		nodeConfig:       nodeCfg,
	}

	defer func() {
		if retErr != nil {
			obj.rollbackComponents()
			componentsCancel()
			cancel()
		}
	}()

	if err := obj.initCore(nodeCfg, log); err != nil {
		return nil, err
	}

	pk := hex.EncodeToString(obj.Core.PublicKey())
	subnet := obj.Core.Subnet()
	log.Printf("Your public key is %s", pk)
	log.Printf("Your IPv6 address is %s", obj.Core.Address().String())
	log.Printf("Your IPv6 subnet is %s", subnet.String())
	log.Printf("Your Yggstack resolver name is %s%s", pk, types.NameMappingSuffix)

	if err := obj.initAdmin(nodeCfg, log); err != nil {
		return nil, err
	}

	if err := obj.initMulticast(cfg, nodeCfg); err != nil {
		return nil, err
	}

	if err := obj.initNetworking(cfg, log); err != nil {
		return nil, err
	}

	// Low Power Mode
	if cfg.LowPower != nil && cfg.ActivityCallback != nil {
		lpmCfg := *cfg.LowPower
		if lpmCfg.IdleTimeout == 0 {
			lpmCfg.IdleTimeout = 60 * time.Second
		}
		lpmCtx, lpmCancel := context.WithCancel(ctx)
		obj.lowPower = newLowPowerManager(obj, lpmCfg, lpmCtx, lpmCancel, log)
		obj.lowPower.setOrigConfig(cfg)
		go obj.lowPower.run()
	}

	// Shutdown on context cancellation
	go func() {
		<-ctx.Done()
		obj.Close()
	}()

	return obj, nil
}

// //

// Close gracefully shuts down the node in reverse initialization order:
// SOCKS5 listener, port forwarding, netstack, multicast, admin socket, core.
// Safe to call multiple times; only the first call performs cleanup.
// Always returns nil.
func (o *Obj) Close() error {
	o.closeOnce.Do(func() {
		o.cancel()
		if o.lowPower != nil {
			o.lowPower.stop()
		}
		o.stopComponents()
	})
	return nil
}

// stopComponents shuts down subsystems without finalizing Obj.
// Called from Close() and from LPM when entering sleep.
func (o *Obj) stopComponents() {
	o.componentsMu.Lock()
	defer o.componentsMu.Unlock()

	// Clear atomic pointer before closing netstack
	o.netstackPtr.Store(nil)

	// Signal all component goroutines to stop
	if o.componentsCancel != nil {
		o.componentsCancel()
	}

	if o.socksListener != nil {
		_ = o.socksListener.Close()
		if o.socksIsUnix {
			_ = os.RemoveAll(o.socksAddr)
			o.logger.Infof("Stopped SOCKS5 UNIX socket listener")
		} else {
			o.logger.Infof("Stopped SOCKS5 TCP listener")
		}
		o.socksListener = nil
	}
	// Port forwarding listeners
	o.closersMu.Lock()
	for _, c := range o.closers {
		_ = c.Close()
	}
	o.closers = nil
	o.closersMu.Unlock()

	// Wait for all component goroutines to finish
	o.componentsWg.Wait()

	// Peer monitor depends on Core, stop it before Core.Stop()
	if o.peerMonitor != nil {
		o.peerMonitor.cancel()
		o.peerMonitor = nil
	}
	if o.Netstack != nil {
		o.Netstack.Close()
		o.Netstack = nil
	}
	if o.Multicast != nil {
		_ = o.Multicast.Stop()
		o.Multicast = nil
	}
	if o.Admin != nil {
		_ = o.Admin.Stop()
		o.Admin = nil
	}
	o.stopCoreWithTimeout()
	o.componentsCancel = nil
	o.componentsCtx = nil
}

// startComponents recreates all subsystems using the stored nodeConfig.
// Called from LPM when waking up.
func (o *Obj) startComponents(cfg ConfigObj) (retErr error) {
	o.componentsMu.Lock()
	defer o.componentsMu.Unlock()

	// New generation context for component goroutines
	o.componentsCtx, o.componentsCancel = context.WithCancel(o.ctx)

	// Channel to signal SOCKS readiness on wake
	if cfg.SocksAddr != "" {
		o.socksReadyCh = make(chan struct{})
	}

	nodeCfg := o.nodeConfig
	log := o.logger

	defer func() {
		if retErr != nil {
			o.netstackPtr.Store(nil)
			o.rollbackComponents()
			o.componentsCancel()
			o.componentsWg.Wait()
			o.componentsCancel = nil
			o.componentsCtx = nil
		}
	}()

	if err := o.initCore(nodeCfg, log); err != nil {
		return err
	}
	if err := o.initAdmin(nodeCfg, log); err != nil {
		return err
	}
	if err := o.initMulticast(cfg, nodeCfg); err != nil {
		return err
	}
	if err := o.initNetworking(cfg, log); err != nil {
		return err
	}

	return nil
}

// //

// stopCoreWithTimeout stops Core with a time limit.
// When timeout == 0 — waits indefinitely (backward-compatible).
func (o *Obj) stopCoreWithTimeout() {
	if o.Core == nil {
		return
	}
	if o.coreStopTimeout == 0 {
		o.Core.Stop()
		o.Core = nil
		return
	}
	done := make(chan struct{})
	go func() {
		o.Core.Stop()
		close(done)
	}()
	select {
	case <-done:
		// Graceful shutdown completed
	case <-time.After(o.coreStopTimeout):
		o.logger.Warnf("core.Stop() timed out after %s, forcing shutdown", o.coreStopTimeout)
	}
	o.Core = nil
}

// rollbackComponents stops partially initialized subsystems.
func (o *Obj) rollbackComponents() {
	if o.peerMonitor != nil {
		o.peerMonitor.cancel()
		o.peerMonitor = nil
	}
	if o.socksListener != nil {
		_ = o.socksListener.Close()
		o.socksListener = nil
	}
	if o.Netstack != nil {
		o.Netstack.Close()
		o.Netstack = nil
	}
	if o.Multicast != nil {
		_ = o.Multicast.Stop()
		o.Multicast = nil
	}
	if o.Admin != nil {
		_ = o.Admin.Stop()
		o.Admin = nil
	}
	o.stopCoreWithTimeout()
}

func (o *Obj) initCore(nodeCfg *config.NodeConfig, log core.Logger) error {
	options := []core.SetupOption{
		core.NodeInfo(nodeCfg.NodeInfo),
		core.NodeInfoPrivacy(nodeCfg.NodeInfoPrivacy),
	}
	for _, addr := range nodeCfg.Listen {
		options = append(options, core.ListenAddress(addr))
	}
	for _, peer := range nodeCfg.Peers {
		options = append(options, core.Peer{URI: peer})
	}
	for intf, peers := range nodeCfg.InterfacePeers {
		for _, peer := range peers {
			options = append(options, core.Peer{URI: peer, SourceInterface: intf})
		}
	}
	for _, allowed := range nodeCfg.AllowedPublicKeys {
		k, err := hex.DecodeString(allowed)
		if err != nil {
			return fmt.Errorf("hex.DecodeString AllowedPublicKeys: %w", err)
		}
		options = append(options, core.AllowedPublicKey(k[:]))
	}
	var err error
	if o.Core, err = core.New(nodeCfg.Certificate, log, options...); err != nil {
		return fmt.Errorf("core.New: %w", err)
	}
	return nil
}

func (o *Obj) initAdmin(nodeCfg *config.NodeConfig, log core.Logger) error {
	options := []admin.SetupOption{
		admin.ListenAddress(nodeCfg.AdminListen),
	}
	if nodeCfg.LogLookups {
		options = append(options, admin.LogLookups{})
	}
	var err error
	if o.Admin, err = admin.New(o.Core, log, options...); err != nil {
		return fmt.Errorf("admin.New: %w", err)
	}
	if o.Admin != nil {
		o.Admin.SetupAdminHandlers()
	}
	return nil
}

func (o *Obj) initMulticast(cfg ConfigObj, nodeCfg *config.NodeConfig) error {
	if cfg.MulticastLogger == nil {
		return nil
	}
	var options []multicast.SetupOption
	for _, intf := range nodeCfg.MulticastInterfaces {
		options = append(options, multicast.MulticastInterface{
			Regex:    regexp.MustCompile(intf.Regex),
			Beacon:   intf.Beacon,
			Listen:   intf.Listen,
			Port:     intf.Port,
			Priority: uint8(intf.Priority),
			Password: intf.Password,
		})
	}
	var err error
	if o.Multicast, err = multicast.New(o.Core, cfg.MulticastLogger, options...); err != nil {
		return fmt.Errorf("multicast.New: %w", err)
	}
	if o.Admin != nil && o.Multicast != nil {
		o.Multicast.SetupAdminHandlers(o.Admin)
	}
	return nil
}

func (o *Obj) initNetworking(cfg ConfigObj, log core.Logger) error {
	// Peer monitor
	if cfg.PeerChangeCallback != nil {
		pCtx, pCancel := context.WithCancel(o.componentsCtx)
		o.peerMonitor = &peerMonitorObj{
			core:        o.Core,
			callback:    cfg.PeerChangeCallback,
			connCounter: &o.connCounter,
			ctx:         pCtx,
			cancel:      pCancel,
		}
		o.componentsWg.Add(1)
		go func() {
			defer o.componentsWg.Done()
			o.peerMonitor.run()
		}()
	}

	// Netstack
	var err error
	if o.Netstack, err = netstack.CreateYggdrasilNetstack(o.Core, log); err != nil {
		return fmt.Errorf("netstack.CreateYggdrasilNetstack: %w", err)
	}
	o.netstackPtr.Store(o.Netstack)

	// SOCKS5
	if cfg.SocksAddr != "" {
		if err := o.startSocks(cfg); err != nil {
			return fmt.Errorf("SOCKS5: %w", err)
		}
	}

	// Port forwarding
	o.startLocalTCP(cfg.Mapping.LocalTCP)
	o.startLocalUDP(cfg.Mapping.LocalUDP, cfg.UDPSessionTimeout)
	o.startRemoteTCP(cfg.Mapping.RemoteTCP)
	o.startRemoteUDP(cfg.Mapping.RemoteUDP, cfg.UDPSessionTimeout)

	return nil
}

// //

// Address returns the node's Yggdrasil IPv6 address (200::/7 range).
func (o *Obj) Address() net.IP {
	addr := o.Core.Address()
	return net.IP(addr[:])
}

// Subnet returns the node's Yggdrasil /64 routed subnet (300::/7 range).
func (o *Obj) Subnet() net.IPNet {
	return o.Core.Subnet()
}

// PublicKey returns the node's ed25519 public key (32 bytes).
func (o *Obj) PublicKey() ed25519.PublicKey {
	return o.Core.PublicKey()
}

// DialContext opens a connection to a Yggdrasil address.
// Supported networks: "tcp", "tcp6", "udp", "udp6".
// Address format: "[ipv6]:port" or "host:port".
// Compatible with http.Transport.DialContext for use as an HTTP client transport.
func (o *Obj) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if o.lowPower != nil {
		ns := o.netstackPtr.Load()
		if ns == nil {
			return nil, fmt.Errorf("node is in low power mode")
		}
		return ns.DialContext(ctx, network, address)
	}
	return o.Netstack.DialContext(ctx, network, address)
}

// DialTCP opens a TCP connection to the given Yggdrasil address.
func (o *Obj) DialTCP(addr *net.TCPAddr) (net.Conn, error) {
	if o.lowPower != nil {
		ns := o.netstackPtr.Load()
		if ns == nil {
			return nil, fmt.Errorf("node is in low power mode")
		}
		return ns.DialTCP(addr)
	}
	return o.Netstack.DialTCP(addr)
}

// DialUDP opens a UDP connection to the given Yggdrasil address.
func (o *Obj) DialUDP(addr *net.UDPAddr) (net.Conn, error) {
	if o.lowPower != nil {
		ns := o.netstackPtr.Load()
		if ns == nil {
			return nil, fmt.Errorf("node is in low power mode")
		}
		return ns.DialUDP(addr)
	}
	return o.Netstack.DialUDP(addr)
}

// ListenTCP listens for incoming TCP connections on the given Yggdrasil address.
// The addr.IP should be the node's own Yggdrasil IPv6 (from Address()).
func (o *Obj) ListenTCP(addr *net.TCPAddr) (net.Listener, error) {
	if o.lowPower != nil {
		ns := o.netstackPtr.Load()
		if ns == nil {
			return nil, fmt.Errorf("node is in low power mode")
		}
		return ns.ListenTCP(addr)
	}
	return o.Netstack.ListenTCP(addr)
}

// ListenUDP listens for incoming UDP packets on the given Yggdrasil address.
// The addr.IP should be the node's own Yggdrasil IPv6 (from Address()).
func (o *Obj) ListenUDP(addr *net.UDPAddr) (net.PacketConn, error) {
	if o.lowPower != nil {
		ns := o.netstackPtr.Load()
		if ns == nil {
			return nil, fmt.Errorf("node is in low power mode")
		}
		return ns.ListenUDP(addr)
	}
	return o.Netstack.ListenUDP(addr)
}
