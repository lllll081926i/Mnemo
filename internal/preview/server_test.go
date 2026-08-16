package preview

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewServer(t *testing.T) {
	s, err := NewServer(t.TempDir())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer s.Close()
	if s.Port == 0 {
		t.Fatal("port not assigned")
	}
	if s.token == "" {
		t.Fatal("token not set")
	}
}

func TestValidToken(t *testing.T) {
	s, _ := NewServer()
	defer s.Close()
	r := httptest.NewRequest("GET", "/proxy/?u=http://x.com&t="+s.token, nil)
	if !s.validToken(r) {
		t.Fatal("valid token rejected")
	}
	r2 := httptest.NewRequest("GET", "/proxy/?u=http://x.com&t=wrong", nil)
	if s.validToken(r2) {
		t.Fatal("invalid token accepted")
	}
}

func TestIsSafeProxyURL(t *testing.T) {
	unsafe := []string{
		"http://127.0.0.1/x",
		"http://10.0.0.1/x",
		"http://192.168.1.1/x",
		"http://172.16.0.1/x",
		"http://localhost/x",
		"ftp://example.com/x",
		"http://169.254.1.1/x",
	}
	for _, u := range unsafe {
		if isSafeProxyURL(u) {
			t.Errorf("expected unsafe: %s", u)
		}
	}
	safe := []string{
		"https://example.com/file.mp4",
		"http://123pan.com/api",
		"https://cdn.example.com/path",
	}
	for _, u := range safe {
		if !isSafeProxyURL(u) {
			t.Errorf("expected safe: %s", u)
		}
	}
}

func TestIsWithinRoots(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewServer(dir)
	defer s.Close()
	if !s.isWithinRoots(dir) {
		t.Error("root itself not within roots")
	}
	if s.isWithinRoots("/etc/passwd") {
		t.Error("/etc/passwd should not be within roots")
	}
}

func TestFilterProxyHeaders(t *testing.T) {
	in := map[string]string{
		"Host":               "x",
		"Connection":         "keep-alive",
		"Authorization":      "Bearer tok",
		"Content-Length":     "100",
		"Proxy-Authorization": "x",
		"Range":              "bytes=0-100",
	}
	out := filterProxyHeaders(in)
	for _, drop := range []string{"host", "connection", "content-length", "proxy-authorization"} {
		if _, ok := out[drop]; ok {
			t.Errorf("header %s should be filtered", drop)
		}
	}
	if out["Authorization"] != "Bearer tok" {
		t.Error("Authorization should be kept")
	}
	if out["Range"] != "bytes=0-100" {
		t.Error("Range should be kept")
	}
}

func TestSanitizeDispositionFilename(t *testing.T) {
	cases := map[string]string{
		"normal.txt":        "normal.txt",
		`has"quote.txt`:     "hasquote.txt",
		`back\slash.txt`:    "backslash.txt",
		"":                  "file",
		"中文文件名.mp4":        "中文文件名.mp4",
		"ctrl\x01char.txt":  "ctrlchar.txt",
	}
	for in, want := range cases {
		got := sanitizeDispositionFilename(in)
		if got != want {
			t.Errorf("sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

var _ = http.MethodGet
