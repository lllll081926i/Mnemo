package thunder

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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

const coreLoginURL = "https://xluser-ssl.xunlei.com/xluser.core.login/v3/login"

var phonePattern = regexp.MustCompile(`^1[0-9]{10}$`)

type VerificationRequiredError struct {
	URL       string
	AttemptID string
}

func (err *VerificationRequiredError) Error() string {
	return "thunder_verification_required\nurl=" + err.URL + "\nattempt=" + err.AttemptID
}

type loginAttempt struct {
	mu           sync.Mutex
	username     string
	loginMode    string
	passwordHash [32]byte
	created      time.Time
	client       *client
	coreSession  string
	creditKey    string
	nickname     string
	challengeURL string
	phase        string
	ctx          context.Context
	cancel       context.CancelFunc
	smsID        string
	smsToken     string
	smsCodeHash  [32]byte
}

var loginAttempts = struct {
	sync.Mutex
	entries map[string]*loginAttempt
}{entries: make(map[string]*loginAttempt)}

func randomID() (string, error) {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(data[:]), nil
}

func CancelLogin(attemptID string) {
	loginAttempts.Lock()
	attempt := loginAttempts.entries[attemptID]
	delete(loginAttempts.entries, attemptID)
	loginAttempts.Unlock()
	if attempt != nil {
		attempt.cancel()
	}
}

func getLoginAttempt(config map[string]string, username, password string) (string, *loginAttempt, error) {
	mode := config["login_mode"]
	if mode == "" {
		mode = "password"
	}
	loginAttempts.Lock()
	defer loginAttempts.Unlock()
	for key, attempt := range loginAttempts.entries {
		if time.Since(attempt.created) > 10*time.Minute {
			attempt.cancel()
			delete(loginAttempts.entries, key)
		}
	}
	if key := config["login_attempt"]; key != "" {
		attempt := loginAttempts.entries[key]
		if attempt == nil {
			return "", nil, errors.New("迅雷验证会话已过期，请重新登录")
		}
		if attempt.username != username || attempt.loginMode != mode || attempt.passwordHash != sha256.Sum256([]byte(password)) || config["device_id"] != "" && config["device_id"] != attempt.client.session.DeviceID {
			return "", nil, errors.New("迅雷登录信息已变更，请重新登录")
		}
		return key, attempt, nil
	}
	if len(loginAttempts.entries) >= 64 {
		return "", nil, errors.New("迅雷待验证会话过多，请关闭旧验证窗口后重试")
	}
	deviceID := config["device_id"]
	var err error
	if deviceID == "" {
		deviceID, err = randomID()
	}
	if err != nil {
		return "", nil, err
	}
	if !validDeviceID(deviceID) {
		return "", nil, errors.New("迅雷设备初始化失败，请重新打开登录窗口")
	}
	key, err := randomID()
	if err != nil {
		return "", nil, err
	}
	httpClient := netx.NewClient(60 * time.Second)
	httpClient.HTTP.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	attemptCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	attempt := &loginAttempt{
		username: username, loginMode: mode, passwordHash: sha256.Sum256([]byte(password)), created: time.Now(),
		client: &client{http: httpClient, token: &model.TokenInfo{}, session: &session{DeviceID: deviceID}},
		ctx:    attemptCtx, cancel: cancel,
	}
	loginAttempts.entries[key] = attempt
	return key, attempt, nil
}

func deviceSign(deviceID string) string {
	digest := sha1.Sum([]byte(deviceID + packageName + "40" + "34a062aaa22f906fca4fefe9fb3a3021"))
	checksum := md5.Sum([]byte(hex.EncodeToString(digest[:])))
	return "div101." + deviceID + hex.EncodeToString(checksum[:])
}

func VerificationURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Port() != "" && parsed.Port() != "443" {
		return nil, errors.New("迅雷返回了无效的安全验证地址")
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "xunlei.com" && !strings.HasSuffix(host, ".xunlei.com") {
		return nil, errors.New("迅雷验证页面不是官方域名")
	}
	return parsed, nil
}

