package lanzou

import (
	"context"
	"errors"
	"strings"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

// CreateShare ports apiLanzouShareCreate: a share covers exactly one file
// (task 22) or folder (task 18), falling back between kinds.
func (d *Driver) CreateShare(ctx context.Context, c drive.Context, params drive.ShareParams) (*model.ShareItem, error) {
	if len(params.FileIDs) != 1 {
		return nil, errors.New("蓝奏分享一次只能选择一个文件或文件夹")
	}
	info, _ := d.fileShare(ctx, c, params.FileIDs[0], false)
	if fidOf(info) == "" {
		info, _ = d.fileShare(ctx, c, params.FileIDs[0], true)
	}
	fid := fidOf(info)
	pwd := strOf(firstOf(info, "pwd"))
	base := strOf(firstOf(info, "isnewd"))
	if base == "" {
		base = LANZOU_DEFAULT.ShareURL
	}
	base = strings.TrimSuffix(base, "/")
	if fid == "" {
		return nil, errors.New("创建分享失败")
	}
	shareName := params.ShareName
	if shareName == "" {
		shareName = "蓝奏分享"
	}
	return &model.ShareItem{
		AccountID:  c.UserID,
		DriveID:    c.DriveID,
		ShareID:    fid,
		ShareURL:   base + "/" + fid,
		SharePwd:   pwd,
		ShareName:  shareName,
		FileID:     params.FileIDs[0],
		FileIDList: params.FileIDs,
		Icon:       "iconwenjian",
	}, nil
}

func fidOf(info map[string]any) string {
	if info == nil {
		return ""
	}
	return strOf(firstOf(info, "f_id", "fid"))
}
