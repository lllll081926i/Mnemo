package store

import (
	"os"
	"strings"

	"mnemo-go/internal/model"
)

const shareImportsFile = "share_imports.json"

func (s *Store) ListShareImports(accountID, driveID string) ([]model.ShareImportReceipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var receipts []model.ShareImportReceipt
	if err := s.readJSON(shareImportsFile, &receipts); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	result := []model.ShareImportReceipt{}
	for _, receipt := range receipts {
		if receipt.AccountID == accountID && receipt.DriveID == driveID {
			result = append(result, receipt)
		}
	}
	return result, nil
}

func (s *Store) SaveShareImport(receipt model.ShareImportReceipt) error {
	if strings.TrimSpace(receipt.ID) == "" || strings.TrimSpace(receipt.AccountID) == "" || len(receipt.Items) == 0 {
		return errInvalid("转存回执身份无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var receipts []model.ShareImportReceipt
	if err := s.readJSON(shareImportsFile, &receipts); err != nil && !os.IsNotExist(err) {
		return err
	}
	receipt.HistorySaved = true
	for index, existing := range receipts {
		if existing.ID == receipt.ID && existing.AccountID == receipt.AccountID && existing.DriveID == receipt.DriveID {
			receipts[index] = receipt
			return s.writeJSONUnlocked(shareImportsFile, receipts)
		}
	}
	return s.writeJSONUnlocked(shareImportsFile, append(receipts, receipt))
}

func (s *Store) ClearShareImportsForAccount(accountID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var receipts []model.ShareImportReceipt
	if err := s.readJSON(shareImportsFile, &receipts); err != nil && !os.IsNotExist(err) {
		return err
	}
	kept := receipts[:0]
	for _, receipt := range receipts {
		if receipt.AccountID != accountID {
			kept = append(kept, receipt)
		}
	}
	return s.writeJSONUnlocked(shareImportsFile, kept)
}
