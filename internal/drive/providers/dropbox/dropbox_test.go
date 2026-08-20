package dropbox

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

type dropboxRoundTripper func(*http.Request) (*http.Response, error)

func (f dropboxRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func dropboxResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

func withDropboxTransport(t *testing.T, rt http.RoundTripper) {
	t.Helper()
	old := netx.TestTransportHook
	netx.TestTransportHook = rt
	t.Cleanup(func() { netx.TestTransportHook = old })
}

func dropboxAPIPath(r *http.Request) string {
	return strings.TrimPrefix(r.URL.Path, "/2")
}

func TestDropboxRedirectURIUsesRcloneCompatibleDefault(t *testing.T) {
	spec, err := resolveRedirectURI(nil)
	if err != nil {
		t.Fatalf("resolve default redirect URI: %v", err)
	}
	if spec.raw != dbDefaultRedirectURI || spec.host != "localhost" || spec.port != 53682 || spec.path != "/" {
		t.Fatalf("unexpected default redirect spec: %#v", spec)
	}
	if got := spec.listenAddr(); got != "127.0.0.1:53682" {
		t.Fatalf("listen address = %q", got)
	}
}

func TestDropboxRedirectURIAllowsConfiguredLoopbackAndRejectsRemote(t *testing.T) {
	spec, err := resolveRedirectURI(map[string]string{"dropbox_redirect_uri": "http://127.0.0.1:4242/oauth/callback"})
	if err != nil {
		t.Fatalf("resolve custom redirect URI: %v", err)
	}
	if spec.raw != "http://127.0.0.1:4242/oauth/callback" || spec.path != "/oauth/callback" || spec.port != 4242 {
		t.Fatalf("unexpected custom redirect spec: %#v", spec)
	}
	if _, err := resolveRedirectURI(map[string]string{"dropbox_redirect_uri": "https://example.com/callback"}); err == nil {
		t.Fatal("remote redirect URI should be rejected")
	}
}

func TestDropboxCallbackValidationUsesConfiguredHostAndPath(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	spec := redirectSpec{host: "localhost", port: port, path: "/oauth/callback"}
	req := httptest.NewRequest(http.MethodGet, "http://localhost:"+fmt.Sprint(port)+"/oauth/callback?code=x&state=y", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	if !validCallbackRequest(req, ln, spec) {
		t.Fatal("valid loopback callback was rejected")
	}
	req.Host = "127.0.0.1:" + fmt.Sprint(port)
	if validCallbackRequest(req, ln, spec) {
		t.Fatal("callback for a different configured host was accepted")
	}
}

func TestDropboxRefreshAccountUpdatesExpiryAndRetainsRotatingFields(t *testing.T) {
	tokenCalls := 0
	withDropboxTransport(t, dropboxRoundTripper(func(r *http.Request) (*http.Response, error) {
		switch dropboxAPIPath(r) {
		case "/oauth2/token":
			tokenCalls++
			if tokenCalls == 1 {
				return dropboxResponse(r, http.StatusOK, `{"access_token":"access-new","expires_in":120,"token_type":"Bearer"}`), nil
			}
			return dropboxResponse(r, http.StatusOK, `{"access_token":"access-new-2","token_type":"Bearer"}`), nil
		case "/users/get_current_account":
			return dropboxResponse(r, http.StatusOK, `{"account_id":"db-acct","email":"user@example.com","name":{"display_name":"Dropbox User"}}`), nil
		case "/users/get_space_usage":
			return dropboxResponse(r, http.StatusOK, `{"used":10,"allocation":{"allocated":100}}`), nil
		default:
			return dropboxResponse(r, http.StatusNotFound, `{}`), nil
		}
	}))

	d := &Driver{}
	tok := &model.TokenInfo{
		TokenFrom:    providerID,
		AccessToken:  "access-old",
		RefreshToken: "refresh-old",
		TokenType:    "Bearer",
		ExpiresIn:    3600,
		DeviceID:     "app-key",
	}
	got, err := d.RefreshAccount(context.Background(), drive.Context{Token: tok}, tok)
	if err != nil {
		t.Fatalf("RefreshAccount returned error: %v", err)
	}
	if got != tok || tok.AccessToken != "access-new" || tok.RefreshToken != "refresh-old" {
		t.Fatalf("refresh fields = %+v", tok)
	}
	if tok.UserID != model.BuildUserID(providerID, "db-acct") || tok.DefaultDriveID != model.BuildDriveID(providerID, "db-acct") {
		t.Fatalf("account identity = %q/%q", tok.UserID, tok.DefaultDriveID)
	}
	expiry, err := time.Parse(time.RFC3339, tok.ExpireTime)
	if err != nil {
		t.Fatalf("ExpireTime = %q: %v", tok.ExpireTime, err)
	}
	if expiry.Before(time.Now().Add(90*time.Second)) || expiry.After(time.Now().Add(150*time.Second)) {
		t.Fatalf("ExpireTime = %s, not near 120 seconds from now", expiry)
	}

	oldExpiry := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	tok.AccessToken = "access-old-2"
	tok.ExpiresIn = 7200
	tok.ExpireTime = oldExpiry
	if _, err := d.RefreshAccount(context.Background(), drive.Context{Token: tok}, tok); err != nil {
		t.Fatalf("RefreshAccount without expires_in returned error: %v", err)
	}
	if tok.RefreshToken != "refresh-old" || tok.ExpiresIn != 7200 || tok.ExpireTime != oldExpiry {
		t.Fatalf("omitted refresh fields were not retained: %+v", tok)
	}
}

func TestDropboxExistingSharedLinkUsesObjectPagination(t *testing.T) {
	continueCalls := 0
	modifyCalls := 0
	withDropboxTransport(t, dropboxRoundTripper(func(r *http.Request) (*http.Response, error) {
		switch dropboxAPIPath(r) {
		case "/sharing/create_shared_link_with_settings":
			return dropboxResponse(r, http.StatusConflict, `{"error_summary":"shared_link_already_exists/"}`), nil
		case "/sharing/list_shared_links":
			return dropboxResponse(r, http.StatusOK, `{"links":[{"id":"sl-1","url":"https://www.dropbox.com/s/one","name":"movie.mkv","link_access_level":"public"}],"has_more":true,"cursor":"cursor-1"}`), nil
		case "/sharing/list_shared_links/continue":
			continueCalls++
			return dropboxResponse(r, http.StatusOK, `{"links":[{"id":"sl-2","url":"https://www.dropbox.com/s/two","name":"other.mkv"}],"has_more":false}`), nil
		case "/sharing/modify_shared_link_settings":
			modifyCalls++
			return dropboxResponse(r, http.StatusOK, `{"id":"sl-1","url":"https://www.dropbox.com/s/one","name":"movie.mkv","link_access_level":"password","expires":"2026-06-01T08:00:00Z"}`), nil
		default:
			return dropboxResponse(r, http.StatusNotFound, `{}`), nil
		}
	}))

	share, err := newClient("access").CreateSharedLink(context.Background(), "/movie.mkv", "2026-06-01T08:00:00Z", "1234")
	if err != nil {
		t.Fatalf("CreateSharedLink returned error: %v", err)
	}
	if continueCalls != 1 || modifyCalls != 1 {
		t.Fatalf("shared-link calls = continue:%d modify:%d", continueCalls, modifyCalls)
	}
	if share.ShareID != "sl-1" || share.ShareURL != "https://www.dropbox.com/s/one" || share.SharePwd != "1234" || share.Expiration == "" {
		t.Fatalf("share = %+v", share)
	}
}

func TestDropboxUploadRequiresRemoteFileID(t *testing.T) {
	body := `{".tag":"file","name":"x.txt"}`
	withDropboxTransport(t, dropboxRoundTripper(func(r *http.Request) (*http.Response, error) {
		return dropboxResponse(r, http.StatusOK, body), nil
	}))

	policy := uploadPolicy{mode: "overwrite"}
	if _, err := newClient("access").UploadSmall(context.Background(), "/x.txt", strings.NewReader("data"), 4, policy); err == nil || !strings.Contains(err.Error(), "missing file id") {
		t.Fatalf("missing upload id error = %v", err)
	}

	body = `{".tag":"file","id":"id:x","name":"x.txt"}`
	path := t.TempDir() + "/x.txt"
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	ui := &model.UploadingUI{Info: model.UploadInfo{LocalFilePath: path, Name: "x.txt"}}
	if err := (&Driver{}).UploadOneFile(context.Background(), drive.Context{Token: &model.TokenInfo{AccessToken: "access"}}, ui); err != nil {
		t.Fatalf("UploadOneFile returned error: %v", err)
	}
	if ui.Upload.FileID != "id:x" {
		t.Fatalf("UploadOneFile file id = %q", ui.Upload.FileID)
	}
}

func TestDropboxUploadSessionFinishRequiresRemoteFileID(t *testing.T) {
	withDropboxTransport(t, dropboxRoundTripper(func(r *http.Request) (*http.Response, error) {
		return dropboxResponse(r, http.StatusOK, `{}`), nil
	}))

	_, err := newClient("access").sessionFinish(context.Background(), "session", 0, []byte("data"), "/x.txt", uploadPolicy{mode: "overwrite"})
	if err == nil || !strings.Contains(err.Error(), "missing file id") {
		t.Fatalf("missing session file id error = %v", err)
	}
}

func TestDropboxUploadSessionKeyIncludesIdentityAndConflictPolicy(t *testing.T) {
	dc := drive.Context{UserID: "dropbox-user", DriveID: "dropbox-drive"}
	base := &model.UploadingUI{Info: model.UploadInfo{SHA1: "sha-a"}}
	overwrite := uploadPolicy{mode: "overwrite"}
	rename := uploadPolicy{mode: "add", autorename: true}
	first := dropboxUploadSessionKey(dc, "/movie.mkv", 100, base, overwrite, 1)
	second := dropboxUploadSessionKey(dc, "/movie.mkv", 100, &model.UploadingUI{Info: model.UploadInfo{SHA1: "sha-b"}}, overwrite, 1)
	third := dropboxUploadSessionKey(dc, "/movie.mkv", 100, base, rename, 1)
	fourth := dropboxUploadSessionKey(dc, "/movie.mkv", 100, nil, overwrite, 2)
	if first == second || first == third || first == fourth {
		t.Fatalf("upload session key does not isolate identity/policy: %q %q %q %q", first, second, third, fourth)
	}
}

func TestDropboxTemporaryLinkDoesNotCarryBearerToken(t *testing.T) {
	withDropboxTransport(t, dropboxRoundTripper(func(r *http.Request) (*http.Response, error) {
		if dropboxAPIPath(r) != "/files/get_temporary_link" {
			return dropboxResponse(r, http.StatusNotFound, `{}`), nil
		}
		return dropboxResponse(r, http.StatusOK, `{"link":"https://dl.dropboxusercontent.com/signed","metadata":{".tag":"file","size":4}}`), nil
	}))

	c := drive.Context{DriveID: "dropbox-drive", Token: &model.TokenInfo{AccessToken: "secret"}}
	dl, err := (&Driver{}).GetDownloadURL(context.Background(), c, "/x.txt", 0)
	if err != nil {
		t.Fatalf("GetDownloadURL returned error: %v", err)
	}
	if len(dl.Headers) != 0 {
		t.Fatalf("temporary link headers = %#v", dl.Headers)
	}
	preview, err := (&Driver{}).GetVideoPreview(context.Background(), c, "/x.txt")
	if err != nil {
		t.Fatalf("GetVideoPreview returned error: %v", err)
	}
	if len(preview.Headers) != 0 || len(preview.Qualities) != 1 || len(preview.Qualities[0].Headers) != 0 {
		t.Fatalf("preview headers = %+v", preview)
	}
}
