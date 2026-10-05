package pan189

import (
	"context"
	"errors"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

func (driver *Driver) GetFileFresh(ctx context.Context, account drive.Context, fileID string) (*model.File, error) {
	if drive.IsRootID(providerID, fileID) || fileID == "-11" || fileID == PAN189PersonalRoot || fileID == PAN189FamilyRoot {
		return driver.GetFile(ctx, account, fileID)
	}
	return nil, errors.New("天翼缺少文件所属目录，请刷新目录后重试，不能使用占位详情校验")
}
