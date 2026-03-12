package mobile

import (
	"encoding/json"
	"fmt"
	"net"

	"github.com/yggdrasil-network/yggdrasil-go/src/config"
	"github.com/yggdrasil-network/yggstack/src/types"
)

// // // // // // // // // //

// GenerateConfig generates a new Yggdrasil node configuration with a random key pair.
// Returns a JSON string suitable for storing and passing to LoadConfigJSON.
func GenerateConfig() (string, error) {
	nodeCfg := config.GenerateConfig()
	nodeCfg.AdminListen = "none"
	b, err := json.MarshalIndent(nodeCfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal config: %w", err)
	}
	return string(b), nil
}

// //

// parseTCPMapping parses two "host:port" strings into a TCPMapping.
// listenStr — the local address to listen on (e.g. "127.0.0.1:8080").
// mappedStr — the remote address to forward to (e.g. "[200::1]:8080").
func parseTCPMapping(listenStr, mappedStr string) (types.TCPMapping, error) {
	listenAddr, err := net.ResolveTCPAddr("tcp", listenStr)
	if err != nil {
		return types.TCPMapping{}, fmt.Errorf("invalid listen address %q: %w", listenStr, err)
	}
	mappedAddr, err := net.ResolveTCPAddr("tcp", mappedStr)
	if err != nil {
		return types.TCPMapping{}, fmt.Errorf("invalid mapped address %q: %w", mappedStr, err)
	}
	return types.TCPMapping{Listen: listenAddr, Mapped: mappedAddr}, nil
}

// parseUDPMapping parses two "host:port" strings into a UDPMapping.
func parseUDPMapping(listenStr, mappedStr string) (types.UDPMapping, error) {
	listenAddr, err := net.ResolveUDPAddr("udp", listenStr)
	if err != nil {
		return types.UDPMapping{}, fmt.Errorf("invalid listen address %q: %w", listenStr, err)
	}
	mappedAddr, err := net.ResolveUDPAddr("udp", mappedStr)
	if err != nil {
		return types.UDPMapping{}, fmt.Errorf("invalid mapped address %q: %w", mappedStr, err)
	}
	return types.UDPMapping{Listen: listenAddr, Mapped: mappedAddr}, nil
}

// parseRemoteTCPMapping builds a remote TCPMapping from a port number and local service address.
// port — the Yggdrasil-side listen port.
// localStr — the local service address to forward to (e.g. "127.0.0.1:8080").
func parseRemoteTCPMapping(port int, localStr string) (types.TCPMapping, error) {
	if port < 1 || port > 65535 {
		return types.TCPMapping{}, fmt.Errorf("port %d out of range 1-65535", port)
	}
	mappedAddr, err := net.ResolveTCPAddr("tcp", localStr)
	if err != nil {
		return types.TCPMapping{}, fmt.Errorf("invalid local address %q: %w", localStr, err)
	}
	return types.TCPMapping{
		Listen: &net.TCPAddr{Port: port},
		Mapped: mappedAddr,
	}, nil
}

// parseRemoteUDPMapping builds a remote UDPMapping from a port number and local service address.
func parseRemoteUDPMapping(port int, localStr string) (types.UDPMapping, error) {
	if port < 1 || port > 65535 {
		return types.UDPMapping{}, fmt.Errorf("port %d out of range 1-65535", port)
	}
	mappedAddr, err := net.ResolveUDPAddr("udp", localStr)
	if err != nil {
		return types.UDPMapping{}, fmt.Errorf("invalid local address %q: %w", localStr, err)
	}
	return types.UDPMapping{
		Listen: &net.UDPAddr{Port: port},
		Mapped: mappedAddr,
	}, nil
}
