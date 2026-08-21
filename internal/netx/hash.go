package netx

import (
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"strconv"
	"strings"
)

// HashKind is a content fingerprint type.
type HashKind string

const (
	HashMD5  HashKind = "md5"
	HashSHA1 HashKind = "sha1"
)

// NewHash returns the hasher for a kind.
func NewHash(kind HashKind) (hash.Hash, error) {
	switch kind {
	case HashMD5:
		return md5.New(), nil
	case HashSHA1:
		return sha1.New(), nil
	default:
		return nil, fmt.Errorf("netx: unsupported hash kind %q", kind)
	}
}

// HashReader hashes a stream and returns the hex digest.
func HashReader(r io.Reader, kind HashKind) (string, error) {
	h, err := NewHash(kind)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// HashFile hashes a local file.
func HashFile(path string, kind HashKind) (string, error) {
	return HashFileWithProgress(path, kind, nil, 0)
}

// HashFileWithProgress hashes a local file while reporting bytes read via the
// optional progress callback. total, if non-zero, sets the denominator for
// pct computation (usually the file size).
func HashFileWithProgress(path string, kind HashKind, progress func(read int64), total int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h, err := NewHash(kind)
	if err != nil {
		return "", err
	}
	if total <= 0 {
		if info, err := f.Stat(); err == nil {
			total = info.Size()
		}
	}
	buf := make([]byte, 256*1024)
	var read int64
	for {
		n, err := f.Read(buf)
		if n > 0 {
			if _, werr := h.Write(buf[:n]); werr != nil {
				return "", werr
			}
			read += int64(n)
			if progress != nil {
				progress(read)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// MD5Hex hashes bytes to hex md5.
func MD5Hex(data []byte) string {
	sum := md5.Sum(data)
	return hex.EncodeToString(sum[:])
}

// SHA1Hex hashes bytes to hex sha1.
func SHA1Hex(data []byte) string {
	sum := sha1.Sum(data)
	return hex.EncodeToString(sum[:])
}

// CRC64ECMA computes the ECMA-182 crc64 (used by aliyun) and returns the
// decimal string form.
func CRC64ECMA(data []byte) string {
	const poly = 0xC96C5795D7870F42
	crc := uint64(0)
	for _, b := range data {
		crc ^= uint64(b)
		for i := 0; i < 8; i++ {
			if crc&1 != 0 {
				crc = (crc >> 1) ^ poly
			} else {
				crc >>= 1
			}
		}
	}
	return strconv.FormatUint(crc, 10)
}

// ParseSize parses "1.5 GB", "2MB", "100B" style strings to bytes.
func ParseSize(s string) int64 {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return 0
	}
	units := []struct {
		suffix string
		mult   int64
	}{
		{"TB", 1 << 40}, {"GB", 1 << 30}, {"MB", 1 << 20}, {"KB", 1 << 10}, {"B", 1},
	}
	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			num := strings.TrimSpace(strings.TrimSuffix(s, u.suffix))
			f, err := strconv.ParseFloat(num, 64)
			if err != nil {
				return 0
			}
			return int64(f * float64(u.mult))
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}
