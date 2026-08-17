package ilanzou

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

const ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"

var (
	httpClient   = &http.Client{Timeout: 60 * time.Second}
	manualClient = &http.Client{
		Timeout: 60 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
)

// fetchMinInterval is the rate-limit spacing between ilanzou API requests
// (legacy withProviderRateLimit: concurrency 2 + min interval 260ms).
var fetchMinInterval = 260 * time.Millisecond

type throttle struct {
	mu   sync.Mutex
	last time.Time
}

func (t *throttle) wait() {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	next := t.last.Add(fetchMinInterval)
	if next.After(now) {
		time.Sleep(next.Sub(now))
		now = time.Now()
	}
	t.last = now
}

var fetchThrottle = &throttle{}

// cred is the parsed refresh_token payload (mirrors legacy parseCred).
type cred struct {
	Username string `json:"username"`
	Password string `json:"password"`
	UUID     string `json:"uuid"`
	Token    string `json:"token"`
	UserID   string `json:"userId"`
	Account  string `json:"account"`
}

func parseCred(refresh string) *cred {
	if refresh == "" || !strings.HasPrefix(refresh, "{") {
		return nil
	}
	var c cred
	if err := json.Unmarshal([]byte(refresh), &c); err != nil {
		return nil
	}
	return &c
}

// loginResult is the outcome of ilanzouLogin.
type loginResult struct {
	token   string
	uuid    string
	userId  string
	account string
}

// requestOptions controls one ilanzouRequest call.
type requestOptions struct {
	method      string
	body        any
	query       map[string]string
	unproved    bool // explicit proved:false (default is proved)
	accessToken string
	uuid        string
}

// ilanzouRequest is the driver-level API entry (port of ilanzouRequestRaw):
// it signs the query with the AES timestamp token, and on code -1/-2 (or a
// missing token) re-logs-in once with stored credentials. Returns the parsed
// JSON plus a non-nil loginResult when a re-login happened (so RefreshAccount
// can persist the fresh session).
func (d *Driver) request(ctx context.Context, c drive.Context, pathName string, opts requestOptions) (map[string]any, *loginResult, error) {
	proved := !opts.unproved
	token := opts.accessToken
	uuid := opts.uuid
	var cr *cred
	if c.Token != nil {
		token = firstNonEmpty(token, c.Token.AccessToken)
		cr = parseCred(c.Token.RefreshToken)
		if cr != nil && uuid == "" {
			uuid = cr.UUID
		}
	}
	if uuid == "" {
		uuid = newDeviceUuid()
	}

	doOnce := func(appToken string) (map[string]any, error) {
		_, tsEnc, err := getTimestampToken(ILANZOU_CONF.Secret)
		if err != nil {
			return nil, err
		}
		params := signParams(uuid, tsEnc)
		if proved && appToken != "" {
			params.Set("appToken", appToken)
		}
		for k, v := range opts.query {
			params.Set(k, v)
		}
		prefix := ILANZOU_CONF.Unproved
		if proved {
			prefix = ILANZOU_CONF.Proved
		}
		rawURL := ILANZOU_CONF.Base + "/" + prefix + pathName + "?" + params.Encode()
		method := strings.ToUpper(opts.method)
		if method == "" {
			method = http.MethodGet
		}
		return ilanzouJSON(ctx, method, rawURL, opts.body)
	}

	j, err := doOnce(token)
	if err != nil {
		return nil, nil, err
	}
	code := numOf(j["code"])
	if proved && (code == -1 || code == -2 || token == "") && cr != nil && cr.Username != "" && cr.Password != "" {
		login, lerr := ilanzouLogin(ctx, cr.Username, cr.Password, cr.UUID)
		if lerr != nil {
			return nil, nil, fmt.Errorf("优享版蓝奏云自动登录失败: %w", lerr)
		}
		token = login.token
		uuid = login.uuid
		if j2, err2 := doOnce(token); err2 == nil {
			j = j2
		} else {
			return nil, nil, err2
		}
		if numOf(j["code"]) == 200 {
			applyLoginSession(c.Token, login)
			return j, login, nil
		}
	}
	if numOf(j["code"]) != 200 {
		msg := strOf(j["msg"])
		if msg == "" {
			msg = "优享版蓝奏云请求失败"
		}
		return nil, nil, fmt.Errorf("%v: %s", j["code"], msg)
	}
	return j, nil, nil
}

// signParams builds the fixed device/timestamp query parameters.
func signParams(uuid, tsEnc string) url.Values {
	params := url.Values{}
	params.Set("uuid", uuid)
	params.Set("devType", "6")
	params.Set("devCode", uuid)
	params.Set("devModel", "chrome")
	params.Set("devVersion", ILANZOU_CONF.DevVersion)
	params.Set("appVersion", "")
	params.Set("timestamp", tsEnc)
	params.Set("extra", "2")
	return params
}

// ilanzouJSON performs one signed request and decodes the JSON document.
func ilanzouJSON(ctx context.Context, method, rawURL string, body any) (map[string]any, error) {
	fetchThrottle.wait()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Origin", ILANZOU_CONF.Site)
	req.Header.Set("Referer", ILANZOU_CONF.Site+"/")
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Content-Type", "application/json") // legacy sends it on every request
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	text, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, truncate(string(text), 200))
	}
	var j map[string]any
	if err := json.Unmarshal(text, &j); err != nil {
		return nil, fmt.Errorf("ilanzou 响应异常: %s", truncate(string(text), 200))
	}
	return j, nil
}

