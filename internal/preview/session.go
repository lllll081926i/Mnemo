package preview

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"mnemo-go/internal/model"
)

type PlaybackSource struct {
	URL                 string
	Headers             map[string]string
	RequestAuth         model.RequestAuthenticator
	AllowPrivateNetwork bool
	Filename            string
	StreamType          string
	ExpiresAt           time.Time
	Refresh             func(context.Context) (PlaybackSource, error)
}

type playbackSession struct {
	mu              sync.Mutex
	source          PlaybackSource
	lastUsed        time.Time
	resources       map[string]string
	resourceIDs     map[string]string
	dashResources   map[string]dashPlaybackResource
	dashResourceIDs map[string]string
	resourceOrder   []playbackResourceRef
	resourceSeq     uint64
}

type dashPlaybackResource struct {
	BaseURL       string
	RawQuery      string
	QueryBindings []dashQueryBinding
}

type dashQueryBinding struct {
	Token    string
	LocalKey string
}

type playbackResourceRef struct {
	ID   string
	Dash bool
}

const playbackSessionTTL = 12 * time.Hour
const maxPlaybackResources = 2048

// dashTokenParam keeps the local stream token separate from an upstream DASH
// URL's query string. Signed query strings stay in the Go-side session and
// are never copied into browser-visible segment URLs.
const dashTokenParam = "_mnemo_stream_token"

func (s *Server) PlaybackURL(source PlaybackSource) (string, error) {
	source = clonePlaybackSource(source)
	if strings.TrimSpace(source.URL) == "" {
		return "", fmt.Errorf("empty playback source")
	}
	// Provider URLs are registered by the Go side. Remembering the initial host
	// also permits local test/provider endpoints while redirects are still
	// checked by checkProxyRedirect.
	s.rememberProxyHost(source.URL, source.AllowPrivateNetwork)
	if !s.isSafeProxyURL(source.URL) {
		return "", fmt.Errorf("playback source host is not allowed")
	}
	idBytes := make([]byte, 24)
	if _, err := rand.Read(idBytes); err != nil {
		return "", fmt.Errorf("create playback session: %w", err)
	}
	id := base64.RawURLEncoding.EncodeToString(idBytes)
	now := time.Now()
	s.sessionsMu.Lock()
	for key, session := range s.sessions {
		session.mu.Lock()
		stale := now.Sub(session.lastUsed) > playbackSessionTTL
		session.mu.Unlock()
		if stale {
			delete(s.sessions, key)
		}
	}
	s.sessions[id] = &playbackSession{
		source:          source,
		lastUsed:        now,
		resources:       make(map[string]string),
		resourceIDs:     make(map[string]string),
		dashResources:   make(map[string]dashPlaybackResource),
		dashResourceIDs: make(map[string]string),
	}
	s.sessionsMu.Unlock()
	return fmt.Sprintf("%s/stream/%s?t=%s", s.BaseURL(), id, url.QueryEscape(s.token)), nil
}

func clonePlaybackSource(source PlaybackSource) PlaybackSource {
	if len(source.Headers) > 0 {
		source.Headers = cloneHeaderMap(source.Headers)
	}
	return source
}

func cloneHeaderMap(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for key, value := range headers {
		out[key] = value
	}
	return out
}

func (s *Server) getPlaybackSession(id string) *playbackSession {
	s.sessionsMu.Lock()
	session := s.sessions[id]
	s.sessionsMu.Unlock()
	if session == nil {
		return nil
	}
	session.mu.Lock()
	if time.Since(session.lastUsed) > playbackSessionTTL {
		session.mu.Unlock()
		s.sessionsMu.Lock()
		if s.sessions[id] == session {
			delete(s.sessions, id)
		}
		s.sessionsMu.Unlock()
		return nil
	}
	session.lastUsed = time.Now()
	session.mu.Unlock()
	return session
}

