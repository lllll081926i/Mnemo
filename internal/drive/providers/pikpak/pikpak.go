package pikpak

import (
	"context"
	"errors"
	"net/http"
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
	cl := newClient(c.Token.AccessToken, deviceID)
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
	return &model.DownloadURL{
		DriveID: c.DriveID, FileID: fileID, URL: u, Size: size,
		DownloadMode: "redirect",
		Headers:      map[string]string{"Authorization": "Bearer " + c.Token.AccessToken},
	}, nil
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

func (d *Driver) CreateShare(ctx context.Context, c drive.Context, params drive.ShareParams) (*model.ShareItem, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	res, err := cl.CreateShare(ctx, params.FileIDs, params.ShareName, params.Password, params.Expiration)
	if err != nil {
		return nil, err
	}
	return &model.ShareItem{
		AccountID: c.UserID, DriveID: c.DriveID,
		ShareID: res.ShareID, ShareURL: res.ShareURL, SharePwd: res.PassCode,
		ShareName: params.ShareName, Expiration: res.Expiration, FileIDList: res.FileIDs,
	}, nil
}

// UploadOneFile uploads one file (GCID + create + optional OSS PUT).
func (d *Driver) UploadOneFile(ctx context.Context, c drive.Context, ui *model.UploadingUI) error {
	cl, err := clientOf(c)
	if err != nil {
		return err
	}
	gcid, err := computeGCID(ui.Info.LocalFilePath)
	if err != nil {
		return err
	}
	body := map[string]any{
		"kind": "drive#file", "name": ui.Info.Name, "size": ui.Info.Size,
		"hash": gcid, "upload_type": "UPLOAD_TYPE_RESUMABLE",
		"resumable": map[string]any{"provider": "PROVIDER_ALIYUN"},
		"folder_type": "NORMAL",
	}
	if ui.Info.ParentFileID != "" && ui.Info.ParentFileID != RootID {
		body["parent_id"] = ui.Info.ParentFileID
	}
	var res struct {
		UploadType string `json:"upload_type"`
		Resumable  *struct {
			Params *model.ConnConfig `json:"params"`
		} `json:"resumable"`
		File *struct {
			ID string `json:"id"`
		} `json:"file"`
		ID    string `json:"id"`
		Error string `json:"error"`
	}
	// Use a client where we can pass X-Captcha-Token if needed; basic flow:
	if err := cl.jsonDo(ctx, httpMethodPost(), apiHost+"/drive/v1/files", body, &res, nil); err != nil {
		return err
	}
	fileID := ""
	if res.File != nil {
		fileID = res.File.ID
	}
	if fileID == "" {
		fileID = res.ID
	}
	if fileID == "" {
		return errors.New("pikpak: upload create missing file id")
	}
	// No resumable params => rapid upload completed server side.
	if res.Resumable == nil || res.Resumable.Params == nil {
		return nil
	}
	params := res.Resumable.Params
	return ossPut(ctx, c, ui.Info.LocalFilePath, params, ui)
}

func (d *Driver) RefreshAccount(ctx context.Context, c drive.Context, token *model.TokenInfo) (*model.TokenInfo, error) {
	if token == nil {
		return nil, nil
	}
	deviceID := token.DeviceID
	if deviceID == "" {
		deviceID = token.UserName
	}
	hc := netx.NewClient(60 * time.Second)
	auth, err := refreshToken(ctx, hc, deviceID, token.RefreshToken)
	if err != nil {
		return nil, err
	}
	token.AccessToken = auth.AccessToken
	token.RefreshToken = auth.RefreshToken
	token.ExpiresIn = auth.ExpiresIn
	if cl := newClient(auth.AccessToken, deviceID); cl != nil {
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

func httpMethodPost() string { return "POST" }

// ---- Share Import (importShare capability) ----

// shareInfoResp is the GET /drive/v1/share response.
type shareInfoResp struct {
	ShareStatus    string `json:"share_status"`
	PassCodeToken  string `json:"pass_code_token"`
	FileID         string `json:"file_id"`
	ID             string `json:"id"`
}

// shareDetailResp is the GET /drive/v1/share/detail response.
type shareDetailResp struct {
	Files []File `json:"files"`
	NextPageToken string `json:"next_page_token"`
}

// ImportShareSession implements drive.ShareImportDriver.
func (d *Driver) ImportShareSession(ctx context.Context, c drive.Context, shareURL, password string) (*drive.ShareImportSession, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	shareID, passCode := parsePikPakShareURL(shareURL)
	if password != "" {
		passCode = password
	}
	if shareID == "" {
		return nil, errors.New("pikpak: 无效的分享链接")
	}
	// Step 1: GET /drive/v1/share → pass_code_token
	q := url.Values{}
	q.Set("share_id", shareID)
	if passCode != "" {
		q.Set("pass_code", passCode)
	}
	var info shareInfoResp
	if err := cl.get(ctx, "/drive/v1/share", q, &info); err != nil {
		return nil, err
	}
	switch info.ShareStatus {
	case "PASS_CODE_EMPTY":
		return nil, errors.New("该分享需要访问密码")
	case "PASS_CODE_ERROR":
		return nil, errors.New("访问密码错误")
	}
	rootFileID := info.FileID
	if rootFileID == "" {
		rootFileID = info.ID
	}
	if rootFileID == "" {
		rootFileID = "root"
	}
	// Step 2: GET /drive/v1/share/detail → list files
	files, err := pikpakShareListAll(ctx, cl, shareID, info.PassCodeToken, rootFileID)
	if err != nil {
		return nil, err
	}
	return &drive.ShareImportSession{
		Provider:      providerID,
		ShareURL:      shareURL,
		ShareID:       shareID,
		Password:      passCode,
		PassCodeToken: info.PassCodeToken,
		RootFileID:    rootFileID,
		Files:         files,
	}, nil
}

func pikpakShareListAll(ctx context.Context, cl *client, shareID, passCodeToken, parentID string) ([]drive.ShareImportFile, error) {
	var out []drive.ShareImportFile
	marker := parentID
	seen := map[string]bool{}
	for {
		q := url.Values{}
		q.Set("share_id", shareID)
		q.Set("parent_id", marker)
		q.Set("pass_code_token", passCodeToken)
		q.Set("limit", "100")
		var resp shareDetailResp
		if err := cl.get(ctx, "/drive/v1/share/detail", q, &resp); err != nil {
			return nil, err
		}
		for i := range resp.Files {
			f := resp.Files[i]
			out = append(out, drive.ShareImportFile{
				FileID: f.ID,
				Name:   f.Name,
				Size:   f.Size,
				IsDir:  strings.Contains(f.Kind, "folder"),
			})
		}
		if resp.NextPageToken == "" {
			break
		}
		if seen[resp.NextPageToken] {
			break
		}
		seen[resp.NextPageToken] = true
		marker = resp.NextPageToken
	}
	return out, nil
}

// SaveShare implements drive.ShareImportDriver.
func (d *Driver) SaveShare(ctx context.Context, c drive.Context, session *drive.ShareImportSession, fileIDs []string, toParentID string) ([]string, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	parent := toParentID
	if parent == "" || parent == "root" || parent == RootID || parent == "*" {
		parent = ""
	}
	body := map[string]any{
		"share_id":       session.ShareID,
		"pass_code_token": session.PassCodeToken,
		"file_ids":        fileIDs,
		"parent_id":       parent,
	}
	if err := cl.jsonDo(ctx, http.MethodPost, "/drive/v1/share/restore", body, nil, nil); err != nil {
		return nil, err
	}
	return fileIDs, nil
}

// parsePikPakShareURL extracts share_id from a PikPak share URL.
func parsePikPakShareURL(raw string) (shareID, passCode string) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", ""
	}
	// https://mypikpak.com/drive/sharing/share/{shareID}?pass_code=xxx
	// or https://mypikpak.com/s/{shareID}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i, p := range parts {
		if p == "share" && i+1 < len(parts) {
			shareID = parts[i+1]
			break
		}
	}
	if shareID == "" && len(parts) > 0 {
		last := parts[len(parts)-1]
		if len(last) > 10 {
			shareID = last
		}
	}
	passCode = u.Query().Get("pass_code")
	if passCode == "" {
		passCode = u.Query().Get("passcode")
	}
	return shareID, passCode
}
// OfflineCreate submits a cloud offline task (magnet/link) to PikPak servers.
func (d *Driver) OfflineCreate(ctx context.Context, c drive.Context, url, fileName, parentID string) (taskID, fileID string, err error) {
	cl, err := clientOf(c)
	if err != nil {
		return "", "", err
	}
	return cl.OfflineCreate(ctx, url, fileName, parentID)
}

// OfflineList returns PikPak offline tasks.
func (d *Driver) OfflineList(ctx context.Context, c drive.Context) ([]OfflineTask, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	return cl.OfflineList(ctx)
}

// OfflineFind locates one task by task id or file id.
func (d *Driver) OfflineFind(ctx context.Context, c drive.Context, taskID, fileID string) (*OfflineTask, error) {
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	return cl.FindOfflineTask(ctx, taskID, fileID)
}
