package pan139

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

type pan139SMSRequiredError struct{}

func (pan139SMSRequiredError) Error() string {
	return "pan139_sms_required\n139 登录需要短信安全校验，请先获取短信验证码"
}

type pan139LoginState struct {
	Username      string
	Password      string
	MailCookies   string
	Client        *netx.Client
	CreatedAt     time.Time
	LastSMSSentAt time.Time
}

var (
	pan139LoginStateMu sync.Mutex
	pan139LoginStates  = map[string]*pan139LoginState{}
)

func savePan139LoginState(state *pan139LoginState) {
	if state == nil || strings.TrimSpace(state.Username) == "" {
		return
	}
	pan139LoginStateMu.Lock()
	defer pan139LoginStateMu.Unlock()
	for user, item := range pan139LoginStates {
		if item == nil || time.Since(item.CreatedAt) > pan139SMSStateTTL {
			delete(pan139LoginStates, user)
		}
	}
	pan139LoginStates[state.Username] = state
}

func loadPan139LoginState(username string) *pan139LoginState {
	pan139LoginStateMu.Lock()
	defer pan139LoginStateMu.Unlock()
	state := pan139LoginStates[strings.TrimSpace(username)]
	if state == nil || time.Since(state.CreatedAt) > pan139SMSStateTTL {
		delete(pan139LoginStates, strings.TrimSpace(username))
		return nil
	}
	return state
}

func reservePan139SMSSend(username string) (*pan139LoginState, error) {
	username = strings.TrimSpace(username)
	pan139LoginStateMu.Lock()
	defer pan139LoginStateMu.Unlock()
	state := pan139LoginStates[username]
	if state == nil || time.Since(state.CreatedAt) > pan139SMSStateTTL {
		delete(pan139LoginStates, username)
		return nil, errors.New("139 登录会话已过期，请重新提交账号密码")
	}
	if !state.LastSMSSentAt.IsZero() {
		remaining := pan139SMSMinInterval - time.Since(state.LastSMSSentAt)
		if remaining > 0 {
			seconds := int((remaining + time.Second - 1) / time.Second)
			return nil, fmt.Errorf("139 验证码已发送，请 %d 秒后再试", seconds)
		}
	}
	state.LastSMSSentAt = time.Now()
	return state, nil
}

func deletePan139LoginState(username string) {
	pan139LoginStateMu.Lock()
	delete(pan139LoginStates, strings.TrimSpace(username))
	pan139LoginStateMu.Unlock()
}

// authLogin handles Authorization, password login, or the second-step SMS login.
func authLogin(ctx context.Context, req drive.AuthRequest) (*model.TokenInfo, error) {
	authorization := strings.TrimSpace(req.Config["authorization"])
	username := strings.TrimSpace(req.Config["username"])
	if authorization == "" {
		mode := strings.ToLower(strings.TrimSpace(req.Config["login_mode"]))
		if mode == "sms" || (req.Config["sms_code"] != "" && req.Config["password"] == "") {
			if username == "" || strings.TrimSpace(req.Config["sms_code"]) == "" {
				return nil, errors.New("pan139: 请输入账号和短信验证码")
			}
			var err error
			authorization, err = loginBySMS(ctx, username, strings.TrimSpace(req.Config["sms_code"]))
			if err != nil {
				return nil, err
			}
		} else {
			password := req.Config["password"]
			if username == "" || password == "" {
				return nil, errors.New("pan139: 请输入账号密码或 Authorization")
			}
			var err error
			authorization, err = loginByPassword(ctx, username, password, req.Config["mail_cookies"])
			if err != nil {
				return nil, err
			}
		}
	}
	_, account, _, _, _, err := decodeAuthorization(authorization)
	if err != nil {
		return nil, err
	}
	hc := netx.NewClient(60 * time.Second)
	next, err := refreshAuthorization(hc, authorization)
	if err != nil {
		return nil, err
	}
	host, err := ensureHost(hc, next, account)
	if err != nil {
		return nil, err
	}
	uid := account
	if uid == "" {
		uid = next
		if len(uid) > 16 {
			uid = uid[:16]
		}
	}
	name := username
	if name == "" {
		name = account
	}
	if name == "" {
		name = uid
	}
	stored := map[string]any{"authorization": next, "account": account, "personalCloudHost": host}
	if u := req.Config["username"]; u != "" {
		stored["username"] = u
		stored["password"] = req.Config["password"]
		stored["account"] = account
	}
	tok := &model.TokenInfo{
		TokenFrom:         providerID,
		AccessToken:       next,
		RefreshToken:      mustJSON(stored),
		TokenType:         "Basic",
		UserID:            model.BuildUserID(providerID, uid),
		UserName:          name,
		NickName:          name,
		Name:              name,
		ProviderAccountID: account,
		ProviderRootID:    "/",
	}
	return tok, nil
}

