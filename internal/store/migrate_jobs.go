package store

import (
	"os"
	"time"

	"mnemo-go/internal/model"
)

const migrateJobsFile = "migrate_jobs.json"

// ListMigrateJobs loads all persisted migrate jobs.
func (s *Store) ListMigrateJobs() ([]model.MigrateJob, error) {
	var list []model.MigrateJob
	err := s.readJSON(migrateJobsFile, &list)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return list, nil
}

// SaveMigrateJob upserts a migrate job record.
func (s *Store) SaveMigrateJob(j *model.MigrateJob) error {
	list, err := s.ListMigrateJobs()
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
	return s.writeJSON(migrateJobsFile, list)
}

// DeleteMigrateJob removes a single migrate job by ID.
func (s *Store) DeleteMigrateJob(id string) error {
	list, err := s.ListMigrateJobs()
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
	return s.writeJSON(migrateJobsFile, out)
}

// ClearMigrateJobs removes finished migrate jobs (completed/canceled/partial/failed).
func (s *Store) ClearMigrateJobs() error {
	list, err := s.ListMigrateJobs()
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
	return s.writeJSON(migrateJobsFile, out)
}