// fetchILanzouUuid prefers the server-assigned uuid (AList Init).
func fetchILanzouUuid(ctx context.Context, seedUuid string) string {
	_, tsEnc, err := getTimestampToken(ILANZOU_CONF.Secret)
	if err != nil {
		return seedUuid
	}
	params := signParams(seedUuid, tsEnc)
	rawURL := ILANZOU_CONF.Base + "/" + ILANZOU_CONF.Unproved + "/getUuid?" + params.Encode()
	j, err := ilanzouJSON(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return seedUuid
	}
	remote := strOf(j["uuid"])
	if remote == "" {
		if data := mapVal(j, "data"); data != nil {
			remote = strOf(data["uuid"])
		}
	}
	if remote != "" {
		return remote
	}
	return seedUuid
}

// ilanzouLogin performs /unproved/login then maps the account id via
// /proved/user/account/map (AList Init).
func ilanzouLogin(ctx context.Context, username, password, uuid string) (*loginResult, error) {
	deviceUuid := uuid
	if deviceUuid == "" {
		deviceUuid = newDeviceUuid()
		deviceUuid = fetchILanzouUuid(ctx, deviceUuid)
	}
	_, tsEnc, err := getTimestampToken(ILANZOU_CONF.Secret)
	if err != nil {
		return nil, err
	}
	params := signParams(deviceUuid, tsEnc)
	rawURL := ILANZOU_CONF.Base + "/" + ILANZOU_CONF.Unproved + "/login?" + params.Encode()
	j, err := ilanzouJSON(ctx, http.MethodPost, rawURL, map[string]any{
		"loginName": username,
		"loginPwd":  password,
	})
	if err != nil {
		return nil, err
	}
	if numOf(j["code"]) != 200 {
		msg := strOf(j["msg"])
		if msg == "" {
			msg = "优享版蓝奏云登录失败"
		}
		return nil, errors.New(msg)
	}
	var token string
	if data := mapVal(j, "data"); data != nil {
		token = strOf(data["appToken"])
	}
	if token == "" {
		return nil, errors.New("优享版蓝奏云未返回 token")
	}

	mapParams := signParams(deviceUuid, tsEnc)
	mapParams.Set("appToken", token)
	mapURL := ILANZOU_CONF.Base + "/" + ILANZOU_CONF.Proved + "/user/account/map?" + mapParams.Encode()
	mapJSON, err := ilanzouJSON(ctx, http.MethodGet, mapURL, nil)
	if err != nil {
		return nil, err
	}
	userId, account := "", ""
	if mm := mapVal(mapJSON, "map"); mm != nil {
		userId = strOf(mm["userId"])
		account = strOf(mm["account"])
	}
	if userId == "" || account == "" {
		if dm := mapVal(mapVal(mapJSON, "data"), "map"); dm != nil {
			userId = strOf(dm["userId"])
			account = strOf(dm["account"])
		}
	}
	userId = firstNonEmpty(userId, username)
	account = firstNonEmpty(account, username)
	return &loginResult{token: token, uuid: deviceUuid, userId: userId, account: account}, nil
}

// buildILanzouDownloadUrl builds the signed /unproved/file/redirect URL.
func buildILanzouDownloadUrl(fileID, userID, token, uuid string) (string, error) {
	ts, tsEnc, err := getTimestampToken(ILANZOU_CONF.Secret)
	if err != nil {
		return "", err
	}
	downloadID, err := aesEncryptToHex(fileID+"|"+userID, ILANZOU_CONF.Secret)
	if err != nil {
		return "", err
	}
	auth, err := aesEncryptToHex(fmt.Sprintf("%s|%d", fileID, ts), ILANZOU_CONF.Secret)
	if err != nil {
		return "", err
	}
	params := signParams(uuid, tsEnc)
	params.Set("appToken", token)
	params.Set("enable", "0")
	params.Set("downloadId", downloadID)
	params.Set("auth", auth)
	return ILANZOU_CONF.Base + "/" + ILANZOU_CONF.Unproved + "/file/redirect?" + params.Encode(), nil
}

// ---- json helpers ----

func numOf(v any) int64 {
	switch t := v.(type) {
	case float64:
		return int64(t)
	case string:
		n, _ := strconv.ParseInt(t, 10, 64)
		return n
	case json.Number:
		n, _ := t.Int64()
		return n
	case int:
		return int64(t)
	case int64:
		return t
	}
	return 0
}

func strOf(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return t.String()
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", t)
	}
}

func mapVal(v any, key string) map[string]any {
	if m, ok := v.(map[string]any); ok {
		if child, ok := m[key].(map[string]any); ok {
			return child
		}
	}
	return nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// RefreshAccount validates the session via /user/account/map; an expired
// token triggers the request-level re-login and the fresh session is persisted
// onto the returned token.
func (d *Driver) RefreshAccount(ctx context.Context, c drive.Context, token *model.TokenInfo) (*model.TokenInfo, error) {
	if token == nil {
		return nil, errors.New("优享版蓝奏云未登录")
	}
	_, login, err := d.request(ctx, c, "/user/account/map", requestOptions{method: http.MethodGet})
	if err != nil {
		return nil, err
	}
	if login != nil {
		applyLoginSession(token, login)
	}
	return token, nil
}

// applyLoginSession updates only credentials that rotate during an automatic
// re-login. The account identity remains unchanged so a transient mapping
// response cannot move an existing account into a new storage namespace.
func applyLoginSession(token *model.TokenInfo, login *loginResult) {
	if token == nil || login == nil {
		return
	}
	cr := parseCred(token.RefreshToken)
	if cr == nil {
		cr = &cred{}
	}
	cr.UUID = login.uuid
	cr.Token = login.token
	cr.UserID = login.userId
	cr.Account = login.account
	token.AccessToken = login.token
	token.DeviceID = login.uuid
	token.RefreshToken = mustJSON(cr)
}
