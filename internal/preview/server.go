// Package preview hosts the internal local HTTP server used by the frontend
// for media playback and downloads that require headers / local proxying.
//
// Security model:
//   - Every /proxy/ and /local/ request must carry a session token (t=...).
//   - /local/ only serves opaque grants under configured download roots.
//   - /proxy/ validates targets and blocks unsafe private-network SSRF.
//   - CORS is restricted to the local Wails/browser origins.
package preview

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

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
	sessionsMu        sync.Mutex
	sessions          map[string]*playbackSession
	localFilesMu      sync.Mutex
	localFiles        map[string]localFileGrant
}

// PlaybackSource is a browser-playable source registered with the local
// proxy. The URL and headers stay in the Go process; only an opaque session
// id is exposed to the WebView. Refresh is called when a provider rejects an
// expired signed URL, allowing an in-flight video request to recover without

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
		sessions:          make(map[string]*playbackSession),
		localFiles:        make(map[string]localFileGrant),
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableCompression = true
	transport.Proxy = globalProxy
	transport.DialContext = s.safeProxyDialContext
	s.proxyTransport = transport
	s.proxyClient = &http.Client{
		Timeout:       0,
		Transport:     transport,
		CheckRedirect: s.checkProxyRedirect,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/proxy/", s.handleProxy)
	mux.HandleFunc("/stream/", s.handleStream)
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

// SetRoots replaces all local preview roots and revokes existing file grants.
// Runtime download-directory changes must not leave previous directories
// permanently readable by the local server.
func (s *Server) SetRoots(dirs ...string) {
	cleanRoots := make([]string, 0, len(dirs))
	seen := make(map[string]bool, len(dirs))
	for _, dir := range dirs {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		abs, err := canonicalPath(dir)
		if err != nil || seen[abs] {
			continue
		}
		seen[abs] = true
		cleanRoots = append(cleanRoots, abs)
	}
	s.mu.Lock()
	s.roots = cleanRoots
	s.mu.Unlock()
	s.localFilesMu.Lock()
	s.localFiles = make(map[string]localFileGrant)
	s.localFilesMu.Unlock()
}

// BaseURL returns http://127.0.0.1:port.
func (s *Server) BaseURL() string { return fmt.Sprintf("http://127.0.0.1:%d", s.Port) }

// ProxyURL builds a proxied media URL for a remote stream. filename is an
// optional display name forwarded to the browser via Content-Disposition so
// the preview shows the real file name instead of a guessed one.

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
