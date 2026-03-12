package yggstack

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"net"
	"time"

	"github.com/yggdrasil-network/yggdrasil-go/src/config"
	"github.com/yggdrasil-network/yggstack/mod/lowpower"
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

	// Validate: LowPower requires ActivityCallback for idle detection
	if cfg.LowPower != nil && cfg.ActivityCallback == nil {
		log.Warnf("LowPower is enabled but ActivityCallback is nil — idle detection will not work correctly")
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

	// Resolve interface adapters: use injected or built-in defaults
	if cfg.NodeMapping != nil {
		obj.nodeMapping = cfg.NodeMapping
	} else {
		obj.nodeMapping = &NodeMappingObj{node: obj}
	}
	if cfg.NodeControl != nil {
		obj.nodeControl = cfg.NodeControl
	} else {
		obj.nodeControl = &NodeControlObj{node: obj}
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
		obj.lowPower = lowpower.NewManager(obj.nodeControl, lpmCfg, lpmCtx, lpmCancel, log)
		cfgCopy := cfg
		cfgCopy.Ctx = nil
		obj.lowPower.SetOrigConfig(cfgCopy)
		go obj.lowPower.Run()
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
			o.lowPower.Stop()
		}
		o.stopComponents()
	})
	return nil
}

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
	ns := o.netstackPtr.Load()
	if ns == nil {
		return nil, fmt.Errorf("netstack is not available")
	}
	return ns.DialContext(ctx, network, address)
}

// DialTCP opens a TCP connection to the given Yggdrasil address.
func (o *Obj) DialTCP(addr *net.TCPAddr) (net.Conn, error) {
	ns := o.netstackPtr.Load()
	if ns == nil {
		return nil, fmt.Errorf("netstack is not available")
	}
	return ns.DialTCP(addr)
}

// DialUDP opens a UDP connection to the given Yggdrasil address.
func (o *Obj) DialUDP(addr *net.UDPAddr) (net.Conn, error) {
	ns := o.netstackPtr.Load()
	if ns == nil {
		return nil, fmt.Errorf("netstack is not available")
	}
	return ns.DialUDP(addr)
}

// ListenTCP listens for incoming TCP connections on the given Yggdrasil address.
// The addr.IP should be the node's own Yggdrasil IPv6 (from Address()).
// The returned listener is automatically closed on node shutdown.
func (o *Obj) ListenTCP(addr *net.TCPAddr) (net.Listener, error) {
	ns := o.netstackPtr.Load()
	if ns == nil {
		return nil, fmt.Errorf("netstack is not available")
	}
	l, err := ns.ListenTCP(addr)
	if err != nil {
		return nil, err
	}
	o.addCloser(l)
	return l, nil
}

// ListenUDP listens for incoming UDP packets on the given Yggdrasil address.
// The addr.IP should be the node's own Yggdrasil IPv6 (from Address()).
// The returned conn is automatically closed on node shutdown.
func (o *Obj) ListenUDP(addr *net.UDPAddr) (net.PacketConn, error) {
	ns := o.netstackPtr.Load()
	if ns == nil {
		return nil, fmt.Errorf("netstack is not available")
	}
	c, err := ns.ListenUDP(addr)
	if err != nil {
		return nil, err
	}
	o.addCloser(c)
	return c, nil
}
