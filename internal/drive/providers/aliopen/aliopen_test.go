package aliopen

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

type aliOpenRoundTripperFunc func(*http.Request) (*http.Response, error)

func (f aliOpenRoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

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

func TestRefreshAccountProfileReplacesIdentifierDisplayFields(t *testing.T) {
	previous := netx.TestTransportHook
	var calls int
	netx.TestTransportHook = aliOpenRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.Method != http.MethodPost || req.URL.Hostname() != "api.alipan.com" || req.URL.Path != "/v2/user/get" {
			t.Fatalf("profile request = %s %s", req.Method, req.URL)
		}
		if req.Header.Get("Authorization") != "Bearer profile-access" {
			t.Fatalf("profile authorization = %q", req.Header.Get("Authorization"))
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(`{
				"user_id":"user-001", "user_name":"138***8000",
				"nick_name":"阿里昵称", "phone":"13800138000",
				"avatar":"https://example.invalid/profile.png"
			}`)),
			Request: req,
		}, nil
	})
	t.Cleanup(func() { netx.TestTransportHook = previous })

	token := &model.TokenInfo{UserName: "user-001", NickName: "user-001", Name: "user-001"}
	sess := &Session{AccessToken: "profile-access", UserID: "user-001", UserName: "user-001", NickName: "user-001"}
	cl := &client{http: netx.NewClient(5 * time.Second), session: sess, token: token}
	cl.refreshAccountProfile(context.Background())
	applyAliOpenProfile(token, sess)

	if calls != 1 || sess.ProfileCheckedAt == 0 {
		t.Fatalf("profile fetch calls/check time = %d/%d", calls, sess.ProfileCheckedAt)
	}
	if token.Name != "阿里昵称" || token.UserName != "138***8000" || token.NickName != "阿里昵称" {
		t.Fatalf("profile did not replace identifier fields: %#v", token)
	}
	if token.Avatar != "https://example.invalid/profile.png" {
		t.Fatalf("profile avatar = %q", token.Avatar)
	}

	cl.refreshAccountProfile(context.Background())
	if calls != 1 {
		t.Fatalf("fresh profile should be cached, calls = %d", calls)
	}
}

func TestApplyAliOpenProfileSkipsIdentifierFallbacks(t *testing.T) {
	token := &model.TokenInfo{UserName: "user-001", NickName: "user-001", Name: "user-001"}
	sess := &Session{UserID: "user-001", UserName: "138***8000", NickName: "user-001", Phone: "13800138000"}
	applyAliOpenProfile(token, sess)
	if token.Name != "138***8000" || token.UserName != "138***8000" || token.NickName != "138***8000" {
		t.Fatalf("identifier fallback still won: %#v", token)
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

func TestCreateShareUsesAliOpenAPI(t *testing.T) {
	previous := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = previous })

	netx.TestTransportHook = aliOpenRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || req.URL.Host != "openapi.alipan.com" || req.URL.Path != "/adrive/v1.0/openFile/createShareLink" {
			t.Errorf("request = %s %s", req.Method, req.URL)
		}
		if req.Header.Get("Authorization") != "Bearer access-token" {
			t.Errorf("authorization = %q", req.Header.Get("Authorization"))
		}
		var body struct {
			DriveID    string   `json:"drive_id"`
			FileIDs    []string `json:"file_id_list"`
			ShareName  string   `json:"share_name"`
			SharePwd   string   `json:"share_pwd"`
			Expiration string   `json:"expiration"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if body.DriveID != "drive-1" || strings.Join(body.FileIDs, ",") != "file-1,file-2" || body.ShareName != "测试分享" || body.SharePwd != "p4ss" || body.Expiration != "2030-01-01T00:00:00Z" {
			t.Errorf("share body = %+v", body)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"share_id":"share-ali","share_url":"https://www.aliyundrive.com/s/share-ali","share_msg":"分享链接","expiration":"2030-01-01T00:00:00Z","status":"enabled","drive_id":"drive-1"}`)),
			Request:    req,
		}, nil
	})

	sess := &Session{AccessToken: "access-token", DriveID: "drive-1"}
	item, err := (&Driver{}).CreateShare(context.Background(), drive.Context{
		UserID: "aliopen:user", DriveID: "aliopen:user",
		Token: &model.TokenInfo{AccessToken: sess.AccessToken, RefreshToken: mustJSON(sess)},
	}, drive.ShareParams{FileIDs: []string{"b:file-1", "b:file-2"}, ShareName: "测试分享", Expiration: "2030-01-01T00:00:00Z", Password: "p4ss"})
	if err != nil {
		t.Fatalf("CreateShare() error = %v", err)
	}
	if item.ShareID != "share-ali" || item.ShareURL != "https://www.aliyundrive.com/s/share-ali" || item.FileID != "b:file-1" || len(item.FileIDList) != 2 || item.AccountID != "aliopen:user" {
		t.Fatalf("share = %+v", item)
	}
	if !(&Driver{}).Capabilities().CombinedShare {
		t.Fatal("Ali Open supports multi-file shares and must advertise combinedShare")
	}
}

