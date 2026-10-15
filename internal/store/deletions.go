package store

import (
	"os"
	"strings"
	"time"

	"mnemo-go/internal/model"
)

const deletionsFile = "deletions.json"

func (s *Store) ListDeletionRecords(userID, driveID string) ([]model.DeletionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var records []model.DeletionRecord
	if err := s.readJSON(deletionsFile, &records); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	result := []model.DeletionRecord{}
	for _, record := range records {
		if record.UserID == userID && record.DriveID == driveID {
			result = append(result, record)
		}
	}
	return result, nil
}

func (s *Store) SaveDeletionRecords(changes []model.DeletionRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var records []model.DeletionRecord
	if err := s.readJSON(deletionsFile, &records); err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, change := range changes {
		if strings.TrimSpace(change.UserID) == "" || strings.TrimSpace(change.FileID) == "" || change.IDKind != "driveItem" {
			return errInvalid("本机删除记录身份无效")
		}
		change.UpdatedAt = time.Now().Unix()
		replaced := false
		for index, existing := range records {
			if existing.UserID == change.UserID && existing.DriveID == change.DriveID && existing.FileID == change.FileID {
				records[index] = change
				replaced = true
				break
			}
		}
		if !replaced {
			records = append(records, change)
		}
	}
	return s.writeJSONUnlocked(deletionsFile, records)
}

func (s *Store) ClearDeletionRecords(userID, driveID string) error {
	return s.clearDeletionRecords(userID, driveID, false)
}

func (s *Store) ClearDeletionRecordsForAccount(userID string) error {
	return s.clearDeletionRecords(userID, "", true)
}

func (s *Store) clearDeletionRecords(userID, driveID string, allDrives bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var records []model.DeletionRecord
	if err := s.readJSON(deletionsFile, &records); err != nil && !os.IsNotExist(err) {
		return err
	}
	kept := records[:0]
	for _, record := range records {
		if record.UserID != userID || !allDrives && record.DriveID != driveID {
			kept = append(kept, record)
		}
	}
	return s.writeJSONUnlocked(deletionsFile, kept)
}
