package thunder

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/logging"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

const (
	apiHost       = "https://api-pan.xunlei.com/drive/v1"
	authHost      = "https://xluser-ssl.xunlei.com/v1"
	clientID      = "Xp6vsxz_7IYVw2BB"
	clientSecret  = "Xp6vsy4tN9toTVdMSpomVdXpRmES"
	clientVersion = "8.31.0.9726"
	packageName   = "com.xunlei.downloadprovider"
	apiUA         = "ANDROID-com.xunlei.downloadprovider/8.31.0.9726 netWorkType/5G appid/40 deviceName/Xiaomi_M2004j7ac deviceModel/M2004J7AC OSVersion/12 protocolVersion/301 platformVersion/10 sdkVersion/512000 Oauth2Client/0.9 (Linux 4_14_186-perf-gddfs8vbb238b) (JAVA 0)"
	downloadUA    = "Dalvik/2.1.0 (Linux; U; Android 12; M2004J7AC Build/SP1A.210812.016)"
)

var captchaSalts = []string{
	"9uJNVj/wLmdwKrJaVj/omlQ", "Oz64Lp0GigmChHMf/6TNfxx7O9PyopcczMsnf", "Eb+L7Ce+Ej48u", "jKY0",
	"ASr0zCl6v8W4aidjPK5KHd1Lq3t+vBFf41dqv5+fnOd", "wQlozdg6r1qxh0eRmt3QgNXOvSZO6q/GXK",
	"gmirk+ciAvIgA/cxUUCema47jr/YToixTT+Q6O", "5IiCoM9B1/788ntB", "P07JH0h6qoM6TSUAK2aL9T5s2QBVeY9JWvalf", "+oK0AN",
}

type session struct {
	RefreshToken string `json:"refresh_token"`
	DeviceID     string `json:"device_id"`
	UserID       string `json:"user_id"`
	ExpiresAt    int64  `json:"expires_at"`
	CaptchaToken string `json:"captcha_token,omitempty"`
	CaptchaSign  string `json:"captcha_sign,omitempty"`
	Timestamp    string `json:"timestamp,omitempty"`
}

type apiError struct {
	Code            int64           `json:"error_code"`
	Kind            string          `json:"error"`
	Description     string          `json:"error_description"`
	CoreCode        json.RawMessage `json:"errorCode"`
	CoreDescription string          `json:"errorDesc"`
	CreditKey       string          `json:"creditkey"`
	ReviewURL       string          `json:"reviewurl"`
	ReviewUserID    json.RawMessage `json:"userID"`
	ReviewMobile    string          `json:"mobile"`
	status          int
}

func (err *apiError) Error() string {
	message := err.Description
	if message == "" {
		message = err.Kind
	}
	if err.status == http.StatusTooManyRequests {
		message = "请求过于频繁，请稍后重试"
	}
	return fmt.Sprintf("迅雷请求失败（HTTP %d，code=%d）：%s", err.status, err.Code, logging.RedactText(message))
}

type client struct {
	http    *netx.Client
	token   *model.TokenInfo
	session *session
	changed bool
}

var accountLocks sync.Map

func withClient(ctx context.Context, account drive.Context, operation func(*client) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if account.Token == nil {
		return drive.ErrUnauthorized
	}
	key := account.UserID + "\x00" + account.DriveID
	lock, _ := accountLocks.LoadOrStore(key, &sync.Mutex{})
	mutex := lock.(*sync.Mutex)
	mutex.Lock()
	defer mutex.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := drive.ReloadToken(account); err != nil {
		return err
	}
	var stored session
	if json.Unmarshal([]byte(account.Token.RefreshToken), &stored) != nil || stored.RefreshToken == "" || !validDeviceID(stored.DeviceID) || stored.UserID == "" {
		return drive.AuthExpired("迅雷登录信息不完整，请重新登录账号")
	}
	if account.UserID != model.BuildUserID(model.ProviderThunder, stored.UserID) {
		return errors.New("迅雷账号上下文与保存的会话不一致")
	}
	cl := &client{http: netx.NewClient(60 * time.Second), token: account.Token, session: &stored}
	var err error
	if cl.token.AccessToken == "" || stored.ExpiresAt > 0 && stored.ExpiresAt <= time.Now().Unix()+30 {
		err = cl.refresh(ctx)
	}
	if err == nil {
		err = operation(cl)
	}
	if cl.changed {
		cl.saveSession()
		err = errors.Join(err, drive.PersistToken(account))
	}
	return err
}