// loginByPassword performs the current mail.10086.cn password login flow.
// When the account is covered by the security upgrade, the server redirects
// to an SMS login state; the state is retained for SendPan139SMS/loginBySMS.
func loginByPassword(ctx context.Context, username, password, mailCookies string) (string, error) {
	state, err := newPan139LoginState(username, password, mailCookies)
	if err != nil {
		return "", err
	}
	location, sid, err := submitPan139Login(ctx, state, false, "")
	if err != nil {
		return "", err
	}
	if sid == "" {
		if pan139NeedsSMS(location) {
			// The password is needed only for the first request. Do not retain it
			// while waiting for the user to enter the SMS second factor.
			state.Password = ""
			savePan139LoginState(state)
			return "", pan139SMSRequiredError{}
		}
		return "", errors.New("139 账密登录失败：服务器未返回登录会话")
	}
	deletePan139LoginState(username)
	return finishPan139Login(ctx, state, sid)
}

func newPan139LoginState(username, password, mailCookies string) (*pan139LoginState, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return nil, errors.New("pan139: 账号密码不能为空")
	}
	hc := netx.NewClient(60 * time.Second)
	hc.HTTP.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	hc.HTTP.Jar = jar
	mailURL, _ := url.Parse(mailLoginURL)
	initial := parseCookieMap(mailCookies)
	if mailURL != nil {
		cookies := make([]*http.Cookie, 0, len(initial))
		for name, value := range initial {
			if name != "" && value != "" {
				cookies = append(cookies, &http.Cookie{Name: name, Value: value, Path: "/"})
			}
		}
		jar.SetCookies(mailURL, cookies)
	}
	return &pan139LoginState{Username: username, Password: password, MailCookies: mailCookies, Client: hc, CreatedAt: time.Now()}, nil
}

