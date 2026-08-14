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
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

const (
	dbAuthorizeURL = "https://www.dropbox.com/oauth2/authorize"
	dbTokenURL     = "https://api.dropboxapi.com/oauth2/token"
	dbRedirectPath = "/callback"
)

// authPKCE performs the Dropbox OAuth2 PKCE flow.
func authPKCE(ctx context.Context, req drive.AuthRequest) (*model.TokenInfo, error) {
	appKey := strings.TrimSpace(req.Config["app_key"])
	if appKey == "" {
		appKey = strings.TrimSpace(req.Config["dropbox_app_key"])
	}
	if appKey == "" {
		return nil, errors.New("未配置 Dropbox App Key（secrets.json dropbox_app_key）")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d%s", port, dbRedirectPath)

	verifier := pkceVerifier()
	challenge := pkceChallenge(verifier)
	state := randToken(16)

	q := url.Values{}
	q.Set("client_id", appKey)
	q.Set("response_type", "code")
	q.Set("redirect_uri", redirectURI)
	q.Set("token_access_type", "offline")
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	_ = url.QueryEscape

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
	form.Set("code_verifier", verifier)

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
	return &model.TokenInfo{
		TokenFrom:    providerID,
		AccessToken:  raw.AccessToken,
		RefreshToken: raw.RefreshToken,
		ExpiresIn:    raw.ExpiresIn,
		TokenType:    raw.TokenType,
	}, nil
}

func waitToken(ctx context.Context, ln net.Listener, state string) (string, error) {
	type res struct {
		code string
		err  error
	}
	ch := make(chan res, 1)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		st := r.URL.Query().Get("state")
		if st != state {
			ch <- res{err: errors.New("oauth: state mismatch")}
			return
		}
		if code == "" {
			ch <- res{err: errors.New("oauth: missing code")}
			return
		}
		_, _ = io.WriteString(w, "登录成功，可以关闭此窗口")
		ch <- res{code: code}
	})}
	go func() {
		err := srv.Serve(ln)
		if err != nil && err != http.ErrServerClosed {
			select {
			case ch <- res{err: err}:
			default:
			}
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

func pkceVerifier() string {
	b := make([]byte, 48)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}