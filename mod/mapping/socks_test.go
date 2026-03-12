package mapping

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// // // // // // // // // //

func TestRemoveUnixSocket_NotExist(t *testing.T) {
	err := removeUnixSocket(filepath.Join(t.TempDir(), "no-such-file.sock"))
	if err == nil {
		t.Fatal("expected error for non-existent path")
	}
}

func TestRemoveUnixSocket_RegularFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sock")
	if err := os.WriteFile(path, []byte("socket-stub"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := removeUnixSocket(path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Error("file should have been removed")
	}
}

func TestRemoveUnixSocket_Symlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.sock")
	link := filepath.Join(dir, "link.sock")

	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	err := removeUnixSocket(link)
	if err == nil {
		t.Fatal("expected error when path is a symlink")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("unexpected error text: %v", err)
	}
	// Both files must remain untouched.
	if _, err := os.Lstat(link); err != nil {
		t.Errorf("symlink unexpectedly removed: %v", err)
	}
	if _, err := os.Lstat(target); err != nil {
		t.Errorf("target unexpectedly removed: %v", err)
	}
}
