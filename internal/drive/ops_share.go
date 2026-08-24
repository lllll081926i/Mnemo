package drive

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"mnemo-go/internal/model"
)

// CreateShare creates a share.
func CreateShare(userID, driveID string, params ShareParams) (share *model.ShareItem, err error) {
	if len(params.FileIDs) == 0 {
		return nil, errors.New("drive: 创建分享至少选择一个文件")
	}
	for _, fileID := range params.FileIDs {
		if strings.TrimSpace(fileID) == "" {
			return nil, errors.New("drive: 分享文件 ID 不能为空")
		}
	}
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	return d.CreateShare(context.Background(), c, params)
}

// CancelShare revokes a provider-managed share. Providers that only generate
// an expiring URL must not advertise this capability because such URLs cannot
// be individually revoked once issued.
func CancelShare(userID, driveID string, share model.ShareHistoryEntry) (err error) {
	if strings.TrimSpace(share.AccountID) == "" {
		share.AccountID = userID
	}
	if strings.TrimSpace(share.DriveID) == "" {
		share.DriveID = driveID
	}
	if strings.TrimSpace(share.ShareID) == "" && strings.TrimSpace(share.ShareURL) == "" {
		return errors.New("drive: 分享标识不能为空")
	}
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return err
	}
	if !d.Capabilities().CancelCreatedShares {
		return errors.New("该网盘不支持取消已创建的分享")
	}
	canceller, ok := d.(ShareCancellationDriver)
	if !ok {
		return errors.New("该网盘未实现分享取消接口")
	}
	defer func() { err = withTokenPersist(err, c) }()
	return canceller.CancelShare(context.Background(), c, share)
}

// ImportShare parses a share link and returns the file listing + session
// state. The session must be passed back to SaveImportedShare to transfer
// selected files. Returns ErrNotImplemented when the provider does not
// support share import.
func ImportShare(userID, driveID, shareURL, password string) (session *ShareImportSession, err error) {
	if strings.TrimSpace(shareURL) == "" {
		return nil, errors.New("drive: 分享链接不能为空")
	}
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	sd, ok := d.(ShareImportDriver)
	if !ok {
		return nil, ErrNotImplemented
	}
	return sd.ImportShareSession(context.Background(), c, shareURL, password)
}

// SaveImportedShare transfers selected files from a parsed share session
// into the account's folder toParentID. Returns the provider-side ids of
// the saved files.
func SaveImportedShare(userID, driveID string, session *ShareImportSession, fileIDs []string, toParentID string) (ids []string, err error) {
	if session == nil || strings.TrimSpace(session.Provider) == "" {
		return nil, errors.New("drive: 分享会话无效")
	}
	if len(fileIDs) == 0 {
		return nil, errors.New("drive: 至少选择一个分享文件")
	}
	for _, fileID := range fileIDs {
		if strings.TrimSpace(fileID) == "" {
			return nil, errors.New("drive: 分享文件 ID 不能为空")
		}
	}
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	sd, ok := d.(ShareImportDriver)
	if !ok {
		return nil, ErrNotImplemented
	}
	if session.Provider != c.TokenFrom {
		return nil, fmt.Errorf("drive: 分享来源网盘与目标账号不匹配")
	}
	allowed := make(map[string]struct{}, len(session.Files))
	for _, file := range session.Files {
		if file.FileID != "" {
			allowed[file.FileID] = struct{}{}
		}
	}
	for _, fileID := range fileIDs {
		if _, exists := allowed[fileID]; !exists {
			return nil, fmt.Errorf("drive: 分享文件不属于当前分享会话: %s", fileID)
		}
	}
	return sd.SaveShare(context.Background(), c, session, fileIDs, toParentID)
}
