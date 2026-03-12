package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	yggconfig "github.com/yggdrasil-network/yggdrasil-go/src/config"

	"github.com/yggdrasil-network/yggstack/mod/yggstack"
)

// // // // // // // // // //

const (
	peer            = "tls://yggdrasil.sunsung.fun:4443"
	port            = 8080
	shutdownTimeout = 5 * time.Second
)

// //

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Plain TCP server.
	tcpListener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		fmt.Println("Error: listen TCP:", err)
		return
	}
	go (&http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "hello from the network")
		}),
	}).Serve(tcpListener)
	fmt.Printf("HTTP    http://localhost:%d\n", port)

	// //

	// Yggdrasil server.
	cfg := yggconfig.GenerateConfig()
	cfg.AdminListen = "none"
	cfg.Peers = []string{peer}

	ygg, err := yggstack.New(yggstack.ConfigObj{Ctx: ctx, Config: cfg, CoreStopTimeout: shutdownTimeout})
	if err != nil {
		fmt.Println("Error: start yggdrasil:", err)
		return
	}
	defer ygg.Close()

	yggListener, err := ygg.ListenTCP(&net.TCPAddr{IP: ygg.Address(), Port: port})
	if err != nil {
		fmt.Println("Error: listen yggdrasil:", err)
		return
	}
	go (&http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "hello from the Yggdrasil network")
		}),
	}).Serve(yggListener)
	fmt.Printf("Yggdrasil http://[%s]:%d\n", ygg.Address(), port)

	// //

	<-ctx.Done()
}
