package pan189

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
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
	User         string
	Pass         string
	CaptchaToken string
	LT           string
	ParamID      string
	ReqID        string
	RsaUsername  string
	RsaPassword  string
}

// pendingLogin caches the params of an in-progress login awaiting a captcha.
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
	loginStateMu.Lock()
	state := pendingLogins[user]
	loginStateMu.Unlock()
	if state == nil || state.User != user || state.Pass != pass || validateCode == "" {
		var err error
		state, err = prepareLoginParam(ctx, user, pass)
		if err != nil {
			return nil, err
		}
	}
	loginStateMu.Lock()
	delete(pendingLogins, user)
	loginStateMu.Unlock()

	// needcaptcha: anything other than "0" requires a graphical captcha.
	if validateCode == "" {
		need, err := needCaptcha(ctx, state)
		if err != nil {
			return nil, err
		}
		if need {
			image, err := fetchCaptchaImage(ctx, state)
			if err != nil {
				return nil, err
			}
			loginStateMu.Lock()
			lastCaptcha = image
			pendingLogins[user] = state
			loginStateMu.Unlock()
			return nil, &CaptchaError{CaptchaImage: image}
		}
	}

	toURL, err := loginSubmit(ctx, state, validateCode)
	if err != nil {
		return nil, err
	}
	return getSessionForPC(ctx, toURL, user)
}

// prepareLoginParam fetches the init params (captchaToken/lt/paramId/reqId)
// and the RSA public key, then encrypts the credentials (AList initLoginParam).
func prepareLoginParam(ctx context.Context, user, pass string) (*pan189LoginState, error) {
	hc := netx.NewClient(60 * time.Second)
	// 1) unifyLoginForPC
	ts := timestamp()
	unifyURL := fmt.Sprintf("%s/api/portal/unifyLoginForPC.action?appId=%s&clientType=%s&returnURL=%s&timeStamp=%d",
		webURL, appID, clientType, url.QueryEscape(returnURL), ts)
	resp, err := hc.Do(ctx, http.MethodGet, unifyURL, map[string]string{
		"User-Agent": ua189,
		"Accept":     "text/html,application/xhtml+xml",
	}, nil)
	if err != nil {
		return nil, err
	}
	html, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	page := string(html)
	captchaToken := pickMatch(page, `'captchaToken'\s*value='(.+?)'`)
	if captchaToken == "" {
		captchaToken = pickMatch(page, `captchaToken["']\s*value=["'](.+?)["']`)
	}
	lt := pickMatch(page, `\blt\s*=\s*"([0-9A-Fa-f]{16,})"`)
	paramID := pickMatch(page, `\bparamId\s*=\s*"([0-9A-Fa-f]{16,})"`)
	reqID := pickMatch(page, `reqId\s*=\s*"(.+?)"`)
	if lt == "" || paramID == "" || reqID == "" {
		return nil, errors.New("初始化 189 登录参数失败，请稍后重试")
	}

	// 2) encryptConf → RSA public key + prefix
	form := url.Values{}
	form.Set("appId", appID)
	cresp, err := hc.Do(ctx, http.MethodPost, authURL+"/api/logbox/config/encryptConf.do", map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"User-Agent":   ua189,
		"Accept":       "application/json",
	}, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	var conf struct {
		Data struct {
			PubKey string `json:"pubKey"`
			Pre    string `json:"pre"`
		} `json:"data"`
	}
	if err := json.NewDecoder(cresp.Body).Decode(&conf); err != nil {
		cresp.Body.Close()
		return nil, err
	}
	cresp.Body.Close()
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
		User: user, Pass: pass,
		CaptchaToken: captchaToken, LT: lt, ParamID: paramID, ReqID: reqID,
		RsaUsername: conf.Data.Pre + rsaUser,
		RsaPassword: conf.Data.Pre + rsaPass,
	}, nil
}

// needCaptcha reports whether the account requires a graphical captcha.
func needCaptcha(ctx context.Context, st *pan189LoginState) (bool, error) {
	hc := netx.NewClient(60 * time.Second)
	form := url.Values{}
	form.Set("appKey", appID)
	form.Set("accountType", accountType)
	form.Set("userName", st.RsaUsername)
	resp, err := hc.Do(ctx, http.MethodPost, authURL+"/api/logbox/oauth2/needcaptcha.do", map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"REQID":        st.ReqID,
		"User-Agent":   ua189,
	}, strings.NewReader(form.Encode()))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return strings.TrimSpace(string(body)) != "0", nil
}

// fetchCaptchaImage returns the captcha image as a base64 data URL.
func fetchCaptchaImage(ctx context.Context, st *pan189LoginState) (string, error) {
	hc := netx.NewClient(60 * time.Second)
	q := url.Values{}
	q.Set("token", st.CaptchaToken)
	q.Set("REQID", st.ReqID)
	q.Set("rnd", fmt.Sprintf("%d", timestamp()))
	u := authURL + "/api/logbox/oauth2/picCaptcha.do?" + q.Encode()
	resp, err := hc.Do(ctx, http.MethodGet, u, map[string]string{"User-Agent": ua189}, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	buf, _ := io.ReadAll(resp.Body)
	if len(buf) <= 20 {
		return "", errors.New("获取 189 验证码图片失败")
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf), nil
}

// loginSubmit sends the RSA-encrypted credentials and returns the redirect URL.
func loginSubmit(ctx context.Context, st *pan189LoginState, validateCode string) (string, error) {
	hc := netx.NewClient(60 * time.Second)
	form := url.Values{}
	form.Set("appKey", appID)
	form.Set("accountType", accountType)
	form.Set("userName", st.RsaUsername)
	form.Set("password", st.RsaPassword)
	form.Set("validateCode", validateCode)
	form.Set("captchaToken", st.CaptchaToken)
	form.Set("returnUrl", returnURL)
	form.Set("dynamicCheck", "FALSE")
	form.Set("clientType", clientType)
	form.Set("cb_SaveName", "1")
	form.Set("isOauth2", "false")
	form.Set("state", "")
	form.Set("paramId", st.ParamID)

	resp, err := hc.Do(ctx, http.MethodPost, authURL+"/api/logbox/oauth2/loginSubmit.do", map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"REQID":        st.ReqID,
		"lt":           st.LT,
		"User-Agent":   ua189,
		"Accept":       "application/json;charset=UTF-8",
	}, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var j map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
		return "", errors.New("189 登录失败（响应异常）")
	}
	toURL := strVal(j, "toUrl")
	if toURL == "" {
		msg := firstNonEmpty(strVal(j, "msg"), strVal(j, "message"), "189 登录失败（未返回 toUrl）")
		return "", errors.New(msg)
	}
	return toURL, nil
}

// getSessionForPC exchanges the redirect URL for the API session key/secret.
func getSessionForPC(ctx context.Context, toURL, loginName string) (*Session, error) {
	hc := netx.NewClient(60 * time.Second)
	u, _ := url.Parse(apiURL + "/getSessionForPC.action")
	q := u.Query()
	for k, v := range clientSuffix() {
		q.Set(k, v)
	}
	q.Set("redirectURL", toURL)
	u.RawQuery = q.Encode()

	resp, err := hc.Do(ctx, http.MethodPost, u.String(), map[string]string{
		"User-Agent": ua189,
		"Accept":     "application/json;charset=UTF-8",
		"Referer":    webURL,
	}, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
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
	cloudType := req.Config["cloud_type"]
	if cloudType == "" {
		cloudType = CloudPersonal
	}
	validateCode := req.Config["validate_code"]

	session, err := loginWithCreds(ctx, username, password, validateCode)
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
