package onedrive

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
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
		}, func(c *drive.Capabilities) {
			c.SetHashes([]string{"sha1", "quickxorhash"}, nil)
		}),
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

// oneDriveScope is the OAuth scope used for token refresh.
const oneDriveScope = "files.readwrite offline_access User.Read"

// refreshOneDriveToken exchanges a refresh_token for a fresh access_token via
// the Microsoft OAuth2 token endpoint.
func refreshOneDriveToken(ctx context.Context, clientID, refreshToken string) (*model.TokenInfo, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("scope", oneDriveScope)

	cl := netx.NewClient(60 * time.Second)
	var raw struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		TokenType    string `json:"token_type"`
	}
	if err := cl.PostForm(ctx, msTokenURL, nil, form, &raw); err != nil {
		return nil, err
	}
	if raw.AccessToken == "" {
		return nil, errors.New("onedrive: refresh returned no access_token")
	}
	return &model.TokenInfo{
		AccessToken:  raw.AccessToken,
		RefreshToken: raw.RefreshToken,
		ExpiresIn:    raw.ExpiresIn,
		TokenType:    raw.TokenType,
	}, nil
}

// fetchOneDriveProfile queries /me and /me/drive to populate the token's
// UserName, quota, and drive id.
func fetchOneDriveProfile(ctx context.Context, accessToken string, tok *model.TokenInfo) {
	cl := newClient(accessToken)

	// /me — account display name
	var me struct {
		ID                string `json:"id"`
		DisplayName       string `json:"displayName"`
		UserPrincipalName string `json:"userPrincipalName"`
	}
	if err := cl.getJSON(ctx, "/me", &me); err == nil {
		if me.UserPrincipalName != "" {
			tok.UserName = me.UserPrincipalName
		} else if me.DisplayName != "" {
			tok.UserName = me.DisplayName
		}
		if me.DisplayName != "" {
			tok.NickName = me.DisplayName
		}
		if me.ID != "" {
			tok.ProviderAccountID = me.ID
		}
	}

	// /me/drive — quota + drive id
	var driveInfo struct {
		ID    string `json:"id"`
		Quota *struct {
			Total int64 `json:"total"`
			Used  int64 `json:"used"`
	} `json:"quota"`
	}
	if err := cl.getJSON(ctx, "/me/drive", &driveInfo); err == nil {
		if driveInfo.ID != "" {
			tok.DefaultDriveID = driveInfo.ID
		}
		if driveInfo.Quota != nil {
			tok.TotalSize = driveInfo.Quota.Total
			tok.UsedSize = driveInfo.Quota.Used
			if tok.TotalSize > tok.UsedSize {
				tok.FreeSize = tok.TotalSize - tok.UsedSize
			}
		}
	}
}

// RefreshAccount renews the OneDrive OAuth access token using the stored
// refresh_token, then fetches account profile and drive quota to update the
// token metadata.
func (d *Driver) RefreshAccount(ctx context.Context, c drive.Context, token *model.TokenInfo) (*model.TokenInfo, error) {
	if token == nil {
		return nil, nil
	}
	refreshToken := strings.TrimSpace(token.RefreshToken)
	if refreshToken == "" {
		return nil, errors.New("onedrive: missing refresh_token")
	}
	clientID := strings.TrimSpace(drive.Secret("onedrive_client_id"))
	if clientID == "" {
		return nil, errors.New("onedrive: client_id 未配置（secrets.json onedrive_client_id）")
	}

	fresh, err := refreshOneDriveToken(ctx, clientID, refreshToken)
	if err != nil {
		return nil, err
	}

	// preserve fields not returned by the token endpoint
	token.AccessToken = fresh.AccessToken
	if fresh.RefreshToken != "" {
		token.RefreshToken = fresh.RefreshToken
	}
	token.ExpiresIn = fresh.ExpiresIn
	if fresh.TokenType != "" {
		token.TokenType = fresh.TokenType
	}
	token.TokenFrom = providerID

	// update account info + quota (non-blocking on error)
	fetchOneDriveProfile(ctx, token.AccessToken, token)

	return token, nil
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