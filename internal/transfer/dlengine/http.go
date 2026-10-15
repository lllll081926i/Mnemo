package dlengine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrReadIdleTimeout error = readIdleTimeoutError{}

type readIdleTimeoutError struct{}

func (readIdleTimeoutError) Error() string   { return "dlengine: 网络读取空闲超时" }
func (readIdleTimeoutError) Timeout() bool   { return true }
func (readIdleTimeoutError) Temporary() bool { return true }

type retryAuthKey struct{}

type retryAuth struct {
	scheme string
	host   string
	auth   func(*http.Request) error
}

func NewRequest(ctx context.Context, method, address string, opts Options) (*http.Request, error) {
	opts.Normalize()
	request, err := http.NewRequestWithContext(ctx, method, address, nil)
	if err != nil {
		return nil, err
	}
	if err := setRequestHeaders(request, opts); err != nil {
		return nil, err
	}
	return request, nil
}

func NewHTTPClient(opts Options) *http.Client {
	opts.Normalize()
	return &http.Client{
		Transport: &downloadTransport{idleTimeout: opts.ReadIdleTimeout, base: &http.Transport{
			Proxy: proxyFunc(),
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ResponseHeaderTimeout: 60 * time.Second,
			TLSHandshakeTimeout:   30 * time.Second,
			MaxIdleConns:          64,
			MaxIdleConnsPerHost:   32,
			IdleConnTimeout:       90 * time.Second,
			ForceAttemptHTTP2:     true,
			DisableCompression:    true,
		}},
	}
}

type httpStatusError struct {
	stage      string
	status     int
	retryAfter time.Duration
}

func (err *httpStatusError) HTTPStatusCode() int { return err.status }
func (err *httpStatusError) RetryAfter() time.Duration {
	return err.retryAfter
}

func responseStatusError(stage string, response *http.Response) error {
	return &httpStatusError{stage: stage, status: response.StatusCode, retryAfter: retryAfter(response.Header.Get("Retry-After"), time.Now())}
}

func (err *httpStatusError) Error() string {
	return fmt.Sprintf("dlengine: %s http %d", err.stage, err.status)
}

type downloadTransport struct {
	base        http.RoundTripper
	idleTimeout time.Duration
	mu          sync.Mutex
	notBefore   time.Time
}

func (transport *downloadTransport) CloseIdleConnections() {
	if closer, ok := transport.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

func (transport *downloadTransport) wait(ctx context.Context) error {
	for {
		transport.mu.Lock()
		delay := time.Until(transport.notBefore)
		transport.mu.Unlock()
		if delay <= 0 {
			return ctx.Err()
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (transport *downloadTransport) deferRequests(delay time.Duration) {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if next := time.Now().Add(delay); next.After(transport.notBefore) {
		transport.notBefore = next
	}
}

func retryAfter(header string, now time.Time) time.Duration {
	if seconds, err := strconv.ParseInt(strings.TrimSpace(header), 10, 64); err == nil && seconds >= 0 {
		if seconds > 120 {
			return 121 * time.Second
		}
		return time.Duration(seconds) * time.Second
	}
	if deadline, err := http.ParseTime(header); err == nil && deadline.After(now) {
		return deadline.Sub(now)
	}
	return 0
}

func transientStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500 && status <= 599
}

func (transport *downloadTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for attempt := 0; attempt < MaxRetriesPerChunk; attempt++ {
		if err := transport.wait(req.Context()); err != nil {
			return nil, err
		}
		ctx, cancel := context.WithCancelCause(req.Context())
		attemptRequest := req.Clone(ctx)
		if auth, ok := req.Context().Value(retryAuthKey{}).(retryAuth); attempt > 0 && ok && auth.host == req.URL.Host && auth.scheme == req.URL.Scheme {
			if err := auth.auth(attemptRequest); err != nil {
				cancel(nil)
				return nil, err
			}
		}
		response, err := transport.base.RoundTrip(attemptRequest)
		if err != nil {
			cancel(nil)
			return nil, err
		}
		delay := retryAfter(response.Header.Get("Retry-After"), time.Now())
		if req.Method != http.MethodGet || !transientStatus(response.StatusCode) || attempt+1 == MaxRetriesPerChunk || delay > 120*time.Second {
			response.Body = &idleBody{body: response.Body, ctx: ctx, cancel: cancel, timeout: transport.idleTimeout}
			return response, nil
		}
		response.Body.Close()
		cancel(nil)
		backoff := time.Duration(1<<attempt) * 500 * time.Millisecond
		if delay < backoff {
			delay = backoff
		}
		transport.deferRequests(delay)
	}
	return nil, errors.New("dlengine: HTTP 重试次数耗尽")
}

type idleBody struct {
	body       io.ReadCloser
	ctx        context.Context
	cancel     context.CancelCauseFunc
	timeout    time.Duration
	mu         sync.Mutex
	generation uint64
	reading    bool
}

func (body *idleBody) Read(buffer []byte) (int, error) {
	body.mu.Lock()
	body.generation++
	generation := body.generation
	body.reading = true
	body.mu.Unlock()
	timer := time.AfterFunc(body.timeout, func() {
		body.mu.Lock()
		defer body.mu.Unlock()
		if body.reading && body.generation == generation {
			body.cancel(ErrReadIdleTimeout)
		}
	})
	count, err := body.body.Read(buffer)
	body.mu.Lock()
	body.reading = false
	timer.Stop()
	body.mu.Unlock()
	if errors.Is(context.Cause(body.ctx), ErrReadIdleTimeout) {
		err = ErrReadIdleTimeout
	}
	return count, err
}

func (body *idleBody) Close() error {
	body.cancel(nil)
	return body.body.Close()
}
