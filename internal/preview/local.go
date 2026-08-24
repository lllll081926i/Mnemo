package preview

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type localFileGrant struct {
	path     string
	lastUsed time.Time
}

const localFileGrantTTL = 12 * time.Hour
const maxLocalFileGrants = 1024

func (s *Server) LocalURL(path string) string {
	resolved, err := canonicalPath(path)
	if err != nil || !s.isWithinRoots(resolved) {
		return ""
	}
	info, err := os.Stat(resolved)
	if err != nil || info.IsDir() {
		return ""
	}
	idBytes := make([]byte, 24)
	if _, err := rand.Read(idBytes); err != nil {
		return ""
	}
	id := base64.RawURLEncoding.EncodeToString(idBytes)
	now := time.Now()
	s.localFilesMu.Lock()
	for key, grant := range s.localFiles {
		if now.Sub(grant.lastUsed) > localFileGrantTTL {
			delete(s.localFiles, key)
		}
	}
	for len(s.localFiles) >= maxLocalFileGrants {
		oldestID := ""
		var oldest time.Time
		for key, grant := range s.localFiles {
			if oldestID == "" || grant.lastUsed.Before(oldest) {
				oldestID, oldest = key, grant.lastUsed
			}
		}
		if oldestID == "" {
			break
		}
		delete(s.localFiles, oldestID)
	}
	s.localFiles[id] = localFileGrant{path: resolved, lastUsed: now}
	s.localFilesMu.Unlock()
	q := url.Values{}
	q.Set("t", s.token)
	return fmt.Sprintf("%s/local/%s?%s", s.BaseURL(), id, q.Encode())
}

func (s *Server) resolveLocalFile(id string) (string, bool) {
	if id == "" || strings.ContainsAny(id, `/\\`) {
		return "", false
	}
	s.localFilesMu.Lock()
	defer s.localFilesMu.Unlock()
	grant, ok := s.localFiles[id]
	if !ok {
		return "", false
	}
	if time.Since(grant.lastUsed) > localFileGrantTTL {
		delete(s.localFiles, id)
		return "", false
	}
	grant.lastUsed = time.Now()
	s.localFiles[id] = grant
	return grant.path, true
}

// validToken checks the session token.

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
	id := strings.TrimPrefix(r.URL.Path, "/local/")
	path, ok := s.resolveLocalFile(id)
	if !ok {
		http.Error(w, "local file grant not found", http.StatusNotFound)
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
