package netx

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestProxyTransportIsReused(t *testing.T) {
	const proxyURL = "http://127.0.0.1:7890"
	first := NewClient(time.Second).WithProxy(proxyURL)
	second := NewClient(2 * time.Second).WithProxy(proxyURL)
	if first.HTTP.Transport != second.HTTP.Transport {
		t.Fatal("clients using the same proxy did not share a connection pool")
	}
	if first.HTTP.Timeout == second.HTTP.Timeout {
		t.Fatal("shared transport unexpectedly replaced per-client timeout")
	}
}

func TestClientDoCancelsBlockedTransport(t *testing.T) {
	started := make(chan struct{}, 1)
	previous := TestTransportHook
	TestTransportHook = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		started <- struct{}{}
		<-req.Context().Done()
		return nil, req.Context().Err()
	})
	t.Cleanup(func() { TestTransportHook = previous })

	client := NewClient(time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := client.Do(ctx, http.MethodGet, "http://127.0.0.1:1/blocked", nil, nil)
		result <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("transport did not receive request")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Do error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Do did not return after cancellation")
	}
}

func TestClientDoWithContentLengthKeepsFileUploadOutOfChunkedMode(t *testing.T) {
	path := t.TempDir() + "/payload.bin"
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	previous := TestTransportHook
	TestTransportHook = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.ContentLength != 4 {
			return nil, errors.New("content length was not propagated")
		}
		for _, encoding := range req.TransferEncoding {
			if strings.EqualFold(encoding, "chunked") {
				return nil, errors.New("known-length file upload unexpectedly used chunked encoding")
			}
		}
		body, readErr := io.ReadAll(req.Body)
		if readErr != nil || string(body) != "data" {
			return nil, errors.New("request body changed")
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{}")), Request: req}, nil
	})
	t.Cleanup(func() { TestTransportHook = previous })

	resp, err := NewClient(time.Minute).DoWithContentLength(context.Background(), http.MethodPost, "https://upload.example/content", nil, f, 4)
	if err != nil {
		t.Fatalf("DoWithContentLength() error = %v", err)
	}
	resp.Body.Close()
	if _, err := NewClient(time.Minute).DoWithContentLength(context.Background(), http.MethodPost, "https://upload.example/content", nil, strings.NewReader("x"), -1); err == nil {
		t.Fatal("negative content length unexpectedly succeeded")
	}
}

func TestNormalizeSystemProxy(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "prefer HTTPS entry", raw: "http=127.0.0.1:7890;https=127.0.0.1:7891", want: "http://127.0.0.1:7891"},
		{name: "bare endpoint", raw: "127.0.0.1:7890", want: "http://127.0.0.1:7890"},
		{name: "SOCKS entry", raw: "socks=127.0.0.1:1080", want: "socks5://127.0.0.1:1080"},
		{name: "ignore unsupported entry", raw: "ftp=127.0.0.1:21", want: ""},
		{name: "reject hostless URL", raw: "http://", want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizeSystemProxy(test.raw); got != test.want {
				t.Fatalf("normalizeSystemProxy(%q) = %q, want %q", test.raw, got, test.want)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