func (session *playbackSession) resolve(ctx context.Context, forceRefresh bool) (PlaybackSource, error) {
	session.mu.Lock()
	defer session.mu.Unlock()
	now := time.Now()
	needsRefresh := forceRefresh
	if !needsRefresh && !session.source.ExpiresAt.IsZero() {
		needsRefresh = !now.Before(session.source.ExpiresAt.Add(-30 * time.Second))
	}
	if needsRefresh && session.source.Refresh != nil {
		refresh := session.source.Refresh
		fresh, err := refresh(ctx)
		if err != nil {
			return PlaybackSource{}, err
		}
		fresh = clonePlaybackSource(fresh)
		if strings.TrimSpace(fresh.URL) == "" {
			return PlaybackSource{}, fmt.Errorf("refreshed playback source is empty")
		}
		if fresh.Refresh == nil {
			fresh.Refresh = refresh
		}
		session.source = fresh
	}
	session.lastUsed = now
	return clonePlaybackSource(session.source), nil
}

func (s *Server) playbackResourceURL(sessionID string, session *playbackSession, target string) string {
	session.mu.Lock()
	if id, ok := session.resourceIDs[target]; ok {
		session.lastUsed = time.Now()
		session.mu.Unlock()
		return fmt.Sprintf("%s/stream/%s/%s?t=%s", s.BaseURL(), sessionID, id, url.QueryEscape(s.token))
	}
	session.resourceSeq++
	id := strconv.FormatUint(session.resourceSeq, 36)
	session.resources[id] = target
	session.resourceIDs[target] = id
	session.resourceOrder = append(session.resourceOrder, playbackResourceRef{ID: id})
	session.trimResourcesLocked()
	session.lastUsed = time.Now()
	session.mu.Unlock()
	return fmt.Sprintf("%s/stream/%s/%s?t=%s", s.BaseURL(), sessionID, id, url.QueryEscape(s.token))
}

func (session *playbackSession) dashResourceID(baseURL, rawQuery string) (string, dashPlaybackResource) {
	key := baseURL + "\x00" + rawQuery
	session.mu.Lock()
	if id, ok := session.dashResourceIDs[key]; ok {
		resource := session.dashResources[id]
		session.lastUsed = time.Now()
		session.mu.Unlock()
		return id, resource
	}
	session.resourceSeq++
	id := strconv.FormatUint(session.resourceSeq, 36)
	resource := dashPlaybackResource{BaseURL: baseURL, RawQuery: rawQuery, QueryBindings: dashQueryBindings(rawQuery)}
	session.dashResources[id] = resource
	session.dashResourceIDs[key] = id
	session.resourceOrder = append(session.resourceOrder, playbackResourceRef{ID: id, Dash: true})
	session.trimResourcesLocked()
	session.lastUsed = time.Now()
	session.mu.Unlock()
	return id, resource
}

func (session *playbackSession) trimResourcesLocked() {
	for len(session.resourceOrder) > maxPlaybackResources {
		oldest := session.resourceOrder[0]
		session.resourceOrder = session.resourceOrder[1:]
		if oldest.Dash {
			if source, ok := session.dashResources[oldest.ID]; ok {
				delete(session.dashResources, oldest.ID)
				delete(session.dashResourceIDs, source.BaseURL+"\x00"+source.RawQuery)
			}
			continue
		}
		if target, ok := session.resources[oldest.ID]; ok {
			delete(session.resources, oldest.ID)
			delete(session.resourceIDs, target)
		}
	}
}

// dashResourceURL maps one concrete DASH resource URL to a local route while
// keeping the dynamic path suffix visible to dash.js. Segment templates such
// as chunk-$Number$.m4s are expanded by the browser after the MPD is parsed,
// so storing only a full opaque target (as HLS does) is not sufficient here.
func (s *Server) dashResourceURL(sessionID string, session *playbackSession, target string) (string, error) {
	targetURL, err := url.Parse(target)
	if err != nil || (targetURL.Scheme != "http" && targetURL.Scheme != "https") || targetURL.Hostname() == "" {
		return "", fmt.Errorf("invalid DASH resource URL")
	}
	if !s.isSafeProxyURL(target) {
		return "", fmt.Errorf("DASH resource host is not allowed")
	}
	baseURL := (&url.URL{
		Scheme: targetURL.Scheme,
		Host:   targetURL.Host,
		User:   targetURL.User,
	}).String()
	resourceID, resource := session.dashResourceID(baseURL, targetURL.RawQuery)
	path := targetURL.EscapedPath()
	if path == "" {
		path = "/"
	}
	query := dashTokenParam + "=" + url.QueryEscape(s.token)
	for _, binding := range resource.QueryBindings {
		// Leave DASH placeholders literal so dash.js can substitute them in the
		// local URL without ever receiving the signed provider query string.
		query += "&" + binding.LocalKey + "=" + binding.Token
	}
	return fmt.Sprintf("%s/stream/%s/r/%s%s?%s", s.BaseURL(), sessionID, resourceID, path, query), nil
}

