package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/drive/providers/pikpak"
	"mnemo-go/internal/model"
	"mnemo-go/internal/store"
)

// ---- drive file operations (thin pass-through to the drive facade) ----

// SaveCloudTextFile writes text content to a temporary file and triggers an upload back to the cloud.
func (a *App) SaveCloudTextFile(userID, driveID, parentID, fileName, content string) error {
	name := filepath.Base(strings.TrimSpace(fileName))
	if name == "." || name == "" || name == string(filepath.Separator) {
		return fmt.Errorf("文件名无效")
	}
	tmpDir := filepath.Join(os.TempDir(), "mnemo_edit")
	_ = os.MkdirAll(tmpDir, 0o755)
	tmpPath := filepath.Join(tmpDir, name)
	if err := os.WriteFile(tmpPath, []byte(content), 0o644); err != nil {
		return err
	}
	uploads := a.uploadQueue()
	if uploads == nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("上传服务未启动")
	}
	if created := uploads.AddFiles(userID, driveID, parentID, []string{tmpPath}); len(created) == 0 {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("文件未能加入上传队列")
	}
	return nil
}

// ListDir lists a directory.
func (a *App) ListDir(userID, driveID, dirID string) ([]model.File, error) {
	files, err := drive.ListDir(userID, driveID, dirID, nil)
	if err == nil {
		drive.RememberListedFiles(userID, driveID, dirID, files)
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
		st, storeErr := a.storeOrError()
		if storeErr != nil {
			a.emit("share:history-error", map[string]string{"error": storeErr.Error()})
			return item, nil
		}
		if storeErr = st.SaveShareHistory(model.ShareHistoryEntry{
			ShareID: item.ShareID, AccountID: userID, DriveID: driveID,
			FileID: firstFileID(params.FileIDs), ShareURL: item.ShareURL,
			SharePwd: item.SharePwd, ShareName: item.ShareName, Provider: drive.ProviderOf(userID, driveID, ""),
		}); storeErr != nil {
			a.emit("share:history-error", map[string]string{"error": storeErr.Error()})
		}
	}
	return item, err
}

// ListShareHistory lists persisted share history.
func (a *App) ListShareHistory(userID string) []model.ShareHistoryEntry {
	st, err := a.storeOrError()
	if err != nil {
		return nil
	}
	list, err := st.ListShareHistory(userID)
	if err != nil {
		return nil
	}
	return list
}

// ImportShare parses a share link and returns the file listing + session
// state for subsequent save. The session must be passed back to
// SaveImportedShare to transfer selected files.
func (a *App) ImportShare(userID, driveID, shareURL, password string) (*drive.ShareImportSession, error) {
	return drive.ImportShare(userID, driveID, shareURL, password)
}

// SaveImportedShare transfers selected files from a parsed share session
// into the account's folder toParentID.
func (a *App) SaveImportedShare(userID, driveID string, session *drive.ShareImportSession, fileIDs []string, toParentID string) ([]string, error) {
	return drive.SaveImportedShare(userID, driveID, session, fileIDs, toParentID)
}

// ---- local tags & favorites ----

// ListLocalTags lists local tags for an account.
func (a *App) ListLocalTags(userID, driveID string) []store.LocalTag {
	st, err := a.storeOrError()
	if err != nil {
		return nil
	}
	list, err := st.ListLocalTags(userID, driveID)
	if err != nil {
		return nil
	}
	return list
}

// SaveLocalTag upserts a local tag.
func (a *App) SaveLocalTag(t store.LocalTag) error {
	st, err := a.storeOrError()
	if err != nil {
		return err
	}
	return st.UpsertLocalTag(t)
}

// DeleteLocalTag removes a local tag.
func (a *App) DeleteLocalTag(userID, driveID, fileID string) error {
	st, err := a.storeOrError()
	if err != nil {
		return err
	}
	return st.DeleteLocalTag(userID, driveID, fileID)
}

// ListFavorites lists local favorites.
func (a *App) ListFavorites(userID, driveID string) []store.Favorite {
	st, err := a.storeOrError()
	if err != nil {
		return nil
	}
	list, err := st.ListFavorites(userID, driveID)
	if err != nil {
		return nil
	}
	return list
}

// AddFavorite adds a favorite.
func (a *App) AddFavorite(userID, driveID string, f store.Favorite) error {
	st, err := a.storeOrError()
	if err != nil {
		return err
	}
	return st.AddFavorite(f)
}

