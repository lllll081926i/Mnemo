package gdrive

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

const providerID = model.ProviderGdrive

func init() {
	drive.Register(drive.Registration{
		ID: providerID, Meta: drive.GetMeta(providerID),
		Caps: drive.NewCapabilities(providerID, map[string]bool{
			"search": true, "favorite": true, "copy": false, "recycleBin": true,
			"permanentDelete": true, "trashView": true, "trashRestore": true, "trashPurge": true, "webEdit": true,
		}, func(c *drive.Capabilities) { c.SetHashes([]string{"md5", "sha1", "sha256"}, nil) }),
		Factory: func() drive.Driver { return &Driver{} }, Auth: authLogin,
		Login: drive.LoginConfig{
			HelpLabel: "打开 OpenList 公共授权页面", HelpURL: "https://api.oplist.org/",
			Notice: "在 OpenList 页面选择 Google Drive，使用服务端预设应用完成授权，再导入刷新令牌。文件传输需要能够访问 Google API。",
			Fields: []drive.LoginField{
				{Key: "refresh_token", Purpose: "token", Type: "password", Label: "刷新令牌", Required: true, Hint: "填写同一 OpenList 服务签发的 refresh_token，不要填写完整回调链接"},
				{Key: "api_url", Purpose: "endpoint", Type: "text", Label: "OpenList 刷新地址", Placeholder: "https://你的域名/googleui/renewapi", Hint: "先在对应自建服务完成授权；该服务必须已配置有效的 Google OAuth 应用"},
				{Key: "root_id", Purpose: "root", Type: "text", Label: "根目录 ID（可选）", Placeholder: "root", Hint: "留空使用我的云端硬盘，也可填写共享盘或文件夹 ID"},
			},
			Methods: []drive.LoginMethodSpec{
				{ID: "online", Label: "OpenList 在线授权", FieldKeys: []string{"refresh_token", "root_id"}, Recipient: defaultRefreshURL, Notice: "首次登录和后续刷新都会把刷新令牌发送到 OpenList 公共服务。Google 授权范围为完整 Drive 权限。", Consent: "我同意通过 OpenList 公共服务交换和刷新令牌"},
				{ID: "custom", Label: "自建 OpenList 服务", FieldKeys: []string{"refresh_token", "api_url", "root_id"}, RequiredKeys: []string{"api_url"}, Recipient: "上方填写的 OpenList 刷新地址", Notice: "首次登录和后续刷新都会把刷新令牌发送到填写的服务。请在同一可信服务获取令牌。", Consent: "我信任填写的 OpenList 服务，并同意向该服务发送刷新令牌"},
			},
		},
	})
}

type Driver struct{ drive.BaseDriver }

var _ drive.Driver = (*Driver)(nil)
var _ drive.RemoteFavorites = (*Driver)(nil)
var _ drive.FreshFileDriver = (*Driver)(nil)

func (*Driver) ID() string                       { return providerID }
func (*Driver) Meta() drive.Meta                 { return drive.GetMeta(providerID) }
func (*Driver) Capabilities() drive.Capabilities { return drive.RegistryCaps(providerID) }
func (*Driver) RootID() string                   { return "root" }

