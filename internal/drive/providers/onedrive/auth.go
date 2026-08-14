package onedrive

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
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
	msAuthorizeURL = "https://login.microsoftonline.com/common/oauth2/v2.0/authorize"
	msTokenURL     = "https://login.microsoftonline.com/common/oauth2/v2.0/token"
	redirectPath   = "/callback"
)

// authPKCE performs the OneDrive OAuth2 PKCE flow:
//  1. open browser to authorize URL with code_challenge
//  2. local callback listener catches ?code=
//  3. exchange code for tokens at the token endpoint
func authPKCE(ctx context.Context, req drive.AuthRequest) (*model.TokenInfo, error) {
	clientID := strings.TrimSpace(req.Config["client_id"])
	if clientID == "" {
		clientID = strings.TrimSpace(req.Config["onedrive_client_id"])
	}
	if clientID == "" {
		return nil, errors.New("未配置 OneDrive OAuth 客户端 ID（secrets.json onedrive_client_id）")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d%s", port, redirectPath)

	verifier, err := pkceVerifier()
	if err != nil {
		return nil, err
	}
	challenge := pkceChallenge(verifier)
	state := randomToken(16)

	q := url.Values{}
	q.Set("client_id", clientID)
	q.Set("response_type", "code")
	q.Set("redirect_uri", redirectURI)
	q.Set("response_mode", "query")
	q.Set("scope", "files.readwrite offline_access User.Read")
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

	tok, err := exchangeToken(ctx, clientID, code, redirectURI, verifier)
	if err != nil {
		return nil, err
	}
	tok.TokenFrom = providerID
	tok.DeviceID = "mnemo"
	return tok, nil
}

func waitCallback(ctx context.Context, ln net.Listener, state string) (string, error) {
	type result struct {
		code string
		err  error
	}
	ch := make(chan result, 1)
	server := &http.Server{}
	go func() {
		err := http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			code := r.URL.Query().Get("code")
			st := r.URL.Query().Get("state")
			if st != state {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, "state mismatch")
				ch <- result{err: errors.New("oauth: state mismatch")}
				return
			}
			if code == "" {
				ch <- result{err: errors.New("oauth: missing code")}
				return
			}
			_, _ = io.WriteString(w, "登录成功，可以关闭此窗口")
			ch <- result{code: code}
		}))
		if err != nil && err != http.ErrServerClosed {
			select {
			case ch <- result{err: err}:
			default:
			}
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

func exchangeToken(ctx context.Context, clientID, code, redirectURI, verifier string) (*model.TokenInfo, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("grant_type", "authorization_code")
	form.Set("code_verifier", verifier)
	form.Set("scope", "files.readwrite offline_access User.Read")

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
	return &model.TokenInfo{
		TokenFrom:    providerID,
		AccessToken:  raw.AccessToken,
		RefreshToken: raw.RefreshToken,
		ExpiresIn:    raw.ExpiresIn,
		TokenType:    raw.TokenType,
	}, nil
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

func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

var _ = json.Marshal
var _ = url.QueryEscape
