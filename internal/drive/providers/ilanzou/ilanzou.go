package ilanzou

import (
	"context"
	"errors"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/drive/driveutil"
	"mnemo-go/internal/model"
)

const providerID = model.ProviderIlanzou

func init() {
	drive.Register(drive.Registration{
		ID:   providerID,
		Meta: drive.GetMeta(providerID),
		Caps: drive.NewCapabilities(providerID, map[string]bool{
			"search":          false,
			"createShare":     false,
			"copy":            false,
			"recycleBin":      false,
			"permanentDelete": true,
			"trashView":       false,
		}, nil),
		Login: drive.LoginConfig{Fields: []drive.LoginField{
			{Key: "username", Type: "text", Label: "账号", Required: true},
			{Key: "password", Type: "password", Label: "密码", Required: true},
		}},
		Auth:    authLogin,
		Factory: func() drive.Driver { return &Driver{} },
	})
}

// Driver implements drive.Driver for 优享版蓝奏云.
type Driver struct {
	drive.BaseDriver
}

func (d *Driver) ID() string                       { return providerID }
func (d *Driver) Meta() drive.Meta                 { return drive.GetMeta(providerID) }
func (d *Driver) Capabilities() drive.Capabilities { return drive.RegistryCaps(providerID) }
func (d *Driver) RootID() string                   { return ILANZOU_ROOT }

// rootID maps root sentinels to ILANZOU_ROOT (legacy isDriveProviderRootId).
func rootID(id string) string {
	if id == "" || id == ILANZOU_ROOT || id == "root" || id == "/" {
		return ILANZOU_ROOT
	}
	return id
}

func isRootSentinel(id string) bool {
	return id == "" || id == ILANZOU_ROOT || id == "root" || id == "/" || id == "0"
}

func (d *Driver) List(ctx context.Context, c drive.Context, dirID string, _ *drive.ListOptions) ([]model.File, error) {
	parent := rootID(dirID)
	parentAPI := "0"
	if parent != ILANZOU_ROOT {
		parentAPI = parent
	}
	items, err := d.fileList(ctx, c, parentAPI)
	if err != nil {
		return nil, err
	}
	out := make([]model.File, 0, len(items))
	for _, it := range items {
		out = append(out, mapILanzouItem(it, c.DriveID, parentAPI))
	}
	return out, nil
}

// GetInfo returns raw provider detail (root pseudo entry or a best-effort
// stub, matching the legacy cache-miss behaviour).
func (d *Driver) GetInfo(ctx context.Context, c drive.Context, fileID string) (any, error) {
	if isRootSentinel(fileID) {
		return *rootFile(c), nil
	}
	f, err := d.GetFile(ctx, c, fileID)
	if err != nil {
		return nil, err
	}
	return *f, nil
}

// GetFile returns the unified file model (legacy served this from the frontend
// meta cache; on a cache miss we return the same stub GetInfo produced).
func (d *Driver) GetFile(ctx context.Context, c drive.Context, fileID string) (*model.File, error) {
	if isRootSentinel(fileID) {
		return rootFile(c), nil
	}
	f := &model.File{
		DriveID:    c.DriveID,
		FileID:     fileID,
		Name:       fileID,
		NameSearch: fileID,
		IsDir:      false,
		Category:   "file",
	}
	return f, nil
}

func rootFile(c drive.Context) *model.File {
	return &model.File{
		DriveID:    c.DriveID,
		FileID:     ILANZOU_ROOT,
		Name:       "优享版蓝奏云",
		NameSearch: "优享版蓝奏云",
		Category:   "folder",
		Icon:       "iconfile-folder",
		IsDir:      true,
	}
}

func (d *Driver) GetDownloadURL(ctx context.Context, c drive.Context, fileID string, _ int) (*model.DownloadURL, error) {
	info := d.downloadInfo(ctx, c, fileID)
	if info.Error != "" {
		return nil, errors.New(info.Error)
	}
	if info.URL == "" {
		return nil, errors.New("获取下载地址失败")
	}
	return &model.DownloadURL{
		DriveID:      c.DriveID,
		FileID:       fileID,
		ExpireTime:   driveutil.GetExpiresTime(info.URL),
		URL:          info.URL,
		Size:         info.Size,
		Headers:      info.Headers,
		DownloadMode: "proxy",
		Concurrency:  1,
	}, nil
}

// GetVideoPreview reuses the download source as an origin-quality proxy stream.
func (d *Driver) GetVideoPreview(ctx context.Context, c drive.Context, fileID string) (*model.VideoPreview, error) {
	u, err := d.GetDownloadURL(ctx, c, fileID, 0)
	if err != nil {
		return nil, err
	}
	return &model.VideoPreview{
		DriveID: c.DriveID, FileID: fileID, Size: u.Size, Headers: u.Headers,
		Qualities: []model.VideoQuality{{
			Quality: "origin", Label: "原画", Value: "origin", URL: u.URL,
			Headers: u.Headers, ForceProxy: true,
		}},
	}, nil
}

