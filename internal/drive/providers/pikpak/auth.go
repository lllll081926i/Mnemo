package pikpak

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

const providerID = model.ProviderPikpak

func init() {
	drive.Register(drive.Registration{
		ID:   providerID,
		Meta: drive.GetMeta(providerID),
		Caps: drive.NewCapabilities(providerID, map[string]bool{
			"offlineDownload": true,
			"createShare":     true,
			"shareExpiration": true,
			"sharePassword":   true,
			"combinedShare":   true,
			"shareHistory":    true,
			"importShare":     true,
			"trashView":       true,
			"trashRestore":    true,
			"recycleBin":      true,
			"favorite":        true,
			"permanentDelete": true,
		}, nil),
		Login: drive.LoginConfig{Fields: []drive.LoginField{
			{Key: "username", Type: "text", Label: "账号（手机/邮箱）", Required: true},
			{Key: "password", Type: "password", Label: "密码", Required: true},
		}},
		Auth:    authSignIn,
		Factory: func() drive.Driver { return &Driver{} },
	})
}

// DeviceStore persists per-account device ids.
var deviceStore *storePath

type apiCaptchaCacheEntry struct {
	token     string
	expiresAt time.Time
}

var apiCaptchaCache = struct {
	sync.Mutex
	items map[string]apiCaptchaCacheEntry
}{items: make(map[string]apiCaptchaCacheEntry)}

type storePath struct{ dir string }

// SetIdentityDir sets the device identity storage dir (app wiring).
func SetIdentityDir(dir string) { deviceStore = &storePath{dir: dir} }

