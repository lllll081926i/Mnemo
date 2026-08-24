package pan189

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/netx"
)

type pan189AuthRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn pan189AuthRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func pan189AuthResponse(req *http.Request, status int, headers http.Header, body string) *http.Response {
	if headers == nil {
		headers = make(http.Header)
	}
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     headers,
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

func TestPan189LoginReusesCookieSessionAcrossCaptchaRetry(t *testing.T) {
	oldTransport := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = oldTransport })
	loginStateMu.Lock()
	pendingLogins = map[string]*pan189LoginState{}
	lastCaptcha = ""
	loginStateMu.Unlock()
	t.Cleanup(func() {
		loginStateMu.Lock()
		pendingLogins = map[string]*pan189LoginState{}
		lastCaptcha = ""
		loginStateMu.Unlock()
	})

	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pubKey := base64.StdEncoding.EncodeToString(pubDER)

	const user = "189-test-user"
	const password = "password"
	counts := map[string]int{}
	netx.TestTransportHook = pan189AuthRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		key := req.URL.Host + req.URL.Path
		counts[key]++
		switch key {
		case "cloud.189.cn/api/portal/loginUrl.action":
			headers := make(http.Header)
			headers.Add("Set-Cookie", "cloud_bootstrap=cloud-cookie; Domain=.cloud.189.cn; Path=/; Secure")
			headers.Set("Location", "https://open.e.189.cn/api/logbox/oauth2/login.do?lt=dynamic-lt&reqId=dynamic-request-id&appId=dynamic-app-id&captchaToken=captcha-token")
			return pan189AuthResponse(req, http.StatusFound, headers, ""), nil

		case "open.e.189.cn/api/logbox/oauth2/login.do":
			return pan189AuthResponse(req, http.StatusOK, nil, "login page"), nil

		case "open.e.189.cn/api/logbox/oauth2/appConf.do":
			body, readErr := io.ReadAll(req.Body)
			if readErr != nil {
				return nil, readErr
			}
			form, parseErr := url.ParseQuery(string(body))
			if parseErr != nil {
				return nil, parseErr
			}
			if form.Get("version") != "2.0" || form.Get("appKey") != "dynamic-app-id" {
				return nil, fmt.Errorf("appConf form = %s", form.Encode())
			}
			return pan189AuthResponse(req, http.StatusOK, nil, `{"result":"0","data":{"accountType":"dynamic-account-type","clientType":9876,"isOauth2":true,"mailSuffix":"@189.cn","paramId":"dynamic-param-id","reqId":"app-conf-request-id","returnUrl":"https://cloud.189.cn/dynamic-return"}}`), nil

		case "open.e.189.cn/api/logbox/config/encryptConf.do":
			headers := make(http.Header)
			headers.Add("Set-Cookie", "auth_session=auth-cookie; Domain=.e.189.cn; Path=/; Secure")
			return pan189AuthResponse(req, http.StatusOK, headers, fmt.Sprintf(`{"data":{"pubKey":%q,"pre":"PRE"}}`, pubKey)), nil

		case "open.e.189.cn/api/logbox/oauth2/picCaptcha.do":
			if !strings.Contains(req.Header.Get("Cookie"), "auth_session=auth-cookie") {
				return nil, fmt.Errorf("captcha image lost the encryptConf cookie")
			}
			return pan189AuthResponse(req, http.StatusOK, nil, "012345678901234567890123456789"), nil

		case "open.e.189.cn/api/logbox/oauth2/loginSubmit.do":
			if !strings.Contains(req.Header.Get("Cookie"), "auth_session=auth-cookie") {
				return nil, fmt.Errorf("loginSubmit lost the encryptConf cookie")
			}
			body, readErr := io.ReadAll(req.Body)
			if readErr != nil {
				return nil, readErr
			}
			form, parseErr := url.ParseQuery(string(body))
			if parseErr != nil {
				return nil, parseErr
			}
			if form.Get("version") != "v2.0" || form.Get("epd") == "" || form.Get("password") != "" ||
				form.Get("appKey") != "dynamic-app-id" || form.Get("accountType") != "dynamic-account-type" ||
				form.Get("returnUrl") != "https://cloud.189.cn/dynamic-return" || form.Get("clientType") != "9876" ||
				form.Get("isOauth2") != "true" || form.Get("paramId") != "dynamic-param-id" ||
				form.Get("captchaToken") != "captcha-token" || form.Get("apToken") != "" ||
				form.Get("captchaType") != "" || form.Get("smsValidateCode") != "" {
				return nil, fmt.Errorf("loginSubmit form = %s", form.Encode())
			}
			if req.Header.Get("lt") != "dynamic-lt" || req.Header.Get("reqid") != "dynamic-request-id" || req.Header.Get("Origin") != "https://open.e.189.cn" {
				return nil, fmt.Errorf("loginSubmit headers = %#v", req.Header)
			}
			if form.Get("validateCode") == "" {
				return pan189AuthResponse(req, http.StatusOK, nil, `{"result":-2,"msg":"请输入验证码","captchaToken":"captcha-token"}`), nil
			}
			if form.Get("validateCode") != "ABCD" {
				return nil, fmt.Errorf("loginSubmit validateCode = %q", form.Get("validateCode"))
			}
			return pan189AuthResponse(req, http.StatusOK, nil, `{"toUrl":"https://cloud.189.cn/login/success?ticket=ticket"}`), nil

		case "api.cloud.189.cn/getSessionForPC.action":
			if !strings.Contains(req.Header.Get("Cookie"), "cloud_bootstrap=cloud-cookie") {
				return nil, fmt.Errorf("getSessionForPC lost the cloud.189.cn cookie")
			}
			return pan189AuthResponse(req, http.StatusOK, nil, `{"res_code":"0","sessionKey":"session-key","sessionSecret":"session-secret","loginName":"189-test-user"}`), nil
		default:
			return nil, fmt.Errorf("unexpected 189 auth request %s %s", req.Method, req.URL.String())
		}
	})

	_, err = loginWithCreds(context.Background(), user, password, "")
	var captchaErr *CaptchaError
	if !errors.As(err, &captchaErr) {
		t.Fatalf("first login error = %v, want CaptchaError", err)
	}
	if captchaErr.CaptchaImage == "" {
		t.Fatal("CaptchaError did not carry the image")
	}
	if counts["cloud.189.cn/api/portal/loginUrl.action"] != 1 || counts["open.e.189.cn/api/logbox/oauth2/appConf.do"] != 1 || counts["open.e.189.cn/api/logbox/config/encryptConf.do"] != 1 {
		t.Fatalf("initial request counts = %#v", counts)
	}

	session, err := loginWithCreds(context.Background(), user, password, "ABCD")
	if err != nil {
		t.Fatalf("captcha retry login error = %v", err)
	}
	if session.SessionKey != "session-key" || session.SessionSecret != "session-secret" {
		t.Fatalf("session = %#v", session)
	}
	if session.Username != user || session.Password != password {
		t.Fatalf("attached credentials = %q/%q", session.Username, session.Password)
	}
	if counts["cloud.189.cn/api/portal/loginUrl.action"] != 1 || counts["open.e.189.cn/api/logbox/oauth2/appConf.do"] != 1 || counts["open.e.189.cn/api/logbox/config/encryptConf.do"] != 1 || counts["open.e.189.cn/api/logbox/oauth2/picCaptcha.do"] != 1 {
		t.Fatalf("captcha retry restarted the login flow: %#v", counts)
	}
	if counts["open.e.189.cn/api/logbox/oauth2/loginSubmit.do"] != 2 || counts["api.cloud.189.cn/getSessionForPC.action"] != 1 {
		t.Fatalf("completion request counts = %#v", counts)
	}
	loginStateMu.Lock()
	_, pending := pendingLogins[user]
	loginStateMu.Unlock()
	if pending {
		t.Fatal("successful captcha retry left a pending login state")
	}
}

