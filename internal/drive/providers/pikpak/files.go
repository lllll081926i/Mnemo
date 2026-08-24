package pikpak

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"mnemo-go/internal/drive/driveutil"
	"mnemo-go/internal/model"
)

const pikpakFileDetailTTL = 45 * time.Second

type pikpakFileDetailCacheEntry struct {
	item      File
	expiresAt time.Time
}

var pikpakFileDetailCache = struct {
	sync.Mutex
	items map[string]pikpakFileDetailCacheEntry
}{items: make(map[string]pikpakFileDetailCacheEntry)}

type pikpakVIPCacheEntry struct {
	isVIP     bool
	expiresAt time.Time
}

var pikpakVIPCache = struct {
	sync.Mutex
	items map[string]pikpakVIPCacheEntry
}{items: make(map[string]pikpakVIPCacheEntry)}

type File struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Hash           string         `json:"hash"`
	Kind           string         `json:"kind"`
	Size           int64          `json:"size"`
	ParentID       string         `json:"parent_id"`
	CreatedTime    string         `json:"created_time"`
	ModifiedTime   string         `json:"modified_time"`
	Trashed        bool           `json:"trashed"`
	Starred        bool           `json:"starred"`
	Thumbnail      string         `json:"thumbnail_link"`
	WebContentLink string         `json:"web_content_link"`
	Medias         []Media        `json:"medias"`
	Phase          string         `json:"phase"`
	FileExtension  string         `json:"file_extension"`
	MimeType       string         `json:"mime_type"`
	Duration       any            `json:"duration"`
	Params         map[string]any `json:"params"`
	Links          *struct {
		ApplicationOctetStream *struct {
			URL string `json:"url"`
		} `json:"application/octet-stream"`
	} `json:"links"`
}

// Media is a video media stream entry.
type Media struct {
	MediaName      string `json:"media_name"`
	ResolutionName string `json:"resolution_name"`
	IsOrigin       bool   `json:"is_origin"`
	Category       string `json:"category"`
	NeedMoreQuota  bool   `json:"need_more_quota"`
	IsVisible      bool   `json:"is_visible"`
	Priority       int    `json:"priority"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	Link           *struct {
		URL  string `json:"url"`
		Type string `json:"type"`
	} `json:"link"`
	Video *MediaVideo `json:"video"`
}

type MediaVideo struct {
	VideoType string `json:"video_type"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Duration  any    `json:"duration"`
}

// listResp is the paginated file listing.
type listResp struct {
	Files         []File `json:"files"`
	NextPageToken string `json:"next_page_token"`
}