func TestCreateShareFallsBackToNativeRouteOnlyAfterNotFound(t *testing.T) {
	previous := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = previous })

	var calls []string
	netx.TestTransportHook = aliOpenRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		calls = append(calls, req.URL.Host+req.URL.Path)
		switch {
		case req.URL.Host == "openapi.alipan.com" && req.URL.Path == "/adrive/v1.0/openFile/createShareLink":
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"code":"NotFound","message":"not found"}`)),
				Request:    req,
			}, nil
		case req.URL.Host == "api.aliyundrive.com" && req.URL.Path == "/adrive/v2/share_link/create":
			var body struct {
				DriveID string   `json:"drive_id"`
				FileIDs []string `json:"file_id_list"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Errorf("decode fallback body: %v", err)
			}
			if body.DriveID != "drive-1" || strings.Join(body.FileIDs, ",") != "file-1" {
				t.Errorf("fallback share body = %+v", body)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"share_id":"share-fallback"}`)),
				Request:    req,
			}, nil
		default:
			t.Errorf("unexpected request %s %s", req.Method, req.URL)
			return &http.Response{StatusCode: http.StatusInternalServerError, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`)), Request: req}, nil
		}
	})

	sess := &Session{AccessToken: "access-token", DriveID: "drive-1"}
	item, err := (&Driver{}).CreateShare(context.Background(), drive.Context{
		UserID: "aliopen:user", DriveID: "aliopen:user",
		Token: &model.TokenInfo{AccessToken: sess.AccessToken, RefreshToken: mustJSON(sess)},
	}, drive.ShareParams{FileIDs: []string{"b:file-1"}})
	if err != nil {
		t.Fatalf("CreateShare() error = %v", err)
	}
	if strings.Join(calls, ",") != "openapi.alipan.com/adrive/v1.0/openFile/createShareLink,api.aliyundrive.com/adrive/v2/share_link/create" {
		t.Fatalf("calls = %v", calls)
	}
	if item.ShareID != "share-fallback" || item.ShareURL != "https://www.alipan.com/s/share-fallback" {
		t.Fatalf("fallback share = %+v", item)
	}
}

func TestAliOpenCancelShareFallsBackToNativeRoute(t *testing.T) {
	previous := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = previous })

	var calls []string
	netx.TestTransportHook = aliOpenRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		calls = append(calls, req.URL.Host+req.URL.Path)
		switch {
		case req.URL.Host == "openapi.alipan.com" && req.URL.Path == "/adrive/v1.0/openFile/cancelShareLink":
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"code":"NotFound","message":"not found"}`)),
				Request:    req,
			}, nil
		case req.URL.Host == "api.aliyundrive.com" && req.URL.Path == "/adrive/v2/share_link/cancel":
			var body struct {
				ShareID string `json:"share_id"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Errorf("decode cancel body: %v", err)
			}
			if body.ShareID != "share-ali" {
				t.Errorf("cancel body = %+v", body)
			}
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`)), Request: req}, nil
		default:
			t.Errorf("unexpected request %s %s", req.Method, req.URL)
			return &http.Response{StatusCode: http.StatusInternalServerError, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`)), Request: req}, nil
		}
	})

	sess := &Session{AccessToken: "access-token", DriveID: "drive-1"}
	err := (&Driver{}).CancelShare(context.Background(), drive.Context{
		UserID: "aliopen:user", DriveID: "aliopen:user",
		Token: &model.TokenInfo{AccessToken: sess.AccessToken, RefreshToken: mustJSON(sess)},
	}, model.ShareHistoryEntry{ShareID: "share-ali"})
	if err != nil {
		t.Fatalf("CancelShare() error = %v", err)
	}
	if strings.Join(calls, ",") != "openapi.alipan.com/adrive/v1.0/openFile/cancelShareLink,api.aliyundrive.com/adrive/v2/share_link/cancel" {
		t.Fatalf("calls = %v", calls)
	}
	if !(&Driver{}).Capabilities().CancelCreatedShares {
		t.Fatal("Ali Open must advertise cancelCreatedShares")
	}
}

