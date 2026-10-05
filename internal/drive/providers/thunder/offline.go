package thunder

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mnemo-go/internal/drive"
)

type remoteTask struct {
	ID       string  `json:"id"`
	TaskID   string  `json:"task_id"`
	FileID   string  `json:"file_id"`
	FileName string  `json:"file_name"`
	Name     string  `json:"name"`
	Phase    string  `json:"phase"`
	Message  string  `json:"message"`
	Progress integer `json:"progress"`
	Size     integer `json:"file_size"`
	Created  string  `json:"created_time"`
	Updated  string  `json:"updated_time"`
	URL      string  `json:"url"`
}

func (task remoteTask) model() drive.OfflineTask {
	id := task.ID
	if id == "" {
		id = task.TaskID
	}
	name := task.FileName
	if name == "" {
		name = task.Name
	}
	progress := int(task.Progress)
	if progress > 100 || task.Phase == "PHASE_TYPE_COMPLETE" {
		progress = 100
	}
	return drive.OfflineTask{TaskID: id, FileID: task.FileID, Name: name, Phase: task.Phase, Message: task.Message,
		Progress: progress, FileSize: int64(task.Size), CreatedTime: task.Created, UpdatedTime: task.Updated, URL: task.URL}
}

func (*Driver) OfflineCreate(ctx context.Context, account drive.Context, link, name, parent string) (taskID, fileID string, err error) {
	if !validLink(link) && !strings.HasPrefix(strings.ToLower(link), "magnet:?") && !strings.HasPrefix(strings.ToLower(link), "ed2k://") {
		return "", "", errors.New("迅雷离线链接必须为 http/https、magnet 或 ed2k")
	}
	var result struct {
		Task remoteTask `json:"task"`
	}
	err = withClient(ctx, account, func(cl *client) error {
		return cl.request(ctx, http.MethodPost, "/files", map[string]any{"kind": "drive#file", "name": name, "parent_id": parentID(parent), "space": "", "upload_type": "UPLOAD_TYPE_URL", "url": map[string]string{"url": link}}, &result)
	})
	if err != nil {
		return "", "", err
	}
	task := result.Task.model()
	if task.Phase == "PHASE_TYPE_ERROR" {
		return "", "", errors.New("迅雷未能创建离线任务，请检查链接或账号权限")
	}
	if task.TaskID == "" && task.FileID == "" {
		return "", "", errors.New("迅雷离线响应缺少任务或文件 ID")
	}
	return task.TaskID, task.FileID, nil
}

func (*Driver) OfflineList(ctx context.Context, account drive.Context) ([]drive.OfflineTask, error) {
	var result []drive.OfflineTask
	err := withClient(ctx, account, func(cl *client) error {
		marker := ""
		seen := make(map[string]bool)
		for page := 0; page < 200; page++ {
			var current struct {
				Tasks []remoteTask `json:"tasks"`
				Next  string       `json:"next_page_token"`
			}
			query := url.Values{"type": {"offline"}, "limit": {"100"}, "page_token": {marker}, "space": {""}}
			if err := cl.request(ctx, http.MethodGet, "/tasks?"+query.Encode(), nil, &current); err != nil {
				return err
			}
			for _, remote := range current.Tasks {
				task := remote.model()
				if task.TaskID == "" && task.FileID == "" {
					return errors.New("迅雷离线列表包含无效任务")
				}
				result = append(result, task)
			}
			if current.Next == "" {
				return nil
			}
			if seen[current.Next] {
				return errors.New("迅雷离线分页游标重复")
			}
			seen[current.Next] = true
			marker = current.Next
		}
		return errors.New("迅雷离线分页超过安全上限")
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (*Driver) OfflineDelete(ctx context.Context, account drive.Context, ids []string, deleteFiles bool) error {
	if err := validateIDs(ids); err != nil {
		return err
	}
	query := url.Values{"task_ids": {strings.Join(ids, ",")}, "delete_files": {strconv.FormatBool(deleteFiles)}, "space": {""}}
	return withClient(ctx, account, func(cl *client) error { return cl.request(ctx, http.MethodDelete, "/tasks?"+query.Encode(), nil, nil) })
}

func (cl *client) mutate(ctx context.Context, method, path string, body any) error {
	var response struct {
		Task remoteTask `json:"task"`
	}
	if err := cl.request(ctx, method, path, body, &response); err != nil {
		return err
	}
	task := response.Task.model()
	if task.TaskID == "" {
		return nil
	}
	for attempt := 0; attempt < 30; attempt++ {
		if task.Phase == "PHASE_TYPE_COMPLETE" {
			return nil
		}
		if task.Phase == "PHASE_TYPE_ERROR" {
			return errors.New("迅雷文件操作任务失败，请刷新目录检查结果")
		}
		var listing struct {
			Tasks []remoteTask `json:"tasks"`
		}
		if err := cl.request(ctx, http.MethodGet, "/tasks?limit=100", nil, &listing); err != nil {
			return err
		}
		for _, remote := range listing.Tasks {
			if current := remote.model(); current.TaskID == task.TaskID {
				task = current
				break
			}
		}
		if task.Phase == "PHASE_TYPE_COMPLETE" {
			return nil
		}
		if task.Phase == "PHASE_TYPE_ERROR" {
			return errors.New("迅雷文件操作任务失败，请刷新目录检查结果")
		}
		timer := time.NewTimer(500 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return errors.New("迅雷操作已提交，但尚未确认完成，请刷新目录查看")
}
