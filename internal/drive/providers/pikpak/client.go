// Package pikpak implements the PikPak provider (api-drive.mypikpak.com).
package pikpak

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

const (
	apiHost    = "https://api-drive.mypikpak.com"
	userHost   = "https://user.mypikpak.com"
	RootID     = "pikpak_root"

	clientID      = "YUMx5nI8ZU8Ap8pm"
	clientVersion = "2.0.0"
	packageName   = "mypikpak.com"
	redirectURI   = "xlaccsdk01://xbase.cloud/callback?state=harbor"
	userAgent     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:129.0) Gecko/20100101 Firefox/129.0"
)

// captchaSalts mirrors the reference client's salt chain for captcha_sign.
var captchaSalts = []string{
	"C9qPpZLN8ucRTaTiUMWYS9cQvWOE", "+r6CQVxjzJV6LCV", "F", "pFJRC",
	"9WXYIDGrwTCz2OiVlgZa90qpECPD6olt", "/750aCr4lm/Sly/c", "RB+DT/gZCrbV", "",
	"CyLsf7hdkIRxRm215hl", "7xHvLi2tOYP0Y92b", "ZGTXXxu8E/MIWaEDB+Sm/", "1UI3",
	"E7fP5Pfijd+7K+t6Tg/NhuLq0eEUVChpJSkrKxpO", "ihtqpG6FMt65+Xk+tWUH2", "NhXXU9rg4XXdzo7u5o",
}

func md5hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

// captchaSign builds the 1.<md5-chain> signature.
func captchaSign(deviceID, timestamp string) string {
	sign := clientID + clientVersion + packageName + deviceID + timestamp
	for _, salt := range captchaSalts {
		sign = md5hex(sign + salt)
	}
	return "1." + sign
}

// CaptchaMeta is the meta object sent to captcha/init.
type CaptchaMeta struct {
	CaptchaSign   string `json:"captcha_sign"`
	ClientVersion string `json:"client_version"`
	PackageName   string `json:"package_name"`
	Timestamp     string `json:"timestamp"`
	Email         string `json:"email,omitempty"`
	PhoneNumber   string `json:"phone_number,omitempty"`
	Username      string `json:"username,omitempty"`
}

func loginCaptchaMeta(username string) CaptchaMeta {
	u := strings.TrimSpace(username)
	meta := CaptchaMeta{ClientVersion: clientVersion, PackageName: packageName}
	if strings.Contains(u, "@") && strings.Contains(u, ".") {
		meta.Email = u
	} else if isPhone(u) {
		meta.PhoneNumber = strings.ReplaceAll(strings.ReplaceAll(u, " ", ""), "-", "")
	} else {
		meta.Username = u
	}
	return meta
}

