package app

import (
	"context"
	"fmt"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/store"
)

// ---- drive file operations (thin pass-through to the drive facade) ----

// ListDir lists a directory.
func (a *App) ListDir(userID, driveID, dirID string) ([]model.File, error) {
	files, err := drive.ListDir(userID, driveID, dirID, nil)
	if err == nil {
		drive.RememberListedFiles(driveID, dirID, files)
	}
	return files, err
}

// ListDirPage lists one page.
func (a *App) ListDirPage(userID, driveID, dirID, marker string) (*drive.DirPage, error) {
	return drive.ListDirPage(userID, driveID, dirID, marker, nil)
}

// SearchFiles searches a drive.
func (a *App) SearchFiles(userID, driveID, keyword string) ([]model.File, error) {
	return drive.SearchDir(userID, driveID, keyword)
}

// ListTrash lists the recycle bin.
func (a *App) ListTrash(userID, driveID string) ([]model.File, error) {
	return drive.ListTrash(userID, driveID, nil)
}

// GetFileDetail returns a file's unified model.
func (a *App) GetFileDetail(userID, driveID, fileID string) (*model.File, error) {
	return drive.GetFile(userID, driveID, fileID)
}

// GetDownloadURL returns a file's download url.
func (a *App) GetDownloadURL(userID, driveID, fileID string) (*model.DownloadURL, error) {
	return drive.GetDownloadURL(userID, driveID, fileID, 14400)
}

// GetVideoPreview returns playback sources.
func (a *App) GetVideoPreview(userID, driveID, fileID string) (*model.VideoPreview, error) {
	return drive.GetVideoPreview(userID, driveID, fileID)
}

// Mkdir creates a folder.
func (a *App) Mkdir(userID, driveID, parentID, name string) (*drive.MkdirResult, error) {
	return drive.Mkdir(userID, driveID, parentID, name)
}

// RenameFile renames a file.
func (a *App) RenameFile(userID, driveID, fileID, name string) (*drive.RenameResult, error) {
	res, err := drive.RenameBatch(userID, driveID, []drive.FileRef{{ID: fileID}}, []string{name})
	if err != nil {
		return nil, err
	}
	if len(res) == 0 {
		return &drive.RenameResult{FileID: fileID, Name: name}, nil
	}
	return &res[0], nil
}

// RenameBatch renames files in batch.
func (a *App) RenameBatch(userID, driveID string, fileRefs []drive.FileRef, names []string) ([]drive.RenameResult, error) {
	return drive.RenameBatch(userID, driveID, fileRefs, names)
}

// TrashFiles moves files to the recycle bin.
func (a *App) TrashFiles(userID, driveID string, fileIDs []string) ([]string, error) {
	return drive.TrashBatch(userID, driveID, fileIDs)
}

// DeleteFiles permanently deletes files.
func (a *App) DeleteFiles(userID, driveID string, fileIDs []string) ([]string, error) {
	refs := make([]drive.FileRef, 0, len(fileIDs))
	for _, id := range fileIDs {
		refs = append(refs, drive.FileRef{ID: id})
	}
	return drive.DeleteBatch(userID, driveID, refs)
}

// RestoreFiles restores files from trash.
func (a *App) RestoreFiles(userID, driveID string, fileIDs []string) ([]string, error) {
	return drive.RestoreBatch(userID, driveID, fileIDs)
}

// MoveFiles moves files to a folder.
func (a *App) MoveFiles(userID, driveID string, fileIDs []string, toParentID string) ([]string, error) {
	refs := make([]drive.FileRef, 0, len(fileIDs))
	for _, id := range fileIDs {
		refs = append(refs, drive.FileRef{ID: id})
	}
	return drive.MoveBatch(userID, driveID, refs, toParentID, "")
}

// CopyFiles copies files into a folder.
func (a *App) CopyFiles(userID, driveID string, fileIDs []string, toParentID string) ([]string, error) {
	refs := make([]drive.FileRef, 0, len(fileIDs))
	for _, id := range fileIDs {
		refs = append(refs, drive.FileRef{ID: id})
	}
	return drive.CopyBatch(userID, driveID, refs, toParentID, "")
}

