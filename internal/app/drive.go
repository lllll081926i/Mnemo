package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/drive/providers/pikpak"
	"mnemo-go/internal/logging"
	"mnemo-go/internal/model"
	"mnemo-go/internal/store"
)

// ---- drive file operations (thin pass-through to the drive facade) ----

// actionLogFields creates the compact, privacy-safe context shared by all
// user-triggered operations. High-frequency reads intentionally keep using
// debug logs at their call sites; this helper is for actions users expect to
// see completed or failed in the log.
func actionLogFields(page, userID, driveID string, fields ...any) []any {
	args := []any{"page", page}
	if provider := drive.ProviderOf(userID, driveID, ""); provider != "" {
		args = append(args, "provider", provider)
	}
	if strings.TrimSpace(userID) != "" {
		args = append(args, "account_id", redactID(userID))
	}
	if strings.TrimSpace(driveID) != "" {
		args = append(args, "drive_id", redactID(driveID))
	}
	return append(args, fields...)
}

func logActionStarted(action, page, userID, driveID string, fields ...any) time.Time {
	started := time.Now()
	logging.Info(action+"开始", actionLogFields(page, userID, driveID, fields...)...)
	return started
}

func logActionFinished(action, page, userID, driveID string, started time.Time, err error, fields ...any) {
	args := actionLogFields(page, userID, driveID, fields...)
	args = append(args, "duration", logging.Duration(started))
	if err != nil {
		args = append(args, "error", err)
		logging.Warn(action+"失败", args...)
		return
	}
	logging.Info(action+"完成", args...)
}

// SaveCloudTextFile writes text content to an isolated temporary file and
// waits until the corresponding remote upload reaches a terminal state. The
// ordinary upload queue remains asynchronous; only this user-facing save flow
// needs a definitive success/failure result.
func (a *App) SaveCloudTextFile(userID, driveID, parentID, fileName, content string) (retErr error) {
	started := logActionStarted("保存云端文本", "pan", userID, driveID,
		"name_length", len([]rune(strings.TrimSpace(fileName))), "content_bytes", len(content))
	jobID := ""
	defer func() {
		fields := []any{}
		if jobID != "" {
			fields = append(fields, "job_id", redactID(jobID))
		}
		logActionFinished("保存云端文本", "pan", userID, driveID, started, retErr, fields...)
	}()
	name := filepath.Base(strings.TrimSpace(fileName))
	if name == "." || name == ".." || name == "" || name == string(filepath.Separator) {
		return fmt.Errorf("文件名无效")
	}
	tmpPath, err := writeCloudTextUploadTemp(name, content)
	if err != nil {
		return err
	}
	uploads := a.uploadQueue()
	if uploads == nil {
		removeCloudTextUploadTemp(tmpPath)
		return fmt.Errorf("上传服务未启动")
	}
	parentID = canonicalUploadParent(userID, driveID, parentID)
	created := uploads.AddFiles(userID, driveID, parentID, "overwrite", []string{tmpPath})
	if len(created) == 0 {
		removeCloudTextUploadTemp(tmpPath)
		return fmt.Errorf("文件未能加入上传队列")
	}
	if !uploads.MarkCleanupOnSuccess(created[0].UploadID, tmpPath) {
		logging.Warn("cloud text temp cleanup marker could not be attached", "job_id", created[0].UploadID)
	}
	jobID = created[0].UploadID
	if _, waitErr := uploads.Wait(jobID, 30*time.Minute); waitErr != nil {
		return fmt.Errorf("云端保存失败: %w", waitErr)
	}
	return nil
}

func writeCloudTextUploadTemp(fileName, content string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "mnemo_edit_")
	if err != nil {
		return "", fmt.Errorf("创建编辑临时目录失败: %w", err)
	}
	tmpPath := filepath.Join(tmpDir, fileName)
	if err := os.WriteFile(tmpPath, []byte(content), 0o600); err != nil {
		_ = os.Remove(tmpDir)
		return "", fmt.Errorf("写入编辑临时文件失败: %w", err)
	}
	return tmpPath, nil
}

func removeCloudTextUploadTemp(path string) {
	if path == "" {
		return
	}
	_ = os.Remove(path)
	_ = os.Remove(filepath.Dir(path))
}

