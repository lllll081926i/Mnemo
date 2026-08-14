package ilanzou

import (
	"crypto/aes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strconv"
	"time"
)

// aesEncryptToHex performs AES-128-ECB (PKCS7) over the UTF-8 plaintext with
// the secret's UTF-8 bytes (must be exactly 16 bytes), returning lowercase hex.
func aesEncryptToHex(plain, secret string) (string, error) {
	key := []byte(secret)
	if len(key) != 16 {
		return "", errors.New("ilanzou secret must be 16 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	padded := pkcs7Pad([]byte(plain), block.BlockSize())
	out := make([]byte, len(padded))
	for i := 0; i < len(padded); i += block.BlockSize() {
		block.Encrypt(out[i:i+block.BlockSize()], padded[i:i+block.BlockSize()])
	}
	return hex.EncodeToString(out), nil
}

// getTimestampToken returns the current unix-ms timestamp and its AES-ECB
// encryption, mirroring the legacy crypto helper.
func getTimestampToken(secret string) (int64, string, error) {
	ts := time.Now().UnixMilli()
	tsEnc, err := aesEncryptToHex(strconv.FormatInt(ts, 10), secret)
	return ts, tsEnc, err
}

// newDeviceUuid returns a 32-char (no-dash) v4 uuid token.
func newDeviceUuid() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10xx
	return hex.EncodeToString(b)
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	pad := blockSize - len(data)%blockSize
	out := make([]byte, len(data)+pad)
	copy(out, data)
	for i := len(data); i < len(out); i++ {
		out[i] = byte(pad)
	}
	return out
}
