// Package webdav implements the WebDAV drive provider (mounted storage).
package webdav

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/drive/driveutil"
	"mnemo-go/internal/model"
	wc "mnemo-go/internal/provider/webdav"
)

const providerID = model.ProviderWebdav

func init() {
	drive.Register(drive.Registration{
		ID:   providerID,
		Meta: drive.GetMeta(providerID),
		Caps: drive.NewCapabilities(providerID, map[string]bool{
			"mountedStorage":  true,
			"permanentDelete": true,
			"recycleBin":      false,
			"trashView":       false,
		}, func(c *drive.Capabilities) {
			c.SetUploadMode(drive.UploadModeDirect)
		}),
		Factory: func() drive.Driver { return &Driver{} },
	})
}

// Driver implements drive.Driver for WebDAV. File ids are server paths.
type Driver struct {
	drive.BaseDriver
}

func (d *Driver) ID() string                 { return providerID }
func (d *Driver) Meta() drive.Meta           { return drive.GetMeta(providerID) }
func (d *Driver) Capabilities() drive.Capabilities { return drive.RegistryCaps(providerID) }
func (d *Driver) RootID() string             { return "/" }

func clientOf(c drive.Context) (*wc.Client, error) {
	if c.Token == nil || c.Token.Conn == nil {
		return nil, errors.New("webdav: 连接不存在，请重新连接")
	}
	return wc.New(c.Token.Conn, 60*time.Second)
}

// pathOf normalizes a file id to a server path.
func pathOf(id string) string {
	if id == "" || id == "/" || id == "root" {
		return "/"
	}
	if !strings.HasPrefix(id, "/") {
		return "/" + id
	}
	return id
}

func (d *Driver) List(ctx context.Context, c drive.Context, dirID string, _ *drive.ListOptions) ([]model.File, error) {
	client, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	entries, err := client.List(ctx, pathOf(dirID))
	if err != nil {
		return nil, err
	}
	out := make([]model.File, 0, len(entries))
	for _, e := range entries {
		f := driveutil.NewFile(c.DriveID, e.Path, pathOf(dirID), e.Name, e.IsDir, e.Size, e.Modified.Unix())
		out = append(out, f)
	}
	return out, nil
}

func (d *Driver) GetInfo(ctx context.Context, c drive.Context, fileID string) (any, error) {
	client, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	p := pathOf(fileID)
	if p == "/" {
		name := "WebDAV"
		if c.Token != nil && c.Token.Conn != nil && c.Token.Conn.Name != "" {
			name = c.Token.Conn.Name
		}
		return driveutil.NewFile(c.DriveID, "/", "", name, true, 0, 0), nil
	}
	entry, err := client.Stat(ctx, p)
	if err != nil {
		return nil, err
	}
	f := driveutil.NewFile(c.DriveID, entry.Path, parentPath(entry.Path), entry.Name, entry.IsDir, entry.Size, entry.Modified.Unix())
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
	client, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	p := pathOf(fileID)
	entry, err := client.Stat(ctx, p)
	if err != nil {
		return nil, err
	}
	if entry.IsDir {
		return nil, errors.New("文件夹不能直接下载")
	}
	return &model.DownloadURL{
		DriveID:      c.DriveID,
		FileID:       fileID,
		URL:          client.DownloadURL(p),
		Size:         entry.Size,
		Headers:      authHeaders(c),
		DownloadMode: "redirect",
	}, nil
}

func (d *Driver) GetVideoPreview(ctx context.Context, c drive.Context, fileID string) (*model.VideoPreview, error) {
	u, err := d.GetDownloadURL(ctx, c, fileID, 0)
	if err != nil {
		return nil, err
	}
	return &model.VideoPreview{
		DriveID: c.DriveID,
		FileID:  fileID,
		Size:    u.Size,
		Headers: u.Headers,
		Qualities: []model.VideoQuality{{
			Quality: "origin", Label: "原画", Value: "origin",
			URL: u.URL, Headers: u.Headers, ForceProxy: true,
		}},
	}, nil
}

func (d *Driver) Mkdir(ctx context.Context, c drive.Context, parentID, name string) (*drive.MkdirResult, error) {
	client, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	target := driveutil.JoinPath(pathOf(parentID), name)
	if err := client.Mkcol(ctx, target); err != nil {
		return &drive.MkdirResult{Error: err.Error()}, nil
	}
	return &drive.MkdirResult{FileID: target}, nil
}

