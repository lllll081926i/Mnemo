package pan123

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math/rand"
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

// ---- signPath (AList drivers/123, os=web version=3) ----

// signTable maps decimal digits 0-9 to letters, mirroring the legacy table.
var signTable = []byte("adefghlmyijnopkqrstubcvwsz")

// crc32sum is the unsigned CRC-32 (IEEE) used by the legacy signPath.
func crc32sum(s string) uint32 {
	h := crc32.NewIEEE()
	_, _ = io.WriteString(h, s)
	return h.Sum32()
}

// signPath computes the two query parameters for an API path:
// [timeSign, dataSignValue] where dataSignValue = "ts-random-dataSign".
func signPath(apiPath string) (string, string) {
	random := fmt.Sprintf("%d", rand.Intn(10000001))
	now := time.Now()
	timestamp := fmt.Sprintf("%d", now.Unix())
	// nowStr is the UTC+8 wall-clock yyyyMMddHHmm.
	utc8 := now.Add(8 * time.Hour).UTC()
	nowStr := utc8.Format("200601021504")
	mapped := make([]byte, len(nowStr))
	for i := 0; i < len(nowStr); i++ {
		d := nowStr[i] - '0'
		if d > 9 {
			d = 0
		}
		mapped[i] = signTable[d]
	}
	timeSign := fmt.Sprintf("%d", crc32sum(string(mapped)))
	data := strings.Join([]string{timestamp, random, apiPath, platform, appVer, timeSign}, "|")
	dataSign := fmt.Sprintf("%d", crc32sum(data))
	return timeSign, strings.Join([]string{timestamp, random, dataSign}, "-")
}

// withAPISign appends the sign parameters to a raw API url.
func withAPISign(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	k, v := signPath(u.Path)
	q := u.Query()
	q.Set(k, v)
	u.RawQuery = q.Encode()
	return u.String()
}

// ---- rate limiting (legacy: concurrency 1 + min interval 700ms + error backoff) ----

var (
	rateMu       sync.Mutex
	lastReqAt    time.Time
	backoffUntil time.Time
	errorStreak  int
)

func pan123RateLimit() {
	rateMu.Lock()
	defer rateMu.Unlock()
	waitUntil := lastReqAt.Add(700 * time.Millisecond)
	if backoffUntil.After(waitUntil) {
		waitUntil = backoffUntil
	}
	if d := time.Until(waitUntil); d > 0 {
		time.Sleep(d)
	}
	lastReqAt = time.Now()
}

func pan123RateLimitError() {
	rateMu.Lock()
	errorStreak++
	base := 1000 * time.Millisecond
	d := base << (errorStreak - 1)
	if errorStreak > 5 {
		d = 20 * time.Second
	}
	backoffUntil = time.Now().Add(d)
	rateMu.Unlock()
}

func pan123RateLimitOK() {
	rateMu.Lock()
	errorStreak = 0
	rateMu.Unlock()
}

// ---- http plumbing ----

// pan123Code accepts both JSON numbers and numeric strings. The legacy client
// used Number(json.code), and the web API has returned both representations.
type pan123Code int

func (c *pan123Code) UnmarshalJSON(raw []byte) error {
	var number int64
	if err := json.Unmarshal(raw, &number); err == nil {
		*c = pan123Code(number)
		return nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return fmt.Errorf("123: invalid response code: %w", err)
	}
	number, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
	if err != nil {
		return fmt.Errorf("123: invalid response code %q: %w", text, err)
	}
	*c = pan123Code(number)
	return nil
}

