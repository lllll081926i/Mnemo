package onedrive

import (
	"context"
	"errors"
	"net/http"
	"os"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

const providerID = model.ProviderOnedrive

func init() {
	drive.Register(drive.Registration{
		ID:   providerID,
		Meta: drive.GetMeta(providerID),
		Caps: drive.NewCapabilities(providerID, map[string]bool{
			"search":        true,
			"createShare":   true,
			"shareExpiration": true,
			"sharePassword": true,
			"shareHistory":  true,
		}, nil),
		Factory: func() drive.Driver { return &Driver{} },
		Auth:    authPKCE,
		Login: drive.LoginConfig{Fields: []drive.LoginField{
			{Key: "action", Type: "oauth", Label: "OAuth 授权"},
		}},
	})
}

// Driver implements drive.Driver for OneDrive.
type Driver struct {
	drive.BaseDriver
}

func (d *Driver) ID() string                       { return providerID }
func (d *Driver) Meta() drive.Meta                 { return drive.GetMeta(providerID) }
func (d *Driver) Capabilities() drive.Capabilities { return drive.RegistryCaps(providerID) }
func (d *Driver) RootID() string                   { return RootID }

func clientOf(c drive.Context) (*client, error) {
	if c.Token == nil || c.Token.AccessToken == "" {
		return nil, drive.ErrUnauthorized
	}
	return newClient(c.Token.AccessToken), nil
}

func (d *Driver) List(ctx context.Context, c drive.Context, dirID string, _ *drive.ListOptions) ([]model.File, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	items, err := cl.List(ctx, dirID)
	if err != nil {
		return nil, err
	}
	return mapItems(items, c.DriveID, dirID), nil
}

func (d *Driver) ListPaged(ctx context.Context, c drive.Context, dirID, marker string, _ *drive.ListOptions) (*drive.DirPage, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	items, next, err := cl.ListPage(ctx, dirID, marker)
	if err != nil {
		return nil, err
	}
	return &drive.DirPage{Items: mapItems(items, c.DriveID, dirID), NextMarker: next}, nil
}

func (d *Driver) Search(ctx context.Context, c drive.Context, keyword string) ([]model.File, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	items, err := cl.Search(ctx, keyword)
	if err != nil {
		return nil, err
	}
	return mapItems(items, c.DriveID, RootID), nil
}

func (d *Driver) GetInfo(ctx context.Context, c drive.Context, fileID string) (any, error) {
	if fileID == RootID {
		f := driveutil_file(c.DriveID, RootID, "")
		return f, nil
	}
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	item, err := cl.Detail(ctx, fileID)
	if err != nil {
		return nil, err
	}
	parent := RootID
	if item.ParentReference != nil && item.ParentReference.ID != "" {
		parent = item.ParentReference.ID
	}
	return mapItem(item, c.DriveID, parent), nil
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
	item, err := cl.Detail(ctx, fileID)
	if err != nil {
		return nil, err
	}
	if item.Folder != nil {
		return nil, errors.New("文件夹不能直接下载")
	}
	return &model.DownloadURL{
		DriveID:      c.DriveID,
		FileID:       fileID,
		URL:          cl.DownloadURL(item),
		Size:         item.Size,
		DownloadMode: "redirect",
		Headers:      map[string]string{"Authorization": "Bearer " + c.Token.AccessToken},
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
			Quality: "origin", Label: "原画", Value: "origin", URL: u.URL, Headers: u.Headers, ForceProxy: true,
		}},
	}, nil
}

func (d *Driver) Mkdir(ctx context.Context, c drive.Context, parentID, name string) (*drive.MkdirResult, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	return cl.Mkdir(ctx, parentID, name)
}

func (d *Driver) Rename(ctx context.Context, c drive.Context, fileID, name string) (*drive.RenameResult, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	item, err := cl.Rename(ctx, fileID, name)
	if err != nil {
		return nil, err
	}
	parent := RootID
	if item.ParentReference != nil && item.ParentReference.ID != "" {
		parent = item.ParentReference.ID
	}
	return &drive.RenameResult{FileID: item.ID, ParentFileID: parent, Name: item.Name, IsDir: item.Folder != nil}, nil
}

func (d *Driver) Trash(ctx context.Context, c drive.Context, fileIDs []string) ([]string, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	var ok []string
	for _, id := range fileIDs {
		if err := cl.Delete(ctx, id); err == nil {
			ok = append(ok, id)
		}
	}
	return ok, nil
}

func (d *Driver) Delete(ctx context.Context, c drive.Context, refs []drive.FileRef) ([]string, error) {
	// OneDrive delete removes directly here.
	ids := make([]string, 0, len(refs))
	for _, r := range refs {
		ids = append(ids, r.ID)
	}
	return d.Trash(ctx, c, ids)
}

func (d *Driver) Restore(ctx context.Context, c drive.Context, fileIDs []string) ([]string, error) {
	return nil, drive.NotSupported("restore")
}

func (d *Driver) Move(ctx context.Context, c drive.Context, refs []drive.FileRef, toParentID, _ string) ([]string, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	var ok []string
	for _, r := range refs {
		if err := cl.Move(ctx, r.ID, toParentID); err == nil {
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
	var ok []string
	for _, r := range refs {
		// fetch name for the copy target
		name := ""
		if item, err := cl.Detail(ctx, r.ID); err == nil {
			name = item.Name
		}
		if err := cl.Copy(ctx, r.ID, toParentID, name); err == nil {
			ok = append(ok, r.ID)
		}
	}
	return ok, nil
}

func (d *Driver) CreateShare(ctx context.Context, c drive.Context, params drive.ShareParams) (*model.ShareItem, error) {
	if len(params.FileIDs) != 1 {
		return nil, errors.New("OneDrive 分享链接一次只能选择一个文件或文件夹")
	}
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	return cl.CreateLink(ctx, params.FileIDs[0], params.Expiration, params.Password)
}

// UploadOneFile uploads one file (simple PUT for small, upload session for large).
func (d *Driver) UploadOneFile(ctx context.Context, c drive.Context, ui *model.UploadingUI) error {
	cl, err := clientOf(c)
	if err != nil {
		return err
	}
	f, err := os.Open(ui.Info.LocalFilePath)
	if err != nil {
		return err
	}
	defer f.Close()
	name := ui.Info.Name
	parentID := ui.Info.ParentFileID
	if ui.Info.Size <= smallUploadLimit {
		target := graphHost + smallUploadPath(parentID, name)
		return cl.rawPut(ctx, target, f)
	}
	return cl.sessionUpload(ctx, f, parentID, name, ui)
}

func mapItems(items []Item, driveID, parentID string) []model.File {
	out := make([]model.File, 0, len(items))
	for i := range items {
		out = append(out, mapItem(&items[i], driveID, parentID))
	}
	return out
}

// mapRootFile builds a synthetic root folder file.
func driveutil_file(driveID, fileID, name string) model.File {
	if name == "" {
		name = "OneDrive"
	}
	return model.File{
		DriveID: driveID, FileID: fileID, ParentFileID: "", Name: name,
		NameSearch: name, Category: "folder", IsDir: true, Icon: "iconfile-folder",
	}
}

var _ = http.MethodGet