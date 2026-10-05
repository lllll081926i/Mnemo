package thunder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"mnemo-go/internal/drive"
)

func prepareSMSConfig(t *testing.T) map[string]string {
	t.Helper()
	smsSendTimes.Lock()
	previous := smsSendTimes.entries
	smsSendTimes.entries = make(map[string]time.Time)
	smsSendTimes.Unlock()
	t.Cleanup(func() {
		smsSendTimes.Lock()
		smsSendTimes.entries = previous
		smsSendTimes.Unlock()
	})
	config := phoneConfig()
	delete(config, "password")
	config["login_mode"] = "sms"
	return config
}

func TestSMSLoginOfficialProtocol(t *testing.T) {
	config := prepareSMSConfig(t)
	sends, verifies, signins := 0, 0, 0
	mockHTTP(t, func(request *http.Request) (*http.Response, error) {
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if _, exists := body["password"]; exists {
			t.Fatal("短信登录发送了密码")
		}
		switch request.URL.Path {
		case "/v1/shield/captcha/init":
			if body["action"] != "POST:/v1/auth/verification" && body["action"] != "POST:/v1/auth/signin" {
				t.Fatal("短信登录使用了错误的风控操作")
			}
			return response(request, 200, `{"captcha_token":"action-captcha"}`), nil
		case "/v1/auth/verification":
			sends++
			if body["phone_number"] != "+86 13800138000" || body["usage"] != "SIGN_IN" || body["target"] != "ANY" || request.Header.Get("X-Captcha-Token") != "action-captcha" {
				t.Fatal("短信发送参数与官方登录协议不符")
			}
			return response(request, 200, `{"verification_id":"sms-id","is_user":true}`), nil
		case "/v1/auth/verification/verify":
			verifies++
			if body["verification_id"] != "sms-id" || body["verification_code"] != "123456" || body["client_id"] != clientID {
				t.Fatal("短信校验会话丢失")
			}
			return response(request, 200, `{"verification_token":"sms-verified-token"}`), nil
		case "/v1/auth/signin":
			signins++
			if body["username"] != "+86 13800138000" || body["verification_token"] != "sms-verified-token" || body["verification_code"] != "123456" || request.Header.Get("X-Captcha-Token") != "action-captcha" {
				t.Fatal("短信授权参数不正确")
			}
			return response(request, 200, `{"access_token":"access","refresh_token":"refresh","sub":"123","expires_in":3600}`), nil
		}
		return nil, fmt.Errorf("意外请求 %s", request.URL.Path)
	})
	result, err := SendSMS(context.Background(), drive.AuthRequest{Config: config})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { CancelLogin(result["login_attempt"]) })
	config["login_attempt"], config["sms_code"] = result["login_attempt"], "123456"
	token, err := authLogin(context.Background(), drive.AuthRequest{Config: config})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(token)
	if token.UserID != "thunder_123" || sends != 1 || verifies != 1 || signins != 1 || strings.Contains(string(encoded), "sms-verified-token") || strings.Contains(string(encoded), `"sms_code"`) || strings.Contains(string(encoded), "sms-id") {
		t.Fatal("短信登录结果错误或持久化了临时验证凭据")
	}
}

