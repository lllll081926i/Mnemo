// Package captcha opens the PikPak challenge in the system default browser
// and receives the verified token via a local HTTP callback server.
//
// PikPak's captcha flow redirects to a callback URL after the user completes
// the slider. The callback carries the captcha_token as a query parameter.
// We start a temporary local HTTP server, rewrite the challenge URL to use
// our callback, and wait for the redirect.
package captcha

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// CompletedFunc receives the token returned after a visual challenge.
type CompletedFunc func(token string)

var (
	mu        sync.Mutex
	server    *http.Server
	listener  net.Listener
	onDone    CompletedFunc
)

// Open launches the system browser at the challenge URL and starts a local
// HTTP server to receive the captcha callback.
func Open(rawURL string, onComplete CompletedFunc) error {
	mu.Lock()
	defer mu.Unlock()

	// stop any previous session
	stopLocked()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("captcha: start callback server: %w", err)
	}
	listener = ln
	onDone = onComplete

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", handleCallback)
	mux.HandleFunc("/redirect", handleRedirect)
	srv := &http.Server{Handler: mux}
	server = srv
	go srv.Serve(ln)

	// The challenge URL points to PikPak's captcha page. After the user
	// completes the slider, PikPak redirects to a callback. We intercept by
	// opening the challenge in the system browser — the redirect will land on
	// our local server if PikPak uses a localhost redirect_uri, or we extract
	// the token from the final URL via the /redirect helper.
	challengeURL := buildChallengeURL(rawURL, ln.Addr().String())

	if err := openBrowser(challengeURL); err != nil {
		stopLocked()
		return fmt.Errorf("captcha: 无法打开浏览器: %w", err)
	}
	return nil
}

// Close stops the callback server.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	stopLocked()
}

func stopLocked() {
	if server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = server.Shutdown(ctx)
		cancel()
		server = nil
	}
	if listener != nil {
		_ = listener.Close()
		listener = nil
	}
	onDone = nil
}

// buildChallengeURL appends our callback as a redirect parameter so PikPak
// knows where to send the token after verification.
func buildChallengeURL(rawURL, callbackAddr string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := parsed.Query()
	// Some flows honor redirect_uri; others use a custom-scheme callback.
	// We set both the standard redirect_uri and our own /redirect endpoint.
	callbackURL := fmt.Sprintf("http://%s/callback", callbackAddr)
	q.Set("redirect_uri", callbackURL)
	parsed.RawQuery = q.Encode()
	return parsed.String()
}

// handleCallback receives the redirect from PikPak after captcha completion.
func handleCallback(w http.ResponseWriter, r *http.Request) {
	token := extractToken(r)
	if token != "" && onDone != nil {
		go onDone(token)
	}
	// Return a minimal HTML that auto-closes the tab.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!html><body><p>验证完成，可关闭此页面</p><script>setTimeout(()=>window.close(),1000)</script></body></html>`))
	// Shut down the server after a short delay.
	go func() {
		time.Sleep(2 * time.Second)
		Close()
	}()
}

// handleRedirect is a fallback: the challenge page can post the token via
// a script that navigates to /redirect?#token=...
func handleRedirect(w http.ResponseWriter, r *http.Request) {
	handleCallback(w, r)
}

func extractToken(r *http.Request) string {
	for _, values := range r.URL.Query() {
		for _, v := range values {
			if t := normalizeToken(v); t != "" {
				return t
			}
		}
	}
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

// openBrowser opens the system default browser at url.
func openBrowser(rawURL string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL).Start()
	case "darwin":
		return exec.Command("open", rawURL).Start()
	default:
		return exec.Command("xdg-open", rawURL).Start()
	}
}

// Unused but kept for JSON parsing of callback bodies.
var _ = json.Decoder{}
