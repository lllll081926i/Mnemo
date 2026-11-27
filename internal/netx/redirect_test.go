package netx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSafeCheckRedirectBlocksHTTPSDowngrade(t *testing.T) {
	prev, _ := http.NewRequest(http.MethodGet, "https://drive.example.com/file", nil)
	next, _ := http.NewRequest(http.MethodGet, "http://cdn.example.com/file", nil)
	err := SafeCheckRedirect(next, []*http.Request{prev})
	if !errors.Is(err, ErrHTTPSDowngrade) {
		t.Fatalf("expected ErrHTTPSDowngrade, got %v", err)
	}
}

func TestSafeCheckRedirectStripsAuthOnCrossHost(t *testing.T) {
	var seenAuth, seenCookie, seenToken string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		seenCookie = r.Header.Get("Cookie")
		seenToken = r.Header.Get("X-Auth-Token")
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/asset", http.StatusFound)
	}))
	defer source.Close()

	client := NewClient(0)
	resp, err := client.Do(t.Context(), http.MethodGet, source.URL+"/start", map[string]string{
		"Authorization": "Bearer secret",
		"Cookie":        "sid=abc",
		"X-Auth-Token":  "tok",
	}, nil)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if seenAuth != "" || seenCookie != "" || seenToken != "" {
		t.Fatalf("expected sensitive headers stripped, got auth=%q cookie=%q token=%q", seenAuth, seenCookie, seenToken)
	}
}

func TestSafeCheckRedirectKeepsAuthOnSameHost(t *testing.T) {
	prev, _ := http.NewRequest(http.MethodGet, "https://drive.example.com/a", nil)
	next, _ := http.NewRequest(http.MethodGet, "https://drive.example.com/b", nil)
	next.Header.Set("Authorization", "Bearer keep-me")
	if err := SafeCheckRedirect(next, []*http.Request{prev}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := next.Header.Get("Authorization"); got != "Bearer keep-me" {
		t.Fatalf("expected auth kept on same host, got %q", got)
	}
}
