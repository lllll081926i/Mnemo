package netx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewHash(t *testing.T) {
	if _, err := NewHash(HashMD5); err != nil {
		t.Fatalf("md5: %v", err)
	}
	if _, err := NewHash(HashSHA1); err != nil {
		t.Fatalf("sha1: %v", err)
	}
	if _, err := NewHash("crc32"); err == nil {
		t.Fatal("unknown kind must error")
	}
}

func TestMD5Hex(t *testing.T) {
	if got := MD5Hex([]byte("abc")); got != "900150983cd24fb0d6963f7d28e17f72" {
		t.Fatalf("MD5Hex(abc) = %q", got)
	}
}

func TestSHA1Hex(t *testing.T) {
	if got := SHA1Hex([]byte("abc")); got != "a9993e364706816aba3e25717850c26c9cd0d89d" {
		t.Fatalf("SHA1Hex(abc) = %q", got)
	}
}

func TestHashReader(t *testing.T) {
	got, err := HashReader(strings.NewReader("abc"), HashMD5)
	if err != nil {
		t.Fatal(err)
	}
	if got != "900150983cd24fb0d6963f7d28e17f72" {
		t.Fatalf("HashReader = %q", got)
	}
}

func TestHashFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(p, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := HashFile(p, HashSHA1)
	if err != nil {
		t.Fatal(err)
	}
	if got != "a9993e364706816aba3e25717850c26c9cd0d89d" {
		t.Fatalf("HashFile = %q", got)
	}
	if _, err := HashFile(filepath.Join(dir, "missing"), HashMD5); err == nil {
		t.Fatal("missing file must error")
	}
}

func TestParseSize(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"1.5 GB", 1.5 * (1 << 30)},
		{"2MB", 2 << 20},
		{"100B", 100},
		{"512", 512},
		{"1kb", 1 << 10},
		{"", 0},
		{"nonsense", 0},
		{" 3 TB ", 3 << 40},
	}
	for _, c := range cases {
		if got := ParseSize(c.in); got != c.want {
			t.Errorf("ParseSize(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}