func submitPan139Login(ctx context.Context, state *pan139LoginState, sms bool, smsCode string) (location, sid string, err error) {
	if state == nil || state.Client == nil {
		return "", "", errors.New("pan139: 登录会话不存在")
	}
	// The browser first opens Login.ashx. This creates the fresh JSESSIONID
	// expected by the password endpoint and also refreshes stale login cookies.
	if !sms {
		preflight, reqErr := state.Client.Do(ctx, http.MethodGet, mailLoginURL, map[string]string{
			"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
			"Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
			"Cache-Control":   "max-age=0",
			"Referer":         "https://mail.10086.cn/default.html",
			"User-Agent":      ua,
		}, nil)
		if reqErr != nil {
			return "", "", fmt.Errorf("139 登录预请求失败: %w", reqErr)
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(preflight.Body, 4096))
		preflight.Body.Close()
		syncPan139JarCookies(state)
	}

	cguid := fmt.Sprint(time.Now().UnixMilli())
	form := url.Values{}
	form.Set("UserName", state.Username)
	form.Set("auto", "on")
	form.Set("clientId", "1003")
	form.Set("authType", "2")
	form.Set("version", "1.0")
	if sms {
		form.Set("passOld", smsCode)
		form.Set("Password", sha1Hex("fetion.com.cn:"+smsCode))
		form.Set("reqFrom", "3")
	} else {
		form.Set("passOld", "")
		form.Set("Password", sha1Hex("fetion.com.cn:"+state.Password))
		form.Set("webIndexPagePwdLogin", "1")
		form.Set("pwdType", "1")
		form.Set("reqFrom", "0")
	}
	referer := fmt.Sprintf("https://mail.10086.cn/default.html?&s=1&v=0&u=%s&m=1&ec=S001&resource=indexLogin&clientid=1003&auto=on&cguid=%s&mtime=45",
		base64.StdEncoding.EncodeToString([]byte(state.Username)), cguid)
	resp, reqErr := state.Client.Do(ctx, http.MethodPost, mailLoginURL+"?_fv=4&cguid="+url.QueryEscape(cguid)+"&resource=indexLogin", map[string]string{
		"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
		"Content-Type":    "application/x-www-form-urlencoded",
		"Origin":          "https://mail.10086.cn",
		"Referer":         referer,
		"User-Agent":      ua,
	}, strings.NewReader(form.Encode()))
	if reqErr != nil {
		return "", "", fmt.Errorf("139 登录请求失败: %w", reqErr)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
	location = resp.Header.Get("Location")
	state.MailCookies = mergePan139ResponseCookies(state.MailCookies, resp.Cookies())
	syncPan139JarCookies(state)
	sid = extractPan139SID(location, resp, body)
	if sid == "" && pan139NeedsSMS(location) {
		return location, "", nil
	}
	if sid == "" && resp.StatusCode >= http.StatusBadRequest {
		return location, "", fmt.Errorf("139 登录失败：HTTP %d", resp.StatusCode)
	}
	if sid == "" && len(bytesTrimSpace(body)) > 0 {
		// Some gateways return the redirect as a plain HTML/JSON fragment.
		text := string(bytesTrimSpace(body))
		if pan139NeedsSMS(text) {
			return text, "", nil
		}
	}
	return location, sid, nil
}

func finishPan139Login(ctx context.Context, state *pan139LoginState, sid string) (string, error) {
	cookies := parseCookieMap(state.MailCookies)
	rmkey := cookies["RMKEY"]
	if rmkey == "" {
		return "", errors.New("139 登录成功但缺少 RMKEY，请重新登录或补充 Cookie")
	}
	artifact, err := getArtifactWithClient(ctx, state.Client, sid, rmkey)
	if err != nil {
		return "", err
	}
	return thirdPartyLogin(ctx, state.Client, state.Username, artifact)
}

// loginBySMS completes the security-upgrade login using the state retained by
// the preceding password attempt.
func loginBySMS(ctx context.Context, username, smsCode string) (string, error) {
	state := loadPan139LoginState(username)
	if state == nil {
		return "", errors.New("139 登录会话已过期，请先提交账号密码并获取验证码")
	}
	location, sid, err := submitPan139Login(ctx, state, true, smsCode)
	if err != nil {
		return "", err
	}
	if sid == "" {
		if pan139NeedsSMS(location) {
			return "", errors.New("139 短信验证码未通过，请检查验证码")
		}
		return "", errors.New("139 短信登录失败：服务器未返回登录会话")
	}
	deletePan139LoginState(username)
	return finishPan139Login(ctx, state, sid)
}

// RequestPan139SMS sends the second-factor code for a pending password login.
func RequestPan139SMS(ctx context.Context, username string) error {
	state, err := reservePan139SMSSend(username)
	if err != nil {
		return err
	}
	encodedUser, err := rsaEncryptPan139LoginName(state.Username)
	if err != nil {
		return err
	}
	body := "<object>" +
		"<string name=\"loginName\">" + encodedUser + "</string>" +
		"<string name=\"fv\">4</string>" +
		"<string name=\"clientId\">1003</string>" +
		"<string name=\"eMode\">1</string>" +
		"<string name=\"loginFailureUrl\"></string>" +
		"<string name=\"loginSuccessUrl\"></string>" +
		"<string name=\"verifyCode\"></string>" +
		"<string name=\"version\">1.0</string>" +
		"<string name=\"scene\">5</string>" +
		"</object>"
	cguid := fmt.Sprint(time.Now().UnixMilli())
	resp, err := state.Client.Do(ctx, http.MethodPost, mailSMSURL+"?func=login:sendSmsCodeByScene&cguid="+url.QueryEscape(cguid), map[string]string{
		"Accept":       "text/javascript",
		"Content-Type": "application/xml",
		"Origin":       "https://mail.10086.cn",
		"Referer":      "https://mail.10086.cn/default.html",
		"User-Agent":   ua,
	}, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("139 获取短信验证码失败: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("139 获取短信验证码失败：HTTP %d", resp.StatusCode)
	}
	var result struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
	}
	if json.Unmarshal(raw, &result) == nil && strings.EqualFold(result.Code, "S_OK") {
		return nil
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		text = "服务器未确认发送"
	}
	return errors.New("139 获取短信验证码失败：" + truncate(text, 120))
}

func rsaEncryptPan139LoginName(value string) (string, error) {
	der, err := base64.StdEncoding.DecodeString(pan139SMSPhonePublicKey)
	if err != nil {
		return "", err
	}
	key, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return "", err
	}
	rsaKey, ok := key.(*rsa.PublicKey)
	if !ok {
		return "", errors.New("139 登录公钥类型无效")
	}
	ciphertext, err := rsa.EncryptPKCS1v15(rand.Reader, rsaKey, []byte(value))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func pan139NeedsSMS(value string) bool {
	value = strings.ToUpper(value)
	return strings.Contains(value, "EC=S045") || strings.Contains(value, "EC=S046") || strings.Contains(value, "S045") || strings.Contains(value, "S046")
}

// extractPan139SID accepts the response variants emitted by the mail login
// gateway.  Older responses put sid in Location's query string or in the
// Os_SSo_Sid cookie; newer gateways may use a URL fragment, a JSON envelope,
// or a differently-cased SSO cookie.  Keep this parser deliberately narrow:
// only fields/cookies named sid (or its common session-id spelling) are
// accepted, so a JSESSIONID cannot be mistaken for the cloud SSO session.
func extractPan139SID(location string, resp *http.Response, body []byte) string {
	if sid := extractPan139SIDValue(location); sid != "" {
		return sid
	}
	if resp != nil {
		for _, cookie := range resp.Cookies() {
			if pan139SIDCookieName(cookie.Name) && cookie.Value != "" {
				return cookie.Value
			}
		}
		// Be tolerant of a non-standard Set-Cookie line that net/http refuses
		// to parse, while still restricting the cookie name to known SSO names.
		for _, header := range resp.Header.Values("Set-Cookie") {
			if sid := extractPan139SIDCookieHeader(header); sid != "" {
				return sid
			}
		}
	}
	if len(bytesTrimSpace(body)) > 0 {
		if sid := extractPan139SIDJSON(body); sid != "" {
			return sid
		}
		// A few gateways return a bare query-string/HTML fragment instead of
		// JSON.  Reuse the same strict sid= matcher for that representation.
		if sid := extractPan139SIDValue(string(body)); sid != "" {
			return sid
		}
	}
	return ""
}

func extractPan139SIDValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if parsed, err := url.Parse(value); err == nil {
		for _, part := range []string{parsed.RawQuery, parsed.Fragment} {
			if sid := extractPan139SIDQueryPart(part); sid != "" {
				return sid
			}
		}
	}
	// Also handle a plain `sid=...` fragment or an escaped redirect embedded
	// in HTML/JSON.  Do not include quotes, whitespace, or URL delimiters.
	match := regexp.MustCompile(`(?i)(?:^|[?&#\s"'])sid(?:=|%3d)([^&#\s"'<>]+)`).FindStringSubmatch(value)
	if len(match) < 2 {
		return ""
	}
	sid, _ := url.QueryUnescape(match[1])
	return strings.TrimSpace(sid)
}

