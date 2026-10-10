package webdav

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type testUploadTransport func(*http.Request) (*http.Response, error)

func (fn testUploadTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestWebDAVPutNotAbortedByShortControlRequestTimeout(t *testing.T) {
	base, err := url.Parse("https://example.test/dav")
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	client := &Client{
		HTTP: &http.Client{
			Timeout: 15 * time.Millisecond,
			Transport: testUploadTransport(func(req *http.Request) (*http.Response, error) {
				requests++
				_, _ = io.ReadAll(req.Body)
				time.Sleep(70 * time.Millisecond)
				if err := req.Context().Err(); err != nil {
					return nil, err
				}
				return &http.Response{
					StatusCode: 201,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("")),
					Request:    req,
				}, nil
			}),
		},
		Endpoint: base.String(),
		base: base,
	}
	if err := client.Put(context.Background(), "/file.bin", strings.NewReader("hello"), 5); err != nil {
		t.Fatalf("long PUT was terminated by short control timeout: %v", err)
	}
	if requests != 1 {
		t.Fatalf("PUT requests=%d", requests)
	}
}