// FavoriteFiles toggles favorite on files.
func (a *App) FavoriteFiles(userID, driveID string, favorite bool, fileIDs []string) ([]string, error) {
	return drive.FavoriteBatch(userID, driveID, favorite, fileIDs)
}

// CreateShare creates a share.
func (a *App) CreateShare(userID, driveID string, params drive.ShareParams) (*model.ShareItem, error) {
	item, err := drive.CreateShare(userID, driveID, params)
	if err == nil && item != nil {
		_ = a.store.SaveShareHistory(model.ShareHistoryEntry{
			ShareID: item.ShareID, AccountID: userID, DriveID: driveID,
			FileID: firstFileID(params.FileIDs), ShareURL: item.ShareURL,
			SharePwd: item.SharePwd, ShareName: item.ShareName, Provider: drive.ProviderOf(userID, driveID, ""),
		})
	}
	return item, err
}

// ListShareHistory lists persisted share history.
func (a *App) ListShareHistory(userID string) []model.ShareHistoryEntry {
	list, err := a.store.ListShareHistory(userID)
	if err != nil {
		return nil
	}
	return list
}

// ---- local tags & favorites ----

// ListLocalTags lists local tags for an account.
func (a *App) ListLocalTags(userID, driveID string) []store.LocalTag {
	list, err := a.store.ListLocalTags(userID, driveID)
	if err != nil {
		return nil
	}
	return list
}

// SaveLocalTag upserts a local tag.
func (a *App) SaveLocalTag(t store.LocalTag) error {
	return a.store.UpsertLocalTag(t)
}

// DeleteLocalTag removes a local tag.
func (a *App) DeleteLocalTag(userID, driveID, fileID string) error {
	return a.store.DeleteLocalTag(userID, driveID, fileID)
}

// ListFavorites lists local favorites.
func (a *App) ListFavorites(userID, driveID string) []store.Favorite {
	list, err := a.store.ListFavorites(userID, driveID)
	if err != nil {
		return nil
	}
	return list
}

// AddFavorite adds a favorite.
func (a *App) AddFavorite(userID, driveID string, f store.Favorite) error {
	return a.store.AddFavorite(f)
}

// RemoveFavorite removes a favorite.
func (a *App) RemoveFavorite(userID, driveID, fileID string) error {
	return a.store.RemoveFavorite(userID, driveID, fileID)
}

// ---- offline download (PikPak cloud) ----

// OfflineDownload submits a PikPak cloud offline task.
func (a *App) OfflineDownload(userID, driveID, url, fileName string) (*model.OfflineTask, error) {
	return a.offlineCreate(userID, driveID, url, fileName)
}

// offlineCreate creates an offline task through the pikpak driver.
func (a *App) offlineCreate(userID, driveID, url, fileName string) (*model.OfflineTask, error) {
	ctx := context.Background()
	c, err := drive.BuildContext(userID, driveID, "")
	if err != nil {
		return nil, err
	}
	d, err := drive.DriverFor(c)
	if err != nil {
		return nil, err
	}
	type offlineDriver interface {
		OfflineCreate(ctx context.Context, url, fileName, parentID string) (taskID, fileID string, err error)
	}
	od, ok := d.(offlineDriver)
	if !ok {
		return nil, fmt.Errorf("当前网盘不支持云离线")
	}
	taskID, fileID, err := od.OfflineCreate(ctx, url, fileName, "")
	if err != nil {
		return nil, err
	}
	t := &model.OfflineTask{
		ID: taskID, UserID: userID, DriveID: driveID,
		TaskID: taskID, URL: url, FileName: fileName,
		Status: "running",
	}
	if fileID != "" {
		t.FileName = fileID
	}
	_ = a.store.SaveOfflineTask(t)
	return t, nil
}

// ListOfflineTasks lists offline tasks.
func (a *App) ListOfflineTasks(userID string) []model.OfflineTask {
	list, err := a.store.ListOfflineTasks()
	if err != nil {
		return nil
	}
	var out []model.OfflineTask
	for _, t := range list {
		if userID == "" || t.UserID == userID {
			out = append(out, t)
		}
	}
	return out
}

func firstFileID(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

var _ = context.Background