func TestPan189NormalLoginDoesNotPreflightCaptcha(t *testing.T) {
	oldTransport := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = oldTransport })

	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pubKey := base64.StdEncoding.EncodeToString(pubDER)

	needCaptchaCalls := 0
	netx.TestTransportHook = pan189AuthRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Host + req.URL.Path {
		case "cloud.189.cn/api/portal/loginUrl.action":
			h := make(http.Header)
			h.Set("Location", "https://open.e.189.cn/login?lt=lt-live&reqId=req-live&appId=app-live")
			return pan189AuthResponse(req, http.StatusFound, h, ""), nil
		case "open.e.189.cn/login":
			return pan189AuthResponse(req, http.StatusOK, nil, "login"), nil
		case "open.e.189.cn/api/logbox/oauth2/appConf.do":
			return pan189AuthResponse(req, http.StatusOK, nil, `{"result":"0","data":{"accountType":"02-live","clientType":10020,"isOauth2":false,"paramId":"param-live","returnUrl":"https://cloud.189.cn/return-live"}}`), nil
		case "open.e.189.cn/api/logbox/config/encryptConf.do":
			return pan189AuthResponse(req, http.StatusOK, nil, fmt.Sprintf(`{"result":0,"data":{"pubKey":%q,"pre":"PRE"}}`, pubKey)), nil
		case "open.e.189.cn/api/logbox/oauth2/needcaptcha.do":
			needCaptchaCalls++
			return pan189AuthResponse(req, http.StatusOK, nil, "1"), nil
		case "open.e.189.cn/api/logbox/oauth2/loginSubmit.do":
			return pan189AuthResponse(req, http.StatusOK, nil, `{"result":0,"msg":"刷新页面后重试","toUrl":"https://cloud.189.cn/success"}`), nil
		case "api.cloud.189.cn/getSessionForPC.action":
			return pan189AuthResponse(req, http.StatusOK, nil, `{"res_code":"0","sessionKey":"session-key","sessionSecret":"session-secret","loginName":"user"}`), nil
		default:
			return nil, fmt.Errorf("unexpected 189 auth request %s %s", req.Method, req.URL.String())
		}
	})

	session, err := loginWithCreds(context.Background(), "user", "password", "")
	if err != nil {
		t.Fatalf("login error = %v", err)
	}
	if session.SessionKey != "session-key" {
		t.Fatalf("session = %#v", session)
	}
	if needCaptchaCalls != 0 {
		t.Fatalf("normal login called needcaptcha %d times", needCaptchaCalls)
	}
}

