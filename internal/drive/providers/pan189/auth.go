package pan189

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

// CloudPersonal and CloudFamily select the mounted cloud at login.
const (
	CloudPersonal = "personal"
	CloudFamily   = "family"

	pan189LoginStateTTL = 5 * time.Minute
)

// CaptchaError is returned when the account requires a graphical captcha.
// The data-URL image is available in LastCaptchaImage and must be supplied back
// via the "validate_code" login-field on the next attempt (the pending login
// parameters are cached and reused for that retry).
type CaptchaError struct {
	CaptchaImage string
}

func (e *CaptchaError) Error() string {
	if e.CaptchaImage != "" {
		return fmt.Sprintf("captcha_required_189\nimage=%s", e.CaptchaImage)
	}
	return "该账号需要图形验证码，请输入图片中的字符后重试"
}

// LastCaptchaImage holds the most recent captcha image as a base64 data URL,
// exposed for the login panel to render when a login fails with CaptchaError.
var (
	loginStateMu  sync.Mutex
	pendingLogins = map[string]*pan189LoginState{}
	lastCaptcha   string
)

// CaptchaImage returns the latest captcha image for UI integrations that do
// not carry the username through the login form.
func CaptchaImage() string {
	loginStateMu.Lock()
	defer loginStateMu.Unlock()
	return lastCaptcha
}

// pan189LoginState mirrors the pending login parameters that must be reused
// between fetching the captcha image and submitting the code.
type pan189LoginState struct {
	User          string
	PasswordProof string
	CaptchaToken  string
	AppID         string
	AccountType   string
	ReturnURL     string
	MailSuffix    string
	ClientType    string
	IsOAuth2      bool
	LT            string
	ParamID       string
	ReqID         string
	Referer       string
	RSAPublicKey  string
	RSAPrefix     string
	RsaUsername   string
	RsaPassword   string
	Client        *netx.Client
	CreatedAt     time.Time
}

func pan189PasswordProof(password string) string {
	sum := sha256.Sum256([]byte(password))
	return base64.RawStdEncoding.EncodeToString(sum[:])
}

func newPan189LoginClient() (*netx.Client, error) {
	hc := netx.NewClient(60 * time.Second)
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	hc.HTTP.Jar = jar
	return hc, nil
}

func takePan189PendingLogin(user, password string) (*pan189LoginState, bool) {
	loginStateMu.Lock()
	defer loginStateMu.Unlock()
	for key, state := range pendingLogins {
		if state == nil || time.Since(state.CreatedAt) > pan189LoginStateTTL {
			delete(pendingLogins, key)
		}
	}
	state := pendingLogins[user]
	if state == nil || state.PasswordProof != pan189PasswordProof(password) || state.Client == nil {
		return nil, false
	}
	return state, true
}

func savePan189PendingLogin(state *pan189LoginState, captcha string) {
	if state == nil || state.User == "" {
		return
	}
	loginStateMu.Lock()
	pendingLogins[state.User] = state
	lastCaptcha = captcha
	loginStateMu.Unlock()
}

func deletePan189PendingLogin(user string) {
	loginStateMu.Lock()
	delete(pendingLogins, user)
	loginStateMu.Unlock()
}

// loginWithCreds runs the 189 account+password login flow. validateCode is the
// graphical captcha text when retrying after a CaptchaError.
func loginWithCreds(ctx context.Context, username, password, validateCode string) (*Session, error) {
	user := strings.TrimSpace(username)
	pass := password
	if user == "" || pass == "" {
		return nil, errors.New("请输入天翼云盘账号和密码")
	}
	session, err := doLogin(ctx, user, pass, validateCode)
	if err != nil {
		return nil, err
	}
	return attachLoginCredentials(session, user, pass), nil
}

// attachLoginCredentials keeps the credentials required for the final silent
// re-login when the provider's open token can no longer refresh a session.
func attachLoginCredentials(session *Session, username, password string) *Session {
	if session == nil {
		return nil
	}
	session.Username = strings.TrimSpace(username)
	session.Password = password
	return session
}