// ListDir lists a directory.
func (a *App) ListDir(userID, driveID, dirID string) ([]model.File, error) {
	started := time.Now()
	logging.Debug("directory listing started", "account_id", redactID(userID), "drive_id", redactID(driveID), "directory_id", redactID(dirID))
	files, err := drive.ListDir(userID, driveID, dirID, nil)
	if err == nil {
		drive.RememberListedFiles(userID, driveID, dirID, files)
		logging.Debug("directory listing completed", "count", len(files), "duration", logging.Duration(started))
	} else {
		logging.Warn("directory listing failed", "provider", drive.ProviderOf(userID, driveID, ""), "error", err, "duration", logging.Duration(started))
	}
	return files, err
}

// ListDirPage lists one page.
func (a *App) ListDirPage(userID, driveID, dirID, marker string) (*drive.DirPage, error) {
	page, err := drive.ListDirPage(userID, driveID, dirID, marker, nil)
	if err == nil && page != nil {
		drive.RememberListedFiles(userID, driveID, dirID, page.Items)
	}
	return page, err
}

// SearchFiles searches a drive.
func (a *App) SearchFiles(userID, driveID, keyword string) ([]model.File, error) {
	logging.Debug("file search started", "account_id", redactID(userID), "drive_id", redactID(driveID), "keyword_length", len(keyword))
	files, err := drive.SearchDir(userID, driveID, keyword)
	if err != nil {
		logging.Warn("file search failed", "error", err)
	} else {
		logging.Debug("file search completed", "count", len(files))
	}
	return files, err
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
	logging.Debug("download URL resolution started", "account_id", redactID(userID), "file_id", redactID(fileID))
	result, err := drive.GetDownloadURL(userID, driveID, fileID, 14400)
	if err != nil {
		logging.Warn("download URL resolution failed", "error", err)
	} else if result != nil {
		logging.Debug("download URL resolved", "expire_time", result.ExpireTime)
	}
	return result, err
}

// GetVideoPreview returns playback sources.
func (a *App) GetVideoPreview(userID, driveID, fileID string) (*model.VideoPreview, error) {
	logging.Debug("video preview resolution started", "account_id", redactID(userID), "file_id", redactID(fileID))
	preview, err := drive.GetVideoPreview(userID, driveID, fileID)
	if err != nil {
		logging.Warn("video preview resolution failed", "error", err)
	} else if preview != nil {
		logging.Debug("video preview resolved", "quality_count", len(preview.Qualities))
	}
	return preview, err
}

// Mkdir creates a folder.
func (a *App) Mkdir(userID, driveID, parentID, name string) (*drive.MkdirResult, error) {
	started := logActionStarted("创建文件夹", "pan", userID, driveID,
		"name_length", len([]rune(strings.TrimSpace(name))))
	result, err := drive.Mkdir(userID, driveID, parentID, name)
	logActionFinished("创建文件夹", "pan", userID, driveID, started, err)
	return result, err
}

// RenameFile renames a file.
func (a *App) RenameFile(userID, driveID, fileID, name string) (*drive.RenameResult, error) {
	res, err := a.renameFiles(userID, driveID, []drive.FileRef{{ID: fileID}}, []string{name})
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
	return a.renameFiles(userID, driveID, fileRefs, names)
}

func (a *App) renameFiles(userID, driveID string, fileRefs []drive.FileRef, names []string) ([]drive.RenameResult, error) {
	started := logActionStarted("文件重命名", "pan", userID, driveID, "requested_count", len(fileRefs))
	result, err := drive.RenameBatch(userID, driveID, fileRefs, names)
	logActionFinished("文件重命名", "pan", userID, driveID, started, err, "completed_count", len(result))
	return result, err
}

// TrashFiles moves files to the recycle bin.
func (a *App) TrashFiles(userID, driveID string, fileIDs []string) ([]string, error) {
	started := logActionStarted("移入回收站", "pan", userID, driveID, "requested_count", len(fileIDs))
	result, err := drive.TrashBatch(userID, driveID, fileIDs)
	logActionFinished("移入回收站", "pan", userID, driveID, started, err, "completed_count", len(result))
	return result, err
}

// DeleteFiles permanently deletes files.
func (a *App) DeleteFiles(userID, driveID string, fileIDs []string) ([]string, error) {
	started := logActionStarted("彻底删除文件", "pan", userID, driveID, "requested_count", len(fileIDs))
	refs := make([]drive.FileRef, 0, len(fileIDs))
	for _, id := range fileIDs {
		refs = append(refs, drive.FileRef{ID: id})
	}
	result, err := drive.DeleteBatch(userID, driveID, refs)
	logActionFinished("彻底删除文件", "pan", userID, driveID, started, err, "completed_count", len(result))
	return result, err
}

