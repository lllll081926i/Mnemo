package lanzou

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

func withNoThrottle(t *testing.T) {
	t.Helper()
	old := fetchMinInterval
	fetchMinInterval = 0
	t.Cleanup(func() { fetchMinInterval = old })
}

func testCtx(driveID, baseURL, cookie string) drive.Context {
	return drive.Context{
		DriveID: driveID,
		Token: &model.TokenInfo{
			AccessToken:  cookie,
			RefreshToken: fmt.Sprintf(`{"type":"cookie","cookie":%q,"baseUrl":%q}`, cookie, baseURL),
		},
	}
}

// TestFileListPagination exercises the task-47 / task-5 paging loop and the
// folder+file mapping.
func TestFileListPagination(t *testing.T) {
	withNoThrottle(t)
	var mu sync.Mutex
	var pgs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch r.Form.Get("task") {
		case "47":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"zt": 1, "text": []any{map[string]any{"fol_id": "9", "name": "dir"}},
			})
		case "5":
			pg := r.Form.Get("pg")
			mu.Lock()
			pgs = append(pgs, pg)
			mu.Unlock()
			if pg == "1" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"zt": 1, "text": []any{map[string]any{"id": "8", "name_all": "a.zip", "size": "2M"}},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"zt": 1, "text": []any{}})
		default:
			http.Error(w, "unknown task", http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	d := &Driver{}
	c := testCtx("lanzou:u", srv.URL, "cookie1")
	items, err := d.fileList(context.Background(), c, "-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Name != "dir" || items[0].FolID != "9" {
		t.Errorf("folder item wrong: %+v", items[0])
	}
	if items[1].NameAll != "a.zip" || items[1].ID != "8" {
		t.Errorf("file item wrong: %+v", items[1])
	}
	// pages 1 and 2 requested, loop stopped on the empty page
	if len(pgs) != 2 || pgs[0] != "1" || pgs[1] != "2" {
		t.Errorf("paging sequence = %v, want [1 2]", pgs)
	}
}