func doLogin(ctx context.Context, user, pass, validateCode string) (*Session, error) {
	var state *pan189LoginState
	if validateCode != "" {
		var ok bool
		state, ok = takePan189PendingLogin(user, pass)
		if !ok {
			return nil, errors.New("captcha_expired_189\n189 图形验证码已过期，请重新登录")
		}
	} else {
		deletePan189PendingLogin(user)
		var err error
		state, err = prepareLoginParam(ctx, user, pass)
		if err != nil {
			return nil, err
		}
	}

	toURL, err := loginSubmit(ctx, state, validateCode, false)
	if err != nil {
		var captchaRequired *pan189CaptchaRequiredError
		if errors.As(err, &captchaRequired) {
			if validateCode != "" {
				return nil, errors.New("captcha_retry_189\n189 图形验证码不正确，请重新登录")
			}
			image, imageErr := fetchCaptchaImage(ctx, state)
			if imageErr != nil {
				return nil, imageErr
			}
			savePan189PendingLogin(state, image)
			return nil, &CaptchaError{CaptchaImage: image}
		}
		return nil, err
	}
	deletePan189PendingLogin(user)
	return getSessionForPC(ctx, state, toURL, user)
}

// prepareLoginParam mirrors AList's current 189 login bootstrap: follow
// loginUrl.action, read the dynamic OAuth identifiers from its final URL, then
// obtain appConf and encryptConf before encrypting the credentials.
func prepareLoginParam(ctx context.Context, user, pass string) (*pan189LoginState, error) {
	hc, err := newPan189LoginClient()
	if err != nil {
		return nil, err
	}
	loginURL := webURL + "/api/portal/loginUrl.action?redirectURL=" + url.QueryEscape(webURL+"/main.action")
	resp, err := hc.Do(ctx, http.MethodGet, loginURL, map[string]string{
		"User-Agent": ua189,
		"Accept":     "text/html,application/xhtml+xml",
	}, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		resp.Body.Close()
		return nil, fmt.Errorf("初始化 189 登录参数失败：HTTP %d", resp.StatusCode)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512*1024))
	resp.Body.Close()
	redirectURL := resp.Request.URL
	if redirectURL == nil {
		return nil, errors.New("初始化 189 登录参数失败：未返回登录地址")
	}
	query := redirectURL.Query()
	lt := query.Get("lt")
	reqID := query.Get("reqId")
	dynamicAppID := query.Get("appId")
	captchaToken := query.Get("captchaToken")
	if lt == "" || reqID == "" || dynamicAppID == "" {
		return nil, errors.New("初始化 189 登录参数失败，请稍后重试")
	}
	headers := map[string]string{
		"lt":         lt,
		"reqid":      reqID,
		"Referer":    redirectURL.String(),
		"Origin":     authURL,
		"User-Agent": ua189,
		"Accept":     "application/json",
	}

	appForm := url.Values{}
	appForm.Set("version", "2.0")
	appForm.Set("appKey", dynamicAppID)
	appResp, err := hc.Do(ctx, http.MethodPost, authURL+"/api/logbox/oauth2/appConf.do", withPan189FormHeader(headers), strings.NewReader(appForm.Encode()))
	if err != nil {
		return nil, err
	}
	if appResp.StatusCode < http.StatusOK || appResp.StatusCode >= http.StatusMultipleChoices {
		appResp.Body.Close()
		return nil, fmt.Errorf("获取 189 应用登录配置失败：HTTP %d", appResp.StatusCode)
	}
	var appConf struct {
		Result json.RawMessage `json:"result"`
		Msg    string          `json:"msg"`
		Data   struct {
			AccountType string `json:"accountType"`
			ClientType  int    `json:"clientType"`
			IsOAuth2    bool   `json:"isOauth2"`
			MailSuffix  string `json:"mailSuffix"`
			ParamID     string `json:"paramId"`
			ReqID       string `json:"reqId"`
			ReturnURL   string `json:"returnUrl"`
		} `json:"data"`
	}
	if err := json.NewDecoder(appResp.Body).Decode(&appConf); err != nil {
		appResp.Body.Close()
		return nil, fmt.Errorf("解析 189 应用登录配置失败：%w", err)
	}
	appResp.Body.Close()
	result := strings.Trim(strings.TrimSpace(string(appConf.Result)), `"`)
	if result != "0" {
		return nil, errors.New(firstNonEmpty(appConf.Msg, "获取 189 应用登录配置失败"))
	}
	if appConf.Data.AccountType == "" || appConf.Data.ClientType == 0 || appConf.Data.ParamID == "" || appConf.Data.ReturnURL == "" {
		return nil, errors.New("获取 189 应用登录配置失败：响应字段不完整")
	}
	form := url.Values{}
	form.Set("appId", dynamicAppID)
	cresp, err := hc.Do(ctx, http.MethodPost, authURL+"/api/logbox/config/encryptConf.do", withPan189FormHeader(headers), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	if cresp.StatusCode < http.StatusOK || cresp.StatusCode >= http.StatusMultipleChoices {
		cresp.Body.Close()
		return nil, fmt.Errorf("获取 189 RSA 公钥失败：HTTP %d", cresp.StatusCode)
	}
	var conf struct {
		Result int `json:"result"`
		Data   struct {
			PubKey string `json:"pubKey"`
			Pre    string `json:"pre"`
		} `json:"data"`
	}
	if err := json.NewDecoder(cresp.Body).Decode(&conf); err != nil {
		cresp.Body.Close()
		return nil, err
	}
	cresp.Body.Close()
	if conf.Result != 0 {
		return nil, errors.New("获取 189 RSA 公钥失败：服务端拒绝请求")
	}
	if conf.Data.PubKey == "" {
		return nil, errors.New("获取 189 RSA 公钥失败")
	}
	rsaUser, err := rsaEncrypt(conf.Data.PubKey, user)
	if err != nil {
		return nil, err
	}
	rsaPass, err := rsaEncrypt(conf.Data.PubKey, pass)
	if err != nil {
		return nil, err
	}
	return &pan189LoginState{
		User: user, PasswordProof: pan189PasswordProof(pass), Client: hc, CreatedAt: time.Now(),
		CaptchaToken: captchaToken, AppID: dynamicAppID, AccountType: appConf.Data.AccountType,
		ReturnURL: appConf.Data.ReturnURL, MailSuffix: appConf.Data.MailSuffix,
		ClientType: strconv.Itoa(appConf.Data.ClientType), IsOAuth2: appConf.Data.IsOAuth2,
		LT: lt, ParamID: appConf.Data.ParamID, ReqID: reqID, Referer: redirectURL.String(),
		RSAPublicKey: conf.Data.PubKey, RSAPrefix: conf.Data.Pre,
		RsaUsername: conf.Data.Pre + strings.ToLower(rsaUser),
		RsaPassword: conf.Data.Pre + strings.ToLower(rsaPass),
	}, nil
}

