package store

import (
	"os"
	"path/filepath"

	"mnemo-go/internal/sync"
)

const syncFile = "sync.json"

// ListSyncConfigs returns all sync jobs.
func (s *Store) ListSyncConfigs() ([]sync.Config, error) {
	var list []sync.Config
	err := s.readJSON(syncFile, &list)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return list, nil
}

// GetSyncConfig returns one sync job by id.
func (s *Store) GetSyncConfig(id string) (sync.Config, error) {
	list, err := s.ListSyncConfigs()
	if err != nil {
		return sync.Config{}, err
	}
	for _, c := range list {
		if c.ID == id {
			return c, nil
		}
	}
	return sync.Config{}, os.ErrNotExist
}

// SaveSyncConfig upserts a sync job.
func (s *Store) SaveSyncConfig(cfg sync.Config) error {
	list, err := s.ListSyncConfigs()
	if err != nil {
		return err
	}
	replaced := false
	for i := range list {
		if list[i].ID == cfg.ID {
			list[i] = cfg
			replaced = true
			break
		}
	}
	if !replaced {
		list = append(list, cfg)
	}
	return s.writeJSON(syncFile, list)
}

// DeleteSyncConfig removes a sync job.
func (s *Store) DeleteSyncConfig(id string) error {
	list, err := s.ListSyncConfigs()
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
	if err := s.writeJSON(syncFile, out); err != nil {
		return err
	}
	// clean up any persisted snapshot for this job
	_ = s.ClearSyncSnapshot(id)
	return nil
}

// syncSnapshotFile returns the file name for a given sync job snapshot.
func syncSnapshotFile(id string) string {
	return "SyncSnapshot_" + id + ".json"
}

// SaveSyncSnapshot persists the last-sync file list for a job.
func (s *Store) SaveSyncSnapshot(id string, entries []sync.Entry) error {
	return s.writeJSON(syncSnapshotFile(id), entries)
}

// LoadSyncSnapshot loads the last-sync file list for a job. Returns nil
// (no error) when the snapshot does not exist yet.
func (s *Store) LoadSyncSnapshot(id string) ([]sync.Entry, error) {
	var entries []sync.Entry
	err := s.readJSON(syncSnapshotFile(id), &entries)
	if err != nil && os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// ClearSyncSnapshot removes the persisted snapshot for a job.
func (s *Store) ClearSyncSnapshot(id string) error {
	if err := os.Remove(filepath.Join(s.dir, syncSnapshotFile(id))); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