// ListPage lists one page.
func (c *client) ListPage(ctx context.Context, parentID, pageToken string, trashed bool) ([]File, string, error) {
	q := url.Values{}
	q.Set("limit", listPageLimit)
	q.Set("thumbnail_size", "SIZE_LARGE")
	q.Set("with_audit", "false")
	if trashed {
		q.Set("parent_id", "*")
		q.Set("filters", `{"trashed":{"eq":true}}`)
	} else {
		q.Set("filters", `{"trashed":{"eq":false},"phase":{"eq":"PHASE_TYPE_COMPLETE"}}`)
		if parentID != "" && parentID != RootID && parentID != "*" {
			q.Set("parent_id", parentID)
		}
	}
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

func (c *client) detailCacheKey(fileID string) string {
	return strings.Join([]string{c.deviceID, c.accountID, md5hex(c.accessToken), fileID}, "\x00")
}

func (c *client) detailOnce(ctx context.Context, fileID string) (*File, error) {
	var f File
	if err := c.get(ctx, "/drive/v1/files/"+url.PathEscape(fileID), nil, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

func fileHasPlayableLink(f *File) bool {
	if originalLink(f) != "" || f.WebContentLink != "" {
		return true
	}
	for _, media := range f.Medias {
		if media.Link != nil && media.Link.URL != "" {
			return true
		}
	}
	return false
}

func linksExpireSoon(f *File) bool {
	deadline := time.Now().Add(time.Minute).UnixMilli()
	urls := []string{originalLink(f), f.WebContentLink}
	for _, media := range f.Medias {
		if media.Link != nil {
			urls = append(urls, media.Link.URL)
		}
	}
	for _, raw := range urls {
		if expiresAt := driveutil.GetExpiresTime(raw); expiresAt > 0 && expiresAt <= deadline {
			return true
		}
	}
	return false
}

// Detail keeps short-lived metadata and signed links per account. The cache
// key includes the access token so a refreshed session cannot reuse an old
// account's links.
func (c *client) Detail(ctx context.Context, fileID string) (*File, error) {
	key := c.detailCacheKey(fileID)
	now := time.Now()
	pikpakFileDetailCache.Lock()
	entry, ok := pikpakFileDetailCache.items[key]
	if ok && entry.expiresAt.After(now) && !linksExpireSoon(&entry.item) {
		item := entry.item
		pikpakFileDetailCache.Unlock()
		return &item, nil
	}
	if ok {
		delete(pikpakFileDetailCache.items, key)
	}
	pikpakFileDetailCache.Unlock()

	item, err := c.detailOnce(ctx, fileID)
	if err != nil {
		return nil, err
	}
	if !fileHasPlayableLink(item) {
		timer := time.NewTimer(400 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
		if retry, retryErr := c.detailOnce(ctx, fileID); retryErr == nil {
			item = retry
		}
	}
	pikpakFileDetailCache.Lock()
	pikpakFileDetailCache.items[key] = pikpakFileDetailCacheEntry{item: *item, expiresAt: time.Now().Add(pikpakFileDetailTTL)}
	pikpakFileDetailCache.Unlock()
	return item, nil
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

func originalLink(f *File) string {
	if f == nil {
		return ""
	}
	if f.Links != nil && f.Links.ApplicationOctetStream != nil && f.Links.ApplicationOctetStream.URL != "" {
		return f.Links.ApplicationOctetStream.URL
	}
	return f.WebContentLink
}

func fidFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Query().Get("fid")
}

// bestDownloadLink mirrors the legacy provider's same-fid media selection.
func bestDownloadLink(f *File) string {
	original := originalLink(f)
	originalFID := fidFromURL(original)
	if originalFID == "" {
		return original
	}
	firstSameFID := ""
	for _, media := range f.Medias {
		if media.Link == nil || !strings.HasPrefix(media.Link.URL, "http") || fidFromURL(media.Link.URL) != originalFID {
			continue
		}
		if firstSameFID == "" {
			firstSameFID = media.Link.URL
		}
		if media.IsOrigin || media.Category == "category_origin" {
			return media.Link.URL
		}
	}
	if firstSameFID != "" {
		return firstSameFID
	}
	return original
}

func originMediaLink(f *File) string {
	original := originalLink(f)
	originalFID := fidFromURL(original)
	for _, media := range f.Medias {
		if media.Link == nil || !media.IsOrigin || !strings.HasPrefix(media.Link.URL, "http") {
			continue
		}
		if originalFID == "" || fidFromURL(media.Link.URL) == originalFID {
			return media.Link.URL
		}
	}
	return original
}

// DownloadURL resolves the direct download URL.
func (c *client) DownloadURL(ctx context.Context, fileID string) (string, int64, error) {
	if f, err := c.Detail(ctx, fileID); err == nil {
		if direct := bestDownloadLink(f); direct != "" {
			return direct, f.Size, nil
		}
	}
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
		if direct := bestDownloadLink(f); direct != "" {
			return direct, f.Size, nil
		}
		if f.WebContentLink == "" {
			return "", 0, errors.New("pikpak: no download url")
		}
		return f.WebContentLink, f.Size, nil
	}
	if resp.URL == "" {
		return "", 0, errors.New("pikpak: no download url")
	}
	return resp.URL, resp.Size, nil
}

// VipInfo returns whether the account has a VIP.
func (c *client) VipInfo(ctx context.Context) bool {
	cacheKey := strings.Join([]string{c.deviceID, c.accountID}, "\x00")
	now := time.Now()
	pikpakVIPCache.Lock()
	if entry, ok := pikpakVIPCache.items[cacheKey]; ok && entry.expiresAt.After(now) {
		pikpakVIPCache.Unlock()
		return entry.isVIP
	}
	pikpakVIPCache.Unlock()
	var resp struct {
		Vip struct {
			Identity int `json:"identity"`
		} `json:"vip"`
	}
	_ = c.get(ctx, "/drive/v1/privilege/vip", nil, &resp)
	isVIP := resp.Vip.Identity > 0
	pikpakVIPCache.Lock()
	pikpakVIPCache.items[cacheKey] = pikpakVIPCacheEntry{isVIP: isVIP, expiresAt: time.Now().Add(10 * time.Minute)}
	pikpakVIPCache.Unlock()
	return isVIP
}

// PlayInfo resolves video transcode qualities.
func (c *client) PlayInfo(ctx context.Context, fileID string) (*model.VideoPreview, error) {
	q := url.Values{}
	q.Set("file_id", fileID)
	q.Set("_", strconv.FormatInt(time.Now().UnixMilli(), 10))
	var resp struct {
		MediaPlayInfo *struct {
			TranscodeList []struct {
				TemplateID     string      `json:"template_id"`
				ResolutionName string      `json:"resolution_name"`
				MediaName      string      `json:"media_name"`
				URL            string      `json:"url"`
				Width          int         `json:"width"`
				Height         int         `json:"height"`
				Status         string      `json:"status"`
				Video          *MediaVideo `json:"video"`
				Audio          *struct {
					AudioType string `json:"audio_type"`
				} `json:"audio"`
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
	preview := &model.VideoPreview{FileID: fileID, Size: f.Size, Duration: fileDurationSeconds(f)}
	if resp.MediaPlayInfo != nil {
		for _, tc := range resp.MediaPlayInfo.TranscodeList {
			if tc.URL == "" || tc.Status != "" && tc.Status != "success" {
				continue
			}
			height := tc.Height
			width := tc.Width
			streamTypeName := ""
			if tc.Video != nil {
				if height == 0 {
					height = tc.Video.Height
				}
				if width == 0 {
					width = tc.Video.Width
				}
				streamTypeName = tc.Video.VideoType
			}
			height = resolutionHeight(height, tc.ResolutionName, tc.MediaName, tc.TemplateID)
			if !isVip && height > 720 {
				continue
			}
			tier := qualityTier(height)
			preview.Qualities = append(preview.Qualities, model.VideoQuality{
				HTML: tier.html, Quality: tier.quality, Height: height, Width: width,
				Label: tier.html, Value: tier.quality, URL: tc.URL,
				Type: streamType(tc.URL, streamTypeName),
			})
		}
	}
	// origin
	origin := originMediaLink(f)
	if origin != "" {
		q := model.VideoQuality{HTML: "原画", Quality: "Origin", Label: "原画", Value: "Origin", URL: origin, Type: streamType(origin, ""), ForceProxy: true}
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

var (
	resolutionPixelPattern = regexp.MustCompile(`(?i)(?:^|[^0-9])([0-9]{3,4})\s*p(?:\b|$)`)
	resolutionKPattern     = regexp.MustCompile(`(?i)(?:^|[^0-9])([248])\s*k(?:\b|$)`)
)

func resolutionHeight(height int, labels ...string) int {
	if height > 0 {
		return height
	}
	label := strings.ToLower(strings.Join(labels, " "))
	if match := resolutionPixelPattern.FindStringSubmatch(label); len(match) == 2 {
		if value, err := strconv.Atoi(match[1]); err == nil {
			return value
		}
	}
	if match := resolutionKPattern.FindStringSubmatch(label); len(match) == 2 {
		if value, err := strconv.Atoi(match[1]); err == nil {
			return value * 512
		}
	}
	return 0
}

func durationSeconds(raw any) int64 {
	value := 0.0
	switch v := raw.(type) {
	case float64:
		value = v
	case float32:
		value = float64(v)
	case int:
		value = float64(v)
	case int64:
		value = float64(v)
	case json.Number:
		value, _ = v.Float64()
	case string:
		value, _ = strconv.ParseFloat(strings.TrimSpace(v), 64)
	}
	if value <= 0 {
		return 0
	}
	if value > 100000 {
		value /= 1000
	}
	return int64(value + 0.5)
}

func fileDurationSeconds(f *File) int64 {
	if f == nil {
		return 0
	}
	best := durationSeconds(f.Duration)
	for _, media := range f.Medias {
		if media.Video != nil {
			if value := durationSeconds(media.Video.Duration); value > best {
				best = value
			}
		}
	}
	if value := durationSeconds(f.Params["duration"]); value > best {
		best = value
	}
	return best
}

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
	if stream := streamTypeHint(t); stream != "" {
		return stream
	}
	if parsed, err := url.Parse(rawURL); err == nil {
		if stream := streamTypePath(parsed.Path); stream != "" {
			return stream
		}
	}
	return "mp4"
}

func streamTypeHint(value string) string {
	value = strings.ToLower(strings.TrimSpace(strings.SplitN(value, ";", 2)[0]))
	switch value {
	case "hls", "m3u8", "application/vnd.apple.mpegurl", "application/x-mpegurl":
		return "hls"
	case "dash", "mpd", "application/dash+xml":
		return "dash"
	case "ts", "video/mp2t", "video/mpegts", "video/x-mpegts":
		return "ts"
	case "webm", "video/webm":
		return "webm"
	case "mkv", "matroska", "video/x-matroska":
		return "mkv"
	case "avi", "video/x-msvideo":
		return "avi"
	case "flv", "video/x-flv":
		return "flv"
	case "wmv", "video/x-ms-wmv":
		return "wmv"
	case "rm", "rmvb":
		return value
	case "video/vnd.rn-realvideo":
		return "rmvb"
	case "m2ts", "mts", "mpeg", "mpg", "mov", "m4v", "3gp":
		return value
	case "video/mpeg":
		return "mpeg"
	case "video/quicktime":
		return "mov"
	case "video/3gpp":
		return "3gp"
	default:
		return ""
	}
}

func streamTypePath(value string) string {
	path := strings.ToLower(value)
	for _, container := range []string{"m3u8", "mpd", "m2ts", "webm", "mkv", "avi", "flv", "wmv", "rmvb", "mpeg", "mts", "mpg", "mov", "m4v", "3gp", "ts", "rm"} {
		if strings.HasSuffix(path, "."+container) {
			switch container {
			case "m3u8":
				return "hls"
			case "mpd":
				return "dash"
			case "m2ts", "mts":
				return "ts"
			default:
				return container
			}
		}
	}
	return ""
}

func apiParentID(parentID string) string {
	switch strings.TrimSpace(parentID) {
	case "", "root", RootID, "*", "/":
		return ""
	default:
		return parentID
	}
}

// Mkdir creates a folder. PikPak uses the same /files endpoint for folder
// creation; the action-specific create_folder endpoint is not part of the
// legacy API used by the desktop and AList clients.
func (c *client) Mkdir(ctx context.Context, parentID, name string) (*driveMkdirResult, error) {
	var res struct {
		File File   `json:"file"`
		ID   string `json:"id"`
	}
	body := map[string]any{
		"kind": "drive#folder",
		"name": name,
	}
	if parent := apiParentID(parentID); parent != "" {
		body["parent_id"] = parent
	}
	if err := c.jsonDo(ctx, http.MethodPost, "/drive/v1/files", body, &res, nil); err != nil {
		return &driveMkdirResult{Error: err.Error()}, nil
	}
	fileID := res.File.ID
	if fileID == "" {
		fileID = res.ID
	}
	return &driveMkdirResult{FileID: fileID}, nil
}

type driveMkdirResult struct {
	FileID string `json:"file_id"`
	Error  string `json:"error"`
}

// batchOp performs a files:batch* call and waits for every async task to
// finish. A batch response can contain more than one task; waiting only for
// the first one leaves the UI reporting success while other files are still
// being moved or copied.
// PikPak returns a list of async tasks in the response; we poll /drive/v1/tasks
// until each completes (phase != PHASE_TYPE_PENDING/running) or a timeout is
// reached. This mirrors the legacy waitForPikPakTask behavior.
func (c *client) batchOp(ctx context.Context, command string, body map[string]any) error {
	var resp struct {
		Tasks []struct {
			ID     string `json:"id"`
			TaskID string `json:"task_id"`
			Phase  string `json:"phase"`
			Status string `json:"status"`
		} `json:"tasks"`
		TaskID string `json:"task_id"`
		ID     string `json:"id"`
		Task   *struct {
			ID     string `json:"id"`
			TaskID string `json:"task_id"`
		} `json:"task"`
	}
	if err := c.jsonDo(ctx, http.MethodPost, "/drive/v1/files:"+command, body, &resp, nil); err != nil {
		return err
	}
	taskIDs := make([]string, 0, len(resp.Tasks)+2)
	seen := make(map[string]struct{}, len(resp.Tasks)+2)
	appendTaskID := func(taskID string) {
		if taskID == "" {
			return
		}
		if _, ok := seen[taskID]; ok {
			return
		}
		seen[taskID] = struct{}{}
		taskIDs = append(taskIDs, taskID)
	}
	appendTaskID(resp.TaskID)
	appendTaskID(resp.ID)
	if resp.Task != nil {
		appendTaskID(resp.Task.TaskID)
		appendTaskID(resp.Task.ID)
	}
	for _, task := range resp.Tasks {
		appendTaskID(task.TaskID)
		appendTaskID(task.ID)
	}
	if len(taskIDs) == 0 {
		return nil
	}
	for _, taskID := range taskIDs {
		if err := c.waitForTasks(ctx, taskID); err != nil {
			return err
		}
	}
	return nil
}

// waitForTasks polls a PikPak async task until it reaches a terminal phase.
func (c *client) waitForTasks(ctx context.Context, taskID string) error {
	if taskID == "" {
		return nil
	}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		task, err := c.findTask(ctx, taskID)
		if err != nil {
			return fmt.Errorf("pikpak: query task %s: %w", taskID, err)
		}
		if task == nil {
			return fmt.Errorf("pikpak: task %s not found", taskID)
		}
		phase := task.Phase
		if phase == "" {
			phase = task.Status
		}
		switch phase {
		case "PHASE_TYPE_COMPLETE", "complete", "completed":
			return nil
		case "PHASE_TYPE_ERROR", "error", "failed":
			if task.Message != "" {
				return fmt.Errorf("pikpak: task %s failed: %s", taskID, task.Message)
			}
			return fmt.Errorf("pikpak: task %s failed", taskID)
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return fmt.Errorf("pikpak: task %s timeout", taskID)
}

// findTask queries the task endpoint used by file batch operations. Offline
// tasks are exposed through a different filtered collection.
func (c *client) findTask(ctx context.Context, taskID string) (*OfflineTask, error) {
	var raw json.RawMessage
	if err := c.get(ctx, "/drive/v1/tasks/"+url.PathEscape(taskID), nil, &raw); err != nil {
		return nil, err
	}
	var task OfflineTask
	if err := json.Unmarshal(raw, &task); err != nil {
		return nil, err
	}
	if task.TaskID != "" || task.Phase != "" || task.Status != "" {
		return &task, nil
	}
	var envelope struct {
		Task json.RawMessage `json:"task"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	if len(envelope.Task) == 0 || string(envelope.Task) == "null" {
		return &task, nil
	}
	if err := json.Unmarshal(envelope.Task, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

// Rename renames a file.
func (c *client) Rename(ctx context.Context, fileID, name string) error {
	return c.jsonDo(ctx, http.MethodPatch, "/drive/v1/files/"+url.PathEscape(fileID), map[string]any{"name": name}, nil, nil)
}

// Trash moves ids to trash.
func (c *client) Trash(ctx context.Context, ids []string) error {
	return c.batchOp(ctx, "batchTrash", map[string]any{"ids": ids})
}

// Delete permanently deletes ids.
func (c *client) Delete(ctx context.Context, ids []string) error {
	return c.batchOp(ctx, "batchDelete", map[string]any{"ids": ids})
}

// Restore restores ids from trash.
func (c *client) Restore(ctx context.Context, ids []string) error {
	return c.batchOp(ctx, "batchUntrash", map[string]any{"ids": ids})
}

// Move moves ids to a parent.
func (c *client) Move(ctx context.Context, ids []string, toParentID string) error {
	to := map[string]any{}
	if parent := apiParentID(toParentID); parent != "" {
		to["parent_id"] = parent
	}
	return c.batchOp(ctx, "batchMove", map[string]any{
		"ids": ids,
		"to":  to,
	})
}

// Copy copies ids to a parent.
func (c *client) Copy(ctx context.Context, ids []string, toParentID string) error {
	to := map[string]any{}
	if parent := apiParentID(toParentID); parent != "" {
		to["parent_id"] = parent
	}
	return c.batchOp(ctx, "batchCopy", map[string]any{
		"ids": ids,
		"to":  to,
	})
}

// Star sets starred state.
func (c *client) Star(ctx context.Context, ids []string, starred bool) error {
	command := "star"
	if !starred {
		command = "unstar"
	}
	return c.jsonDo(ctx, http.MethodPost, "/drive/v1/files:"+command, map[string]any{"ids": ids}, nil, nil)
}

// ShareResult is a created share.
