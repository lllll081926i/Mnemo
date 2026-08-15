// Package preview hosts the internal local HTTP server used by the frontend
// for media playback and downloads that require headers / local proxying.
//
// Routes:
//
//	GET /proxy/<url>?headers=<base64json>   streaming proxy (Range supported)
//	GET /local/<path>                        local file serving (Range)
package preview

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Server is the internal media proxy.
type Server struct {
	ln     net.Listener
	server *http.Server
	Port   int
}

// NewServer starts the internal HTTP server on a random port.
func NewServer() (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &Server{ln: ln, Port: ln.Addr().(*net.TCPAddr).Port}
	mux := http.NewServeMux()
	mux.HandleFunc("/proxy/", s.handleProxy)
	mux.HandleFunc("/local/", s.handleLocal)
	s.server = &http.Server{Handler: mux, ReadHeaderTimeout: 30 * time.Second}
	go func() { _ = s.server.Serve(ln) }()
	return s, nil
}

// BaseURL returns http://127.0.0.1:port.
func (s *Server) BaseURL() string { return fmt.Sprintf("http://127.0.0.1:%d", s.Port) }

// ProxyURL builds a proxied media URL for a remote stream.
func (s *Server) ProxyURL(target string, headers map[string]string) string {
	hdrs, _ := json.Marshal(headers)
	q := url.Values{}
	q.Set("u", target)
	q.Set("h", base64.StdEncoding.EncodeToString(hdrs))
	return fmt.Sprintf("%s/proxy/?%s", s.BaseURL(), q.Encode())
}

// LocalURL builds a URL for a local file.
func (s *Server) LocalURL(path string) string {
	return fmt.Sprintf("%s/local/?p=%s", s.BaseURL(), url.QueryEscape(path))
}

func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	// CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	target := r.URL.Query().Get("u")
	if target == "" {
		http.Error(w, "missing url", http.StatusBadRequest)
		return
	}
	var headers map[string]string
	if h := r.URL.Query().Get("h"); h != "" {
		if b, err := base64.StdEncoding.DecodeString(h); err == nil {
			_ = json.Unmarshal(b, &headers)
		}
	}
	proxyRequest(w, r, target, headers)
}

func (s *Server) handleLocal(w http.ResponseWriter, r *http.Request) {
	// CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	path := r.URL.Query().Get("p")
	if path == "" {
		http.Error(w, "missing path", http.StatusBadRequest)
		return
	}
	// validate: local files only reachable via explicit paths (no traversal)
	clean := strings.TrimPrefix(path, "/")
	if strings.Contains(clean, "..") {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	f, err := os.Open(clean)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, _ := f.Stat()
	w.Header().Set("Content-Type", contentTypeFor(clean))
	http.ServeContent(w, r, clean, info.ModTime(), f)
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
	client := &http.Client{Timeout: 0}
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
	// ensure CORS on proxied response
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
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

var _ = strconv.Itoa
