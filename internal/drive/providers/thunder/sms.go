package thunder

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

var smsCodePattern = regexp.MustCompile(`^[0-9]{6}$`)

var smsSendTimes = struct {
	sync.Mutex
	entries map[string]time.Time
}{entries: make(map[string]time.Time)}

func reserveSMSSend(username string) error {
	smsSendTimes.Lock()
	defer smsSendTimes.Unlock()
	now := time.Now()
	for phone, sent := range smsSendTimes.entries {
		if now.Sub(sent) >= time.Minute {
			delete(smsSendTimes.entries, phone)
		}
	}
	if err := smsCooldownLocked(username); err != nil {
		return err
	}
	smsSendTimes.entries[username] = now
	return nil
}

func smsCooldownLocked(username string) error {
	if sent, ok := smsSendTimes.entries[username]; ok && time.Since(sent) < time.Minute {
		remaining := int(time.Until(sent.Add(time.Minute)).Seconds()) + 1
		return errors.New("请等待 " + strconv.Itoa(remaining) + " 秒后重新获取迅雷短信验证码")
	}
	return nil
}

func withSMSAttempt(ctx context.Context, request drive.AuthRequest, operation func(context.Context, string, *loginAttempt) (*model.TokenInfo, error)) (*model.TokenInfo, error) {
	username := strings.TrimPrefix(strings.TrimSpace(request.Config["username"]), "+86")
	username = strings.TrimSpace(username)
	if !phonePattern.MatchString(username) {
		return nil, errors.New("请输入正确的 11 位迅雷手机号")
	}
	if request.Config["login_mode"] != "sms" {
		return nil, errors.New("请切换到迅雷短信验证码登录")
	}
	key, attempt, err := getLoginAttempt(request.Config, username, "")
	if err != nil {
		return nil, err
	}
	attempt.mu.Lock()
	defer attempt.mu.Unlock()
	operationCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(attempt.ctx, cancel)
	defer stop()
	defer cancel()
	if attempt.ctx.Err() != nil {
		CancelLogin(key)
		return nil, errors.New("迅雷短信登录会话已取消或过期，请重新获取验证码")
	}
	token, err := operation(operationCtx, key, attempt)
	var failure *apiError
	if code := request.Config["sms_code"]; code != "" && errors.As(err, &failure) {
		failure.Description = strings.ReplaceAll(failure.Description, code, "[已隐藏]")
	}
	var required *VerificationRequiredError
	if errors.As(err, &required) {
		required.AttemptID = key
	} else if err != nil || token != nil {
		CancelLogin(key)
	}
	return token, err
}

func (attempt *loginAttempt) smsCaptcha(ctx context.Context, config map[string]string, action, phase string) error {
	if attempt.challengeURL != "" && attempt.phase == phase {
		if config["captcha_verified"] != "true" {
			return &VerificationRequiredError{URL: attempt.challengeURL}
		}
		if token := config["captcha_token"]; token != "" {
			attempt.client.session.CaptchaToken = token
			attempt.challengeURL, attempt.phase = "", ""
			return nil
		}
	}
	redirectURI := config["captcha_redirect_uri"]
	if redirectURI == "" {
		redirectURI = "xlaccsdk01://xunlei.com/callback?state=harbor"
	}
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	var result struct {
		Token string `json:"captcha_token"`
		URL   string `json:"url"`
	}
	err := attempt.client.raw(ctx, http.MethodPost, authHost+"/shield/captcha/init", map[string]any{
		"action": action, "client_id": clientID, "device_id": attempt.client.session.DeviceID,
		"captcha_token": attempt.client.session.CaptchaToken, "redirect_uri": redirectURI,
		"meta": map[string]string{
			"phone_number": "+86 " + attempt.username, "client_version": clientVersion, "package_name": packageName,
			"timestamp": timestamp, "captcha_sign": captchaSign(attempt.client.session.DeviceID, timestamp),
		},
	}, false, &result)
	if err != nil {
		return err
	}
	if result.Token == "" {
		return errors.New("迅雷短信安全验证缺少临时授权")
	}
	attempt.client.session.CaptchaToken = result.Token
	if result.URL != "" {
		if _, err := VerificationURL(result.URL); err != nil {
			return err
		}
		attempt.challengeURL, attempt.phase = result.URL, phase
		return &VerificationRequiredError{URL: result.URL}
	}
	attempt.challengeURL, attempt.phase = "", ""
	return nil
}