func (d *Driver) Mkdir(ctx context.Context, c drive.Context, parentID, name string) (*drive.MkdirResult, error) {
	return d.mkdir(ctx, c, parentID, name)
}

// Rename uses the meta cache for the kind when available, otherwise tries a
// file rename then falls back to a folder rename (legacy resolveKinds).
func (d *Driver) Rename(ctx context.Context, c drive.Context, fileID, name string) (*drive.RenameResult, error) {
	if isDir, ok := drive.Lookup(c.DriveID, fileID); ok {
		if err := d.rename(ctx, c, fileID, name, isDir); err != nil {
			return nil, err
		}
		return &drive.RenameResult{FileID: fileID, Name: name, IsDir: isDir}, nil
	}
	if err := d.rename(ctx, c, fileID, name, false); err == nil {
		return &drive.RenameResult{FileID: fileID, Name: name, IsDir: false}, nil
	} else if err2 := d.rename(ctx, c, fileID, name, true); err2 != nil {
		return nil, err
	}
	return &drive.RenameResult{FileID: fileID, Name: name, IsDir: true}, nil
}

// Trash: 优享版蓝奏云 has no recycle bin; the legacy adapter returned [].
func (d *Driver) Trash(ctx context.Context, c drive.Context, fileIDs []string) ([]string, error) {
	return []string{}, nil
}

// Delete removes files/folders in batches, resolving the kind from the ref or
// the meta cache and falling back file→folder for unknown refs.
func (d *Driver) Delete(ctx context.Context, c drive.Context, refs []drive.FileRef) ([]string, error) {
	var known, unknown []drive.FileRef
	for _, ref := range refs {
		if ref.IsDir != nil {
			known = append(known, ref)
			continue
		}
		if isDir, ok := drive.Lookup(c.DriveID, ref.ID); ok {
			known = append(known, drive.FileRef{ID: ref.ID, IsDir: &isDir})
			continue
		}
		unknown = append(unknown, ref)
	}
	var ok []string
	if len(known) > 0 {
		deleted, err := d.deleteBatch(ctx, c, known)
		if err != nil {
			return nil, err
		}
		ok = append(ok, deleted...)
	}
	if len(unknown) > 0 {
		deleted := d.deleteWithFallback(ctx, c, unknown)
		ok = append(ok, deleted...)
	}
	if len(ok) == 0 {
		ids := make([]string, 0, len(refs))
		for _, r := range refs {
			ids = append(ids, r.ID)
		}
		return ids, nil
	}
	return ok, nil
}

// deleteWithFallback tries the batch as files, then as folders.
func (d *Driver) deleteWithFallback(ctx context.Context, c drive.Context, refs []drive.FileRef) []string {
	deleted, err := d.deleteBatch(ctx, c, refs)
	if err == nil {
		return deleted
	}
	folders := make([]drive.FileRef, len(refs))
	dir := true
	for i, r := range refs {
		folders[i] = drive.FileRef{ID: r.ID, IsDir: &dir}
	}
	deleted, err = d.deleteBatch(ctx, c, folders)
	if err != nil {
		return nil
	}
	return deleted
}

// Move moves files/folders in batches with kind resolution + fallback.
func (d *Driver) Move(ctx context.Context, c drive.Context, refs []drive.FileRef, toParentID, _ string) ([]string, error) {
	target := rootID(toParentID)
	if target == ILANZOU_ROOT {
		target = "0"
	}
	var known, unknown []drive.FileRef
	for _, ref := range refs {
		if ref.IsDir != nil {
			known = append(known, ref)
			continue
		}
		if isDir, ok := drive.Lookup(c.DriveID, ref.ID); ok {
			known = append(known, drive.FileRef{ID: ref.ID, IsDir: &isDir})
			continue
		}
		unknown = append(unknown, ref)
	}
	var ok []string
	if len(known) > 0 {
		moved, err := d.moveBatch(ctx, c, known, target)
		if err != nil {
			return nil, err
		}
		ok = append(ok, moved...)
	}
	if len(unknown) > 0 {
		moved := d.moveWithFallback(ctx, c, unknown, target)
		ok = append(ok, moved...)
	}
	if len(ok) == 0 {
		ids := make([]string, 0, len(refs))
		for _, r := range refs {
			ids = append(ids, r.ID)
		}
		return ids, nil
	}
	return ok, nil
}

func (d *Driver) moveWithFallback(ctx context.Context, c drive.Context, refs []drive.FileRef, target string) []string {
	moved, err := d.moveBatch(ctx, c, refs, target)
	if err == nil {
		return moved
	}
	folders := make([]drive.FileRef, len(refs))
	dir := true
	for i, r := range refs {
		folders[i] = drive.FileRef{ID: r.ID, IsDir: &dir}
	}
	moved, err = d.moveBatch(ctx, c, folders, target)
	if err != nil {
		return nil
	}
	return moved
}

// Copy: 优享版蓝奏云 API 不支持服务端复制（legacy 返回空数组）。
func (d *Driver) Copy(ctx context.Context, c drive.Context, refs []drive.FileRef, toParentID, _ string) ([]string, error) {
	return []string{}, nil
}
