package yggstack

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	golog "github.com/gologme/log"
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
		cancel:    cancel,
		socksAddr: cfg.SocksAddr,
		logger:    log,
	}

	// Cleanup on initialization error
	defer func() {
		if retErr != nil {
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

	// Multicast
	{
		options := []multicast.SetupOption{}
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
		// multicast.New requires *log.Logger — type assertion or discard
		mcastLog, ok := log.(*golog.Logger)
		if !ok {
			mcastLog = golog.New(io.Discard, "", 0)
		}
		var err error
		if obj.Multicast, err = multicast.New(obj.Core, mcastLog, options...); err != nil {
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
	obj.startLocalTCP(cfg.LocalTCP)
	obj.startLocalUDP(cfg.LocalUDP, cfg.UDPSessionTimeout)
	obj.startRemoteTCP(cfg.RemoteTCP)
	obj.startRemoteUDP(cfg.RemoteUDP, cfg.UDPSessionTimeout)

	// Shutdown on context cancellation
	go func() {
		<-ctx.Done()
		obj.Close()
	}()

	return obj, nil
}

// //

// Close gracefully shuts down the node
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

// //

// Address returns the node's IPv6 address
func (o *Obj) Address() net.IP {
	addr := o.Core.Address()
	return net.IP(addr[:])
}

// Subnet returns the node's IPv6 subnet
func (o *Obj) Subnet() net.IPNet {
	return o.Core.Subnet()
}

// PublicKey returns the node's public key
func (o *Obj) PublicKey() ed25519.PublicKey {
	return o.Core.PublicKey()
}
