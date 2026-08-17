// Package aliopen implements the Aliyun Drive Open API provider (AList-sourced).
package aliopen

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
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

	defaultDownloadExpireSec = 14400
)

const providerID = model.ProviderAliopen

// aliOpenRateLimiter mirrors the legacy provider limiter: at most two
// in-flight API calls and a small spacing between requests to reduce
// wind-control responses from the Open API.
type aliOpenRateLimiter struct {
	mu           sync.Mutex
	last         time.Time
	blockedUntil time.Time
	slots        chan struct{}
	interval     time.Duration
}

func newAliOpenRateLimiter(concurrency int, interval time.Duration) *aliOpenRateLimiter {
	if concurrency < 1 {
		concurrency = 1
	}
	return &aliOpenRateLimiter{
		slots:    make(chan struct{}, concurrency),
		interval: interval,
	}
}

func (r *aliOpenRateLimiter) run(ctx context.Context, fn func() error) error {
	select {
	case r.slots <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-r.slots }()

	for {
		r.mu.Lock()
		now := time.Now()
		wait := r.interval - now.Sub(r.last)
		if blockedWait := r.blockedUntil.Sub(now); blockedWait > wait {
			wait = blockedWait
		}
		if wait <= 0 {
			r.last = now
			r.mu.Unlock()
			return fn()
		}
		r.mu.Unlock()

		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		}
	}
}

func (r *aliOpenRateLimiter) penalize(delay time.Duration) {
	if delay <= 0 {
		return
	}
	r.mu.Lock()
	until := time.Now().Add(delay)
	if until.After(r.blockedUntil) {
		r.blockedUntil = until
	}
	r.mu.Unlock()
}

var aliOpenLimiter = newAliOpenRateLimiter(2, 220*time.Millisecond)

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
	FileID          string `json:"file_id"`
	Name            string `json:"name"`
	ParentFileID    string `json:"parent_file_id"`
	Type            string `json:"type"`
	Size            int64  `json:"size"`
	UpdatedAt       string `json:"updated_at"`
	CreatedAt       string `json:"created_at"`
	ContentHash     string `json:"content_hash"`
	ContentHashName string `json:"content_hash_name"`
	Thumbnail       string `json:"thumbnail"`
	Category        string `json:"category"`
	Status          string `json:"status"`
	DownloadURL     string `json:"download_url"`
}

// client is an authenticated aliopen session.
type client struct {
	http    *netx.Client
	session *Session
	token   *model.TokenInfo
}

func clientOf(c drive.Context) (*client, error) {
	if c.Token == nil {
		return nil, drive.ErrUnauthorized
	}
	sess := parseSession(c.Token)
	if sess == nil {
		return nil, errors.New("aliopen: invalid session")
	}
	cl := &client{http: netx.NewClient(60 * time.Second), session: sess, token: c.Token}
	if sess.AccessToken == "" {
		if err := cl.refresh(context.Background(), c.UserID); err != nil {
			return nil, err
		}
	}
	if sess.DriveID == "" {
		if err := cl.ensureDrive(context.Background()); err != nil {
			return nil, err
		}
	}
	cl.persistSession()
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
			if sess.RefreshToken == "" && tok.OpenAPIRefreshToken != "" {
				sess.RefreshToken = tok.OpenAPIRefreshToken
			}
			return &sess
		}
		// Older builds persisted the refresh token itself instead of the JSON
		// session. Keep those accounts refreshable after migration.
		if len(strings.TrimSpace(raw)) > 20 {
			return &Session{AccessToken: tok.AccessToken, RefreshToken: strings.TrimSpace(raw)}
		}
	}
	if tok.AccessToken != "" {
		return &Session{AccessToken: tok.AccessToken, RefreshToken: tok.RefreshToken}
	}
	return nil
}

func (c *client) persistSession() {
	if c == nil || c.token == nil || c.session == nil {
		return
	}
	c.token.AccessToken = c.session.AccessToken
	c.token.RefreshToken = mustJSON(c.session)
	c.token.OpenAPIAccessToken = c.session.AccessToken
	c.token.OpenAPIRefreshToken = c.session.RefreshToken
}

func (c *client) refresh(ctx context.Context, _ string) error {
	if err := c.refreshToken(ctx); err != nil {
		return err
	}
	if c.session.DriveID == "" {
		if err := c.ensureDrive(ctx); err != nil {
			return err
		}
	}
	c.persistSession()
	return nil
}

