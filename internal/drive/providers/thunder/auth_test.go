package thunder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"mnemo-go/internal/drive"
)

func phoneConfig() map[string]string {
	return map[string]string{"username": "13800138000", "password": "test-password", "device_id": "0123456789abcdef0123456789abcdef"}
}

func TestPhonePasswordLoginAndMetadata(t *testing.T) {
	fields := drive.Require("thunder").Login.Fields
	if len(fields) != 4 || fields[0].Key != "login_mode" || len(fields[0].Options) != 2 || fields[1].Key != "username" || fields[2].Key != "password" || fields[2].Type != "password" || fields[3].Key != "sms_code" {
		t.Fatal("普通登录入口暴露了协议参数")
	}
	calls := 0
	mockHTTP(t, func(request *http.Request) (*http.Response, error) {
		calls++
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if !validDeviceID(request.Header.Get("X-Device-Id")) {
			t.Fatal("未自动初始化设备")
		}
		switch request.URL.Path {
		case "/xluser.core.login/v3/login":
			if body["userName"] != "13800138000" || body["passWord"] != "test-password" || body["isMd5Pwd"] != "0" || body["devicesign"] != deviceSign(request.Header.Get("X-Device-Id")) || !strings.Contains(request.Header.Get("User-Agent"), "xl-acc-sdk") {
				t.Fatal("密码登录参数不正确")
			}
			return response(request, 200, `{"sessionID":"core-session","userID":"123","nickName":"测试账号","errorCode":"0"}`), nil
		case "/v1/shield/captcha/init":
			meta := body["meta"].(map[string]any)
			if meta["phone_number"] != "13800138000" || body["action"] != "POST:/v1/auth/signin/token" {
				t.Fatal("登录安全验证参数不正确")
			}
			return response(request, 200, `{"captcha_token":"captcha-ready"}`), nil
		case "/v1/auth/signin/token":
			if body["provider"] != "access_end_point_token" || body["signin_token"] != "core-session" || request.Header.Get("X-Captcha-Token") != "captcha-ready" {
				t.Fatal("登录授权交换不正确")
			}
			return response(request, 200, `{"access_token":"access","refresh_token":"refresh","user_id":"123","expires_in":3600}`), nil
		}
		return nil, fmt.Errorf("意外请求 %s", request.URL)
	})
	config := phoneConfig()
	delete(config, "device_id")
	token, err := authLogin(context.Background(), drive.AuthRequest{Config: config})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 || token.UserID != "thunder_123" || token.UserName != "测试账号" || !validDeviceID(token.DeviceID) || strings.Contains(token.RefreshToken, "test-password") || strings.Contains(token.RefreshToken, "core-session") {
		t.Fatalf("登录结果错误：%+v", token)
	}
}

func TestPasswordCaptchaContinuesWithoutResendingPassword(t *testing.T) {
	for _, callback := range []bool{false, true} {
		t.Run(fmt.Sprint(callback), func(t *testing.T) {
			coreCalls, captchaCalls, signins := 0, 0, 0
			mockHTTP(t, func(request *http.Request) (*http.Response, error) {
				switch request.URL.Path {
				case "/xluser.core.login/v3/login":
					coreCalls++
					return response(request, 200, `{"sessionID":"core","userID":"123"}`), nil
				case "/v1/shield/captcha/init":
					captchaCalls++
					if captchaCalls == 1 {
						return response(request, 200, `{"captcha_token":"initial","url":"https://captcha.xunlei.com/verify"}`), nil
					}
					return response(request, 200, `{"captcha_token":"verified"}`), nil
				case "/v1/auth/signin/token":
					signins++
					if request.Header.Get("X-Captcha-Token") != "verified" {
						t.Fatal("未使用完成验证后的会话")
					}
					return response(request, 200, `{"access_token":"access","refresh_token":"refresh","sub":"123"}`), nil
				}
				return nil, fmt.Errorf("意外请求")
			})
			config := phoneConfig()
			_, err := authLogin(context.Background(), drive.AuthRequest{Config: config})
			var required *VerificationRequiredError
			if !errors.As(err, &required) || signins != 0 {
				t.Fatal("没有等待人工验证")
			}
			t.Cleanup(func() { CancelLogin(required.AttemptID) })
			config["login_attempt"], config["captcha_verified"] = required.AttemptID, "true"
			if callback {
				config["captcha_token"] = "verified"
			}
			if _, err := authLogin(context.Background(), drive.AuthRequest{Config: config}); err != nil {
				t.Fatal(err)
			}
			if coreCalls != 1 || signins != 1 || callback && captchaCalls != 1 || !callback && captchaCalls != 2 {
				t.Fatalf("重复登录：core=%d captcha=%d signin=%d", coreCalls, captchaCalls, signins)
			}
		})
	}
}

