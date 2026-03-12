package types

import (
	"context"
	"crypto/ed25519"
	"net"
	"strings"
	"testing"
)

// // // // // // // // // //

// validPubKeyHex — 32 bytes encoded as hex (64 chars).
const validPubKeyHex = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"

// newResolver creates a NameResolver with no DNS server (for tests that do not make network calls).
func newResolver() *NameResolver {
	return NewNameResolver(nil, "")
}

// //

func TestResolver_PkYgg_Valid(t *testing.T) {
	r := newResolver()
	ctx := context.Background()

	name := validPubKeyHex + NameMappingSuffix
	_, ip, err := r.Resolve(ctx, name)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip == nil {
		t.Fatal("got nil IP")
	}
	// Yggdrasil addresses always start with 0x02.
	if len(ip) < 1 || ip[0] != 0x02 {
		t.Errorf("expected Yggdrasil address (first byte 0x02), got %v", ip)
	}
}

func TestResolver_PkYgg_Subdomain(t *testing.T) {
	r := newResolver()
	ctx := context.Background()

	// Subdomain before pubkey should be stripped.
	name := "sub." + validPubKeyHex + NameMappingSuffix
	_, ip, err := r.Resolve(ctx, name)
	if err != nil {
		t.Fatalf("unexpected error with subdomain: %v", err)
	}
	if ip == nil {
		t.Fatal("got nil IP")
	}
}

func TestResolver_PkYgg_InvalidHex(t *testing.T) {
	r := newResolver()
	ctx := context.Background()

	name := "notvalidhex" + NameMappingSuffix
	_, _, err := r.Resolve(ctx, name)
	if err == nil {
		t.Fatal("expected error for invalid hex")
	}
	if !strings.Contains(err.Error(), "hex.DecodeString") {
		t.Errorf("unexpected error text: %v", err)
	}
}

func TestResolver_PkYgg_WrongKeyLength(t *testing.T) {
	r := newResolver()
	ctx := context.Background()

	// 4 bytes — not a valid ed25519 key.
	shortHex := "deadbeef" + NameMappingSuffix
	_, _, err := r.Resolve(ctx, shortHex)
	if err == nil {
		t.Fatal("expected error for short key")
	}
	if !strings.Contains(err.Error(), "public key must be") {
		t.Errorf("unexpected error text: %v", err)
	}
}

func TestResolver_PkYgg_WrongKeyLength_TooLong(t *testing.T) {
	r := newResolver()
	ctx := context.Background()

	// 33 bytes — key is too long.
	longHex := strings.Repeat("aa", ed25519.PublicKeySize+1) + NameMappingSuffix
	_, _, err := r.Resolve(ctx, longHex)
	if err == nil {
		t.Fatal("expected error for oversized key")
	}
}

func TestResolver_IPLiteral_IPv4(t *testing.T) {
	r := newResolver()
	ctx := context.Background()

	_, ip, err := r.Resolve(ctx, "192.168.1.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip.Equal(net.ParseIP("192.168.1.1")) == false {
		t.Errorf("got %v, want 192.168.1.1", ip)
	}
}

func TestResolver_IPLiteral_IPv6(t *testing.T) {
	r := newResolver()
	ctx := context.Background()

	_, ip, err := r.Resolve(ctx, "2001:db8::1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip.Equal(net.ParseIP("2001:db8::1")) == false {
		t.Errorf("got %v, want 2001:db8::1", ip)
	}
}
