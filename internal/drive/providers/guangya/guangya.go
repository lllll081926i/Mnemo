// Package guangya implements the 光鸭云盘 provider.
package guangya

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/drive/driveutil"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

const (
	accountHost = "https://account.guangyapan.com"
	apiHost     = "https://api.guangyapan.com"
	clientID    = "aMe-8VSlkrbQXpUR"
	RootID      = "guangya_root"
	ua          = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

const providerID = model.ProviderGuangya

func init() {
	drive.Register(drive.Registration{
		ID:   providerID,
		Meta: drive.GetMeta(providerID),
		Caps: drive.NewCapabilities(providerID, map[string]bool{
			"copy":            true,
			"permanentDelete": true,
		}, func(c *drive.Capabilities) {
			c.SetHashes([]string{"md5"}, nil)
		}),
		Login: drive.LoginConfig{Fields: []drive.LoginField{
			{Key: "phone", Type: "text", Label: "手机号", Required: false},
			{Key: "sms_code", Type: "text", Label: "短信验证码", Required: false},
			{Key: "refresh_token", Type: "text", Label: "Refresh Token（可选，直接登录）", Required: false},
		}},
		Auth:    authLogin,
		Factory: func() drive.Driver { return &Driver{} },
	})
}

// Session is the guangya credential set.
type Session struct {
	AccessToken   string `json:"access_token"`
	RefreshToken  string `json:"refresh_token"`
	DeviceID      string `json:"device_id"`
	ClientID      string `json:"client_id"`
	RootFolderID  string `json:"root_folder_id"`
	Phone         string `json:"phone,omitempty"`
}

// client is an authenticated guangya session.
type client struct {
	http *netx.Client
	sess *Session
}

func newDeviceID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func clientOf(c drive.Context) (*client, error) {
	if c.Token == nil {
		return nil, drive.ErrUnauthorized
	}
	sess := parseSession(c.Token)
	if sess == nil {
		return nil, errors.New("光鸭云盘未登录")
	}
	if sess.DeviceID == "" {
		sess.DeviceID = newDeviceID()
	}
	if sess.ClientID == "" {
		sess.ClientID = clientID
	}
	return &client{http: netx.NewClient(60 * time.Second), sess: sess}, nil
}

func parseSession(tok *model.TokenInfo) *Session {
	if tok == nil {
		return nil
	}
	var s Session
	if tok.RefreshToken != "" {
		if err := json.Unmarshal([]byte(tok.RefreshToken), &s); err == nil && (s.AccessToken != "" || s.RefreshToken != "") {
			if s.AccessToken == "" {
				s.AccessToken = tok.AccessToken
			}
			return &s
		}
	}
	if tok.AccessToken != "" {
		return &Session{AccessToken: tok.AccessToken, DeviceID: newDeviceID(), ClientID: clientID}
	}
	return nil
}

// apiHeaders builds the account headers.
func (c *client) apiHeaders(extra map[string]string) map[string]string {
	h := map[string]string{
		"Content-Type": "application/json", "Accept": "application/json, text/plain, */*",
		"X-Client-Id": c.sess.ClientID, "X-Client-Version": "0.0.1",
		"X-Device-Id": c.sess.DeviceID, "X-Device-Model": "chrome%2F147.0.0.0",
		"X-Device-Name": "PC-Chrome",
		"X-Device-Sign": fmt.Sprintf("wdi10.%sxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", c.sess.DeviceID),
		"X-Net-Work-Type": "NONE", "X-OS-Version": "MacIntel", "X-Platform-Version": "1",
		"X-Protocol-Version": "301", "X-Provider-Name": "NONE", "X-SDK-Version": "9.0.2",
		"User-Agent": ua,
	}
	if c.sess.AccessToken != "" {
		h["Authorization"] = "Bearer " + c.sess.AccessToken
	}
	for k, v := range extra {
		h[k] = v
	}
	return h
}

// post calls the guangya API (rate limited 500ms).
func (c *client) post(ctx context.Context, path string, body any, out any) error {
	time.Sleep(500 * time.Millisecond)
	resp, err := c.http.Do(ctx, http.MethodPost, apiHost+path, c.apiHeaders(nil), netx.JSONBody(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return errors.New("guangya: http " + fmt.Sprint(resp.StatusCode) + ": " + truncate(data, 200))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

// File is a raw guangya file entry.
type File struct {
	FileID   string `json:"fileId"`
	FileName string `json:"fileName"`
	FileSize int64  `json:"fileSize"`
	ResType  int    `json:"resType"` // 2 = folder
	CTime    int64  `json:"cTime"`
	UTime    int64  `json:"uTime"`
	ParentID string `json:"parentId"`
	MD5      string `json:"md5,omitempty"`
}

// List returns files of a folder.
func (c *client) List(ctx context.Context, parentID string) ([]File, error) {
	var out []File
	for page := 0; page < 200; page++ {
		var resp struct {
			Data *struct {
				List  []map[string]any `json:"list"`
				Total int              `json:"total"`
			} `json:"data"`
		}
		err := c.post(ctx, "/userres/v1/file/get_file_list", map[string]any{
			"parentId": parentID, "page": page, "pageSize": 100,
			"orderBy": 3, "sortType": 1, "fileTypes": []any{},
		}, &resp)
		if err != nil {
			return nil, err
		}
		if resp.Data == nil {
			break
		}
		list := resp.Data.List
		for _, item := range list {
			out = append(out, File{
				FileID:   str(item["fileId"], item["file_id"]),
				FileName: str(item["fileName"], item["name"]),
				FileSize: num(item["fileSize"], item["size"]),
				ResType:  int(num(item["resType"], item["res_type"])),
				CTime:    int64(num(item["cTime"], item["ctime"])),
				UTime:    int64(num(item["uTime"], item["utime"])),
				ParentID: parentID,
			})
		}
		if len(list) < 100 {
			break
		}
		if resp.Data.Total > 0 && len(out) >= resp.Data.Total {
			break
		}
	}
	return out, nil
}

// Detail returns one file detail (with md5).
func (c *client) Detail(ctx context.Context, fileID string) (*File, error) {
	var resp struct {
		Data *struct {
			List []map[string]any `json:"list"`
		} `json:"data"`
	}
	err := c.post(ctx, "/userres/v1/file/get_file_detail", map[string]any{"fileId": fileID}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Data != nil && len(resp.Data.List) > 0 {
		item := resp.Data.List[0]
		return &File{
			FileID: fileID, FileName: str(item["fileName"], item["name"]),
			FileSize: num(item["fileSize"], item["size"]),
			ResType:  int(num(item["resType"], item["res_type"])),
			UTime:    int64(num(item["uTime"], item["utime"])),
			MD5:      str(item["md5"]),
		}, nil
	}
	return &File{FileID: fileID, FileName: fileID}, nil
}

// DownloadInfo resolves the download url.
func (c *client) DownloadInfo(ctx context.Context, fileID string) (string, error) {
	var resp struct {
		Data *struct {
			URL string `json:"downloadUrl"`
		} `json:"data"`
	}
	err := c.post(ctx, "/userres/v1/file/get_res_download_url", map[string]any{"fileId": fileID}, &resp)
	if err != nil {
		return "", err
	}
	if resp.Data == nil || resp.Data.URL == "" {
		return "", errors.New("guangya: 获取下载地址失败")
	}
	return resp.Data.URL, nil
}

// Mkdir creates a folder.
func (c *client) Mkdir(ctx context.Context, parentID, name string) (*drive.MkdirResult, error) {
	var resp struct {
		Data *struct {
			FileID string `json:"fileId"`
		} `json:"data"`
	}
	err := c.post(ctx, "/userres/v1/file/create_dir", map[string]any{"parentId": parentID, "dirName": name}, &resp)
	if err != nil {
		return &drive.MkdirResult{Error: err.Error()}, nil
	}
	if resp.Data != nil {
		return &drive.MkdirResult{FileID: resp.Data.FileID}, nil
	}
	return &drive.MkdirResult{Error: "创建文件夹失败"}, nil
}

// Rename renames a file/folder.
func (c *client) Rename(ctx context.Context, fileID, name string) error {
	return c.post(ctx, "/userres/v1/file/rename", map[string]any{"fileId": fileID, "fileName": name}, nil)
}

// Move moves files.
func (c *client) Move(ctx context.Context, fileIDs []string, toParentID string) error {
	return c.post(ctx, "/userres/v1/file/move_file", map[string]any{"fileIds": fileIDs, "parentId": toParentID}, nil)
}

// Copy copies files.
func (c *client) Copy(ctx context.Context, fileIDs []string, toParentID string) error {
	return c.post(ctx, "/userres/v1/file/copy_file", map[string]any{"fileIds": fileIDs, "parentId": toParentID}, nil)
}

// Delete permanently deletes files.
func (c *client) Delete(ctx context.Context, fileIDs []string) error {
	return c.post(ctx, "/userres/v1/file/delete_file", map[string]any{"fileIds": fileIDs}, nil)
}

// SpaceInfo returns quota.
func (c *client) SpaceInfo(ctx context.Context) (used, total int64) {
	var resp struct {
		Data *struct {
			UsedSize  int64 `json:"usedSize"`
			TotalSize int64 `json:"totalSize"`
		} `json:"data"`
	}
	if err := c.post(ctx, "/userres/v1/user/space", map[string]any{}, &resp); err == nil && resp.Data != nil {
		return resp.Data.UsedSize, resp.Data.TotalSize
	}
	return 0, 0
}

// ---- driver ----

// Driver implements drive.Driver for guangya.
type Driver struct {
	drive.BaseDriver
}

func (d *Driver) ID() string                       { return providerID }
func (d *Driver) Meta() drive.Meta                 { return drive.GetMeta(providerID) }
func (d *Driver) Capabilities() drive.Capabilities { return drive.RegistryCaps(providerID) }
func (d *Driver) RootID() string                   { return RootID }

func toID(id string) string {
	if id == "" || id == RootID || id == "root" || id == "/" {
		return ""
	}
	return id
}

func (d *Driver) List(ctx context.Context, c drive.Context, dirID string, _ *drive.ListOptions) ([]model.File, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	items, err := cl.List(ctx, toID(dirID))
	if err != nil {
		return nil, err
	}
	out := make([]model.File, 0, len(items))
	for _, it := range items {
		out = append(out, mapFile(&it, c.DriveID, dirID))
	}
	return out, nil
}

func (d *Driver) GetInfo(ctx context.Context, c drive.Context, fileID string) (any, error) {
	if toID(fileID) == "" {
		return model.File{DriveID: c.DriveID, FileID: RootID, Name: "光鸭云盘", NameSearch: "guangya", IsDir: true, Icon: "iconfile-folder"}, nil
	}
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	it, err := cl.Detail(ctx, fileID)
	if err != nil {
		return nil, err
	}
	f := mapFile(it, c.DriveID, "")
	return f, nil
}

func (d *Driver) GetFile(ctx context.Context, c drive.Context, fileID string) (*model.File, error) {
	v, err := d.GetInfo(ctx, c, fileID)
	if err != nil {
		return nil, err
	}
	f, ok := v.(model.File)
	if !ok {
		return nil, drive.ErrNotFound
	}
	return &f, nil
}

func (d *Driver) GetDownloadURL(ctx context.Context, c drive.Context, fileID string, _ int) (*model.DownloadURL, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	u, err := cl.DownloadInfo(ctx, fileID)
	if err != nil {
		return nil, err
	}
	return &model.DownloadURL{DriveID: c.DriveID, FileID: fileID, URL: u, DownloadMode: "redirect"}, nil
}

func (d *Driver) GetVideoPreview(ctx context.Context, c drive.Context, fileID string) (*model.VideoPreview, error) {
	u, err := d.GetDownloadURL(ctx, c, fileID, 0)
	if err != nil {
		return nil, err
	}
	return &model.VideoPreview{
		DriveID: c.DriveID, FileID: fileID, Size: u.Size, Headers: u.Headers,
		Qualities: []model.VideoQuality{{Quality: "origin", Label: "原画", Value: "origin", URL: u.URL, Headers: u.Headers, ForceProxy: true}},
	}, nil
}

func (d *Driver) Mkdir(ctx context.Context, c drive.Context, parentID, name string) (*drive.MkdirResult, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	return cl.Mkdir(ctx, toID(parentID), name)
}

func (d *Driver) Rename(ctx context.Context, c drive.Context, fileID, name string) (*drive.RenameResult, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	if err := cl.Rename(ctx, fileID, name); err != nil {
		return nil, err
	}
	return &drive.RenameResult{FileID: fileID, Name: name}, nil
}

func (d *Driver) Trash(ctx context.Context, c drive.Context, fileIDs []string) ([]string, error) {
	return d.Delete(ctx, c, idsToRefs(fileIDs))
}

func (d *Driver) Delete(ctx context.Context, c drive.Context, refs []drive.FileRef) ([]string, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(refs))
	for _, r := range refs {
		ids = append(ids, r.ID)
	}
	if err := cl.Delete(ctx, ids); err != nil {
		return nil, err
	}
	return ids, nil
}

func (d *Driver) Move(ctx context.Context, c drive.Context, refs []drive.FileRef, toParentID, _ string) ([]string, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(refs))
	for _, r := range refs {
		ids = append(ids, r.ID)
	}
	if err := cl.Move(ctx, ids, toID(toParentID)); err != nil {
		return nil, err
	}
	return ids, nil
}

func (d *Driver) Copy(ctx context.Context, c drive.Context, refs []drive.FileRef, toParentID, _ string) ([]string, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(refs))
	for _, r := range refs {
		ids = append(ids, r.ID)
	}
	if err := cl.Copy(ctx, ids, toID(toParentID)); err != nil {
		return nil, err
	}
	return ids, nil
}

func (d *Driver) ResolveTransferHash(ctx context.Context, c drive.Context, fileID, method string, _ bool) (string, error) {
	if method != "md5" {
		return "", nil
	}
	cl, err := clientOf(c)
	if err != nil {
		return "", err
	}
	it, err := cl.Detail(ctx, fileID)
	if err != nil {
		return "", err
	}
	return it.MD5, nil
}

func (d *Driver) RefreshAccount(ctx context.Context, c drive.Context, token *model.TokenInfo) (*model.TokenInfo, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	token.UsedSize, token.TotalSize = cl.SpaceInfo(ctx)
	return token, nil
}

func mapFile(it *File, driveID, parentID string) model.File {
	isDir := it.ResType == 2
	f := driveutil.NewFile(driveID, it.FileID, parentID, it.FileName, isDir, it.FileSize, it.UTime)
	f.ContentHash = it.MD5
	if it.MD5 != "" {
		f.ContentHashName = "md5"
	}
	return f
}

// ---- auth ----

func normalizePhone(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(p), " ", "")
	if strings.HasPrefix(p, "+") {
		return p
	}
	if len(p) == 11 && p[0] == '1' {
		return "+86" + p
	}
	return p
}

// authLogin handles refresh_token or SMS login.
func authLogin(ctx context.Context, req drive.AuthRequest) (*model.TokenInfo, error) {
	refresh := strings.TrimSpace(req.Config["refresh_token"])
	if refresh != "" {
		return loginByRefreshToken(ctx, refresh)
	}
	phone := strings.TrimSpace(req.Config["phone"])
	code := strings.TrimSpace(req.Config["sms_code"])
	if phone == "" || code == "" {
		return nil, errors.New("光鸭云盘：请填写手机号和验证码（或 Refresh Token）")
	}
	return loginBySms(ctx, phone, code, req.Config["verification_id"], req.Config["device_id"])
}

func loginByRefreshToken(ctx context.Context, refresh string) (*model.TokenInfo, error) {
	hc := netx.NewClient(60 * time.Second)
	deviceID := newDeviceID()
	headers := map[string]string{
		"Content-Type": "application/json", "Accept": "application/json",
		"X-Client-Id": clientID, "X-Client-Version": "0.0.1", "X-Device-Id": deviceID,
	}
	var resp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	err := hc.PostJSON(ctx, accountHost+"/v1/auth/token", headers, map[string]any{
		"client_id": clientID, "grant_type": "refresh_token", "refresh_token": refresh,
	}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.AccessToken == "" {
		return nil, errors.New("guangya: 刷新 token 失败")
	}
	return buildToken(&Session{
		AccessToken: resp.AccessToken, RefreshToken: firstNonEmpty(resp.RefreshToken, refresh),
		DeviceID: deviceID, ClientID: clientID,
	}), nil
}

// SendSms requests a verification code for a phone.
func SendSms(ctx context.Context, phone string) (verificationID, deviceID, captchaToken string, err error) {
	hc := netx.NewClient(60 * time.Second)
	deviceID = newDeviceID()
	headers := map[string]string{
		"Content-Type": "application/json", "Accept": "application/json",
		"X-Client-Id": clientID, "X-Client-Version": "0.0.1", "X-Device-Id": deviceID,
	}
	var resp struct {
		VerificationID string `json:"verification_id"`
		Error          string `json:"error"`
		ErrorDesc      string `json:"error_description"`
	}
	if err := hc.PostJSON(ctx, accountHost+"/v1/auth/verification", headers, map[string]any{
		"phone_number": normalizePhone(phone), "target": "ANY", "client_id": clientID,
	}, &resp); err != nil {
		return "", "", "", err
	}
	if resp.VerificationID == "" {
		msg := resp.ErrorDesc
		if msg == "" {
			msg = resp.Error
		}
		if msg == "" {
			msg = "发送验证码失败"
		}
		return "", "", "", errors.New(msg)
	}
	return resp.VerificationID, deviceID, "", nil
}

func loginBySms(ctx context.Context, phone, code, verificationID, deviceID string) (*model.TokenInfo, error) {
	if deviceID == "" {
		deviceID = newDeviceID()
	}
	hc := netx.NewClient(60 * time.Second)
	headers := map[string]string{
		"Content-Type": "application/json", "Accept": "application/json",
		"X-Client-Id": clientID, "X-Client-Version": "0.0.1", "X-Device-Id": deviceID,
	}
	var verifyResp struct {
		VerificationToken string `json:"verification_token"`
	}
	if err := hc.PostJSON(ctx, accountHost+"/v1/auth/verification/verify", headers, map[string]any{
		"verification_id": verificationID, "verification_code": code, "client_id": clientID,
	}, &verifyResp); err != nil {
		return nil, err
	}
	if verifyResp.VerificationToken == "" {
		return nil, errors.New("guangya: 验证码校验失败")
	}
	var signResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	err := hc.PostJSON(ctx, accountHost+"/v1/auth/signin", headers, map[string]any{
		"verification_code": code, "verification_token": verifyResp.VerificationToken,
		"username": normalizePhone(phone), "client_id": clientID,
	}, &signResp)
	if err != nil {
		return nil, err
	}
	if signResp.AccessToken == "" {
		return nil, errors.New("guangya: 登录失败")
	}
	return buildToken(&Session{
		AccessToken: signResp.AccessToken, RefreshToken: signResp.RefreshToken,
		DeviceID: deviceID, ClientID: clientID, Phone: normalizePhone(phone),
	}), nil
}

func buildToken(sess *Session) *model.TokenInfo {
	uid := sess.Phone
	if uid == "" {
		uid = sess.DeviceID[:8]
	}
	name := "光鸭 " + uid
	return &model.TokenInfo{
		TokenFrom:    providerID,
		AccessToken:  sess.AccessToken,
		RefreshToken: mustJSON(sess),
		TokenType:    "Bearer",
		UserID:       model.BuildUserID(providerID, uid),
		UserName:     name,
		NickName:     name,
		Name:         name,
		ProviderAccountID: uid,
		ProviderRootID:    RootID,
	}
}

// ---- helpers ----

func str(vals ...any) string {
	for _, v := range vals {
		switch t := v.(type) {
		case string:
			if t != "" {
				return t
			}
		case float64:
			return fmt.Sprintf("%.0f", t)
		}
	}
	return ""
}

func num(vals ...any) int64 {
	for _, v := range vals {
		switch t := v.(type) {
		case float64:
			return int64(t)
		case int:
			return int64(t)
		}
	}
	return 0
}

func idsToRefs(ids []string) []drive.FileRef {
	refs := make([]drive.FileRef, 0, len(ids))
	for _, id := range ids {
		refs = append(refs, drive.FileRef{ID: id})
	}
	return refs
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func truncate(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n])
	}
	return string(b)
}
// SendSms satisfies the app binding interface (sends a verification code).
func (d *Driver) SendSms(ctx context.Context, phone string) (verificationID, deviceID, captchaToken string, err error) {
	return SendSms(ctx, phone)
}
