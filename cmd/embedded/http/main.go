package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	golog "github.com/gologme/log"
	qrcode "github.com/skip2/go-qrcode"
	yggconfig "github.com/yggdrasil-network/yggdrasil-go/src/config"
	"github.com/yggdrasil-network/yggstack/mod/yggstack"
)

// // // // // // // // // //

const shutdownTimeout = 2 * time.Second

// //

func main() {
	wwwPath := flag.String("www", "www", "path to the www directory")
	cfgPath := flag.String("config", "conf.yml", "path to the config file")
	flag.Parse()

	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		fmt.Println("Error: load config:", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	nodeCfg := yggconfig.GenerateConfig()
	nodeCfg.AdminListen = "none"
	nodeCfg.Peers = cfg.Peers
	if cfg.PrivateKey != "" {
		key, err := hex.DecodeString(cfg.PrivateKey)
		if err != nil || len(key) != 64 {
			fmt.Println("Error: invalid private_key — must be a 128-char hex string (64 bytes)")
			os.Exit(1)
		}
		nodeCfg.PrivateKey = key
	}

	logger := golog.New(os.Stdout, "", golog.LstdFlags)
	logger.EnableLevel("info")
	logger.EnableLevel("warn")
	logger.EnableLevel("error")

	ygg, err := yggstack.New(yggstack.ConfigObj{
		Ctx:             ctx,
		Config:          nodeCfg,
		CoreStopTimeout: shutdownTimeout,
		Logger:          logger,
	})
	if err != nil {
		fmt.Println("Error: start yggdrasil:", err)
		os.Exit(1)
	}
	defer ygg.Close()

	// Log the private key when auto-generated so the user can persist it in conf.yml.
	if cfg.PrivateKey == "" {
		logger.Warnf("auto-generated private key (add to conf.yml to keep the same address across restarts):")
		logger.Warnf("private_key: %s", hex.EncodeToString(nodeCfg.PrivateKey))
	}

	info := newInfoHandler(ygg.Core, cfg, logger)

	yggAddr := ygg.Core.Address().String()
	qrURL := fmt.Sprintf("http://[%s]:%d/", yggAddr, cfg.YggPorts[0])
	qrHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		png, err := qrcode.Encode(qrURL, qrcode.Medium, 256)
		if err != nil {
			http.Error(w, "qr error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(png)
	})

	// Plain HTTP servers.
	for _, port := range cfg.HTTPPorts {
		l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			fmt.Printf("Error: listen HTTP :%d: %v\n", port, err)
			os.Exit(1)
		}
		go (&http.Server{
			Handler:           buildMux(*wwwPath, info, false, qrHandler),
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       60 * time.Second,
		}).Serve(l)
		fmt.Printf("HTTP       http://%s:%d/\n", cfg.Hostname, port)
	}

	// Yggdrasil HTTP servers.
	for _, port := range cfg.YggPorts {
		l, err := ygg.ListenTCP(&net.TCPAddr{IP: ygg.Core.Address(), Port: port})
		if err != nil {
			fmt.Printf("Error: listen Yggdrasil :%d: %v\n", port, err)
			os.Exit(1)
		}
		go (&http.Server{
			Handler:           buildMux(*wwwPath, info, true, qrHandler),
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       60 * time.Second,
		}).Serve(l)
		fmt.Printf("Yggdrasil  http://[%s]:%d/\n", yggAddr, port)
	}

	<-ctx.Done()
}

// //

func buildMux(wwwPath string, info *InfoHandlerObj, isYgg bool, qr http.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/yggdrasil-server.json", info.Handler(isYgg))
	mux.Handle("/ygg-qr.png", qr)
	if isYgg {
		mux.Handle("/", newYggFileHandler(wwwPath))
	} else {
		mux.Handle("/", newPlainFileHandler(wwwPath))
	}
	return mux
}
