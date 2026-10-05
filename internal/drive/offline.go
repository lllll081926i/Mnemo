package drive

import "context"

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

type OfflineDriver interface {
	OfflineCreate(ctx context.Context, c Context, url, fileName, parentID string) (taskID, fileID string, err error)
	OfflineList(ctx context.Context, c Context) ([]OfflineTask, error)
	OfflineDelete(ctx context.Context, c Context, taskIDs []string, deleteFiles bool) error
}
