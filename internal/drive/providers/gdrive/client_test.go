package gdrive

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(r *http.Request, status int, body string) *http.Response {
	return &http.Response{Request: r, StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
func useTransport(t *testing.T, f roundTrip) {
	t.Helper()
	previous := netx.TestTransportHook
	netx.TestTransportHook = f
	t.Cleanup(func() { netx.TestTransportHook = previous })
}
func account() drive.Context {
	return drive.Context{UserID: "gdrive_account", DriveID: "gdrive:account", TokenFrom: providerID, Token: &model.TokenInfo{TokenFrom: providerID, AccessToken: "access", RefreshToken: "refresh", ProviderRootID: "root-id", OAuthRefreshURL: defaultRefreshURL}}
}

func TestOnlineRefreshContract(t *testing.T) {
	useTransport(t, func(r *http.Request) (*http.Response, error) {
		q := r.URL.Query()
		if r.Method != "GET" || r.URL.Host != "api.oplist.org" || r.URL.Path != "/googleui/renewapi" || q.Get("refresh_ui") != "refresh+a/b" || q.Get("server_use") != "true" || q.Get("driver_txt") != "googleui_go" {
			t.Fatalf("刷新契约错误：%s %s", r.Method, r.URL.Path)
		}
		return response(r, 200, `{"access_token":"fresh","refresh_token":"rotated"}`), nil
	})
	token := &model.TokenInfo{RefreshToken: "refresh+a/b"}
	if err := renew(t.Context(), token); err != nil {
		t.Fatal(err)
	}
	if token.AccessToken != "fresh" || token.RefreshToken != "rotated" || token.OAuthRefreshURL != defaultRefreshURL || token.ExpiresIn != 0 || token.ExpireTime != "" {
		t.Fatalf("刷新结果错误：%+v", token)
	}
}

func TestRefreshErrorsDoNotLeakSecretsOrRemoveHealthyAccounts(t *testing.T) {
	for _, test := range []struct {
		name, body string
		status     int
		auth       bool
		contains   string
	}{
		{"应用删除", `{"text":"The OAuth client was deleted. refresh-secret"}`, 500, false, "服务当前不可用"},
		{"令牌失效", `{"error":"invalid_grant"}`, 500, true, "重新授权"},
		{"空访问令牌", `{"refresh_token":"refresh-secret"}`, 200, false, "未返回有效"},
		{"非JSON", `<html>refresh-secret</html>`, 502, false, "无效响应"},
		{"上游错误", `{"text":"https://service/?refresh_ui=refresh-secret"}`, 500, false, "未返回有效"},
		{"重定向", `{}`, 302, false, "未返回有效"},
	} {
		t.Run(test.name, func(t *testing.T) {
			useTransport(t, func(r *http.Request) (*http.Response, error) { return response(r, test.status, test.body), nil })
			token := &model.TokenInfo{AccessToken: "old", RefreshToken: "refresh-secret"}
			err := renew(t.Context(), token)
			if err == nil || !strings.Contains(err.Error(), test.contains) || strings.Contains(err.Error(), "refresh-secret") || errors.Is(err, drive.ErrUnauthorized) != test.auth || token.AccessToken != "old" {
				t.Fatalf("错误处理不符合要求：%v", err)
			}
		})
	}
}

func TestRefreshRejectsUnsafeEndpointAndRedactsNetworkError(t *testing.T) {
	for _, raw := range []string{"http://service.test/googleui/renewapi", "https://user:pass@service.test/googleui/renewapi", "https://service.test/googleui/renewapi?x=1", "https://service.test/googleui/renewapi#x", "https://service.test/other"} {
		if _, err := refreshEndpoint(raw); err == nil {
			t.Fatalf("接受了无效地址：%s", raw)
		}
	}
	useTransport(t, func(r *http.Request) (*http.Response, error) {
		return nil, &url.Error{Op: "Get", URL: r.URL.String(), Err: errors.New("secret-body")}
	})
	err := renew(t.Context(), &model.TokenInfo{RefreshToken: "private-refresh"})
	if err == nil || strings.Contains(err.Error(), "private-refresh") || strings.Contains(err.Error(), "secret-body") {
		t.Fatalf("异常泄露令牌：%v", err)
	}
}

func TestGoogle401RefreshesOnlyOnce(t *testing.T) {
	requests, refreshes := 0, 0
	useTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "api.oplist.org" {
			refreshes++
			return response(r, 200, `{"access_token":"fresh","refresh_token":"refresh"}`), nil
		}
		requests++
		return response(r, 401, `{"error":{"code":401}}`), nil
	})
	c := account()
	cl, _ := newClient(c)
	_, err := cl.detail(t.Context(), "file")
	if err == nil || requests != 2 || refreshes != 1 {
		t.Fatalf("401 发生无界重试：%d/%d %v", requests, refreshes, err)
	}
}

