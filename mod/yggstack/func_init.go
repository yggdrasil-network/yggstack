package yggstack

import (
	"encoding/hex"
	"fmt"
	"regexp"

	"github.com/yggdrasil-network/yggdrasil-go/src/admin"
	"github.com/yggdrasil-network/yggdrasil-go/src/config"
	"github.com/yggdrasil-network/yggdrasil-go/src/core"
	"github.com/yggdrasil-network/yggdrasil-go/src/multicast"
	"github.com/yggdrasil-network/yggstack/mod/mapping"
	"github.com/yggdrasil-network/yggstack/mod/peers"
	"github.com/yggdrasil-network/yggstack/src/netstack"
)

// // // // // // // // // //

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
		re, err := regexp.Compile(intf.Regex)
		if err != nil {
			return fmt.Errorf("invalid multicast interface regex %q: %w", intf.Regex, err)
		}
		options = append(options, multicast.MulticastInterface{
			Regex:    re,
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
		o.peerMonitor = peers.NewMonitor(o.Core, cfg.PeerChangeCallback, &o.connCounter, o.componentsCtx)
		o.componentsWg.Add(1)
		go func() {
			defer o.componentsWg.Done()
			o.peerMonitor.Run()
		}()
	}

	// Netstack
	ns, err := netstack.CreateYggdrasilNetstack(o.Core, log)
	if err != nil {
		return fmt.Errorf("netstack.CreateYggdrasilNetstack: %w", err)
	}
	o.netstackPtr.Store(ns)

	// SOCKS5
	if cfg.SocksAddr != "" {
		result, err := mapping.StartSocks(o.nodeMapping, mapping.SocksConfigObj{
			Addr:       cfg.SocksAddr,
			Nameserver: cfg.Nameserver,
			Verbose:    cfg.SocksVerbose,
		}, o.socksReadyCh)
		if err != nil {
			return fmt.Errorf("SOCKS5: %w", err)
		}
		o.socksListener = result.Listener
		o.socksIsUnix = result.IsUnix
		o.socksAddr = cfg.SocksAddr
	}

	// Port forwarding
	mapping.StartLocalTCP(o.nodeMapping, cfg.Mapping.LocalTCP)
	mapping.StartLocalUDP(o.nodeMapping, cfg.Mapping.LocalUDP, cfg.UDPSessionTimeout)
	mapping.StartRemoteTCP(o.nodeMapping, cfg.Mapping.RemoteTCP)
	mapping.StartRemoteUDP(o.nodeMapping, cfg.Mapping.RemoteUDP, cfg.UDPSessionTimeout)

	return nil
}