func TestAliOpenNotFoundRequiresStructuredStatus(t *testing.T) {
	if !aliOpenNotFound(aliOpenRequestErrorOf([]byte(`{"code":"NotFound","message":"not found"}`), http.StatusNotFound)) {
		t.Fatal("structured HTTP 404 should enable the narrow endpoint fallback")
	}
	if aliOpenNotFound(errors.New("aliopen: http 404: not found")) {
		t.Fatal("formatted error text alone must not enable an endpoint fallback")
	}
	if aliOpenNotFound(aliOpenRequestErrorOf([]byte(`{"code":"NotFound","message":"not found"}`), http.StatusOK)) {
		t.Fatal("provider error code without HTTP 404 must not enable an endpoint fallback")
	}
}

func TestRapidUploadByHashValidatesAndCleansPendingMiss(t *testing.T) {
	previous := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = previous })
	var paths []string
	netx.TestTransportHook = aliOpenRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		paths = append(paths, req.URL.Path)
		body := `{}`
		switch req.URL.Path {
		case "/adrive/v1.0/openFile/create":
			body = `{"file_id":"pending-1","upload_id":"upload-1","rapid_upload":false}`
		case "/adrive/v1.0/openFile/delete":
		default:
			return nil, errors.New("unexpected aliopen request: " + req.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	})
	sess := &Session{AccessToken: "access-token", DriveID: "drive-1"}
	c := drive.Context{UserID: "aliopen:user", DriveID: "aliopen:user", Token: &model.TokenInfo{AccessToken: sess.AccessToken, RefreshToken: mustJSON(sess)}}
	req := drive.RapidUploadRequest{ParentID: "b:root", FileName: "movie.mp4", Method: "sha1", Hash: strings.Repeat("a", 40), Size: 4096}
	result, err := (&Driver{}).RapidUploadByHash(t.Context(), c, req)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Reuse || strings.Join(paths, ",") != "/adrive/v1.0/openFile/create,/adrive/v1.0/openFile/delete" {
		t.Fatalf("result/paths = %+v / %v", result, paths)
	}

	for _, invalid := range []drive.RapidUploadRequest{
		{Method: "md5", Hash: strings.Repeat("a", 40), Size: 1},
		{Method: "sha1", Hash: "invalid", Size: 1},
		{Method: "sha1", Hash: strings.Repeat("a", 40), Size: -1},
	} {
		paths = nil
		got, err := (&Driver{}).RapidUploadByHash(t.Context(), c, invalid)
		if err != nil || got == nil || got.Reuse || len(paths) != 0 {
			t.Fatalf("invalid request = %+v, result=%+v err=%v paths=%v", invalid, got, err, paths)
		}
	}
}

func TestResolveTransferHashValidatesSHA1AndPreservesDetailError(t *testing.T) {
	previous := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = previous })
	sess := &Session{AccessToken: "access-token", DriveID: "drive-1"}
	c := drive.Context{UserID: "aliopen:user", DriveID: "aliopen:user", Token: &model.TokenInfo{AccessToken: sess.AccessToken, RefreshToken: mustJSON(sess)}}

	for _, tc := range []struct {
		name     string
		status   int
		body     string
		wantHash string
		wantErr  bool
	}{
		{name: "valid", status: http.StatusOK, body: `{"content_hash_name":"sha1","content_hash":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}`, wantHash: strings.Repeat("a", 40)},
		{name: "invalid hash", status: http.StatusOK, body: `{"content_hash_name":"sha1","content_hash":"not-sha1"}`},
		{name: "detail error", status: http.StatusInternalServerError, body: `{"code":"ServerError","message":"detail failed"}`, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			netx.TestTransportHook = aliOpenRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body)), Request: req}, nil
			})
			hash, err := (&Driver{}).ResolveTransferHash(t.Context(), c, "b:file-1", "sha1", false)
			if hash != tc.wantHash || (err != nil) != tc.wantErr {
				t.Fatalf("ResolveTransferHash() = %q, %v", hash, err)
			}
		})
	}
}
