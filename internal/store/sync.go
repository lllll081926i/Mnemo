package store

import (
	"os"

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
	return s.writeJSON(syncFile, out)
}
