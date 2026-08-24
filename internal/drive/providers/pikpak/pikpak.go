package pikpak

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/drive/driveutil"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

// Driver implements drive.Driver for PikPak.
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
	deviceID := c.Token.DeviceID
	if deviceID == "" {
		deviceID = c.Token.UserName
	}
	accountID := strings.TrimSpace(c.Token.ProviderAccountID)
	if accountID == "" {
		accountID = model.StripUserID(providerID, c.Token.UserID)
	}
	cl := newClient(c.Token.AccessToken, deviceID, accountID)
	if cl.deviceID == "" {
		cl.deviceID = c.Token.DeviceID
	}
	return cl, nil
}

func (d *Driver) List(ctx context.Context, c drive.Context, dirID string, _ *drive.ListOptions) ([]model.File, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	return listPages(ctx, cl, c, dirID, false)
}

func (d *Driver) ListPaged(ctx context.Context, c drive.Context, dirID, marker string, _ *drive.ListOptions) (*drive.DirPage, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	rooted := rootID(dirID)
	items, next, err := cl.ListPage(ctx, rooted, marker, false)
	if err != nil {
		return nil, err
	}
	return &drive.DirPage{Items: mapFiles(items, c.DriveID, rooted), NextMarker: next}, nil
}

func listPages(ctx context.Context, cl *client, c drive.Context, dirID string, trashed bool) ([]model.File, error) {
	items, err := cl.List(ctx, rootID(dirID), trashed)
	if err != nil {
		return nil, err
	}
	return mapFiles(items, c.DriveID, rootID(dirID)), nil
}

func (d *Driver) ListTrash(ctx context.Context, c drive.Context, _ *drive.ListOptions) ([]model.File, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	return listPages(ctx, cl, c, RootID, true)
}

func (d *Driver) GetInfo(ctx context.Context, c drive.Context, fileID string) (any, error) {
	if fileID == RootID {
		return model.File{DriveID: c.DriveID, FileID: RootID, Name: "网盘文件", NameSearch: "pikpak", IsDir: true, Icon: "iconfile-folder"}, nil
	}
	return d.GetFile(ctx, c, fileID)
}

func (d *Driver) GetFile(ctx context.Context, c drive.Context, fileID string) (*model.File, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	item, err := cl.Detail(ctx, fileID)
	if err != nil {
		return nil, err
	}
	f := mapFile(item, c.DriveID, firstNonEmpty(item.ParentID, "pikpak_root"))
	return &f, nil
}

func (d *Driver) GetDownloadURL(ctx context.Context, c drive.Context, fileID string, _ int) (*model.DownloadURL, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	u, size, err := cl.DownloadURL(ctx, fileID)
	if err != nil {
		return nil, err
	}
	// Detail/download normally return an OSS/CDN pre-signed URL. Sending the
	// PikPak account token to that foreign origin is unnecessary, can make a
	// valid signed download fail, and exposes a credential outside the API
	// host. Keep Authorization only for the documented API fallback URL.
	var headers map[string]string
	if pikpakDownloadNeedsAuthorization(u) {
		headers = map[string]string{"Authorization": "Bearer " + c.Token.AccessToken}
	}
	return &model.DownloadURL{
		DriveID: c.DriveID, FileID: fileID, URL: u, Size: size,
		DownloadMode: "redirect",
		Headers:      headers,
	}, nil
}

// pikpakDownloadNeedsAuthorization restricts a bearer token to the HTTPS
// PikPak Drive API. All object-storage links returned by that API are treated
// as self-contained signed URLs, including malformed/unknown URLs by default.
func pikpakDownloadNeedsAuthorization(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Scheme, "https") &&
		(u.Port() == "" || u.Port() == "443") &&
		strings.EqualFold(u.Hostname(), "api-drive.mypikpak.com")
}

func (d *Driver) GetVideoPreview(ctx context.Context, c drive.Context, fileID string) (*model.VideoPreview, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	p, err := cl.PlayInfo(ctx, fileID)
	if err != nil {
		return nil, err
	}
	p.DriveID = c.DriveID
	p.FileID = fileID
	return p, nil
}

func (d *Driver) Mkdir(ctx context.Context, c drive.Context, parentID, name string) (*drive.MkdirResult, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	res, err := cl.Mkdir(ctx, rootID(parentID), name)
	if err != nil {
		return nil, err
	}
	return &drive.MkdirResult{FileID: res.FileID, Error: res.Error}, nil
}

func (d *Driver) Rename(ctx context.Context, c drive.Context, fileID, name string) (*drive.RenameResult, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	item, err := cl.Detail(ctx, fileID)
	if err != nil {
		return nil, err
	}
	if err := cl.Rename(ctx, fileID, name); err != nil {
		return nil, err
	}
	return &drive.RenameResult{FileID: fileID, Name: name, ParentFileID: item.ParentID, IsDir: item.Kind == "drive#folder"}, nil
}

