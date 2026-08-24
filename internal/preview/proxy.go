package preview

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"mnemo-go/internal/model"
)

func (s *Server) ProxyURL(target string, headers map[string]string, filename string) string {
	s.rememberProxyHost(target, false)
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

// PlaybackURL registers a source-backed stream session and returns its local
// URL. Unlike ProxyURL, this does not put the upstream URL or auth headers in
// the browser-visible query string. The session is also the anchor used to
// rewrite HLS playlist references to local proxy routes.

func (s *Server) validToken(r *http.Request) bool {
	query := r.URL.Query()
	return query.Get("t") == s.token || query.Get(dashTokenParam) == s.token
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

// handleStream serves an opaque playback session. The optional path suffix is
// a URL-safe encoded HLS resource; it lets the browser request relative
// segments while the session keeps the provider headers private.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
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
	pathValue := strings.TrimPrefix(r.URL.EscapedPath(), "/stream/")
	parts := strings.Split(pathValue, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "missing playback session", http.StatusBadRequest)
		return
	}
	sessionID := parts[0]
	session := s.getPlaybackSession(sessionID)
	if session == nil {
		http.Error(w, "playback session expired", http.StatusGone)
		return
	}
	resourceTarget := ""
	if len(parts) > 1 {
		if parts[1] == "r" {
			if len(parts) < 3 || parts[2] == "" {
				http.Error(w, "invalid DASH playback resource", http.StatusBadRequest)
				return
			}
			escapedSuffix := "/"
			if len(parts) > 3 {
				escapedSuffix += strings.Join(parts[3:], "/")
			}
			var ok bool
			resourceTarget, ok = s.dashResourceTarget(session, parts[2], escapedSuffix, r.URL.Query())
			if !ok {
				http.Error(w, "invalid DASH playback resource", http.StatusBadRequest)
				return
			}
		} else if len(parts) == 2 && parts[1] != "" {
			var ok bool
			resourceTarget, ok = session.resource(parts[1])
			if !ok {
				http.Error(w, "invalid playback resource", http.StatusBadRequest)
				return
			}
		} else {
			http.Error(w, "invalid playback resource", http.StatusBadRequest)
			return
		}
	}
	s.proxySessionRequest(w, r, sessionID, session, resourceTarget)
}

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

// proxyRequest streams a remote URL with Range passthrough. It is kept for
// non-player previews; video playback uses the opaque /stream/ session route.
func proxyRequest(s *Server, w http.ResponseWriter, r *http.Request, target string, headers map[string]string, filename string) {
	resp, err := s.doProxyRequest(r.Context(), r.Method, target, headers, nil, r.Header.Get("Range"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	copyProxyResponseHeaders(w, resp, filename, false)
	corsHeaders(w, r)
	w.WriteHeader(resp.StatusCode)
	if r.Method != http.MethodHead {
		_, _ = io.Copy(w, resp.Body)
	}
}

func (s *Server) proxySessionRequest(w http.ResponseWriter, r *http.Request, sessionID string, session *playbackSession, resourceTarget string) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	for attempt := 0; attempt < 2; attempt++ {
		source, err := session.resolve(ctx, attempt > 0 && resourceTarget == "")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		target := source.URL
		if resourceTarget != "" {
			target = resourceTarget
		}
		if !s.isSafeProxyURL(target) {
			http.Error(w, "url not allowed", http.StatusBadRequest)
			return
		}
		resp, err := s.doProxyRequest(ctx, r.Method, target, source.Headers, source.RequestAuth, r.Header.Get("Range"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		if (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) && attempt == 0 && source.Refresh != nil {
			resp.Body.Close()
			// For a stale HLS/DASH segment we cannot assume the new stream uses
			// the same signed path. Refresh the session first; retrying once still
			// covers providers that rotate only headers, and the adaptive client
			// then reloads the fresh root manifest if its old segment remains gone.
			if resourceTarget != "" {
				if _, err := session.resolve(ctx, true); err != nil {
					http.Error(w, err.Error(), http.StatusBadGateway)
					return
				}
			}
			continue
		}
		defer resp.Body.Close()
		isHLS := isHLSPlaylist(source.StreamType, resp.Header.Get("Content-Type"), target, resourceTarget == "")
		isDASH := isDASHManifest(source.StreamType, resp.Header.Get("Content-Type"), target, resourceTarget == "")
		if (isHLS || isDASH) && r.Method != http.MethodHead && resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
			body, readErr := readPlaylistBody(resp)
			if readErr != nil {
				http.Error(w, readErr.Error(), http.StatusBadGateway)
				return
			}
			var rewritten []byte
			var rewriteErr error
			if isHLS {
				rewritten, rewriteErr = s.rewriteHLSPlaylist(sessionID, session, target, body)
			} else {
				rewritten, rewriteErr = s.rewriteDASHManifest(sessionID, session, target, body)
			}
			if rewriteErr != nil {
				http.Error(w, rewriteErr.Error(), http.StatusBadGateway)
				return
			}
			copyProxyResponseHeaders(w, resp, source.Filename, true)
			if isHLS {
				w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			} else {
				w.Header().Set("Content-Type", "application/dash+xml")
			}
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(rewritten)))
			corsHeaders(w, r)
			w.WriteHeader(resp.StatusCode)
			_, _ = w.Write(rewritten)
			return
		}
		if isSRTSubtitle(source.StreamType, resp.Header.Get("Content-Type"), target) && r.Method != http.MethodHead && resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
			body, readErr := readPlaylistBody(resp)
			if readErr != nil {
				http.Error(w, readErr.Error(), http.StatusBadGateway)
				return
			}
			converted := srtToWebVTT(body)
			copyProxyResponseHeaders(w, resp, source.Filename, true)
			w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(converted)))
			corsHeaders(w, r)
			w.WriteHeader(resp.StatusCode)
			_, _ = w.Write(converted)
			return
		}
		copyProxyResponseHeaders(w, resp, source.Filename, false)
		corsHeaders(w, r)
		w.WriteHeader(resp.StatusCode)
		if r.Method != http.MethodHead {
			_, _ = io.Copy(w, resp.Body)
		}
		return
	}
	http.Error(w, "upstream playback URL expired", http.StatusBadGateway)
}

func (s *Server) doProxyRequest(ctx context.Context, method, target string, headers map[string]string, requestAuth model.RequestAuthenticator, byteRange string) (*http.Response, error) {
	if err := s.validateSafeProxyURL(ctx, target); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return nil, err
	}
	for key, value := range filterProxyHeaders(headers) {
		req.Header.Set(key, value)
	}
	if byteRange != "" {
		req.Header.Set("Range", byteRange)
	}
	if requestAuth != nil {
		if err := requestAuth(req); err != nil {
			return nil, err
		}
	}
	return s.proxyClient.Do(req)
}

func copyProxyResponseHeaders(w http.ResponseWriter, resp *http.Response, filename string, omitEncoding bool) {
	for _, header := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "Last-Modified", "ETag", "Cache-Control", "Expires", "Content-Encoding"} {
		if omitEncoding && header == "Content-Encoding" {
			continue
		}
		if value := resp.Header.Get(header); value != "" {
			w.Header().Set(header, value)
		}
	}
	if filename != "" {
		w.Header().Set("Content-Disposition", "inline; filename=\""+sanitizeDispositionFilename(filename)+"\"")
	} else if disposition := resp.Header.Get("Content-Disposition"); disposition != "" {
		w.Header().Set("Content-Disposition", disposition)
	}
}

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