func (c *client) refreshToken(ctx context.Context) error {
	if strings.TrimSpace(c.session.RefreshToken) == "" {
		return errors.New("aliopen: refresh_token 缺失")
	}
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
	var resp *http.Response
	if err := aliOpenLimiter.run(ctx, func() error {
		var err error
		resp, err = c.http.Do(ctx, http.MethodPost, url, map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		}, netx.JSONBody(body))
		return err
	}); err != nil {
		return err
	}
	data, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if readErr != nil {
		return readErr
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		aliOpenLimiter.penalize(aliOpenRetryAfter(resp, 8*time.Second))
	}
	if resp.StatusCode >= 400 {
		return errors.New(aliOpenErrorMessage(data, resp.StatusCode))
	}
	var res struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return fmt.Errorf("aliopen: refresh response: %w", err)
	}
	if res.AccessToken == "" {
		return errors.New(aliOpenErrorMessage(data, resp.StatusCode))
	}
	c.session.AccessToken = res.AccessToken
	if res.RefreshToken != "" {
		c.session.RefreshToken = res.RefreshToken
	}
	c.persistSession()
	return nil
}

func (c *client) ensureDrive(ctx context.Context) error {
	type driveInfo struct {
		DefaultDriveID  string `json:"default_drive_id"`
		ResourceDriveID string `json:"resource_drive_id"`
		BackupDriveID   string `json:"backup_drive_id"`
	}
	var info driveInfo
	if err := c.apiPost(ctx, "/adrive/v1.0/user/getDriveInfo", map[string]any{}, &info); err != nil {
		return err
	}
	driveID := strings.TrimSpace(info.DefaultDriveID)
	if driveID == "" {
		driveID = strings.TrimSpace(info.ResourceDriveID)
	}
	if driveID == "" {
		driveID = strings.TrimSpace(info.BackupDriveID)
	}
	if driveID == "" {
		return errors.New("aliopen: 获取 drive_id 失败")
	}
	c.session.DriveID = driveID
	c.session.ResourceDriveID = strings.TrimSpace(info.ResourceDriveID)
	c.session.BackupDriveID = strings.TrimSpace(info.BackupDriveID)
	c.persistSession()
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
	return c.apiPostWith(ctx, path, body, out, nil)
}

// apiPostWith is apiPost with extra headers (e.g. x-share-token).
func (c *client) apiPostWith(ctx context.Context, path string, body any, out any, extraHeaders map[string]string) error {
	return c.apiPostWithRetry(ctx, path, body, out, extraHeaders, true)
}

