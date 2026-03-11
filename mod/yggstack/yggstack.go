package yggstack

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
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

	obj := &Obj{
		ctx:       ctx,
		cancel:    cancel,
		socksAddr: cfg.SocksAddr,
		logger:    log,
	}

	// Rollback on initialization error
	defer func() {
		if retErr != nil {
			if obj.Netstack != nil {
				obj.Netstack.Close()
			}
			if obj.socksListener != nil {
				_ = obj.socksListener.Close()
			}
			if obj.Multicast != nil {
				_ = obj.Multicast.Stop()
			}
			if obj.Admin != nil {
				_ = obj.Admin.Stop()
			}
			if obj.Core != nil {
				obj.Core.Stop()
			}
			cancel()
		}
	}()

	// Core
	{
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
				return nil, fmt.Errorf("hex.DecodeString AllowedPublicKeys: %w", err)
			}
			options = append(options, core.AllowedPublicKey(k[:]))
		}
		var err error
		if obj.Core, err = core.New(nodeCfg.Certificate, log, options...); err != nil {
			return nil, fmt.Errorf("core.New: %w", err)
		}
		pk := hex.EncodeToString(obj.Core.PublicKey())
		subnet := obj.Core.Subnet()
		log.Printf("Your public key is %s", pk)
		log.Printf("Your IPv6 address is %s", obj.Core.Address().String())
		log.Printf("Your IPv6 subnet is %s", subnet.String())
		log.Printf("Your Yggstack resolver name is %s%s", pk, types.NameMappingSuffix)
	}

	// Admin socket
	{
		options := []admin.SetupOption{
			admin.ListenAddress(nodeCfg.AdminListen),
		}
		if nodeCfg.LogLookups {
			options = append(options, admin.LogLookups{})
		}
		var err error
		if obj.Admin, err = admin.New(obj.Core, log, options...); err != nil {
			return nil, fmt.Errorf("admin.New: %w", err)
		}
		if obj.Admin != nil {
			obj.Admin.SetupAdminHandlers()
		}
	}

	// Multicast (optional, requires MulticastLogger)
	if cfg.MulticastLogger != nil {
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
		if obj.Multicast, err = multicast.New(obj.Core, cfg.MulticastLogger, options...); err != nil {
			return nil, fmt.Errorf("multicast.New: %w", err)
		}
		if obj.Admin != nil && obj.Multicast != nil {
			obj.Multicast.SetupAdminHandlers(obj.Admin)
		}
	}

	// Netstack
	{
		var err error
		if obj.Netstack, err = netstack.CreateYggdrasilNetstack(obj.Core, log); err != nil {
			return nil, fmt.Errorf("netstack.CreateYggdrasilNetstack: %w", err)
		}
	}

	// SOCKS5
	if cfg.SocksAddr != "" {
		if err := obj.startSocks(cfg); err != nil {
			return nil, fmt.Errorf("SOCKS5: %w", err)
		}
	}

	// Port forwarding
	obj.startLocalTCP(ctx, cfg.LocalTCP)
	obj.startLocalUDP(ctx, cfg.LocalUDP, cfg.UDPSessionTimeout)
	obj.startRemoteTCP(ctx, cfg.RemoteTCP)
	obj.startRemoteUDP(ctx, cfg.RemoteUDP, cfg.UDPSessionTimeout)

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
		if o.socksListener != nil {
			_ = o.socksListener.Close()
			if o.socksAddr != "" && !strings.Contains(o.socksAddr, ":") {
				_ = os.RemoveAll(o.socksAddr)
				o.logger.Infof("Stopped SOCKS5 UNIX socket listener")
			} else {
				o.logger.Infof("Stopped SOCKS5 TCP listener")
			}
		}
		// Port forwarding listeners
		o.closersMu.Lock()
		for _, c := range o.closers {
			_ = c.Close()
		}
		o.closersMu.Unlock()
		if o.Netstack != nil {
			o.Netstack.Close()
		}
		if o.Multicast != nil {
			_ = o.Multicast.Stop()
		}
		if o.Admin != nil {
			_ = o.Admin.Stop()
		}
		o.Core.Stop()
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
	return o.Netstack.DialContext(ctx, network, address)
}

// DialTCP opens a TCP connection to the given Yggdrasil address.
func (o *Obj) DialTCP(addr *net.TCPAddr) (net.Conn, error) {
	return o.Netstack.DialTCP(addr)
}

// DialUDP opens a UDP connection to the given Yggdrasil address.
func (o *Obj) DialUDP(addr *net.UDPAddr) (net.Conn, error) {
	return o.Netstack.DialUDP(addr)
}

// ListenTCP listens for incoming TCP connections on the given Yggdrasil address.
// The addr.IP should be the node's own Yggdrasil IPv6 (from Address()).
func (o *Obj) ListenTCP(addr *net.TCPAddr) (net.Listener, error) {
	return o.Netstack.ListenTCP(addr)
}

// ListenUDP listens for incoming UDP packets on the given Yggdrasil address.
// The addr.IP should be the node's own Yggdrasil IPv6 (from Address()).
func (o *Obj) ListenUDP(addr *net.UDPAddr) (net.PacketConn, error) {
	return o.Netstack.ListenUDP(addr)
}
