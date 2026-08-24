package pikpak

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mnemo-go/internal/drive"
)

func (c *client) OfflineCreate(ctx context.Context, urlValue, fileName, parentID string) (taskID, fileID string, err error) {
	var res struct {
		Task   OfflineTask     `json:"task"`
		TaskID string          `json:"task_id"`
		FileID string          `json:"file_id"`
		File   json.RawMessage `json:"file"`
		ID     string          `json:"id"`
	}
	body := map[string]any{
		"kind":        "drive#file",
		"upload_type": "UPLOAD_TYPE_URL",
		"url":         map[string]any{"url": urlValue},
	}
	if strings.TrimSpace(fileName) != "" {
		body["name"] = fileName
	}
	if parent := apiParentID(parentID); parent != "" {
		body["parent_id"] = parent
		body["folder_type"] = ""
	} else {
		body["folder_type"] = "DOWNLOAD"
	}
	if err := c.jsonDo(ctx, http.MethodPost, "/drive/v1/files", body, &res, nil); err != nil {
		return "", "", err
	}
	taskID = firstNonEmpty(res.Task.TaskID, res.TaskID, res.ID)
	fileID = res.FileID
	if fileID == "" {
		fileID, _, _ = offlineResourceInfo(res.File)
	}
	if taskID == "" && fileID == "" {
		return "", "", errors.New("pikpak: 离线任务响应缺少任务或文件 ID")
	}
	return taskID, fileID, nil
}

// OfflineTask is a raw offline task.
type OfflineTask struct {
	TaskID      string `json:"task_id"`
	FileID      string `json:"file_id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Progress    int    `json:"progress"`
	Phase       string `json:"phase"`
	Message     string `json:"message"`
	FileSize    int64  `json:"file_size,omitempty"`
	CreatedTime string `json:"created_time,omitempty"`
	UpdatedTime string `json:"updated_time,omitempty"`
	URL         string `json:"url,omitempty"`
}

type offlineTaskWire struct {
	ID                string                     `json:"id"`
	TaskID            string                     `json:"task_id"`
	FileID            string                     `json:"file_id"`
	FileName          string                     `json:"file_name"`
	Name              string                     `json:"name"`
	Status            string                     `json:"status"`
	Phase             string                     `json:"phase"`
	Progress          json.RawMessage            `json:"progress"`
	Message           string                     `json:"message"`
	Error             string                     `json:"error"`
	ErrorDescription  string                     `json:"error_description"`
	FileSize          json.RawMessage            `json:"file_size"`
	CreatedTime       string                     `json:"created_time"`
	UpdatedTime       string                     `json:"updated_time"`
	ReferenceResource json.RawMessage            `json:"reference_resource"`
	File              json.RawMessage            `json:"file"`
	Params            map[string]json.RawMessage `json:"params"`
}

func offlineRawString(raw json.RawMessage) string {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return ""
	}
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return value
	}
	return strings.Trim(strings.TrimSpace(string(raw)), `"`)
}

func offlineRawInt64(raw json.RawMessage) int64 {
	value, err := strconv.ParseFloat(offlineRawString(raw), 64)
	if err != nil || value <= 0 {
		return 0
	}
	return int64(value)
}

func offlineProgress(raw json.RawMessage) int {
	value, err := strconv.ParseFloat(offlineRawString(raw), 64)
	if err != nil || value <= 0 {
		return 0
	}
	if value <= 1 {
		value *= 100
	}
	if value > 100 {
		value = 100
	}
	return int(value)
}

func offlineResourceInfo(raw json.RawMessage) (id, name string, size int64) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return "", "", 0
	}
	var resource struct {
		ID   string          `json:"id"`
		Name string          `json:"name"`
		Size json.RawMessage `json:"size"`
	}
	if json.Unmarshal(raw, &resource) == nil && (resource.ID != "" || resource.Name != "" || len(resource.Size) > 0) {
		return resource.ID, resource.Name, offlineRawInt64(resource.Size)
	}
	return offlineRawString(raw), "", 0
}

