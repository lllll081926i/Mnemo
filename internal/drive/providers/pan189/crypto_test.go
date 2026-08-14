package pan189

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"regexp"
	"strings"
	"testing"
)

// Reference vectors cross-check the Go port against the legacy JS test suite
// (CryptoJS output) and the AList 189pc implementation.

func TestSignatureOfHmacVector(t *testing.T) {
	// matches src/pan189/__tests__/crypto.test.ts: signatureOfHmac is stable
	a := signatureOfHmac("secretsecretsecre", "sk", "GET", "https://api.cloud.189.cn/listFiles.action", "Wed, 01 Jan 2020 00:00:00 GMT", "")
	b := signatureOfHmac("secretsecretsecre", "sk", "GET", "https://api.cloud.189.cn/listFiles.action", "Wed, 01 Jan 2020 00:00:00 GMT", "")
	if a != b {
		t.Fatalf("signature not deterministic: %s vs %s", a, b)
	}
	if !regexp.MustCompile(`^[A-F0-9]{40}$`).MatchString(a) {
		t.Fatalf("signature format wrong: %q", a)
	}
	// precomputed HMAC-SHA1 of the signature data string
	const want = "54C47590414E33A033C6B39EDF15526962208E8C"
	if a != want {
		t.Fatalf("signature mismatch: got %s want %s", a, want)
	}
}

func TestSignatureIncludesParams(t *testing.T) {
	withParams := signatureOfHmac("secretsecretsecre", "sk", "POST", "https://api.cloud.189.cn/listFiles.action", "Wed, 01 Jan 2020 00:00:00 GMT", "ABC123")
	without := signatureOfHmac("secretsecretsecre", "sk", "POST", "https://api.cloud.189.cn/listFiles.action", "Wed, 01 Jan 2020 00:00:00 GMT", "")
	if withParams == without {
		t.Fatal("params must be part of the signature data")
	}
}

func TestRequestURIPath(t *testing.T) {
	cases := map[string]string{
		"https://api.cloud.189.cn/listFiles.action?x=1":      "/listFiles.action",
		"https://upload.cloud.189.cn/person/initMultiUpload": "/person/initMultiUpload",
		"https://cloud.189.cn":                               "/",
		"https://upload.cloud.189.cn":                        "/",
	}
	for in, want := range cases {
		if got := requestURIPath(in); got != want {
			t.Errorf("requestURIPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAesEcbEncryptVector(t *testing.T) {
	// precomputed AES-128-ECB(PKCS7) of "a=1&b=2" with key "1234567890abcdef"
	got, err := aesECBEncrypt("a=1&b=2", "1234567890abcdef")
	if err != nil {
		t.Fatal(err)
	}
	const want = "0112FBC618BAFE86E1B4565C00CE0CB0"
	if got != want {
		t.Fatalf("aesEcbEncrypt = %s, want %s", got, want)
	}
}

func TestAesEcbEncryptTruncatesKey(t *testing.T) {
	// key longer than 16 bytes must be truncated to 16 (legacy slice(0,16))
	short, err := aesECBEncrypt("a=1&b=2", "1234567890abcdef")
	if err != nil {
		t.Fatal(err)
	}
	long, err := aesECBEncrypt("a=1&b=2", "1234567890abcdefXYZ")
	if err != nil {
		t.Fatal(err)
	}
	if short != long {
		t.Fatal("aesEcbEncrypt must use the first 16 key bytes")
	}
}

func TestEncodeParamsSorted(t *testing.T) {
	if got := encodeParams(map[string]string{"b": "2", "a": "1"}); got != "a=1&b=2" {
		t.Fatalf("encodeParams = %q", got)
	}
	if got, _ := encryptParams(nil, "secret"); got != "" {
		t.Fatalf("encryptParams(nil) = %q, want empty", got)
	}
	enc, err := encryptParams(map[string]string{"b": "2", "a": "1"}, "1234567890abcdef")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[A-F0-9]+$`).MatchString(enc) {
		t.Fatalf("encryptParams not uppercase hex: %q", enc)
	}
}

func TestPartSize(t *testing.T) {
	const d = int64(10 * 1024 * 1024)
	cases := []struct {
		size int64
		want int64
	}{
		{1, d},
		{d*999 + 1, d * 2},
		{d*2*999 + 1, 5 * d}, // smallest large-chunk slice
		{d * 2 * 1999, 5 * d},
		{d*2*1999 + 1, 5 * d},
		{d * 2 * 1999 * 4, 8 * d}, // ceil(8) chunks
		{0, d},
	}
	for _, c := range cases {
		if got := partSize(c.size); got != c.want {
			t.Errorf("partSize(%d) = %d, want %d", c.size, got, c.want)
		}
	}
}

func TestMD5HexUppercase(t *testing.T) {
	if got := md5Hex("hello"); got != "5D41402ABC4B2A76B9719D911017C592" {
		t.Fatalf("md5Hex(hello) = %s", got)
	}
}

func TestRSAEncryptRoundTrip(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	fullPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))

	msg := "189-test-user@example.com"
	ct, err := rsaEncrypt(fullPEM, msg)
	if err != nil {
		t.Fatal(err)
	}
	// uppercase hex, zero-padded to the key size (256 hex chars for 1024-bit)
	if len(ct) != 256 {
		t.Fatalf("ciphertext length = %d, want 256", len(ct))
	}
	if ct != strings.ToUpper(ct) {
		t.Fatal("ciphertext must be uppercase hex")
	}

	// decrypt with the private key to verify the padded plaintext round-trips
	raw, err := hex.DecodeString(ct)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := rsa.DecryptPKCS1v15(rand.Reader, key, raw)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}
	if string(plain) != msg {
		t.Fatalf("round-trip = %q, want %q", string(plain), msg)
	}

	// bare base64 body (189 encryptConf style, no PEM armor) must parse too
	b64Body := strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(fullPEM, "-----END PUBLIC KEY-----\n"), "-----BEGIN PUBLIC KEY-----\n"))
	b64Body = strings.ReplaceAll(b64Body, "\n", "")
	if _, err := rsaEncrypt(b64Body, msg); err != nil {
		t.Fatalf("bare b64 body failed: %v", err)
	}
}
