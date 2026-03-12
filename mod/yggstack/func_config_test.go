package yggstack

import (
	"strings"
	"testing"

	"github.com/yggdrasil-network/yggdrasil-go/src/config"
	"github.com/yggdrasil-network/yggdrasil-go/src/core"
)

// // // // // // // // // //

func TestAddPeer_InvalidURI(t *testing.T) {
	cfg := config.GenerateConfig()
	c, err := core.New(cfg.Certificate, noopLoggerObj{})
	if err != nil {
		t.Fatalf("core.New: %s", err)
	}
	defer c.Stop()

	obj := &Obj{Core: c}
	err = obj.AddPeer("://invalid")
	if err == nil {
		t.Fatal("expected error for invalid URI")
	}
}

func TestRemovePeer_InvalidURI(t *testing.T) {
	cfg := config.GenerateConfig()
	c, err := core.New(cfg.Certificate, noopLoggerObj{})
	if err != nil {
		t.Fatalf("core.New: %s", err)
	}
	defer c.Stop()

	obj := &Obj{Core: c}
	err = obj.RemovePeer("://invalid")
	if err == nil {
		t.Fatal("expected error for invalid URI")
	}
}

func TestAddPeer_ValidURI(t *testing.T) {
	cfg := config.GenerateConfig()
	c, err := core.New(cfg.Certificate, noopLoggerObj{})
	if err != nil {
		t.Fatalf("core.New: %s", err)
	}
	defer c.Stop()

	obj := &Obj{Core: c}
	err = obj.AddPeer("tcp://192.0.2.1:12345")
	if err != nil {
		if !strings.Contains(err.Error(), "unsupported") {
			t.Fatalf("unexpected error: %s", err)
		}
	}
}

func TestRetryPeersNow_NoPanic(t *testing.T) {
	cfg := config.GenerateConfig()
	c, err := core.New(cfg.Certificate, noopLoggerObj{})
	if err != nil {
		t.Fatalf("core.New: %s", err)
	}
	defer c.Stop()

	obj := &Obj{Core: c}
	obj.RetryPeersNow()
}
