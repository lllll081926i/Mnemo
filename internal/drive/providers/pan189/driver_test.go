package pan189

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

func TestRegistration(t *testing.T) {
	reg, ok := drive.Get(model.ProviderPan189)
	if !ok {
		t.Fatal("pan189 not registered")
	}
	if reg.ID != model.ProviderPan189 {
		t.Fatalf("id = %q", reg.ID)
	}
	if reg.Meta.Label != "天翼云盘" {
		t.Fatalf("meta label = %q", reg.Meta.Label)
	}
	if reg.Auth == nil {
		t.Fatal("Auth must be wired for account+password login")
	}
	// login form must carry username/password
	keys := map[string]bool{}
	for _, f := range reg.Login.Fields {
		keys[f.Key] = true
	}
	if !keys["username"] || !keys["password"] {
		t.Fatalf("login fields missing username/password: %v", keys)
	}
	// factory builds a working driver
	d := reg.Factory()
	if d.ID() != model.ProviderPan189 {
		t.Fatalf("driver id = %q", d.ID())
	}
	if d.RootID() != PAN189Root {
		t.Fatalf("root = %q", d.RootID())
	}
}

func TestCapabilities(t *testing.T) {
	caps := drive.RegistryCaps(model.ProviderPan189)
	// legacy pan189 overrides
	if !caps.Copy {
		t.Error("copy must be enabled (legacy: copy: true)")
	}
	if !caps.RecycleBin {
		t.Error("recycleBin must be enabled")
	}
	if !caps.PermanentDelete {
		t.Error("permanentDelete must be enabled")
	}
	if caps.Search {
		t.Error("search must stay disabled (legacy: search: false)")
	}
	if !caps.CreateShare || !caps.ShareExpiration || !caps.ShareHistory {
		t.Errorf("share capabilities = %+v", caps)
	}
	if caps.SharePassword {
		t.Error("天翼云盘分享提取码由服务端生成，不能声明自定义密码能力")
	}
	if got := caps.ShareExpirationOptions; len(got) != 3 || got[0] != 0 || got[1] != 1 || got[2] != 7 {
		t.Errorf("shareExpirationOptions = %v, want [0 1 7]", got)
	}
	if caps.TrashView {
		t.Error("trashView must stay disabled")
	}
	// md5 hashes both directions
	if len(caps.ProvideHashes) != 1 || caps.ProvideHashes[0] != "md5" {
		t.Errorf("provideHashes = %v", caps.ProvideHashes)
	}
	if len(caps.RapidUploadHashes) != 1 || caps.RapidUploadHashes[0] != "md5" {
		t.Errorf("rapidUploadHashes = %v", caps.RapidUploadHashes)
	}
	// standard file baseline retained
	if !caps.Upload || !caps.Download || !caps.CreateFolder || !caps.Rename || !caps.Move {
		t.Error("standard file capabilities must remain enabled")
	}
}

func TestExpireTimeFromURL(t *testing.T) {
	// AWS-style signed URL
	aws := "https://dl.example.com/x?X-Amz-Date=20240102T030405Z&X-Amz-Expires=3600&sig=abc"
	// base 2024-01-02 03:04:05 UTC = 1704164645000 ms
	if got := expireTimeFromURL(aws); got != 1704164645000+3600*1000 {
		t.Fatalf("aws expire = %d", got)
	}
	// x-oss-expires epoch seconds
	oss := "https://dl.example.com/x?x-oss-expires=1700000000"
	if got := expireTimeFromURL(oss); got != 1700000000*1000 {
		t.Fatalf("oss expire = %d", got)
	}
	// plain expire query (seconds timestamp)
	if got := expireTimeFromURL("https://dl.example.com/x?expire=1700000000&e=1"); got != 1700000000*1000 {
		t.Fatalf("expire = %d", got)
	}
	// RFC3339 expire value
	if got := expireTimeFromURL("https://dl.example.com/x?expires=2024-01-02T03:04:05Z"); got != 1704164645000 {
		t.Fatalf("rfc3339 expire = %d", got)
	}
	// no expiry params → 0
	if got := expireTimeFromURL("https://dl.example.com/x?foo=bar"); got != 0 {
		t.Fatalf("no-expire = %d", got)
	}
	if got := expireTimeFromURL(""); got != 0 {
		t.Fatalf("empty = %d", got)
	}
	// 189 download urls carry an expires-like query (present in practice)
	real := "https://d.pcs.189.cn/a?&expires=1710000000&x=1"
	if got := expireTimeFromURL(real); got != 1710000000*1000 {
		t.Fatalf("189-style expire = %d", got)
	}
}

