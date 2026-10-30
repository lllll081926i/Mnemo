package thunder

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/drive/driveutil"
	"mnemo-go/internal/model"
)

type integer int64

func (value *integer) UnmarshalJSON(data []byte) error {
	text := strings.Trim(string(data), "\"")
	if text == "null" || text == "" {
		*value = 0
		return nil
	}
	parsed, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return err
	}
	if parsed < 0 {
		return errors.New("迅雷文件大小或进度不能为负数")
	}
	*value = integer(parsed)
	return nil
}

type file struct {
	ID           string    `json:"id"`
	Kind         string    `json:"kind"`
	ParentID     string    `json:"parent_id"`
	Name         string    `json:"name"`
	Hash         string    `json:"hash"`
	Phase        string    `json:"phase"`
	Size         integer   `json:"size"`
	ModifiedTime time.Time `json:"modified_time"`
	CreatedTime  time.Time `json:"created_time"`
	Trashed      bool      `json:"trashed"`
	DeleteTime   string    `json:"delete_time"`
	Thumbnail    string    `json:"thumbnail_link"`
	DownloadLink string    `json:"web_content_link"`
	Medias       []struct {
		ID     string `json:"media_id"`
		Name   string `json:"resolution_name"`
		Origin bool   `json:"is_origin"`
		Link   struct {
			URL string `json:"url"`
		} `json:"link"`
	} `json:"medias"`
}

func parentID(id string) string {
	if id == RootID || id == "root" {
		return ""
	}
	return id
}

func (entry file) model(driveID string) model.File {
	modified := entry.ModifiedTime
	if modified.IsZero() {
		modified = entry.CreatedTime
	}
	stamp := int64(0)
	if !modified.IsZero() {
		stamp = modified.Unix()
	}
	parent := entry.ParentID
	if parent == "" {
		parent = RootID
	}
	result := driveutil.NewFile(driveID, entry.ID, parent, entry.Name, entry.Kind == "drive#folder", int64(entry.Size), stamp)
	result.Thumbnail = entry.Thumbnail
	result.ContentHash, result.ContentHashName = entry.Hash, "gcid"
	return result
}

