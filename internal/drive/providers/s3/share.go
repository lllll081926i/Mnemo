package s3

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

func (d *Driver) CreateShare(ctx context.Context, c drive.Context, params drive.ShareParams) (*model.ShareItem, error) {
	if len(params.FileIDs) != 1 || strings.TrimSpace(params.FileIDs[0]) == "" {
		return nil, errors.New("S3 临时链接一次只能选择一个文件")
	}
	if strings.TrimSpace(params.Password) != "" {
		return nil, errors.New("S3 预签名链接不支持密码保护")
	}
	expiresIn, err := s3ShareExpiration(params.Expiration)
	if err != nil {
		return nil, err
	}
	fileID := strings.TrimSpace(params.FileIDs[0])
	download, err := d.GetDownloadURL(ctx, c, fileID, int(expiresIn/time.Second))
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(params.ShareName)
	if name == "" {
		name = baseOf(fileID)
	}
	if name == "" {
		name = "S3 临时链接"
	}
	expiration := time.UnixMilli(download.ExpireTime).UTC().Format(time.RFC3339)
	return &model.ShareItem{
		AccountID:   c.UserID,
		DriveID:     c.DriveID,
		ShareID:     fileID + ":" + strconv.FormatInt(download.ExpireTime, 10),
		ShareURL:    download.URL,
		ShareName:   name,
		SharePolicy: "presigned",
		Expiration:  expiration,
		FileID:      fileID,
		FileIDList:  []string{fileID},
		ShareMsg:    "创建成功",
	}, nil
}

func s3ShareExpiration(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 24 * time.Hour, nil
	}
	if days, err := strconv.Atoi(value); err == nil {
		if days == 1 || days == 7 {
			return time.Duration(days) * 24 * time.Hour, nil
		}
		return 0, errors.New("S3 临时链接只支持 1 天或 7 天")
	}
	target, err := parseS3ShareTime(value)
	if err != nil {
		return 0, errors.New("S3 临时链接有效期格式无效")
	}
	remaining := time.Until(target)
	switch {
	case remaining <= 0:
		return 0, errors.New("S3 临时链接有效期已过期")
	case remaining <= 24*time.Hour:
		return 24 * time.Hour, nil
	case remaining <= 7*24*time.Hour:
		return 7 * 24 * time.Hour, nil
	default:
		return 0, errors.New("S3 临时链接只支持 1 天或 7 天")
	}
}

func parseS3ShareTime(value string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("invalid time")
}
