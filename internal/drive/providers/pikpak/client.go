// Package pikpak implements the PikPak provider (api-drive.mypikpak.com).
package pikpak

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mnemo-go/internal/netx"
)

const (
	apiHost  = "https://api-drive.mypikpak.com"
	userHost = "https://user.mypikpak.com"
	RootID   = "pikpak_root"

	// 云离线历史可能非常长。使用小页并在页间让出一点时间，避免打开
	// 传输页或轮询进行中任务时一次向 PikPak 拉取超大响应。
	offlineListPageLimit = 100
	offlineListPageDelay = 150 * time.Millisecond

	clientID      = "YUMx5nI8ZU8Ap8pm"
	clientVersion = "2.0.0"
	packageName   = "mypikpak.com"
	userAgent     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:129.0) Gecko/20100101 Firefox/129.0"
	listPageLimit = "50"
)

// captchaSalts mirrors the reference client's salt chain for captcha_sign.
var captchaSalts = []string{
	"C9qPpZLN8ucRTaTiUMWYS9cQvWOE", "+r6CQVxjzJV6LCV", "F", "pFJRC",
	"9WXYIDGrwTCz2OiVlgZa90qpECPD6olt", "/750aCr4lm/Sly/c", "RB+DT/gZCrbV", "",
	"CyLsf7hdkIRxRm215hl", "7xHvLi2tOYP0Y92b", "ZGTXXxu8E/MIWaEDB+Sm/", "1UI3",
	"E7fP5Pfijd+7K+t6Tg/NhuLq0eEUVChpJSkrKxpO", "ihtqpG6FMt65+Xk+tWUH2", "NhXXU9rg4XXdzo7u5o",
}

func md5hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

// captchaSign builds the 1.<md5-chain> signature.
func captchaSign(deviceID, timestamp string) string {
	sign := clientID + clientVersion + packageName + deviceID + timestamp
	for _, salt := range captchaSalts {
		sign = md5hex(sign + salt)
	}
	return "1." + sign
}

// CaptchaMeta is the meta object sent to captcha/init.
type CaptchaMeta struct {
	CaptchaSign   string `json:"captcha_sign"`
	ClientVersion string `json:"client_version"`
	PackageName   string `json:"package_name"`
	Timestamp     string `json:"timestamp"`
	UserID        string `json:"user_id,omitempty"`
	Email         string `json:"email,omitempty"`
	PhoneNumber   string `json:"phone_number,omitempty"`
	Username      string `json:"username,omitempty"`
}

func loginCaptchaMeta(username string) CaptchaMeta {
	u := strings.TrimSpace(username)
	meta := CaptchaMeta{ClientVersion: clientVersion, PackageName: packageName}
	if strings.Contains(u, "@") && strings.Contains(u, ".") {
		meta.Email = u
	} else if isPhone(u) {
		meta.PhoneNumber = strings.ReplaceAll(strings.ReplaceAll(u, " ", ""), "-", "")
	} else {
		meta.Username = u
	}
	return meta
}

