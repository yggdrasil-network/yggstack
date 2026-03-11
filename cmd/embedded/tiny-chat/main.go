package main

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	stdlog "log"

	"github.com/gologme/log"

	"github.com/yggdrasil-network/yggdrasil-go/src/address"
	yggconfig "github.com/yggdrasil-network/yggdrasil-go/src/config"

	"github.com/yggdrasil-network/yggstack/mod/yggstack"
)

// // // // // // // // // //

const chatPort = 9998

var shutdownCh = make(chan struct{})

// //

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	scanner := bufio.NewScanner(os.Stdin)

	fmt.Print("Yggdrasil peer URI: ")
	if !scanner.Scan() {
		return
	}
	peerURI := strings.TrimSpace(scanner.Text())
	if peerURI == "" {
		fmt.Println("Error: empty peer URI")
		return
	}

	cfg := yggconfig.GenerateConfig()
	cfg.AdminListen = "none"
	cfg.Peers = []string{peerURI}

	logger := log.New(io.Discard, "", 0)

	ygg, err := yggstack.New(yggstack.ConfigObj{
		Ctx:    ctx,
		Config: cfg,
		Logger: logger,
	})
	if err != nil {
		fmt.Println("Error: failed to start node:", err)
		return
	}
	defer ygg.Close()

	listener, err := ygg.Netstack.ListenTCP(&net.TCPAddr{
		IP:   ygg.Address(),
		Port: chatPort,
	})
	if err != nil {
		fmt.Println("Error: failed to listen:", err)
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", handlePing)
	mux.HandleFunc("/input", handleInput)

	srv := &http.Server{
		Handler:  mux,
		ErrorLog: stdlog.New(io.Discard, "", 0),
	}
	go srv.Serve(listener)

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: ygg.Netstack.DialContext,
		},
		Timeout: 10 * time.Second,
	}

	fmt.Println()
	fmt.Println("Your public key:", hex.EncodeToString(ygg.PublicKey()))
	fmt.Println()
	fmt.Print("Peer public key: ")
	if !scanner.Scan() {
		return
	}
	peerKeyHex := strings.TrimSpace(scanner.Text())

	peerKeyBytes, err := hex.DecodeString(peerKeyHex)
	if err != nil || len(peerKeyBytes) != ed25519.PublicKeySize {
		fmt.Println("Error: invalid public key")
		return
	}

	var peerKey [ed25519.PublicKeySize]byte
	copy(peerKey[:], peerKeyBytes)
	peerAddr := address.AddrForKey(peerKey[:])
	peerIP := net.IP(peerAddr[:])
	peerURL := fmt.Sprintf("http://[%s]:%d", peerIP, chatPort)

	fmt.Println()
	fmt.Print("Waiting for peer...")
	for {
		if ctx.Err() != nil {
			fmt.Println()
			return
		}
		resp, err := client.Get(peerURL + "/ping")
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if string(body) == "OK-chat" {
				break
			}
		}
		fmt.Print(".")
		time.Sleep(2 * time.Second)
	}
	fmt.Println(" connected!")
	fmt.Println()
	fmt.Println("Chat started. Type messages and press Enter. /bye to quit.")
	fmt.Println()

	inputCh := make(chan string)
	go func() {
		for scanner.Scan() {
			inputCh <- scanner.Text()
		}
		close(inputCh)
	}()

	for {
		select {
		case <-shutdownCh:
			return
		case <-ctx.Done():
			return
		case line, ok := <-inputCh:
			if !ok {
				return
			}
			if line == "" {
				continue
			}
			if line == "/bye" {
				sendMessage(client, peerURL, "/bye")
				fmt.Println("Bye!")
				return
			}
			if err := sendMessage(client, peerURL, line); err != nil {
				fmt.Println("[delivery error]", err)
			}
		}
	}
}

// //

func handlePing(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "OK-chat")
}

func handleInput(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		w.WriteHeader(http.StatusOK)
		return
	}
	if msg == "/bye" {
		fmt.Println("[peer disconnected]")
		w.WriteHeader(http.StatusOK)
		close(shutdownCh)
		return
	}
	fmt.Printf(">> %s\n", msg)
	w.WriteHeader(http.StatusOK)
}

func sendMessage(client *http.Client, peerURL string, msg string) error {
	resp, err := client.Post(peerURL+"/input", "text/plain", strings.NewReader(msg))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
