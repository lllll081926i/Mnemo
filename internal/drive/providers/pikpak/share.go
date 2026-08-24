package pikpak

import (
	"context"
	"encoding/json"
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

type ShareResult struct {
	ShareID    string   `json:"share_id"`
	ShareURL   string   `json:"share_url"`
	PassCode   string   `json:"pass_code"`
	Expiration string   `json:"expiration"`
	FileIDs    []string `json:"file_ids"`
}

func (r *ShareResult) UnmarshalJSON(data []byte) error {
	var raw struct {
		ShareID    string   `json:"share_id"`
		ID         string   `json:"id"`
		ShareURL   string   `json:"share_url"`
		ShareLink  string   `json:"share_link"`
		PassCode   string   `json:"pass_code"`
		Passcode   string   `json:"passcode"`
		Expiration string   `json:"expiration"`
		FileIDs    []string `json:"file_ids"`
		FileIDList []string `json:"file_id_list"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	r.ShareID = firstNonEmpty(raw.ShareID, raw.ID)
	r.ShareURL = firstNonEmpty(raw.ShareURL, raw.ShareLink)
	r.PassCode = firstNonEmpty(raw.PassCode, raw.Passcode)
	r.Expiration = raw.Expiration
	r.FileIDs = raw.FileIDs
	if len(r.FileIDs) == 0 {
		r.FileIDs = raw.FileIDList
	}
	return nil
}

// CreateShare creates a share.
func (c *client) CreateShare(ctx context.Context, fileIDs []string, _ string, passCode string, expiration string) (*ShareResult, error) {
	shareTo := "publiclink"
	passCodeOption := "NOT_REQUIRED"
	if strings.TrimSpace(passCode) != "" {
		shareTo = "encryptedlink"
		passCodeOption = "REQUIRED"
	}
	body := map[string]any{
		"file_ids":         fileIDs,
		"share_to":         shareTo,
		"expiration_days":  pikpakExpirationDays(expiration),
		"pass_code_option": passCodeOption,
	}
	var res ShareResult
	if err := c.jsonDo(ctx, http.MethodPost, "/drive/v1/share", body, &res, nil); err != nil {
		return nil, err
	}
	if res.ShareID == "" || res.ShareURL == "" {
		return nil, errors.New("pikpak: 创建分享未返回有效链接")
	}
	return &res, nil
}

// DeleteShare revokes a PikPak share through the provider's batch endpoint.
// This is a single request and intentionally has no blind endpoint fallback:
// cancellation is destructive and must never be retried against a different
// endpoint after an ambiguous response.
func (c *client) DeleteShare(ctx context.Context, shareID string) error {
	shareID = strings.TrimSpace(shareID)
	if shareID == "" {
		return errors.New("pikpak: 分享标识为空")
	}
	return c.jsonDo(ctx, http.MethodPost, "/drive/v1/share:batchDelete", map[string]any{"ids": []string{shareID}}, nil, nil)
}

func pikpakExpirationDays(expiration string) int {
	value := strings.TrimSpace(expiration)
	if value == "" {
		return -1
	}
	if days, err := strconv.Atoi(value); err == nil && days > 0 {
		return days
	}
	target, err := time.Parse(time.RFC3339, value)
	if err != nil {
		target, err = time.Parse("2006-01-02", value)
	}
	if err != nil {
		return -1
	}
	remaining := time.Until(target)
	if remaining <= 0 {
		return 1
	}
	return int((remaining + 24*time.Hour - 1) / (24 * time.Hour))
}

// OfflineCreate submits an offline download task (magnet/link). PikPak uses
// the file creation API with UPLOAD_TYPE_URL; the /tasks?type=offline shape is
// not compatible with the legacy web client.

func (d *Driver) CreateShare(ctx context.Context, c drive.Context, params drive.ShareParams) (*model.ShareItem, error) {
	if len(params.FileIDs) == 0 {
		return nil, errors.New("pikpak: 创建分享至少选择一个文件")
	}
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	res, err := cl.CreateShare(ctx, params.FileIDs, params.ShareName, params.Password, params.Expiration)
	if err != nil {
		return nil, err
	}
	expiration := res.Expiration
	if expiration == "" {
		expiration = params.Expiration
	}
	fileIDs := res.FileIDs
	if len(fileIDs) == 0 {
		fileIDs = append([]string(nil), params.FileIDs...)
	}
	return &model.ShareItem{
		AccountID: c.UserID, DriveID: c.DriveID,
		ShareID: res.ShareID, ShareURL: res.ShareURL, SharePwd: res.PassCode,
		ShareName: params.ShareName, SharePolicy: "public", Expiration: expiration,
		FileID: fileIDs[0], FileIDList: fileIDs, ShareMsg: "创建成功",
	}, nil
}

// CancelShare invalidates the remote PikPak sharing link before the app drops
// its local history record.
func (d *Driver) CancelShare(ctx context.Context, c drive.Context, share model.ShareHistoryEntry) error {
	cl, err := clientOf(c)
	if err != nil {
		return err
	}
	return cl.DeleteShare(ctx, share.ShareID)
}

// ---- Share Import (importShare capability) ----

// shareInfoResp is the GET /drive/v1/share response.
type shareInfoResp struct {
	ShareStatus   string `json:"share_status"`
	PassCodeToken string `json:"pass_code_token"`
	FileID        string `json:"file_id"`
	ID            string `json:"id"`
}

// shareDetailResp is the GET /drive/v1/share/detail response.
type shareDetailResp struct {
	Files           []File `json:"files"`
	NextPageToken   string `json:"next_page_token"`
	ShareStatus     string `json:"share_status"`
	ShareStatusText string `json:"share_status_text"`
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
	pageToken := ""
	seen := map[string]bool{}
	for {
		q := url.Values{}
		q.Set("share_id", shareID)
		q.Set("parent_id", parentID)
		q.Set("pass_code_token", passCodeToken)
		q.Set("thumbnail_size", "SIZE_LARGE")
		q.Set("with_audit", "true")
		q.Set("limit", "100")
		q.Set("filters", `{"phase":{"eq":"PHASE_TYPE_COMPLETE"},"trashed":{"eq":false}}`)
		if pageToken != "" {
			q.Set("page_token", pageToken)
		}
		var resp shareDetailResp
		if err := cl.get(ctx, "/drive/v1/share/detail", q, &resp); err != nil {
			return nil, err
		}
		if resp.ShareStatus != "" && resp.ShareStatus != "OK" {
			if resp.ShareStatusText != "" {
				return nil, errors.New(resp.ShareStatusText)
			}
			return nil, fmt.Errorf("pikpak: 分享状态 %s", resp.ShareStatus)
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
			return nil, errors.New("pikpak: 分享文件分页游标重复")
		}
		seen[resp.NextPageToken] = true
		pageToken = resp.NextPageToken
	}
	return out, nil
}

// SaveShare implements drive.ShareImportDriver.
func (d *Driver) SaveShare(ctx context.Context, c drive.Context, session *drive.ShareImportSession, fileIDs []string, toParentID string) ([]string, error) {
	if session == nil || (session.Provider != "" && session.Provider != providerID) || strings.TrimSpace(session.ShareID) == "" || strings.TrimSpace(session.PassCodeToken) == "" {
		return nil, errors.New("pikpak: 分享会话无效")
	}
	if len(fileIDs) == 0 {
		return nil, errors.New("pikpak: 至少选择一个分享文件")
	}
	cl, err := clientOf(c)
	if err != nil {
		return nil, err
	}
	parent := toParentID
	if parent == "" || parent == "root" || parent == RootID || parent == "*" {
		parent = ""
	}
	body := map[string]any{
		"share_id":        session.ShareID,
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
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	// url.Parse treats a link without a scheme as a path. Normalize it so
	// `mypikpak.com/s/<id>` behaves the same as the copied HTTPS link.
	normalized := raw
	if !strings.Contains(normalized, "://") && !strings.HasPrefix(normalized, "//") {
		normalized = "https://" + normalized
	}
	u, err := url.Parse(normalized)
	if err != nil {
		return "", ""
	}
	query := u.Query()
	for _, key := range []string{"share_id", "shareId", "shareid"} {
		if value := strings.TrimSpace(query.Get(key)); value != "" {
			shareID = value
			break
		}
	}
	// https://mypikpak.com/drive/sharing/share/{shareID}?pass_code=xxx
	// https://mypikpak.com/s/{shareID}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := 0; i+1 < len(parts); i++ {
		switch strings.ToLower(strings.TrimSpace(parts[i])) {
		case "share", "s":
			if shareID == "" {
				shareID = strings.TrimSpace(parts[i+1])
			}
		}
	}
	for _, key := range []string{"pass_code", "passcode", "pwd", "password"} {
		if value := strings.TrimSpace(query.Get(key)); value != "" {
			passCode = value
			break
		}
	}
	return shareID, passCode
}

// OfflineCreate submits a cloud offline task (magnet/link) to PikPak servers.
