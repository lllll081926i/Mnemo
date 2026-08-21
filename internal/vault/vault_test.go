package vault

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestEncryptDecryptIsolatedByDirectory(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()
	plain := []byte("account secret")

	ciphertext, err := Encrypt(plain, first)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := Decrypt(ciphertext, first); err != nil || string(got) != string(plain) {
		t.Fatalf("first directory round trip = %q, %v", got, err)
	}
	if _, err := Decrypt(ciphertext, second); err == nil {
		t.Fatal("ciphertext unexpectedly decrypted with another directory key")
	}

	secondCiphertext, err := Encrypt(plain, second)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := Decrypt(secondCiphertext, second); err != nil || string(got) != string(plain) {
		t.Fatalf("second directory round trip = %q, %v", got, err)
	}
}

func TestInvalidLegacyKeyFailsClosed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, keyfile), []byte("invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Encrypt([]byte("secret"), dir); err == nil {
		t.Fatal("invalid legacy key should not be replaced silently")
	}
}

func TestUnreadableProtectedKeyWithoutLegacyFailsClosed(t *testing.T) {
	dir := t.TempDir()
	broken := errors.New("DPAPI decrypt failed")
	withProtectedKeyAdapter(t,
		func(string) ([]byte, error) { return nil, broken },
		func(string, []byte) error {
			t.Fatal("must not replace an unreadable protected key")
			return nil
		},
	)

	if _, err := Encrypt([]byte("secret"), dir); !errors.Is(err, broken) {
		t.Fatalf("Encrypt error = %v, want protected-key error", err)
	}
	if _, err := os.Stat(filepath.Join(dir, keyfile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy replacement = %v, want no file", err)
	}
}

func TestUnreadableProtectedKeyUsesValidatedLegacyWithoutOverwrite(t *testing.T) {
	dir := t.TempDir()
	legacy := make([]byte, 32)
	for i := range legacy {
		legacy[i] = byte(i + 1)
	}
	if err := os.WriteFile(filepath.Join(dir, keyfile), legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	broken := errors.New("DPAPI decrypt failed")
	writes := 0
	withProtectedKeyAdapter(t,
		func(string) ([]byte, error) { return nil, broken },
		func(string, []byte) error {
			writes++
			return nil
		},
	)

	ciphertext, err := Encrypt([]byte("account secret"), dir)
	if err != nil {
		t.Fatalf("Encrypt using validated legacy key: %v", err)
	}
	if writes != 0 {
		t.Fatalf("protected key was unexpectedly overwritten %d times", writes)
	}
	plain, err := Decrypt(ciphertext, dir)
	if err != nil || string(plain) != "account secret" {
		t.Fatalf("legacy fallback round trip = %q, %v", plain, err)
	}
}

func withProtectedKeyAdapter(t *testing.T, read func(string) ([]byte, error), write func(string, []byte) error) {
	t.Helper()
	keyMu.Lock()
	oldRead, oldWrite := readProtectedKey, writeProtectedKey
	oldCache, oldLoaded, oldDir := keyCache, keyLoaded, keyDir
	readProtectedKey, writeProtectedKey = read, write
	keyCache = [32]byte{}
	keyLoaded = false
	keyDir = ""
	keyMu.Unlock()
	t.Cleanup(func() {
		keyMu.Lock()
		readProtectedKey, writeProtectedKey = oldRead, oldWrite
		keyCache, keyLoaded, keyDir = oldCache, oldLoaded, oldDir
		keyMu.Unlock()
	})
}
