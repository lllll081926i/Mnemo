package app

import (
	"errors"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

func (a *App) ListDeletionHistory(userID, driveID string) ([]model.DeletionRecord, error) {
	storage, err := a.storeOrError()
	if err != nil {
		return nil, err
	}
	return storage.ListDeletionRecords(userID, driveID)
}

func (a *App) RestoreDeletionHistory(userID, driveID, fileID string) error {
	records, err := a.ListDeletionHistory(userID, driveID)
	if err != nil {
		return err
	}
	for _, record := range records {
		if record.FileID != fileID {
			continue
		}
		if record.IDKind != "driveItem" || record.ProviderType != "personal" {
			return errors.New("此项目无法直接恢复，请前往官网回收站处理")
		}
		if record.Status == "restored" {
			return nil
		}
		_, err := drive.RestoreBatch(userID, driveID, []string{fileID})
		return err
	}
	return errors.New("删除记录不属于当前账号和空间")
}

func (a *App) ClearDeletionHistory(userID, driveID string) error {
	storage, err := a.storeOrError()
	if err != nil {
		return err
	}
	return storage.ClearDeletionRecords(userID, driveID)
}

func (a *App) OpenTrashWebsite(userID, driveID string) error {
	website, err := drive.TrashWebsite(a.appContext(), userID, driveID)
	if err != nil {
		return err
	}
	if website == "" {
		return errors.New("此网盘未提供官网回收站入口")
	}
	return a.OpenBrowser(website)
}

func (a *App) PreviewEnterpriseTrash(userID, driveID string) (*drive.TrashSnapshot, error) {
	return drive.PreviewEnterpriseTrash(a.appContext(), userID, driveID)
}