// RestoreFiles restores files from trash.
func (a *App) RestoreFiles(userID, driveID string, fileIDs []string) ([]string, error) {
	started := logActionStarted("还原文件", "pan", userID, driveID, "requested_count", len(fileIDs))
	result, err := drive.RestoreBatch(userID, driveID, fileIDs)
	logActionFinished("还原文件", "pan", userID, driveID, started, err, "completed_count", len(result))
	return result, err
}

// MoveFiles moves files to a folder.
func (a *App) MoveFiles(userID, driveID string, fileIDs []string, toParentID string) ([]string, error) {
	started := logActionStarted("移动文件", "pan", userID, driveID, "requested_count", len(fileIDs))
	refs := make([]drive.FileRef, 0, len(fileIDs))
	for _, id := range fileIDs {
		refs = append(refs, drive.FileRef{ID: id})
	}
	result, err := drive.MoveBatch(userID, driveID, refs, toParentID, "")
	logActionFinished("移动文件", "pan", userID, driveID, started, err, "completed_count", len(result))
	return result, err
}

// CopyFiles copies files into a folder.
func (a *App) CopyFiles(userID, driveID string, fileIDs []string, toParentID string) ([]string, error) {
	started := logActionStarted("复制文件", "pan", userID, driveID, "requested_count", len(fileIDs))
	refs := make([]drive.FileRef, 0, len(fileIDs))
	for _, id := range fileIDs {
		refs = append(refs, drive.FileRef{ID: id})
	}
	result, err := drive.CopyBatch(userID, driveID, refs, toParentID, "")
	logActionFinished("复制文件", "pan", userID, driveID, started, err, "completed_count", len(result))
	return result, err
}

// FavoriteFiles toggles favorite on files.
func (a *App) FavoriteFiles(userID, driveID string, favorite bool, fileIDs []string) ([]string, error) {
	started := logActionStarted("同步云端收藏", "pan", userID, driveID,
		"favorite", favorite, "requested_count", len(fileIDs))
	result, err := drive.FavoriteBatch(userID, driveID, favorite, fileIDs)
	logActionFinished("同步云端收藏", "pan", userID, driveID, started, err, "completed_count", len(result))
	return result, err
}

// CreateShare creates a share.
func (a *App) CreateShare(userID, driveID string, params drive.ShareParams) (*model.ShareItem, error) {
	started := logActionStarted("创建分享", "share", userID, driveID,
		"file_count", len(params.FileIDs), "expiration", params.Expiration,
		"has_password", strings.TrimSpace(params.Password) != "")
	item, err := drive.CreateShare(userID, driveID, params)
	if err != nil {
		logActionFinished("创建分享", "share", userID, driveID, started, err)
		return item, err
	}
	historySaved := false
	if shouldPersistShareHistory(item) {
		st, storeErr := a.storeOrError()
		if storeErr != nil {
			a.emit("share:history-error", map[string]string{"error": storeErr.Error()})
		} else {
			storeErr = st.SaveShareHistory(model.ShareHistoryEntry{
				ShareID: item.ShareID, AccountID: userID, DriveID: driveID,
				FileID: firstFileID(params.FileIDs), ShareURL: item.ShareURL,
				SharePwd: item.SharePwd, ShareName: item.ShareName, Provider: drive.ProviderOf(userID, driveID, ""),
			})
			if storeErr != nil {
				a.emit("share:history-error", map[string]string{"error": storeErr.Error()})
			} else {
				historySaved = true
			}
		}
	}
	logActionFinished("创建分享", "share", userID, driveID, started, nil,
		"history_saved", historySaved, "share_id", redactShareID(item))
	return item, nil
}