func withPan189FormHeader(headers map[string]string) map[string]string {
	result := make(map[string]string, len(headers)+1)
	for key, value := range headers {
		result[key] = value
	}
	result["Content-Type"] = "application/x-www-form-urlencoded"
	return result
}

// fetchCaptchaImage returns the captcha image as a base64 data URL.
func fetchCaptchaImage(ctx context.Context, st *pan189LoginState) (string, error) {
	if st == nil || st.Client == nil {
		return "", errors.New("189 登录会话已失效，请重新登录")
	}
	q := url.Values{}
	q.Set("token", st.CaptchaToken)
	q.Set("REQID", st.ReqID)
	q.Set("rnd", fmt.Sprintf("%d", timestamp()))
	u := authURL + "/api/logbox/oauth2/picCaptcha.do?" + q.Encode()
	resp, err := st.Client.Do(ctx, http.MethodGet, u, map[string]string{"User-Agent": ua189}, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	buf, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("获取 189 验证码图片失败：HTTP %d", resp.StatusCode)
	}
	if len(buf) <= 20 {
		return "", errors.New("获取 189 验证码图片失败")
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf), nil
}

// loginSubmit sends the RSA-encrypted credentials and returns the redirect URL.
func loginSubmit(ctx context.Context, st *pan189LoginState, validateCode string, sms bool) (string, error) {
	if st == nil || st.Client == nil {
		return "", errors.New("189 登录会话已失效，请重新登录")
	}
	form := url.Values{}
	form.Set("version", "v2.0")
	form.Set("apToken", "")
	form.Set("appKey", st.AppID)
	form.Set("accountType", st.AccountType)
	form.Set("userName", st.RsaUsername)
	form.Set("epd", st.RsaPassword)
	form.Set("captchaType", "")
	form.Set("validateCode", validateCode)
	form.Set("smsValidateCode", "")
	form.Set("captchaToken", st.CaptchaToken)
	form.Set("returnUrl", st.ReturnURL)
	form.Set("mailSuffix", st.MailSuffix)
	if sms {
		form.Set("dynamicCheck", "TRUE")
	} else {
		form.Set("dynamicCheck", "FALSE")
	}
	form.Set("clientType", st.ClientType)
	form.Set("cb_SaveName", "3")
	form.Set("isOauth2", strconv.FormatBool(st.IsOAuth2))
	form.Set("state", "")
	form.Set("paramId", st.ParamID)

	resp, err := st.Client.Do(ctx, http.MethodPost, authURL+"/api/logbox/oauth2/loginSubmit.do", map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"REQID":        st.ReqID,
		"lt":           st.LT,
		"Referer":      st.Referer,
		"Origin":       authURL,
		"User-Agent":   ua189,
		"Accept":       "application/json;charset=UTF-8",
	}, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("189 登录失败：HTTP %d", resp.StatusCode)
	}
	var j map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
		return "", errors.New("189 登录失败（响应异常）")
	}
	toURL := strVal(j, "toUrl")
	if toURL == "" {
		msg := firstNonEmpty(strVal(j, "msg"), strVal(j, "message"), strVal(j, "errorMsg"), strVal(j, "desc"), "189 登录失败（未返回 toUrl）")
		if isPan189CaptchaFailure(msg) {
			if token := strVal(j, "captchaToken"); token != "" {
				st.CaptchaToken = token
			}
			return "", &pan189CaptchaRequiredError{message: msg}
		}
		return "", errors.New(msg)
	}
	return toURL, nil
}

