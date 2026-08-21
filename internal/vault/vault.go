// Package vault provides AES-256-GCM encryption for sensitive local data
// (login credentials). On Windows the random master key is protected with
// DPAPI; non-Windows platforms retain the legacy 0600 keyfile until a native
// Keychain/Secret-Service adapter is added. Existing legacy keyfiles remain
// readable for migration.
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
	magic   = "MNEMOVAULT1"
	keyfile = "vault.key"
)

var (
	keyCache          [32]byte
	keyLoaded         bool
	keyDir            string
	keyMu             sync.Mutex
	readProtectedKey  = readOSProtectedKey
	writeProtectedKey = writeOSProtectedKey
)

// loadKey loads (or generates on first run) the 32-byte master key from the
// keyfile in dir.
func loadKey(dir string) ([32]byte, error) {
	keyMu.Lock()
	defer keyMu.Unlock()
	if keyLoaded && keyDir == dir {
		return keyCache, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return [32]byte{}, err
	}
	path := filepath.Join(dir, keyfile)
	protected, protectedErr := readProtectedKey(dir)
	if protectedErr == nil {
		if len(protected) != len(keyCache) {
			return [32]byte{}, errors.New("vault: OS protected key has invalid length")
		}
		copy(keyCache[:], protected)
		keyDir = dir
		keyLoaded = true
		return keyCache, nil
	}
	b, err := os.ReadFile(path)
	if err == nil && len(b) == 32 {
		copy(keyCache[:], b)
		keyDir = dir
		keyLoaded = true
		// Only create a protected copy when no protected key exists. If DPAPI
		// has an existing but unreadable blob, the validated legacy key remains
		// usable; do not overwrite the evidence or create a different key.
		if errors.Is(protectedErr, os.ErrNotExist) {
			_ = writeProtectedKey(dir, b)
		}
		return keyCache, nil
	}
	if err == nil && len(b) != 32 {
		return [32]byte{}, errors.New("vault: legacy key has invalid length")
	}
	if protectedErr != nil && !errors.Is(protectedErr, os.ErrNotExist) {
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return [32]byte{}, errors.Join(errors.New("vault: OS protected key unavailable"), protectedErr, err)
		}
		// The protected file exists but cannot be decrypted and there is no
		// usable legacy copy. Generating a replacement would make existing
		// encrypted accounts permanently unreadable, so fail closed instead.
		return [32]byte{}, errors.Join(errors.New("vault: OS protected key cannot be read; refusing to generate a replacement"), protectedErr)
	}
	// generate new key
	var k [32]byte
	if _, err := io.ReadFull(rand.Reader, k[:]); err != nil {
		return k, err
	}
	if err := writeProtectedKey(dir, k[:]); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return k, err
		}
		if err := writePrivateFileAtomically(path, k[:]); err != nil {
			return k, err
		}
	}
	keyCache = k
	keyDir = dir
	keyLoaded = true
	return k, nil
}

// writePrivateFileAtomically keeps a completed key file intact if the process
// is interrupted while writing a new key. Callers only use it for a missing
// destination; an existing unreadable key is never replaced automatically.
func writePrivateFileAtomically(path string, data []byte) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".vault-key-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		if tmpPath != "" {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	tmpPath = ""
	return os.Chmod(path, 0o600)
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
