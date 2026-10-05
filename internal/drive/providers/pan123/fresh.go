package pan123

import (
	"context"
	"errors"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

func (driver *Driver) GetFileFresh(ctx context.Context, account drive.Context, fileID string) (*model.File, error) {
	if drive.IsRootID(providerID, fileID) || fileID == "0" {
		return driver.GetFile(ctx, account, RootID)
	}
	detail, err := driver.detail(ctx, account, fileID)
	if err != nil {
		return nil, err
	}
	if detail.FileID == "" || detail.FileID != toPan123FileID(fileID) {
		return nil, errors.New("123: 文件详情未返回匹配 ID")
	}
	file := mapFile(*detail, account.DriveID, detail.ParentFileID)
	return &file, nil
}