func authLogin(ctx context.Context, req drive.AuthRequest) (*model.TokenInfo, error) {
	if req.Config["relay_consent"] != "true" {
		return nil, errors.New("请先同意 OpenList 令牌接收方说明")
	}
	endpoint := defaultRefreshURL
	if req.Config["login_method"] == "custom" {
		if strings.TrimSpace(req.Config["api_url"]) == "" {
			return nil, errors.New("请填写自建 OpenList 刷新地址")
		}
		endpoint = strings.TrimSpace(req.Config["api_url"])
	} else if req.Config["login_method"] != "" && req.Config["login_method"] != "online" {
		return nil, errors.New("Google Drive 登录方式无效")
	}
	endpoint, err := refreshEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	refresh := strings.TrimSpace(req.Config["refresh_token"])
	if refresh == "" || strings.ContainsAny(refresh, "\r\n\t #") || strings.Contains(refresh, "://") {
		return nil, errors.New("请填写有效的刷新令牌，不要填写回调链接")
	}
	rootID := strings.TrimSpace(req.Config["root_id"])
	if rootID == "" {
		rootID = "root"
	}
	if !validID(rootID) {
		return nil, errors.New("Google Drive 根目录 ID 无效")
	}
	token := &model.TokenInfo{TokenFrom: providerID, RefreshToken: refresh, OAuthRefreshURL: endpoint, ProviderRootID: rootID}
	if err := renew(ctx, token); err != nil {
		return nil, err
	}
	c := drive.Context{Token: token, TokenFrom: providerID}
	cl, err := newClient(c)
	if err != nil {
		return nil, err
	}
	if err := cl.profile(ctx); err != nil {
		return nil, err
	}
	rootItem, err := cl.detail(ctx, rootID)
	if err != nil {
		return nil, fmt.Errorf("无法打开 Google Drive 根目录：%w", err)
	}
	if rootItem.MIME != folderMIME {
		return nil, errors.New("Google Drive 根目录必须是文件夹或共享盘")
	}
	token.ProviderRootID = rootItem.ID
	token.ProviderSpaceID = rootItem.DriveID
	token.UserID = model.BuildUserID(providerID, token.ProviderAccountID)
	token.DefaultDriveID = model.BuildDriveID(providerID, token.ProviderAccountID)
	return token, nil
}

func (cl *client) profile(ctx context.Context) error {
	var result struct {
		User struct {
			ID     string `json:"permissionId"`
			Name   string `json:"displayName"`
			Email  string `json:"emailAddress"`
			Avatar string `json:"photoLink"`
		} `json:"user"`
		Quota struct {
			Limit string `json:"limit"`
			Usage string `json:"usage"`
		} `json:"storageQuota"`
	}
	if err := cl.json(ctx, http.MethodGet, "/about", url.Values{"fields": {"user(permissionId,displayName,emailAddress,photoLink),storageQuota(limit,usage)"}}, nil, &result); err != nil {
		return err
	}
	if result.User.ID == "" {
		return errors.New("Google Drive 未返回稳定的账号标识")
	}
	token := cl.c.Token
	if token.ProviderAccountID != "" && token.ProviderAccountID != result.User.ID {
		return errors.New("Google Drive 授权账号与原账号不一致，请重新添加账号")
	}
	token.ProviderAccountID = result.User.ID
	token.Name, token.NickName, token.UserName, token.Avatar = result.User.Name, result.User.Name, result.User.Email, result.User.Avatar
	token.TotalSize, token.UsedSize, token.FreeSize = 0, 0, 0
	if result.Quota.Usage != "" {
		n, err := number(result.Quota.Usage)
		if err != nil {
			return err
		}
		token.UsedSize = n
	}
	if result.Quota.Limit != "" {
		n, err := number(result.Quota.Limit)
		if err != nil {
			return err
		}
		token.TotalSize = n
		token.FreeSize = max(0, n-token.UsedSize)
	}
	return nil
}

func (*Driver) RefreshAccount(ctx context.Context, c drive.Context, token *model.TokenInfo) (*model.TokenInfo, error) {
	c.Token = token
	cl, err := newClient(c)
	if err != nil {
		return nil, err
	}
	if err := cl.profile(ctx); err != nil {
		return nil, err
	}
	return token, nil
}