func TestSMSVisualVerificationPhases(t *testing.T) {
	config := prepareSMSConfig(t)
	sends, verifies, captchaCalls := 0, 0, 0
	mockHTTP(t, func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/v1/shield/captcha/init":
			captchaCalls++
			return response(request, 200, `{"captcha_token":"initial-token","url":"https://captcha.xunlei.com/verify"}`), nil
		case "/v1/auth/verification":
			sends++
			if request.Header.Get("X-Captcha-Token") != "send-verified-token" {
				t.Fatal("未携带发送前的可视验证结果")
			}
			return response(request, 200, `{"verification_id":"sms-id","is_user":true}`), nil
		case "/v1/auth/verification/verify":
			verifies++
			return response(request, 200, `{"verification_token":"verified"}`), nil
		case "/v1/auth/signin":
			if request.Header.Get("X-Captcha-Token") != "signin-verified-token" {
				t.Fatal("登录验证与发送验证未隔离")
			}
			return response(request, 200, `{"access_token":"access","refresh_token":"refresh","sub":"123"}`), nil
		}
		return nil, fmt.Errorf("意外请求")
	})
	_, err := SendSMS(context.Background(), drive.AuthRequest{Config: config})
	var required *VerificationRequiredError
	if !errors.As(err, &required) || sends != 0 {
		t.Fatal("未在发送短信前完成风控")
	}
	t.Cleanup(func() { CancelLogin(required.AttemptID) })
	config["login_attempt"], config["captcha_verified"], config["captcha_token"] = required.AttemptID, "true", "send-verified-token"
	if _, err := SendSMS(context.Background(), drive.AuthRequest{Config: config}); err != nil {
		t.Fatal(err)
	}
	delete(config, "captcha_verified")
	delete(config, "captcha_token")
	config["sms_code"] = "123456"
	_, err = authLogin(context.Background(), drive.AuthRequest{Config: config})
	if !errors.As(err, &required) || verifies != 0 || sends != 1 {
		t.Fatal("登录风控前消费了短信验证码或重复发送短信")
	}
	config["captcha_verified"], config["captcha_token"] = "true", "signin-verified-token"
	if _, err := authLogin(context.Background(), drive.AuthRequest{Config: config}); err != nil {
		t.Fatal(err)
	}
	if sends != 1 || verifies != 1 || captchaCalls != 2 {
		t.Fatal("短信流程重复请求或重建了风控会话")
	}
}

func TestSMSVerifiedCodeIsNotReplayedAfterSigninChallenge(t *testing.T) {
	config := prepareSMSConfig(t)
	verifies, signins, captchaCalls := 0, 0, 0
	mockHTTP(t, func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/v1/shield/captcha/init":
			captchaCalls++
			if captchaCalls == 3 {
				return response(request, 200, `{"captcha_token":"captcha","url":"https://captcha.xunlei.com/verify"}`), nil
			}
			return response(request, 200, `{"captcha_token":"captcha"}`), nil
		case "/v1/auth/verification":
			return response(request, 200, `{"verification_id":"sms-id","is_user":true}`), nil
		case "/v1/auth/verification/verify":
			verifies++
			return response(request, 200, `{"verification_token":"verified"}`), nil
		case "/v1/auth/signin":
			signins++
			if signins == 1 {
				return response(request, 403, `{"error":"captcha_required","error_code":9}`), nil
			}
			if request.Header.Get("X-Captcha-Token") != "verified-captcha-token" {
				t.Fatal("登录续办未携带可视验证结果")
			}
			return response(request, 200, `{"access_token":"access","refresh_token":"refresh","sub":"123"}`), nil
		}
		return nil, fmt.Errorf("意外请求 %s", request.URL.Path)
	})
	result, err := SendSMS(context.Background(), drive.AuthRequest{Config: config})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { CancelLogin(result["login_attempt"]) })
	config["login_attempt"], config["sms_code"] = result["login_attempt"], "123456"
	_, err = authLogin(context.Background(), drive.AuthRequest{Config: config})
	var required *VerificationRequiredError
	if !errors.As(err, &required) {
		t.Fatal("授权风控未要求可视验证")
	}
	config["captcha_verified"], config["captcha_token"] = "true", "verified-captcha-token"
	if _, err := authLogin(context.Background(), drive.AuthRequest{Config: config}); err != nil {
		t.Fatal(err)
	}
	if verifies != 1 || signins != 2 || captchaCalls != 3 {
		t.Fatal("登录续办重复消费短信验证码或重建了验证会话")
	}
}

