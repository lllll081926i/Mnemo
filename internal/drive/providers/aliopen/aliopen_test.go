package aliopen

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

func TestRefreshTokenStoresProfileAndNumericDriveID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"access_token":"access-next",
			"refresh_token":"refresh-next",
			"user_id":"user-001",
			"user_name":"ali-user",
			"nick_name":"阿里昵称",
			"avatar":"https://example.invalid/avatar.png",
			"default_drive_id":10001
		}`))
	}))
	defer server.Close()

	token := &model.TokenInfo{}
	sess := &Session{RefreshToken: "refresh-old", OAuthTokenURL: server.URL}
	cl := &client{http: netx.NewClient(5 * time.Second), session: sess, token: token}
	if err := cl.refreshToken(context.Background()); err != nil {
		t.Fatalf("refreshToken() error = %v", err)
	}

	if sess.AccessToken != "access-next" || sess.RefreshToken != "refresh-next" {
		t.Fatalf("session tokens = %#v", sess)
	}
	if sess.UserID != "user-001" || sess.DriveID != "10001" {
		t.Fatalf("session identity = %#v", sess)
	}
	if sess.displayName() != "阿里昵称" || sess.accountID() != "user-001" {
		t.Fatalf("profile resolution = name %q id %q", sess.displayName(), sess.accountID())
	}
	if token.RefreshToken == "" || token.OpenAPIAccessToken != "access-next" {
		t.Fatalf("persisted token = %#v", token)
	}
}

func TestParseAliOpenSpaceInfoSupportsNumbersAndStrings(t *testing.T) {
	used, total := parseAliOpenSpaceInfo([]byte(`{
		"personal_space_info":{"used_size":"12","total_size":100}
	}`))
	if used != 12 || total != 100 {
		t.Fatalf("string quota = %d/%d, want 12/100", used, total)
	}

	used, total = parseAliOpenSpaceInfo([]byte(`{
		"personal_space_info":{"usedSize":120,"totalSize":100}
	}`))
	if used != 100 || total != 100 {
		t.Fatalf("clamped numeric quota = %d/%d, want 100/100", used, total)
	}

	used, total = parseAliOpenSpaceInfo([]byte(`{"personal_space_info":{"total_size":"bad"}}`))
	if used != 0 || total != 0 {
		t.Fatalf("invalid quota = %d/%d, want 0/0", used, total)
	}
}

func TestApplyAliOpenQuotaPreservesLastKnownValueOnMissingQuota(t *testing.T) {
	token := &model.TokenInfo{UsedSize: 2, TotalSize: 10, FreeSize: 8}
	applyAliOpenQuota(token, 0, 0)
	if token.UsedSize != 2 || token.TotalSize != 10 || token.FreeSize != 8 {
		t.Fatalf("missing quota replaced last known values: %#v", token)
	}
}
