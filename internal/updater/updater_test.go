package updater

import (
	"crypto/ed25519"
	"encoding/base64"
	"testing"
)

func TestVerifyReleaseSignature(t *testing.T) {
	previous := updateSigningPublicKey
	defer func() { updateSigningPublicKey = previous }()
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	message := []byte("sha256  Mnemo-windows-x64-Setup.exe\n")
	signature := ed25519.Sign(privateKey, message)
	updateSigningPublicKey = base64.StdEncoding.EncodeToString(publicKey)
	if err := verifyReleaseSignature(message, []byte(base64.StdEncoding.EncodeToString(signature))); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	message[0] = 'X'
	if err := verifyReleaseSignature(message, []byte(base64.StdEncoding.EncodeToString(signature))); err == nil {
		t.Fatal("tampered checksums accepted")
	}
}

func TestVerifyReleaseSignatureRequiresKey(t *testing.T) {
	previous := updateSigningPublicKey
	defer func() { updateSigningPublicKey = previous }()
	updateSigningPublicKey = ""
	if err := verifyReleaseSignature([]byte("checksums"), []byte("signature")); err == nil {
		t.Fatal("signature verification unexpectedly succeeded without public key")
	}
}

func TestReleaseSignatureRequiredWhenPublicKeyIsConfigured(t *testing.T) {
	previous := updateSigningPublicKey
	defer func() { updateSigningPublicKey = previous }()

	updateSigningPublicKey = ""
	if releaseSignatureRequired() {
		t.Fatal("empty public key must keep historical unsigned releases compatible")
	}
	updateSigningPublicKey = "test-public-key"
	if !releaseSignatureRequired() {
		t.Fatal("configured public key must require a release signature")
	}
}