func mapItem(c drive.Context, raw item) model.File {
	parent := ""
	if len(raw.Parents) > 0 {
		parent = raw.Parents[0]
	}
	tm, _ := time.Parse(time.RFC3339Nano, raw.Modified)
	if tm.IsZero() {
		tm, _ = time.Parse(time.RFC3339Nano, raw.Created)
	}
	stamp := int64(0)
	if !tm.IsZero() {
		stamp = tm.UnixMilli()
	}
	size := int64(-1)
	if raw.Size != nil {
		size = *raw.Size
	}
	isDir := raw.MIME == folderMIME || raw.Shortcut != nil && raw.Shortcut.MIME == folderMIME
	if isDir {
		size = 0
	}
	f := model.File{DriveID: c.DriveID, FileID: raw.ID, ParentFileID: parent, Name: raw.Name, NameSearch: strings.ToLower(raw.Name), MimeType: raw.MIME, Size: size, Time: stamp, Starred: raw.Starred, IsDir: isDir, Ext: strings.TrimPrefix(strings.ToLower(filepath.Ext(raw.Name)), ".")}
	if size >= 0 {
		f.SizeStr = model.FormatBytes(size)
	} else {
		f.SizeStr = "—"
	}
	if raw.MD5 != "" {
		f.ContentHash, f.ContentHashName = raw.MD5, "md5"
	} else if raw.SHA1 != "" {
		f.ContentHash, f.ContentHashName = raw.SHA1, "sha1"
	}
	return f
}

func (cl *client) page(ctx context.Context, query, marker string) (*drive.DirPage, error) {
	q := url.Values{"q": {query}, "pageSize": {"1000"}, "fields": {"files(" + fileFields + "),nextPageToken,incompleteSearch"}, "orderBy": {"folder,name_natural"}, "includeItemsFromAllDrives": {"true"}}
	if marker != "" {
		q.Set("pageToken", marker)
	}
	if cl.c.Token.ProviderSpaceID != "" {
		q.Set("corpora", "drive")
		q.Set("driveId", cl.c.Token.ProviderSpaceID)
	} else {
		q.Set("corpora", "user")
	}
	var raw struct {
		Files      []item `json:"files"`
		Next       string `json:"nextPageToken"`
		Incomplete bool   `json:"incompleteSearch"`
	}
	if err := cl.json(ctx, http.MethodGet, "/files", q, nil, &raw); err != nil {
		return nil, err
	}
	if raw.Files == nil || raw.Incomplete {
		return nil, errors.New("Google Drive 未返回完整文件列表，请刷新后重试")
	}
	page := &drive.DirPage{Items: make([]model.File, 0, len(raw.Files)), NextMarker: raw.Next}
	for _, f := range raw.Files {
		if !validID(f.ID) || f.Name == "" {
			return nil, errors.New("Google Drive 列表项缺少文件 ID 或名称")
		}
		page.Items = append(page.Items, mapItem(cl.c, f))
	}
	return page, nil
}

func (cl *client) listAll(ctx context.Context, query string) ([]model.File, error) {
	files := []model.File{}
	marker := ""
	seen := map[string]bool{}
	for {
		if seen[marker] {
			return nil, errors.New("Google Drive 分页游标重复，文件列表未完整返回")
		}
		seen[marker] = true
		page, err := cl.page(ctx, query, marker)
		if err != nil {
			return nil, err
		}
		files = append(files, page.Items...)
		if page.NextMarker == "" {
			return files, nil
		}
		marker = page.NextMarker
	}
}

func (cl *client) folder(ctx context.Context, id string) (string, error) {
	id = root(cl.c, id)
	f, err := cl.detail(ctx, id)
	if err != nil {
		return "", err
	}
	if f.Shortcut != nil && f.Shortcut.MIME == folderMIME {
		if f.Shortcut.ResourceKey != "" {
			cl.resourceKeys[f.Shortcut.TargetID] = f.Shortcut.ResourceKey
		}
		target, err := cl.detail(ctx, f.Shortcut.TargetID)
		if err != nil {
			return "", err
		}
		if target.MIME != folderMIME {
			return "", errors.New("Google Drive 快捷方式目标不是文件夹")
		}
		return target.ID, nil
	}
	if f.MIME != folderMIME {
		return "", errors.New("Google Drive 目标不是文件夹")
	}
	return f.ID, nil
}