func TestConcurrentRefreshReloadsPersistedToken(t *testing.T) {
	var mu sync.Mutex
	stored := drive.CloneToken(account().Token)
	refreshes := 0
	drive.SetTokenResolver(func(string, string) (*model.TokenInfo, error) {
		mu.Lock()
		defer mu.Unlock()
		return drive.CloneToken(stored), nil
	})
	drive.SetTokenUpdater(func(_, _ string, token *model.TokenInfo) error {
		mu.Lock()
		defer mu.Unlock()
		stored = drive.CloneToken(token)
		return nil
	})
	t.Cleanup(func() { drive.SetTokenResolver(nil); drive.SetTokenUpdater(nil) })
	useTransport(t, func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		refreshes++
		mu.Unlock()
		return response(r, 200, `{"access_token":"fresh","refresh_token":"rotated"}`), nil
	})
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			cl, _ := newClient(account())
			if err := cl.refresh(t.Context(), "access"); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if refreshes != 1 || stored.RefreshToken != "rotated" {
		t.Fatalf("并发刷新未合并：%d", refreshes)
	}
}

func TestAuthConsentIdentityAndCustomEndpoint(t *testing.T) {
	requests := 0
	useTransport(t, func(r *http.Request) (*http.Response, error) {
		requests++
		switch r.URL.Path {
		case "/googleui/renewapi":
			if r.URL.Host != "trusted.test" {
				t.Fatal("令牌发往错误接收方")
			}
			return response(r, 200, `{"access_token":"fresh","refresh_token":"rotated"}`), nil
		case "/drive/v3/about":
			return response(r, 200, `{"user":{"permissionId":"stable-user","displayName":"测试账号","emailAddress":"user@example.test"},"storageQuota":{"limit":"100","usage":"20"}}`), nil
		case "/drive/v3/files/root":
			return response(r, 200, `{"id":"real-root","mimeType":"application/vnd.google-apps.folder","driveId":"shared-drive"}`), nil
		}
		t.Fatalf("意外请求：%s", r.URL.Path)
		return nil, nil
	})
	config := map[string]string{"refresh_token": "valid-refresh", "login_method": "custom", "api_url": "https://trusted.test/googleui/renewapi"}
	if _, err := authLogin(t.Context(), drive.AuthRequest{Config: config}); err == nil || requests != 0 {
		t.Fatal("未取得确认便发送了令牌")
	}
	config["relay_consent"] = "true"
	token, err := authLogin(t.Context(), drive.AuthRequest{Config: config})
	if err != nil {
		t.Fatal(err)
	}
	if token.UserID != "gdrive_stable-user" || token.DefaultDriveID != "gdrive:stable-user" || token.OAuthRefreshURL != config["api_url"] || token.ProviderRootID != "real-root" || token.ProviderSpaceID != "shared-drive" || token.FreeSize != 80 {
		t.Fatalf("账号装配错误：%+v", token)
	}
	encoded, _ := json.Marshal(token)
	var restored model.TokenInfo
	if err := json.Unmarshal(encoded, &restored); err != nil || restored.OAuthRefreshURL != token.OAuthRefreshURL || restored.ProviderSpaceID != token.ProviderSpaceID {
		t.Fatal("账号配置未能完整保存")
	}
}

func TestRefreshCancellation(t *testing.T) {
	useTransport(t, func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := renew(ctx, &model.TokenInfo{RefreshToken: "refresh"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("取消未传播：%v", err)
	}
}
