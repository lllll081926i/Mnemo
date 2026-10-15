package store

import (
	"os"
	"strings"
	"time"

	"mnemo-go/internal/logging"
	"mnemo-go/internal/model"
)

const tasksFile = "tasks.json"

// ListDownloadTasks loads persisted download tasks.
func (s *Store) ListDownloadTasks() ([]model.DownloadTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var list []model.DownloadTask
	err := s.readJSON(tasksFile, &list)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return list, nil
}

// SaveDownloadTask upserts a download task.
func (s *Store) SaveDownloadTask(t *model.DownloadTask) error {
	if t == nil || t.ID == "" {
		return errInvalid("download task id is empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var list []model.DownloadTask
	err := s.readJSON(tasksFile, &list)
	if os.IsNotExist(err) {
		err = nil
	}
	if err != nil {
		return err
	}
	for i := range list {
		list[i] = safeStoredDownloadTask(list[i])
	}
	safeTask := safeStoredDownloadTask(*t)
	replaced := false
	for i := range list {
		if list[i].ID == t.ID {
			list[i] = safeTask
			replaced = true
			break
		}
	}
	if !replaced {
		list = append(list, safeTask)
	}
	return s.writeJSONUnlocked(tasksFile, list)
}

func safeStoredDownloadTask(task model.DownloadTask) model.DownloadTask {
	task.URL = ""
	task.Headers = nil
	task.Error = logging.RedactText(task.Error)
	return task
}

// ClearDownloadTasks removes finished tasks (completed/canceled/failed).
func (s *Store) ClearDownloadTasks() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var list []model.DownloadTask
	err := s.readJSON(tasksFile, &list)
	if os.IsNotExist(err) {
		err = nil
	}
	if err != nil {
		return err
	}
	out := list[:0]
	activeGroups := map[string]bool{}
	for _, task := range list {
		if task.IsDir && task.Status != "completed" && task.Status != "canceled" {
			activeGroups[task.ID] = true
		}
	}
	for i := range list {
		switch list[i].Status {
		case "completed", "canceled", "failed":
			if !activeGroups[list[i].ID] && !activeGroups[list[i].GroupID] {
				continue
			}
		}
		out = append(out, list[i])
	}
	return s.writeJSONUnlocked(tasksFile, out)
}

// DeleteDownloadTask removes a single task record by ID.
func (s *Store) DeleteDownloadTask(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var list []model.DownloadTask
	err := s.readJSON(tasksFile, &list)
	if os.IsNotExist(err) {
		err = nil
	}
	if err != nil {
		return err
	}
	out := list[:0]
	for i := range list {
		if list[i].ID == id {
			continue
		}
		out = append(out, list[i])
	}
	return s.writeJSONUnlocked(tasksFile, out)
}

// ShareHistory persistence.
const shareHistoryFile = "sharehistory.json"

// SaveShareHistory persists a share history entry.
func (s *Store) SaveShareHistory(e model.ShareHistoryEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var list []model.ShareHistoryEntry
	err := s.readJSON(shareHistoryFile, &list)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if e.CreatedAt == 0 {
		e.CreatedAt = time.Now().Unix()
	}
	if e.RemoteStatus == "" {
		e.RemoteStatus = "unchecked"
	}
	replaced := false
	for i := range list {
		if sameShareHistoryIdentity(list[i], e) {
			list[i] = e
			replaced = true
			break
		}
	}
	if !replaced {
		list = append(list, e)
	}
	return s.writeJSONUnlocked(shareHistoryFile, list)
}

func sameShareHistoryIdentity(first, second model.ShareHistoryEntry) bool {
	if first.AccountID != second.AccountID || first.DriveID != second.DriveID {
		return false
	}
	if first.ShareID != "" && second.ShareID != "" {
		return first.ShareID == second.ShareID
	}
	return first.ShareURL != "" && first.ShareURL == second.ShareURL
}

// ListShareHistory returns share history of an account.
func (s *Store) ListShareHistory(userID string) ([]model.ShareHistoryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var list []model.ShareHistoryEntry
	err := s.readJSON(shareHistoryFile, &list)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	var out []model.ShareHistoryEntry
	for _, e := range list {
		if userID == "" || e.AccountID == userID {
			if e.RemoteStatus == "" {
				e.RemoteStatus = "unchecked"
			}
			out = append(out, e)
		}
	}
	return out, nil
}

// DeleteShareHistory removes the local record only after its remote share has
// been revoked. ShareID is authoritative; legacy records without it fall back
// to the share URL. Account ID is always part of the match so equal provider
// ids in multiple accounts cannot remove each other.
func (s *Store) DeleteShareHistory(accountID, shareID, shareURL string) error {
	return s.deleteShareHistory(accountID, nil, shareID, shareURL)
}

func (s *Store) DeleteShareHistoryScoped(accountID, driveID, shareID, shareURL string) error {
	if strings.TrimSpace(accountID) == "" {
		return errInvalid("删除分享记录需要账号")
	}
	return s.deleteShareHistory(accountID, &driveID, shareID, shareURL)
}

func (s *Store) deleteShareHistory(accountID string, driveID *string, shareID, shareURL string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var list []model.ShareHistoryEntry
	err := s.readJSON(shareHistoryFile, &list)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	shareID = strings.TrimSpace(shareID)
	shareURL = strings.TrimSpace(shareURL)
	out := list[:0]
	for _, entry := range list {
		match := entry.AccountID == accountID && (driveID == nil || entry.DriveID == *driveID)
		if match && shareID != "" {
			match = entry.ShareID == shareID
		} else if match {
			match = shareURL != "" && entry.ShareURL == shareURL
		}
		if !match {
			out = append(out, entry)
		}
	}
	return s.writeJSONUnlocked(shareHistoryFile, out)
}

// Offline tasks persistence.
const offlineFile = "offline.json"

// ListOfflineTasks loads offline tasks.
func (s *Store) ListOfflineTasks() ([]model.OfflineTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var list []model.OfflineTask
	err := s.readJSON(offlineFile, &list)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if sanitizeOfflineTasks(list) {
		if err := s.writeJSONUnlocked(offlineFile, list); err != nil {
			return nil, err
		}
	}
	return list, nil
}

func sanitizeOfflineTasks(list []model.OfflineTask) bool {
	changed := false
	for index := range list {
		safeURL := model.SafeOfflineDisplayURL(list[index].URL)
		if safeURL != list[index].URL {
			list[index].URL = safeURL
			changed = true
		}
	}
	return changed
}

// SaveOfflineTask upserts an offline task.
func (s *Store) SaveOfflineTask(t *model.OfflineTask) error {
	if t == nil || t.ID == "" {
		return errInvalid("offline task id is empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var list []model.OfflineTask
	err := s.readJSON(offlineFile, &list)
	if os.IsNotExist(err) {
		err = nil
	}
	if err != nil {
		return err
	}
	sanitizeOfflineTasks(list)
	safeTask := *t
	safeTask.URL = model.SafeOfflineDisplayURL(safeTask.URL)
	replaced := false
	for i := range list {
		if list[i].ID == t.ID && list[i].UserID == t.UserID && list[i].DriveID == t.DriveID {
			list[i] = safeTask
			replaced = true
			break
		}
	}
	if !replaced {
		if safeTask.Created == 0 {
			safeTask.Created = time.Now().Unix()
			t.Created = safeTask.Created
		}
		list = append(list, safeTask)
	}
	return s.writeJSONUnlocked(offlineFile, list)
}

// DeleteOfflineTaskScoped removes a task only within the requested account
// and drive. The remote task ID remains unchanged for provider API calls.
func (s *Store) DeleteOfflineTaskScoped(userID, driveID, id string) error {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(id) == "" {
		return errInvalid("删除离线记录需要账号和任务 ID")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var list []model.OfflineTask
	err := s.readJSON(offlineFile, &list)
	if os.IsNotExist(err) {
		err = nil
	}
	if err != nil {
		return err
	}
	out := list[:0]
	for i := range list {
		if list[i].ID == id && list[i].UserID == userID && list[i].DriveID == driveID {
			continue
		}
		out = append(out, list[i])
	}
	sanitizeOfflineTasks(out)
	return s.writeJSONUnlocked(offlineFile, out)
}

func (s *Store) ReconcileOfflineTasks(userID, driveID string, remote []model.OfflineTask, complete bool) ([]model.OfflineTask, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, errInvalid("离线任务对账需要账号")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var list []model.OfflineTask
	if err := s.readJSON(offlineFile, &list); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	sanitizeOfflineTasks(list)
	positions := map[string]int{}
	for index, task := range list {
		if task.UserID == userID && task.DriveID == driveID {
			positions[task.ID] = index
		}
	}
	seen := map[string]bool{}
	for _, task := range remote {
		if task.ID == "" || task.UserID != userID || task.DriveID != driveID {
			return nil, errInvalid("离线列表身份与当前账号或空间不匹配")
		}
		task.URL = model.SafeOfflineDisplayURL(task.URL)
		if index, exists := positions[task.ID]; exists {
			if task.Created == 0 {
				task.Created = list[index].Created
			}
			list[index] = task
		} else {
			if task.Created == 0 {
				task.Created = time.Now().Unix()
			}
			positions[task.ID] = len(list)
			list = append(list, task)
		}
		seen[task.ID] = true
	}
	result := []model.OfflineTask{}
	for index := range list {
		task := &list[index]
		if task.UserID != userID || task.DriveID != driveID {
			continue
		}
		if complete && !seen[task.ID] {
			task.Status = "notFoundOnServer"
			task.Message = "服务端完整列表未返回此任务，完成结果未确认；请核对目标目录"
		}
		result = append(result, *task)
	}
	if err := s.writeJSONUnlocked(offlineFile, list); err != nil {
		return nil, err
	}
	return result, nil
}

// Upload tasks persistence.
const uploadsFile = "uploads.json"

// ListUploadTasks loads persisted upload jobs.
func (s *Store) ListUploadTasks() ([]model.UploadingUI, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var list []model.UploadingUI
	err := s.readJSON(uploadsFile, &list)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return list, nil
}

// SaveUploadTask upserts an upload job.
func (s *Store) SaveUploadTask(j *model.UploadingUI) error {
	if j == nil || j.UploadID == "" {
		return errInvalid("upload task id is empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var list []model.UploadingUI
	err := s.readJSON(uploadsFile, &list)
	if os.IsNotExist(err) {
		err = nil
	}
	if err != nil {
		return err
	}
	replaced := false
	for i := range list {
		if list[i].UploadID == j.UploadID {
			list[i] = *j
			replaced = true
			break
		}
	}
	if !replaced {
		list = append(list, *j)
	}
	return s.writeJSONUnlocked(uploadsFile, list)
}

// ClearUploadTasks clears completed uploads only. Failed or stopped
// uploads remain available for manual retry.
func (s *Store) ClearUploadTasks() error {
	return s.clearUploadTasks(false)
}

func (s *Store) DeleteUploadTasks(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var list []model.UploadingUI
	if err := s.readJSON(uploadsFile, &list); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	selected := make(map[string]bool, len(ids))
	for _, id := range ids {
		selected[id] = true
	}
	remaining := list[:0]
	for _, task := range list {
		if !selected[task.UploadID] {
			remaining = append(remaining, task)
		}
	}
	return s.writeJSONUnlocked(uploadsFile, remaining)
}

// ClearFinishedUploadTasks applies the explicit "do not keep tasks" setting,
// including failed and stopped attempts as in previous versions.
func (s *Store) ClearFinishedUploadTasks() error {
	return s.clearUploadTasks(true)
}

func (s *Store) clearUploadTasks(includeRetryable bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var list []model.UploadingUI
	err := s.readJSON(uploadsFile, &list)
	if os.IsNotExist(err) {
		err = nil
	}
	if err != nil {
		return err
	}
	out := list[:0]
	for i := range list {
		if list[i].Upload.IsCompleted || (includeRetryable && (list[i].Upload.IsFailed || list[i].Upload.IsStop)) {
			continue
		}
		out = append(out, list[i])
	}
	return s.writeJSONUnlocked(uploadsFile, out)
}
