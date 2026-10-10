package guangya

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"

	"mnemo-go/internal/drive"
)

var _ drive.OfflineDriver = (*Driver)(nil)

type offlineResource struct {
	FileIndex      *json.Number      `json:"fileIndex"`
	IsDir          bool              `json:"isDir"`
	Subfiles       []offlineResource `json:"subfiles"`
	SubfilesNum    int               `json:"subfilesNum"`
	ExcludeIndices []json.Number     `json:"excludeIndices"`
}

type offlineRemoteTask struct {
	TaskID    string      `json:"taskId"`
	FileID    string      `json:"fileId"`
	FileName  string      `json:"fileName"`
	URL       string      `json:"url"`
	Status    json.Number `json:"status"`
	Progress  json.Number `json:"progress"`
	TotalSize json.Number `json:"totalSize"`
	ErrMsg    string      `json:"errMsg"`
}

func validOfflineURL(raw string) bool {
	if strings.HasPrefix(strings.ToLower(raw), "ed2k://|file|") {
		return strings.HasSuffix(raw, "|/")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return parsed.Hostname() != "" && parsed.User == nil
	case "magnet":
		return parsed.Query().Get("xt") != ""
	}
	return false
}

func (resource *offlineResource) selectedIndexes() ([]int64, error) {
	excluded := make(map[int64]bool)
	for _, value := range resource.ExcludeIndices {
		index, err := value.Int64()
		if err != nil || index < 0 {
			return nil, errors.New("光鸭资源解析返回了无效的排除索引")
		}
		excluded[index] = true
	}
	var selected []int64
	seen := make(map[int64]bool)
	var collect func([]offlineResource) error
	collect = func(files []offlineResource) error {
		for _, file := range files {
			if file.IsDir {
				if err := collect(file.Subfiles); err != nil {
					return err
				}
				continue
			}
			if file.FileIndex == nil {
				return errors.New("光鸭资源解析缺少子文件索引")
			}
			index, err := file.FileIndex.Int64()
			if err != nil || index < 0 {
				return errors.New("光鸭资源解析返回了无效的子文件索引")
			}
			if !excluded[index] && !seen[index] {
				selected = append(selected, index)
				seen[index] = true
			}
		}
		return nil
	}
	if err := collect(resource.Subfiles); err != nil {
		return nil, err
	}
	if len(selected) == 0 {
		return nil, errors.New("光鸭资源没有可添加的子文件")
	}
	return selected, nil
}

func (task offlineRemoteTask) model() (drive.OfflineTask, error) {
	if strings.TrimSpace(task.TaskID) == "" {
		return drive.OfflineTask{}, errors.New("光鸭云离线列表缺少任务 ID")
	}
	state, err := task.Status.Int64()
	if err != nil {
		return drive.OfflineTask{}, errors.New("光鸭云离线任务缺少有效状态")
	}
	status := "unknown"
	message := task.ErrMsg
	switch state {
	case 0:
		status = "queued"
	case 1:
		status = "running"
	case 2:
		status = "completed"
	case 3:
		status = "failed"
	case 4:
		status = "canceled"
	case 5:
		status = "partial"
		if message == "" {
			message = "部分文件已完成，请到光鸭官网查看未完成文件"
		}
	}
	progress := 0
	if task.Progress != "" {
		value, err := task.Progress.Float64()
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return drive.OfflineTask{}, errors.New("光鸭云离线任务进度无效")
		}
		progress = int(math.Round(math.Max(0, math.Min(100, value))))
	}
	if state == 2 {
		progress = 100
	}
	var size int64
	if task.TotalSize != "" {
		size, err = task.TotalSize.Int64()
		if err != nil || size < 0 {
			return drive.OfflineTask{}, errors.New("光鸭云离线任务文件大小无效")
		}
	}
	return drive.OfflineTask{TaskID: task.TaskID, FileID: task.FileID, Name: task.FileName, URL: task.URL,
		Status: status, Progress: progress, Message: message, FileSize: size}, nil
}

