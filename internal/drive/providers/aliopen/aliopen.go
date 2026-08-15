// Package aliopen implements the Aliyun Drive Open API provider (AList-sourced).
package aliopen

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/drive/driveutil"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

const (
	apiHost      = "https://openapi.alipan.com"
	oauthDefault = "https://api.alistgo.com/alist/ali_open/token"
	RootID       = "aliopen_root"
	BackupRoot   = "backup_root"
	ResourceRoot = "resource_root"

	partSize = 10 * 1024 * 1024 // 10 MiB upload parts
)

const providerID = model.ProviderAliopen

func init() {
	drive.Register(drive.Registration{
		ID:   providerID,
		Meta: drive.GetMeta(providerID),
		Caps: drive.NewCapabilities(providerID, map[string]bool{
			"search":          true,
			"createShare":     true,
			"shareExpiration": true,
			"sharePassword":   true,
			"shareHistory":    true,
			"importShare":     true,
			"copy":            true,
			"recycleBin":      true,
			"permanentDelete": true,
			"trashView":       false,
			"trashRestore":    false,
		}, func(c *drive.Capabilities) {
			c.SetHashes([]string{"sha1"}, []string{"sha1"})
		}),
		Auth: authRefreshToken,
		Login: drive.LoginConfig{Fields: []drive.LoginField{
			{Key: "refresh_token", Type: "text", Label: "Refresh Token", Required: true, Placeholder: "粘贴阿里云盘 Open refresh_token"},
			{Key: "client_id", Type: "text", Label: "Client ID（可选）", Required: false},
			{Key: "client_secret", Type: "password", Label: "Client Secret（可选）", Required: false},
		}},
		Factory: func() drive.Driver { return &Driver{} },
	})
}

// Session is the raw credential JSON blob.
type Session struct {
	AccessToken     string `json:"access_token"`
	RefreshToken    string `json:"refresh_token"`
	DriveID         string `json:"drive_id"`
	ResourceDriveID string `json:"resource_drive_id"`
	BackupDriveID   string `json:"backup_drive_id"`
	ClientID        string `json:"client_id,omitempty"`
	ClientSecret    string `json:"client_secret,omitempty"`
	OAuthTokenURL   string `json:"oauth_token_url,omitempty"`
}

// Scope is the drive scope: backup or resource (shares).
type Scope string

const (
	ScopeBackup   Scope = "backup"
	ScopeResource Scope = "resource"
)

// FileRef is a parsed file id that includes the scope.
type FileRef struct {
	Scope Scope
	FID   string
}

func parseRef(fileID string) FileRef {
	switch fileID {
	case ResourceRoot:
		return FileRef{Scope: ScopeResource, FID: "root"}
	case BackupRoot:
		return FileRef{Scope: ScopeBackup, FID: "root"}
	}
	if strings.HasPrefix(fileID, "r:") {
		fid := fileID[2:]
		if fid == "" {
			fid = "root"
		}
		return FileRef{Scope: ScopeResource, FID: fid}
	}
	if strings.HasPrefix(fileID, "b:") {
		fid := fileID[2:]
		if fid == "" {
			fid = "root"
		}
		return FileRef{Scope: ScopeBackup, FID: fid}
	}
	return FileRef{Scope: ScopeBackup, FID: fileID}
}

func wrapRef(scope Scope, fid string) string {
	if fid == "" || fid == "root" {
		if scope == ScopeResource {
			return ResourceRoot
		}
		return BackupRoot
	}
	prefix := "b:"
	if scope == ScopeResource {
		prefix = "r:"
	}
	return prefix + fid
}

// aliFile is a raw API file item.
type aliFile struct {
	FileID         string `json:"file_id"`
	Name           string `json:"name"`
	ParentFileID   string `json:"parent_file_id"`
	Type           string `json:"type"`
	Size           int64  `json:"size"`
	UpdatedAt      string `json:"updated_at"`
	CreatedAt      string `json:"created_at"`
	ContentHash    string `json:"content_hash"`
	ContentHashName string `json:"content_hash_name"`
	Thumbnail      string `json:"thumbnail"`
	Category       string `json:"category"`
	Status         string `json:"status"`
	DownloadURL    string `json:"download_url"`
}

