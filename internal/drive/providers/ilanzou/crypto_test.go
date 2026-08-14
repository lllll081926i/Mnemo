package ilanzou

import (
	"crypto/aes"
	"encoding/hex"
	"regexp"
	"testing"
)

// NIST FIPS-197 C.1: AES-128-ECB of 00112233445566778899aabbccddeeff with key
// 000102030405060708090a0b0c0d0e0f → 69c4e0d86a7b0430d8cdb78070b4c55a.
func TestAesEncryptKnownVector(t *testing.T) {
	key, _ := hex.DecodeString("000102030405060708090a0b0c0d0e0f")
	plain, _ := hex.DecodeString("00112233445566778899aabbccddeeff")
	got, err := aesEncryptToHex(string(plain), string(key))
	if err != nil {
		t.Fatal(err)
	}
	// PKCS7 pads the aligned 16-byte plaintext with a full pad block; the
	// first block must equal the raw NIST C.1 vector.
	if got[:32] != "69c4e0d86a7b0430d8cdb78070b4c55a" {
		t.Fatalf("AES-ECB first block mismatch: %s", got)
	}
	// cross-check the whole padded output against direct stdlib block usage
	block, _ := aes.NewCipher(key)
	padded := pkcs7Pad(plain, block.BlockSize())
	want := make([]byte, len(padded))
	for i := 0; i < len(padded); i += block.BlockSize() {
		block.Encrypt(want[i:i+block.BlockSize()], padded[i:i+block.BlockSize()])
	}
	if got != hex.EncodeToString(want) {
		t.Fatalf("padded ECB mismatch: %s", got)
	}
	// 8-byte plaintext is PKCS7-padded to 16 → 32 hex chars
	got, err = aesEncryptToHex("12345678", string(key))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 32 {
		t.Fatalf("padded block length = %d, want 32", len(got))
	}
}

func TestAesEncryptToHexIlanzou(t *testing.T) {
	got, err := aesEncryptToHex("1234567890", "lanZouY-disk-app")
	if err != nil {
		t.Fatal(err)
	}
	hexRe := regexp.MustCompile(`^[0-9a-f]+$`)
	if !hexRe.MatchString(got) {
		t.Fatalf("not lowercase hex: %s", got)
	}
	if len(got)%32 != 0 {
		t.Fatalf("hex length %d not a multiple of 32", len(got))
	}
	// deterministic for the same input
	again, _ := aesEncryptToHex("1234567890", "lanZouY-disk-app")
	if again != got {
		t.Fatalf("non-deterministic AES: %s vs %s", got, again)
	}
}

func TestAesEncryptSecretLength(t *testing.T) {
	if _, err := aesEncryptToHex("x", "short"); err == nil {
		t.Fatal("expected error for non-16-byte secret")
	}
}

func TestGetTimestampToken(t *testing.T) {
	ts, tsEnc, err := getTimestampToken("lanZouY-disk-app")
	if err != nil {
		t.Fatal(err)
	}
	if ts <= 0 {
		t.Fatalf("ts = %d", ts)
	}
	if ok, _ := regexp.MatchString(`^[0-9a-f]+$`, tsEnc); !ok {
		t.Fatalf("tsEnc not hex: %s", tsEnc)
	}
}

func TestNewDeviceUuid(t *testing.T) {
	id := newDeviceUuid()
	if ok, _ := regexp.MatchString(`^[0-9a-f]{32}$`, id); !ok {
		t.Fatalf("uuid = %q, want 32 hex chars", id)
	}
	// version nibble of byte[6] must be 4
	b, _ := hex.DecodeString(id)
	if b[6]>>4 != 4 {
		t.Fatalf("uuid version nibble = %x, want 4", b[6]>>4)
	}
}