func (*Driver) OfflineCreate(ctx context.Context, account drive.Context, link, name, parentID string) (taskID, fileID string, err error) {
	link = strings.TrimSpace(link)
	if !validOfflineURL(link) {
		return "", "", errors.New("光鸭云离线支持 http/https、magnet 和 ed2k 链接")
	}
	cl, err := clientOf(account)
	if err != nil {
		return "", "", err
	}
	defer func() { err = errors.Join(err, drive.PersistToken(account)) }()
	var resolved struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data *struct {
			URL          string           `json:"url"`
			BTResInfo    *offlineResource `json:"btResInfo"`
			URLResInfo   *offlineResource `json:"urlResInfo"`
			EmuleResInfo *offlineResource `json:"emuleResInfo"`
		} `json:"data"`
	}
	if err = cl.post(ctx, "/nd.bizcloudcollection.s/v1/resolve_res", map[string]string{"url": link}, &resolved); err != nil {
		return "", "", err
	}
	if resolved.Code != 0 {
		return "", "", fmt.Errorf("光鸭解析云离线链接失败（code=%d）：%s", resolved.Code, resolved.Msg)
	}
	if resolved.Data == nil || !validOfflineURL(resolved.Data.URL) {
		return "", "", errors.New("光鸭资源解析未返回有效链接")
	}
	body := map[string]any{"url": resolved.Data.URL, "parentId": toID(parentID)}
	if name = strings.TrimSpace(name); name != "" {
		body["newName"] = name
	}
	for _, resource := range []*offlineResource{resolved.Data.BTResInfo, resolved.Data.URLResInfo, resolved.Data.EmuleResInfo} {
		if resource != nil && (resource.SubfilesNum > 0 || len(resource.Subfiles) > 0) {
			indexes, err := resource.selectedIndexes()
			if err != nil {
				return "", "", err
			}
			body["fileIndexes"] = indexes
			break
		}
	}
	var created struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data *struct {
			TaskID string `json:"taskId"`
			FileID string `json:"fileId"`
		} `json:"data"`
	}
	if err = cl.post(ctx, "/nd.bizcloudcollection.s/v1/create_task", body, &created); err != nil {
		return "", "", err
	}
	if created.Code != 0 {
		return "", "", fmt.Errorf("光鸭提交云离线任务失败（code=%d）：%s", created.Code, created.Msg)
	}
	if created.Data == nil || strings.TrimSpace(created.Data.TaskID) == "" {
		return "", "", errors.New("光鸭云离线响应缺少任务 ID")
	}
	return created.Data.TaskID, created.Data.FileID, nil
}

// OfflineList uses the zero-based page pagination documented by the provider
// web API. A cursor field is not an input to /v1/list_task.
func (*Driver) OfflineList(ctx context.Context, account drive.Context) (tasks []drive.OfflineTask, err error) {
	cl, err := clientOf(account)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, drive.PersistToken(account)) }()
	const pageSize = 50
	result := make([]drive.OfflineTask, 0)
	seenTasks := make(map[string]bool)
	for page := 0; page < 200; page++ {
		var response struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
			Data *struct {
				List    []offlineRemoteTask `json:"list"`
				HasMore *bool               `json:"hasMore"`
				Total   json.Number         `json:"total"`
			} `json:"data"`
		}
		body := map[string]any{"page": page, "pageSize": pageSize, "status": []int{0, 1, 2, 3, 4, 5}}
		if err := cl.post(ctx, "/nd.bizcloudcollection.s/v1/list_task", body, &response); err != nil {
			return nil, err
		}
		if response.Code != 0 {
			return nil, fmt.Errorf("光鸭查询云离线任务失败（code=%d）：%s", response.Code, response.Msg)
		}
		if response.Data == nil || response.Data.List == nil {
			return nil, errors.New("光鸭云离线列表响应缺少任务数据")
		}
		added := 0
		for _, item := range response.Data.List {
			task, err := item.model()
			if err != nil {
				return nil, err
			}
			if !seenTasks[task.TaskID] {
				result = append(result, task)
				seenTasks[task.TaskID] = true
				added++
			}
		}
		// Do not mask a provider that ignores the page index.
		if page > 0 && len(response.Data.List) > 0 && added == 0 {
			return nil, errors.New("光鸭云离线分页返回了重复任务，可能忽略了页码")
		}
		if response.Data.HasMore != nil {
			if !*response.Data.HasMore {
				return result, nil
			}
		} else {
			if response.Data.Total != "" {
				total, parseErr := response.Data.Total.Int64()
				if parseErr == nil && total >= 0 && int64((page+1)*pageSize) >= total {
					return result, nil
				}
			}
			if len(response.Data.List) < pageSize {
				return result, nil
			}
		}
	}
	return nil, errors.New("光鸭云离线分页超过安全上限")
}

func (*Driver) OfflineDelete(ctx context.Context, account drive.Context, taskIDs []string, deleteFiles bool) (err error) {
	if len(taskIDs) == 0 {
		return nil
	}
	for _, id := range taskIDs {
		if strings.TrimSpace(id) == "" {
			return errors.New("光鸭云离线任务 ID 不能为空")
		}
	}
	cl, err := clientOf(account)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, drive.PersistToken(account)) }()
	var response struct {
		Code *int   `json:"code"`
		Msg  string `json:"msg"`
		Data *struct {
			DeleteFileTaskID string `json:"deleteFileTaskId"`
		} `json:"data"`
	}
	if err := cl.post(ctx, "/nd.bizcloudcollection.s/v2/delete_task", map[string]any{"taskIds": taskIDs, "deleteFile": deleteFiles}, &response); err != nil {
		return err
	}
	if response.Code == nil {
		return errors.New("光鸭删除云离线任务响应缺少结果")
	}
	if *response.Code != 0 {
		return fmt.Errorf("光鸭删除云离线任务失败（code=%d）：%s", *response.Code, response.Msg)
	}
	if deleteFiles && response.Data != nil && response.Data.DeleteFileTaskID != "" {
		return cl.waitFileTask(ctx, response.Data.DeleteFileTaskID, "删除云离线文件")
	}
	return nil
}