func extractPan139SIDQueryPart(part string) string {
	part = strings.TrimLeft(strings.TrimSpace(part), "?#")
	if part == "" {
		return ""
	}
	values, err := url.ParseQuery(part)
	if err != nil {
		return ""
	}
	for key, values := range values {
		if !pan139SIDFieldName(key) || len(values) == 0 {
			continue
		}
		if sid := strings.TrimSpace(values[0]); sid != "" {
			return sid
		}
	}
	return ""
}

func extractPan139SIDJSON(body []byte) string {
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return ""
	}
	var walk func(any) string
	walk = func(node any) string {
		switch typed := node.(type) {
		case map[string]any:
			for key, value := range typed {
				if pan139SIDFieldName(key) {
					if sid, ok := value.(string); ok && strings.TrimSpace(sid) != "" {
						return strings.TrimSpace(sid)
					}
				}
			}
			for _, value := range typed {
				if sid := walk(value); sid != "" {
					return sid
				}
			}
		case []any:
			for _, value := range typed {
				if sid := walk(value); sid != "" {
					return sid
				}
			}
		}
		return ""
	}
	return walk(value)
}

func pan139SIDFieldName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "sid", "sessionid", "session_id":
		return true
	default:
		return false
	}
}

func pan139SIDCookieName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "os_sso_sid", "sso_sid", "sid", "sessionid", "session_id":
		return true
	default:
		return false
	}
}

