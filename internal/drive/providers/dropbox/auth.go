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
	"strings"
	"sync"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

const (
	dbAuthorizeURL = "https://www.dropbox.com/oauth2/authorize"
	dbTokenURL     = "https://api.dropboxapi.com/oauth2/token"
	dbRedirectPath = "/callback"

	builtinAppKey    = "5jcck7diasz0rqy"
	builtinAppSecret = "1n9m04y2zx7bf26"
)

// authPKCE performs the Dropbox OAuth2 PKCE flow.
func authPKCE(ctx context.Context, req drive.AuthRequest) (*model.TokenInfo, error) {
	appKey, appSecret := resolveCredentials(req.Config["app_key"], req.Config["app_secret"], req.Config["dropbox_app_key"], req.Config["dropbox_app_secret"])

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d%s", port, dbRedirectPath)

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

	code, err := waitToken(ctx, ln, state)
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

	cl := netx.NewClient(60 * time.Second)
	var raw struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		TokenType    string `json:"token_type"`
	}
	if err := cl.PostForm(ctx, dbTokenURL, nil, form, &raw); err != nil {
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

func waitToken(ctx context.Context, ln net.Listener, state string) (string, error) {
	type res struct {
		code string
		err  error
	}
	ch := make(chan res, 1)
	var sendOnce sync.Once
	send := func(value res) { sendOnce.Do(func() { ch <- value }) }
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validCallbackRequest(r, ln, dbRedirectPath) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, "forbidden callback")
			return
		}
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != dbRedirectPath {
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

func validCallbackRequest(r *http.Request, ln net.Listener, path string) bool {
	if r == nil || ln == nil || r.URL == nil || r.URL.Path != path {
		return false
	}
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok || addr.Port == 0 {
		return false
	}
	host, port, err := net.SplitHostPort(r.Host)
	if err != nil || host != "127.0.0.1" || port != fmt.Sprint(addr.Port) {
		return false
	}
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(remoteHost)
	return ip != nil && ip.IsLoopback()
}
