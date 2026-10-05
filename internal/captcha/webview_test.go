package captcha

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestThunderCreditKeyCallback(t *testing.T) {
	results := make(chan VerificationResult, 2)
	session, err := StartForProviderWithResult("thunder", func(_ Session, result VerificationResult) { results <- result })
	if err != nil {
		t.Fatal(err)
	}
	defer Close()
	key := "verified-key+with/slashes=="
	raw := "xlaccsdk01://xunlei.com/callback?state=harbor&creditkey=" + url.QueryEscape(key)
	result, ok := callbackResult(*session, raw)
	if !ok || result.CreditKey != key || result.CaptchaToken != "" {
		t.Fatal("信任参数丢失或被当作验证码 Token")
	}
	fragmentResult, ok := callbackResult(*session, session.CallbackURL+"#creditkey="+url.QueryEscape(key))
	if !ok || fragmentResult != result {
		t.Fatal("未解析浏览器片段中的信任参数")
	}
	if _, ok := callbackResult(*session, strings.Replace(raw, "state=harbor", "state=other", 1)); ok {
		t.Fatal("接受了无关的迅雷回调")
	}
	response, err := http.Get(session.CallbackURL + "?creditkey=" + url.QueryEscape(key))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	select {
	case got := <-results:
		if got != result {
			t.Fatal("本地回调未传递验证信任参数")
		}
	case <-time.After(time.Second):
		t.Fatal("验证结果没有送达")
	}
	if _, accepted := completeResult(session.ID, result); accepted {
		t.Fatal("重复验证结果被接受")
	}
	pikpak := Session{Provider: "pikpak", CallbackURL: session.CallbackURL}
	if got, _ := callbackResult(pikpak, pikpak.CallbackURL+"?creditkey="+url.QueryEscape(key)); got.CreditKey != "" {
		t.Fatal("PikPak 会话接受了迅雷信任参数")
	}
}

func TestThunderVerificationSessionIsolation(t *testing.T) {
	results := make(chan string, 1)
	session, err := StartForProvider("thunder", func(_ Session, token string) { results <- token })
	if err != nil {
		t.Fatal(err)
	}
	defer Close()
	token := strings.Repeat("verified", 5)
	if _, ok := callbackToken(*session, "xlaccsdk01://xbase.cloud/callback?captcha_token="+token); ok {
		t.Fatal("迅雷会话接受了 PikPak 回调")
	}
	if _, ok := callbackToken(*session, "xlaccsdk01://xunlei.com/callback?state=harbor&captcha_token="+token); !ok {
		t.Fatal("迅雷原生回调被拒绝")
	}
	if acceptReport(session.ID, "https://user.mypikpak.com/credit/v1/report", []byte(`{"captcha_token":"`+token+`"}`)) {
		t.Fatal("迅雷会话接受了其他网盘的验证结果")
	}
	if acceptReport(session.ID, "https://xunlei.com.evil.test/credit/v1/report", []byte(`{"captcha_token":"`+token+`"}`)) {
		t.Fatal("接受了仿冒迅雷域名")
	}
	if !acceptReport(session.ID, "https://xluser-ssl.xunlei.com/credit/v1/report", []byte(`{"captcha_token":"`+token+`"}`)) {
		t.Fatal("官方验证结果被拒绝")
	}
	select {
	case received := <-results:
		if received != token {
			t.Fatal("验证结果丢失")
		}
	case <-time.After(time.Second):
		t.Fatal("验证结果没有回调")
	}
}

func TestCloseStaleVerificationSessionDoesNotCloseReplacement(t *testing.T) {
	first, err := StartForProvider("thunder", func(Session, string) {})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Start(func(Session, string) {})
	if err != nil {
		t.Fatal(err)
	}
	defer Close()
	CloseSession(first.ID)
	mu.Lock()
	stillActive := activeSession.ID == second.ID && onDone != nil
	mu.Unlock()
	if !stillActive {
		t.Fatal("关闭旧验证会话影响了新登录")
	}
	if _, err := ShowForProvider("thunder", second.ID, "https://captcha.xunlei.com/verify", t.TempDir()); err == nil {
		t.Fatal("显示窗口未检查网盘会话归属")
	}
}

func TestReportTokenBeforeTokenlessRedirect(t *testing.T) {
	results := make(chan string, 2)
	session, err := Start(func(_ Session, token string) { results <- token })
	if err != nil {
		t.Fatal(err)
	}
	defer Close()
	queueCallback(session.ID, "")
	token := strings.Repeat("final-token", 4)
	if !acceptReport(session.ID, "https://user.mypikpak.com/credit/v1/report", []byte(`{"data":{"captcha_token":"`+token+`"}}`)) {
		t.Fatal("valid report was not accepted")
	}
	select {
	case got := <-results:
		if got != token {
			t.Fatalf("got %q, want final report token", got)
		}
	case <-time.After(time.Second):
		t.Fatal("completion not delivered")
	}
	select {
	case <-results:
		t.Fatal("duplicate completion")
	case <-time.After(650 * time.Millisecond):
	}
}

func TestReportRejectsUntrustedOrStaleResults(t *testing.T) {
	results := make(chan string, 2)
	session, err := Start(func(_ Session, token string) { results <- token })
	if err != nil {
		t.Fatal(err)
	}
	defer Close()
	body := []byte(`{"captcha_token":"` + strings.Repeat("x", 40) + `"}`)
	for _, raw := range []string{"https://mypikpak.com.evil.test/credit/v1/report", "http://user.mypikpak.com/credit/v1/report", "https://user.mypikpak.com/v1/captcha/init"} {
		if acceptReport(session.ID, raw, body) {
			t.Fatalf("accepted %s", raw)
		}
	}
	if acceptReport("old-session", "https://user.mypikpak.com/credit/v1/report", body) {
		t.Fatal("accepted stale report")
	}
	queueCallback(session.ID, "")
	if _, err := Start(func(_ Session, token string) { results <- token }); err != nil {
		t.Fatal(err)
	}
	select {
	case <-results:
		t.Fatal("old callback completed replacement session")
	case <-time.After(650 * time.Millisecond):
	}
}

func TestCallbackURLTokenAndScope(t *testing.T) {
	s := Session{CallbackURL: "http://127.0.0.1:2345/callback/current"}
	token := strings.Repeat("t", 40)
	for _, raw := range []string{s.CallbackURL + "#captcha_token=" + token, "xlaccsdk01://xbase.cloud/callback?captcha_token=" + token} {
		got, ok := callbackToken(s, raw)
		if !ok || got != token {
			t.Fatalf("callback %s: %q %v", raw, got, ok)
		}
	}
	for _, raw := range []string{"http://127.0.0.1:2345/callback/stale", "https://evil.test/callback", "xlaccsdk01://evil.test/callback"} {
		if _, ok := callbackToken(s, raw); ok {
			t.Fatalf("accepted unrelated callback %s", raw)
		}
	}
}