func (attempt *loginAttempt) coreLogin(ctx context.Context, password, redirectURI string) error {
	var result struct {
		SessionID string `json:"sessionID"`
		UserID    string `json:"userID"`
		Nickname  string `json:"nickName"`
	}
	err := attempt.client.raw(ctx, http.MethodPost, coreLoginURL, map[string]string{
		"protocolVersion": "301", "sequenceNo": "1000012", "platformVersion": "10", "isCompressed": "0",
		"appid": "40", "clientVersion": clientVersion, "peerID": "00000000000000000000000000000000",
		"appName": "ANDROID-" + packageName, "sdkVersion": "512000", "devicesign": deviceSign(attempt.client.session.DeviceID),
		"netWorkType": "WIFI", "providerName": "NONE", "deviceModel": "M2004J7AC", "deviceName": "Xiaomi_M2004j7ac",
		"OSVersion": "12", "creditkey": attempt.creditKey, "hl": "zh-CN", "userName": attempt.username,
		"passWord": password, "verifyKey": "", "verifyCode": "", "isMd5Pwd": "0",
	}, false, &result)
	if err != nil {
		var failure *apiError
		if errors.As(err, &failure) {
			failure.Description = strings.ReplaceAll(failure.Description, password, "[已隐藏]")
			if failure.Kind == "review_panel" {
				parsed, urlErr := VerificationURL(failure.ReviewURL)
				if urlErr != nil {
					return urlErr
				}
				if failure.CreditKey == "" {
					return errors.New("迅雷安全验证缺少临时会话，请稍后重新登录")
				}
				attempt.creditKey = failure.CreditKey
				query := parsed.Query()
				query.Set("appid", "40")
				query.Set("appName", "ANDROID-"+packageName)
				query.Set("clientVersion", clientVersion)
				query.Set("sdkVersion", "512000")
				query.Set("creditkey", failure.CreditKey)
				query.Set("hl", "zh-CN")
				if query.Get("mobile") == "" {
					mobile := failure.ReviewMobile
					if mobile == "" {
						mobile = attempt.username
					}
					query.Set("mobile", mobile)
				}
				if query.Get("userID") == "" {
					var userID json.Number
					if json.Unmarshal(failure.ReviewUserID, &userID) == nil && userID.String() != "" {
						query.Set("userID", userID.String())
					}
				}
				query.Set("deviceid", deviceSign(attempt.client.session.DeviceID))
				query.Set("redirect_uri", redirectURI)
				parsed.RawQuery = query.Encode()
				attempt.challengeURL, attempt.phase = parsed.String(), "review"
				return &VerificationRequiredError{URL: attempt.challengeURL}
			}
		}
		return err
	}
	if result.SessionID == "" || result.UserID == "" {
		return errors.New("迅雷登录响应缺少会话或用户标识")
	}
	attempt.coreSession, attempt.nickname = result.SessionID, result.Nickname
	attempt.client.session.UserID = result.UserID
	return nil
}

func (attempt *loginAttempt) loginCaptcha(ctx context.Context, redirectURI string) error {
	var result struct {
		Token string `json:"captcha_token"`
		URL   string `json:"url"`
	}
	err := attempt.client.raw(ctx, http.MethodPost, authHost+"/shield/captcha/init", map[string]any{
		"action": "POST:/v1/auth/signin/token", "client_id": clientID, "device_id": attempt.client.session.DeviceID,
		"captcha_token": attempt.client.session.CaptchaToken, "redirect_uri": redirectURI,
		"meta": map[string]string{"phone_number": attempt.username},
	}, false, &result)
	if err != nil {
		return err
	}
	if result.Token == "" {
		return errors.New("迅雷验证响应缺少临时会话")
	}
	attempt.client.session.CaptchaToken = result.Token
	if result.URL != "" {
		if _, err := VerificationURL(result.URL); err != nil {
			return err
		}
		attempt.challengeURL, attempt.phase = result.URL, "captcha"
		return &VerificationRequiredError{URL: result.URL}
	}
	return nil
}