func validDeviceID(value string) bool {
	if len(value) != 32 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func (cl *client) saveSession() {
	encoded, _ := json.Marshal(cl.session)
	cl.token.RefreshToken = string(encoded)
	cl.token.DeviceID = cl.session.DeviceID
	cl.token.ProviderAccountID = cl.session.UserID
	cl.token.TokenFrom = model.ProviderThunder
	cl.token.UserID = model.BuildUserID(model.ProviderThunder, cl.session.UserID)
	cl.token.ProviderRootID = RootID
}

func (cl *client) raw(ctx context.Context, method, endpoint string, body any, authenticated bool, out any) error {
	headers := map[string]string{
		"Content-Type": "application/json", "Accept": "application/json;charset=UTF-8", "User-Agent": apiUA,
		"X-Device-Id": cl.session.DeviceID, "X-Client-Id": clientID, "X-Client-Version": clientVersion,
	}
	if authenticated {
		headers["Authorization"] = "Bearer " + cl.token.AccessToken
		headers["X-Captcha-Token"] = cl.session.CaptchaToken
	}
	if endpoint == coreLoginURL {
		headers["User-Agent"] = "android-ok-http-client/xl-acc-sdk/version-5.0.12.512000"
	}
	if endpoint == authHost+"/auth/signin/token" || endpoint == authHost+"/auth/signin" || endpoint == authHost+"/auth/verification" {
		headers["X-Captcha-Token"] = cl.session.CaptchaToken
	}
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = strings.NewReader(string(encoded))
	}
	response, err := cl.http.Do(ctx, method, endpoint, headers, reader)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil {
		return err
	}
	if len(data) > 4<<20 {
		return errors.New("迅雷响应超过安全大小上限")
	}
	failure := &apiError{status: response.StatusCode}
	if len(data) > 0 {
		if err := json.Unmarshal(data, failure); err != nil {
			if response.StatusCode >= 400 {
				failure.Description = "服务器返回了非 JSON 错误响应"
				return failure
			}
			return fmt.Errorf("迅雷响应无法解析（HTTP %d）", response.StatusCode)
		}
		if text := strings.Trim(string(failure.CoreCode), "\""); text != "" && text != "null" {
			code, err := strconv.ParseInt(text, 10, 64)
			if err != nil {
				return errors.New("迅雷登录响应的错误码无效")
			}
			if failure.Code == 0 {
				failure.Code = code
			}
		}
		if failure.Description == "" && failure.Code != 0 {
			failure.Description = failure.CoreDescription
		}
	}
	if response.StatusCode >= 400 || failure.Kind != "success" && (failure.Code != 0 || failure.Kind != "" || failure.Description != "") {
		return failure
	}
	if out != nil {
		if response.StatusCode == http.StatusNoContent {
			return nil
		}
		if len(data) == 0 {
			return errors.New("迅雷响应缺少数据")
		}
		return json.Unmarshal(data, out)
	}
	return nil
}

func (cl *client) refresh(ctx context.Context) error {
	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		UserID       string `json:"user_id"`
		Subject      string `json:"sub"`
	}
	err := cl.raw(ctx, http.MethodPost, authHost+"/auth/token", map[string]string{
		"grant_type": "refresh_token", "refresh_token": cl.session.RefreshToken,
		"client_id": clientID, "client_secret": clientSecret,
	}, false, &result)
	if err != nil {
		var failure *apiError
		if errors.As(err, &failure) && (failure.status == http.StatusUnauthorized || failure.Kind == "invalid_grant" || failure.Kind == "invalid_token" || expiredCode(failure.Code)) {
			return drive.AuthExpired("迅雷授权已失效，请重新登录账号")
		}
		return err
	}
	if strings.TrimSpace(result.AccessToken) == "" {
		return errors.New("迅雷刷新响应缺少访问 Token")
	}
	userID := result.UserID
	if userID == "" {
		userID = result.Subject
	}
	if userID != "" && cl.session.UserID != "" && userID != cl.session.UserID {
		return errors.New("迅雷刷新响应的用户与当前账号不一致")
	}
	if userID != "" {
		cl.session.UserID = userID
	}
	if cl.session.UserID == "" {
		return errors.New("迅雷授权缺少账号标识，请重新登录")
	}
	if result.RefreshToken != "" {
		cl.session.RefreshToken = result.RefreshToken
	}
	cl.session.ExpiresAt = 0
	if result.ExpiresIn > 0 {
		cl.session.ExpiresAt = time.Now().Unix() + result.ExpiresIn
	}
	cl.token.AccessToken = result.AccessToken
	cl.token.TokenType = "Bearer"
	cl.token.ExpiresIn = result.ExpiresIn
	cl.changed = true
	cl.saveSession()
	return nil
}

func expiredCode(code int64) bool { return code == 4122 || code == 4121 || code == 10 || code == 16 }

