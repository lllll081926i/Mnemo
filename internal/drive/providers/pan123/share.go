package pan123

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
	"mnemo-go/internal/model"
)

// ---- share (legacy share.ts) ----

// formatPan123Expiration normalizes a selected expiration for the current
// /a/api/share/create contract. An empty selection means a long-lived link;
// an invalid non-empty value remains empty so CreateShare can reject it before
// sending a malformed request.
func formatPan123Expiration(value string) string {
	if strings.TrimSpace(value) == "" {
		return pan123PermanentShareExpiration
	}
	t := parseFlexibleTime(value)
	if t.IsZero() {
		return ""
	}
	return t.In(time.FixedZone("CST", 8*60*60)).Format(time.RFC3339)
}

// formatPan123Time renders a time in local wall clock yyyy-MM-dd HH:mm:ss.
func formatPan123Time(t time.Time) string {
	return t.Local().Format("2006-01-02 15:04:05")
}

// parseLocalTime accepts ISO-8601 and common local layouts. Zone-less strings
// are parsed in the local location to mirror JS Date parsing (new Date(str)).
func parseLocalTime(value string) time.Time {
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
	} {
		var (
			t   time.Time
			err error
		)
		if layout == time.RFC3339 {
			t, err = time.Parse(layout, value)
		} else {
			t, err = time.ParseInLocation(layout, value, time.Local)
		}
		if err == nil {
			return t
		}
	}
	return time.Time{}
}

// parseFlexibleTime accepts ISO-8601 and common local layouts.
func parseFlexibleTime(value string) time.Time {
	return parseLocalTime(value)
}

func (d *Driver) CreateShare(ctx context.Context, c drive.Context, params drive.ShareParams) (*model.ShareItem, error) {
	if len(params.FileIDs) == 0 {
		return nil, errors.New("创建分享失败：至少选择一个文件")
	}
	shareName := params.ShareName
	if shareName == "" {
		shareName = "分享文件"
	}
	fileIDs := make([]string, 0, len(params.FileIDs))
	for _, id := range params.FileIDs {
		fileID := toPan123Number(id)
		if fileID <= 0 {
			return nil, fmt.Errorf("创建分享失败：无效文件 ID %q", id)
		}
		fileIDs = append(fileIDs, strconv.FormatInt(fileID, 10))
	}
	expiration := formatPan123Expiration(params.Expiration)
	if expiration == "" {
		return nil, errors.New("创建分享失败：有效期格式无效")
	}
	body := map[string]any{
		"driveId":    0,
		"expiration": expiration,
		"fileIdList": strings.Join(fileIDs, ","),
		"shareName":  shareName,
		"sharePwd":   params.Password,
		"event":      "shareCreate",
	}
	resp, err := d.api(ctx, c, http.MethodPost, apiShareURL, body, nil)
	if err != nil {
		return nil, err
	}
	data := parseMap(resp.Data)
	shareKey := firstString(data, "ShareKey", "ShareId")
	shareURL := firstString(data, "ShareUrl", "shareUrl")
	if shareURL == "" && shareKey != "" {
		shareURL = "https://www.123pan.com/s/" + shareKey
	}
	if shareURL == "" {
		return nil, errors.New("创建分享失败：未返回链接")
	}
	pwd := firstString(data, "SharePwd", "sharePwd")
	if pwd == "" {
		pwd = params.Password
	}
	item := &model.ShareItem{
		AccountID:   c.UserID,
		ShareID:     shareKey,
		ShareURL:    shareURL,
		SharePwd:    pwd,
		ShareName:   shareName,
		Expiration:  params.Expiration,
		DriveID:     c.DriveID,
		FileID:      params.FileIDs[0],
		FileIDList:  params.FileIDs,
		SharePolicy: "public",
		ShareMsg:    "创建成功",
	}
	return item, nil
}

// ---- Share Import (importShare capability) ----

// ImportShareSession implements drive.ShareImportDriver.
func (d *Driver) ImportShareSession(ctx context.Context, c drive.Context, shareURL, password string) (*drive.ShareImportSession, error) {
	shareKey, sharePwd := parsePan123ShareURL(shareURL)
	if password != "" {
		sharePwd = password
	}
	if shareKey == "" {
		return nil, errors.New("123: 无效的分享链接")
	}
	files, err := pan123ShareList(ctx, d, c, shareKey, sharePwd, "0")
	if err != nil {
		return nil, err
	}
	return &drive.ShareImportSession{
		Provider: providerID,
		ShareURL: shareURL,
		ShareID:  shareKey,
		ShareKey: shareKey,
		Password: sharePwd,
		Files:    files,
	}, nil
}

func pan123ShareList(ctx context.Context, d *Driver, c drive.Context, shareKey, sharePwd, parentFileID string) ([]drive.ShareImportFile, error) {
	query := map[string]string{
		"limit":          "100",
		"next":           "0",
		"orderBy":        "file_name",
		"orderDirection": "desc",
		"parentFileId":   parentFileID,
		"Page":           "1",
		"shareKey":       shareKey,
		"SharePwd":       sharePwd,
	}
	resp, err := d.api(ctx, c, http.MethodGet, apiShareGet, nil, query)
	if err != nil {
		return nil, err
	}
	data := parseMap(resp.Data)
	list := rawList(data)
	var out []drive.ShareImportFile
	for _, raw := range list {
		if m, ok := raw.(map[string]any); ok {
			f := normalizePan123File(m)
			if f.FileID == "" {
				continue
			}
			out = append(out, drive.ShareImportFile{
				FileID: f.FileID,
				Name:   f.FileName,
				Size:   f.Size,
				IsDir:  f.Type == 1,
			})
		}
	}
	return out, nil
}

// SaveShare implements drive.ShareImportDriver.
func (d *Driver) SaveShare(ctx context.Context, c drive.Context, session *drive.ShareImportSession, fileIDs []string, toParentID string) ([]string, error) {
	if session == nil || (session.Provider != "" && session.Provider != providerID) || strings.TrimSpace(session.ShareKey) == "" {
		return nil, errors.New("123: 分享会话无效")
	}
	if len(fileIDs) == 0 {
		return nil, errors.New("123: 至少选择一个分享文件")
	}
	parent := toPan123Number(toParentID)
	fileIDList := make([]any, 0, len(fileIDs))
	for _, id := range fileIDs {
		fileIDList = append(fileIDList, map[string]any{"fileId": toPan123Number(id)})
	}
	body := map[string]any{
		"fileIdList":   fileIDList,
		"targetFileId": parent,
		"event":        "shareTransfer",
		"shareKey":     session.ShareKey,
		"sharePwd":     session.Password,
	}
	if _, err := d.api(ctx, c, http.MethodPost, apiFileAsync, body, nil); err != nil {
		return nil, err
	}
	return fileIDs, nil
}

// parsePan123ShareURL extracts shareKey from a 123 pan share URL.
func parsePan123ShareURL(raw string) (shareKey, sharePwd string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	if !strings.Contains(raw, "://") && !strings.HasPrefix(raw, "//") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", ""
	}
	// https://www.123pan.com/s/{shareKey}.html
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) >= 2 && parts[0] == "s" {
		shareKey = strings.TrimSuffix(parts[1], ".html")
	}
	if shareKey == "" && len(parts) > 0 {
		last := strings.TrimSuffix(parts[len(parts)-1], ".html")
		if len(last) > 6 {
			shareKey = last
		}
	}
	sharePwd = u.Query().Get("pwd")
	if sharePwd == "" {
		sharePwd = u.Query().Get("SharePwd")
	}
	return shareKey, sharePwd
}