func (d *Driver) Trash(ctx context.Context, c drive.Context, fileIDs []string) ([]string, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	if err := cl.Trash(ctx, fileIDs); err != nil {
		return nil, err
	}
	return fileIDs, nil
}

func (d *Driver) Delete(ctx context.Context, c drive.Context, refs []drive.FileRef) ([]string, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	ids := refIDs(refs)
	if err := cl.Delete(ctx, ids); err != nil {
		return nil, err
	}
	return ids, nil
}

func (d *Driver) Restore(ctx context.Context, c drive.Context, fileIDs []string) ([]string, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	if err := cl.Restore(ctx, fileIDs); err != nil {
		return nil, err
	}
	return fileIDs, nil
}

func (d *Driver) Move(ctx context.Context, c drive.Context, refs []drive.FileRef, toParentID, _ string) ([]string, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	ids := refIDs(refs)
	if err := cl.Move(ctx, ids, rootID(toParentID)); err != nil {
		return nil, err
	}
	return ids, nil
}

func (d *Driver) Copy(ctx context.Context, c drive.Context, refs []drive.FileRef, toParentID, _ string) ([]string, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	ids := refIDs(refs)
	if err := cl.Copy(ctx, ids, rootID(toParentID)); err != nil {
		return nil, err
	}
	return ids, nil
}

func (d *Driver) Favorite(ctx context.Context, c drive.Context, fileIDs []string, favorite bool) ([]string, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	if err := cl.Star(ctx, fileIDs, favorite); err != nil {
		return nil, err
	}
	return fileIDs, nil
}

func (d *Driver) RefreshAccount(ctx context.Context, c drive.Context, token *model.TokenInfo) (*model.TokenInfo, error) {
	if token == nil {
		return nil, errors.New("PikPak 未登录")
	}
	refresh := strings.TrimSpace(token.RefreshToken)
	if refresh == "" {
		return nil, errors.New("PikPak 缺少 refresh token，请重新登录")
	}
	deviceID := token.DeviceID
	if deviceID == "" {
		deviceID = token.UserName
	}
	hc := netx.NewClient(60 * time.Second)
	auth, err := refreshToken(ctx, hc, deviceID, refresh)
	if err != nil {
		return nil, err
	}
	token.AccessToken = auth.AccessToken
	// PikPak may omit refresh_token or expires_in when rotation is not
	// required. Never erase a still-valid persisted session field in that case.
	if strings.TrimSpace(auth.RefreshToken) != "" {
		token.RefreshToken = auth.RefreshToken
	}
	if auth.ExpiresIn > 0 {
		token.ExpiresIn = auth.ExpiresIn
	}
	if auth.TokenType != "" {
		token.TokenType = auth.TokenType
	}
	if cl := newClient(auth.AccessToken, deviceID, token.ProviderAccountID); cl != nil {
		token.UsedSize, token.TotalSize = cl.About(ctx)
	}
	return token, nil
}

func rootID(id string) string {
	if id == "" || id == "root" || id == RootID || id == "/" {
		return RootID
	}
	return id
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func refIDs(refs []drive.FileRef) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.ID)
	}
	return out
}

// mapFile converts a PikPak file to the unified model.
func mapFile(item *File, driveID, parentID string) model.File {
	isDir := item.Kind == "drive#folder"
	timeUnix := int64(0)
	if parsed, err := time.Parse(time.RFC3339, item.ModifiedTime); err == nil {
		timeUnix = parsed.Unix()
	}
	if timeUnix == 0 {
		if parsed, err := time.Parse(time.RFC3339, item.CreatedTime); err == nil {
			timeUnix = parsed.Unix()
		}
	}
	f := driveutil.NewFile(driveID, item.ID, parentID, item.Name, isDir, item.Size, timeUnix)
	f.Thumbnail = item.Thumbnail
	f.Starred = item.Starred
	f.Ext = item.FileExtension
	f.Category = driveutil.GuessCategory(item.Name)
	f.Description = item.Phase
	if hash, ok := normalizePikPakGCID(item.Hash); ok {
		f.ContentHash = strings.ToLower(hash)
		f.ContentHashName = "gcid"
	}
	if item.WebContentLink != "" {
		f.DownloadURL = item.WebContentLink
	}
	return f
}

func mapFiles(items []File, driveID, parentID string) []model.File {
	out := make([]model.File, 0, len(items))
	for i := range items {
		out = append(out, mapFile(&items[i], driveID, parentID))
	}
	return out
}