type apiResp struct {
	Code    pan123Code      `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type pan123APIError struct {
	Code    pan123Code
	Message string
}

func (e *pan123APIError) Error() string { return e.Message }

// apiClient issues signed, rate-limited requests with a concrete bearer token.
type apiClient struct {
	hc    *netx.Client
	token string
}

func newAPIClient(token string) *apiClient {
	return &apiClient{hc: netx.NewClient(60 * time.Second), token: token}
}

func pan123Headers(token string) map[string]string {
	return map[string]string{
		"origin":        "https://yun.123pan.com",
		"referer":       referer,
		"authorization": "Bearer " + token,
		"platform":      platform,
		"app-version":   appVer,
		"content-type":  "application/json",
		"user-agent":    ua,
	}
}

func (a *apiClient) do(ctx context.Context, method, rawURL string, body any, query map[string]string) (*apiResp, error) {
	pan123RateLimit()
	finalURL := withAPISign(rawURL)
	if len(query) > 0 {
		u, err := url.Parse(finalURL)
		if err != nil {
			return nil, err
		}
		q := u.Query()
		for k, v := range query {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
		finalURL = u.String()
	}
	headers := pan123Headers(a.token)
	if parsed, parseErr := url.Parse(finalURL); parseErr == nil && strings.EqualFold(parsed.Hostname(), "www.123pan.com") {
		headers["origin"] = "https://www.123pan.com"
		headers["referer"] = "https://www.123pan.com/"
	}
	var resp *http.Response
	var err error
	if method == http.MethodGet {
		resp, err = a.hc.Do(ctx, method, finalURL, headers, nil)
	} else {
		resp, err = a.hc.Do(ctx, method, finalURL, headers, netx.JSONBody(body))
	}
	if err != nil {
		pan123RateLimitError()
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	var out apiResp
	if err := json.Unmarshal(raw, &out); err != nil {
		if resp.StatusCode >= 400 {
			pan123RateLimitError()
			return nil, fmt.Errorf("http %d: %s", resp.StatusCode, truncateBytes(raw, 300))
		}
		pan123RateLimitError()
		return nil, fmt.Errorf("123: 非法响应: %s", truncateBytes(raw, 200))
	}
	pan123RateLimitOK()
	if resp.StatusCode >= 400 && out.Code == 0 {
		out.Code = pan123Code(resp.StatusCode)
	}
	return &out, nil
}

// api is the Driver-level entry: token from context, 401 → auto re-login with
// stored credentials, then code != 0 → error.
func (d *Driver) api(ctx context.Context, c drive.Context, method, rawURL string, body any, query map[string]string) (*apiResp, error) {
	if c.Token == nil || c.Token.AccessToken == "" {
		return nil, errors.New("123 云盘未登录")
	}
	a := newAPIClient(c.Token.AccessToken)
	resp, err := a.do(ctx, method, rawURL, body, query)
	if err != nil {
		return nil, err
	}
	if resp.Code == 401 {
		if nt, ok := reloginFromToken(ctx, c.Token); ok {
			c.Token.AccessToken = nt
			a = newAPIClient(nt)
			resp, err = a.do(ctx, method, rawURL, body, query)
			if err != nil {
				return nil, err
			}
		}
	}
	if resp.Code != 0 {
		msg := resp.Message
		if msg == "" {
			msg = fmt.Sprintf("123 云盘请求失败 code=%d", resp.Code)
		}
		return nil, &pan123APIError{Code: resp.Code, Message: msg}
	}
	return resp, nil
}

// reloginFromToken replays the stored username/password on 401.
func reloginFromToken(ctx context.Context, tok *model.TokenInfo) (string, bool) {
	username, password, ok := storedPan123Credentials(tok)
	if !ok {
		return "", false
	}
	next, err := pan123Login(ctx, username, password)
	if err != nil || next == "" {
		return "", false
	}
	return next, true
}

func storedPan123Credentials(tok *model.TokenInfo) (username, password string, ok bool) {
	if tok == nil {
		return "", "", false
	}
	var cred struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal([]byte(tok.RefreshToken), &cred); err != nil {
		return "", "", false
	}
	username = strings.TrimSpace(cred.Username)
	if username == "" || cred.Password == "" {
		return "", "", false
	}
	return username, cred.Password, true
}

// ---- login (legacy auth.ts: username+password, no RSA/captcha in web API) ----

// pan123LoginBody builds the sign_in body: email → mail/type, else passport/remember.
func pan123LoginBody(username, password string) map[string]any {
	if strings.Contains(username, "@") {
		return map[string]any{"mail": username, "password": password, "type": 2}
	}
	return map[string]any{"passport": username, "password": password, "remember": true}
}

// pan123Login performs POST /api/user/sign_in and returns the token.
func pan123Login(ctx context.Context, username, password string) (string, error) {
	hc := netx.NewClient(60 * time.Second)
	headers := map[string]string{
		"origin":       "https://yun.123pan.com",
		"referer":      referer,
		"platform":     platform,
		"app-version":  appVer,
		"content-type": "application/json",
		"user-agent":   ua,
	}
	var out struct {
		Code    pan123Code `json:"code"`
		Message string     `json:"message"`
		Data    struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := hc.PostJSON(ctx, apiSignIn, headers, pan123LoginBody(username, password), &out); err != nil {
		return "", err
	}
	if out.Code != 200 {
		msg := out.Message
		if msg == "" {
			msg = "123 云盘登录失败"
		}
		return "", errors.New(msg)
	}
	if out.Data.Token == "" {
		return "", errors.New("123 云盘未返回 token")
	}
	return out.Data.Token, nil
}

// authLogin is the registration AuthFunc: login → user info → TokenInfo.
func authLogin(ctx context.Context, req drive.AuthRequest) (*model.TokenInfo, error) {
	username := strings.TrimSpace(req.Config["username"])
	password := req.Config["password"]
	if username == "" || password == "" {
		return nil, errors.New("请输入 123 云盘账号和密码")
	}
	token, err := pan123Login(ctx, username, password)
	if err != nil {
		return nil, err
	}
	resp, err := newAPIClient(token).do(ctx, http.MethodGet, apiUserInfo, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("123 云盘登录后获取账号信息失败: %w", err)
	}
	if resp.Code != 0 {
		msg := resp.Message
		if msg == "" {
			msg = fmt.Sprintf("123 云盘获取账号信息失败 code=%d", resp.Code)
		}
		return nil, errors.New(msg)
	}
	data := parseMap(resp.Data)
	uid := firstString(data, "Uid", "uid")
	if uid == "" {
		uid = username
	}
	name := firstString(data, "Nickname", "nickname", "Passport", "passport", "Mail", "mail")
	if name == "" {
		name = username
	}
	used := firstInt64(data, "SpaceUsed", "spaceUsed")
	total := firstInt64(data, "SpacePermanent", "spacePermanent") + firstInt64(data, "SpaceTemp", "spaceTemp")
	free := total - used
	if free < 0 {
		free = 0
	}
	tok := &model.TokenInfo{
		TokenFrom:         providerID,
		AccessToken:       token,
		RefreshToken:      mustJSON(map[string]string{"username": username, "password": password}),
		TokenType:         "Bearer",
		UserID:            model.BuildUserID(providerID, uid),
		UserName:          name,
		NickName:          name,
		Name:              name,
		Avatar:            firstString(data, "HeadImage", "headImage"),
		DefaultDriveID:    model.BuildDriveID(providerID, uid),
		ProviderAccountID: uid,
		ProviderRootID:    "0",
		UsedSize:          used,
		TotalSize:         total,
		FreeSize:          free,
	}
	return tok, nil
}

// RefreshAccount refreshes quota + profile from /user/info.
func (d *Driver) RefreshAccount(ctx context.Context, c drive.Context, token *model.TokenInfo) (*model.TokenInfo, error) {
	if token == nil {
		return nil, errors.New("123 云盘未登录")
	}
	resp, err := d.api(ctx, c, http.MethodGet, apiUserInfo, nil, nil)
	if err != nil {
		// Do not report an expired/invalid session as a successful refresh. The
		// caller needs the error to surface the account state and offer login
		// again; d.api already retries once with the stored credentials on 401.
		return token, err
	}
	data := parseMap(resp.Data)
	used := firstInt64(data, "SpaceUsed", "spaceUsed")
	total := firstInt64(data, "SpacePermanent", "spacePermanent") + firstInt64(data, "SpaceTemp", "spaceTemp")
	if used != 0 || total != 0 {
		token.UsedSize = used
		token.TotalSize = total
		token.FreeSize = total - used
		if token.FreeSize < 0 {
			token.FreeSize = 0
		}
	}
	name := firstString(data, "Nickname", "nickname", "Passport", "passport", "Mail", "mail")
	if name != "" {
		token.UserName = name
		token.NickName = name
		token.Name = name
	}
	if avatar := firstString(data, "HeadImage", "headImage"); avatar != "" {
		token.Avatar = avatar
	}
	return token, nil
}

// ---- file pool (AList drivers/123 runtime File pool) ----