func (c *client) apiPostWithRetry(ctx context.Context, path string, body any, out any, extraHeaders map[string]string, allowRefresh bool) error {
	hdrs := map[string]string{
		"Authorization": "Bearer " + c.session.AccessToken,
		"Content-Type":  "application/json",
		"Accept":        "application/json",
	}
	for k, v := range extraHeaders {
		switch strings.ToLower(k) {
		case "authorization", "content-type", "accept":
			continue
		}
		hdrs[k] = v
	}
	var resp *http.Response
	if err := aliOpenLimiter.run(ctx, func() error {
		var err error
		resp, err = c.http.Do(ctx, http.MethodPost, apiHost+path, hdrs, netx.JSONBody(body))
		return err
	}); err != nil {
		return err
	}
	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return err
	}
	if aliOpenAuthResponse(resp.StatusCode, data) {
		if !allowRefresh {
			return errors.New(aliOpenErrorMessage(data, resp.StatusCode))
		}
		if err := c.refreshToken(ctx); err != nil {
			return err
		}
		return c.apiPostWithRetry(ctx, path, body, out, extraHeaders, false)
	}
	if aliOpenRateLimitResponse(resp.StatusCode, data) {
		fallback := 5 * time.Second
		if resp.StatusCode == http.StatusTooManyRequests {
			fallback = 5 * time.Second
		}
		aliOpenLimiter.penalize(aliOpenRetryAfter(resp, fallback))
	}
	if resp.StatusCode >= 400 {
		return errors.New(aliOpenErrorMessage(data, resp.StatusCode))
	}
	if aliOpenAPIError(data) {
		return errors.New(aliOpenErrorMessage(data, resp.StatusCode))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

type aliOpenErrorBody struct {
	Code             string `json:"code"`
	Message          string `json:"message"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

func aliOpenErrorBodyOf(data []byte) aliOpenErrorBody {
	var body aliOpenErrorBody
	_ = json.Unmarshal(data, &body)
	return body
}

func aliOpenAuthResponse(status int, data []byte) bool {
	if status == http.StatusUnauthorized {
		return true
	}
	body := aliOpenErrorBodyOf(data)
	code := strings.ToLower(strings.TrimSpace(body.Code))
	return strings.Contains(code, "accesstoken") ||
		strings.Contains(code, "tokenexpired") ||
		strings.Contains(code, "invalid_token") ||
		code == "unauthorized"
}

func aliOpenRateLimitResponse(status int, data []byte) bool {
	if status == http.StatusTooManyRequests {
		return true
	}
	body := aliOpenErrorBodyOf(data)
	text := strings.ToLower(body.Code + " " + body.Message + " " + body.Error)
	return strings.Contains(text, "limit") ||
		strings.Contains(text, "toomany") ||
		strings.Contains(text, "frequency") ||
		strings.Contains(text, "429")
}

func aliOpenAPIError(data []byte) bool {
	body := aliOpenErrorBodyOf(data)
	return strings.TrimSpace(body.Code) != "" && !strings.EqualFold(strings.TrimSpace(body.Code), "success")
}

func aliOpenErrorMessage(data []byte, status int) string {
	body := aliOpenErrorBodyOf(data)
	for _, msg := range []string{body.Message, body.ErrorDescription, body.Error, body.Code} {
		if strings.TrimSpace(msg) != "" {
			return strings.TrimSpace(msg)
		}
	}
	if status > 0 {
		return fmt.Sprintf("aliopen: http %d", status)
	}
	return "aliopen: request failed"
}

func aliOpenRetryAfter(resp *http.Response, fallback time.Duration) time.Duration {
	if resp != nil {
		if raw := strings.TrimSpace(resp.Header.Get("Retry-After")); raw != "" {
			if seconds, err := strconv.Atoi(raw); err == nil && seconds >= 0 {
				return time.Duration(seconds) * time.Second
			}
		}
	}
	return fallback
}

// listResp is the paginated listing response.
type listResp struct {
	Items  []aliFile `json:"items"`
	Marker string    `json:"next_marker"`
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
	seenMarkers := map[string]bool{}
	for {
		items, next, err := c.ListPage(ctx, scope, parentID, marker)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
		if next == "" {
			break
		}
		if seenMarkers[next] {
			return nil, errors.New("aliopen: duplicate list cursor")
		}
		seenMarkers[next] = true
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
		Items  []aliFile `json:"items"`
		Marker string    `json:"next_marker"`
	}
	if err := c.apiPost(ctx, "/adrive/v1.0/openFile/search", map[string]any{
		"drive_id":                c.scopedDriveID(scope),
		"query":                   "name match \"" + keyword + "\"",
		"limit":                   200,
		"image_thumbnail_process": "image/resize,w_256/format,jpeg",
	}, &resp); err != nil {
		return nil, err
	}
	return resp.Items, nil
}

// DownloadInfo gets the download URL and headers.
func (c *client) DownloadInfo(ctx context.Context, scope Scope, fileID string, expireSec int) (string, int64, map[string]string, int64, error) {
	var resp struct {
		URL        string            `json:"url"`
		Size       int64             `json:"size"`
		Headers    map[string]string `json:"headers"`
		Expiration string            `json:"expiration"`
	}
	if expireSec <= 0 {
		expireSec = defaultDownloadExpireSec
	}
	if err := c.apiPost(ctx, "/adrive/v1.0/openFile/getDownloadUrl", map[string]any{
		"file_id":    fileID,
		"drive_id":   c.scopedDriveID(scope),
		"expire_sec": expireSec,
	}, &resp); err != nil {
		return "", 0, nil, 0, err
	}
	if strings.TrimSpace(resp.URL) == "" {
		return "", 0, nil, 0, errors.New("aliopen: 获取下载地址失败")
	}
	expireTime := parseAliOpenExpiration(resp.Expiration)
	if expireTime == 0 {
		expireTime = driveutil.GetExpiresTime(resp.URL)
	}
	return resp.URL, resp.Size, resp.Headers, expireTime, nil
}

// Mkdir creates a folder.
func (c *client) Mkdir(ctx context.Context, scope Scope, parentID, name string) (*drive.MkdirResult, error) {
	var res struct {
		FileID string `json:"file_id"`
	}
	if err := c.apiPost(ctx, "/adrive/v1.0/openFile/create", map[string]any{
		"name":            name,
		"parent_file_id":  parentID,
		"drive_id":        c.scopedDriveID(scope),
		"type":            "folder",
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
		"file_id":           fileID,
		"drive_id":          c.scopedDriveID(scope),
		"to_parent_file_id": toParentID,
	}, nil)
}

// Copy copies files.
func (c *client) Copy(ctx context.Context, scope Scope, fileID, toParentID, toDriveID string) error {
	return c.apiPost(ctx, "/adrive/v1.0/openFile/copy", map[string]any{
		"file_id":           fileID,
		"drive_id":          c.scopedDriveID(scope),
		"to_parent_file_id": toParentID,
		"to_drive_id":       toDriveID,
	}, nil)
}

// CreateShare creates a share link.
func (c *client) CreateShare(ctx context.Context, scope Scope, fileIDs []string, shareName, expiration, password string) (*model.ShareItem, error) {
	body := map[string]any{
		"drive_id":     c.scopedDriveID(scope),
		"file_id_list": fileIDs,
		"share_name":   shareName,
		"share_pwd":    password,
		"expiration":   expiration,
		"description":  shareName,
	}
	var share struct {
		ShareID    string `json:"share_id"`
		ShareURL   string `json:"share_url"`
		ShareMsg   string `json:"share_msg"`
		Expiration string `json:"expiration"`
		Status     string `json:"status"`
		DriveID    string `json:"drive_id"`
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
		Exist bool   `json:"exist"`
		Error string `json:"error"`
	}
	if err := c.apiPost(ctx, "/adrive/v1.0/openFile/create", map[string]any{
		"name":              name,
		"parent_file_id":    parentID,
		"drive_id":          c.scopedDriveID(scope),
		"type":              "file",
		"size":              size,
		"content_hash":      sha1Str,
		"content_hash_name": "sha1",
		"check_name_mode":   "ignore",
	}, &res); err != nil {
		return nil, err
	}
	// A miss still returns file_id for the pending upload object. Only the
	// explicit rapid_upload flag means the remote file is already complete.
	if res.RapidUpload {
		if strings.TrimSpace(res.FileID) == "" {
			return &drive.RapidUploadResult{Reuse: false, Message: "秒传响应缺少 file_id"}, nil
		}
		return &drive.RapidUploadResult{Reuse: true, FileID: res.FileID}, nil
	}
	// Do not expose the pending file id: the migration layer treats a file id
	// as an accepted transfer for compatibility with providers that return one
	// on a successful probe.
	return &drive.RapidUploadResult{Reuse: false, Message: "秒传未命中，需要上传"}, nil
}

// CreateUploadFile creates an upload entry and returns parts.
func (c *client) CreateUploadFile(ctx context.Context, scope Scope, parentID, name string, size int64) (string, string, []struct {
	PartNumber int    `json:"part_number"`
	UploadURL  string `json:"upload_url"`
}, error) {
	partCount := int(size / int64(partSize))
	if size%int64(partSize) != 0 || partCount == 0 {
		partCount++
	}
	partInfoList := make([]map[string]int, partCount)
	for i := range partInfoList {
		partInfoList[i] = map[string]int{"part_number": i + 1}
	}
	var res struct {
		FileID       string `json:"file_id"`
		UploadID     string `json:"upload_id"`
		PartInfoList []struct {
			PartNumber int    `json:"part_number"`
			UploadURL  string `json:"upload_url"`
		} `json:"part_info_list"`
	}
	if err := c.apiPost(ctx, "/adrive/v1.0/openFile/create", map[string]any{
		"name":            name,
		"parent_file_id":  parentID,
		"drive_id":        c.scopedDriveID(scope),
		"type":            "file",
		"size":            size,
		"check_name_mode": "ignore",
		"part_info_list":  partInfoList,
	}, &res); err != nil {
		return "", "", nil, err
	}
	return res.FileID, res.UploadID, res.PartInfoList, nil
}

// CompleteUpload marks the upload as complete.
func (c *client) CompleteUpload(ctx context.Context, scope Scope, fileID, uploadID string) error {
	return c.apiPost(ctx, "/adrive/v1.0/openFile/complete", map[string]any{
		"file_id":   fileID,
		"drive_id":  c.scopedDriveID(scope),
		"upload_id": uploadID,
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
	var searchErrs []error
	for _, sc := range scopes {
		items, err := cl.Search(ctx, sc, keyword)
		if err != nil {
			searchErrs = append(searchErrs, fmt.Errorf("%s: %w", sc, err))
			continue
		}
		for i := range items {
			out = append(out, mapFile(&items[i], c.DriveID, wrapRef(sc, items[i].ParentFileID), string(sc)))
		}
	}
	if len(out) == 0 && len(searchErrs) > 0 {
		return nil, errors.Join(searchErrs...)
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

func (d *Driver) GetDownloadURL(ctx context.Context, c drive.Context, fileID string, expireSec int) (*model.DownloadURL, error) {
	ref := parseRef(fileID)
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	url, size, headers, expireTime, err := cl.DownloadInfo(ctx, ref.Scope, ref.FID, expireSec)
	if err != nil {
		return nil, err
	}
	return &model.DownloadURL{
		DriveID: c.DriveID, FileID: fileID, URL: url, Size: size,
		ExpireTime: expireTime,
		Headers:    headers, DownloadMode: "redirect",
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

func parseAliOpenExpiration(raw string) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if value, err := strconv.ParseInt(raw, 10, 64); err == nil {
		if value > 0 && value < 10_000_000_000 {
			return value * 1000
		}
		if value >= 10_000_000_000 {
			return value
		}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.UnixMilli()
		}
	}
	return 0
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
	scope, err := sameAliOpenScopeIDs(fileIDs...)
	if err != nil {
		return nil, err
	}
	var ok []string
	var failed []error
	for _, id := range fileIDs {
		ref := parseRef(id)
		if err := cl.Trash(ctx, scope, ref.FID); err == nil {
			ok = append(ok, wrapRef(scope, ref.FID))
		} else {
			failed = append(failed, fmt.Errorf("%s: %w", wrapRef(scope, ref.FID), err))
		}
	}
	return ok, errors.Join(failed...)
}

func (d *Driver) Delete(ctx context.Context, c drive.Context, refs []drive.FileRef) ([]string, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	scope, err := sameAliOpenRefScope(refs, "")
	if err != nil {
		return nil, err
	}
	var ok []string
	var failed []error
	for _, r := range refs {
		ref := parseRef(r.ID)
		if err := cl.Delete(ctx, scope, ref.FID); err == nil {
			ok = append(ok, wrapRef(scope, ref.FID))
		} else {
			failed = append(failed, fmt.Errorf("%s: %w", wrapRef(scope, ref.FID), err))
		}
	}
	return ok, errors.Join(failed...)
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
	scope, err := sameAliOpenRefScope(refs, toParentID)
	if err != nil {
		return nil, err
	}
	var ok []string
	var failed []error
	for _, r := range refs {
		ref := parseRef(r.ID)
		if err := cl.Move(ctx, scope, ref.FID, target.FID); err == nil {
			ok = append(ok, r.ID)
		} else {
			failed = append(failed, fmt.Errorf("%s: %w", r.ID, err))
		}
	}
	return ok, errors.Join(failed...)
}

func (d *Driver) Copy(ctx context.Context, c drive.Context, refs []drive.FileRef, toParentID, _ string) ([]string, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	target := parseRef(toParentID)
	scope, err := sameAliOpenRefScope(refs, toParentID)
	if err != nil {
		return nil, err
	}
	var ok []string
	var failed []error
	for _, r := range refs {
		ref := parseRef(r.ID)
		toDrive := cl.scopedDriveID(scope)
		if err := cl.Copy(ctx, scope, ref.FID, target.FID, toDrive); err == nil {
			ok = append(ok, r.ID)
		} else {
			failed = append(failed, fmt.Errorf("%s: %w", r.ID, err))
		}
	}
	return ok, errors.Join(failed...)
}

func (d *Driver) CreateShare(ctx context.Context, c drive.Context, params drive.ShareParams) (*model.ShareItem, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	scope, err := sameAliOpenScopeIDs(params.FileIDs...)
	if err != nil {
		return nil, err
	}
	fids := make([]string, 0, len(params.FileIDs))
	for _, id := range params.FileIDs {
		ref := parseRef(id)
		fids = append(fids, ref.FID)
	}
	return cl.CreateShare(ctx, scope, fids, params.ShareName, params.Expiration, params.Password)
}

func (d *Driver) UploadOneFile(ctx context.Context, c drive.Context, ui *model.UploadingUI) error {
	if ui == nil || strings.TrimSpace(ui.Info.LocalFilePath) == "" {
		return errors.New("aliopen: 上传文件路径为空")
	}
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
	ui.Info.Size = size
	// Try rapid upload by sha1
	if ui.Info.SHA1 != "" {
		res, err := cl.RapidUpload(ctx, ref.Scope, ref.FID, ui.Info.Name, size, ui.Info.SHA1)
		if err == nil && res.Reuse {
			return nil
		}
	}
	// Create upload file
	fileID, uploadID, parts, err := cl.CreateUploadFile(ctx, ref.Scope, ref.FID, ui.Info.Name, size)
	if err != nil {
		return err
	}
	// Resume: load previously uploaded part numbers
	sessionKey := drive.UploadSessionKey(c.UserID, c.DriveID, ui.Info.ParentFileID, ui.Info.Name, size)
	savedSessionID, savedParts := drive.LoadUploadSessionState(sessionKey)
	uploadedSet := make(map[int]bool)
	if savedSessionID == uploadID {
		for _, pn := range savedParts {
			uploadedSet[pn] = true
		}
	}
	// Upload parts
	buf := make([]byte, partSize)
	var pos int64
	partCount := int(size / int64(partSize))
	if size%int64(partSize) != 0 || partCount == 0 {
		partCount++
	}
	if len(parts) == 0 {
		for partNumber := 1; partNumber <= partCount; partNumber++ {
			parts = append(parts, struct {
				PartNumber int    `json:"part_number"`
				UploadURL  string `json:"upload_url"`
			}{PartNumber: partNumber})
		}
	} else {
		knownParts := make(map[int]bool, len(parts))
		for _, part := range parts {
			knownParts[part.PartNumber] = true
		}
		for partNumber := 1; partNumber <= partCount; partNumber++ {
			if !knownParts[partNumber] {
				parts = append(parts, struct {
					PartNumber int    `json:"part_number"`
					UploadURL  string `json:"upload_url"`
				}{PartNumber: partNumber})
			}
		}
	}
	sort.SliceStable(parts, func(i, j int) bool { return parts[i].PartNumber < parts[j].PartNumber })
	missingParts := make([]map[string]int, 0)
	for _, part := range parts {
		if !uploadedSet[part.PartNumber] && strings.TrimSpace(part.UploadURL) == "" {
			missingParts = append(missingParts, map[string]int{"part_number": part.PartNumber})
		}
	}
	for start := 0; start < len(missingParts); start += 100 {
		end := start + 100
		if end > len(missingParts) {
			end = len(missingParts)
		}
		var refreshed struct {
			PartInfoList []struct {
				PartNumber int    `json:"part_number"`
				UploadURL  string `json:"upload_url"`
			} `json:"part_info_list"`
		}
		if err := cl.apiPost(ctx, "/adrive/v1.0/openFile/getUploadUrl", map[string]any{
			"drive_id":       cl.scopedDriveID(ref.Scope),
			"file_id":        fileID,
			"upload_id":      uploadID,
			"part_info_list": missingParts[start:end],
		}, &refreshed); err != nil {
			return err
		}
		urls := make(map[int]string, len(refreshed.PartInfoList))
		for _, part := range refreshed.PartInfoList {
			urls[part.PartNumber] = strings.TrimSpace(part.UploadURL)
		}
		for i := range parts {
			if url := urls[parts[i].PartNumber]; url != "" {
				parts[i].UploadURL = url
			}
		}
	}
	for _, part := range parts {
		if !uploadedSet[part.PartNumber] && strings.TrimSpace(part.UploadURL) == "" {
			return fmt.Errorf("aliopen: 分片 %d 无上传地址", part.PartNumber)
		}
	}
	lastURLRefresh := time.Now()
	for _, part := range parts {
		n, err := f.ReadAt(buf, pos)
		if err != nil && err != io.EOF {
			return err
		}
		chunk := buf[:n]
		if !uploadedSet[part.PartNumber] && part.UploadURL != "" {
			if time.Since(lastURLRefresh) >= 50*time.Minute {
				var refreshed struct {
					PartInfoList []struct {
						PartNumber int    `json:"part_number"`
						UploadURL  string `json:"upload_url"`
					} `json:"part_info_list"`
				}
				if refreshErr := cl.apiPost(ctx, "/adrive/v1.0/openFile/getUploadUrl", map[string]any{
					"drive_id":       cl.scopedDriveID(ref.Scope),
					"file_id":        fileID,
					"upload_id":      uploadID,
					"part_info_list": []map[string]int{{"part_number": part.PartNumber}},
				}, &refreshed); refreshErr != nil || len(refreshed.PartInfoList) == 0 || refreshed.PartInfoList[0].UploadURL == "" {
					if refreshErr != nil {
						return refreshErr
					}
					return fmt.Errorf("aliopen: 分片 %d 刷新上传地址失败", part.PartNumber)
				}
				part.UploadURL = refreshed.PartInfoList[0].UploadURL
				lastURLRefresh = time.Now()
			}
			putPart := func(uploadURL string) (int, error) {
				req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(chunk))
				if err != nil {
					return 0, err
				}
				req.Header.Set("Content-Length", strconv.FormatInt(int64(len(chunk)), 10))
				resp, err := cl.http.HTTP.Do(req)
				if err != nil {
					return 0, err
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				status := resp.StatusCode
				resp.Body.Close()
				return status, nil
			}
			status, err := putPart(part.UploadURL)
			if err != nil {
				return err
			}
			if (status == http.StatusUnauthorized || status == http.StatusForbidden) && status >= 400 {
				// A persisted session can contain expired pre-signed URLs. Refresh
				// only the current part so a recoverable expiry does not discard the
				// whole upload session.
				var refreshed struct {
					PartInfoList []struct {
						PartNumber int    `json:"part_number"`
						UploadURL  string `json:"upload_url"`
					} `json:"part_info_list"`
				}
				if refreshErr := cl.apiPost(ctx, "/adrive/v1.0/openFile/getUploadUrl", map[string]any{
					"drive_id":       cl.scopedDriveID(ref.Scope),
					"file_id":        fileID,
					"upload_id":      uploadID,
					"part_info_list": []map[string]int{{"part_number": part.PartNumber}},
				}, &refreshed); refreshErr == nil && len(refreshed.PartInfoList) > 0 && refreshed.PartInfoList[0].UploadURL != "" {
					part.UploadURL = refreshed.PartInfoList[0].UploadURL
					status, err = putPart(part.UploadURL)
					if err != nil {
						return err
					}
				}
			}
			if status < 200 || status >= 300 {
				return fmt.Errorf("aliopen: 分片上传失败 HTTP %d", status)
			}
		}
		// Persist uploaded part number incrementally
		if !uploadedSet[part.PartNumber] {
			uploadedSet[part.PartNumber] = true
			_ = drive.SaveUploadSessionState(sessionKey, uploadID, drive.SortedUniqueParts(uploadedSet))
		}
		pos += int64(n)
		if ui != nil {
			ui.Upload.DownSize = pos
			if size > 0 {
				ui.Upload.DownProcess = int(pos * 100 / size)
			} else {
				ui.Upload.DownProcess = 100
			}
		}
	}
	err = cl.CompleteUpload(ctx, ref.Scope, fileID, uploadID)
	if err == nil {
		drive.ClearUploadSession(sessionKey)
	}
	return err
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

// ---- Share Import (importShare capability) ----

// shareListResp is the aliopen listByShare response.
type shareListResp struct {
	Items      []aliFile `json:"items"`
	NextMarker string    `json:"next_marker"`
}

// ImportShareSession implements drive.ShareImportDriver.
func (d *Driver) ImportShareSession(ctx context.Context, c drive.Context, shareURL, password string) (*drive.ShareImportSession, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	shareID, _ := parseAliShareURL(shareURL)
	if shareID == "" {
		return nil, errors.New("aliopen: 无效的分享链接")
	}
	// Step 1: getShareToken
	var tokenResp struct {
		ShareToken string `json:"share_token"`
	}
	body := map[string]any{"share_id": shareID}
	if password != "" {
		body["share_pwd"] = password
	}
	if err := cl.apiPost(ctx, "/adrive/v1.0/openFile/getShareToken", body, &tokenResp); err != nil {
		return nil, err
	}
	if tokenResp.ShareToken == "" {
		return nil, errors.New("获取分享凭证失败（链接无效、已取消或提取码错误）")
	}
	// Step 2: listByShare
	hdrs := map[string]string{"x-share-token": tokenResp.ShareToken}
	files, err := aliShareListAll(ctx, cl, shareID, tokenResp.ShareToken, "root", hdrs)
	if err != nil {
		return nil, err
	}
	return &drive.ShareImportSession{
		Provider:   providerID,
		ShareURL:   shareURL,
		ShareID:    shareID,
		Password:   password,
		ShareToken: tokenResp.ShareToken,
		RootFileID: "root",
		Files:      files,
	}, nil
}

func aliShareListAll(ctx context.Context, cl *client, shareID, shareToken, parentFileID string, hdrs map[string]string) ([]drive.ShareImportFile, error) {
	var out []drive.ShareImportFile
	marker := ""
	seenMarkers := map[string]bool{}
	for {
		body := map[string]any{
			"share_id":        shareID,
			"parent_file_id":  parentFileID,
			"limit":           100,
			"order_by":        "name",
			"order_direction": "ASC",
		}
		if marker != "" {
			body["marker"] = marker
		}
		var resp shareListResp
		if err := cl.apiPostWith(ctx, "/adrive/v1.0/openFile/listByShare", body, &resp, hdrs); err != nil {
			return nil, err
		}
		for i := range resp.Items {
			item := resp.Items[i]
			out = append(out, drive.ShareImportFile{
				FileID: item.FileID,
				Name:   item.Name,
				Size:   item.Size,
				IsDir:  item.Type == "folder",
			})
		}
		if resp.NextMarker == "" {
			break
		}
		if seenMarkers[resp.NextMarker] {
			return nil, errors.New("aliopen: duplicate share list cursor")
		}
		seenMarkers[resp.NextMarker] = true
		marker = resp.NextMarker
	}
	return out, nil
}

// SaveShare implements drive.ShareImportDriver.
func (d *Driver) SaveShare(ctx context.Context, c drive.Context, session *drive.ShareImportSession, fileIDs []string, toParentID string) ([]string, error) {
	if session == nil || session.Provider != providerID || strings.TrimSpace(session.ShareID) == "" || strings.TrimSpace(session.ShareToken) == "" {
		return nil, errors.New("aliopen: 分享会话无效或已过期")
	}
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	// Determine target drive + parent
	targetDrive := cl.session.ResourceDriveID
	if targetDrive == "" {
		targetDrive = cl.session.DriveID
	}
	if targetDrive == "" {
		return nil, errors.New("aliopen: 缺少目标 drive_id")
	}
	parent := toParentID
	if parent == "" || parent == "root" || parent == "/" || parent == RootID {
		parent = "root"
	}
	ref := parseRef(parent)
	parent = ref.FID
	if parent == "" {
		parent = "root"
	}
	// If the target scope's drive_id differs from targetDrive, use the target drive
	if ref.Scope == ScopeResource && cl.session.ResourceDriveID != "" {
		targetDrive = cl.session.ResourceDriveID
	}
	hdrs := map[string]string{"x-share-token": session.ShareToken}
	var saved []string
	var failed []error
	for _, fileID := range fileIDs {
		body := map[string]any{
			"share_id":          session.ShareID,
			"file_id":           fileID,
			"to_drive_id":       targetDrive,
			"to_parent_file_id": parent,
			"auto_rename":       true,
		}
		if err := cl.apiPostWith(ctx, "/adrive/v1.0/openFile/copy", body, nil, hdrs); err != nil {
			failed = append(failed, fmt.Errorf("%s: %w", fileID, err))
			continue
		}
		saved = append(saved, fileID)
	}
	return saved, errors.Join(failed...)
}

// parseAliShareURL extracts share_id from an Aliyun share URL.
func parseAliShareURL(raw string) (shareID string, pwd string) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", ""
	}
	// https://www.alipan.com/s/{shareID}
	// https://www.aliyundrive.com/s/{shareID}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) >= 2 && parts[0] == "s" {
		shareID = parts[1]
	}
	if shareID == "" && len(parts) > 0 {
		last := parts[len(parts)-1]
		if len(last) > 8 {
			shareID = last
		}
	}
	pwd = u.Query().Get("p")
	if pwd == "" {
		pwd = u.Query().Get("pwd")
	}
	return shareID, pwd
}

func (d *Driver) RefreshAccount(ctx context.Context, c drive.Context, token *model.TokenInfo) (*model.TokenInfo, error) {
	sess := parseSession(token)
	if sess == nil {
		return nil, errors.New("aliopen: 会话不存在，请重新登录")
	}
	cl := &client{http: netx.NewClient(60 * time.Second), session: sess, token: token}
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

func sameAliOpenScopeIDs(ids ...string) (Scope, error) {
	if len(ids) == 0 {
		return ScopeBackup, nil
	}
	scope := parseRef(ids[0]).Scope
	for _, id := range ids[1:] {
		if parseRef(id).Scope != scope {
			return "", errors.New("aliopen: 不能跨备份盘与资源盘操作")
		}
	}
	return scope, nil
}

func sameAliOpenRefScope(refs []drive.FileRef, targetID string) (Scope, error) {
	ids := make([]string, 0, len(refs)+1)
	for _, ref := range refs {
		ids = append(ids, ref.ID)
	}
	if targetID != "" {
		ids = append(ids, targetID)
	}
	return sameAliOpenScopeIDs(ids...)
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
	uid := strings.TrimSpace(sess.DriveID)
	if uid == "" {
		return nil, errors.New("aliopen: 登录成功但未返回 drive_id")
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
