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

	"github.com/gologme/log"
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

	logger := cfg.Logger
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	if cfg.LogLevel != "" {
		setLogLevel(cfg.LogLevel, logger)
	} else {
		setLogLevel("info", logger)
	}

	obj := &Obj{
		cancel:    cancel,
		socksAddr: cfg.SocksAddr,
		logger:    logger,
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
		if obj.Core, err = core.New(nodeCfg.Certificate, logger, options...); err != nil {
			return nil, fmt.Errorf("core.New: %w", err)
		}
		pk := hex.EncodeToString(obj.Core.PublicKey())
		subnet := obj.Core.Subnet()
		logger.Printf("Your public key is %s", pk)
		logger.Printf("Your IPv6 address is %s", obj.Core.Address().String())
		logger.Printf("Your IPv6 subnet is %s", subnet.String())
		logger.Printf("Your Yggstack resolver name is %s%s", pk, types.NameMappingSuffix)
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
		if obj.Admin, err = admin.New(obj.Core, logger, options...); err != nil {
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
		var err error
		if obj.Multicast, err = multicast.New(obj.Core, logger, options...); err != nil {
			return nil, fmt.Errorf("multicast.New: %w", err)
		}
		if obj.Admin != nil && obj.Multicast != nil {
			obj.Multicast.SetupAdminHandlers(obj.Admin)
		}
	}

	// Netstack
	{
		var err error
		if obj.Netstack, err = netstack.CreateYggdrasilNetstack(obj.Core); err != nil {
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
	obj.startLocalUDP(cfg.LocalUDP)
	obj.startRemoteTCP(cfg.RemoteTCP)
	obj.startRemoteUDP(cfg.RemoteUDP)

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
