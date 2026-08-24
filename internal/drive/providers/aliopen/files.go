package aliopen

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/drive/driveutil"
	"mnemo-go/internal/model"
)

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

type aliOpenDownloadResponse struct {
	URL             string            `json:"url"`
	Size            int64             `json:"size"`
	Headers         map[string]string `json:"headers"`
	Expiration      string            `json:"expiration"`
	StreamsURL      map[string]any    `json:"streamsUrl"`
	StreamsURLSnake map[string]any    `json:"streams_url"`
}

type listResp struct {
	Items  []aliFile `json:"items"`
	Marker string    `json:"next_marker"`
}

// ListPage returns one page of a directory.
func (c *client) ListPage(ctx context.Context, scope Scope, parentID, marker string) ([]aliFile, string, error) {
	body := map[string]any{
		"parent_file_id": parentID,
		"drive_id":       c.scopedDriveID(scope),
		"limit":          aliOpenListPageLimit,
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
	var resp aliOpenDownloadResponse
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
	// Live Photo files can expose only streamsUrl/streams_url. Prefer the
	// playable MOV stream, then JPEG, and keep the normal URL as fallback.
	url := strings.TrimSpace(resp.URL)
	for _, streams := range []map[string]any{resp.StreamsURL, resp.StreamsURLSnake} {
		for _, key := range []string{"mov", "jpeg"} {
			if stream := aliOpenStreamURL(streams, key); stream != "" {
				url = stream
				break
			}
		}
		if url != strings.TrimSpace(resp.URL) {
			break
		}
	}
	if url == "" {
		return "", 0, nil, 0, errors.New("aliopen: 获取下载地址失败")
	}
	expireTime := parseAliOpenExpiration(resp.Expiration)
	if expireTime == 0 {
		expireTime = driveutil.GetExpiresTime(url)
	}
	return url, resp.Size, resp.Headers, expireTime, nil
}

func aliOpenStreamURL(streams map[string]any, key string) string {
	if len(streams) == 0 {
		return ""
	}
	value, ok := streams[key]
	if !ok {
		return ""
	}
	if raw, ok := value.(string); ok {
		return strings.TrimSpace(raw)
	}
	return ""
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
