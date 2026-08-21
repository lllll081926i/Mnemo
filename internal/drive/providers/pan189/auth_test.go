package pan189

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

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
		case "cloud.189.cn/api/portal/unifyLoginForPC.action":
			headers := make(http.Header)
			headers.Add("Set-Cookie", "cloud_bootstrap=cloud-cookie; Domain=.cloud.189.cn; Path=/; Secure")
			page := `<input 'captchaToken' value='captcha-token'><script>var lt = "0123456789abcdef"; var paramId = "abcdef0123456789"; var reqId = "request-id";</script>`
			return pan189AuthResponse(req, http.StatusOK, headers, page), nil

		case "open.e.189.cn/api/logbox/config/encryptConf.do":
			headers := make(http.Header)
			headers.Add("Set-Cookie", "auth_session=auth-cookie; Domain=.e.189.cn; Path=/; Secure")
			return pan189AuthResponse(req, http.StatusOK, headers, fmt.Sprintf(`{"data":{"pubKey":%q,"pre":"PRE"}}`, pubKey)), nil

		case "open.e.189.cn/api/logbox/oauth2/needcaptcha.do":
			if !strings.Contains(req.Header.Get("Cookie"), "auth_session=auth-cookie") {
				return nil, fmt.Errorf("needcaptcha lost the encryptConf cookie")
			}
			return pan189AuthResponse(req, http.StatusOK, nil, "1"), nil

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
			if form.Get("validateCode") != "ABCD" || form.Get("captchaToken") != "captcha-token" || form.Get("paramId") != "abcdef0123456789" {
				return nil, fmt.Errorf("loginSubmit form = %s", form.Encode())
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
	if counts["cloud.189.cn/api/portal/unifyLoginForPC.action"] != 1 || counts["open.e.189.cn/api/logbox/config/encryptConf.do"] != 1 {
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
	if counts["cloud.189.cn/api/portal/unifyLoginForPC.action"] != 1 || counts["open.e.189.cn/api/logbox/config/encryptConf.do"] != 1 || counts["open.e.189.cn/api/logbox/oauth2/needcaptcha.do"] != 1 || counts["open.e.189.cn/api/logbox/oauth2/picCaptcha.do"] != 1 {
		t.Fatalf("captcha retry restarted the login flow: %#v", counts)
	}
	if counts["open.e.189.cn/api/logbox/oauth2/loginSubmit.do"] != 1 || counts["api.cloud.189.cn/getSessionForPC.action"] != 1 {
		t.Fatalf("completion request counts = %#v", counts)
	}
	loginStateMu.Lock()
	_, pending := pendingLogins[user]
	loginStateMu.Unlock()
	if pending {
		t.Fatal("successful captcha retry left a pending login state")
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
}