func (d *Driver) ListPaged(ctx context.Context, c drive.Context, dirID, marker string, opts *drive.ListOptions) (*drive.DirPage, error) {
	cl, err := newClient(c)
	if err != nil {
		return nil, err
	}
	if opts != nil && opts.Trashed {
		return cl.page(ctx, "trashed=true", marker)
	}
	id, err := cl.folder(ctx, dirID)
	if err != nil {
		return nil, err
	}
	q := quote(id) + " in parents and trashed=false"
	if opts != nil && opts.Search != "" {
		q += " and name contains " + quote(opts.Search)
	}
	return cl.page(ctx, q, marker)
}

func (d *Driver) List(ctx context.Context, c drive.Context, dirID string, opts *drive.ListOptions) ([]model.File, error) {
	cl, err := newClient(c)
	if err != nil {
		return nil, err
	}
	if opts != nil && opts.Trashed {
		return cl.listAll(ctx, "trashed=true")
	}
	id, err := cl.folder(ctx, dirID)
	if err != nil {
		return nil, err
	}
	q := quote(id) + " in parents and trashed=false"
	if opts != nil && opts.Search != "" {
		q += " and name contains " + quote(opts.Search)
	}
	return cl.listAll(ctx, q)
}

func (*Driver) ListTrash(ctx context.Context, c drive.Context, _ *drive.ListOptions) ([]model.File, error) {
	cl, err := newClient(c)
	if err != nil {
		return nil, err
	}
	return cl.listAll(ctx, "trashed=true")
}
func (*Driver) Search(ctx context.Context, c drive.Context, word string) ([]model.File, error) {
	cl, err := newClient(c)
	if err != nil {
		return nil, err
	}
	return cl.listAll(ctx, "trashed=false and name contains "+quote(word))
}
func (*Driver) GetInfo(ctx context.Context, c drive.Context, id string) (any, error) {
	cl, err := newClient(c)
	if err != nil {
		return nil, err
	}
	return cl.detail(ctx, root(c, id))
}
func (*Driver) GetFile(ctx context.Context, c drive.Context, id string) (*model.File, error) {
	cl, err := newClient(c)
	if err != nil {
		return nil, err
	}
	f, err := cl.detail(ctx, root(c, id))
	if err != nil {
		return nil, err
	}
	mapped := mapItem(c, *f)
	return &mapped, nil
}
func (d *Driver) GetFileFresh(ctx context.Context, c drive.Context, id string) (*model.File, error) {
	return d.GetFile(ctx, c, id)
}