func TestPasswordReviewRetainsCreditKey(t *testing.T) {
	for _, creditKey := range []string{"", "verified-review-key"} {
		t.Run("callback="+creditKey, func(t *testing.T) {
			coreCalls := 0
			mockHTTP(t, func(request *http.Request) (*http.Response, error) {
				switch request.URL.Path {
				case "/xluser.core.login/v3/login":
					coreCalls++
					if coreCalls == 1 {
						return response(request, 200, `{"error":"review_panel","errorCode":"100","creditkey":"review-key","reviewurl":"https://i.xunlei.com/xlcaptcha/vertifyPhone.html?userID=123&mobile=138****8000&trigger=1"}`), nil
					}
					var body map[string]string
					json.NewDecoder(request.Body).Decode(&body)
					expected := creditKey
					if expected == "" {
						expected = "review-key"
					}
					if body["creditkey"] != expected {
						t.Fatal("丢失安全验证会话")
					}
					return response(request, 200, `{"sessionID":"core","userID":"123"}`), nil
				case "/v1/shield/captcha/init":
					return response(request, 200, `{"captcha_token":"verified"}`), nil
				case "/v1/auth/signin/token":
					return response(request, 200, `{"access_token":"access","refresh_token":"refresh"}`), nil
				}
				return nil, fmt.Errorf("意外请求")
			})
			config := phoneConfig()
			_, err := authLogin(context.Background(), drive.AuthRequest{Config: config})
			var required *VerificationRequiredError
			if !errors.As(err, &required) || !strings.Contains(required.URL, "deviceid=") {
				t.Fatal("安全验证地址未正确构造")
			}
			t.Cleanup(func() { CancelLogin(required.AttemptID) })
			parsed, err := url.Parse(required.URL)
			if err != nil {
				t.Fatal(err)
			}
			query := parsed.Query()
			for key, expected := range map[string]string{
				"appid": "40", "appName": "ANDROID-" + packageName, "clientVersion": clientVersion,
				"creditkey": "review-key", "userID": "123", "mobile": "138****8000", "trigger": "1",
				"deviceid": deviceSign(config["device_id"]), "redirect_uri": "xlaccsdk01://xunlei.com/callback?state=harbor",
			} {
				if query.Get(key) != expected {
					t.Fatalf("验证参数 %s 不正确", key)
				}
			}
			config["login_attempt"], config["captcha_verified"] = required.AttemptID, "true"
			config["creditkey"] = creditKey
			if _, err := authLogin(context.Background(), drive.AuthRequest{Config: config}); err != nil {
				t.Fatal(err)
			}
			if coreCalls != 2 {
				t.Fatal("安全验证后未按原会话继续登录")
			}
		})
	}
}

func TestPasswordReviewUserIDFromResponse(t *testing.T) {
	for _, userID := range []string{`"123"`, `123`} {
		t.Run(userID, func(t *testing.T) {
			mockHTTP(t, func(request *http.Request) (*http.Response, error) {
				return response(request, 200, `{"error":"review_panel","creditkey":"review-key","userID":`+userID+`,"reviewurl":"https://i.xunlei.com/xlcaptcha/vertifyPhone.html"}`), nil
			})
			_, err := authLogin(context.Background(), drive.AuthRequest{Config: phoneConfig()})
			var required *VerificationRequiredError
			if !errors.As(err, &required) {
				t.Fatal(err)
			}
			t.Cleanup(func() { CancelLogin(required.AttemptID) })
			parsed, _ := url.Parse(required.URL)
			if parsed.Query().Get("userID") != "123" || parsed.Query().Get("mobile") != "13800138000" {
				t.Fatal("未补齐服务端账号标识与登录手机号")
			}
		})
	}
}

