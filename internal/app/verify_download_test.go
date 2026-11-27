package app

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestHashLocalFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.bin")
	payload := []byte("mnemo high performance verification payload")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	want := hex.EncodeToString(sum[:])

	got, label, err := hashLocalFile(t.Context(), path, "sha256")
	if err != nil {
		t.Fatalf("hashLocalFile: %v", err)
	}
	if label != "SHA256" || got != want {
		t.Fatalf("got (%s, %s), want (SHA256, %s)", label, got, want)
	}
}