// CancelShare revokes a share on the provider before removing its local
// history item. It never presents a local-only deletion as a cloud revocation.
func (a *App) CancelShare(entry model.ShareHistoryEntry) error {
	userID := strings.TrimSpace(entry.AccountID)
	driveID := strings.TrimSpace(entry.DriveID)
	if userID == "" || driveID == "" {
		return fmt.Errorf("分享记录缺少账号信息，无法取消")
	}
	if err := validateShareRecordProvider(entry, drive.ProviderOf(userID, driveID, "")); err != nil {
		return err
	}
	started := logActionStarted("取消分享", "share", userID, driveID, "share_id", redactID(entry.ShareID))
	if err := drive.CancelShare(userID, driveID, entry); err != nil {
		logActionFinished("取消分享", "share", userID, driveID, started, err, "share_id", redactID(entry.ShareID))
		return err
	}
	st, err := a.storeOrError()
	if err != nil {
		wrapped := fmt.Errorf("云端分享已取消，但无法清理本地记录: %w", err)
		logActionFinished("取消分享", "share", userID, driveID, started, wrapped, "share_id", redactID(entry.ShareID))
		return wrapped
	}
	if err := st.DeleteShareHistory(userID, entry.ShareID, entry.ShareURL); err != nil {
		wrapped := fmt.Errorf("云端分享已取消，但本地记录清理失败: %w", err)
		logActionFinished("取消分享", "share", userID, driveID, started, wrapped, "share_id", redactID(entry.ShareID))
		return wrapped
	}
	a.emit("share:history-changed", map[string]string{"account_id": userID, "share_id": entry.ShareID})
	logActionFinished("取消分享", "share", userID, driveID, started, nil, "share_id", redactID(entry.ShareID))
	return nil
}

func redactShareID(item *model.ShareItem) string {
	if item == nil || strings.TrimSpace(item.ShareID) == "" {
		return ""
	}
	return redactID(item.ShareID)
}

// validateShareRecordProvider rejects a tampered or stale history entry before
// it can be sent to a different provider's destructive cancellation endpoint.
// Empty Provider is retained for history records created by older versions.
func validateShareRecordProvider(entry model.ShareHistoryEntry, accountProvider string) error {
	recorded := strings.TrimSpace(entry.Provider)
	if recorded == "" || recorded == accountProvider {
		return nil
	}
	return fmt.Errorf("分享记录与账号网盘不匹配，无法取消")
}

// shouldPersistShareHistory excludes expiring bearer-style URLs. S3
// presigned links contain an access signature and become stale on expiry, so
// keeping them in the permanent local share log is both misleading and an
// unnecessary credential exposure.
func shouldPersistShareHistory(item *model.ShareItem) bool {
	return item != nil && strings.TrimSpace(item.SharePolicy) != "presigned"
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
	started := logActionStarted("解析分享链接", "share", userID, driveID,
		"url_host", urlHost(shareURL), "has_password", strings.TrimSpace(password) != "")
	session, err := drive.ImportShare(userID, driveID, shareURL, password)
	if err != nil {
		logActionFinished("解析分享链接", "share", userID, driveID, started, err)
		return nil, err
	}
	logActionFinished("解析分享链接", "share", userID, driveID, started, nil, "file_count", len(session.Files))
	return session, nil
}

// SaveImportedShare transfers selected files from a parsed share session
// into the account's folder toParentID.
func (a *App) SaveImportedShare(userID, driveID string, session *drive.ShareImportSession, fileIDs []string, toParentID string) ([]string, error) {
	started := logActionStarted("保存分享文件", "share", userID, driveID, "requested_count", len(fileIDs))
	result, err := drive.SaveImportedShare(userID, driveID, session, fileIDs, toParentID)
	logActionFinished("保存分享文件", "share", userID, driveID, started, err, "completed_count", len(result))
	return result, err
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
	started := logActionStarted("保存本地标签", "pan", t.UserID, t.DriveID,
		"name_length", len([]rune(strings.TrimSpace(t.Name))))
	st, err := a.storeOrError()
	if err != nil {
		logActionFinished("保存本地标签", "pan", t.UserID, t.DriveID, started, err)
		return err
	}
	err = st.UpsertLocalTag(t)
	logActionFinished("保存本地标签", "pan", t.UserID, t.DriveID, started, err)
	return err
}

// DeleteLocalTag removes a local tag.
func (a *App) DeleteLocalTag(userID, driveID, fileID string) error {
	started := logActionStarted("删除本地标签", "pan", userID, driveID)
	st, err := a.storeOrError()
	if err != nil {
		logActionFinished("删除本地标签", "pan", userID, driveID, started, err)
		return err
	}
	err = st.DeleteLocalTag(userID, driveID, fileID)
	logActionFinished("删除本地标签", "pan", userID, driveID, started, err)
	return err
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
	started := logActionStarted("加入本地收藏", "pan", userID, driveID)
	st, err := a.storeOrError()
	if err != nil {
		logActionFinished("加入本地收藏", "pan", userID, driveID, started, err)
		return err
	}
	err = st.AddFavorite(f)
	logActionFinished("加入本地收藏", "pan", userID, driveID, started, err)
	return err
}

