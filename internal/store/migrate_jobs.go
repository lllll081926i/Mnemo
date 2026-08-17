package store

import (
	"fmt"
	"os"
	"time"

	"mnemo-go/internal/model"
)

const migrateJobsFile = "migrate_jobs.json"

// ListMigrateJobs loads all persisted migrate jobs.
func (s *Store) ListMigrateJobs() ([]model.MigrateJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var list []model.MigrateJob
	err := s.readJSON(migrateJobsFile, &list)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return list, nil
}

// SaveMigrateJob upserts a migrate job record.
func (s *Store) SaveMigrateJob(j *model.MigrateJob) error {
	if j == nil || j.ID == "" {
		return fmt.Errorf("store: migrate job id is empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var list []model.MigrateJob
	err := s.readJSON(migrateJobsFile, &list)
	if os.IsNotExist(err) {
		err = nil
	}
	if err != nil {
		return err
	}
	if j.CreatedAt == 0 {
		j.CreatedAt = time.Now().Unix()
	}
	j.UpdatedAt = time.Now().Unix()
	replaced := false
	for i := range list {
		if list[i].ID == j.ID {
			list[i] = *j
			replaced = true
			break
		}
	}
	if !replaced {
		list = append(list, *j)
	}
	return s.writeJSONUnlocked(migrateJobsFile, list)
}

// DeleteMigrateJob removes a single migrate job by ID.
func (s *Store) DeleteMigrateJob(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var list []model.MigrateJob
	err := s.readJSON(migrateJobsFile, &list)
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
	return s.writeJSONUnlocked(migrateJobsFile, out)
}

// ClearMigrateJobs removes finished migrate jobs (completed/canceled/partial/failed).
func (s *Store) ClearMigrateJobs() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var list []model.MigrateJob
	err := s.readJSON(migrateJobsFile, &list)
	if os.IsNotExist(err) {
		err = nil
	}
	if err != nil {
		return err
	}
	out := list[:0]
	for i := range list {
		switch list[i].Status {
		case "completed", "canceled", "partial", "failed":
			continue
		}
		out = append(out, list[i])
	}
	return s.writeJSONUnlocked(migrateJobsFile, out)
}