func TestDriverImplementsInterface(t *testing.T) {
	var _ drive.Driver = (*Driver)(nil)
}

func TestGetInfoPseudoEntries(t *testing.T) {
	d := &Driver{}
	c := drive.Context{UserID: "pan189:u", DriveID: "pan189:u"}
	for _, root := range []string{PAN189Root, "-11", "root", "/"} {
		info, err := d.GetInfo(t.Context(), c, root)
		if err != nil {
			t.Fatal(err)
		}
		f, ok := info.(model.File)
		if !ok || !f.IsDir || f.FileID != PAN189Root {
			t.Fatalf("GetInfo(%q) = %+v", root, info)
		}
	}
	if _, err := d.GetInfo(t.Context(), c, "file-1"); err == nil {
		t.Fatal("uncached file metadata must not be fabricated")
	}
}

func TestGetFileAndResolveTransferHashUseCachedMetadata(t *testing.T) {
	drive.ClearFileMetaCache()
	t.Cleanup(drive.ClearFileMetaCache)
	c := drive.Context{UserID: "pan189:cached", DriveID: "pan189:cached"}
	drive.RememberFile(c.UserID, c.DriveID, model.File{
		DriveID: c.DriveID, FileID: "file-1", ParentFileID: PAN189Root,
		Name: "movie.mp4", Size: 4096,
		ContentHashName: "md5", ContentHash: "D41D8CD98F00B204E9800998ECF8427E",
	})

	f, err := (&Driver{}).GetFile(t.Context(), c, "file-1")
	if err != nil {
		t.Fatal(err)
	}
	if f.Name != "movie.mp4" || f.Size != 4096 || f.ContentHash == "" {
		t.Fatalf("GetFile() = %+v", f)
	}
	hash, err := (&Driver{}).ResolveTransferHash(t.Context(), c, "file-1", "md5", false)
	if err != nil {
		t.Fatal(err)
	}
	if hash != "d41d8cd98f00b204e9800998ecf8427e" {
		t.Fatalf("ResolveTransferHash() = %q", hash)
	}
}