// dashResourceTarget reconstructs an upstream target from a local /r/ route.
// The stored URL is origin-only; the MPD-controlled suffix cannot replace the
// origin, which preserves the proxy's SSRF boundary.
func (s *Server) dashResourceTarget(session *playbackSession, resourceID, escapedSuffix string, query url.Values) (string, bool) {
	if !strings.HasPrefix(escapedSuffix, "/") || strings.HasPrefix(escapedSuffix, "//") {
		return "", false
	}
	resource, ok := session.dashResource(resourceID)
	if !ok {
		return "", false
	}
	baseURL, err := url.Parse(resource.BaseURL)
	if err != nil || baseURL.Hostname() == "" {
		return "", false
	}
	reference, err := url.Parse(escapedSuffix)
	if err != nil || reference.IsAbs() || reference.Host != "" {
		return "", false
	}
	rawQuery, ok := resolveDashQuery(resource, query)
	if !ok {
		return "", false
	}
	reference.RawQuery = rawQuery
	target := baseURL.ResolveReference(reference).String()
	return target, s.isSafeProxyURL(target)
}

func dashQueryBindings(rawQuery string) []dashQueryBinding {
	if rawQuery == "" {
		return nil
	}
	seen := make(map[string]struct{})
	bindings := make([]dashQueryBinding, 0, 2)
	for offset := 0; offset < len(rawQuery); {
		startOffset := strings.IndexByte(rawQuery[offset:], '$')
		if startOffset < 0 {
			break
		}
		start := offset + startOffset
		endOffset := strings.IndexByte(rawQuery[start+1:], '$')
		if endOffset < 0 {
			break
		}
		end := start + endOffset + 2
		token := rawQuery[start:end]
		offset = end
		if !isDASHTemplateToken(token) {
			continue
		}
		if _, exists := seen[token]; exists {
			continue
		}
		seen[token] = struct{}{}
		bindings = append(bindings, dashQueryBinding{Token: token, LocalKey: fmt.Sprintf("_mnemo_dash_%d", len(bindings))})
	}
	return bindings
}

func isDASHTemplateToken(token string) bool {
	if len(token) < 3 || token[0] != '$' || token[len(token)-1] != '$' {
		return false
	}
	name := token[1 : len(token)-1]
	for _, prefix := range []string{"Number", "Time", "RepresentationID", "Bandwidth", "SubNumber"} {
		if name == prefix || strings.HasPrefix(name, prefix+"%") {
			return true
		}
	}
	return false
}

func resolveDashQuery(resource dashPlaybackResource, query url.Values) (string, bool) {
	rawQuery := resource.RawQuery
	for _, binding := range resource.QueryBindings {
		values, ok := query[binding.LocalKey]
		if !ok || len(values) == 0 {
			return "", false
		}
		rawQuery = strings.ReplaceAll(rawQuery, binding.Token, url.QueryEscape(values[0]))
	}
	return rawQuery, true
}

func (session *playbackSession) dashResource(id string) (dashPlaybackResource, bool) {
	session.mu.Lock()
	defer session.mu.Unlock()
	resource, ok := session.dashResources[id]
	if ok {
		session.lastUsed = time.Now()
	}
	return resource, ok
}

func (session *playbackSession) resource(id string) (string, bool) {
	session.mu.Lock()
	defer session.mu.Unlock()
	target, ok := session.resources[id]
	if ok {
		session.lastUsed = time.Now()
	}
	return target, ok
}

// LocalURL registers one exact local file and returns an opaque local URL.
// Invalid, missing, directory, or out-of-root paths are never registered.
