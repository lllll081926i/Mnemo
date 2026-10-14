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
	replaced := false
	for i := range list {
		if list[i].ShareID == e.ShareID && list[i].AccountID == e.AccountID {
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
		match := entry.AccountID == accountID
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
	return list, nil
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
	replaced := false
	for i := range list {
		if list[i].ID == t.ID && list[i].UserID == t.UserID && list[i].DriveID == t.DriveID {
			list[i] = *t
			replaced = true
			break
		}
	}
	if !replaced {
		if t.Created == 0 {
			t.Created = time.Now().Unix()
		}
		list = append(list, *t)
	}
	return s.writeJSONUnlocked(offlineFile, list)
}

// DeleteOfflineTask removes the matching remote task record. A legacy
// unscoped call is retained for older callers; new callers must use
// DeleteOfflineTaskScoped to avoid deleting another account's equal ID.
func (s *Store) DeleteOfflineTask(id string) error {
	return s.DeleteOfflineTaskScoped("", "", id)
}

// DeleteOfflineTaskScoped removes a task only within the requested account
// and drive. The remote task ID remains unchanged for provider API calls.
func (s *Store) DeleteOfflineTaskScoped(userID, driveID, id string) error {
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
		if list[i].ID == id && (userID == "" || list[i].UserID == userID) && (driveID == "" || list[i].DriveID == driveID) {
			continue
		}
		out = append(out, list[i])
	}
	return s.writeJSONUnlocked(offlineFile, out)
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