func TestPasswordAttemptBindingExpiryAndCancellation(t *testing.T) {
	for _, change := range []string{"account", "password", "device", "expired", "cancelled"} {
		t.Run(change, func(t *testing.T) {
			config := phoneConfig()
			key, attempt, err := getLoginAttempt(config, config["username"], config["password"])
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { CancelLogin(key) })
			config["login_attempt"] = key
			switch change {
			case "account":
				config["username"] = "13900139000"
			case "password":
				config["password"] = "changed"
			case "device":
				config["device_id"] = strings.Repeat("a", 32)
			case "expired":
				attempt.created = time.Now().Add(-11 * time.Minute)
			case "cancelled":
				CancelLogin(key)
			}
			mockHTTP(t, func(*http.Request) (*http.Response, error) {
				t.Fatal("无效会话发出了登录请求")
				return nil, nil
			})
			if _, err := authLogin(context.Background(), drive.AuthRequest{Config: config}); err == nil {
				t.Fatal("接受了无效登录会话")
			}
		})
	}
}

func TestPasswordRejectsCredentialsRedirectAndRateLimit(t *testing.T) {
	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusTooManyRequests, http.StatusOK} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			mockHTTP(t, func(request *http.Request) (*http.Response, error) {
				calls++
				result := response(request, status, `{"errorCode":"6","errorDesc":"密码错误 test-password"}`)
				if status == 307 {
					result.Header.Set("Location", "https://evil.test/login")
				}
				return result, nil
			})
			_, err := authLogin(context.Background(), drive.AuthRequest{Config: phoneConfig()})
			if err == nil || calls != 1 || strings.Contains(err.Error(), "test-password") {
				t.Fatalf("错误处理或凭据保护异常：%v，请求=%d", err, calls)
			}
		})
	}
}

func TestThunderVerificationURLRejectsOtherOrigins(t *testing.T) {
	for _, raw := range []string{"https://xunlei.com.evil.test/verify", "http://captcha.xunlei.com/verify", "https://user:password@xunlei.com/verify", "https://xunlei.com:8080/verify"} {
		if _, err := VerificationURL(raw); err == nil {
			t.Fatalf("接受了非官方验证地址 %s", raw)
		}
	}
}

func TestPasswordSigninCaptchaRetryIsBounded(t *testing.T) {
	for _, outcome := range []string{"success", "captcha_invalid", "rate_limit"} {
		t.Run(outcome, func(t *testing.T) {
			coreCalls, captchaCalls, signins := 0, 0, 0
			mockHTTP(t, func(request *http.Request) (*http.Response, error) {
				switch request.URL.Path {
				case "/xluser.core.login/v3/login":
					coreCalls++
					return response(request, 200, `{"sessionID":"core","userID":"123"}`), nil
				case "/v1/shield/captcha/init":
					captchaCalls++
					return response(request, 200, fmt.Sprintf(`{"captcha_token":"captcha-%d"}`, captchaCalls)), nil
				case "/v1/auth/signin/token":
					signins++
					if request.Header.Get("X-Captcha-Token") != fmt.Sprintf("captcha-%d", captchaCalls) {
						t.Fatal("重试未使用新的验证会话")
					}
					if outcome == "rate_limit" {
						return response(request, 429, `{"error":"captcha_invalid","error_code":9}`), nil
					}
					if outcome == "captcha_invalid" || signins == 1 {
						return response(request, 400, `{"error":"captcha_invalid","error_code":9}`), nil
					}
					return response(request, 200, `{"access_token":"access","refresh_token":"refresh","sub":"123"}`), nil
				}
				return nil, fmt.Errorf("意外请求 %s", request.URL)
			})
			_, err := authLogin(context.Background(), drive.AuthRequest{Config: phoneConfig()})
			if (err == nil) != (outcome == "success") {
				t.Fatalf("登录结果不正确：%v", err)
			}
			wantAttempts := 2
			if outcome == "rate_limit" {
				wantAttempts = 1
			}
			if coreCalls != 1 || captchaCalls != wantAttempts || signins != wantAttempts {
				t.Fatalf("登录重试次数错误：core=%d captcha=%d signin=%d", coreCalls, captchaCalls, signins)
			}
		})
	}
}

func TestPasswordCancellationStopsPendingRequest(t *testing.T) {
	started := make(chan struct{})
	mockHTTP(t, func(request *http.Request) (*http.Response, error) {
		close(started)
		<-request.Context().Done()
		return nil, request.Context().Err()
	})
	config := phoneConfig()
	key, _, err := getLoginAttempt(config, config["username"], config["password"])
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { CancelLogin(key) })
	config["login_attempt"] = key
	result := make(chan error, 1)
	go func() {
		_, err := authLogin(context.Background(), drive.AuthRequest{Config: config})
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("登录请求未启动")
	}
	CancelLogin(key)
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("取消没有停止登录请求：%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("取消后登录请求仍在等待")
	}
}
