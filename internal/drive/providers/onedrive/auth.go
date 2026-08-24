package onedrive

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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
	msAuthorizeURL = "https://login.microsoftonline.com/common/oauth2/v2.0/authorize"
	msTokenURL     = "https://login.microsoftonline.com/common/oauth2/v2.0/token"
	redirectPath   = "/"
	redirectURI    = "http://localhost:53682/"
	listenAddress  = "localhost:53682"

	// Keep the same public desktop OAuth application used by the legacy
	// client/rclone. A release may override these through secrets.json.
	builtinClientID     = "b15665d9-eda6-4092-8539-0eec376afd59"
	builtinClientSecret = "qtyfaBBYA403=unZUP40~_#"
)

// authPKCE performs the OneDrive OAuth2 PKCE flow:
//  1. open browser to authorize URL with code_challenge
//  2. local callback listener catches ?code=
//  3. exchange code for tokens at the token endpoint
func authPKCE(ctx context.Context, req drive.AuthRequest) (*model.TokenInfo, error) {
	clientID, clientSecret := resolveCredentials(req.Config["client_id"], req.Config["client_secret"], req.Config["onedrive_client_id"], req.Config["onedrive_client_secret"])

	ln, err := net.Listen("tcp", listenAddress)
	if err != nil {
		return nil, err
	}
	defer ln.Close()

	verifier, err := pkceVerifier()
	if err != nil {
		return nil, err
	}
	challenge := pkceChallenge(verifier)
	state, err := randomToken(16)
	if err != nil {
		return nil, err
	}

	q := url.Values{}
	q.Set("client_id", clientID)
	q.Set("response_type", "code")
	q.Set("redirect_uri", redirectURI)
	q.Set("response_mode", "query")
	q.Set("prompt", "select_account")
	q.Set("scope", oneDriveScope)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	authURL := msAuthorizeURL + "?" + q.Encode()

	if req.Open != nil {
		if err := req.Open(authURL); err != nil {
			return nil, err
		}
	}

	code, err := waitCallback(ctx, ln, state)
	if err != nil {
		return nil, err
	}

	tok, err := exchangeToken(ctx, clientID, clientSecret, code, redirectURI, verifier)
	if err != nil {
		return nil, err
	}
	fetchOneDriveProfile(ctx, tok.AccessToken, tok)
	applyOneDriveIdentity(tok)
	tok.TokenFrom = providerID
	// Keep the actual OAuth client id with the account. Refresh must use the
	// same public application as the authorization-code exchange; a generic
	// marker makes custom/rclone-compatible credentials impossible to refresh.
	tok.DeviceID = clientID
	return tok, nil
}

// applyOneDriveIdentity mirrors the legacy client: account identity comes
// from Microsoft Graph when available, with a stable token fallback so a
// transient profile failure can never produce an empty account key.
func applyOneDriveIdentity(tok *model.TokenInfo) {
	if tok == nil {
		return
	}
	id := strings.TrimSpace(tok.ProviderAccountID)
	if id == "" {
		sum := sha256.Sum256([]byte(tok.RefreshToken + "|" + tok.AccessToken))
		id = hex.EncodeToString(sum[:8])
	}
	tok.ProviderAccountID = id
	tok.UserID = model.BuildUserID(providerID, id)
	if strings.TrimSpace(tok.DefaultDriveID) == "" {
		tok.DefaultDriveID = model.BuildDriveID(providerID, id)
	}
}

func waitCallback(ctx context.Context, ln net.Listener, state string) (string, error) {
	type result struct {
		code string
		err  error
	}
	ch := make(chan result, 1)
	var sendOnce sync.Once
	send := func(value result) { sendOnce.Do(func() { ch <- value }) }
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validCallbackRequest(r, ln, redirectPath) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, "forbidden callback")
			return
		}
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != redirectPath {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		code := r.URL.Query().Get("code")
		st := r.URL.Query().Get("state")
		if st != state {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, "state mismatch")
			send(result{err: errors.New("oauth: state mismatch")})
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
			send(result{err: fmt.Errorf("oauth: %s", firstNonEmpty(oauthErr, detail))})
			return
		}
		if code == "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, "缺少授权 code")
			send(result{err: errors.New("oauth: missing code")})
			return
		}
		_, _ = io.WriteString(w, "登录成功，可以关闭此窗口")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		send(result{code: code})
	})}
	go func() {
		err := server.Serve(ln)
		if err != nil && err != http.ErrServerClosed {
			send(result{err: err})
		}
	}()
	defer server.Close()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case res := <-ch:
		if res.err != nil {
			return "", res.err
		}
		return res.code, nil
	case <-time.After(10 * time.Minute):
		return "", errors.New("oauth: 登录超时")
	}
}

func exchangeToken(ctx context.Context, clientID, clientSecret, code, redirectURI, verifier string) (*model.TokenInfo, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("grant_type", "authorization_code")
	form.Set("code_verifier", verifier)
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}
	form.Set("scope", oneDriveScope)

	cl := netx.NewClient(60 * time.Second)
	var raw struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		TokenType    string `json:"token_type"`
	}
	if err := cl.PostForm(ctx, msTokenURL, nil, form, &raw); err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw.AccessToken) == "" {
		return nil, errors.New("onedrive: token response missing access_token")
	}
	if strings.TrimSpace(raw.RefreshToken) == "" {
		return nil, errors.New("onedrive: token response missing refresh_token")
	}
	if raw.ExpiresIn <= 0 {
		raw.ExpiresIn = 3600
	}
	return &model.TokenInfo{
		TokenFrom:    providerID,
		AccessToken:  raw.AccessToken,
		RefreshToken: raw.RefreshToken,
		ExpiresIn:    raw.ExpiresIn,
		TokenType:    raw.TokenType,
		ExpireTime:   time.Now().Add(time.Duration(raw.ExpiresIn) * time.Second).UTC().Format(time.RFC3339),
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "oauth callback failed"
}

func resolveCredentials(clientID, clientSecret, configuredID, configuredSecret string) (string, string) {
	id := strings.TrimSpace(clientID)
	secret := strings.TrimSpace(clientSecret)
	configuredID = strings.TrimSpace(configuredID)
	configuredSecret = strings.TrimSpace(configuredSecret)
	if id == "" {
		id = configuredID
	}
	if id == "" {
		id = builtinClientID
	}
	// A configured secret belongs only to its configured client id. Passing it
	// to a different per-account/custom id makes refresh fail as
	// invalid_client, which then leaves a stale Graph access token in place.
	if secret == "" && configuredSecret != "" && id == configuredID {
		secret = configuredSecret
	}
	if secret == "" && id == builtinClientID && configuredID == "" {
		secret = builtinClientSecret
	}
	return id, secret
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

func randomToken(n int) (string, error) {
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
	host = strings.Trim(host, "[]")
	if err != nil || !isLoopbackCallbackHost(host) || port != fmt.Sprint(addr.Port) {
		return false
	}
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(remoteHost)
	return ip != nil && ip.IsLoopback()
}

func isLoopbackCallbackHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}
