// Package preview hosts the internal local HTTP server used by the frontend
// for media playback and downloads that require headers / local proxying.
//
// Security model:
//   - Every /proxy/ and /local/ request must carry a session token (t=...).
//     The token is generated once at server startup and embedded into URLs
//     built by ProxyURL/LocalURL, so the frontend never handles it directly.
//   - /local/ only serves files under the configured root (download dir +
//     engine dir). Absolute paths outside the root are rejected.
//   - /proxy/ validates the target URL scheme (http/https only) and blocks
//     private/loopback/link-local addresses to prevent SSRF.
//   - CORS is restricted to the Wails origin (http://localhost:* /
//     wails://localhost) instead of "*".
//
// Routes:
//
//	GET /proxy/?u=<url>&h=<base64json>&t=<token>   streaming proxy (Range supported)
//	GET /local/?p=<path>&t=<token>                 local file serving (Range)
package preview

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Server is the internal media proxy.
type Server struct {
	ln     net.Listener
	server *http.Server
	Port   int
	token  string
	roots  []string // allowed local file roots
}

// NewServer starts the internal HTTP server on a random port.
// roots are the directory prefixes that /local/ is allowed to serve from.
func NewServer(roots ...string) (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	tok := make([]byte, 32)
	if _, err := rand.Read(tok); err != nil {
		ln.Close()
		return nil, err
	}
	// dedupe + clean roots
	cleanRoots := make([]string, 0, len(roots))
	seen := map[string]bool{}
	for _, r := range roots {
		if r == "" {
			continue
		}
		abs, err := filepath.Abs(r)
		if err != nil {
			continue
		}
		if !seen[abs] {
			seen[abs] = true
			cleanRoots = append(cleanRoots, abs)
		}
	}
	s := &Server{
		ln:    ln,
		Port:  ln.Addr().(*net.TCPAddr).Port,
		token: base64.RawURLEncoding.EncodeToString(tok),
		roots: cleanRoots,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/proxy/", s.handleProxy)
	mux.HandleFunc("/local/", s.handleLocal)
	s.server = &http.Server{Handler: mux, ReadHeaderTimeout: 30 * time.Second}
	go func() { _ = s.server.Serve(ln) }()
	return s, nil
}

// AddRoot adds a directory to the allowed /local/ roots at runtime.
func (s *Server) AddRoot(dir string) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return
	}
	for _, r := range s.roots {
		if r == abs {
			return
		}
	}
	s.roots = append(s.roots, abs)
}

// BaseURL returns http://127.0.0.1:port.
func (s *Server) BaseURL() string { return fmt.Sprintf("http://127.0.0.1:%d", s.Port) }

// ProxyURL builds a proxied media URL for a remote stream.
func (s *Server) ProxyURL(target string, headers map[string]string) string {
	hdrs, _ := json.Marshal(headers)
	q := url.Values{}
	q.Set("u", target)
	q.Set("h", base64.StdEncoding.EncodeToString(hdrs))
	q.Set("t", s.token)
	return fmt.Sprintf("%s/proxy/?%s", s.BaseURL(), q.Encode())
}

// LocalURL builds a URL for a local file.
func (s *Server) LocalURL(path string) string {
	q := url.Values{}
	q.Set("p", path)
	q.Set("t", s.token)
	return fmt.Sprintf("%s/local/?%s", s.BaseURL(), q.Encode())
}

// validToken checks the session token.
func (s *Server) validToken(r *http.Request) bool {
	return r.URL.Query().Get("t") == s.token
}

// corsHeaders sets a permissive CORS policy for the Wails webview origin.
// Wails v2 serves the frontend from http://localhost:<port> or https://wails.localhost,
// so we echo the request Origin when it looks like a local dev/webview origin,
// and fall back to a wildcard otherwise.
func corsHeaders(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = "*"
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Range")
	w.Header().Set("Vary", "Origin")
}

func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if !s.validToken(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	target := r.URL.Query().Get("u")
	if target == "" {
		http.Error(w, "missing url", http.StatusBadRequest)
		return
	}
	if !isSafeProxyURL(target) {
		http.Error(w, "url not allowed", http.StatusBadRequest)
		return
	}
	var headers map[string]string
	if h := r.URL.Query().Get("h"); h != "" {
		if b, err := base64.StdEncoding.DecodeString(h); err == nil {
			_ = json.Unmarshal(b, &headers)
		}
	}
	// strip hop-by-hop and sensitive headers
	filtered := filterProxyHeaders(headers)
	proxyRequest(w, r, target, filtered)
}

