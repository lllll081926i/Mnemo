package preview

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"mnemo-go/internal/netx"
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
		"Host":                "x",
		"Connection":          "keep-alive",
		"Authorization":       "Bearer tok",
		"Content-Length":      "100",
		"Proxy-Authorization": "x",
		"Range":               "bytes=0-100",
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
		"normal.txt":       "normal.txt",
		`has"quote.txt`:    "hasquote.txt",
		`back\slash.txt`:   "backslash.txt",
		"":                 "file",
		"中文文件名.mp4":        "中文文件名.mp4",
		"ctrl\x01char.txt": "ctrlchar.txt",
	}
	for in, want := range cases {
		got := sanitizeDispositionFilename(in)
		if got != want {
			t.Errorf("sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestProxyRangeAndHead(t *testing.T) {
	data := []byte("0123456789")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		http.ServeContent(w, r, "video.mp4", time.Unix(1, 0), bytes.NewReader(data))
	}))
	defer upstream.Close()

	s, err := NewServer()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	proxyURL := s.ProxyURL(upstream.URL+"/video.mp4", nil, "演示\"文件.mp4")

	req, err := http.NewRequest(http.MethodGet, proxyURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=2-5")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusPartialContent || string(body) != "2345" {
		t.Fatalf("range response = %d %q", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Content-Range"); got != "bytes 2-5/10" {
		t.Fatalf("Content-Range = %q", got)
	}
	if got := resp.Header.Get("Content-Disposition"); !strings.Contains(got, "inline") || strings.Contains(got, "\\\"") {
		t.Fatalf("unsafe Content-Disposition = %q", got)
	}

	headReq, err := http.NewRequest(http.MethodHead, proxyURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	headResp, err := http.DefaultClient.Do(headReq)
	if err != nil {
		t.Fatal(err)
	}
	headBody, _ := io.ReadAll(headResp.Body)
	headResp.Body.Close()
	if headResp.StatusCode != http.StatusOK || len(headBody) != 0 {
		t.Fatalf("head response = %d body=%q", headResp.StatusCode, headBody)
	}
}

func TestProxyUsesGlobalNetworkProxy(t *testing.T) {
	previousProxy := netx.GlobalProxy()
	t.Cleanup(func() { netx.SetGlobalProxy(previousProxy) })

	targetHits := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetHits++
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer target.Close()

	proxyHits := 0
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyHits++
		if r.URL.String() != target.URL+"/video.mp4" {
			t.Errorf("proxy target = %q, want %q", r.URL.String(), target.URL+"/video.mp4")
		}
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("proxied-video"))
	}))
	defer proxy.Close()
	netx.SetGlobalProxy(proxy.URL)

	s, err := NewServer()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	resp, err := http.Get(s.ProxyURL(target.URL+"/video.mp4", nil, "video.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if resp.StatusCode != http.StatusOK || string(body) != "proxied-video" {
		t.Fatalf("proxy response = %d %q", resp.StatusCode, body)
	}
	if proxyHits != 1 || targetHits != 0 {
		t.Fatalf("proxy hits=%d target hits=%d, want proxy=1 target=0", proxyHits, targetHits)
	}
}

func TestLocalRangeAndHead(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/sample.mp4"
	if err := os.WriteFile(path, []byte("abcdefghij"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	req, err := http.NewRequest(http.MethodGet, s.LocalURL(path), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=3-6")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusPartialContent || string(body) != "defg" {
		t.Fatalf("local range response = %d %q", resp.StatusCode, body)
	}

	headReq, err := http.NewRequest(http.MethodHead, s.LocalURL(path), nil)
	if err != nil {
		t.Fatal(err)
	}
	headResp, err := http.DefaultClient.Do(headReq)
	if err != nil {
		t.Fatal(err)
	}
	headBody, _ := io.ReadAll(headResp.Body)
	headResp.Body.Close()
	if headResp.StatusCode != http.StatusOK || len(headBody) != 0 {
		t.Fatalf("local head response = %d body=%q", headResp.StatusCode, headBody)
	}
}

func TestProxyErrorsAndRedirectProtection(t *testing.T) {
	s, err := NewServer()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	badToken := s.BaseURL() + "/proxy/?u=" + "https%3A%2F%2Fexample.com%2Fvideo.mp4&t=wrong"
	resp, err := http.Get(badToken)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("bad token status = %d", resp.StatusCode)
	}

	errorUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream failed", http.StatusServiceUnavailable)
	}))
	defer errorUpstream.Close()
	resp, err = http.Get(s.ProxyURL(errorUpstream.URL, nil, "error.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("upstream error status = %d", resp.StatusCode)
	}

	privateTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("must not be reached"))
	}))
	defer privateTarget.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, nil, privateTarget.URL, http.StatusFound)
	}))
	defer redirector.Close()
	resp, err = http.Get(s.ProxyURL(redirector.URL, nil, "redirect.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("private redirect status = %d", resp.StatusCode)
	}
}

var _ = http.MethodGet