// client is an authenticated aliopen session.
type client struct {
	http    *netx.Client
	session *Session
}

func clientOf(c drive.Context) (*client, error) {
	if c.Token == nil {
		return nil, drive.ErrUnauthorized
	}
	sess := parseSession(c.Token)
	if sess == nil {
		return nil, errors.New("aliopen: invalid session")
	}
	cl := &client{http: netx.NewClient(60 * time.Second), session: sess}
	if sess.AccessToken == "" {
		if err := cl.refresh(context.Background(), c.UserID); err != nil {
			return nil, err
		}
	}
	return cl, nil
}

func parseSession(tok *model.TokenInfo) *Session {
	if tok == nil {
		return nil
	}
	var sess Session
	raw := tok.RefreshToken
	if raw == "" && tok.OpenAPIRefreshToken != "" {
		raw = tok.OpenAPIRefreshToken
	}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &sess); err == nil && (sess.RefreshToken != "" || sess.AccessToken != "") {
			if sess.AccessToken == "" && tok.AccessToken != "" {
				sess.AccessToken = tok.AccessToken
			}
			return &sess
		}
	}
	if tok.AccessToken != "" {
		return &Session{AccessToken: tok.AccessToken, RefreshToken: tok.RefreshToken}
	}
	return nil
}

func (c *client) refresh(ctx context.Context, userID string) error {
	url := c.session.OAuthTokenURL
	if c.session.ClientID != "" {
		url = apiHost + "/oauth/access_token"
	}
	if url == "" {
		url = oauthDefault
	}
	body := map[string]string{"grant_type": "refresh_token", "refresh_token": c.session.RefreshToken}
	if c.session.ClientID != "" {
		body["client_id"] = c.session.ClientID
		if c.session.ClientSecret != "" {
			body["client_secret"] = c.session.ClientSecret
		}
	}
	var res struct {
		AccessToken string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.http.PostJSON(ctx, url, nil, body, &res); err != nil {
		return err
	}
	if res.AccessToken == "" {
		return errors.New("aliopen: refresh token failed")
	}
	c.session.AccessToken = res.AccessToken
	if res.RefreshToken != "" {
		c.session.RefreshToken = res.RefreshToken
	}
	// Ensure drive IDs
	if c.session.DriveID == "" {
		_ = c.ensureDrive(ctx)
	}
	return nil
}

func (c *client) ensureDrive(ctx context.Context) error {
	type driveInfo struct {
		DefaultDriveID   string `json:"default_drive_id"`
		ResourceDriveID  string `json:"resource_drive_id"`
		BackupDriveID    string `json:"backup_drive_id"`
	}
	var info driveInfo
	if err := c.apiPost(ctx, "/adrive/v1.0/user/getDriveInfo", map[string]any{}, &info); err != nil {
		return err
	}
	c.session.DriveID = info.DefaultDriveID
	c.session.ResourceDriveID = info.ResourceDriveID
	c.session.BackupDriveID = info.BackupDriveID
	return nil
}

func (c *client) scopedDriveID(scope Scope) string {
	if scope == ScopeResource && c.session.ResourceDriveID != "" {
		return c.session.ResourceDriveID
	}
	return c.session.DriveID
}

// apiPost calls the Aliyun API with auth and rate limiting.
func (c *client) apiPost(ctx context.Context, path string, body any, out any) error {
	resp, err := c.http.Do(ctx, http.MethodPost, apiHost+path,
		map[string]string{"Authorization": "Bearer " + c.session.AccessToken, "Content-Type": "application/json"},
		netx.JSONBody(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		var errBody struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(data, &errBody)
		msg := errBody.Message
		if msg == "" {
			msg = fmt.Sprintf("aliopen: http %d", resp.StatusCode)
		}
		return errors.New(msg)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

// listResp is the paginated listing response.
type listResp struct {
	Items []aliFile `json:"items"`
	Marker string   `json:"next_marker"`
}

// ListPage returns one page of a directory.
func (c *client) ListPage(ctx context.Context, scope Scope, parentID, marker string) ([]aliFile, string, error) {
	body := map[string]any{
		"parent_file_id": parentID,
		"drive_id":       c.scopedDriveID(scope),
		"limit":          200,
	}
	if marker != "" {
		body["marker"] = marker
	}
	// Also include images/playlists
	body["fields"] = "*"
	var resp listResp
	if err := c.apiPost(ctx, "/adrive/v1.0/openFile/list", body, &resp); err != nil {
		return nil, "", err
	}
	return resp.Items, resp.Marker, nil
}

// List fetches all pages.
func (c *client) List(ctx context.Context, scope Scope, parentID string) ([]aliFile, error) {
	var out []aliFile
	marker := ""
	for {
		items, next, err := c.ListPage(ctx, scope, parentID, marker)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
		if next == "" {
			break
		}
		marker = next
	}
	return out, nil
}

// Detail returns one file detail.
func (c *client) Detail(ctx context.Context, scope Scope, fileID string) (*aliFile, error) {
	var file aliFile
	if err := c.apiPost(ctx, "/adrive/v1.0/openFile/get", map[string]any{
		"file_id":  fileID,
		"drive_id": c.scopedDriveID(scope),
	}, &file); err != nil {
		return nil, err
	}
	return &file, nil
}

// Search searches across a drive.
func (c *client) Search(ctx context.Context, scope Scope, keyword string) ([]aliFile, error) {
	var resp struct {
		Items []aliFile `json:"items"`
		Marker string   `json:"next_marker"`
	}
	if err := c.apiPost(ctx, "/adrive/v1.0/openFile/search", map[string]any{
		"drive_id":  c.scopedDriveID(scope),
		"query":     "name match \"" + keyword + "\"",
		"limit":     200,
		"image_thumbnail_process": "image/resize,w_256/format,jpeg",
	}, &resp); err != nil {
		return nil, err
	}
	return resp.Items, nil
}

// DownloadInfo gets the download URL and headers.
func (c *client) DownloadInfo(ctx context.Context, scope Scope, fileID string) (string, int64, map[string]string, error) {
	var resp struct {
		URL     string            `json:"url"`
		Size    int64             `json:"size"`
		Headers map[string]string `json:"headers"`
		Expiration string        `json:"expiration"`
	}
	if err := c.apiPost(ctx, "/adrive/v1.0/openFile/getDownloadUrl", map[string]any{
		"file_id":  fileID,
		"drive_id": c.scopedDriveID(scope),
	}, &resp); err != nil {
		return "", 0, nil, err
	}
	return resp.URL, resp.Size, resp.Headers, nil
}

// Mkdir creates a folder.
func (c *client) Mkdir(ctx context.Context, scope Scope, parentID, name string) (*drive.MkdirResult, error) {
	var res struct {
		FileID string `json:"file_id"`
	}
	if err := c.apiPost(ctx, "/adrive/v1.0/openFile/create", map[string]any{
		"name":           name,
		"parent_file_id": parentID,
		"drive_id":       c.scopedDriveID(scope),
		"type":           "folder",
		"check_name_mode": "refuse",
	}, &res); err != nil {
		return &drive.MkdirResult{Error: err.Error()}, nil
	}
	return &drive.MkdirResult{FileID: res.FileID}, nil
}

// Rename patches the name.
func (c *client) Rename(ctx context.Context, scope Scope, fileID, name string) error {
	return c.apiPost(ctx, "/adrive/v1.0/openFile/update", map[string]any{
		"file_id":  fileID,
		"drive_id": c.scopedDriveID(scope),
		"name":     name,
	}, nil)
}

// Trash moves files to recycle bin.
func (c *client) Trash(ctx context.Context, scope Scope, fileIDs ...string) error {
	for _, id := range fileIDs {
		if err := c.apiPost(ctx, "/adrive/v1.0/openFile/recyclebin/trash", map[string]any{
			"file_id":  id,
			"drive_id": c.scopedDriveID(scope),
		}, nil); err != nil {
			return err
		}
	}
	return nil
}

// Delete permanently deletes.
func (c *client) Delete(ctx context.Context, scope Scope, fileIDs ...string) error {
	for _, id := range fileIDs {
		if err := c.apiPost(ctx, "/adrive/v1.0/openFile/delete", map[string]any{
			"file_id":  id,
			"drive_id": c.scopedDriveID(scope),
		}, nil); err != nil {
			return err
		}
	}
	return nil
}

// Move moves files.
func (c *client) Move(ctx context.Context, scope Scope, fileID, toParentID string) error {
	return c.apiPost(ctx, "/adrive/v1.0/openFile/move", map[string]any{
		"file_id":       fileID,
		"drive_id":      c.scopedDriveID(scope),
		"to_parent_file_id": toParentID,
	}, nil)
}

// Copy copies files.
func (c *client) Copy(ctx context.Context, scope Scope, fileID, toParentID, toDriveID string) error {
	return c.apiPost(ctx, "/adrive/v1.0/openFile/copy", map[string]any{
		"file_id":       fileID,
		"drive_id":      c.scopedDriveID(scope),
		"to_parent_file_id": toParentID,
		"to_drive_id":      toDriveID,
	}, nil)
}

// CreateShare creates a share link.
func (c *client) CreateShare(ctx context.Context, scope Scope, fileIDs []string, shareName, expiration, password string) (*model.ShareItem, error) {
	body := map[string]any{
		"drive_id":  c.scopedDriveID(scope),
		"file_id_list": fileIDs,
		"share_name":  shareName,
		"share_pwd":   password,
		"expiration":  expiration,
		"description": shareName,
	}
	var share struct {
		ShareID  string `json:"share_id"`
		ShareURL string `json:"share_url"`
		ShareMsg string `json:"share_msg"`
		Expiration string `json:"expiration"`
		Status   string `json:"status"`
		DriveID  string `json:"drive_id"`
	}
	if err := c.apiPost(ctx, "/adrive/v1.0/openFile/createShareLink", body, &share); err != nil {
		return nil, err
	}
	return &model.ShareItem{
		ShareID: share.ShareID, ShareURL: share.ShareURL, ShareMsg: share.ShareMsg,
		Expiration: share.Expiration, Status: share.Status, DriveID: share.DriveID,
		ShareName: shareName, SharePwd: password,
	}, nil
}

// RapidUpload attempts sha1-based rapid upload (秒传).
func (c *client) RapidUpload(ctx context.Context, scope Scope, parentID, name string, size int64, sha1Str string) (*drive.RapidUploadResult, error) {
	var res struct {
		FileID       string `json:"file_id"`
		RapidUpload  bool   `json:"rapid_upload"`
		PartInfoList []struct {
			PartNumber int    `json:"part_number"`
			UploadURL  string `json:"upload_url"`
		} `json:"part_info_list"`
		Exist bool `json:"exist"`
		Error string `json:"error"`
	}
	if err := c.apiPost(ctx, "/adrive/v1.0/openFile/create", map[string]any{
		"name":             name,
		"parent_file_id":   parentID,
		"drive_id":         c.scopedDriveID(scope),
		"type":             "file",
		"size":             size,
		"content_hash":     sha1Str,
		"content_hash_name": "sha1",
		"check_name_mode":  "ignore",
	}, &res); err != nil {
		return nil, err
	}
	if res.RapidUpload || res.FileID != "" {
		return &drive.RapidUploadResult{Reuse: true, FileID: res.FileID}, nil
	}
	return &drive.RapidUploadResult{Reuse: false, FileID: res.FileID, Message: "秒传未命中，需要上传"}, nil
}

// CreateUploadFile creates an upload entry and returns parts.
func (c *client) CreateUploadFile(ctx context.Context, scope Scope, parentID, name string, size int64) (string, []struct {
	PartNumber int    `json:"part_number"`
	UploadURL  string `json:"upload_url"`
}, error) {
	var res struct {
		FileID       string `json:"file_id"`
		PartInfoList []struct {
			PartNumber int    `json:"part_number"`
			UploadURL  string `json:"upload_url"`
		} `json:"part_info_list"`
	}
	if err := c.apiPost(ctx, "/adrive/v1.0/openFile/create", map[string]any{
		"name":             name,
		"parent_file_id":   parentID,
		"drive_id":         c.scopedDriveID(scope),
		"type":             "file",
		"size":             size,
		"check_name_mode":  "ignore",
	}, &res); err != nil {
		return "", nil, err
	}
	return res.FileID, res.PartInfoList, nil
}

// CompleteUpload marks the upload as complete.
func (c *client) CompleteUpload(ctx context.Context, scope Scope, fileID string) error {
	return c.apiPost(ctx, "/adrive/v1.0/openFile/complete", map[string]any{
		"file_id":  fileID,
		"drive_id": c.scopedDriveID(scope),
		"upload_id": "",
	}, nil)
}

// ResolveHash extracts the sha1 from a file's metadata.
func (c *client) ResolveHash(ctx context.Context, scope Scope, fileID string) string {
	file, err := c.Detail(ctx, scope, fileID)
	if err != nil {
		return ""
	}
	if file.ContentHashName == "sha1" && file.ContentHash != "" {
		return file.ContentHash
	}
	return ""
}

// GetSpaceInfo returns quota.
func (c *client) GetSpaceInfo(ctx context.Context) (used, total int64) {
	var resp struct {
		PersonalSpaceInfo struct {
			UsedSize  string `json:"used_size"`
			TotalSize string `json:"total_size"`
		} `json:"personal_space_info"`
	}
	if err := c.apiPost(ctx, "/adrive/v1.0/user/getSpaceInfo", map[string]any{}, &resp); err != nil {
		return 0, 0
	}
	fmt.Sscanf(resp.PersonalSpaceInfo.UsedSize, "%d", &used)
	fmt.Sscanf(resp.PersonalSpaceInfo.TotalSize, "%d", &total)
	return
}

// mapFile converts an API file to the unified model.
func mapFile(item *aliFile, driveID, parentID, scopePrefix string) model.File {
	isDir := item.Type == "folder"
	timeUnix := int64(0)
	if parsed, err := time.Parse(time.RFC3339, item.UpdatedAt); err == nil {
		timeUnix = parsed.Unix()
	}
	if timeUnix == 0 {
		if parsed, err := time.Parse(time.RFC3339, item.CreatedAt); err == nil {
			timeUnix = parsed.Unix()
		}
	}
	f := driveutil.NewFile(driveID, wrapRef(Scope(scopePrefix), item.FileID), wrapRef(Scope(scopePrefix), parentID), item.Name, isDir, item.Size, timeUnix)
	f.Thumbnail = item.Thumbnail
	f.ContentHash = item.ContentHash
	f.ContentHashName = item.ContentHashName
	f.Category = item.Category
	f.Description = item.Status
	return f
}

// ---- Driver ----

// Driver implements drive.Driver for Aliyun Drive Open.
type Driver struct {
	drive.BaseDriver
}

func (d *Driver) ID() string                       { return providerID }
func (d *Driver) Meta() drive.Meta                 { return drive.GetMeta(providerID) }
func (d *Driver) Capabilities() drive.Capabilities { return drive.RegistryCaps(providerID) }
func (d *Driver) RootID() string                   { return RootID }

func (d *Driver) List(ctx context.Context, c drive.Context, dirID string, _ *drive.ListOptions) ([]model.File, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	ref := parseRef(dirID)
	// Root: show virtual drive dirs
	if dirID == RootID || dirID == "root" {
		dirs := []model.File{virtualFile(c.DriveID, ScopeBackup)}
		// check if resource drive exists
		hasResource := cl.session.ResourceDriveID != ""
		if hasResource {
			dirs = append(dirs, virtualFile(c.DriveID, ScopeResource))
		}
		return dirs, nil
	}
	items, err := cl.List(ctx, ref.Scope, ref.FID)
	if err != nil {
		return nil, err
	}
	return mapFiles(items, c.DriveID, dirID, ref.Scope), nil
}

func (d *Driver) ListPaged(ctx context.Context, c drive.Context, dirID, marker string, _ *drive.ListOptions) (*drive.DirPage, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	ref := parseRef(dirID)
	if dirID == RootID || dirID == "root" {
		dirs := []model.File{virtualFile(c.DriveID, ScopeBackup)}
		if cl.session.ResourceDriveID != "" {
			dirs = append(dirs, virtualFile(c.DriveID, ScopeResource))
		}
		return &drive.DirPage{Items: dirs}, nil
	}
	items, next, err := cl.ListPage(ctx, ref.Scope, ref.FID, marker)
	if err != nil {
		return nil, err
	}
	return &drive.DirPage{Items: mapFiles(items, c.DriveID, dirID, ref.Scope), NextMarker: next}, nil
}

func (d *Driver) Search(ctx context.Context, c drive.Context, keyword string) ([]model.File, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	scopes := []Scope{ScopeBackup}
	if cl.session.ResourceDriveID != "" {
		scopes = append(scopes, ScopeResource)
	}
	var out []model.File
	for _, sc := range scopes {
		items, err := cl.Search(ctx, sc, keyword)
		if err != nil {
			continue
		}
		for i := range items {
			out = append(out, mapFile(&items[i], c.DriveID, wrapRef(sc, items[i].ParentFileID), string(sc)))
		}
	}
	return out, nil
}

func (d *Driver) GetInfo(ctx context.Context, c drive.Context, fileID string) (any, error) {
	ref := parseRef(fileID)
	if fileID == RootID || fileID == "root" {
		return model.File{DriveID: c.DriveID, FileID: RootID, Name: "阿里云盘", NameSearch: "aliyun", IsDir: true, Icon: "iconfile-folder"}, nil
	}
	if fileID == BackupRoot || fileID == ResourceRoot {
		return virtualFile(c.DriveID, ref.Scope), nil
	}
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	item, err := cl.Detail(ctx, ref.Scope, ref.FID)
	if err != nil {
		return nil, err
	}
	return mapFile(item, c.DriveID, wrapRef(ref.Scope, item.ParentFileID), string(ref.Scope)), nil
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
	ref := parseRef(fileID)
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	url, size, headers, err := cl.DownloadInfo(ctx, ref.Scope, ref.FID)
	if err != nil {
		return nil, err
	}
	return &model.DownloadURL{
		DriveID: c.DriveID, FileID: fileID, URL: url, Size: size,
		Headers: headers, DownloadMode: "redirect",
	}, nil
}

func (d *Driver) GetVideoPreview(ctx context.Context, c drive.Context, fileID string) (*model.VideoPreview, error) {
	u, err := d.GetDownloadURL(ctx, c, fileID, 0)
	if err != nil {
		return nil, err
	}
	return &model.VideoPreview{
		DriveID: c.DriveID, FileID: fileID, Size: u.Size, Headers: u.Headers,
		Qualities: []model.VideoQuality{{
			Quality: "origin", Label: "原画", Value: "origin", URL: u.URL, Headers: u.Headers, ForceProxy: true,
		}},
	}, nil
}

func (d *Driver) Mkdir(ctx context.Context, c drive.Context, parentID, name string) (*drive.MkdirResult, error) {
	ref := parseRef(parentID)
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	return cl.Mkdir(ctx, ref.Scope, ref.FID, name)
}

func (d *Driver) Rename(ctx context.Context, c drive.Context, fileID, name string) (*drive.RenameResult, error) {
	ref := parseRef(fileID)
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	if err := cl.Rename(ctx, ref.Scope, ref.FID, name); err != nil {
		return nil, err
	}
	return &drive.RenameResult{FileID: fileID, Name: name}, nil
}

func (d *Driver) Trash(ctx context.Context, c drive.Context, fileIDs []string) ([]string, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	groups := groupByScope(fileIDs)
	var ok []string
	for sc, ids := range groups {
		for _, id := range ids {
			if err := cl.Trash(ctx, sc, id); err == nil {
				ok = append(ok, wrapRef(sc, id))
			}
		}
	}
	return ok, nil
}

func (d *Driver) Delete(ctx context.Context, c drive.Context, refs []drive.FileRef) ([]string, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	groups := make(map[Scope][]string)
	for _, r := range refs {
		ref := parseRef(r.ID)
		groups[ref.Scope] = append(groups[ref.Scope], ref.FID)
	}
	var ok []string
	for sc, ids := range groups {
		for _, id := range ids {
			if err := cl.Delete(ctx, sc, id); err == nil {
				ok = append(ok, wrapRef(sc, id))
			}
		}
	}
	return ok, nil
}

func (d *Driver) Restore(ctx context.Context, c drive.Context, fileIDs []string) ([]string, error) {
	return nil, drive.NotSupported("restore")
}

func (d *Driver) Move(ctx context.Context, c drive.Context, refs []drive.FileRef, toParentID, _ string) ([]string, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	target := parseRef(toParentID)
	var ok []string
	for _, r := range refs {
		ref := parseRef(r.ID)
		if ref.Scope != target.Scope {
			continue
		}
		if err := cl.Move(ctx, ref.Scope, ref.FID, target.FID); err == nil {
			ok = append(ok, r.ID)
		}
	}
	return ok, nil
}

func (d *Driver) Copy(ctx context.Context, c drive.Context, refs []drive.FileRef, toParentID, _ string) ([]string, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	target := parseRef(toParentID)
	var ok []string
	for _, r := range refs {
		ref := parseRef(r.ID)
		toDrive := cl.scopedDriveID(target.Scope)
		if err := cl.Copy(ctx, ref.Scope, ref.FID, target.FID, toDrive); err == nil {
			ok = append(ok, r.ID)
		}
	}
	return ok, nil
}

func (d *Driver) CreateShare(ctx context.Context, c drive.Context, params drive.ShareParams) (*model.ShareItem, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	fids := make([]string, 0, len(params.FileIDs))
	for _, id := range params.FileIDs {
		ref := parseRef(id)
		fids = append(fids, ref.FID)
	}
	scope := ScopeBackup
	if len(fids) > 0 {
		ref := parseRef(params.FileIDs[0])
		scope = ref.Scope
	}
	return cl.CreateShare(ctx, scope, fids, params.ShareName, params.Expiration, params.Password)
}

func (d *Driver) UploadOneFile(ctx context.Context, c drive.Context, ui *model.UploadingUI) error {
	ref := parseRef(ui.Info.ParentFileID)
	cl, err := clientOf(c)
	if err != nil {
		return err
	}
	f, err := os.Open(ui.Info.LocalFilePath)
	if err != nil {
		return err
	}
	defer f.Close()
	info, _ := f.Stat()
	if info == nil {
		return errors.New("aliopen: stat file failed")
	}
	size := info.Size()
	// Try rapid upload by sha1
	if ui.Info.SHA1 != "" {
		res, err := cl.RapidUpload(ctx, ref.Scope, ref.FID, ui.Info.Name, size, ui.Info.SHA1)
		if err == nil && res.Reuse {
			return nil
		}
	}
	// Create upload file
	fileID, parts, err := cl.CreateUploadFile(ctx, ref.Scope, ref.FID, ui.Info.Name, size)
	if err != nil {
		return err
	}
	// Upload parts
	buf := make([]byte, partSize)
	var pos int64
	if len(parts) == 0 {
		parts = append(parts, struct {
			PartNumber int    `json:"part_number"`
			UploadURL  string `json:"upload_url"`
		}{PartNumber: 1, UploadURL: ""})
	}
	for _, part := range parts {
		n, err := f.ReadAt(buf, pos)
		if err != nil && err != io.EOF {
			return err
		}
		chunk := buf[:n]
		if part.UploadURL != "" {
			req, _ := http.NewRequestWithContext(ctx, http.MethodPut, part.UploadURL, strings.NewReader(string(chunk)))
			resp, err2 := cl.http.HTTP.Do(req)
			if err2 != nil {
				return err2
			}
			resp.Body.Close()
		}
		pos += int64(n)
		if ui != nil {
			ui.Upload.DownSize = pos
			ui.Upload.DownProcess = int(pos * 100 / size)
		}
	}
	return cl.CompleteUpload(ctx, ref.Scope, fileID)
}

func (d *Driver) RapidUploadByHash(ctx context.Context, c drive.Context, req drive.RapidUploadRequest) (*drive.RapidUploadResult, error) {
	if req.Method != "sha1" {
		return nil, errors.New("aliopen: only sha1 supported")
	}
	ref := parseRef(req.ParentID)
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	return cl.RapidUpload(ctx, ref.Scope, ref.FID, req.FileName, req.Size, req.Hash)
}

func (d *Driver) ResolveTransferHash(ctx context.Context, c drive.Context, fileID, method string, _ bool) (string, error) {
	if method != "sha1" {
		return "", nil
	}
	ref := parseRef(fileID)
	cl, err := clientOf(c)
	if err != nil {
		return "", err
	}
	return cl.ResolveHash(ctx, ref.Scope, ref.FID), nil
}

func (d *Driver) RefreshAccount(ctx context.Context, c drive.Context, token *model.TokenInfo) (*model.TokenInfo, error) {
	sess := parseSession(token)
	if sess == nil {
		return nil, nil
	}
	cl := &client{http: netx.NewClient(60 * time.Second), session: sess}
	if err := cl.refresh(ctx, c.UserID); err != nil {
		return nil, err
	}
	// persist the updated session
	token.AccessToken = sess.AccessToken
	token.RefreshToken = mustJSON(sess)
	token.OpenAPIAccessToken = sess.AccessToken
	token.OpenAPIRefreshToken = sess.RefreshToken
	used, total := cl.GetSpaceInfo(ctx)
	token.UsedSize = used
	token.TotalSize = total
	return token, nil
}

// helpers

func virtualFile(driveID string, scope Scope) model.File {
	if scope == ScopeResource {
		return model.File{DriveID: driveID, FileID: ResourceRoot, ParentFileID: RootID, Name: "资源盘", NameSearch: "资源盘 ziyuanpan", IsDir: true, Icon: "iconfile-folder", Description: "保存分享的文件在这里"}
	}
	return model.File{DriveID: driveID, FileID: BackupRoot, ParentFileID: RootID, Name: "备份盘", NameSearch: "备份盘 beifenpan", IsDir: true, Icon: "iconfile-folder", Description: "备份盘"}
}

func mapFiles(items []aliFile, driveID, parentID string, scope Scope) []model.File {
	out := make([]model.File, 0, len(items))
	for i := range items {
		out = append(out, mapFile(&items[i], driveID, parentID, string(scope)))
	}
	return out
}

func groupByScope(fileIDs []string) map[Scope][]string {
	m := map[Scope][]string{}
	for _, id := range fileIDs {
		ref := parseRef(id)
		m[ref.Scope] = append(m[ref.Scope], ref.FID)
	}
	return m
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// authRefreshToken handles aliyun open login with a refresh_token.
func authRefreshToken(ctx context.Context, req drive.AuthRequest) (*model.TokenInfo, error) {
	refreshToken := strings.TrimSpace(req.Config["refresh_token"])
	if refreshToken == "" {
		return nil, errors.New("aliopen: 请输入 refresh_token")
	}
	clientID := strings.TrimSpace(req.Config["client_id"])
	clientSecret := strings.TrimSpace(req.Config["client_secret"])

	sess := &Session{
		RefreshToken: refreshToken,
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}
	cl := &client{http: netx.NewClient(60 * time.Second), session: sess}
	if err := cl.refresh(ctx, ""); err != nil {
		return nil, err
	}
	if err := cl.ensureDrive(ctx); err != nil {
		return nil, err
	}
	uid := sess.DriveID
	if uid == "" {
		uid = "ali"
	}
	name := "阿里云盘 " + uid[:min(8, len(uid))]
	used, total := cl.GetSpaceInfo(ctx)

	tok := &model.TokenInfo{
		TokenFrom:           providerID,
		AccessToken:         sess.AccessToken,
		RefreshToken:        mustJSON(sess),
		OpenAPIAccessToken:  sess.AccessToken,
		OpenAPIRefreshToken: sess.RefreshToken,
		TokenType:           "Bearer",
		UserID:              model.BuildUserID(providerID, uid),
		UserName:            name,
		NickName:            name,
		Name:                name,
		DefaultDriveID:      model.BuildDriveID(providerID, uid),
		ProviderAccountID:   uid,
		ProviderRootID:      "root",
		UsedSize:            used,
		TotalSize:           total,
		FreeSize:            total - used,
		DeviceID:            "mnemo",
	}
	return tok, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var _ = hex.EncodeToString
var _ = sha1.New
var _ = json.NewDecoder