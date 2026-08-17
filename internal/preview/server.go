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
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"mnemo-go/internal/netx"
)

// Server is the internal media proxy.
type Server struct {
	ln                net.Listener
	server            *http.Server
	Port              int
	token             string
	roots             []string // allowed local file roots
	allowedProxyHosts map[string]struct{}
	proxyClient       *http.Client
	proxyTransport    *http.Transport
	mu                sync.Mutex // guards roots and allowedProxyHosts
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
		abs, err := canonicalPath(r)
		if err != nil {
			continue
		}
		if !seen[abs] {
			seen[abs] = true
			cleanRoots = append(cleanRoots, abs)
		}
	}
	s := &Server{
		ln:                ln,
		Port:              ln.Addr().(*net.TCPAddr).Port,
		token:             base64.RawURLEncoding.EncodeToString(tok),
		roots:             cleanRoots,
		allowedProxyHosts: make(map[string]struct{}),
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableCompression = true
	transport.Proxy = globalProxy
	s.proxyTransport = transport
	s.proxyClient = &http.Client{
		Timeout:       0,
		Transport:     transport,
		CheckRedirect: s.checkProxyRedirect,
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
	abs, err := canonicalPath(dir)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.roots {
		if r == abs {
			return
		}
	}
	s.roots = append(s.roots, abs)
}

// BaseURL returns http://127.0.0.1:port.
func (s *Server) BaseURL() string { return fmt.Sprintf("http://127.0.0.1:%d", s.Port) }

// ProxyURL builds a proxied media URL for a remote stream. filename is an
// optional display name forwarded to the browser via Content-Disposition so
// the preview shows the real file name instead of a guessed one.
func (s *Server) ProxyURL(target string, headers map[string]string, filename string) string {
	s.rememberProxyHost(target)
	hdrs, _ := json.Marshal(headers)
	q := url.Values{}
	q.Set("u", target)
	q.Set("h", base64.StdEncoding.EncodeToString(hdrs))
	q.Set("t", s.token)
	if filename != "" {
		q.Set("f", filename)
	}
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
		return
	}
	if !isLocalOrigin(origin) {
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Range, Content-Type, Accept, If-Range")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range, Accept-Ranges, Content-Disposition, ETag")
	w.Header().Set("Vary", "Origin")
}

func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD, OPTIONS")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
	if !s.isSafeProxyURL(target) {
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
	filename := r.URL.Query().Get("f")
	proxyRequest(s, w, r, target, filtered, filename)
}

func (s *Server) handleLocal(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD, OPTIONS")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
	abs, err := canonicalPath(path)
	if err != nil {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	if !s.isWithinRoots(abs) {
		http.Error(w, "path not allowed", http.StatusForbidden)
		return
	}
	resolved, err := canonicalPath(abs)
	if err != nil || !s.isWithinRoots(resolved) {
		http.Error(w, "path not allowed", http.StatusForbidden)
		return
	}
	f, err := os.Open(resolved)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info == nil {
		http.Error(w, "stat failed", http.StatusInternalServerError)
		return
	}
	if info.IsDir() {
		http.Error(w, "is a directory", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", contentTypeFor(resolved))
	http.ServeContent(w, r, resolved, info.ModTime(), f)
}

// isSafeProxyURL validates that a proxy target is http(s) and not a private
// address to prevent SSRF.
func isSafeProxyURL(raw string) bool {
	return isSafeProxyURLWithAllow(raw, nil)
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
func proxyRequest(s *Server, w http.ResponseWriter, r *http.Request, target string, headers map[string]string, filename string) {
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
	// Media responses must be byte-for-byte identical to the upstream stream.
	// The server-level transport keeps connections reusable across Range
	// requests; DisableCompression prevents the body from being decompressed
	// while Content-Encoding is copied to mpv.
	resp, err := s.proxyClient.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// copy status + headers
	for _, h := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "Last-Modified", "ETag", "Cache-Control", "Expires", "Content-Encoding"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	corsHeaders(w, r)
	// surface the real file name so the browser preview shows it correctly
	// instead of guessing from the opaque proxy URL.
	if filename != "" {
		w.Header().Set("Content-Disposition", "inline; filename=\""+sanitizeDispositionFilename(filename)+"\"")
	} else if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		w.Header().Set("Content-Disposition", cd)
	}
	w.WriteHeader(resp.StatusCode)
	if r.Method == http.MethodHead {
		return
	}
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

func (s *Server) checkProxyRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("too many redirects")
	}
	if !s.isSafeProxyURL(req.URL.String()) {
		return fmt.Errorf("redirect target not allowed")
	}
	return nil
}

func contentTypeFor(path string) string {
	if typ := mime.TypeByExtension(filepath.Ext(path)); typ != "" {
		return typ
	}
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
func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	if s.proxyTransport != nil {
		s.proxyTransport.CloseIdleConnections()
	}
	if s.server == nil {
		return nil
	}
	return s.server.Close()
}

func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return filepath.Clean(real), nil
	}
	// The final file may not exist yet. Resolve the parent so a symlinked
	// directory cannot be used to escape the allowed root.
	parent := filepath.Dir(abs)
	if realParent, err := filepath.EvalSymlinks(parent); err == nil {
		return filepath.Join(realParent, filepath.Base(abs)), nil
	}
	return abs, nil
}

