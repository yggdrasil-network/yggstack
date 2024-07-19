package main

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"

	"github.com/gologme/log"
	gsyslog "github.com/hashicorp/go-syslog"
	"github.com/hjson/hjson-go/v4"
	"github.com/things-go/go-socks5"

	"github.com/yggdrasil-network/yggdrasil-go/src/address"
	"github.com/yggdrasil-network/yggdrasil-go/src/admin"
	"github.com/yggdrasil-network/yggdrasil-go/src/config"
	"github.com/yggdrasil-network/yggdrasil-go/src/core"
	"github.com/yggdrasil-network/yggdrasil-go/src/multicast"
	"github.com/yggdrasil-network/yggdrasil-go/src/version"

	"github.com/yggdrasil-network/yggstack/src/netstack"
	"github.com/yggdrasil-network/yggstack/src/types"

	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
)

type node struct {
	core           *core.Core
	multicast      *multicast.Multicast
	admin          *admin.AdminSocket
	socks5Listener net.Listener
}

type UDPSession struct {
	conn       interface{}
	remoteAddr net.Addr
}

// The main function is responsible for configuring and starting Yggdrasil.
func main() {
	var localtcp types.TCPLocalMappings
	var localudp types.UDPLocalMappings
	var remotetcp types.TCPRemoteMappings
	var remoteudp types.UDPRemoteMappings
	genconf := flag.Bool("genconf", false, "print a new config to stdout")
	useconf := flag.Bool("useconf", false, "read HJSON/JSON config from stdin")
	useconffile := flag.String("useconffile", "", "read HJSON/JSON config from specified file path")
	normaliseconf := flag.Bool("normaliseconf", false, "use in combination with either -useconf or -useconffile, outputs your configuration normalised")
	exportkey := flag.Bool("exportkey", false, "use in combination with either -useconf or -useconffile, outputs your private key in PEM format")
	confjson := flag.Bool("json", false, "print configuration from -genconf or -normaliseconf as JSON instead of HJSON")
	autoconf := flag.Bool("autoconf", false, "automatic mode (dynamic IP, peer with IPv6 neighbors)")
	ver := flag.Bool("version", false, "prints the version of this build")
	logto := flag.String("logto", "stdout", "file path to log to, \"syslog\" or \"stdout\"")
	getaddr := flag.Bool("address", false, "use in combination with either -useconf or -useconffile, outputs your IPv6 address")
	getsnet := flag.Bool("subnet", false, "use in combination with either -useconf or -useconffile, outputs your IPv6 subnet")
	getpkey := flag.Bool("publickey", false, "use in combination with either -useconf or -useconffile, outputs your public key")
	loglevel := flag.String("loglevel", "info", "loglevel to enable")
	socks := flag.String("socks", "", "address to listen on for SOCKS, i.e. :1080; or UNIX socket file path, i.e. /tmp/yggstack.sock")
	nameserver := flag.String("nameserver", "", "the Yggdrasil IPv6 address to use as a DNS server for SOCKS")
	flag.Var(&localtcp, "local-tcp", "TCP ports to forward to the remote Yggdradil node, e.g. 22:[a:b:c:d]:22, 127.0.0.1:22:[a:b:c:d]:22")
	flag.Var(&localudp, "local-udp", "UDP ports to forward to the remote Yggdrasil node, e.g. 22:[a:b:c:d]:2022, 127.0.0.1:[a:b:c:d]:22")
	flag.Var(&remotetcp, "remote-tcp", "TCP ports to expose to the network, e.g. 22, 2022:22, 22:192.168.1.1:2022")
	flag.Var(&remoteudp, "remote-udp", "UDP ports to expose to the network, e.g. 22, 2022:22, 22:192.168.1.1:2022")
	flag.Parse()

	// Catch interrupts from the operating system to exit gracefully.
	ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	// Create a new logger that logs output to stdout.
	var logger *log.Logger
	switch *logto {
	case "stdout":
		logger = log.New(os.Stdout, "", log.Flags())

	case "syslog":
		if syslogger, err := gsyslog.NewLogger(gsyslog.LOG_NOTICE, "DAEMON", version.BuildName()); err == nil {
			logger = log.New(syslogger, "", log.Flags()&^(log.Ldate|log.Ltime))
		}

	default:
		if logfd, err := os.OpenFile(*logto, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			logger = log.New(logfd, "", log.Flags())
		}
	}
	if logger == nil {
		logger = log.New(os.Stdout, "", log.Flags())
		logger.Warnln("Logging defaulting to stdout")
	}
	if *normaliseconf {
		setLogLevel("error", logger)
	} else {
		setLogLevel(*loglevel, logger)
	}

	cfg := config.GenerateConfig()
	var err error
	switch {
	case *ver:
		fmt.Println("Build name:", version.BuildName())
		fmt.Println("Build version:", version.BuildVersion())
		return

	case *autoconf:
		// Force AdminListen to none in yggstack
		cfg.AdminListen = "none"
		// Use an autoconf-generated config, this will give us random keys and
		// port numbers, and will use an automatically selected TUN interface.

	case *useconf:
		if _, err := cfg.ReadFrom(os.Stdin); err != nil {
			panic(err)
		}

	case *useconffile != "":
		f, err := os.Open(*useconffile)
		if err != nil {
			panic(err)
		}
		if _, err := cfg.ReadFrom(f); err != nil {
			panic(err)
		}
		_ = f.Close()

	case *genconf:
		// Force AdminListen to none in yggstack
		cfg.AdminListen = "none"
		var bs []byte
		if *confjson {
			bs, err = json.MarshalIndent(cfg, "", "  ")
		} else {
			bs, err = hjson.Marshal(cfg)
		}
		if err != nil {
			panic(err)
		}
		fmt.Println(string(bs))
		return

	default:
		fmt.Println("Usage:")
		flag.PrintDefaults()

		if *getaddr || *getsnet {
			fmt.Println("\nError: You need to specify some config data using -useconf or -useconffile.")
		}
		return
	}

	privateKey := ed25519.PrivateKey(cfg.PrivateKey)
	publicKey := privateKey.Public().(ed25519.PublicKey)

	switch {
	case *getaddr:
		addr := address.AddrForKey(publicKey)
		ip := net.IP(addr[:])
		fmt.Println(ip.String())
		return

	case *getsnet:
		snet := address.SubnetForKey(publicKey)
		ipnet := net.IPNet{
			IP:   append(snet[:], 0, 0, 0, 0, 0, 0, 0, 0),
			Mask: net.CIDRMask(len(snet)*8, 128),
		}
		fmt.Println(ipnet.String())
		return

	case *getpkey:
		fmt.Println(hex.EncodeToString(publicKey))
		return

	case *normaliseconf:
		cfg.AdminListen = "none"
		if cfg.PrivateKeyPath != "" {
			cfg.PrivateKey = nil
		}
		var bs []byte
		if *confjson {
			bs, err = json.MarshalIndent(cfg, "", "  ")
		} else {
			bs, err = hjson.Marshal(cfg)
		}
		if err != nil {
			panic(err)
		}
		fmt.Println(string(bs))
		return

	case *exportkey:
		pem, err := cfg.MarshalPEMPrivateKey()
		if err != nil {
			panic(err)
		}
		fmt.Println(string(pem))
		return
	}

	n := &node{}

	// Setup the Yggdrasil node itself.
	{
		options := []core.SetupOption{
			core.NodeInfo(cfg.NodeInfo),
			core.NodeInfoPrivacy(cfg.NodeInfoPrivacy),
		}
		for _, addr := range cfg.Listen {
			options = append(options, core.ListenAddress(addr))
		}
		for _, peer := range cfg.Peers {
			options = append(options, core.Peer{URI: peer})
		}
		for intf, peers := range cfg.InterfacePeers {
			for _, peer := range peers {
				options = append(options, core.Peer{URI: peer, SourceInterface: intf})
			}
		}
		for _, allowed := range cfg.AllowedPublicKeys {
			k, err := hex.DecodeString(allowed)
			if err != nil {
				panic(err)
			}
			options = append(options, core.AllowedPublicKey(k[:]))
		}
		if n.core, err = core.New(cfg.Certificate, logger, options...); err != nil {
			panic(err)
		}
		address, subnet := n.core.Address(), n.core.Subnet()
		publicstr := hex.EncodeToString(n.core.PublicKey())
		logger.Printf("Your public key is %s", publicstr)
		logger.Printf("Your IPv6 address is %s", address.String())
		logger.Printf("Your IPv6 subnet is %s", subnet.String())
		logger.Printf("Your Yggstack resolver name is %s%s", publicstr, types.NameMappingSuffix)
	}

	// Setup the admin socket.
	{
		options := []admin.SetupOption{
			admin.ListenAddress(cfg.AdminListen),
		}
		if cfg.LogLookups {
			options = append(options, admin.LogLookups{})
		}
		if n.admin, err = admin.New(n.core, logger, options...); err != nil {
			panic(err)
		}
		if n.admin != nil {
			n.admin.SetupAdminHandlers()
		}
	}

	// Setup the multicast module.
	{
		options := []multicast.SetupOption{}
		for _, intf := range cfg.MulticastInterfaces {
			options = append(options, multicast.MulticastInterface{
				Regex:    regexp.MustCompile(intf.Regex),
				Beacon:   intf.Beacon,
				Listen:   intf.Listen,
				Port:     intf.Port,
				Priority: uint8(intf.Priority),
				Password: intf.Password,
			})
		}
		if n.multicast, err = multicast.New(n.core, logger, options...); err != nil {
			panic(err)
		}
		if n.admin != nil && n.multicast != nil {
			n.multicast.SetupAdminHandlers(n.admin)
		}
	}

	// Setup Yggdrasil netstack
	s, err := netstack.CreateYggdrasilNetstack(n.core)
	if err != nil {
		panic(err)
	}

	// Create SOCKS server
	{
		if socks != nil && *socks != "" {
			socksOptions := []socks5.Option{
				socks5.WithDial(s.DialContext),
			}
			if nameserver != nil && *nameserver != "" {
				resolver := types.NewNameResolver(s, *nameserver)
				socksOptions = append(socksOptions, socks5.WithResolver(resolver))
			} else {
				logger.Infof("DNS nameserver is not set!")
				logger.Infof("SOCKS server will not be able to resolve hostnames other than .pk.ygg !")
			}
			if logger.GetLevel("debug") {
				socksOptions = append(socksOptions, socks5.WithLogger(logger))
			}
			server := socks5.NewServer(socksOptions...)
			if strings.Contains(*socks, ":") {
				logger.Infof("Starting SOCKS server on %s", *socks)
				go server.ListenAndServe("tcp", *socks) // nolint:errcheck
			} else {
				logger.Infof("Starting SOCKS server with socket file %s", *socks)
				n.socks5Listener, err = net.Listen("unix", *socks)
				if err != nil {
					// If address in use, try connecting to
					// the socket to see if other yggstack
					// instance is listening on it

					if isErrorAddressAlreadyInUse(err) {
						_, err = net.Dial("unix", *socks)
						if err != nil {
							// Unlink dead socket if not connected
							err = os.RemoveAll(*socks)
							if err != nil {
								panic(err)
							}
						} else {
							panic(fmt.Errorf("Another yggstack instance is listening on socket '%s'", *socks))
						}
					} else {
						panic(err)
					}
				}
				go server.Serve(n.socks5Listener) // nolint:errcheck
			}
		}
	}

	// Create local TCP mappings (forwarding connections from local port
	// to remote Yggdrasil node)
	{
		for _, mapping := range localtcp {
			go func(mapping types.TCPMapping) {
				listener, err := net.ListenTCP("tcp", mapping.Listen)
				if err != nil {
					panic(err)
				}
				logger.Infof("Mapping local TCP port %d to Yggdrasil %s", mapping.Listen.Port, mapping.Mapped)
				for {
					c, err := listener.Accept()
					if err != nil {
						panic(err)
					}
					r, err := s.DialTCP(mapping.Mapped)
					if err != nil {
						logger.Errorf("Failed to connect to %s: %s", mapping.Mapped, err)
						_ = c.Close()
						continue
					}
					go types.ProxyTCP(n.core.MTU(), c, r)
				}
			}(mapping)
		}
	}

	// Create local UDP mappings (forwarding connections from local port
	// to remote Yggdrasil node)
	{
		for _, mapping := range localudp {
			go func(mapping types.UDPMapping) {
				mtu := n.core.MTU()
				udpListenConn, err := net.ListenUDP("udp", mapping.Listen)
				if err != nil {
					panic(err)
				}
				logger.Infof("Mapping local UDP port %d to Yggdrasil %s", mapping.Listen.Port, mapping.Mapped)
				localUdpConnections := new(sync.Map)
				udpBuffer := make([]byte, mtu)
				for {
					bytesRead, remoteUdpAddr, err := udpListenConn.ReadFrom(udpBuffer)
					if err != nil {
						if bytesRead == 0 {
							continue
						}
					}

					remoteUdpAddrStr := remoteUdpAddr.String()

					connVal, ok := localUdpConnections.Load(remoteUdpAddrStr)

					if !ok {
						logger.Infof("Creating new session for %s", remoteUdpAddr.String())
						udpFwdConn, err := s.DialUDP(mapping.Mapped)
						if err != nil {
							logger.Errorf("Failed to connect to %s: %s", mapping.Mapped, err)
							continue
						}
						udpSession := &UDPSession{
							conn:       udpFwdConn,
							remoteAddr: remoteUdpAddr,
						}
						localUdpConnections.Store(remoteUdpAddrStr, udpSession)
						go types.ReverseProxyUDP(mtu, udpListenConn, remoteUdpAddr, udpFwdConn)
					}

					udpSession, ok := connVal.(*UDPSession)
					if !ok {
						continue
					}

					udpFwdConnPtr := udpSession.conn.(*gonet.UDPConn)
					udpFwdConn := *udpFwdConnPtr

					_, err = udpFwdConn.Write(udpBuffer[:bytesRead])
					if err != nil {
						logger.Debugf("Cannot write from yggdrasil to udp listener: %q", err)
						udpFwdConn.Close()
						localUdpConnections.Delete(remoteUdpAddrStr)
						continue
					}
				}
			}(mapping)
		}
	}

	// Create remote TCP mappings (forwarding connections from Yggdrasil
	// node to local port)
	{
		for _, mapping := range remotetcp {
			go func(mapping types.TCPMapping) {
				listener, err := s.ListenTCP(mapping.Listen)
				if err != nil {
					panic(err)
				}
				logger.Infof("Mapping Yggdrasil TCP port %d to %s", mapping.Listen.Port, mapping.Mapped)
				for {
					c, err := listener.Accept()
					if err != nil {
						panic(err)
					}
					r, err := net.DialTCP("tcp", nil, mapping.Mapped)
					if err != nil {
						logger.Errorf("Failed to connect to %s: %s", mapping.Mapped, err)
						_ = c.Close()
						continue
					}
					go types.ProxyTCP(n.core.MTU(), c, r)
				}
			}(mapping)
		}
	}

	// Create remote UDP mappings (forwarding connections from Yggdrasil
	// node to local port)
	{
		for _, mapping := range remoteudp {
			go func(mapping types.UDPMapping) {
				mtu := n.core.MTU()
				udpListenConn, err := s.ListenUDP(mapping.Listen)
				if err != nil {
					panic(err)
				}
				logger.Infof("Mapping Yggdrasil UDP port %d to %s", mapping.Listen.Port, mapping.Mapped)
				remoteUdpConnections := new(sync.Map)
				udpBuffer := make([]byte, mtu)
				for {
					bytesRead, remoteUdpAddr, err := udpListenConn.ReadFrom(udpBuffer)
					if err != nil {
						if bytesRead == 0 {
							continue
						}
					}

					remoteUdpAddrStr := remoteUdpAddr.String()

					connVal, ok := remoteUdpConnections.Load(remoteUdpAddrStr)

					if !ok {
						logger.Infof("Creating new session for %s", remoteUdpAddr.String())
						udpFwdConn, err := net.DialUDP("udp", nil, mapping.Mapped)
						if err != nil {
							logger.Errorf("Failed to connect to %s: %s", mapping.Mapped, err)
							continue
						}
						udpSession := &UDPSession{
							conn:       udpFwdConn,
							remoteAddr: remoteUdpAddr,
						}
						remoteUdpConnections.Store(remoteUdpAddrStr, udpSession)
						go types.ReverseProxyUDP(mtu, udpListenConn, remoteUdpAddr, udpFwdConn)
					}

					udpSession, ok := connVal.(*UDPSession)
					if !ok {
						continue
					}

					udpFwdConnPtr := udpSession.conn.(*net.UDPConn)
					udpFwdConn := *udpFwdConnPtr

					_, err = udpFwdConn.Write(udpBuffer[:bytesRead])
					if err != nil {
						logger.Debugf("Cannot write from yggdrasil to udp listener: %q", err)
						udpFwdConn.Close()
						remoteUdpConnections.Delete(remoteUdpAddrStr)
						continue
					}
				}
			}(mapping)
		}
	}

	// Block until we are told to shut down.
	<-ctx.Done()

	// Shut down the node.
	_ = n.admin.Stop()
	_ = n.multicast.Stop()
	if n.socks5Listener != nil {
		_ = n.socks5Listener.Close()
		_ = os.RemoveAll(*socks)
		logger.Infof("Stopped UNIX socket listener")
	}
	n.core.Stop()
}

// Helper to detect if socket address is in use
// https://stackoverflow.com/a/52152912
func isErrorAddressAlreadyInUse(err error) bool {
	var eOsSyscall *os.SyscallError
	if !errors.As(err, &eOsSyscall) {
		return false
	}
	var errErrno syscall.Errno // doesn't need a "*" (ptr) because it's already a ptr (uintptr)
	if !errors.As(eOsSyscall, &errErrno) {
		return false
	}
	if errors.Is(errErrno, syscall.EADDRINUSE) {
		return true
	}
	const WSAEADDRINUSE = 10048
	if runtime.GOOS == "windows" && errErrno == WSAEADDRINUSE {
		return true
	}
	return false
}

// Helper to set logging level
func setLogLevel(loglevel string, logger *log.Logger) {
	levels := [...]string{"error", "warn", "info", "debug", "trace"}
	loglevel = strings.ToLower(loglevel)

	contains := func() bool {
		for _, l := range levels {
			if l == loglevel {
				return true
			}
		}
		return false
	}

	if !contains() { // set default log level
		logger.Infoln("Loglevel parse failed. Set default level(info)")
		loglevel = "info"
	}

	for _, l := range levels {
		logger.EnableLevel(l)
		if l == loglevel {
			break
		}
	}
}