func TestPan189CaptchaFailureMarker(t *testing.T) {
	for _, message := range []string{"验证码错误", "captcha invalid", "validateCode mismatch"} {
		if !isPan189CaptchaFailure(message) {
			t.Fatalf("isPan189CaptchaFailure(%q) = false", message)
		}
	}
	if isPan189CaptchaFailure("账号或密码错误") {
		t.Fatal("ordinary credential failure was classified as a captcha failure")
	}
	if isPan189CaptchaFailure("刷新页面后重试") {
		t.Fatal("refresh-page response was classified as a captcha failure")
	}
}

func TestPan189SMSLoginUsesOfficialDynamicFlow(t *testing.T) {
	oldTransport := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = oldTransport })
	loginStateMu.Lock()
	pendingLogins = map[string]*pan189LoginState{}
	loginStateMu.Unlock()
	t.Cleanup(func() {
		loginStateMu.Lock()
		pendingLogins = map[string]*pan189LoginState{}
		loginStateMu.Unlock()
	})

	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pubKey := base64.StdEncoding.EncodeToString(pubDER)
	const user = "18900000000"
	const smsCode = "654321"
	counts := map[string]int{}

	decrypt := func(ciphertext string) string {
		t.Helper()
		ciphertext = strings.TrimPrefix(ciphertext, "PRE")
		raw, decodeErr := hex.DecodeString(ciphertext)
		if decodeErr != nil {
			t.Fatalf("decode RSA ciphertext: %v", decodeErr)
		}
		plain, decryptErr := rsa.DecryptPKCS1v15(rand.Reader, privateKey, raw)
		if decryptErr != nil {
			t.Fatalf("decrypt RSA ciphertext: %v", decryptErr)
		}
		return string(plain)
	}

	netx.TestTransportHook = pan189AuthRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		key := req.URL.Host + req.URL.Path
		counts[key]++
		switch key {
		case "cloud.189.cn/api/portal/loginUrl.action":
			h := make(http.Header)
			h.Set("Location", "https://open.e.189.cn/login?lt=sms-lt&reqId=sms-req&appId=sms-app")
			return pan189AuthResponse(req, http.StatusFound, h, ""), nil
		case "open.e.189.cn/login":
			return pan189AuthResponse(req, http.StatusOK, nil, "login"), nil
		case "open.e.189.cn/api/logbox/oauth2/appConf.do":
			return pan189AuthResponse(req, http.StatusOK, nil, `{"result":"0","data":{"accountType":"02","clientType":10020,"isOauth2":false,"paramId":"sms-param","returnUrl":"https://cloud.189.cn/sms-return"}}`), nil
		case "open.e.189.cn/api/logbox/config/encryptConf.do":
			return pan189AuthResponse(req, http.StatusOK, nil, fmt.Sprintf(`{"data":{"pubKey":%q,"pre":"PRE"}}`, pubKey)), nil
		case "open.e.189.cn/api/logbox/oauth2/smsNeedcaptcha.do":
			body, _ := io.ReadAll(req.Body)
			form, _ := url.ParseQuery(string(body))
			if form.Get("appKey") != "sms-app" || decrypt(form.Get("mobile")) != user {
				return nil, fmt.Errorf("smsNeedcaptcha form = %s", form.Encode())
			}
			return pan189AuthResponse(req, http.StatusOK, nil, "1"), nil
		case "open.e.189.cn/api/logbox/oauth2/web/sendSmsCode.do":
			loginStateMu.Lock()
			_, savedBeforeSend := pendingLogins[user]
			loginStateMu.Unlock()
			if !savedBeforeSend {
				return nil, errors.New("SMS login state was not saved before sendSmsCode")
			}
			body, _ := io.ReadAll(req.Body)
			form, _ := url.ParseQuery(string(body))
			if form.Get("version") != "v2.0" || form.Get("appKey") != "sms-app" || decrypt(form.Get("mobile")) != user {
				return nil, fmt.Errorf("sendSmsCode form = %s", form.Encode())
			}
			return pan189AuthResponse(req, http.StatusOK, nil, `{"result":0,"msg":"success"}`), nil
		case "open.e.189.cn/api/logbox/oauth2/loginSubmit.do":
			body, _ := io.ReadAll(req.Body)
			form, _ := url.ParseQuery(string(body))
			if form.Get("dynamicCheck") != "TRUE" || form.Get("smsValidateCode") != "" || decrypt(form.Get("epd")) != smsCode {
				return nil, fmt.Errorf("SMS loginSubmit form = %s", form.Encode())
			}
			return pan189AuthResponse(req, http.StatusOK, nil, `{"result":0,"toUrl":"https://cloud.189.cn/sms-success"}`), nil
		case "api.cloud.189.cn/getSessionForPC.action":
			return pan189AuthResponse(req, http.StatusOK, nil, `{"res_code":"0","sessionKey":"sms-session","sessionSecret":"sms-secret","loginName":"18900000000"}`), nil
		default:
			return nil, fmt.Errorf("unexpected 189 SMS request %s %s", req.Method, req.URL.String())
		}
	})

	if err := RequestPan189SMS(context.Background(), user); err != nil {
		t.Fatalf("RequestPan189SMS() error = %v", err)
	}
	token, err := login189(context.Background(), drive.AuthRequest{Config: map[string]string{
		"login_mode": "sms", "username": user, "sms_code": smsCode, "cloud_type": CloudPersonal,
	}})
	if err != nil {
		t.Fatalf("SMS login error = %v", err)
	}
	if token.AccessToken != "sms-session" || token.UserID == "" {
		t.Fatalf("SMS token = %#v", token)
	}
	if counts["open.e.189.cn/api/logbox/oauth2/smsNeedcaptcha.do"] != 1 || counts["open.e.189.cn/api/logbox/oauth2/web/sendSmsCode.do"] != 1 || counts["open.e.189.cn/api/logbox/oauth2/loginSubmit.do"] != 1 {
		t.Fatalf("SMS request counts = %#v", counts)
	}
	loginStateMu.Lock()
	_, pending := pendingLogins[user]
	loginStateMu.Unlock()
	if pending {
		t.Fatal("successful SMS login left a pending state")
	}
}