// RemoveFavorite removes a favorite.
func (a *App) RemoveFavorite(userID, driveID, fileID string) error {
	st, err := a.storeOrError()
	if err != nil {
		return err
	}
	return st.RemoveFavorite(userID, driveID, fileID)
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
	localID := taskID
	if localID == "" {
		localID = fileID
	}
	t := &model.OfflineTask{
		ID: localID, UserID: userID, DriveID: driveID,
		TaskID: taskID, FileID: fileID, URL: url, FileName: fileName,
		Status: "running",
	}
	if t.ID == "" {
		return nil, fmt.Errorf("云离线任务响应缺少任务或文件 ID")
	}
	st, err := a.storeOrError()
	if err != nil {
		return nil, err
	}
	if err := st.SaveOfflineTask(t); err != nil {
		return nil, err
	}
	return t, nil
}

// ListOfflineTasks lists offline tasks.
func (a *App) ListOfflineTasks(userID string) []model.OfflineTask {
	st, err := a.storeOrError()
	if err != nil {
		return nil
	}
	list, err := st.ListOfflineTasks()
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

// RefreshOfflineTasks queries the provider for live offline task status and
// updates the local cache. Returns the refreshed list.
func (a *App) RefreshOfflineTasks(userID, driveID string) ([]model.OfflineTask, error) {
	ctx := context.Background()
	c, err := drive.BuildContext(userID, driveID, "")
	if err != nil {
		return nil, err
	}
	d, err := drive.DriverFor(c)
	if err != nil {
		return nil, err
	}
	type lister interface {
		OfflineList(ctx context.Context, c drive.Context) ([]pikpak.OfflineTask, error)
	}
	l, ok := d.(lister)
	if !ok {
		return nil, fmt.Errorf("当前网盘不支持云离线")
	}
	tasks, err := l.OfflineList(ctx, c)
	if err != nil {
		return nil, err
	}
	st, err := a.storeOrError()
	if err != nil {
		return nil, err
	}
	var out []model.OfflineTask
	for _, t := range tasks {
		localID := t.TaskID
		if localID == "" {
			localID = t.FileID
		}
		ot := model.OfflineTask{
			ID: localID, UserID: userID, DriveID: driveID,
			TaskID: t.TaskID, FileID: t.FileID, URL: t.URL, FileName: t.Name,
			Status: firstNonEmptyOfflineStatus(t.Phase, t.Status), Progress: t.Progress,
			Message: t.Message, FileSize: t.FileSize,
			CreatedTime: t.CreatedTime, UpdatedTime: t.UpdatedTime,
		}
		if ot.ID == "" {
			continue
		}
		if err := st.SaveOfflineTask(&ot); err != nil {
			return nil, err
		}
		out = append(out, ot)
	}
	return out, nil
}

func firstNonEmptyOfflineStatus(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return "unknown"
}

// DeleteOfflineTask cancels and removes a PikPak cloud offline task.
func (a *App) DeleteOfflineTask(userID, driveID, taskID string, deleteFiles bool) error {
	ctx := context.Background()
	c, err := drive.BuildContext(userID, driveID, "")
	if err != nil {
		return err
	}
	d, err := drive.DriverFor(c)
	if err != nil {
		return err
	}
	type deleter interface {
		OfflineDelete(ctx context.Context, c drive.Context, taskIDs []string, deleteFiles bool) error
	}
	del, ok := d.(deleter)
	if !ok {
		return fmt.Errorf("当前网盘不支持删除云离线任务")
	}
	if err := del.OfflineDelete(ctx, c, []string{taskID}, deleteFiles); err != nil {
		return err
	}
	st, err := a.storeOrError()
	if err != nil {
		return err
	}
	return st.DeleteOfflineTask(taskID)
}

func firstFileID(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

var _ = context.Background

// SendGuangyaSms requests a SMS verification code (guangya provider).
func (a *App) SendGuangyaSms(phone string) (map[string]string, error) {
	if !drive.IsRegistered(model.ProviderGuangya) {
		return nil, fmt.Errorf("光鸭云盘未注册")
	}
	type sender interface {
		SendSms(ctx context.Context, phone string) (verificationID, deviceID, captchaToken string, err error)
	}
	d := drive.New(model.ProviderGuangya)
	if s, ok := d.(sender); ok {
		vid, dev, capTok, err := s.SendSms(context.Background(), phone)
		if err != nil {
			return nil, err
		}
		return map[string]string{"verification_id": vid, "device_id": dev, "captcha_token": capTok}, nil
	}
	return nil, fmt.Errorf("光鸭云盘不支持发送验证码")
}