func isPhone(s string) bool {
	if len(s) < 6 || len(s) > 18 {
		return false
	}
	for _, r := range s {
		if r == '+' {
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// client is an authenticated PikPak session.
type client struct {
	http       *netx.Client
	accessToken string
	deviceID   string
}

func newClient(accessToken, deviceID string) *client {
	return &client{http: netx.NewClient(90 * time.Second), accessToken: accessToken, deviceID: deviceID}
}

func (c *client) headers(extra map[string]string) map[string]string {
	h := map[string]string{
		"User-Agent":       userAgent,
		"X-Device-Id":      c.deviceID,
		"X-Client-Version": clientVersion,
	}
	if c.accessToken != "" {
		h["Authorization"] = "Bearer " + c.accessToken
	}
	for k, v := range extra {
		h[k] = v
	}
	return h
}

// jsonDo performs an authenticated JSON call against the drive API.
func (c *client) jsonDo(ctx context.Context, method, path string, body any, out any, extra map[string]string) error {
	target := apiHost + path
	var reader io.Reader
	if body != nil {
		reader = netx.JSONBody(body)
	}
	resp, err := c.http.Do(ctx, method, target, c.headers(extra), reader)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return parseAPIError(data, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

// get performs a GET with query params.
func (c *client) get(ctx context.Context, path string, q url.Values, out any) error {
	target := apiHost + path
	if q != nil {
		target += "?" + q.Encode()
	}
	resp, err := c.http.Do(ctx, http.MethodGet, target, c.headers(nil), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return parseAPIError(data, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

func parseAPIError(data []byte, status int) error {
	var e struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
		Message          string `json:"message"`
		Reason           string `json:"reason"`
		Code             int    `json:"code"`
	}
	_ = json.Unmarshal(data, &e)
	// 常见错误友好化（对齐旧版 parsePikPakError）
	switch e.Error {
	case "invalid_account_or_password":
		return errors.New("PikPak 账号或密码错误")
	case "captcha_invalid", "captcha_required":
		return errors.New("PikPak 验证失败，请重试登录")
	}
	msg := e.ErrorDescription
	if msg == "" {
		msg = e.Message
	}
	if msg == "" {
		msg = e.Error
	}
	if msg == "" {
		msg = fmt.Sprintf("pikpak: http %d", status)
	}
	if e.Reason != "" {
		msg = e.Reason + ": " + msg
	}
	return errors.New(msg)
}

// File is a raw PikPak drive file item.
type File struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Size        int64  `json:"size"`
	ParentID    string `json:"parent_id"`
	CreatedTime string `json:"created_time"`
	ModifiedTime string `json:"modified_time"`
	Trashed     bool   `json:"trashed"`
	Starred     bool   `json:"starred"`
	Thumbnail   string `json:"thumbnail_link"`
	WebContentLink string `json:"web_content_link"`
	Medias      []Media `json:"medias"`
	Phase       string `json:"phase"`
	FileExtension string `json:"file_extension"`
	Links       *struct {
		ApplicationOctetStream *struct {
			URL string `json:"url"`
		} `json:"application/octet-stream"`
	} `json:"links"`
}

// Media is a video media stream entry.
type Media struct {
	MediaName   string `json:"media_name"`
	ResolutionName string `json:"resolution_name"`
	IsOrigin    bool   `json:"is_origin"`
	Category    string `json:"category"`
	NeedMoreQuota bool `json:"need_more_quota"`
	IsVisible   bool   `json:"is_visible"`
	Priority    int    `json:"priority"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Link        *struct {
		URL  string `json:"url"`
		Type string `json:"type"`
	} `json:"link"`
	Video *struct {
		VideoType string `json:"video_type"`
	} `json:"video"`
}

// listResp is the paginated file listing.
type listResp struct {
	Files        []File `json:"files"`
	NextPageToken string `json:"next_page_token"`
}

// ListPage lists one page.
func (c *client) ListPage(ctx context.Context, parentID, pageToken string, trashed bool) ([]File, string, error) {
	q := url.Values{}
	q.Set("parent_id", parentID)
	q.Set("page_size", "100")
	q.Set("thumbnail_size", "SIZE_LARGE")
	q.Set("filters", fmt.Sprintf(`{"trashed":%t}`, trashed))
	q.Set("with_audit", "false")
	if pageToken != "" {
		q.Set("page_token", pageToken)
	}
	var resp listResp
	if err := c.get(ctx, "/drive/v1/files", q, &resp); err != nil {
		return nil, "", err
	}
	return resp.Files, resp.NextPageToken, nil
}

// List fetches all pages.
func (c *client) List(ctx context.Context, parentID string, trashed bool) ([]File, error) {
	var out []File
	token := ""
	seen := map[string]bool{}
	for {
		items, next, err := c.ListPage(ctx, parentID, token, trashed)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
		if next == "" {
			break
		}
		if seen[next] {
			return nil, errors.New("pikpak: duplicate page token")
		}
		seen[next] = true
		token = next
	}
	return out, nil
}

// Detail returns one file.
func (c *client) Detail(ctx context.Context, fileID string) (*File, error) {
	var f File
	if err := c.get(ctx, "/drive/v1/files/"+url.PathEscape(fileID), nil, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// About returns quota.
func (c *client) About(ctx context.Context) (used, total int64) {
	var resp struct {
		Quota struct {
			Limit int64 `json:"limit"`
			Used  int64 `json:"used"`
		} `json:"quota"`
	}
	if err := c.get(ctx, "/drive/v1/about", nil, &resp); err == nil {
		return resp.Quota.Used, resp.Quota.Limit
	}
	return 0, 0
}

// DownloadURL resolves the direct download URL.
func (c *client) DownloadURL(ctx context.Context, fileID string) (string, int64, error) {
	var resp struct {
		URL  string `json:"url"`
		Size int64  `json:"size"`
	}
	if err := c.get(ctx, "/drive/v1/files/"+url.PathEscape(fileID)+"/download?redirect=false", nil, &resp); err != nil {
		// fall back to detail web_content_link
		f, err2 := c.Detail(ctx, fileID)
		if err2 != nil {
			return "", 0, err
		}
		if f.WebContentLink == "" {
			return "", 0, errors.New("pikpak: no download url")
		}
		return f.WebContentLink, f.Size, nil
	}
	return resp.URL, resp.Size, nil
}

// VipInfo returns whether the account has a VIP.
func (c *client) VipInfo(ctx context.Context) bool {
	var resp struct {
		Vip struct {
			Identity int `json:"identity"`
		} `json:"vip"`
	}
	_ = c.get(ctx, "/drive/v1/privilege/vip", nil, &resp)
	return resp.Vip.Identity > 0
}

// PlayInfo resolves video transcode qualities.
func (c *client) PlayInfo(ctx context.Context, fileID string) (*model.VideoPreview, error) {
	q := url.Values{}
	q.Set("file_id", fileID)
	q.Set("_", strconv.FormatInt(time.Now().UnixMilli(), 10))
	var resp struct {
		MediaPlayInfo *struct {
			TranscodeList []struct {
				TemplateID   string `json:"template_id"`
				URL          string `json:"url"`
				Width        int    `json:"width"`
				Height       int    `json:"height"`
				Status       string `json:"status"`
				Video        *struct{ VideoType string `json:"video_type"` } `json:"video"`
				Audio        *struct{ AudioType string `json:"audio_type"` } `json:"audio"`
			} `json:"transcode_list"`
		} `json:"media_play_info"`
	}
	if err := c.get(ctx, "/drive/v1/files/"+url.PathEscape(fileID)+"/video/play_info", q, &resp); err != nil {
		return nil, err
	}
	f, err := c.Detail(ctx, fileID)
	if err != nil {
		return nil, err
	}
	isVip := c.VipInfo(ctx)
	preview := &model.VideoPreview{FileID: fileID, Size: f.Size}
	if resp.MediaPlayInfo != nil {
		for _, tc := range resp.MediaPlayInfo.TranscodeList {
			if tc.URL == "" || tc.Status != "" && tc.Status != "success" {
				continue
			}
			if !isVip && tc.Height > 720 {
				continue
			}
				tier := qualityTier(tc.Height)
				var streamTypeName string
				if tc.Video != nil {
					streamTypeName = tc.Video.VideoType
				}
				preview.Qualities = append(preview.Qualities, model.VideoQuality{
					HTML: tier.html, Quality: tier.quality, Height: tc.Height, Width: tc.Width,
					Label: tier.html, Value: tier.quality, URL: tc.URL,
					Type: streamType(tc.URL, streamTypeName),
				})
		}
	}
	// origin
	origin := f.WebContentLink
	if origin == "" {
		for _, m := range f.Medias {
			if m.IsOrigin || m.Category == "category_origin" {
				if m.Link != nil && m.Link.URL != "" {
					origin = m.Link.URL
				}
			}
		}
	}
	if origin != "" {
		q := model.VideoQuality{HTML: "原画", Quality: "Origin", Label: "原画", Value: "Origin", URL: origin, ForceProxy: true}
		if isVip {
			preview.Qualities = append([]model.VideoQuality{q}, preview.Qualities...)
		} else {
			preview.Qualities = append(preview.Qualities, q)
		}
	}
	if len(preview.Qualities) == 0 {
		return nil, errors.New("pikpak: no playable quality")
	}
	return preview, nil
}

type tierInfo struct{ quality, html string }

func qualityTier(height int) tierInfo {
	switch {
	case height >= 2000:
		return tierInfo{"QHD", "2560p"}
	case height >= 1000:
		return tierInfo{"FHD", "1080P"}
	case height >= 700:
		return tierInfo{"HD", "720P"}
	case height >= 500:
		return tierInfo{"SD", "540P"}
	default:
		return tierInfo{"LD", "480P"}
	}
}

func streamType(rawURL, t string) string {
	u := strings.ToLower(rawURL)
	if strings.Contains(u, ".m3u8") || t == "m3u8" {
		return "hls"
	}
	if strings.Contains(u, ".mpd") {
		return "dash"
	}
	return "mp4"
}

// Mkdir creates a folder.
func (c *client) Mkdir(ctx context.Context, parentID, name string) (*driveMkdirResult, error) {
	var res struct {
		File File `json:"file"`
	}
	body := map[string]any{"name": name, "kind": "drive#folder"}
	if parentID != "" && parentID != RootID {
		body["parent_id"] = parentID
	}
	if err := c.jsonDo(ctx, http.MethodPost, "/drive/v1/files:create_folder", body, &res, nil); err != nil {
		return &driveMkdirResult{Error: err.Error()}, nil
	}
	return &driveMkdirResult{FileID: res.File.ID}, nil
}

type driveMkdirResult struct {
	FileID string `json:"file_id"`
	Error  string `json:"error"`
}

// batchOp performs a files:batch_* call.
func (c *client) batchOp(ctx context.Context, command string, body map[string]any) error {
	return c.jsonDo(ctx, http.MethodPost, "/drive/v1/files:"+command, body, nil, nil)
}

// Rename renames a file.
func (c *client) Rename(ctx context.Context, fileID, name string) error {
	return c.batchOp(ctx, "batch_rename", map[string]any{"requests": []any{map[string]any{"id": fileID, "name": name}}})
}

// Trash moves ids to trash.
func (c *client) Trash(ctx context.Context, ids []string) error {
	return c.batchOp(ctx, "batch_trash", map[string]any{"ids": ids})
}

// Delete permanently deletes ids.
func (c *client) Delete(ctx context.Context, ids []string) error {
	return c.batchOp(ctx, "batch_delete", map[string]any{"ids": ids})
}

// Restore restores ids from trash.
func (c *client) Restore(ctx context.Context, ids []string) error {
	return c.batchOp(ctx, "batch_restore", map[string]any{"ids": ids})
}

// Move moves ids to a parent.
func (c *client) Move(ctx context.Context, ids []string, toParentID string) error {
	return c.batchOp(ctx, "batch_move", map[string]any{"requests": []any{map[string]any{"ids": ids, "to_parent_id": toParentID}}})
}

// Copy copies ids to a parent.
func (c *client) Copy(ctx context.Context, ids []string, toParentID string) error {
	return c.batchOp(ctx, "batch_copy", map[string]any{"requests": []any{map[string]any{"ids": ids, "to_parent_id": toParentID}}})
}

// Star sets starred state.
func (c *client) Star(ctx context.Context, ids []string, starred bool) error {
	return c.batchOp(ctx, "batch_star", map[string]any{"ids": ids, "starred": starred})
}

// ShareResult is a created share.
type ShareResult struct {
	ShareID  string `json:"share_id"`
	ShareURL string `json:"share_url"`
	PassCode string `json:"pass_code"`
	Expiration string `json:"expiration"`
	FileIDs  []string `json:"file_ids"`
}

// CreateShare creates a share.
func (c *client) CreateShare(ctx context.Context, fileIDs []string, shareName string, passCode string, expiration string) (*ShareResult, error) {
	body := map[string]any{
		"file_ids":   fileIDs,
		"share_name": shareName,
		"pass_code":  passCode,
		"expiration": expiration,
	}
	var res ShareResult
	if err := c.jsonDo(ctx, http.MethodPost, "/drive/v1/share", body, &res, nil); err != nil {
		return nil, err
	}
	return &res, nil
}

// OfflineCreate submits an offline download task (magnet/link).
func (c *client) OfflineCreate(ctx context.Context, urlValue, fileName, parentID string) (taskID, fileID string, err error) {
	var res struct {
		TaskID string `json:"task_id"`
		FileID string `json:"file_id"`
	}
	body := map[string]any{
		"kind":        "drive#task",
		"upload_type": "UPLOAD_TYPE_FORM",
		"urls":        []string{urlValue},
		"name":        fileName,
	}
	if parentID != "" && parentID != RootID {
		body["parent_id"] = parentID
	}
	if err := c.jsonDo(ctx, http.MethodPost, "/drive/v1/tasks?type=offline", body, &res, nil); err != nil {
		return "", "", err
	}
	return res.TaskID, res.FileID, nil
}

// OfflineTask is a raw offline task.
type OfflineTask struct {
	TaskID string `json:"task_id"`
	FileID string `json:"file_id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	// progress fields
	Progress int `json:"progress"`
	Phase    string `json:"phase"`
	Message  string `json:"message"`
}

// OfflineList returns offline tasks.
func (c *client) OfflineList(ctx context.Context) ([]OfflineTask, error) {
	q := url.Values{}
	q.Set("type", "offline")
	q.Set("page_size", "100")
	var resp struct {
		Tasks []OfflineTask `json:"tasks"`
	}
	if err := c.get(ctx, "/drive/v1/tasks", q, &resp); err != nil {
		return nil, err
	}
	return resp.Tasks, nil
}

// FindOfflineTask searches tasks for one matching task/file id.
func (c *client) FindOfflineTask(ctx context.Context, taskID, fileID string) (*OfflineTask, error) {
	tasks, err := c.OfflineList(ctx)
	if err != nil {
		return nil, err
	}
	for i := range tasks {
		if (taskID != "" && tasks[i].TaskID == taskID) || (fileID != "" && tasks[i].FileID == fileID) {
			return &tasks[i], nil
		}
	}
	return nil, nil
}

var _ = netx.DefaultUA