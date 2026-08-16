// Package vault provides AES-256-GCM encryption for sensitive local data
// (login credentials). The encryption key is a random 32-byte secret
// generated on first run and stored as keyfile next to the accounts file in
// the user config dir. The encrypted file is useless without the keyfile, and
// the keyfile persists across reinstalls (independent of install path).
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// magic identifies the vault file format version.
const (
	magic    = "MNEMOVAULT1"
	keyfile  = "vault.key"
)

var (
	keyCache  [32]byte
	keyLoaded bool
	keyMu     sync.Mutex
)

// loadKey loads (or generates on first run) the 32-byte master key from the
// keyfile in dir.
func loadKey(dir string) ([32]byte, error) {
	keyMu.Lock()
	defer keyMu.Unlock()
	if keyLoaded {
		return keyCache, nil
	}
	_ = os.MkdirAll(dir, 0o700)
	path := filepath.Join(dir, keyfile)
	b, err := os.ReadFile(path)
	if err == nil && len(b) == 32 {
		copy(keyCache[:], b)
		keyLoaded = true
		return keyCache, nil
	}
	// generate new key
	var k [32]byte
	if _, err := io.ReadFull(rand.Reader, k[:]); err != nil {
		return k, err
	}
	if err := os.WriteFile(path, k[:], 0o600); err != nil {
		return k, err
	}
	keyCache = k
	keyLoaded = true
	return k, nil
}

// Encrypt encrypts plaintext bytes and returns a base64-encoded payload
// (magic + 12-byte nonce + ciphertext+tag). dir is where the keyfile lives.
func Encrypt(plain []byte, dir string) (string, error) {
	key, err := loadKey(dir)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nil, nonce, plain, []byte(magic))
	payload := append([]byte(magic), nonce...)
	payload = append(payload, ct...)
	return base64.StdEncoding.EncodeToString(payload), nil
}

// Decrypt reverses Encrypt.
func Decrypt(encoded string, dir string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	if len(raw) < len(magic)+12 || string(raw[:len(magic)]) != magic {
		return nil, errors.New("vault: invalid payload")
	}
	key, err := loadKey(dir)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	off := len(magic)
	nonce := raw[off : off+gcm.NonceSize()]
	ct := raw[off+gcm.NonceSize():]
	return gcm.Open(nil, nonce, ct, []byte(magic))
}
