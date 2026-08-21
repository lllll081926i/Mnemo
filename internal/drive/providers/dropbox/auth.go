package dropbox

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

const (
	dbAuthorizeURL         = "https://www.dropbox.com/oauth2/authorize"
	dbTokenURL             = "https://api.dropboxapi.com/oauth2/token"
	dbTokenExchangeTimeout = 20 * time.Second
	// Dropbox applications require the redirect URI to exactly match the URI
	// registered in the app console.  Keep the same fixed loopback endpoint as
	// rclone so the built-in application credentials work without requiring the
	// user to register a random port for every login.
	dbDefaultRedirectURI = "http://localhost:53682/"
	dbDefaultBindHost    = "127.0.0.1"

	builtinAppKey    = "5jcck7diasz0rqy"
	builtinAppSecret = "1n9m04y2zx7bf26"
)

// authPKCE performs the Dropbox OAuth2 PKCE flow.
func authPKCE(ctx context.Context, req drive.AuthRequest) (*model.TokenInfo, error) {
	appKey, appSecret := resolveCredentials(req.Config["app_key"], req.Config["app_secret"], req.Config["dropbox_app_key"], req.Config["dropbox_app_secret"])
	redirect, err := resolveRedirectURI(req.Config)
	if err != nil {
		return nil, err
	}

	ln, err := net.Listen("tcp4", redirect.listenAddr())
	if err != nil {
		return nil, fmt.Errorf("dropbox: 无法监听 OAuth 回调 %s（请关闭占用该端口的程序后重试）: %w", redirect.listenAddr(), err)
	}
	defer ln.Close()
	redirectURI := redirect.raw

	verifier, err := pkceVerifier()
	if err != nil {
		return nil, err
	}
	challenge := pkceChallenge(verifier)
	state, err := randToken(16)
	if err != nil {
		return nil, err
	}

	q := url.Values{}
	q.Set("client_id", appKey)
	q.Set("response_type", "code")
	q.Set("redirect_uri", redirectURI)
	q.Set("token_access_type", "offline")
	if appSecret == "" {
		q.Set("code_challenge", challenge)
		q.Set("code_challenge_method", "S256")
	}
	q.Set("state", state)

	if req.Open != nil {
		if err := req.Open(dbAuthorizeURL + "?" + q.Encode()); err != nil {
			return nil, err
		}
	}

	code, err := waitToken(ctx, ln, state, redirect)
	if err != nil {
		return nil, err
	}

	form := url.Values{}
	form.Set("client_id", appKey)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("grant_type", "authorization_code")
	if appSecret != "" {
		form.Set("client_secret", appSecret)
	} else {
		form.Set("code_verifier", verifier)
	}

	var raw struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		TokenType    string `json:"token_type"`
	}
	if err := exchangeDropboxAuthorizationCode(ctx, form, &raw); err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw.AccessToken) == "" {
		return nil, errors.New("dropbox: token response missing access_token")
	}
	if strings.TrimSpace(raw.RefreshToken) == "" {
		return nil, errors.New("dropbox: token response missing refresh_token")
	}
	if raw.ExpiresIn <= 0 {
		raw.ExpiresIn = 14400
	}
	tok := &model.TokenInfo{
		TokenFrom:    providerID,
		AccessToken:  raw.AccessToken,
		RefreshToken: raw.RefreshToken,
		ExpiresIn:    raw.ExpiresIn,
		TokenType:    raw.TokenType,
		ExpireTime:   time.Now().Add(time.Duration(raw.ExpiresIn) * time.Second).UTC().Format(time.RFC3339),
	}
	fetchDropboxProfile(ctx, tok.AccessToken, tok)
	applyDropboxIdentity(tok)
	tok.DeviceID = appKey
	return tok, nil
}

// exchangeDropboxAuthorizationCode keeps the single-use code exchange short
// and never retries it. A completed local callback and this outbound request
// are separate stages, so a connection failure must not be reported as a
// callback error.
func exchangeDropboxAuthorizationCode(ctx context.Context, form url.Values, out any) error {
	exchangeCtx, cancel := context.WithTimeout(ctx, dbTokenExchangeTimeout)
	defer cancel()
	if err := netx.NewClientWithSystemProxy(dbTokenExchangeTimeout).PostForm(exchangeCtx, dbTokenURL, nil, form, out); err != nil {
		return explainDropboxTokenExchangeError(err)
	}
	return nil
}

func explainDropboxTokenExchangeError(err error) error {
	if err == nil || !isDropboxTokenNetworkFailure(err) {
		return err
	}
	return fmt.Errorf("dropbox: 无法连接授权服务，请检查网络或系统/应用代理: %w", err)
}