func TestSMSResendLimitSurvivesCancellation(t *testing.T) {
	config := prepareSMSConfig(t)
	sends := 0
	mockHTTP(t, func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/v1/shield/captcha/init" {
			return response(request, 200, `{"captcha_token":"captcha"}`), nil
		}
		sends++
		return response(request, 200, `{"verification_id":"sms-id","is_user":true}`), nil
	})
	result, err := SendSMS(context.Background(), drive.AuthRequest{Config: config})
	if err != nil {
		t.Fatal(err)
	}
	CancelLogin(result["login_attempt"])
	if _, err := SendSMS(context.Background(), drive.AuthRequest{Config: config}); err == nil || !strings.Contains(err.Error(), "等待") || sends != 1 {
		t.Fatal("切换登录方式绕过了短信重发限制")
	}
}

func TestSMSRejectsInvalidAndIncompleteResponses(t *testing.T) {
	for _, phase := range []string{"send", "unregistered", "verify", "signin", "rate-limit"} {
		t.Run(phase, func(t *testing.T) {
			config := prepareSMSConfig(t)
			signins := 0
			mockHTTP(t, func(request *http.Request) (*http.Response, error) {
				switch request.URL.Path {
				case "/v1/shield/captcha/init":
					return response(request, 200, `{"captcha_token":"captcha"}`), nil
				case "/v1/auth/verification":
					if phase == "send" {
						return response(request, 200, `{}`), nil
					}
					if phase == "unregistered" {
						return response(request, 200, `{"verification_id":"sms-id","is_user":false}`), nil
					}
					return response(request, 200, `{"verification_id":"sms-id","is_user":true}`), nil
				case "/v1/auth/verification/verify":
					if phase == "verify" {
						return response(request, 200, `{}`), nil
					}
					return response(request, 200, `{"verification_token":"verified"}`), nil
				case "/v1/auth/signin":
					signins++
					if phase == "rate-limit" {
						return response(request, 429, `{"error":"captcha_required"}`), nil
					}
					return response(request, 200, `{"access_token":"access"}`), nil
				}
				t.Fatal("短信登录尝试创建账号或调用了其他端点")
				return nil, nil
			})
			result, err := SendSMS(context.Background(), drive.AuthRequest{Config: config})
			if phase == "send" || phase == "unregistered" {
				if err == nil || result != nil {
					t.Fatal("未完整发送或未注册的账号被视为成功")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { CancelLogin(result["login_attempt"]) })
			config["login_attempt"], config["sms_code"] = result["login_attempt"], "123456"
			if token, err := authLogin(context.Background(), drive.AuthRequest{Config: config}); err == nil || token != nil {
				t.Fatal("不完整或被限流的短信授权被视为成功")
			}
			if phase == "rate-limit" && signins != 1 {
				t.Fatal("收到 429 后重复登录")
			}
		})
	}
}

func TestSMSAttemptRejectsChangedIdentityAndMethod(t *testing.T) {
	for _, change := range []string{"account", "device", "mode", "cancelled", "expired"} {
		t.Run(change, func(t *testing.T) {
			config := prepareSMSConfig(t)
			key, attempt, err := getLoginAttempt(config, config["username"], "")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { CancelLogin(key) })
			attempt.smsID = "sms-id"
			config["login_attempt"], config["sms_code"] = key, "123456"
			switch change {
			case "account":
				config["username"] = "13900139000"
			case "device":
				config["device_id"] = strings.Repeat("a", 32)
			case "mode":
				config["login_mode"], config["password"] = "password", "secret-password"
			case "cancelled":
				CancelLogin(key)
			case "expired":
				attempt.created = time.Now().Add(-11 * time.Minute)
			}
			mockHTTP(t, func(*http.Request) (*http.Response, error) {
				t.Fatal("已失效的短信会话发出了请求")
				return nil, nil
			})
			if _, err := authLogin(context.Background(), drive.AuthRequest{Config: config}); err == nil {
				t.Fatal("接受了已变更或失效的短信登录会话")
			}
		})
	}
}
