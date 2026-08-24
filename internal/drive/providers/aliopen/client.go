package aliopen

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

// aliOpenRateLimiter mirrors the legacy provider limiter: at most two
// in-flight API calls and a small spacing between requests to reduce
// wind-control responses from the Open API.
type aliOpenRateLimiter struct {
	mu           sync.Mutex
	last         time.Time
	blockedUntil time.Time
	slots        chan struct{}
	interval     time.Duration
}

func newAliOpenRateLimiter(concurrency int, interval time.Duration) *aliOpenRateLimiter {
	if concurrency < 1 {
		concurrency = 1
	}
	return &aliOpenRateLimiter{
		slots:    make(chan struct{}, concurrency),
		interval: interval,
	}
}

func (r *aliOpenRateLimiter) run(ctx context.Context, fn func() error) error {
	select {
	case r.slots <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-r.slots }()

	for {
		r.mu.Lock()
		now := time.Now()
		wait := r.interval - now.Sub(r.last)
		if blockedWait := r.blockedUntil.Sub(now); blockedWait > wait {
			wait = blockedWait
		}
		if wait <= 0 {
			r.last = now
			r.mu.Unlock()
			return fn()
		}
		r.mu.Unlock()

		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		}
	}
}

func (r *aliOpenRateLimiter) penalize(delay time.Duration) {
	if delay <= 0 {
		return
	}
	r.mu.Lock()
	until := time.Now().Add(delay)
	if until.After(r.blockedUntil) {
		r.blockedUntil = until
	}
	r.mu.Unlock()
}

var aliOpenLimiter = newAliOpenRateLimiter(2, 220*time.Millisecond)

// client is an authenticated aliopen session.
type client struct {
	http    *netx.Client
	session *Session
	token   *model.TokenInfo
}

func (c *client) apiPost(ctx context.Context, path string, body any, out any) error {
	return c.apiPostWith(ctx, path, body, out, nil)
}

// apiPostWith is apiPost with extra headers (e.g. x-share-token).
func (c *client) apiPostWith(ctx context.Context, path string, body any, out any, extraHeaders map[string]string) error {
	return c.apiPostAtWithRetry(ctx, apiHost, path, body, out, extraHeaders, true)
}

// apiPostAt calls a compatible Ali endpoint. It is only used for a narrowly
// scoped fallback when the documented Open endpoint has been retired by an
// account region; auth refresh and throttling remain identical to apiPost.
func (c *client) apiPostAt(ctx context.Context, host, path string, body any, out any) error {
	return c.apiPostAtWithRetry(ctx, host, path, body, out, nil, true)
}

func (c *client) apiPostAtWithRetry(ctx context.Context, host, path string, body any, out any, extraHeaders map[string]string, allowRefresh bool) error {
	host = strings.TrimRight(strings.TrimSpace(host), "/")
	if host == "" {
		return errors.New("aliopen: API 地址为空")
	}
	hdrs := map[string]string{
		"Authorization": "Bearer " + c.session.AccessToken,
		"Content-Type":  "application/json",
		"Accept":        "application/json",
	}
	for k, v := range extraHeaders {
		switch strings.ToLower(k) {
		case "authorization", "content-type", "accept":
			continue
		}
		hdrs[k] = v
	}
	var resp *http.Response
	if err := aliOpenLimiter.run(ctx, func() error {
		var err error
		resp, err = c.http.Do(ctx, http.MethodPost, host+path, hdrs, netx.JSONBody(body))
		return err
	}); err != nil {
		return err
	}
	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return err
	}
	if aliOpenAuthResponse(resp.StatusCode, data) {
		if !allowRefresh {
			return aliOpenRequestErrorOf(data, resp.StatusCode)
		}
		if err := c.refreshToken(ctx); err != nil {
			return err
		}
		return c.apiPostAtWithRetry(ctx, host, path, body, out, extraHeaders, false)
	}
	if aliOpenRateLimitResponse(resp.StatusCode, data) {
		fallback := 5 * time.Second
		if resp.StatusCode == http.StatusTooManyRequests {
			fallback = 5 * time.Second
		}
		aliOpenLimiter.penalize(aliOpenRetryAfter(resp, fallback))
	}
	if resp.StatusCode >= 400 {
		return aliOpenRequestErrorOf(data, resp.StatusCode)
	}
	if aliOpenAPIError(data) {
		return aliOpenRequestErrorOf(data, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

type aliOpenErrorBody struct {
	Code             string `json:"code"`
	Message          string `json:"message"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// aliOpenRequestError keeps HTTP status and the provider error code separate
// from display text. Endpoint fallbacks must be driven by this structured
// information instead of by translated or reformatted error strings.
type aliOpenRequestError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *aliOpenRequestError) Error() string {
	if e == nil {
		return "aliopen: request failed"
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Code != "" {
		return e.Code
	}
	if e.StatusCode > 0 {
		return fmt.Sprintf("aliopen: http %d", e.StatusCode)
	}
	return "aliopen: request failed"
}

func aliOpenErrorBodyOf(data []byte) aliOpenErrorBody {
	var body aliOpenErrorBody
	_ = json.Unmarshal(data, &body)
	return body
}

func aliOpenRequestErrorOf(data []byte, status int) *aliOpenRequestError {
	body := aliOpenErrorBodyOf(data)
	return &aliOpenRequestError{
		StatusCode: status,
		Code:       strings.TrimSpace(body.Code),
		Message:    aliOpenErrorMessage(data, status),
	}
}

func aliOpenAuthResponse(status int, data []byte) bool {
	if status == http.StatusUnauthorized {
		return true
	}
	body := aliOpenErrorBodyOf(data)
	code := strings.ToLower(strings.TrimSpace(body.Code))
	return strings.Contains(code, "accesstoken") ||
		strings.Contains(code, "tokenexpired") ||
		strings.Contains(code, "invalid_token") ||
		code == "unauthorized"
}

func aliOpenRateLimitResponse(status int, data []byte) bool {
	if status == http.StatusTooManyRequests {
		return true
	}
	body := aliOpenErrorBodyOf(data)
	text := strings.ToLower(body.Code + " " + body.Message + " " + body.Error)
	return strings.Contains(text, "limit") ||
		strings.Contains(text, "toomany") ||
		strings.Contains(text, "frequency") ||
		strings.Contains(text, "429")
}

func aliOpenAPIError(data []byte) bool {
	body := aliOpenErrorBodyOf(data)
	return strings.TrimSpace(body.Code) != "" && !strings.EqualFold(strings.TrimSpace(body.Code), "success")
}

func aliOpenErrorMessage(data []byte, status int) string {
	body := aliOpenErrorBodyOf(data)
	for _, msg := range []string{body.Message, body.ErrorDescription, body.Error, body.Code} {
		if strings.TrimSpace(msg) != "" {
			return strings.TrimSpace(msg)
		}
	}
	if status > 0 {
		return fmt.Sprintf("aliopen: http %d", status)
	}
	return "aliopen: request failed"
}

func aliOpenRetryAfter(resp *http.Response, fallback time.Duration) time.Duration {
	if resp != nil {
		if raw := strings.TrimSpace(resp.Header.Get("Retry-After")); raw != "" {
			if seconds, err := strconv.Atoi(raw); err == nil && seconds >= 0 {
				return time.Duration(seconds) * time.Second
			}
		}
	}
	return fallback
}

// listResp is the paginated listing response.