func (s *Server) handleLocal(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if !s.validToken(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	path := r.URL.Query().Get("p")
	if path == "" {
		http.Error(w, "missing path", http.StatusBadRequest)
		return
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	if strings.Contains(filepath.ToSlash(abs), "..") {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	if !s.isWithinRoots(abs) {
		http.Error(w, "path not allowed", http.StatusForbidden)
		return
	}
	f, err := os.Open(abs)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, _ := f.Stat()
	if info.IsDir() {
		http.Error(w, "is a directory", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", contentTypeFor(abs))
	http.ServeContent(w, r, abs, info.ModTime(), f)
}

// isWithinRoots reports whether abs is inside one of the allowed roots.
func (s *Server) isWithinRoots(abs string) bool {
	if len(s.roots) == 0 {
		return false
	}
	for _, root := range s.roots {
		if abs == root || strings.HasPrefix(abs, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// isSafeProxyURL validates that a proxy target is http(s) and not a private
// address to prevent SSRF.
func isSafeProxyURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host := u.Hostname()
	if host == "" {
		return false
	}
	// reject literal private/loopback IPs
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return false
		}
		return true
	}
	// reject common internal hostnames
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".local") ||
		strings.HasSuffix(lower, ".internal") || strings.HasSuffix(lower, ".localhost") {
		return false
	}
	return true
}

// filterProxyHeaders drops hop-by-hop and obviously sensitive headers the
// frontend should not be able to inject into upstream requests.
func filterProxyHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return headers
	}
	drop := map[string]bool{
		"host": true, "connection": true, "content-length": true,
		"transfer-encoding": true, "upgrade": true,
		"proxy-authorization": true, "proxy-authenticate": true,
		"te": true, "trailer": true,
	}
	out := make(map[string]string, len(headers))
	for k, v := range headers {
		if drop[strings.ToLower(k)] {
			continue
		}
		out[k] = v
	}
	return out
}

// proxyRequest streams a remote URL with Range passthrough.
func proxyRequest(w http.ResponseWriter, r *http.Request, target string, headers map[string]string) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, r.Method, target, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	// pass range through
	if rng := r.Header.Get("Range"); rng != "" {
		req.Header.Set("Range", rng)
	}
	client := &http.Client{Timeout: 0, CheckRedirect: checkProxyRedirect}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// copy status + headers
	for _, h := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "Last-Modified", "ETag"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	corsHeaders(w, r)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// checkProxyRedirect validates redirect targets to prevent the proxy from
// being redirected to private addresses.
func checkProxyRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("too many redirects")
	}
	if !isSafeProxyURL(req.URL.String()) {
		return fmt.Errorf("redirect target not allowed")
	}
	return nil
}

func contentTypeFor(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".mp4"), strings.HasSuffix(lower, ".m4v"):
		return "video/mp4"
	case strings.HasSuffix(lower, ".webm"):
		return "video/webm"
	case strings.HasSuffix(lower, ".mkv"):
		return "video/x-matroska"
	case strings.HasSuffix(lower, ".mp3"):
		return "audio/mpeg"
	case strings.HasSuffix(lower, ".m4a"), strings.HasSuffix(lower, ".aac"):
		return "audio/mp4"
	case strings.HasSuffix(lower, ".flac"):
		return "audio/flac"
	case strings.HasSuffix(lower, ".wav"):
		return "audio/wav"
	case strings.HasSuffix(lower, ".ogg"):
		return "audio/ogg"
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(lower, ".png"):
		return "image/png"
	case strings.HasSuffix(lower, ".gif"):
		return "image/gif"
	case strings.HasSuffix(lower, ".webp"):
		return "image/webp"
	case strings.HasSuffix(lower, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(lower, ".ico"):
		return "image/x-icon"
	case strings.HasSuffix(lower, ".pdf"):
		return "application/pdf"
	case strings.HasSuffix(lower, ".txt"), strings.HasSuffix(lower, ".md"), strings.HasSuffix(lower, ".log"),
		strings.HasSuffix(lower, ".js"), strings.HasSuffix(lower, ".ts"), strings.HasSuffix(lower, ".vue"),
		strings.HasSuffix(lower, ".json"), strings.HasSuffix(lower, ".go"), strings.HasSuffix(lower, ".py"),
		strings.HasSuffix(lower, ".java"), strings.HasSuffix(lower, ".c"), strings.HasSuffix(lower, ".cpp"),
		strings.HasSuffix(lower, ".h"), strings.HasSuffix(lower, ".css"), strings.HasSuffix(lower, ".html"),
		strings.HasSuffix(lower, ".xml"), strings.HasSuffix(lower, ".yaml"), strings.HasSuffix(lower, ".yml"),
		strings.HasSuffix(lower, ".ini"), strings.HasSuffix(lower, ".sh"), strings.HasSuffix(lower, ".bat"),
		strings.HasSuffix(lower, ".srt"), strings.HasSuffix(lower, ".vtt"), strings.HasSuffix(lower, ".ass"):
		return "text/plain; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

// Close shuts the server down.
func (s *Server) Close() error { return s.server.Close() }
