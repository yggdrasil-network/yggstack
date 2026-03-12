package types

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"net"
	"strings"

	"github.com/yggdrasil-network/yggdrasil-go/src/address"
	"github.com/yggdrasil-network/yggstack/src/netstack"
)

// // // // // // // // // //

const NameMappingSuffix = ".pk.ygg"

type NameResolver struct {
	resolver *net.Resolver
}

func NewNameResolver(stack *netstack.YggdrasilNetstack, nameserver string) *NameResolver {
	res := &NameResolver{
		resolver: &net.Resolver{
			PreferGo: true,
		},
	}
	if nameserver != "" {
		ns := nameserver
		res.resolver.Dial = func(ctx context.Context, network, address string) (net.Conn, error) { // nolint:staticcheck
			host, port, err := net.SplitHostPort(ns)
			if err != nil {
				// Default to dns service when no port given.
				port = "dns"
				host = ns
			}
			address = net.JoinHostPort(host, port)
			return stack.DialContext(ctx, network, address)
		}
	}
	return res
}

// //

func (r *NameResolver) Resolve(ctx context.Context, name string) (context.Context, net.IP, error) {
	if strings.HasSuffix(name, NameMappingSuffix) {
		name = strings.TrimSuffix(name, NameMappingSuffix)
		// If the remaining part contains dots, take only the rightmost label
		// as the public key (e.g. "subdomain.<pubkey>.pk.ygg").
		if idx := strings.LastIndex(name, "."); idx >= 0 {
			name = name[idx+1:]
		}
		b, err := hex.DecodeString(name)
		if err != nil {
			return nil, nil, fmt.Errorf("hex.DecodeString: %w", err)
		}
		// Reject keys with wrong length — copy() would silently zero-pad, producing an invalid address.
		if len(b) != ed25519.PublicKeySize {
			return nil, nil, fmt.Errorf("public key must be %d bytes, got %d", ed25519.PublicKeySize, len(b))
		}
		var pk [ed25519.PublicKeySize]byte
		copy(pk[:], b)
		return ctx, net.IP(address.AddrForKey(pk[:])[:]), nil
	}
	ip := net.ParseIP(name)
	if ip == nil {
		addrs, err := r.resolver.LookupIP(ctx, "ip6", name)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to lookup %q: %s", name, err)
		}
		if len(addrs) == 0 {
			return nil, nil, fmt.Errorf("no addresses for %q", name)
		}
		return ctx, addrs[0], nil
	}
	return ctx, ip, nil
}