func pathWithin(root, candidate string) bool {
	if runtime.GOOS == "windows" && strings.EqualFold(root, candidate) {
		return true
	}
	if root == candidate {
		return true
	}
	rel, err := filepath.Rel(root, candidate)
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (s *Server) isWithinRoots(abs string) bool {
	candidate, err := canonicalPath(abs)
	if err != nil {
		return false
	}
	s.mu.Lock()
	roots := append([]string(nil), s.roots...)
	s.mu.Unlock()
	for _, root := range roots {
		if pathWithin(root, candidate) {
			return true
		}
	}
	return false
}

func proxyHostKey(u *url.URL) string {
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if port == "" {
		return host
	}
	return net.JoinHostPort(host, port)
}

func (s *Server) rememberProxyHost(raw string) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return
	}
	key := proxyHostKey(u)
	s.mu.Lock()
	if s.allowedProxyHosts == nil {
		s.allowedProxyHosts = make(map[string]struct{})
	}
	s.allowedProxyHosts[key] = struct{}{}
	s.mu.Unlock()
}

func (s *Server) isAllowedProxyHost(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return false
	}
	key := proxyHostKey(u)
	s.mu.Lock()
	_, ok := s.allowedProxyHosts[key]
	s.mu.Unlock()
	return ok
}

func globalProxy(_ *http.Request) (*url.URL, error) {
	raw := strings.TrimSpace(netx.GlobalProxy())
	if raw == "" {
		return nil, nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Hostname() == "" {
		if err == nil {
			err = fmt.Errorf("invalid proxy URL")
		}
		return nil, err
	}
	return u, nil
}

func (s *Server) isSafeProxyURL(raw string) bool {
	return isSafeProxyURLWithAllow(raw, s.isAllowedProxyHost)
}

func isSafeProxyURLWithAllow(raw string, allow func(string) bool) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host := u.Hostname()
	if host == "" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return allow != nil && allow(raw)
		}
		return true
	}
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".local") ||
		strings.HasSuffix(lower, ".internal") || strings.HasSuffix(lower, ".localhost") {
		return allow != nil && allow(raw)
	}
	return true
}

func isLocalOrigin(raw string) bool {
	if raw == "null" {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "localhost" || host == "wails.localhost" || host == "127.0.0.1" || host == "::1"
}

// sanitizeDispositionFilename strips characters that are invalid inside a
// Content-Disposition filename quoted-string (RFC 6266). It keeps CJK and
// common punctuation but drops quotes, backslashes and control bytes.
func sanitizeDispositionFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "file"
	}
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		if r == '"' || r == '\\' || r < 0x20 {
			continue
		}
		b.WriteRune(r)
	}
	s := b.String()
	if s == "" {
		return "file"
	}
	return s
}