// RequestPan189SMS starts the SMS-login flow used by the current official
// Tianyi account page. The provider requires a dynamic login bootstrap and may
// require its own slider before it will send a code. We only continue when
// smsNeedcaptcha.do explicitly returns 1; a 0 is surfaced to the UI instead of
// attempting to bypass the provider challenge.
func RequestPan189SMS(ctx context.Context, username string) error {
	user := strings.TrimSpace(username)
	if !regexp.MustCompile(`^1\d{10}$`).MatchString(user) {
		return errors.New("请输入有效的 11 位手机号")
	}
	deletePan189PendingLogin(user)
	state, err := prepareLoginParam(ctx, user, "")
	if err != nil {
		return err
	}
	// The code submission must reuse the exact cookies, lt/reqId and RSA key
	// created here. Persist before sendSmsCode so a fast UI submit cannot race
	// with state creation; failed sends remove the unusable state below.
	savePan189PendingLogin(state, "")
	if err := sendPan189SMS(ctx, state); err != nil {
		deletePan189PendingLogin(user)
		return err
	}
	return nil
}

func sendPan189SMS(ctx context.Context, state *pan189LoginState) error {
	if state == nil || state.Client == nil {
		return errors.New("天翼云盘短信登录会话已失效，请重新获取验证码")
	}
	needForm := url.Values{}
	needForm.Set("mobile", state.RsaUsername)
	needForm.Set("appKey", state.AppID)
	resp, err := state.Client.Do(ctx, http.MethodPost, authURL+"/api/logbox/oauth2/smsNeedcaptcha.do", pan189LoginHeaders(state), strings.NewReader(needForm.Encode()))
	if err != nil {
		return fmt.Errorf("检查天翼云盘短信安全验证失败: %w", err)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("检查天翼云盘短信安全验证失败：HTTP %d", resp.StatusCode)
	}
	switch strings.TrimSpace(string(body)) {
	case "1":
		// The official page sends the SMS directly only for this response.
	case "0":
		return errors.New("该手机号发送短信前需要完成天翼账号安全验证，请先在官方登录页完成滑块验证后重试")
	default:
		return errors.New("检查天翼云盘短信安全验证失败：服务器响应异常")
	}

	sendForm := url.Values{}
	sendForm.Set("version", "v2.0")
	sendForm.Set("mobile", state.RsaUsername)
	sendForm.Set("appKey", state.AppID)
	resp, err = state.Client.Do(ctx, http.MethodPost, authURL+"/api/logbox/oauth2/web/sendSmsCode.do", pan189LoginHeaders(state), strings.NewReader(sendForm.Encode()))
	if err != nil {
		return fmt.Errorf("获取天翼云盘短信验证码失败: %w", err)
	}
	body, _ = io.ReadAll(io.LimitReader(resp.Body, 16*1024))
	resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("获取天翼云盘短信验证码失败：HTTP %d", resp.StatusCode)
	}
	var result struct {
		Result json.RawMessage `json:"result"`
		Msg    string          `json:"msg"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return errors.New("获取天翼云盘短信验证码失败：服务器响应异常")
	}
	code, ok := pan189JSONInt(result.Result)
	if !ok {
		return errors.New("获取天翼云盘短信验证码失败：服务器响应异常")
	}
	if code == 0 {
		return nil
	}
	switch code {
	case 20104, 51129:
		return errors.New("短信验证码发送次数过多，请稍后重试")
	case 20107:
		return errors.New("您输入了无效的手机号码，请重新输入")
	case -10320:
		return errors.New("该手机号暂不支持短信验证，请使用账号密码登录")
	default:
		return errors.New(firstNonEmpty(result.Msg, "获取天翼云盘短信验证码失败"))
	}
}

func pan189LoginHeaders(state *pan189LoginState) map[string]string {
	return map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"lt":           state.LT,
		"REQID":        state.ReqID,
		"Referer":      state.Referer,
		"Origin":       authURL,
		"User-Agent":   ua189,
		"Accept":       "application/json;charset=UTF-8",
	}
}

func pan189JSONInt(raw json.RawMessage) (int, bool) {
	text := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if text == "" {
		return 0, false
	}
	value, err := strconv.Atoi(text)
	return value, err == nil
}

func loginWithSMS(ctx context.Context, username, smsCode string) (*Session, error) {
	user := strings.TrimSpace(username)
	code := strings.TrimSpace(smsCode)
	if !regexp.MustCompile(`^1\d{10}$`).MatchString(user) || code == "" {
		return nil, errors.New("请输入手机号和短信验证码")
	}
	state, ok := takePan189PendingLogin(user, "")
	if !ok || state.RSAPublicKey == "" {
		return nil, errors.New("天翼云盘短信登录会话已过期，请重新获取验证码")
	}
	encrypted, err := rsaEncrypt(state.RSAPublicKey, code)
	if err != nil {
		return nil, err
	}
	state.RsaPassword = state.RSAPrefix + strings.ToLower(encrypted)
	toURL, err := loginSubmit(ctx, state, "", true)
	if err != nil {
		return nil, err
	}
	session, err := getSessionForPC(ctx, state, toURL, user)
	if err != nil {
		return nil, err
	}
	deletePan189PendingLogin(user)
	return attachLoginCredentials(session, user, ""), nil
}

type pan189CaptchaRequiredError struct{ message string }

func (e *pan189CaptchaRequiredError) Error() string { return e.message }

func isPan189CaptchaFailure(message string) bool {
	message = strings.ToLower(message)
	return strings.Contains(message, "验证码") || strings.Contains(message, "captcha") || strings.Contains(message, "validatecode")
}

// getSessionForPC exchanges the redirect URL for the API session key/secret.
func getSessionForPC(ctx context.Context, st *pan189LoginState, toURL, loginName string) (*Session, error) {
	if st == nil || st.Client == nil {
		return nil, errors.New("189 登录会话已失效，请重新登录")
	}
	u, _ := url.Parse(apiURL + "/getSessionForPC.action")
	q := u.Query()
	for k, v := range clientSuffix() {
		q.Set(k, v)
	}
	q.Set("redirectURL", toURL)
	u.RawQuery = q.Encode()

	resp, err := st.Client.Do(ctx, http.MethodPost, u.String(), map[string]string{
		"User-Agent": ua189,
		"Accept":     "application/json;charset=UTF-8",
		"Referer":    webURL,
	}, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("获取 189 Session 失败：HTTP %d", resp.StatusCode)
	}
	var j map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
		return nil, errors.New("获取 189 Session 失败")
	}
	resCode := strVal(j, "res_code")
	if resCode != "" && resCode != "0" && strVal(j, "sessionKey") == "" {
		return nil, errors.New(firstNonEmpty(strVal(j, "res_message"), "获取 189 Session 失败"))
	}
	s := &Session{
		SessionKey:          strVal(j, "sessionKey"),
		SessionSecret:       strVal(j, "sessionSecret"),
		FamilySessionKey:    strVal(j, "familySessionKey"),
		FamilySessionSecret: strVal(j, "familySessionSecret"),
		AccessToken:         strVal(j, "accessToken"),
		RefreshToken:        strVal(j, "refreshToken"),
		LoginName:           firstNonEmpty(strVal(j, "loginName"), loginName),
	}
	if s.SessionKey == "" || s.SessionSecret == "" {
		return nil, errors.New("189 Session 不完整")
	}
	return s, nil
}

func pickMatch(text, pattern string) string {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return ""
	}
	m := re.FindStringSubmatch(text)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// login189 implements the provider AuthFunc (账户+密码; optionally 家庭云).
// Form keys: username / password / cloud_type (personal|family) /
// validate_code (captcha retry).
func login189(ctx context.Context, req drive.AuthRequest) (*model.TokenInfo, error) {
	username := strings.TrimSpace(req.Config["username"])
	password := req.Config["password"]
	loginMode := strings.ToLower(strings.TrimSpace(req.Config["login_mode"]))
	cloudType := strings.ToLower(strings.TrimSpace(req.Config["cloud_type"]))
	if cloudType != CloudFamily {
		cloudType = CloudPersonal
	}
	validateCode := req.Config["validate_code"]

	var session *Session
	var err error
	if loginMode == "sms" {
		session, err = loginWithSMS(ctx, username, req.Config["sms_code"])
	} else {
		session, err = loginWithCreds(ctx, username, password, validateCode)
	}
	if err != nil {
		return nil, err
	}
	uid := session.LoginName
	if uid == "" {
		uid = username
	}

	if cloudType == CloudFamily {
		if session.FamilySessionKey == "" || session.FamilySessionSecret == "" {
			return nil, errors.New("该账号未开通家庭云，请先在官方 App 中创建或加入家庭")
		}
		families, err := getFamilyList(ctx, session)
		if err != nil {
			return nil, err
		}
		if len(families) == 0 {
			return nil, errors.New("该账号未加入任何家庭云，请先在官方 App 中创建或加入家庭")
		}
		family := families[0]
		for _, f := range families {
			if f.RemarkName != "" && strings.Contains(uid, f.RemarkName) {
				family = f
				break
			}
		}
		session.CloudType = CloudFamily
		session.FamilyID = family.FamilyID
		session.FamilyName = family.RemarkName
		if session.FamilyName == "" {
			session.FamilyName = "家庭云"
		}
	} else {
		session.CloudType = CloudPersonal
	}

	isFamily := session.CloudType == CloudFamily
	accountID := uid
	if isFamily {
		accountID = uid + "_family"
	}
	name := uid
	tok := &model.TokenInfo{
		TokenFrom:         providerID,
		AccessToken:       session.SessionKey,
		RefreshToken:      mustJSON(session),
		TokenType:         "Session",
		UserID:            model.BuildUserID(providerID, accountID),
		UserName:          name,
		NickName:          name,
		Name:              name,
		Avatar:            "",
		DefaultDriveID:    model.BuildDriveID(providerID, accountID),
		ProviderAccountID: accountID,
		ProviderRootID:    Pan189DefaultFolder,
	}
	return tok, nil
}