// TestFetchTextAcwRetry verifies that a challenge page is solved and the acw
// cookie is re-sent on the next attempt.
func TestFetchTextAcwRetry(t *testing.T) {
	withNoThrottle(t)
	attempts := 0
	var gotCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<script>var arg1='00112233445566778899AABBCCDDEEFF00112233';</script>`))
			return
		}
		gotCookie = r.Header.Get("Cookie")
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	res, err := fetchText(context.Background(), http.MethodGet, srv.URL, nil, nil, "base=1", false)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if res.text != "ok" {
		t.Fatalf("final text = %q, want ok", res.text)
	}
	if !strings.Contains(gotCookie, "acw_sc__v2=41eb1062441a5dadc03039c05aff6731a59d0124") {
		t.Errorf("acw cookie missing from %q", gotCookie)
	}
	if !strings.Contains(gotCookie, "base=1") {
		t.Errorf("original cookie lost in %q", gotCookie)
	}
}

func TestLanzouCookieLogin(t *testing.T) {
	withNoThrottle(t)
	oldDefaults := LANZOU_DEFAULT
	defer func() { LANZOU_DEFAULT = oldDefaults }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mydisk.php" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Cookie") != "ylogin=cookie-login" {
			t.Fatalf("cookie header = %q", r.Header.Get("Cookie"))
		}
		_, _ = w.Write([]byte(`uid=uid-cookie-login&data : {'vei':'vei-cookie-login'}`))
	}))
	defer srv.Close()
	LANZOU_DEFAULT.BaseURL = srv.URL

	tok, err := loginLanzouWithCookie(context.Background(), " ylogin=cookie-login ")
	if err != nil {
		t.Fatalf("cookie login: %v", err)
	}
	if tok.UserID != "lanzou_uid-cookie-login" || tok.ProviderAccountID != "uid-cookie-login" || tok.DeviceID != "vei-cookie-login" {
		t.Fatalf("unexpected identity: %+v", tok)
	}
	if tok.AccessToken != "ylogin=cookie-login" || tok.DefaultDriveID != "lanzou:uid-cookie-login" {
		t.Fatalf("unexpected session: %+v", tok)
	}
	cr := parseLanzouCred(tok.RefreshToken)
	if cr == nil || cr.Type != "cookie" || cr.Cookie != tok.AccessToken || cr.UID != "uid-cookie-login" || cr.VEI != "vei-cookie-login" || cr.BaseURL != srv.URL {
		t.Fatalf("unexpected persisted credentials: %+v", cr)
	}
}

func TestLanzouAccountLoginAndRefresh(t *testing.T) {
	withNoThrottle(t)
	oldDefaults := LANZOU_DEFAULT
	oldHTTPClient := httpClient
	oldManualClient := manualClient
	t.Cleanup(func() {
		LANZOU_DEFAULT = oldDefaults
		httpClient = oldHTTPClient
		manualClient = oldManualClient
	})

	var mu sync.Mutex
	loginCalls := 0
	expired := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/mlogin.php":
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse login form: %v", err)
			}
			if r.Form.Get("task") != "3" || r.Form.Get("uid") != "account-1" || r.Form.Get("pwd") != "secret-1" {
				t.Errorf("login form = %v", r.Form)
			}
			mu.Lock()
			loginCalls++
			call := loginCalls
			mu.Unlock()
			w.Header().Add("Set-Cookie", fmt.Sprintf("PHPSESSID=session-%d; Path=/", call))
			w.Header().Add("Set-Cookie", fmt.Sprintf("ylogin=login-%d; Path=/", call))
			_ = json.NewEncoder(w).Encode(map[string]any{"zt": 1})
		case "/mydisk.php":
			mu.Lock()
			isExpired := expired
			calls := loginCalls
			mu.Unlock()
			if isExpired && calls == 1 {
				_, _ = w.Write([]byte("<html>登录已过期</html>"))
				return
			}
			_, _ = w.Write([]byte(fmt.Sprintf("uid=uid-account-1&data : {'vei':'vei-account-%d'}", calls)))
		case "/doupload.php":
			_ = r.ParseForm()
			_ = json.NewEncoder(w).Encode(map[string]any{"zt": 0})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	LANZOU_DEFAULT.BaseURL = srv.URL
	rewrite := lanzouRewriteRT{host: srv.Listener.Addr().String()}
	httpClient = &http.Client{Transport: rewrite}
	manualClient = &http.Client{Transport: rewrite, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	tok, err := loginLanzouWithAccount(context.Background(), " account-1 ", "secret-1")
	if err != nil {
		t.Fatalf("account login: %v", err)
	}
	if tok.UserID != "lanzou_uid-account-1" || tok.ProviderAccountID != "uid-account-1" || tok.DeviceID != "vei-account-1" {
		t.Fatalf("unexpected account identity: %+v", tok)
	}
	if !strings.Contains(tok.AccessToken, "session-1") || !strings.Contains(tok.AccessToken, "ylogin=login-1") {
		t.Fatalf("unexpected login cookie: %q", tok.AccessToken)
	}

	mu.Lock()
	expired = true
	mu.Unlock()
	refreshed, err := (&Driver{}).RefreshAccount(context.Background(), drive.Context{Token: tok, UserID: tok.UserID, DriveID: tok.DefaultDriveID}, tok)
	if err != nil {
		t.Fatalf("account refresh: %v", err)
	}
	if refreshed != tok || !strings.Contains(tok.AccessToken, "session-2") || tok.DeviceID != "vei-account-2" {
		t.Fatalf("refresh did not replace session: %+v", tok)
	}
	cr := parseLanzouCred(tok.RefreshToken)
	if cr == nil || cr.Type != "account" || cr.Account != "account-1" || cr.Password != "secret-1" || cr.Cookie != tok.AccessToken || cr.UID != "uid-account-1" || cr.VEI != "vei-account-2" {
		t.Fatalf("refresh credentials not persisted: %+v", cr)
	}
	mu.Lock()
	gotLogins := loginCalls
	mu.Unlock()
	if gotLogins != 2 {
		t.Fatalf("login calls = %d, want 2", gotLogins)
	}
}

func TestLanzouUploadReloginPersistsRotatedCookie(t *testing.T) {
	withNoThrottle(t)
	oldDefaults := LANZOU_DEFAULT
	oldHTTPClient := httpClient
	oldManualClient := manualClient
	t.Cleanup(func() {
		LANZOU_DEFAULT = oldDefaults
		httpClient = oldHTTPClient
		manualClient = oldManualClient
	})

	var mu sync.Mutex
	loginCalls := 0
	uploadCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/html5up.php":
			mu.Lock()
			uploadCalls++
			call := uploadCalls
			mu.Unlock()
			if call == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{"zt": 9, "info": "expired"})
				return
			}
			if !strings.Contains(r.Header.Get("Cookie"), "ylogin=login-1") {
				t.Errorf("retry cookie = %q", r.Header.Get("Cookie"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"zt": 1})
		case "/mlogin.php":
			mu.Lock()
			loginCalls++
			call := loginCalls
			mu.Unlock()
			w.Header().Add("Set-Cookie", fmt.Sprintf("ylogin=login-%d; Path=/", call))
			_ = json.NewEncoder(w).Encode(map[string]any{"zt": 1})
		case "/mydisk.php":
			_, _ = w.Write([]byte("uid=uid-upload&data : {'vei':'vei-upload'}"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	LANZOU_DEFAULT.BaseURL = srv.URL
	rewrite := lanzouRewriteRT{host: srv.Listener.Addr().String()}
	httpClient = &http.Client{Transport: rewrite}
	manualClient = &http.Client{Transport: rewrite, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	tok := &model.TokenInfo{
		TokenFrom: model.ProviderLanzou, AccessToken: "ylogin=expired",
		RefreshToken: mustJSON(cred{Type: "account", Cookie: "ylogin=expired", Account: "account-1", Password: "secret-1", UID: "uid-upload", VEI: "vei-old", BaseURL: srv.URL}),
	}
	path := t.TempDir() + "/upload.txt"
	if err := os.WriteFile(path, []byte("upload-data"), 0600); err != nil {
		t.Fatal(err)
	}
	ui := &model.UploadingUI{Info: model.UploadInfo{LocalFilePath: path, Name: "upload.txt", ParentFileID: "-1"}}
	if err := (&Driver{}).UploadOneFile(context.Background(), drive.Context{Token: tok}, ui); err != nil {
		t.Fatalf("upload: %v", err)
	}
	if ui.Upload.DownState != "completed" || ui.Upload.DownProcess != 100 {
		t.Fatalf("upload state = %+v", ui.Upload)
	}
	if ui.Info.Size != int64(len("upload-data")) {
		t.Fatalf("upload size = %d, want %d", ui.Info.Size, len("upload-data"))
	}
	if tok.AccessToken != "ylogin=login-1" {
		t.Fatalf("rotated cookie not persisted: %q", tok.AccessToken)
	}
	cr := parseLanzouCred(tok.RefreshToken)
	if cr == nil || cr.Cookie != "ylogin=login-1" || cr.VEI != "vei-upload" {
		t.Fatalf("rotated credentials not persisted: %+v", cr)
	}
	mu.Lock()
	gotLogins, gotUploads := loginCalls, uploadCalls
	mu.Unlock()
	if gotLogins != 1 || gotUploads != 2 {
		t.Fatalf("login/upload calls = %d/%d, want 1/2", gotLogins, gotUploads)
	}
}

func TestLanzouShareDownloadAndVideoPreview(t *testing.T) {
	withNoThrottle(t)
	oldDefaults := LANZOU_DEFAULT
	oldManualClient := manualClient
	t.Cleanup(func() {
		LANZOU_DEFAULT = oldDefaults
		manualClient = oldManualClient
	})

	var serverURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/doupload.php":
			_ = r.ParseForm()
			if r.Form.Get("task") != "22" {
				t.Errorf("share task = %q", r.Form.Get("task"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"zt": 1, "info": map[string]any{
				"f_id": "share-1", "pwd": "", "isnewd": serverURL, "size": "2M",
			}})
		case "/share-1":
			if r.Header.Get("Cookie") != "cookie1" {
				t.Errorf("share cookie = %q, want cookie1", r.Header.Get("Cookie"))
			}
			_, _ = w.Write([]byte(`<iframe src="/download-page"></iframe>`))
		case "/download-page":
			_, _ = w.Write([]byte(`<script>var x='/ajaxm.php?file=123';</script><script>var data : {'uid':'u'};</script><title>video.mp4 - 蓝奏云</title>`))
		case "/ajaxm.php":
			_ = json.NewEncoder(w).Encode(map[string]any{"zt": 1, "dom": serverURL, "url": "direct-1", "inf": "video.mp4"})
		case "/file/direct-1":
			w.Header().Set("Location", serverURL+"/media/video.mp4")
			w.WriteHeader(http.StatusFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	serverURL = srv.URL
	LANZOU_DEFAULT.BaseURL = srv.URL
	LANZOU_DEFAULT.ShareURL = srv.URL
	manualClient = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	c := testCtx("lanzou:u", srv.URL, "cookie1")
	d := &Driver{}
	share, err := d.CreateShare(context.Background(), c, drive.ShareParams{FileIDs: []string{"file-1"}, ShareName: "测试分享"})
	if err != nil {
		t.Fatalf("create share: %v", err)
	}
	if share.ShareID != "share-1" || share.ShareURL != srv.URL+"/share-1" || share.ShareName != "测试分享" {
		t.Fatalf("share = %+v", share)
	}
	download, err := d.GetDownloadURL(context.Background(), c, "file-1", 0)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if download.URL != srv.URL+"/media/video.mp4" || download.Size != 2*1024*1024 {
		t.Fatalf("download = %+v", download)
	}
	preview, err := d.GetVideoPreview(context.Background(), c, "file-1")
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if preview.Size != download.Size || len(preview.Qualities) != 1 || !preview.Qualities[0].ForceProxy {
		t.Fatalf("preview = %+v", preview)
	}

	// Historical Lanzou tokens sometimes used the literal "cookie" marker in
	// access_token while the real Cookie lived in refresh_token.
	c.Token.AccessToken = "cookie"
	if _, err := d.GetDownloadURL(context.Background(), c, "file-1", 0); err != nil {
		t.Fatalf("download with legacy cookie marker: %v", err)
	}
}

func TestLanzouMoveRejectsCachedFolder(t *testing.T) {
	const userID = "lanzou_move-cache-test"
	const driveID = "lanzou:move-cache-test"
	drive.RememberFile(userID, driveID, model.File{DriveID: driveID, FileID: "folder-1", IsDir: true})
	_, err := (&Driver{}).Move(context.Background(), drive.Context{UserID: userID, DriveID: driveID}, []drive.FileRef{{ID: "folder-1"}}, "target", "")
	if err == nil || !strings.Contains(err.Error(), "不支持移动文件夹") {
		t.Fatalf("move folder error = %v", err)
	}
}

type lanzouRewriteRT struct{ host string }

func (r lanzouRewriteRT) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Hostname() != "up.woozooo.com" {
		return http.DefaultTransport.RoundTrip(req)
	}
	u := *req.URL
	u.Scheme = "http"
	u.Host = r.host
	req2 := req.Clone(req.Context())
	req2.URL = &u
	req2.RequestURI = ""
	return http.DefaultTransport.RoundTrip(req2)
}