func (*Driver) GetDownloadURL(ctx context.Context, c drive.Context, id string, _ int) (*model.DownloadURL, error) {
	cl, err := newClient(c)
	if err != nil {
		return nil, err
	}
	f, err := cl.detail(ctx, id)
	if err != nil {
		return nil, err
	}
	resourceKey := f.ResourceKey
	if f.Shortcut != nil {
		resourceKey = f.Shortcut.ResourceKey
		if resourceKey != "" {
			cl.resourceKeys[f.Shortcut.TargetID] = resourceKey
		}
		f, err = cl.detail(ctx, f.Shortcut.TargetID)
		if err != nil {
			return nil, err
		}
	}
	if resourceKey == "" {
		resourceKey = f.ResourceKey
	}
	if strings.HasPrefix(f.MIME, "application/vnd.google-apps.") {
		return nil, drive.NotSupported("Google 原生文档请使用网页打开；文件夹不能直接下载")
	}
	if f.Size == nil || *f.Size < 0 {
		return nil, errors.New("Google Drive 未返回可验证的文件大小")
	}
	if f.Capabilities.CanDownload != nil && !*f.Capabilities.CanDownload {
		return nil, errors.New("文件所有者或组织策略禁止下载此 Google Drive 文件")
	}
	if err := cl.refresh(ctx, c.Token.AccessToken); err != nil {
		return nil, err
	}
	target := apiHost + apiPath + "/files/" + f.ID + "?alt=media&supportsAllDrives=true"
	header := map[string]string{"Authorization": "Bearer " + c.Token.AccessToken}
	if resourceKey != "" {
		header["X-Goog-Drive-Resource-Keys"] = f.ID + "/" + resourceKey
	}
	authContext := c
	authContext.Token = drive.CloneToken(c.Token)
	authContext.TokenSnapshot = drive.CloneToken(c.Token)
	authClient, _ := newClient(authContext)
	var mu sync.Mutex
	lastRefresh := time.Now()
	requestAuth := func(req *http.Request) error {
		if !trustedGoogleURL(req.URL.String()) {
			return errors.New("Google Drive 下载鉴权目标无效")
		}
		mu.Lock()
		defer mu.Unlock()
		if err := drive.ReloadToken(authContext); err != nil {
			return err
		}
		// 在线服务不返回有效期；长任务按固定策略预刷新，不伪造令牌过期时间。
		if time.Since(lastRefresh) >= 10*time.Minute {
			if err := authClient.refresh(req.Context(), authContext.Token.AccessToken); err != nil {
				return err
			}
			lastRefresh = time.Now()
		}
		req.Header.Set("Authorization", "Bearer "+authContext.Token.AccessToken)
		return nil
	}
	return &model.DownloadURL{DriveID: c.DriveID, FileID: id, URL: target, Size: *f.Size, Headers: header, RequestAuth: requestAuth, DownloadMode: "proxy", ForceLocalProxy: true}, nil
}

func (*Driver) WebEditURL(ctx context.Context, c drive.Context, id string) (string, error) {
	cl, err := newClient(c)
	if err != nil {
		return "", err
	}
	f, err := cl.detail(ctx, id)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(f.WebURL)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || (u.Hostname() != "drive.google.com" && u.Hostname() != "docs.google.com") {
		return "", errors.New("Google Drive 未返回可信的网页地址")
	}
	return u.String(), nil
}

func (*Driver) Mkdir(ctx context.Context, c drive.Context, parent, name string) (*drive.MkdirResult, error) {
	cl, err := newClient(c)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("文件夹名称不能为空")
	}
	id, err := cl.folder(ctx, parent)
	if err != nil {
		return nil, err
	}
	var f item
	if err := cl.json(ctx, http.MethodPost, "/files", url.Values{"fields": {fileFields}}, map[string]any{"name": name, "mimeType": folderMIME, "parents": []string{id}}, &f); err != nil {
		return nil, err
	}
	if !validID(f.ID) || f.MIME != folderMIME {
		return nil, errors.New("Google Drive 未确认文件夹创建成功")
	}
	return &drive.MkdirResult{FileID: f.ID}, nil
}

func (*Driver) Rename(ctx context.Context, c drive.Context, id, name string) (*drive.RenameResult, error) {
	cl, err := newClient(c)
	if err != nil {
		return nil, err
	}
	if !validID(id) || strings.TrimSpace(name) == "" {
		return nil, errors.New("文件 ID 或名称无效")
	}
	var f item
	if err := cl.json(ctx, http.MethodPatch, "/files/"+id, url.Values{"fields": {fileFields}}, map[string]any{"name": name}, &f); err != nil {
		return nil, err
	}
	if f.ID != id || f.Name != name {
		return nil, errors.New("Google Drive 未确认重命名结果")
	}
	parent := ""
	if len(f.Parents) > 0 {
		parent = f.Parents[0]
	}
	return &drive.RenameResult{FileID: id, ParentFileID: parent, Name: name, IsDir: f.MIME == folderMIME}, nil
}

func batch(ctx context.Context, ids []string, op func(string) error) ([]string, error) {
	done := []string{}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return done, err
		}
		if !validID(id) || id == "root" {
			return done, errors.New("Google Drive 操作目标无效")
		}
		if err := op(id); err != nil {
			return done, err
		}
		done = append(done, id)
	}
	return done, nil
}

