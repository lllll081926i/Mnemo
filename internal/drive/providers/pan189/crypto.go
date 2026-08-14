package pan189

import (
	"crypto/aes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// md5Hex returns the uppercase MD5 hex digest (189 uses uppercase md5).
func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

// md5Sum returns the raw MD5 digest of b.
func md5Sum(b []byte) []byte {
	sum := md5.Sum(b)
	return sum[:]
}

func hexEncode(b []byte) string { return hex.EncodeToString(b) }

func base64StdEncode(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// timestamp returns the current unix millis.
func timestamp() int64 { return time.Now().UnixMilli() }

// getHTTPDateStr renders the RFC1123 GMT date used in the signature header
// (mirrors new Date().toUTCString()).
func getHTTPDateStr() string {
	return time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
}

// clientSuffix returns the client identity query params appended to every
// request (mirrors legacy clientSuffix()).
func clientSuffix() map[string]string {
	r1 := randN(1e5)
	r2 := randN(1e10)
	return map[string]string{
		"clientType": pc,
		"version":    version,
		"channelId":  channelID,
		"rand":       fmt.Sprintf("%d_%d", r1, r2),
	}
}

func randN(limit int64) int64 {
	if limit <= 0 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(limit))
	if err != nil {
		return time.Now().UnixNano() % limit
	}
	return n.Int64()
}

// requestURIPath extracts the path component used in the signature
// (mirrors fullUrl.match(/:\/\/[^/]+((\/[^/\s?#]+)*)/)).
func requestURIPath(fullURL string) string {
	u, err := url.Parse(fullURL)
	if err != nil {
		return "/"
	}
	p := u.Path
	if p == "" {
		return "/"
	}
	return p
}

// signatureOfHmac computes the SessionKey/Operate/RequestURI/Date HMAC-SHA1
// signature (uppercase hex) with the session secret.
func signatureOfHmac(sessionSecret, sessionKey, operate, fullURL, dateGMT, param string) string {
	data := "SessionKey=" + sessionKey + "&Operate=" + operate + "&RequestURI=" + requestURIPath(fullURL) + "&Date=" + dateGMT
	if param != "" {
		data += "&params=" + param
	}
	mac := hmac.New(sha1.New, []byte(sessionSecret))
	_, _ = mac.Write([]byte(data))
	return strings.ToUpper(hex.EncodeToString(mac.Sum(nil)))
}

// aesECBEncrypt encrypts data with AES-128-ECB (PKCS7) using the first 16
// bytes of key as UTF-8 bytes; output is uppercase hex (params encryption).
func aesECBEncrypt(data, key string) (string, error) {
	keyBytes := []byte(key)
	if len(keyBytes) > 16 {
		keyBytes = keyBytes[:16]
	}
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", err
	}
	padded := pkcs7Pad([]byte(data), block.BlockSize())
	out := make([]byte, len(padded))
	for i := 0; i < len(padded); i += block.BlockSize() {
		block.Encrypt(out[i:i+block.BlockSize()], padded[i:i+block.BlockSize()])
	}
	return strings.ToUpper(hex.EncodeToString(out)), nil
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

// encodeParams serialises params with sorted keys (k=v&k2=v2).
func encodeParams(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+params[k])
	}
	return strings.Join(parts, "&")
}

// encryptParams AES-ECB encrypts the sorted params with the session secret.
func encryptParams(params map[string]string, sessionSecret string) (string, error) {
	if len(params) == 0 {
		return "", nil
	}
	return aesECBEncrypt(encodeParams(params), sessionSecret)
}

// rsaEncrypt encrypts origData with PKCS#1 v1.5 using the SPKI/PKCS#8 public
// key (pem text without the ---- header lines, as returned by encryptConf);
// output is uppercase hex zero-padded to the key size.
func rsaEncrypt(pubKeyPEM, origData string) (string, error) {
	key, err := parseRSAPublicKey(pubKeyPEM)
	if err != nil {
		return "", err
	}
	ct, err := rsa.EncryptPKCS1v15(rand.Reader, key, []byte(origData))
	if err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(ct)), nil
}

// parseRSAPublicKey parses a PEM-encoded SPKI/PKCS#8 RSA public key. The 189
// encryptConf endpoint returns the base64 body without the PEM header lines.
func parseRSAPublicKey(pemBody string) (*rsa.PublicKey, error) {
	// Case 1: full PEM armor (with header/footer lines).
	if block, _ := pem.Decode([]byte(pemBody)); block != nil && block.Type == "PUBLIC KEY" {
		if pub, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
			if rk, ok := pub.(*rsa.PublicKey); ok {
				return rk, nil
			}
		}
	}
	// Case 2: bare base64 body (189 style); strip armor lines + whitespace.
	der, err := decodeBase64Lenient(pemBody)
	if err != nil {
		return nil, errors.New("189: RSA 公钥解码失败")
	}
	if pub, err := x509.ParsePKIXPublicKey(der); err == nil {
		if rk, ok := pub.(*rsa.PublicKey); ok {
			return rk, nil
		}
	}
	return parseManualRSAPublicKey(der)
}

// parseManualRSAPublicKey walks a DER SEQUENCE reading the first two INTEGERs
// (n, e), mirroring the legacy simplified parser.
func parseManualRSAPublicKey(der []byte) (*rsa.PublicKey, error) {
	ints := make([]*big.Int, 0, 2)
	i := 0
	readLen := func() int {
		l := int(der[i])
		i++
		if l&0x80 != 0 {
			n := l & 0x7f
			l = 0
			for j := 0; j < n; j++ {
				l = (l << 8) | int(der[i])
				i++
			}
		}
		return l
	}
	for i < len(der) && len(ints) < 2 {
		tag := der[i]
		i++
		switch tag {
		case 0x02: // INTEGER
			l := readLen()
			start := i
			if der[start] == 0 {
				start++
			}
			v := new(big.Int)
			v.SetBytes(der[start : i+l])
			ints = append(ints, v)
			i += l
		case 0x30, 0x03, 0x00:
			if tag == 0x03 { // BIT STRING
				readLen()
				i++ // unused bits
			} else {
				readLen()
			}
		default:
			i += readLen()
		}
	}
	if len(ints) < 2 {
		return nil, errors.New("189: 解析 RSA 公钥失败")
	}
	return &rsa.PublicKey{N: ints[0], E: int(ints[1].Int64())}, nil
}

// decodeBase64Lenient decodes base64 tolerating line breaks.
func decodeBase64Lenient(s string) ([]byte, error) {
	var sb strings.Builder
	for _, r := range s {
		switch r {
		case '\n', '\r', ' ', '\t':
			continue
		default:
			sb.WriteRune(r)
		}
	}
	clean := sb.String()
	if pad := len(clean) % 4; pad != 0 {
		clean += strings.Repeat("=", 4-pad)
	}
	return base64.StdEncoding.DecodeString(clean)
}

// partSize computes the upload chunk size (mirrors legacy partSize):
//   - default 10 MiB
//   - files > 999 chunks (≈9.99 GiB) use 20 MiB chunks
//   - files > 1999*20 MiB (≈39.02 GiB) use a larger chunk so the part count
//     stays under 1999.
func partSize(size int64) int64 {
	const def = int64(10 * 1024 * 1024)
	switch {
	case size > def*2*999:
		single := (size + 1999*def - 1) / (1999 * def) // ceil(size/1999/DEFAULT)
		if single < 5 {
			single = 5
		}
		return single * def
	case size > def*999:
		return def * 2
	default:
		return def
	}
}
