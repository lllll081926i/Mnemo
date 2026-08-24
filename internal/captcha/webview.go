// Package captcha receives the result of an embedded PikPak challenge through
// a short-lived local HTTP callback server.
//
// PikPak's captcha flow redirects to a callback URL after the user completes
// the slider. The callback carries the captcha_token as a query parameter.
// We start a temporary local HTTP server, rewrite the challenge URL to use
// our callback, and wait for the redirect.
package captcha

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"mnemo-go/internal/logging"
)

// Session identifies one application-owned captcha callback endpoint.
// The random path keeps unrelated local requests from completing a login.
type Session struct {
	ID          string
	CallbackURL string
}

// CompletedFunc receives the token returned after a visual challenge.
// A token can be empty when PikPak only signals completion through its redirect;
// callers can then exchange the original challenge token with the API.
type CompletedFunc func(session Session, token string)

var (
	mu            sync.Mutex
	server        *http.Server
	listener      net.Listener
	onDone        CompletedFunc
	activeSession Session
	completed     bool
)

// Start creates a localhost callback endpoint without opening a browser. The
// caller supplies Session.CallbackURL to PikPak while it initializes a visual
// challenge, so the challenge itself redirects back to this application.
func Start(onComplete CompletedFunc) (*Session, error) {
	mu.Lock()
	defer mu.Unlock()
	session, err := startLocked(onComplete)
	if err != nil {
		logging.Warn("captcha callback server start failed", "error", err)
	} else {
		logging.Debug("captcha callback server started", "session_id", session.ID, "callback_host", "127.0.0.1")
	}
	return session, err
}

func startLocked(onComplete CompletedFunc) (*Session, error) {
	// Stop any previous session: only one login modal can be active at once.
	stopLocked()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("captcha: start callback server: %w", err)
	}
	id, err := newSessionID()
	if err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("captcha: create callback session: %w", err)
	}
	session := Session{
		ID:          id,
		CallbackURL: fmt.Sprintf("http://%s/callback/%s", ln.Addr().String(), id),
	}

	listener = ln
	onDone = onComplete
	activeSession = session
	completed = false

	mux := http.NewServeMux()
	mux.HandleFunc("/callback/"+id, handleCallback)
	mux.HandleFunc("/redirect/"+id, handleRedirect)
	srv := &http.Server{Handler: mux}
	server = srv
	go func() { _ = srv.Serve(ln) }()
	return &session, nil
}

func newSessionID() (string, error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Close stops the callback server.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	stopLocked()
	logging.Debug("captcha callback server stopped")
}

func stopLocked() {
	// Detach first while the session lock is held. Shutdown can wait for an
	// in-flight callback, which may itself need this lock to report completion.
	// Closing the listener rejects new requests immediately; the detached server
	// then drains asynchronously without blocking a replacement session.
	srv := server
	ln := listener
	server = nil
	listener = nil
	onDone = nil
	activeSession = Session{}
	completed = false
	if ln != nil {
		_ = ln.Close()
	}
	if srv != nil {
		go shutdownServer(srv)
	}
}

func shutdownServer(srv *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

// handleCallback receives the redirect from PikPak after captcha completion.
func handleCallback(w http.ResponseWriter, r *http.Request) {
	token := extractToken(r)
	session, accepted := complete(token)
	logging.Info("captcha callback received", "session_id", session.ID, "accepted", accepted, "has_token", token != "")
	// Return a minimal HTML that auto-closes the tab.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!html><body><p>验证完成，可关闭此页面</p><script>setTimeout(()=>window.close(),1000)</script></body></html>`))
	// Shut down the server after a short delay.
	if accepted {
		go func(sessionID string) {
			time.Sleep(2 * time.Second)
			closeSession(sessionID)
		}(session.ID)
	}
}

// handleRedirect is a fallback: the challenge page can post the token via
// a script that navigates to /redirect?#token=...
func handleRedirect(w http.ResponseWriter, r *http.Request) {
	handleCallback(w, r)
}

func complete(token string) (Session, bool) {
	mu.Lock()
	if completed || onDone == nil {
		mu.Unlock()
		logging.Debug("captcha callback ignored", "reason", "no active session or already completed")
		return Session{}, false
	}
	completed = true
	callback := onDone
	session := activeSession
	mu.Unlock()
	go callback(session, token)
	return session, true
}

func closeSession(sessionID string) {
	if sessionID == "" {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if activeSession.ID == sessionID {
		stopLocked()
	}
}

func extractToken(r *http.Request) string {
	if frag := r.URL.Fragment; frag != "" {
		if parsed, err := url.ParseQuery(frag); err == nil {
			for _, key := range []string{"captcha_token", "captchaToken", "token"} {
				if t := normalizeToken(parsed.Get(key)); t != "" {
					return t
				}
			}
		}
	}
	for _, key := range []string{"captcha_token", "captchaToken", "token"} {
		if t := normalizeToken(r.URL.Query().Get(key)); t != "" {
			return t
		}
	}
	return ""
}

func normalizeToken(raw string) string {
	t := strings.TrimSpace(raw)
	if len(t) > 20 {
		return t
	}
	return ""
}