func TestPan189SMSStopsWhenOfficialSliderIsRequired(t *testing.T) {
	state, err := newPan189LoginClient()
	if err != nil {
		t.Fatal(err)
	}
	login := &pan189LoginState{User: "18900000000", AppID: "app", RsaUsername: "encrypted-mobile", LT: "lt", ReqID: "req", Client: state, CreatedAt: time.Now()}
	oldTransport := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = oldTransport })
	sendCalls := 0
	netx.TestTransportHook = pan189AuthRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/logbox/oauth2/smsNeedcaptcha.do":
			return pan189AuthResponse(req, http.StatusOK, nil, "0"), nil
		case "/api/logbox/oauth2/web/sendSmsCode.do":
			sendCalls++
			return pan189AuthResponse(req, http.StatusOK, nil, `{"result":0}`), nil
		default:
			return nil, fmt.Errorf("unexpected request %s", req.URL.String())
		}
	})

	err = sendPan189SMS(context.Background(), login)
	if err == nil || !strings.Contains(err.Error(), "安全验证") || !strings.Contains(err.Error(), "官方登录页") {
		t.Fatalf("sendPan189SMS() error = %v", err)
	}
	if sendCalls != 0 {
		t.Fatalf("sendSmsCode was called %d times after slider requirement", sendCalls)
	}
}
