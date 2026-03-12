package main

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gologme/log"
	gsyslog "github.com/hashicorp/go-syslog"
	"github.com/hjson/hjson-go/v4"

	"github.com/yggdrasil-network/yggdrasil-go/src/address"
	"github.com/yggdrasil-network/yggdrasil-go/src/config"
	"github.com/yggdrasil-network/yggdrasil-go/src/version"

	"github.com/yggdrasil-network/yggstack/mod/yggstack"
	"github.com/yggdrasil-network/yggstack/src/types"
)

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

	ygg, err := yggstack.New(yggstack.ConfigObj{
		Ctx:             ctx,
		Config:          cfg,
		Logger:          logger,
		MulticastLogger: logger,
		SocksAddr:       *socks,
		SocksVerbose:    logger.GetLevel("debug"),
		Nameserver:      *nameserver,
		LocalTCP:        localtcp,
		LocalUDP:        localudp,
		RemoteTCP:       remotetcp,
		RemoteUDP:       remoteudp,
	})
	if err != nil {
		panic(err)
	}

	// Create SOCKS server
	{
		if socks != nil && *socks != "" {
			socksOptions := []socks5.Option{
				socks5.WithDial(s.DialContext),
			}
			var resolver *types.NameResolver = nil
			if nameserver != nil && *nameserver != "" {
				resolver = types.NewNameResolver(s, *nameserver)
			} else {
				logger.Infof("DNS nameserver is not set!")
				logger.Infof("SOCKS server will not be able to resolve hostnames other than .pk.ygg !")
				resolver = types.NewNameResolver(s, "")
			}
			socksOptions = append(socksOptions, socks5.WithResolver(resolver))
			if logger.GetLevel("debug") {
				socksOptions = append(socksOptions, socks5.WithLogger(logger))
			}
			server := socks5.NewServer(socksOptions...)
			if strings.Contains(*socks, ":") {
				logger.Infof("Starting SOCKS server on %s", *socks)
				n.socks5Tcp, err = net.Listen("tcp", *socks)
				if err != nil {
					panic(err)
				}
				go func() {
					err := server.Serve(n.socks5Tcp)
					if err != nil {
						panic(err)
					}
				}()
			} else {
				logger.Infof("Starting SOCKS server with socket file %s", *socks)
				n.socks5Unix, err = net.Listen("unix", *socks)
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
				go func() {
					err := server.Serve(n.socks5Unix)
					if err != nil {
						panic(err)
					}
				}()
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
						logger.Debugf("Creating new session for %s", remoteUdpAddr.String())
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
						logger.Debugf("udp readFrom error: %v", err)
					}
					if bytesRead == 0 {
						continue
					}

					remoteUdpAddrStr := remoteUdpAddr.String()

					var udpSession *UDPSession = nil

					connVal, ok := remoteUdpConnections.Load(remoteUdpAddrStr)

					if !ok {
						logger.Debugf("Creating new session for %s", remoteUdpAddr.String())
						udpFwdConn, err := net.DialUDP("udp", nil, mapping.Mapped)
						if err != nil {
							logger.Errorf("Failed to connect to %s: %s", mapping.Mapped, err)
							continue
						}
						udpSession = &UDPSession{
							conn:       udpFwdConn,
							remoteAddr: remoteUdpAddr,
						}
						remoteUdpConnections.Store(remoteUdpAddrStr, udpSession)
						go types.ReverseProxyUDP(mtu, udpListenConn, remoteUdpAddr, udpFwdConn)
					} else {
						udpSession, ok = connVal.(*UDPSession)

						if !ok {
							continue
						}
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


// //

func setLogLevel(loglevel string, logger *log.Logger) {
	levels := [...]string{"error", "warn", "info", "debug", "trace"}
	loglevel = strings.TrimSpace(strings.ToLower(loglevel))

	found := false
	for _, lvl := range levels {
		if lvl == loglevel {
			found = true
			break
		}
	}
	if !found {
		logger.Infoln("Loglevel parse failed. Set default level(info)")
		loglevel = "info"
	}

	for _, lvl := range levels {
		logger.EnableLevel(lvl)
		if lvl == loglevel {
			break
		}
	}
}