func isDropboxTokenNetworkFailure(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var networkErr net.Error
	if errors.As(err, &networkErr) && networkErr.Timeout() {
		return true
	}
	var operationErr *net.OpError
	if !errors.As(err, &operationErr) {
		return false
	}
	switch operationErr.Op {
	case "dial", "read", "write":
		return true
	default:
		return false
	}
}

func resolveCredentials(appKey, appSecret, configuredKey, configuredSecret string) (string, string) {
	key := strings.TrimSpace(appKey)
	secret := strings.TrimSpace(appSecret)
	if key == "" {
		key = strings.TrimSpace(configuredKey)
	}
	if key == "" {
		key = builtinAppKey
	}
	if secret == "" {
		secret = strings.TrimSpace(configuredSecret)
	}
	if secret == "" && key == builtinAppKey && strings.TrimSpace(configuredKey) == "" {
		secret = builtinAppSecret
	}
	return key, secret
}

type redirectSpec struct {
	raw  string
	host string
	port int
	path string
}

func (r redirectSpec) listenAddr() string {
	return net.JoinHostPort(dbDefaultBindHost, strconv.Itoa(r.port))
}

// resolveRedirectURI validates an optional custom loopback redirect URI.  A
// remote host is deliberately rejected: the callback server handles OAuth
// codes and must never be exposed beyond the local machine.
func resolveRedirectURI(config map[string]string) (redirectSpec, error) {
	raw := strings.TrimSpace(config["dropbox_redirect_uri"])
	if raw == "" {
		raw = strings.TrimSpace(config["redirect_uri"])
	}
	if raw == "" {
		raw = dbDefaultRedirectURI
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return redirectSpec{}, fmt.Errorf("dropbox: redirect_uri 必须是无查询参数的本机 http 地址")
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host != "localhost" && host != "127.0.0.1" {
		return redirectSpec{}, fmt.Errorf("dropbox: redirect_uri 只允许 localhost 或 127.0.0.1")
	}
	portText := u.Port()
	if portText == "" {
		return redirectSpec{}, fmt.Errorf("dropbox: redirect_uri 必须显式指定端口")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return redirectSpec{}, fmt.Errorf("dropbox: redirect_uri 端口无效: %q", portText)
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		return redirectSpec{}, fmt.Errorf("dropbox: redirect_uri 路径无效")
	}
	return redirectSpec{raw: raw, host: host, port: port, path: path}, nil
}

func waitToken(ctx context.Context, ln net.Listener, state string, redirect redirectSpec) (string, error) {
	type res struct {
		code string
		err  error
	}
	ch := make(chan res, 1)
	var sendOnce sync.Once
	send := func(value res) { sendOnce.Do(func() { ch <- value }) }
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validCallbackRequest(r, ln, redirect) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, "forbidden callback")
			return
		}
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.URL.EscapedPath() != redirect.path {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		code := r.URL.Query().Get("code")
		st := r.URL.Query().Get("state")
		if st != state {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, "state mismatch")
			send(res{err: errors.New("oauth: state mismatch")})
			return
		}
		if oauthErr := r.URL.Query().Get("error"); oauthErr != "" {
			detail := r.URL.Query().Get("error_description")
			w.WriteHeader(http.StatusBadRequest)
			if detail != "" {
				_, _ = io.WriteString(w, "授权失败："+detail)
			} else {
				_, _ = io.WriteString(w, "授权失败："+oauthErr)
			}
			send(res{err: fmt.Errorf("oauth: %s", firstNonEmpty(oauthErr, detail))})
			return
		}
		if code == "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, "缺少授权 code")
			send(res{err: errors.New("oauth: missing code")})
			return
		}
		_, _ = io.WriteString(w, "登录成功，可以关闭此窗口")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		send(res{code: code})
	})}
	go func() {
		err := srv.Serve(ln)
		if err != nil && err != http.ErrServerClosed {
			send(res{err: err})
		}
	}()
	defer srv.Close()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case r := <-ch:
		if r.err != nil {
			return "", r.err
		}
		return r.code, nil
	case <-time.After(10 * time.Minute):
		return "", errors.New("oauth: 登录超时")
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "oauth callback failed"
}

func pkceVerifier() (string, error) {
	b := make([]byte, 48)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func validCallbackRequest(r *http.Request, ln net.Listener, redirect redirectSpec) bool {
	if r == nil || ln == nil || r.URL == nil || r.URL.EscapedPath() != redirect.path {
		return false
	}
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok || addr.Port == 0 || addr.Port != redirect.port {
		return false
	}
	host, port, err := net.SplitHostPort(r.Host)
	if err != nil || strings.ToLower(host) != redirect.host || port != strconv.Itoa(redirect.port) {
		return false
	}
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(remoteHost)
	return ip != nil && ip.IsLoopback()
}
