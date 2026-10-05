package lanzou

import (
	"context"
	"errors"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

func (driver *Driver) GetFileFresh(ctx context.Context, account drive.Context, fileID string) (*model.File, error) {
	if isRootSentinel(fileID) {
		return driver.GetFile(ctx, account, fileID)
	}
	return nil, errors.New("蓝奏缺少文件所属目录，请刷新目录后重试，不能使用占位详情校验")
}
