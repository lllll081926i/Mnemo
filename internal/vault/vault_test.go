package vault

import (
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