func extractPan139SIDCookieHeader(header string) string {
	for _, part := range strings.Split(header, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || !pan139SIDCookieName(name) {
			continue
		}
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func mergePan139ResponseCookies(existing string, responseCookies []*http.Cookie) string {
	cookies := parseCookieMap(existing)
	for _, cookie := range responseCookies {
		if cookie != nil && cookie.Name != "" && cookie.Value != "" {
			cookies[cookie.Name] = cookie.Value
		}
	}
	return formatCookieMap(cookies)
}

func syncPan139JarCookies(state *pan139LoginState) {
	if state == nil || state.Client == nil || state.Client.HTTP == nil || state.Client.HTTP.Jar == nil {
		return
	}
	u, err := url.Parse(mailHostURL)
	if err != nil {
		return
	}
	state.MailCookies = mergePan139ResponseCookies(state.MailCookies, state.Client.HTTP.Jar.Cookies(u))
}

func bytesTrimSpace(value []byte) []byte { return []byte(strings.TrimSpace(string(value))) }

func getArtifact(ctx context.Context, sid, rmkey string) (string, error) {
	return getArtifactWithClient(ctx, netx.NewClient(60*time.Second), sid, rmkey)
}

func getArtifactWithClient(ctx context.Context, hc *netx.Client, sid, rmkey string) (string, error) {
	if hc == nil {
		return "", errors.New("139 artifact 请求客户端不存在")
	}
	urlValue := fmt.Sprintf("https://smsrebuild1.mail.10086.cn/setting/s?func=%s&sid=%s&cguid=%s",
		url.QueryEscape("umc:getArtifact"), url.QueryEscape(sid), fmt.Sprint(time.Now().UnixMilli()))
	resp, err := hc.Do(ctx, http.MethodPost, urlValue, map[string]string{
		"Host":       "smsrebuild1.mail.10086.cn",
		"Cookie":     "RMKEY=" + rmkey,
		"User-Agent": ua,
		"Accept":     "text/plain, */*",
	}, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 32*1024))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("获取 artifact 失败：HTTP %d", resp.StatusCode)
	}
	text := string(body)
	// artifact is embedded in the response script; parse "getArtifact" value
	m := regexp.MustCompile(`(?s)artifact["']?\s*[:=]\s*["']([^"']+)`).FindStringSubmatch(text)
	if len(m) < 2 {
		m2 := regexp.MustCompile(`(?s)"data"\s*:\s*\{[^}]*"value"\s*:\s*"([^"]+)"`).FindStringSubmatch(text)
		if len(m2) < 2 {
			return "", errors.New("获取 artifact 失败")
		}
		return m2[1], nil
	}
	return m[1], nil
}