func (t *OfflineTask) UnmarshalJSON(data []byte) error {
	var raw offlineTaskWire
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	resourceID, resourceName, resourceSize := offlineResourceInfo(raw.ReferenceResource)
	fileID, fileName, fileSize := offlineResourceInfo(raw.File)
	t.TaskID = firstNonEmpty(raw.TaskID, raw.ID)
	t.FileID = firstNonEmpty(raw.FileID, resourceID, fileID)
	t.Name = firstNonEmpty(raw.Name, raw.FileName, resourceName, fileName)
	t.Status = raw.Status
	t.Phase = raw.Phase
	if t.Phase == "" {
		t.Phase = t.Status
	}
	if t.Status == "" {
		t.Status = t.Phase
	}
	t.Progress = offlineProgress(raw.Progress)
	if t.Phase == "PHASE_TYPE_COMPLETE" {
		t.Progress = 100
	}
	t.Message = firstNonEmpty(raw.Message, raw.ErrorDescription, raw.Error)
	t.FileSize = offlineRawInt64(raw.FileSize)
	if t.FileSize == 0 {
		t.FileSize = resourceSize
	}
	if t.FileSize == 0 {
		t.FileSize = fileSize
	}
	t.CreatedTime = raw.CreatedTime
	t.UpdatedTime = raw.UpdatedTime
	if raw.Params != nil {
		t.URL = offlineRawString(raw.Params["url"])
	}
	return nil
}

// OfflineList returns offline tasks.
func (c *client) OfflineList(ctx context.Context) ([]OfflineTask, error) {
	const filters = `{"phase":{"in":"PHASE_TYPE_RUNNING,PHASE_TYPE_ERROR,PHASE_TYPE_COMPLETE,PHASE_TYPE_PENDING"}}`
	var out []OfflineTask
	pageToken := ""
	seenTokens := map[string]struct{}{}
	for {
		q := url.Values{}
		q.Set("type", "offline")
		q.Set("thumbnail_size", "SIZE_SMALL")
		q.Set("limit", strconv.Itoa(offlineListPageLimit))
		q.Set("filters", filters)
		q.Set("with", "reference_resource")
		if pageToken != "" {
			q.Set("page_token", pageToken)
		}
		var resp struct {
			Tasks         []OfflineTask `json:"tasks"`
			NextPageToken string        `json:"next_page_token"`
		}
		if err := c.get(ctx, "/drive/v1/tasks", q, &resp); err != nil {
			return nil, err
		}
		out = append(out, resp.Tasks...)
		if resp.NextPageToken == "" {
			break
		}
		if _, ok := seenTokens[resp.NextPageToken]; ok {
			return nil, errors.New("pikpak: offline task page token repeated")
		}
		seenTokens[resp.NextPageToken] = struct{}{}
		pageToken = resp.NextPageToken
		timer := time.NewTimer(offlineListPageDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return out, nil
}

// OfflineDelete cancels and removes an offline download task. deleteFiles=true
// also removes any partially downloaded file.
func (c *client) OfflineDelete(ctx context.Context, taskIDs []string, deleteFiles bool) error {
	if len(taskIDs) == 0 {
		return nil
	}
	q := url.Values{}
	q.Set("task_ids", strings.Join(taskIDs, ","))
	q.Set("delete_files", strconv.FormatBool(deleteFiles))
	target := apiHost + "/drive/v1/tasks?" + q.Encode()
	resp, err := c.http.Do(ctx, http.MethodDelete, target, c.headers(nil), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return parseAPIErrorWithRetry(b, resp.StatusCode, resp.Header.Get("Retry-After"))
	}
	return nil
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

// OfflineDelete cancels and removes offline tasks.
func (d *Driver) OfflineDelete(ctx context.Context, c drive.Context, taskIDs []string, deleteFiles bool) error {
	cl, err := clientOf(c)
	if err != nil {
		return err
	}
	return cl.OfflineDelete(ctx, taskIDs, deleteFiles)
}
