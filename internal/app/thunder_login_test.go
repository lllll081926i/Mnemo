package app

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"mnemo-go/internal/captcha"
	"mnemo-go/internal/drive/providers/thunder"
	"mnemo-go/internal/netx"
	"mnemo-go/internal/store"
)

func TestThunderPasswordVerificationApplicationFlow(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	application := &App{store: st}
	previous := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = previous; captcha.Close() })
	coreCalls, captchaCalls := 0, 0
	netx.TestTransportHook = favoriteTestTransport(func(request *http.Request) (*http.Response, error) {
		body := `{}`
		switch request.URL.Path {
		case "/xluser.core.login/v3/login":
			coreCalls++
			body = `{"sessionID":"core","userID":"123"}`
		case "/v1/shield/captcha/init":
			captchaCalls++
			var payload map[string]any
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["redirect_uri"] == "" {
				t.Fatal("没有注入应用验证回调")
			}
			if captchaCalls == 1 {
				body = `{"captcha_token":"initial","url":"https://captcha.xunlei.com/verify"}`
			} else {
				body = `{"captcha_token":"verified"}`
			}
		case "/v1/auth/signin/token":
			body = `{"access_token":"access","refresh_token":"refresh","user_id":"123","expires_in":3600}`
		default:
			t.Fatalf("意外请求 %s", request.URL)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})
	config := map[string]string{"username": "13800138000", "password": "private-password"}
	_, err = application.ProviderLogin("thunder", config)
	var required *thunder.VerificationRequiredError
	if !errors.As(err, &required) || !strings.Contains(err.Error(), "\nsession=") {
		t.Fatalf("没有传递验证续办信息：%v", err)
	}
	t.Cleanup(func() { thunder.CancelLogin(required.AttemptID) })
	accounts, _ := st.ListAccounts()
	if len(accounts) != 0 {
		t.Fatal("验证前保存了账号")
	}
	config["login_attempt"], config["captcha_verified"] = required.AttemptID, "true"
	account, err := application.ProviderLogin("thunder", config)
	if err != nil {
		t.Fatal(err)
	}
	if account.UserID != "thunder_123" || coreCalls != 1 || captchaCalls != 2 {
		t.Fatal("账号验证后没有正确续办")
	}
	accounts, err = st.ListAccounts()
	if err != nil || len(accounts) != 1 {
		t.Fatal("授权账号未保存")
	}
	encoded, _ := json.Marshal(accounts)
	if strings.Contains(string(encoded), "private-password") || strings.Contains(string(encoded), "core-session") {
		t.Fatal("账号记录保存了登录密码或临时凭据")
	}
}

func TestThunderSMSSendChallengeApplicationFlow(t *testing.T) {
	application := &App{}
	previous := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = previous; captcha.Close() })
	netx.TestTransportHook = favoriteTestTransport(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/v1/shield/captcha/init" {
			t.Fatal("完成安全验证前发送了短信")
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["redirect_uri"] == "" || payload["action"] != "POST:/v1/auth/verification" {
			t.Fatal("未建立应用拥有的短信验证回调")
		}
		body := `{"captcha_token":"initial-token","url":"https://captcha.xunlei.com/verify"}`
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})
	config := map[string]string{"username": "13800138000", "login_mode": "sms"}
	_, err := application.SendThunderSMS(config)
	var challenge *thunder.VerificationRequiredError
	if !errors.As(err, &challenge) || !strings.Contains(err.Error(), "\nsession=") || challenge.AttemptID == "" {
		t.Fatalf("没有向前端传递短信发送的安全验证会话：%v", err)
	}
	t.Cleanup(func() { thunder.CancelLogin(challenge.AttemptID) })
	if config["captcha_redirect_uri"] != "" {
		t.Fatal("应用修改了调用方的登录表单")
	}
}