func TestRapidUploadByHashHitMissInvalidAndMalformedResponses(t *testing.T) {
	previous := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = previous })
	sess := &Session{SessionKey: "session-key", SessionSecret: "session-secret", CloudType: CloudPersonal}
	c := drive.Context{UserID: "pan189:user", DriveID: "pan189:user", Token: &model.TokenInfo{AccessToken: sess.SessionKey, RefreshToken: mustJSON(sess)}}
	d := &Driver{}
	valid := drive.RapidUploadRequest{ParentID: PAN189Root, FileName: "movie.mp4", Method: "md5", Hash: "d41d8cd98f00b204e9800998ecf8427e", Size: 4096}

	for _, tc := range []struct {
		name       string
		createBody string
		wantReuse  bool
		wantErr    bool
	}{
		{name: "hit", createBody: `{"data":{"uploadFileId":"upload-1","fileCommitUrl":"https://api.cloud.189.cn/commit.action","fileDataExists":1}}`, wantReuse: true},
		{name: "miss", createBody: `{"data":{"uploadFileId":"upload-2","fileDataExists":0}}`},
		{name: "malformed", createBody: `{"data":"invalid"}`, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			netx.TestTransportHook = pan189AuthRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/createUploadFile.action":
					return pan189AuthResponse(req, http.StatusOK, nil, tc.createBody), nil
				case "/commit.action":
					return pan189AuthResponse(req, http.StatusOK, nil, `<response><id>file-9</id></response>`), nil
				default:
					return nil, fmt.Errorf("unexpected request %s", req.URL.String())
				}
			})
			result, err := d.RapidUploadByHash(t.Context(), c, valid)
			if (err != nil) != tc.wantErr {
				t.Fatalf("RapidUploadByHash() error = %v", err)
			}
			if err == nil && (result == nil || result.Reuse != tc.wantReuse) {
				t.Fatalf("RapidUploadByHash() = %+v", result)
			}
		})
	}

	for _, req := range []drive.RapidUploadRequest{
		{Method: "sha1", Hash: valid.Hash, Size: 1},
		{Method: "md5", Hash: "invalid", Size: 1},
		{Method: "md5", Hash: valid.Hash, Size: -1},
	} {
		result, err := d.RapidUploadByHash(t.Context(), c, req)
		if err != nil {
			t.Fatalf("invalid request returned error: %v", err)
		}
		if result == nil || result.Reuse {
			t.Fatalf("invalid request result = %+v", result)
		}
	}
}

func TestCreateShareUsesPersonalShareEndpoint(t *testing.T) {
	previous := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = previous })
	var requests int
	netx.TestTransportHook = pan189AuthRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		if req.Method != http.MethodGet || req.URL.Host != "cloud.189.cn" || req.URL.Path != "/api/open/share/createShareLink.action" {
			return nil, fmt.Errorf("unexpected request %s %s", req.Method, req.URL.String())
		}
		query := req.URL.Query()
		if query.Get("fileId") != "file-1" || query.Get("expireTime") != "7" || query.Get("shareType") != "3" || query.Get("noCache") == "" {
			return nil, fmt.Errorf("share query = %s", query.Encode())
		}
		if req.Header.Get("SessionKey") != "session-key" {
			return nil, fmt.Errorf("session key = %q", req.Header.Get("SessionKey"))
		}
		return pan189AuthResponse(req, http.StatusOK, nil, `{"res_code":0,"shareLinkList":[{"shareId":12345,"accessCode":"p4ss","accessUrl":"https://cloud.189.cn/t/test-share"}]}`), nil
	})

	sess := &Session{SessionKey: "session-key", SessionSecret: "session-secret", CloudType: CloudPersonal}
	item, err := (&Driver{}).CreateShare(context.Background(), drive.Context{
		UserID: "pan189:user", DriveID: "pan189:user",
		Token: &model.TokenInfo{AccessToken: sess.SessionKey, RefreshToken: mustJSON(sess)},
	}, drive.ShareParams{FileIDs: []string{"file-1"}, ShareName: "测试分享", Expiration: "7"})
	if err != nil {
		t.Fatalf("CreateShare() error = %v", err)
	}
	if requests != 1 {
		t.Fatalf("request count = %d, want 1", requests)
	}
	if item.ShareID != "12345" || item.ShareURL != "https://cloud.189.cn/t/test-share" || item.SharePwd != "p4ss" || item.FileID != "file-1" {
		t.Fatalf("share = %+v", item)
	}
}

func TestCreateShareRejectsFamilyCloud(t *testing.T) {
	sess := &Session{
		SessionKey: "session-key", SessionSecret: "session-secret", CloudType: CloudFamily,
		FamilySessionKey: "family-key", FamilySessionSecret: "family-secret",
	}
	_, err := (&Driver{}).CreateShare(t.Context(), drive.Context{Token: &model.TokenInfo{AccessToken: sess.SessionKey, RefreshToken: mustJSON(sess)}}, drive.ShareParams{FileIDs: []string{"file-1"}})
	if err == nil {
		t.Fatal("family cloud share must be rejected before a request")
	}
}