func isPhone(s string) bool {
	if len(s) < 6 || len(s) > 18 {
		return false
	}
	for _, r := range s {
		if r == '+' {
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// client is an authenticated PikPak session.
type client struct {
	http        *netx.Client
	accessToken string
	deviceID    string
	accountID   string
}

func newClient(accessToken, deviceID, accountID string) *client {
	return &client{
		http:        netx.NewClient(90 * time.Second),
		accessToken: accessToken,
		deviceID:    deviceID,
		accountID:   strings.TrimSpace(accountID),
	}
}

func (c *client) headers(extra map[string]string) map[string]string {
	h := map[string]string{
		"User-Agent":       userAgent,
		"Accept":           "application/json",
		"Referer":          "https://mypikpak.com/",
		"X-Client-Id":      clientID,
		"X-Device-Id":      c.deviceID,
		"X-Client-Version": clientVersion,
	}
	if c.accessToken != "" {
		h["Authorization"] = "Bearer " + c.accessToken
	}
	for k, v := range extra {
		h[k] = v
	}
	return h
}

// jsonDo performs an authenticated JSON call against the drive API.
func (c *client) jsonDo(ctx context.Context, method, path string, body any, out any, extra map[string]string) error {
	action := method + ":" + path
	err := c.jsonDoOnce(ctx, method, path, body, out, withCaptchaToken(extra, cachedAPICaptchaToken(c, action)))
	if err != nil && isCaptchaError(err) {
		// Exchange the cached token (when present) for a fresh action token and retry once.
		tok, terr := apiCaptchaToken(ctx, c, action, true)
		if terr == nil && tok != "" {
			return c.jsonDoOnce(ctx, method, path, body, out, withCaptchaToken(extra, tok))
		}
	}
	return err
}

// jsonDoOnce performs a single authenticated JSON call without captcha retry.
func (c *client) jsonDoOnce(ctx context.Context, method, path string, body any, out any, extra map[string]string) error {
	target := apiURL(path)
	var reader io.Reader
	if body != nil {
		reader = netx.JSONBody(body)
	}
	resp, err := c.http.Do(ctx, method, target, c.headers(extra), reader)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return parseAPIErrorWithRetry(data, resp.StatusCode, resp.Header.Get("Retry-After"))
	}
	if out == nil {
		return nil
	}
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

// isCaptchaError reports whether err is a PikPak captcha challenge that can be
// retried after acquiring a fresh X-Captcha-Token.
func isCaptchaError(err error) bool {
	if err == nil {
		return false
	}
	var captchaErr *pikpakCaptchaError
	if errors.As(err, &captchaErr) {
		return true
	}
	s := err.Error()
	return strings.Contains(s, "captcha_required") || strings.Contains(s, "captcha_invalid") || strings.Contains(s, "验证失败")
}

func isCaptchaRequiredError(err error) bool {
	var captchaErr *pikpakCaptchaError
	if errors.As(err, &captchaErr) {
		return captchaErr.reason == "captcha_required"
	}
	return strings.Contains(errString(err), "captcha_required")
}

// isLoginCaptchaInvalidError matches the sole login retry condition in
// rclone's pikpakAuthorize: reason captcha_invalid with protocol code 4002.
func isLoginCaptchaInvalidError(err error) bool {
	var captchaErr *pikpakCaptchaError
	return errors.As(err, &captchaErr) && captchaErr.reason == "captcha_invalid" && captchaErr.code == 4002
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

type pikpakCaptchaError struct {
	reason string
	code   int
}

func (e *pikpakCaptchaError) Error() string {
	switch e.reason {
	case "captcha_invalid", "captcha_required":
		return "PikPak captcha validation failed; retry sign-in"
	default:
		return "PikPak captcha validation failed"
	}
}

// get performs a GET with query params.
func (c *client) get(ctx context.Context, path string, q url.Values, out any) error {
	action := http.MethodGet + ":" + path
	err := c.getOnce(ctx, path, q, out, withCaptchaToken(nil, cachedAPICaptchaToken(c, action)))
	if err != nil && isCaptchaError(err) {
		tok, terr := apiCaptchaToken(ctx, c, action, true)
		if terr == nil && tok != "" {
			return c.getOnce(ctx, path, q, out, withCaptchaToken(nil, tok))
		}
	}
	return err
}

func withCaptchaToken(extra map[string]string, token string) map[string]string {
	if token == "" && len(extra) == 0 {
		return nil
	}
	merged := make(map[string]string, len(extra)+1)
	for k, v := range extra {
		merged[k] = v
	}
	if token != "" {
		merged["X-Captcha-Token"] = token
	}
	return merged
}

func (c *client) getOnce(ctx context.Context, path string, q url.Values, out any, extra map[string]string) error {
	target := apiURL(path)
	if q != nil {
		target += "?" + q.Encode()
	}
	resp, err := c.http.Do(ctx, http.MethodGet, target, c.headers(extra), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return parseAPIErrorWithRetry(data, resp.StatusCode, resp.Header.Get("Retry-After"))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

func apiURL(path string) string {
	if strings.HasPrefix(path, "https://") || strings.HasPrefix(path, "http://") {
		return path
	}
	return apiHost + "/" + strings.TrimPrefix(path, "/")
}

const (
	// PikPak sometimes supplies an explicit Retry-After. Preserve a short
	// value instead of turning every response into a 30-second wait; callers
	// can then apply their own UI/process cap without losing server context.
	pikpakDefaultRateLimitSeconds    = 30
	pikpakRiskControlCooldownSeconds = 10 * 60
)

// PikPakAccessProhibitedError marks a provider-side risk-control block. It is
// deliberately distinct from ordinary authentication failures so callers do
// not immediately repeat the same request and extend the block.
type PikPakAccessProhibitedError struct {
	RetryAfterSeconds int
}

func (e *PikPakAccessProhibitedError) Error() string {
	return fmt.Sprintf("PikPak access was prohibited by provider risk control; retry after %d seconds", e.retryAfterSeconds())
}

func (e *PikPakAccessProhibitedError) retryAfterSeconds() int {
	seconds := e.RetryAfterSeconds
	if seconds < pikpakRiskControlCooldownSeconds {
		seconds = pikpakRiskControlCooldownSeconds
	}
	return seconds
}

// RetryAfter keeps provider-side risk control on the affected account only,
// while preventing a quick manual retry from extending the block.
func (e *PikPakAccessProhibitedError) RetryAfter() time.Duration {
	return time.Duration(e.retryAfterSeconds()) * time.Second
}

// PikPakRateLimitError tells the UI how long the provider asks the user to
// wait before trying the request again. The server has returned both 429 and
// provider-specific "too many" payloads, so status alone is not sufficient.
type PikPakRateLimitError struct {
	RetryAfterSeconds int
}

func (e *PikPakRateLimitError) Error() string {
	return fmt.Sprintf("PikPak login requests are rate limited; retry after %d seconds", e.retryAfterSeconds())
}

func (e *PikPakRateLimitError) retryAfterSeconds() int {
	if e != nil && e.RetryAfterSeconds > 0 {
		return e.RetryAfterSeconds
	}
	return pikpakDefaultRateLimitSeconds
}

// RetryAfter exposes the provider-supplied cooldown to generic callers such
// as the account refresh cache. Keeping the typed duration on the error means
// callers do not need to parse localized error text to avoid an early retry.
func (e *PikPakRateLimitError) RetryAfter() time.Duration {
	return time.Duration(e.retryAfterSeconds()) * time.Second
}

func parseAPIError(data []byte, status int) error {
	return parseAPIErrorWithRetry(data, status, "")
}

func parseAPIErrorWithRetry(data []byte, status int, retryAfter string) error {
	var e struct {
		Error            string          `json:"error"`
		ErrorDescription string          `json:"error_description"`
		Message          string          `json:"message"`
		Reason           string          `json:"reason"`
		Code             int             `json:"code"`
		ErrorCode        int             `json:"error_code"`
		RetryAfter       json.RawMessage `json:"retry_after"`
		RetryAfterSecond json.RawMessage `json:"retry_after_seconds"`
	}
	_ = json.Unmarshal(data, &e)
	detail := strings.ToLower(strings.Join([]string{e.Error, e.ErrorDescription, e.Message, e.Reason, string(data)}, " "))
	if status == http.StatusTooManyRequests || e.Code == http.StatusTooManyRequests || e.ErrorCode == http.StatusTooManyRequests ||
		strings.Contains(detail, "too_many") || strings.Contains(detail, "too many") ||
		strings.Contains(detail, "too_frequent") || strings.Contains(detail, "too frequent") || strings.Contains(detail, "request_frequency") ||
		strings.Contains(detail, "too_fluent") || strings.Contains(detail, "too fluent") || strings.Contains(detail, "too fast") ||
		strings.Contains(detail, "rate_limit") || strings.Contains(detail, "rate limited") ||
		strings.Contains(detail, "请求频繁") || strings.Contains(detail, "操作频繁") {
		seconds := 0
		for _, raw := range []json.RawMessage{e.RetryAfter, e.RetryAfterSecond} {
			if value := parseRetryAfterSeconds(raw); value > seconds {
				seconds = value
			}
		}
		if value := parseRetryAfterHeader(retryAfter); value > seconds {
			seconds = value
		}
		if seconds <= 0 {
			seconds = pikpakDefaultRateLimitSeconds
		}
		return &PikPakRateLimitError{RetryAfterSeconds: seconds}
	}
	if strings.Contains(detail, "accessprohibited") || strings.Contains(detail, "access_prohibited") ||
		strings.Contains(detail, "access prohibited") {
		seconds := pikpakRiskControlCooldownSeconds
		for _, raw := range []json.RawMessage{e.RetryAfter, e.RetryAfterSecond} {
			if value := parseRetryAfterSeconds(raw); value > seconds {
				seconds = value
			}
		}
		if value := parseRetryAfterHeader(retryAfter); value > seconds {
			seconds = value
		}
		return &PikPakAccessProhibitedError{RetryAfterSeconds: seconds}
	}
	// 常见错误友好化（对齐旧版 parsePikPakError）
	switch strings.ToLower(e.Error) {
	case "invalid_account_or_password":
		return errors.New("PikPak invalid account or password")
	case "captcha_invalid", "captcha_required":
		code := e.ErrorCode
		if code == 0 {
			code = e.Code
		}
		return &pikpakCaptchaError{reason: strings.ToLower(e.Error), code: code}
	}
	msg := e.ErrorDescription
	if msg == "" {
		msg = e.Message
	}
	if msg == "" {
		msg = e.Error
	}
	if msg == "" {
		msg = fmt.Sprintf("pikpak: http %d", status)
	}
	if e.Reason != "" {
		msg = e.Reason + ": " + msg
	}
	return errors.New(msg)
}

func parseRetryAfterSeconds(raw json.RawMessage) int {
	value := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if value == "" {
		return 0
	}
	seconds, err := strconv.ParseFloat(value, 64)
	if err == nil && seconds > 0 {
		return int(seconds + 0.999999)
	}
	if when, err := http.ParseTime(value); err == nil {
		remaining := time.Until(when)
		if remaining > 0 {
			return int((remaining + time.Second - 1) / time.Second)
		}
	}
	return 0
}

func parseRetryAfterHeader(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	return parseRetryAfterSeconds(json.RawMessage(strconv.Quote(value)))
}

// File is a raw PikPak drive file item.

var _ = netx.DefaultUA