func (s *storePath) get(username string) string {
	if s == nil {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(s.dir, idKey(username)+".id"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func (s *storePath) set(username, id string) {
	if s == nil {
		return
	}
	_ = os.MkdirAll(s.dir, 0o755)
	_ = os.WriteFile(filepath.Join(s.dir, idKey(username)+".id"), []byte(id), 0o644)
}

func idKey(username string) string {
	return md5hex(strings.ToLower(strings.TrimSpace(username)))
}

// createDeviceID generates an RFC4122 v4 UUID.
func createDeviceID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// getOrCreateDeviceID returns a stable device id for an account.
func getOrCreateDeviceID(username string) string {
	id := ""
	if deviceStore != nil {
		id = deviceStore.get(username)
	}
	compact := strings.ReplaceAll(id, "-", "")
	if len(compact) == 32 && isHex(compact) {
		return id
	}
	id = createDeviceID()
	if deviceStore != nil {
		deviceStore.set(username, id)
	}
	return id
}

func isHex(s string) bool {
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

// initCaptcha requests a captcha token (challenge url may be returned).
func initCaptcha(ctx context.Context, hc *netx.Client, deviceID, username string, action string) (string, string, error) {
	return initCaptchaWithPrev(ctx, hc, deviceID, username, action, "")
}

// initCaptchaWithPrev requests a captcha token; previousToken chains a verified
// slider token so the server registers the result (legacy previousToken retry).
// signin 动作的 meta 只含 email/phone_number/username 单字段（对齐旧版）。
func initCaptchaWithPrev(ctx context.Context, hc *netx.Client, deviceID, username, action, previousToken string) (string, string, error) {
	u := strings.TrimSpace(username)
	meta := map[string]string{}
	if strings.Contains(u, "@") && strings.Contains(u, ".") {
		meta["email"] = u
	} else if isPhone(u) {
		meta["phone_number"] = strings.ReplaceAll(strings.ReplaceAll(u, " ", ""), "-", "")
	} else {
		meta["username"] = u
	}
	body := map[string]any{
		"client_id":    clientID,
		"action":       action,
		"device_id":    deviceID,
		"meta":         meta,
		"redirect_uri": redirectURI,
	}
	if previousToken != "" {
		body["captcha_token"] = previousToken
	}
	var res struct {
		CaptchaToken string `json:"captcha_token"`
		URL          string `json:"url"`
	}
	resp, err := hc.Do(ctx, http.MethodPost, userHost+"/v1/shield/captcha/init", captchaHeaders(deviceID, ""), netx.JSONBody(body))
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", "", parseAPIErrorWithRetry(data, resp.StatusCode, resp.Header.Get("Retry-After"))
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return "", "", err
	}
	if strings.TrimSpace(res.CaptchaToken) == "" {
		return "", "", errors.New("pikpak: 验证接口未返回 captcha token")
	}
	return res.CaptchaToken, res.URL, nil
}

// initAPICaptcha follows the drive API captcha protocol. Its meta payload is
// different from the signin payload: API requests identify the account by
// user_id and carry the signed client metadata inside meta.
func initAPICaptcha(ctx context.Context, hc *netx.Client, deviceID, accountID, accessToken, action, previousToken string) (string, string, error) {
	timestamp := timestampNow()
	meta := CaptchaMeta{
		CaptchaSign:   captchaSign(deviceID, timestamp),
		ClientVersion: clientVersion,
		PackageName:   packageName,
		Timestamp:     timestamp,
		UserID:        model.StripUserID(providerID, strings.TrimSpace(accountID)),
	}
	body := map[string]any{
		"client_id": clientID,
		"action":    action,
		"device_id": deviceID,
		"meta":      meta,
	}
	if previousToken != "" {
		body["captcha_token"] = previousToken
	}
	var res struct {
		CaptchaToken string `json:"captcha_token"`
		URL          string `json:"url"`
	}
	resp, err := hc.Do(ctx, http.MethodPost, userHost+"/v1/shield/captcha/init", captchaHeaders(deviceID, accessToken), netx.JSONBody(body))
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", "", parseAPIErrorWithRetry(data, resp.StatusCode, resp.Header.Get("Retry-After"))
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return "", "", err
	}
	return res.CaptchaToken, res.URL, nil
}

func captchaHeaders(deviceID, token string) map[string]string {
	h := map[string]string{
		"User-Agent":       userAgent,
		"Accept":           "application/json",
		"Referer":          "https://mypikpak.com/",
		"X-Client-Id":      clientID,
		"X-Device-Id":      deviceID,
		"X-Client-Version": clientVersion,
		"Content-Type":     "application/json",
	}
	if token != "" {
		h["Authorization"] = "Bearer " + token
	}
	return h
}

func timestampNow() string { return fmt.Sprint(time.Now().UnixMilli()) }

func apiCaptchaCacheKey(c *client, action string) string {
	return strings.Join([]string{c.deviceID, c.accountID, md5hex(c.accessToken), action}, "\x00")
}

func cachedAPICaptchaToken(c *client, action string) string {
	if c == nil {
		return ""
	}
	key := apiCaptchaCacheKey(c, action)
	now := time.Now()
	apiCaptchaCache.Lock()
	defer apiCaptchaCache.Unlock()
	entry, ok := apiCaptchaCache.items[key]
	if !ok || entry.token == "" || !entry.expiresAt.After(now.Add(10*time.Second)) {
		if ok {
			delete(apiCaptchaCache.items, key)
		}
		return ""
	}
	return entry.token
}

// apiCaptchaToken gets a cached action token or exchanges the previous token
// for a fresh one. PikPak scopes these tokens by device, account and action.
func apiCaptchaToken(ctx context.Context, c *client, action string, forceRefresh bool) (string, error) {
	if c == nil || c.deviceID == "" || c.accountID == "" {
		return "", errors.New("pikpak: 缺少验证码设备或账号标识")
	}
	key := apiCaptchaCacheKey(c, action)
	var previous string
	if !forceRefresh {
		if token := cachedAPICaptchaToken(c, action); token != "" {
			return token, nil
		}
	}
	apiCaptchaCache.Lock()
	if entry, ok := apiCaptchaCache.items[key]; ok {
		previous = entry.token
	}
	apiCaptchaCache.Unlock()
	token, _, err := initAPICaptcha(ctx, c.http, c.deviceID, c.accountID, c.accessToken, action, previous)
	if err != nil {
		return "", err
	}
	if token == "" {
		return "", errors.New("pikpak: 验证接口未返回 captcha token")
	}
	apiCaptchaCache.Lock()
	apiCaptchaCache.items[key] = apiCaptchaCacheEntry{token: token, expiresAt: time.Now().Add(4*time.Minute + 30*time.Second)}
	apiCaptchaCache.Unlock()
	return token, nil
}

// AuthResp is the signin/token response.
type AuthResp struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int64  `json:"expires_in"`
	TokenType        string `json:"token_type"`
	Sub              string `json:"sub"`
	UserID           string `json:"user_id"`
	UserName         string `json:"user_name"`
	Avatar           string `json:"avatar"`
	NickName         string `json:"nick_name"`
	DeviceID         string `json:"device_id"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// signIn logs in with username+password (+captcha).
// 字段与旧版 signInPikPak 一致：body 只有 client_id/username/password，
// captcha_token 走 X-Captcha-Token 头（放 body 会被服务端忽略 → 报 "add captcha"）。
func signIn(ctx context.Context, hc *netx.Client, deviceID, username, password, captchaToken string) (*AuthResp, error) {
	body := map[string]any{
		"client_id": clientID,
		"username":  strings.TrimSpace(username),
		"password":  password,
	}
	headers := captchaHeaders(deviceID, "")
	if captchaToken != "" {
		headers["X-Captcha-Token"] = captchaToken
	}
	var res AuthResp
	resp, err := hc.Do(ctx, http.MethodPost, userHost+"/v1/auth/signin", headers, netx.JSONBody(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, parseAPIErrorWithRetry(data, resp.StatusCode, resp.Header.Get("Retry-After"))
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	if res.AccessToken == "" {
		if res.ErrorDescription != "" {
			return nil, errors.New(res.ErrorDescription)
		}
		return nil, errors.New("pikpak: login failed")
	}
	res.DeviceID = deviceID
	return &res, nil
}

// refreshToken obtains new tokens.
func refreshToken(ctx context.Context, hc *netx.Client, deviceID, refresh string) (*AuthResp, error) {
	body := map[string]any{
		"client_id":     clientID,
		"grant_type":    "refresh_token",
		"refresh_token": refresh,
	}
	var res AuthResp
	resp, err := hc.Do(ctx, http.MethodPost, userHost+"/v1/auth/token", captchaHeaders(deviceID, ""), netx.JSONBody(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, parseAPIErrorWithRetry(data, resp.StatusCode, resp.Header.Get("Retry-After"))
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	if res.AccessToken == "" {
		return nil, errors.New("pikpak: refresh failed")
	}
	return &res, nil
}

var pikpakCaptchaExchangeRetryDelays = []time.Duration{
	600 * time.Millisecond,
	1200 * time.Millisecond,
	2000 * time.Millisecond,
}

// exchangeLoginCaptcha waits for PikPak to register a completed slider and
// chains the newest token through the legacy retry sequence.
func exchangeLoginCaptcha(ctx context.Context, hc *netx.Client, deviceID, username, action, previousToken string) (string, string, error) {
	previous := previousToken
	for attempt := 0; ; attempt++ {
		token, challengeURL, err := initCaptchaWithPrev(ctx, hc, deviceID, username, action, previous)
		if err != nil {
			return "", "", err
		}
		if challengeURL == "" || attempt >= len(pikpakCaptchaExchangeRetryDelays) {
			return token, challengeURL, nil
		}
		timer := time.NewTimer(pikpakCaptchaExchangeRetryDelays[attempt])
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", "", ctx.Err()
		case <-timer.C:
		}
		previous = token
	}
}

// authSignIn is the Registration.Auth login flow.
func authSignIn(ctx context.Context, req drive.AuthRequest) (*model.TokenInfo, error) {
	username := strings.TrimSpace(req.Config["username"])
	password := req.Config["password"]
	if username == "" || password == "" {
		return nil, errors.New("pikpak: 请输入账号和密码")
	}
	deviceID := getOrCreateDeviceID(username)
	hc := netx.NewClient(60 * time.Second)

	captchaToken := strings.TrimSpace(req.Config["captcha_token"])
	if captchaToken == "" {
		// A failed captcha init must stop here. Continuing with an empty token
		// turns a transport/rate-limit failure into a misleading login error.
		tok, urlValue, err := initCaptcha(ctx, hc, deviceID, username, "POST:/v1/auth/signin")
		if err != nil {
			return nil, err
		}
		captchaToken = tok
		if urlValue != "" {
			// 需要滑块验证：把验证 URL 与 token 带回前端，用户完成后带 token 重试
			return nil, fmt.Errorf("pikpak: captcha_required\nurl=%s\ntoken=%s", urlValue, tok)
		}
	} else {
		// 滑块完成后的重试：链式带 previousToken 重发 init，等服务端登记验证结果
		// （对齐旧版 loginPikPakWithCaptcha 的退避链）
		tok, urlValue, err := exchangeLoginCaptcha(ctx, hc, deviceID, username, "POST:/v1/auth/signin", captchaToken)
		if err != nil {
			return nil, err
		}
		if urlValue != "" {
			return nil, fmt.Errorf("pikpak: captcha_required\nurl=%s\ntoken=%s", urlValue, tok)
		}
		captchaToken = tok
	}

	auth, err := signIn(ctx, hc, deviceID, username, password, captchaToken)
	if err != nil && isCaptchaError(err) {
		// Some accounts return no challenge URL from init but reject the first
		// signin anyway. Reuse that token as the legacy flow does, then retry
		// signin once with the exchanged token.
		tok, urlValue, exchangeErr := exchangeLoginCaptcha(ctx, hc, deviceID, username, "POST:/v1/auth/signin", captchaToken)
		if exchangeErr != nil {
			return nil, exchangeErr
		}
		if urlValue != "" {
			return nil, fmt.Errorf("pikpak: captcha_required\nurl=%s\ntoken=%s", urlValue, tok)
		}
		captchaToken = tok
		auth, err = signIn(ctx, hc, deviceID, username, password, captchaToken)
	}
	if err != nil {
		return nil, err
	}

	accountID := firstNonEmpty(auth.UserID, auth.Sub, username)
	used, total := int64(0), int64(0)
	if cl := newClient(auth.AccessToken, deviceID, accountID); cl != nil {
		used, total = cl.About(ctx)
	}
	name := auth.NickName
	if name == "" {
		name = auth.UserName
	}
	if name == "" {
		name = username
	}
	tok := &model.TokenInfo{
		TokenFrom:         providerID,
		AccessToken:       auth.AccessToken,
		RefreshToken:      auth.RefreshToken,
		ExpiresIn:         auth.ExpiresIn,
		TokenType:         auth.TokenType,
		UserID:            model.BuildUserID(providerID, accountID),
		ProviderAccountID: accountID,
		UserName:          name,
		NickName:          name,
		Name:              name,
		Avatar:            auth.Avatar,
		DeviceID:          deviceID,
		UsedSize:          used,
		TotalSize:         total,
	}
	return tok, nil
}