func (cl *client) filePage(ctx context.Context, dirID, marker string, trashed bool) (*struct {
	Files []file `json:"files"`
	Next  string `json:"next_page_token"`
}, error) {
	query := url.Values{"parent_id": {parentID(dirID)}, "page_token": {marker}, "limit": {"100"}, "space": {""},
		"__type": {"drive"}, "refresh": {"true"}, "__sync": {"true"}, "with_audit": {"true"},
		"filters": {`{"phase":{"eq":"PHASE_TYPE_COMPLETE"},"trashed":{"eq":false}}`},
	}
	if trashed {
		query.Del("parent_id")
		query.Set("filters", `{"trashed":{"eq":true}}`)
	}
	var result struct {
		Files []file `json:"files"`
		Next  string `json:"next_page_token"`
	}
	if err := cl.request(ctx, http.MethodGet, "/files?"+query.Encode(), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (*Driver) ListPaged(ctx context.Context, account drive.Context, dirID, marker string, opts *drive.ListOptions) (*drive.DirPage, error) {
	if opts != nil && opts.Search != "" {
		return nil, drive.ErrNotImplemented
	}
	result := &drive.DirPage{}
	err := withClient(ctx, account, func(cl *client) error {
		trashed := opts != nil && opts.Trashed
		page, err := cl.filePage(ctx, dirID, marker, trashed)
		if err != nil {
			return err
		}
		result.NextMarker = page.Next
		for _, entry := range page.Files {
			if entry.ID == "" || entry.Name == "" {
				return errors.New("迅雷列表包含缺少 ID 或名称的文件")
			}
			mapped := entry.model(account.DriveID)
			if trashed {
				if !entry.Trashed {
					return errors.New("迅雷回收站响应包含未删除文件，请刷新后重试")
				}
				if deleted, err := time.Parse(time.RFC3339, entry.DeleteTime); err == nil {
					mapped.Time, mapped.TimeStr = deleted.Unix(), deleted.Local().Format("2006-01-02 15:04:05")
				}
			}
			result.Items = append(result.Items, mapped)
		}
		return nil
	})
	return result, err
}

func (driver *Driver) List(ctx context.Context, account drive.Context, dirID string, opts *drive.ListOptions) ([]model.File, error) {
	var result []model.File
	marker := ""
	seen := make(map[string]bool)
	seenIDs := make(map[string]bool)
	for page := 0; page < 200; page++ {
		current, err := driver.ListPaged(ctx, account, dirID, marker, opts)
		if err != nil {
			return nil, err
		}
		for _, entry := range current.Items {
			if seenIDs[entry.FileID] {
				return nil, errors.New("迅雷文件分页包含重复文件，请刷新后重试")
			}
			seenIDs[entry.FileID] = true
			result = append(result, entry)
		}
		if current.NextMarker == "" {
			return result, nil
		}
		if seen[current.NextMarker] {
			return nil, errors.New("迅雷文件分页游标重复")
		}
		seen[current.NextMarker] = true
		marker = current.NextMarker
	}
	return nil, errors.New("迅雷文件分页超过安全上限")
}

func detail(ctx context.Context, cl *client, fileID string) (*file, error) {
	if strings.TrimSpace(fileID) == "" {
		return nil, errors.New("迅雷文件 ID 不能为空")
	}
	var result file
	if err := cl.request(ctx, http.MethodGet, "/files/"+url.PathEscape(fileID), nil, &result); err != nil {
		return nil, err
	}
	if result.ID != fileID {
		return nil, errors.New("迅雷详情返回的文件 ID 与请求不一致")
	}
	return &result, nil
}

func (*Driver) GetInfo(ctx context.Context, account drive.Context, fileID string) (any, error) {
	var result *file
	err := withClient(ctx, account, func(cl *client) error { var err error; result, err = detail(ctx, cl, fileID); return err })
	return result, err
}

func (driver *Driver) GetFile(ctx context.Context, account drive.Context, fileID string) (*model.File, error) {
	result, err := driver.GetInfo(ctx, account, fileID)
	if err != nil {
		return nil, err
	}
	entry := result.(*file)
	if entry.Phase != "" && entry.Phase != "PHASE_TYPE_COMPLETE" {
		return nil, errors.New("迅雷文件尚未完成，不能确认上传成功")
	}
	mapped := entry.model(account.DriveID)
	return &mapped, nil
}

func validLink(link string) bool {
	parsed, err := url.Parse(link)
	return err == nil && parsed.Host != "" && parsed.User == nil && (parsed.Scheme == "http" || parsed.Scheme == "https")
}

func downloadHeaders() map[string]string { return map[string]string{"User-Agent": downloadUA} }

func (driver *Driver) GetDownloadURL(ctx context.Context, account drive.Context, fileID string, _ int) (*model.DownloadURL, error) {
	result, err := driver.GetInfo(ctx, account, fileID)
	if err != nil {
		return nil, err
	}
	entry := result.(*file)
	link := entry.DownloadLink
	if !validLink(link) {
		for _, media := range entry.Medias {
			if media.Origin && validLink(media.Link.URL) {
				link = media.Link.URL
				break
			}
		}
	}
	if !validLink(link) {
		return nil, errors.New("迅雷未返回可用下载链接，文件可能尚未完成或账号无下载权限")
	}
	return &model.DownloadURL{DriveID: account.DriveID, FileID: fileID, URL: link, Size: int64(entry.Size), DownloadMode: "redirect", Headers: downloadHeaders(), RetrySingleConnection: true}, nil
}

func (driver *Driver) GetVideoPreview(ctx context.Context, account drive.Context, fileID string) (*model.VideoPreview, error) {
	result, err := driver.GetInfo(ctx, account, fileID)
	if err != nil {
		return nil, err
	}
	entry := result.(*file)
	preview := &model.VideoPreview{DriveID: account.DriveID, FileID: fileID, Size: int64(entry.Size), Headers: downloadHeaders()}
	for index, media := range entry.Medias {
		if !validLink(media.Link.URL) {
			continue
		}
		quality := fmt.Sprintf("media-%d", index)
		label := media.Name
		if label == "" {
			label = "视频流"
		}
		if media.Origin {
			quality, label = "origin", "原画"
		}
		preview.Qualities = append(preview.Qualities, model.VideoQuality{Quality: quality, Value: quality, Label: label, URL: media.Link.URL, Headers: downloadHeaders(), ForceProxy: true})
	}
	if len(preview.Qualities) == 0 && validLink(entry.DownloadLink) {
		preview.Qualities = []model.VideoQuality{{Quality: "origin", Value: "origin", Label: "原画", URL: entry.DownloadLink, Headers: downloadHeaders(), ForceProxy: true}}
	}
	if len(preview.Qualities) == 0 {
		return nil, errors.New("迅雷未返回可用播放链接")
	}
	return preview, nil
}

func (*Driver) RefreshAccount(ctx context.Context, account drive.Context, token *model.TokenInfo) (*model.TokenInfo, error) {
	account.Token = token
	err := withClient(ctx, account, func(cl *client) error {
		return cl.refreshQuota(ctx)
	})
	return token, err
}

func (*Driver) Mkdir(ctx context.Context, account drive.Context, parent, name string) (*drive.MkdirResult, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("文件夹名称不能为空")
	}
	var created file
	err := withClient(ctx, account, func(cl *client) error {
		return cl.request(ctx, http.MethodPost, "/files", map[string]string{"kind": "drive#folder", "name": name, "parent_id": parentID(parent), "space": ""}, &created)
	})
	if err != nil {
		return nil, err
	}
	if created.ID == "" {
		return nil, errors.New("迅雷新建目录响应缺少文件 ID")
	}
	return &drive.MkdirResult{FileID: created.ID}, nil
}

func (*Driver) Rename(ctx context.Context, account drive.Context, fileID, name string) (*drive.RenameResult, error) {
	if strings.TrimSpace(fileID) == "" || strings.TrimSpace(name) == "" {
		return nil, errors.New("文件 ID 和名称不能为空")
	}
	err := withClient(ctx, account, func(cl *client) error {
		return cl.mutate(ctx, http.MethodPatch, "/files/"+url.PathEscape(fileID), map[string]string{"name": name, "space": ""})
	})
	if err != nil {
		return nil, err
	}
	return &drive.RenameResult{FileID: fileID, Name: name}, nil
}

func (*Driver) Trash(ctx context.Context, account drive.Context, ids []string) ([]string, error) {
	if err := validateIDs(ids); err != nil {
		return nil, err
	}
	var done []string
	err := withClient(ctx, account, func(cl *client) error {
		for _, id := range ids {
			if err := cl.mutate(ctx, http.MethodPatch, "/files/"+url.PathEscape(id)+"/trash", map[string]any{}); err != nil {
				return err
			}
			done = append(done, id)
		}
		return nil
	})
	return done, err
}

func validateIDs(ids []string) error {
	if len(ids) == 0 {
		return errors.New("迅雷操作缺少文件或任务 ID")
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			return errors.New("迅雷文件或任务 ID 不能为空")
		}
		if seen[id] {
			return errors.New("迅雷文件或任务 ID 不能重复")
		}
		seen[id] = true
	}
	return nil
}

func batch(ctx context.Context, account drive.Context, refs []drive.FileRef, parent, action string) ([]string, error) {
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		ids = append(ids, ref.ID)
	}
	if err := validateIDs(ids); err != nil {
		return nil, err
	}
	err := withClient(ctx, account, func(cl *client) error {
		return cl.mutate(ctx, http.MethodPost, "/files:"+action, map[string]any{"ids": ids, "to": map[string]string{"parent_id": parentID(parent)}, "space": ""})
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (*Driver) Move(ctx context.Context, account drive.Context, refs []drive.FileRef, parent, _ string) ([]string, error) {
	return batch(ctx, account, refs, parent, "batchMove")
}

func (*Driver) Copy(ctx context.Context, account drive.Context, refs []drive.FileRef, parent, _ string) ([]string, error) {
	return batch(ctx, account, refs, parent, "batchCopy")
}