func SendSMS(ctx context.Context, request drive.AuthRequest) (map[string]string, error) {
	var result map[string]string
	_, err := withSMSAttempt(ctx, request, func(ctx context.Context, key string, attempt *loginAttempt) (*model.TokenInfo, error) {
		smsSendTimes.Lock()
		cooldownErr := smsCooldownLocked(attempt.username)
		smsSendTimes.Unlock()
		if cooldownErr != nil {
			return nil, cooldownErr
		}
		if err := attempt.smsCaptcha(ctx, request.Config, "POST:/v1/auth/verification", "sms_send_captcha"); err != nil {
			return nil, err
		}
		if err := reserveSMSSend(attempt.username); err != nil {
			return nil, err
		}
		var response struct {
			ID     string `json:"verification_id"`
			IsUser *bool  `json:"is_user"`
		}
		if err := attempt.client.raw(ctx, http.MethodPost, authHost+"/auth/verification", map[string]string{
			"client_id": clientID, "phone_number": "+86 " + attempt.username, "target": "ANY", "usage": "SIGN_IN",
		}, false, &response); err != nil {
			return nil, err
		}
		if response.ID == "" {
			return nil, errors.New("迅雷未返回短信验证会话，请稍后重试")
		}
		if response.IsUser == nil || !*response.IsUser {
			return nil, errors.New("该手机号未返回已注册的迅雷账号；请先在迅雷官方应用完成注册")
		}
		attempt.smsID, attempt.smsToken = response.ID, ""
		attempt.smsCodeHash = [32]byte{}
		result = map[string]string{"login_attempt": key, "device_id": attempt.client.session.DeviceID, "retry_after": "60"}
		return nil, nil
	})
	return result, err
}

func smsLogin(ctx context.Context, request drive.AuthRequest) (*model.TokenInfo, error) {
	code := strings.TrimSpace(request.Config["sms_code"])
	if !smsCodePattern.MatchString(code) {
		return nil, errors.New("请输入 6 位迅雷短信验证码")
	}
	if request.Config["login_attempt"] == "" {
		return nil, errors.New("请先获取迅雷短信验证码")
	}
	return withSMSAttempt(ctx, request, func(ctx context.Context, _ string, attempt *loginAttempt) (*model.TokenInfo, error) {
		if attempt.smsID == "" {
			return nil, errors.New("请先获取迅雷短信验证码")
		}
		if attempt.smsToken != "" && attempt.smsCodeHash != sha256.Sum256([]byte(code)) {
			return nil, errors.New("迅雷短信验证码已变更，请重新获取验证码")
		}
		if err := attempt.smsCaptcha(ctx, request.Config, "POST:/v1/auth/signin", "sms_signin_captcha"); err != nil {
			return nil, err
		}
		if attempt.smsToken == "" {
			var verified struct {
				Token string `json:"verification_token"`
			}
			if err := attempt.client.raw(ctx, http.MethodPost, authHost+"/auth/verification/verify", map[string]string{
				"client_id": clientID, "verification_id": attempt.smsID, "verification_code": code,
			}, false, &verified); err != nil {
				return nil, err
			}
			if verified.Token == "" {
				return nil, errors.New("迅雷短信校验未返回验证授权")
			}
			attempt.smsToken, attempt.smsCodeHash = verified.Token, sha256.Sum256([]byte(code))
		}
		var authorized struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			UserID       string `json:"user_id"`
			Subject      string `json:"sub"`
			ExpiresIn    int64  `json:"expires_in"`
		}
		for retry := 0; retry < 2; retry++ {
			err := attempt.client.raw(ctx, http.MethodPost, authHost+"/auth/signin", map[string]string{
				"client_id": clientID, "username": "+86 " + attempt.username,
				"verification_code": code, "verification_token": attempt.smsToken,
			}, false, &authorized)
			if err == nil {
				break
			}
			var failure *apiError
			if retry > 0 || !errors.As(err, &failure) || failure.status == http.StatusTooManyRequests || failure.Code != 9 && failure.Kind != "captcha_required" && failure.Kind != "captcha_invalid" {
				return nil, err
			}
			if err := attempt.smsCaptcha(ctx, request.Config, "POST:/v1/auth/signin", "sms_signin_captcha"); err != nil {
				return nil, err
			}
		}
		userID := authorized.UserID
		if userID == "" {
			userID = authorized.Subject
		}
		if authorized.AccessToken == "" || authorized.RefreshToken == "" || userID == "" {
			return nil, errors.New("迅雷短信登录未返回完整账号授权")
		}
		attempt.client.session.UserID, attempt.client.session.RefreshToken = userID, authorized.RefreshToken
		if authorized.ExpiresIn > 0 {
			attempt.client.session.ExpiresAt = time.Now().Unix() + authorized.ExpiresIn
		}
		token := attempt.client.token
		token.AccessToken, token.TokenType, token.ExpiresIn = authorized.AccessToken, "Bearer", authorized.ExpiresIn
		token.UserName, token.Name = "迅雷 "+attempt.username, "迅雷 "+attempt.username
		attempt.client.saveSession()
		return token, nil
	})
}
