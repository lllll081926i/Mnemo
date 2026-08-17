package store

import (
	"os"
	"time"

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
	replaced := false
	for i := range list {
		if list[i].ID == t.ID {
			list[i] = *t
			replaced = true
			break
		}
	}
	if !replaced {
		list = append(list, *t)
	}
	return s.writeJSONUnlocked(tasksFile, list)
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
	for i := range list {
		switch list[i].Status {
		case "completed", "canceled", "failed":
			continue
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
		if list[i].ID == t.ID {
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

// DeleteOfflineTask removes an offline task.
func (s *Store) DeleteOfflineTask(id string) error {
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
		if list[i].ID == id {
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

// ClearUploadTasks removes finished upload jobs.
func (s *Store) ClearUploadTasks() error {
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
		if list[i].Upload.IsCompleted || list[i].Upload.IsFailed || list[i].Upload.IsStop {
			continue
		}
		out = append(out, list[i])
	}
	return s.writeJSONUnlocked(uploadsFile, out)
}
