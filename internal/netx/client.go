// Package netx provides HTTP plumbing shared by every provider: a configurable
// client (UA/referer/proxy/timeout), JSON request helpers, content hashing and
// upload stream helpers.
package netx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultUA mirrors the legacy renderer user agent.
const DefaultUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

// Client is a reusable HTTP client factory.
type Client struct {
	HTTP  *http.Client
	UA    string
	Proxy string // optional http(s) proxy url
}

// TestTransportHook, when set, is used as the transport of every NewClient
// http.Client. It exists so provider integration tests can route real outbound
// requests to local mock servers (never set in production).
var TestTransportHook http.RoundTripper

// globalProxy is the application-wide proxy URL set by SetGlobalProxy. Every
// NewClient call picks it up so providers don't each need proxy plumbing.
var globalProxy string

// SetGlobalProxy configures the proxy used by all subsequently created netx
// clients (and the download engine). An empty string disables proxying.
func SetGlobalProxy(proxyURL string) { globalProxy = proxyURL }

// GlobalProxy returns the currently configured global proxy URL.
func GlobalProxy() string { return globalProxy }

// NewClient builds a client with sane defaults.
func NewClient(timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	c := &Client{HTTP: &http.Client{Timeout: timeout}, UA: DefaultUA}
	if TestTransportHook != nil {
		c.HTTP = &http.Client{Timeout: timeout, Transport: TestTransportHook}
		return c
	}
	if globalProxy != "" {
		return c.WithProxy(globalProxy)
	}
	return c
}

// WithProxy returns a client that routes traffic through proxyURL.
func (c *Client) WithProxy(proxyURL string) *Client {
	clone := *c
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if u, err := url.Parse(proxyURL); err == nil && u.Scheme != "" {
		transport.Proxy = http.ProxyURL(u)
	}
	clone.HTTP = &http.Client{Transport: transport, Timeout: c.HTTP.Timeout}
	clone.Proxy = proxyURL
	return &clone
}

// Req builds an http.Request with the client's UA.
func (c *Client) Req(ctx context.Context, method, rawURL string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UA)
	return req, nil
}

// Do executes a request with the client UA preset.
func (c *Client) Do(ctx context.Context, method, rawURL string, headers map[string]string, body io.Reader) (*http.Response, error) {
	req, err := c.Req(ctx, method, rawURL, body)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return c.HTTP.Do(req)
}

// GetJSON GETs rawURL and decodes a JSON response.
func (c *Client) GetJSON(ctx context.Context, rawURL string, headers map[string]string, out any) error {
	resp, err := c.Do(ctx, http.MethodGet, rawURL, headers, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return DecodeJSON(resp, out)
}

// PostJSON POSTs a JSON body and decodes the JSON response.
func (c *Client) PostJSON(ctx context.Context, rawURL string, headers map[string]string, body any, out any) error {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return err
		}
	}
	req, err := c.Req(ctx, http.MethodPost, rawURL, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return DecodeJSON(resp, out)
}

// PostForm POSTs form data and decodes JSON.
func (c *Client) PostForm(ctx context.Context, rawURL string, headers map[string]string, form url.Values, out any) error {
	req, err := c.Req(ctx, http.MethodPost, rawURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return DecodeJSON(resp, out)
}

// DecodeJSON reads a response into out, tolerating gzip.
func DecodeJSON(resp *http.Response, out any) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("http %d: %s", resp.StatusCode, truncate(body, 500))
	}
	if len(body) == 0 {
		if out == nil {
			return nil
		}
		return fmt.Errorf("empty response body")
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("json decode: %w: %s", err, truncate(body, 300))
	}
	return nil
}

// ReadBody reads and returns the response body (checked status).
func ReadBody(resp *http.Response, max int64) ([]byte, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, max))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return body, fmt.Errorf("http %d: %s", resp.StatusCode, truncate(body, 500))
	}
	return body, nil
}

func truncate(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n]) + "..."
	}
	return string(b)
}

// JSONBody is a convenience wrapper for request bodies.
func JSONBody(v any) io.Reader {
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(v)
	return &buf
}