// thirdPartyLogin exchanges the artifact through the current encrypted mobile
// cloud SSO endpoint. The older Authorize.ashx endpoint is no longer reliable
// after the 139 security-login upgrade.
func thirdPartyLogin(ctx context.Context, hc *netx.Client, username, artifact string) (string, error) {
	body := map[string]any{
		"clientkey_decrypt": "l3TryM&Q+X7@dzwk)qP",
		"clienttype":        "886",
		"cpid":              "507",
		"dycpwd":            artifact,
		"extInfo":           map[string]any{"ifOpenAccount": "0"},
		"loginMode":         "0",
		"msisdn":            username,
		"pintype":           "13",
		"secinfo":           strings.ToUpper(sha1Hex("fetion.com.cn:" + artifact)),
		"version":           "20250901",
	}
	plain := sortedJSONStringify(body)
	payload := aesCBCEncryptBase64Payload(plain, pan139ThirdLoginKey1)
	if payload == "" {
		return "", errors.New("139 SSO 请求加密失败")
	}
	resp, err := hc.Do(ctx, http.MethodPost, thirdLoginURL, map[string]string{
		"Accept":              "application/json, text/plain, */*",
		"Content-Type":        "text/plain;charset=UTF-8",
		"User-Agent":          "okhttp/3.12.2",
		"hcy-cool-flag":       "1",
		"x-huawei-channelSrc": "10246600",
		"x-sdk-channelSrc":    "",
		"x-MM-Source":         "0",
		"x-UserAgent":         "android|23116PN5BC|android15|1.2.6|||1440x3200|10246600",
		"x-DeviceInfo":        "4|127.0.0.1|5|1.2.6|Xiaomi|23116PN5BC||02-00-00-00-00-00|android 15|1440x3200|android|||",
	}, strings.NewReader(payload))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("139 SSO 失败：HTTP %d", resp.StatusCode)
	}
	plainResponse := raw
	if len(bytesTrimSpace(raw)) == 0 {
		return "", errors.New("139 SSO 返回为空")
	}
	if first := bytesTrimSpace(raw)[0]; first != '{' {
		decoded, decodeErr := aesCBCDecryptFromBase64Payload(string(bytesTrimSpace(raw)), pan139ThirdLoginKey1)
		if decodeErr != nil {
			return "", fmt.Errorf("139 SSO 响应解密失败: %w", decodeErr)
		}
		plainResponse = []byte(decoded)
	}
	var envelope struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(plainResponse, &envelope); err != nil {
		return "", errors.New("139 SSO 响应格式无效")
	}
	if envelope.Data == "" {
		return "", errors.New("139 SSO 未返回授权数据")
	}
	inner, err := aesECBDecryptHex(envelope.Data, pan139ThirdLoginKey2)
	if err != nil {
		return "", fmt.Errorf("139 SSO 授权数据解密失败: %w", err)
	}
	var result struct {
		AuthToken    string `json:"authToken"`
		Account      string `json:"account"`
		UserDomainID string `json:"userDomainId"`
	}
	if err := json.Unmarshal([]byte(inner), &result); err != nil {
		return "", errors.New("139 SSO 授权数据格式无效")
	}
	if result.AuthToken == "" || result.Account == "" || result.UserDomainID == "" {
		return "", errors.New("139 SSO 授权信息不完整")
	}
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("pc:%s:%s", result.Account, result.AuthToken))), nil
}

// authorizeWithArtifact calls the authorize endpoint to get the Basic authorization.
func authorizeWithArtifact(ctx context.Context, sid, artifact string, cookies map[string]string, referer string) (string, error) {
	cguid := fmt.Sprint(time.Now().UnixMilli())
	urlValue := fmt.Sprintf("https://mail.10086.cn/Login/Authorize.ashx?sid=%s&func=%s&cguid=%s",
		url.QueryEscape(sid), url.QueryEscape("umc:authorize"), cguid)
	form := url.Values{}
	form.Set("func", "umc:authorize")
	form.Set("sid", sid)
	form.Set("cguid", cguid)
	form.Set("appId", "33000002")
	form.Set("redirect_uri", "https://yun.139.com/w/")
	form.Set("action", "http://yun.139.com/w/")
	form.Set("userType", "1")
	form.Set("artifact", artifact)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlValue, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://mail.10086.cn")
	req.Header.Set("Referer", "https://mail.10086.cn/")
	req.Header.Set("User-Agent", ua)
	if c := formatCookieMap(cookies); c != "" {
		req.Header.Set("Cookie", c)
	}
	client := &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	text := string(body)
	m := regexp.MustCompile(`(?s)"authorization"\s*:\s*"([^"]+)"`).FindStringSubmatch(text)
	if len(m) < 2 {
		return "", errors.New("获取 authorization 失败：" + truncate(text, 120))
	}
	return m[1], nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func parseCookieMap(raw string) map[string]string {
	m := map[string]string{}
	for _, part := range strings.Split(raw, ";") {
		i := strings.Index(part, "=")
		if i <= 0 {
			continue
		}
		name := strings.TrimSpace(part[:i])
		value := strings.TrimSpace(part[i+1:])
		if name != "" {
			m[name] = value
		}
	}
	return m
}

func formatCookieMap(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k, v := range m {
		if v != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+m[k])
	}
	return strings.Join(parts, "; ")
}