func passwordLogin(ctx context.Context, request drive.AuthRequest) (*model.TokenInfo, error) {
	username := strings.TrimPrefix(strings.TrimSpace(request.Config["username"]), "+86")
	password := request.Config["password"]
	if !phonePattern.MatchString(username) {
		return nil, errors.New("请输入正确的 11 位迅雷手机号")
	}
	if password == "" {
		return nil, errors.New("请输入迅雷账号密码")
	}
	key, attempt, err := getLoginAttempt(request.Config, username, password)
	if err != nil {
		return nil, err
	}
	attempt.mu.Lock()
	defer attempt.mu.Unlock()
	operationCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(attempt.ctx, cancel)
	defer stop()
	defer cancel()
	ctx = operationCtx
	if err := attempt.ctx.Err(); err != nil {
		CancelLogin(key)
		return nil, errors.New("迅雷验证会话已取消或过期，请重新登录")
	}
	keep := false
	defer func() {
		if !keep {
			CancelLogin(key)
		}
	}()
	challenge := func(err error) (*model.TokenInfo, error) {
		var required *VerificationRequiredError
		if errors.As(err, &required) {
			required.AttemptID = key
			keep = true
		}
		return nil, err
	}
	verified := request.Config["captcha_verified"] == "true"
	if attempt.challengeURL != "" && !verified {
		return challenge(&VerificationRequiredError{URL: attempt.challengeURL})
	}
	redirectURI := request.Config["captcha_redirect_uri"]
	if redirectURI == "" {
		redirectURI = "xlaccsdk01://xunlei.com/callback?state=harbor"
	}
	if attempt.coreSession == "" {
		if verified && attempt.phase == "review" && request.Config["creditkey"] != "" {
			attempt.creditKey = request.Config["creditkey"]
		}
		if err := attempt.coreLogin(ctx, password, redirectURI); err != nil {
			return challenge(err)
		}
	}
	if verified && attempt.phase == "captcha" && request.Config["captcha_token"] != "" {
		attempt.client.session.CaptchaToken = request.Config["captcha_token"]
	} else if err := attempt.loginCaptcha(ctx, redirectURI); err != nil {
		return challenge(err)
	}
	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		UserID       string `json:"user_id"`
		Subject      string `json:"sub"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	for retry := 0; retry < 2; retry++ {
		err = attempt.client.raw(ctx, http.MethodPost, authHost+"/auth/signin/token", map[string]string{
			"client_id": clientID, "client_secret": clientSecret, "provider": "access_end_point_token", "signin_token": attempt.coreSession,
		}, false, &result)
		if err == nil {
			break
		}
		var failure *apiError
		if retry > 0 || !errors.As(err, &failure) || failure.status == http.StatusTooManyRequests || failure.Code != 9 && failure.Kind != "captcha_required" && failure.Kind != "captcha_invalid" {
			return nil, err
		}
		if err := attempt.loginCaptcha(ctx, redirectURI); err != nil {
			return challenge(err)
		}
	}
	if err != nil {
		return nil, err
	}
	if result.AccessToken == "" || result.RefreshToken == "" {
		return nil, errors.New("迅雷登录未返回完整授权，请稍后重试")
	}
	userID := result.UserID
	if userID == "" {
		userID = result.Subject
	}
	if userID != "" && userID != attempt.client.session.UserID {
		return nil, errors.New("迅雷授权与登录账号不一致")
	}
	attempt.client.session.RefreshToken = result.RefreshToken
	if result.ExpiresIn > 0 {
		attempt.client.session.ExpiresAt = time.Now().Unix() + result.ExpiresIn
	}
	token := attempt.client.token
	token.AccessToken, token.TokenType, token.ExpiresIn = result.AccessToken, "Bearer", result.ExpiresIn
	token.UserName = attempt.nickname
	if token.UserName == "" {
		token.UserName = "迅雷 " + username
	}
	token.Name = token.UserName
	attempt.client.saveSession()
	return token, nil
}
