package mobile

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/quic-go/quic-go"
)

// CheckQUICPeer measures RTT to a QUIC peer by performing handshake
// Returns RTT in milliseconds or -1 on error
// URI format: quic://host:port
func CheckQUICPeer(uri string) int64 {
	// Add panic recovery
	defer func() {
		if r := recover(); r != nil {
			// Can't log here, just return -1
		}
	}()

	// Quick validation
	if uri == "" {
		return -1
	}

	rtt, err := checkQUICConnection(uri, 5*time.Second)
	if err != nil {
		return -1
	}
	return rtt
}

// checkQUICConnection performs QUIC handshake and measures RTT
func checkQUICConnection(uriString string, timeout time.Duration) (int64, error) {
	// Parse URI
	u, err := url.Parse(uriString)
	if err != nil {
		return 0, fmt.Errorf("failed to parse URI: %w", err)
	}

	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "443" // Default QUIC port
	}

	addr := net.JoinHostPort(host, port)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Configure TLS to match Yggdrasil's config
	// - No ALPN/NextProtos (yggdrasil doesn't use them)
	// - TLS 1.3 only
	// - InsecureSkipVerify for RTT measurement
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS13,
		MaxVersion:         tls.VersionTLS13,
	}

	// Configure QUIC
	quicConf := &quic.Config{
		HandshakeIdleTimeout: timeout,
		MaxIdleTimeout:       timeout,
	}

	// Measure RTT: start time before dial
	startTime := time.Now()

	// Dial QUIC connection
	conn, err := quic.DialAddr(ctx, addr, tlsConf, quicConf)
	if err != nil {
		return 0, err
	}
	defer conn.CloseWithError(0, "")

	// Calculate RTT (time from start to handshake complete)
	rtt := time.Since(startTime)

	return rtt.Milliseconds(), nil
}