func captchaSign(deviceID, timestamp string) string {
	value := clientID + clientVersion + packageName + deviceID + timestamp
	for _, salt := range captchaSalts {
		digest := md5.Sum([]byte(value + salt))
		value = hex.EncodeToString(digest[:])
	}
	return "1." + value
}

func (cl *client) renewCaptcha(ctx context.Context, action string) error {
	timestamp, sign := cl.session.Timestamp, cl.session.CaptchaSign
	if sign == "" {
		timestamp = strconv.FormatInt(time.Now().UnixMilli(), 10)
		sign = captchaSign(cl.session.DeviceID, timestamp)
	}
	var result struct {
		CaptchaToken string `json:"captcha_token"`
		URL          string `json:"url"`
	}
	err := cl.raw(ctx, http.MethodPost, authHost+"/shield/captcha/init", map[string]any{
		"action": action, "client_id": clientID, "device_id": cl.session.DeviceID,
		"captcha_token": cl.session.CaptchaToken, "redirect_uri": "xlaccsdk01://xunlei.com/callback?state=harbor",
		"meta": map[string]string{"user_id": cl.session.UserID, "client_version": clientVersion, "package_name": packageName, "timestamp": timestamp, "captcha_sign": sign},
	}, false, &result)
	if err != nil {
		return err
	}
	if result.URL != "" {
		return errors.New("迅雷要求重新进行安全验证，请重新登录账号；应用不会自动重复登录")
	}
	if result.CaptchaToken == "" {
		return errors.New("迅雷验证码响应缺少 Captcha Token")
	}
	cl.session.CaptchaToken = result.CaptchaToken
	cl.changed = true
	return nil
}

func (cl *client) request(ctx context.Context, method, path string, body, out any) error {
	refreshed, captchaRenewed := false, false
	for attempt := 0; attempt < 3; attempt++ {
		err := cl.raw(ctx, method, apiHost+path, body, true, out)
		if err == nil {
			return nil
		}
		var failure *apiError
		if !errors.As(err, &failure) || failure.status == http.StatusTooManyRequests {
			return err
		}
		if !refreshed && (failure.status == http.StatusUnauthorized || expiredCode(failure.Code)) {
			refreshed = true
			if err := cl.refresh(ctx); err != nil {
				return err
			}
			continue
		}
		if !captchaRenewed && (failure.Code == 9 || failure.Kind == "captcha_required" || failure.Kind == "captcha_invalid") {
			captchaRenewed = true
			actionPath := strings.SplitN(path, "?", 2)[0]
			if err := cl.renewCaptcha(ctx, method+":/drive/v1"+actionPath); err != nil {
				return err
			}
			continue
		}
		return err
	}
	return errors.New("迅雷请求重试次数已达上限")
}

func authLogin(ctx context.Context, request drive.AuthRequest) (*model.TokenInfo, error) {
	if strings.TrimSpace(request.Config["refresh_token"]) == "" {
		if request.Config["login_mode"] == "sms" {
			return smsLogin(ctx, request)
		}
		if mode := request.Config["login_mode"]; mode != "" && mode != "password" {
			return nil, errors.New("迅雷不支持该登录方式")
		}
		return passwordLogin(ctx, request)
	}
	return refreshTokenLogin(ctx, request)
}

func refreshTokenLogin(ctx context.Context, request drive.AuthRequest) (*model.TokenInfo, error) {
	stored := &session{
		RefreshToken: strings.TrimSpace(request.Config["refresh_token"]), DeviceID: strings.TrimSpace(request.Config["device_id"]),
		UserID: strings.TrimSpace(request.Config["user_id"]), CaptchaToken: strings.TrimSpace(request.Config["captcha_token"]),
		CaptchaSign: strings.TrimSpace(request.Config["captcha_sign"]), Timestamp: strings.TrimSpace(request.Config["timestamp"]),
	}
	if stored.RefreshToken == "" {
		return nil, errors.New("请输入迅雷 Refresh Token")
	}
	if !validDeviceID(stored.DeviceID) {
		return nil, errors.New("迅雷设备 ID 必须为同一客户端的 32 位十六进制 X-Device-Id")
	}
	if (stored.CaptchaSign == "") != (stored.Timestamp == "") {
		return nil, errors.New("Captcha Sign 与签名时间戳必须同时填写")
	}
	if stored.Timestamp != "" {
		if value, err := strconv.ParseInt(stored.Timestamp, 10, 64); err != nil || value <= 0 {
			return nil, errors.New("迅雷签名时间戳必须为正整数")
		}
	}
	cl := &client{http: netx.NewClient(60 * time.Second), token: &model.TokenInfo{}, session: stored}
	if err := cl.refresh(ctx); err != nil {
		return nil, err
	}
	cl.token.UserName = "迅雷 " + stored.UserID
	cl.token.Name = cl.token.UserName
	return cl.token, nil
}