// RemoveFavorite removes a favorite.
func (a *App) RemoveFavorite(userID, driveID, fileID string) error {
	started := logActionStarted("移出本地收藏", "pan", userID, driveID)
	st, err := a.storeOrError()
	if err != nil {
		logActionFinished("移出本地收藏", "pan", userID, driveID, started, err)
		return err
	}
	err = st.RemoveFavorite(userID, driveID, fileID)
	logActionFinished("移出本地收藏", "pan", userID, driveID, started, err)
	return err
}

// ---- offline download (PikPak cloud) ----

// OfflineDownload submits a PikPak cloud offline task.
func (a *App) OfflineDownload(userID, driveID, url, fileName string) (*model.OfflineTask, error) {
	started := time.Now()
	baseFields := actionLogFields("transfer", userID, driveID,
		"url_host", urlHost(url), "has_file_name", strings.TrimSpace(fileName) != "")
	logging.Debug("云离线任务创建", baseFields...)
	task, err := a.offlineCreate(userID, driveID, url, fileName)
	fields := append([]any(nil), baseFields...)
	if task != nil {
		fields = append(fields, "task_id", redactID(task.ID))
	}
	fields = append(fields, "duration", logging.Duration(started))
	if err != nil {
		fields = append(fields, "error", err)
		logging.Warn("云离线任务创建失败", fields...)
	} else {
		logging.Debug("云离线任务已创建", fields...)
	}
	return task, err
}

// offlineCreate creates an offline task through the pikpak driver.
func (a *App) offlineCreate(userID, driveID, url, fileName string) (*model.OfflineTask, error) {
	ctx := a.appContext()
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
	ctx := a.appContext()
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
func (a *App) DeleteOfflineTask(userID, driveID, taskID string, deleteFiles bool) (retErr error) {
	started := logActionStarted("删除云离线任务", "transfer", userID, driveID,
		"task_id", redactID(taskID), "delete_files", deleteFiles)
	defer func() {
		logActionFinished("删除云离线任务", "transfer", userID, driveID, started, retErr,
			"task_id", redactID(taskID))
	}()
	ctx := a.appContext()
	c, retErr := drive.BuildContext(userID, driveID, "")
	if retErr != nil {
		return retErr
	}
	d, retErr := drive.DriverFor(c)
	if retErr != nil {
		return retErr
	}
	type deleter interface {
		OfflineDelete(ctx context.Context, c drive.Context, taskIDs []string, deleteFiles bool) error
	}
	del, ok := d.(deleter)
	if !ok {
		return fmt.Errorf("当前网盘不支持删除云离线任务")
	}
	if retErr = del.OfflineDelete(ctx, c, []string{taskID}, deleteFiles); retErr != nil {
		return retErr
	}
	st, retErr := a.storeOrError()
	if retErr != nil {
		return retErr
	}
	retErr = st.DeleteOfflineTask(taskID)
	return retErr
}

func firstFileID(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

// SendGuangyaSms requests a SMS verification code (guangya provider).
func (a *App) SendGuangyaSms(phone string) (map[string]string, error) {
	started := logActionStarted("发送光鸭短信验证码", "login", "", "", "provider", model.ProviderGuangya)
	if !drive.IsRegistered(model.ProviderGuangya) {
		err := fmt.Errorf("光鸭云盘未注册")
		logActionFinished("发送光鸭短信验证码", "login", "", "", started, err, "provider", model.ProviderGuangya)
		return nil, err
	}
	type sender interface {
		SendSms(ctx context.Context, phone string) (verificationID, deviceID, captchaToken string, err error)
	}
	d := drive.New(model.ProviderGuangya)
	if s, ok := d.(sender); ok {
		vid, dev, capTok, err := s.SendSms(a.appContext(), phone)
		if err != nil {
			logActionFinished("发送光鸭短信验证码", "login", "", "", started, err, "provider", model.ProviderGuangya)
			return nil, err
		}
		logActionFinished("发送光鸭短信验证码", "login", "", "", started, nil,
			"provider", model.ProviderGuangya, "has_captcha_token", capTok != "")
		return map[string]string{"verification_id": vid, "device_id": dev, "captcha_token": capTok}, nil
	}
	err := fmt.Errorf("光鸭云盘不支持发送验证码")
	logActionFinished("发送光鸭短信验证码", "login", "", "", started, err, "provider", model.ProviderGuangya)
	return nil, err
}