func (d *Driver) Rename(ctx context.Context, c drive.Context, fileID, name string) (*drive.RenameResult, error) {
	client, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	p := pathOf(fileID)
	info, err := client.Stat(ctx, p)
	if err != nil {
		return nil, err
	}
	target := driveutil.JoinPath(parentPath(p), name)
	if err := client.Move(ctx, p, target); err != nil {
		return nil, err
	}
	return &drive.RenameResult{FileID: target, ParentFileID: parentPath(target), Name: name, IsDir: info.IsDir}, nil
}

func (d *Driver) Trash(ctx context.Context, c drive.Context, fileIDs []string) ([]string, error) {
	// WebDAV has no recycle bin; permanent delete directly.
	return d.Delete(ctx, c, idsToRefs(fileIDs))
}

func (d *Driver) Delete(ctx context.Context, c drive.Context, refs []drive.FileRef) ([]string, error) {
	client, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	var ok []string
	for _, ref := range refs {
		if err := client.Delete(ctx, pathOf(ref.ID)); err == nil {
			ok = append(ok, ref.ID)
		}
	}
	return ok, nil
}

func (d *Driver) Restore(ctx context.Context, c drive.Context, fileIDs []string) ([]string, error) {
	return nil, drive.NotSupported("restore")
}

func (d *Driver) Move(ctx context.Context, c drive.Context, refs []drive.FileRef, toParentID, _ string) ([]string, error) {
	client, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	targetParent := pathOf(toParentID)
	var ok []string
	for _, ref := range refs {
		base := ref.ID
		if i := strings.LastIndex(base, "/"); i >= 0 {
			base = base[i+1:]
		}
		to := driveutil.JoinPath(targetParent, base)
		if err := client.Move(ctx, pathOf(ref.ID), to); err == nil {
			ok = append(ok, ref.ID)
		}
	}
	return ok, nil
}

func (d *Driver) Copy(ctx context.Context, c drive.Context, refs []drive.FileRef, toParentID, _ string) ([]string, error) {
	client, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	targetParent := pathOf(toParentID)
	var ok []string
	for _, ref := range refs {
		base := ref.ID
		if i := strings.LastIndex(base, "/"); i >= 0 {
			base = base[i+1:]
		}
		to := driveutil.JoinPath(targetParent, base)
		if err := client.Copy(ctx, pathOf(ref.ID), to); err == nil {
			ok = append(ok, ref.ID)
		}
	}
	return ok, nil
}

// UploadOneFile performs a direct PUT upload. It honors ui.Info.ConflictPolicy
// when the target path already exists: refuse returns an error, rename uploads
// to a generated non-conflicting name, overwrite (the default) replaces it.
func (d *Driver) UploadOneFile(ctx context.Context, c drive.Context, ui *model.UploadingUI) error {
	client, err := clientOf(c)
	if err != nil {
		return err
	}
	target := driveutil.JoinPath(pathOf(ui.Info.ParentFileID), ui.Info.Name)

	switch driveutil.ResolveConflictPolicy(ui.Info.ConflictPolicy) {
	case driveutil.ConflictRefuse:
		if _, e := client.Stat(ctx, target); e == nil {
			return errors.New("webdav: 目标文件已存在")
		}
	case driveutil.ConflictRename:
		if _, e := client.Stat(ctx, target); e == nil {
			newName := driveutil.GenerateConflictName(ui.Info.Name)
			ui.Info.Name = newName
			target = driveutil.JoinPath(pathOf(ui.Info.ParentFileID), newName)
		}
	}

	f, err := os.Open(ui.Info.LocalFilePath)
	if err != nil {
		return err
	}
	defer f.Close()
	// report read progress back to the upload UI
	pr := driveutil.NewProgressReader(f, ui.Info.Size, func(read int64) {
		ui.Upload.DownSize = read
		if ui.Info.Size > 0 {
			ui.Upload.DownProcess = int(read * 100 / ui.Info.Size)
		}
	})
	return client.Put(ctx, target, pr, ui.Info.Size)
}

func authHeaders(c drive.Context) map[string]string {
	if c.Token != nil && c.Token.Conn != nil {
		conn := c.Token.Conn
		if conn.Username != "" || conn.Password != "" {
			return map[string]string{"Authorization": basicAuth(conn.Username, conn.Password)}
		}
	}
	return nil
}

func basicAuth(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

func idsToRefs(ids []string) []drive.FileRef {
	refs := make([]drive.FileRef, 0, len(ids))
	for _, id := range ids {
		refs = append(refs, drive.FileRef{ID: id})
	}
	return refs
}

func parentPath(p string) string {
	p = strings.TrimRight(p, "/")
	if i := strings.LastIndex(p, "/"); i > 0 {
		return p[:i]
	}
	return "/"
}
