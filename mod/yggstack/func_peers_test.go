package yggstack

import (
	"encoding/json"
	"testing"

	"github.com/yggdrasil-network/yggdrasil-go/src/config"
	"github.com/yggdrasil-network/yggdrasil-go/src/core"
)

// // // // // // // // // //

func TestGetPeers_NoPeers(t *testing.T) {
	cfg := config.GenerateConfig()
	c, err := core.New(cfg.Certificate, noopLoggerObj{})
	if err != nil {
		t.Fatalf("core.New: %s", err)
	}
	defer c.Stop()

	obj := &Obj{Core: c}
	peers := obj.GetPeers()
	if peers == nil {
		t.Fatal("GetPeers should return non-nil slice")
	}
	if len(peers) != 0 {
		t.Fatalf("expected 0 peers, got %d", len(peers))
	}
}

func TestGetPeersJSON_ValidJSON(t *testing.T) {
	cfg := config.GenerateConfig()
	c, err := core.New(cfg.Certificate, noopLoggerObj{})
	if err != nil {
		t.Fatalf("core.New: %s", err)
	}
	defer c.Stop()

	obj := &Obj{Core: c}
	data, err := obj.GetPeersJSON()
	if err != nil {
		t.Fatalf("GetPeersJSON error: %s", err)
	}

	var result []PeerInfoObj
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid JSON: %s", err)
	}
}

func TestPeerInfoObj_JSONFields(t *testing.T) {
	info := PeerInfoObj{
		URI:       "tcp://example.com:443",
		Up:        true,
		Inbound:   false,
		PublicKey: "abcdef1234567890",
		RXBytes:   1024,
		TXBytes:   2048,
		Uptime:    60.5,
		Latency:   12.3,
		LastError: "",
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("json.Marshal: %s", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal: %s", err)
	}

	if parsed["uri"] != "tcp://example.com:443" {
		t.Fatalf("wrong uri: %v", parsed["uri"])
	}
	if parsed["up"] != true {
		t.Fatalf("wrong up: %v", parsed["up"])
	}
	// last_error with omitempty must be absent from JSON when empty
	if _, exists := parsed["last_error"]; exists {
		t.Fatal("last_error should be omitted when empty")
	}
}