func changeFlag(ctx context.Context, c drive.Context, ids []string, flag string, value bool) ([]string, error) {
	cl, err := newClient(c)
	if err != nil {
		return nil, err
	}
	return batch(ctx, ids, func(id string) error {
		var f struct {
			ID      string `json:"id"`
			Trashed bool   `json:"trashed"`
			Starred bool   `json:"starred"`
		}
		if err := cl.json(ctx, http.MethodPatch, "/files/"+id, url.Values{"fields": {"id,trashed,starred"}}, map[string]bool{flag: value}, &f); err != nil {
			return err
		}
		if f.ID != id || flag == "trashed" && f.Trashed != value || flag == "starred" && f.Starred != value {
			return errors.New("Google Drive 未确认文件状态变更")
		}
		return nil
	})
}

func (*Driver) Trash(ctx context.Context, c drive.Context, ids []string) ([]string, error) {
	return changeFlag(ctx, c, ids, "trashed", true)
}
func (*Driver) Restore(ctx context.Context, c drive.Context, ids []string) ([]string, error) {
	return changeFlag(ctx, c, ids, "trashed", false)
}
func (*Driver) Favorite(ctx context.Context, c drive.Context, ids []string, value bool) ([]string, error) {
	return changeFlag(ctx, c, ids, "starred", value)
}
func (*Driver) SupportsRemoteFavorites(context.Context, drive.Context) (bool, error) {
	return true, nil
}
func (*Driver) ListFavorites(ctx context.Context, c drive.Context) ([]model.File, error) {
	cl, err := newClient(c)
	if err != nil {
		return nil, err
	}
	return cl.listAll(ctx, "trashed=false and starred=true")
}
func (*Driver) Delete(ctx context.Context, c drive.Context, refs []drive.FileRef) ([]string, error) {
	cl, err := newClient(c)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		ids = append(ids, ref.ID)
	}
	return batch(ctx, ids, func(id string) error { return cl.json(ctx, http.MethodDelete, "/files/"+id, nil, nil, nil) })
}
func (*Driver) Move(ctx context.Context, c drive.Context, refs []drive.FileRef, parent, _ string) ([]string, error) {
	cl, err := newClient(c)
	if err != nil {
		return nil, err
	}
	dst, err := cl.folder(ctx, parent)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		ids = append(ids, ref.ID)
	}
	return batch(ctx, ids, func(id string) error {
		f, err := cl.detail(ctx, id)
		if err != nil {
			return err
		}
		for _, p := range f.Parents {
			if p == dst {
				return nil
			}
		}
		if len(f.Parents) == 0 {
			return errors.New("Google Drive 未返回源父目录，无法安全移动")
		}
		var moved item
		err = cl.json(ctx, http.MethodPatch, "/files/"+id, url.Values{"fields": {fileFields}, "addParents": {dst}, "removeParents": {strings.Join(f.Parents, ",")}}, map[string]any{}, &moved)
		if err != nil {
			return err
		}
		if moved.ID != id || len(moved.Parents) != 1 || moved.Parents[0] != dst {
			return errors.New("Google Drive 未确认移动结果")
		}
		return nil
	})
}
func (*Driver) Copy(context.Context, drive.Context, []drive.FileRef, string, string) ([]string, error) {
	return nil, drive.NotSupported("Google Drive 同盘复制")
}
func (*Driver) ResolveTransferHash(ctx context.Context, c drive.Context, id, method string, _ bool) (string, error) {
	cl, err := newClient(c)
	if err != nil {
		return "", err
	}
	f, err := cl.detail(ctx, id)
	if err != nil {
		return "", err
	}
	switch method {
	case "md5":
		return f.MD5, nil
	case "sha1":
		return f.SHA1, nil
	case "sha256":
		return f.SHA256, nil
	}
	return "", nil
}
