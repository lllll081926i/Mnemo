package app

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/logging"
	"mnemo-go/internal/model"
	"mnemo-go/internal/preview"
)

// ---- transfer bindings ----

// DownloadFile enqueues a download.
func (a *App) DownloadFile(userID, driveID string, f model.File) (*model.DownloadTask, error) {
	started := time.Now()
	baseFields := actionLogFields("pan", userID, driveID, "size", f.Size)
	logging.Debug("下载任务入队", baseFields...)
	dl := a.downloadManager()
	if dl == nil {
		err := errors.New("下载服务未启动")
		logging.Warn("下载任务入队失败", append(baseFields, "duration", logging.Duration(started), "error", err)...)
		return nil, err
	}
	task, err := dl.AddDownload(userID, driveID, f)
	fields := append([]any(nil), baseFields...)
	if task != nil {
		fields = append(fields, "task_id", redactID(task.ID))
	}
	fields = append(fields, "duration", logging.Duration(started))
	if err != nil {
		fields = append(fields, "error", err)
		logging.Warn("下载任务入队失败", fields...)
	} else {
		logging.Debug("下载任务已入队", fields...)
	}
	return task, err
}

// PinFileSnapshot keeps the exact list-row metadata available for a provider
// before preview/player code resolves an authenticated URL by file ID.
func (a *App) PinFileSnapshot(userID string, driveID string, f model.File) {
	drive.RememberFile(userID, driveID, f)
}

// DownloadURL enqueues a direct URL download.
func (a *App) DownloadURL(name, url string, headers map[string]string) (*model.DownloadTask, error) {
	started := time.Now()
	baseFields := []any{
		"page", "transfer", "name_length", len([]rune(strings.TrimSpace(name))),
		"url_host", urlHost(url), "header_count", len(headers),
	}
	logging.Debug("链接下载任务入队", baseFields...)
	dl := a.downloadManager()
	if dl == nil {
		err := errors.New("下载服务未启动")
		logging.Warn("链接下载任务入队失败", append(baseFields, "duration", logging.Duration(started), "error", err)...)
		return nil, err
	}
	task, err := dl.AddDownloadURL(name, url, headers)
	fields := append([]any(nil), baseFields...)
	if task != nil {
		fields = append(fields, "task_id", redactID(task.ID))
	}
	fields = append(fields, "duration", logging.Duration(started))
	if err != nil {
		fields = append(fields, "error", err)
		logging.Warn("链接下载任务入队失败", fields...)
	} else {
		logging.Debug("链接下载任务已入队", fields...)
	}
	return task, err
}

// ListDownloads lists download tasks.
func (a *App) ListDownloads() []model.DownloadTask {
	dl := a.downloadManager()
	if dl == nil {
		return nil
	}
	return dl.List()
}

// runTransferCommand records the requested control action once. Transfer
// workers already record their terminal state, so this deliberately avoids
// emitting per-progress logs.
func (a *App) runTransferCommand(action, taskID string, run func() bool) {
	if strings.TrimSpace(taskID) != "" {
		logging.Debug(action+"请求", "page", "transfer", "task_id", redactID(taskID))
	}
	if !run() {
		started := logActionStarted(action, "transfer", "", "", "task_id", redactID(taskID))
		logActionFinished(action, "transfer", "", "", started, errors.New("传输服务未启动"), "task_id", redactID(taskID))
		return
	}
	a.queueTransferCommandLog(action)
}

func (a *App) queueTransferCommandLog(action string) {
	const settleDelay = 350 * time.Millisecond
	now := time.Now()
	a.transferLogMu.Lock()
	if a.transferCommandLogs == nil {
		a.transferCommandLogs = make(map[string]*transferCommandLog)
	}
	entry := a.transferCommandLogs[action]
	if entry == nil {
		entry = &transferCommandLog{count: 1, started: now}
		a.transferCommandLogs[action] = entry
		entry.timer = time.AfterFunc(settleDelay, func() { a.flushTransferCommandLog(action) })
		a.transferLogMu.Unlock()
		logging.Info(action+"开始", "page", "transfer", "count", 1)
		return
	}
	entry.count++
	entry.timer.Reset(settleDelay)
	a.transferLogMu.Unlock()
}

func (a *App) flushTransferCommandLog(action string) {
	a.transferLogMu.Lock()
	entry := a.transferCommandLogs[action]
	if entry != nil {
		delete(a.transferCommandLogs, action)
	}
	a.transferLogMu.Unlock()
	if entry == nil {
		return
	}
	logging.Info(action+"完成", "page", "transfer", "count", entry.count, "duration", logging.Duration(entry.started))
}

// PauseDownload pauses a task.
func (a *App) PauseDownload(id string) {
	a.runTransferCommand("暂停下载", id, func() bool {
		if dl := a.downloadManager(); dl != nil {
			dl.Pause(id)
			return true
		}
		return false
	})
}

// ResumeDownload resumes a task.
func (a *App) ResumeDownload(id string) {
	a.runTransferCommand("继续下载", id, func() bool {
		if dl := a.downloadManager(); dl != nil {
			dl.Resume(id)
			return true
		}
		return false
	})
}

// CancelDownload cancels a task.
func (a *App) CancelDownload(id string) {
	a.runTransferCommand("取消下载", id, func() bool {
		if dl := a.downloadManager(); dl != nil {
			dl.Cancel(id)
			return true
		}
		return false
	})
}

// RemoveDownload hard-deletes a task record immediately (删除即从列表移除).
func (a *App) RemoveDownload(id string) {
	a.runTransferCommand("删除下载记录", id, func() bool {
		if dl := a.downloadManager(); dl != nil {
			dl.Remove(id)
			return true
		}
		return false
	})
}

// PrioritizeDownload boosts one task: pauses others so it gets full bandwidth.
func (a *App) PrioritizeDownload(id string) {
	a.runTransferCommand("优先下载", id, func() bool {
		if dl := a.downloadManager(); dl != nil {
			dl.Prioritize(id)
			return true
		}
		return false
	})
}

// ClearDownloads removes finished tasks.
func (a *App) ClearDownloads() {
	a.runTransferCommand("清理已完成下载", "", func() bool {
		if dl := a.downloadManager(); dl != nil {
			dl.ClearCompleted()
			return true
		}
		return false
	})
}

// UploadFiles enqueues uploads.
func canonicalUploadParent(userID, driveID, parentID string) string {
	if strings.TrimSpace(parentID) != "" && strings.TrimSpace(parentID) != "root" {
		return parentID
	}
	if root, err := drive.RootID(userID, driveID); err == nil && root != "" {
		return root
	}
	return parentID
}

// UploadFiles enqueues local files or folders for upload. conflictPolicy
// controls behavior when a same-name file already exists remotely:// "overwrite" (default), "rename" (keep both, append suffix), "skip".
func (a *App) UploadFiles(userID, driveID, parentID, conflictPolicy string, localPaths []string) []*model.UploadingUI {
	started := logActionStarted("加入上传队列", "pan", userID, driveID,
		"conflict_policy", conflictPolicy, "path_count", len(localPaths))
	uploads := a.uploadQueue()
	if uploads == nil {
		logActionFinished("加入上传队列", "pan", userID, driveID, started, errors.New("上传服务未启动"))
		return nil
	}
	// The frontend can briefly still hold the generic "root" sentinel while
	// provider metadata is loading. Canonicalize it before the asynchronous
	// queue starts resolving the remote parent.
	parentID = canonicalUploadParent(userID, driveID, parentID)
	items := uploads.AddFiles(userID, driveID, parentID, conflictPolicy, localPaths)
	logActionFinished("加入上传队列", "pan", userID, driveID, started, nil, "item_count", len(items))
	return items
}

// ValidateUploadFiles checks selected local files against the optional target
// provider policy before the frontend opens a conflict dialog or adds queue
// jobs. UploadOneFile validates again when the worker actually starts.
func (a *App) ValidateUploadFiles(userID, driveID string, localPaths []string) error {
	logging.Debug("upload selection validation requested", "account_id", redactID(userID), "drive_id", redactID(driveID), "path_count", len(localPaths))
	items, err := collectUploadValidationItems(localPaths)
	if err != nil {
		return err
	}
	return drive.ValidateUploadItems(userID, driveID, items)
}

func collectUploadValidationItems(localPaths []string) ([]drive.UploadValidationItem, error) {
	if len(localPaths) == 0 {
		return nil, errors.New("未选择上传文件")
	}
	items := make([]drive.UploadValidationItem, 0, len(localPaths))
	for _, rawPath := range localPaths {
		localPath := strings.TrimSpace(rawPath)
		if localPath == "" {
			return nil, errors.New("待上传文件无效")
		}
		info, err := os.Stat(localPath)
		if err != nil {
			return nil, fmt.Errorf("无法读取待上传文件: %w", err)
		}
		if info.IsDir() {
			// Directory contents are enumerated by UploadQueue in the background.
			// Every discovered file is validated again by the provider worker, so a
			// synchronous recursive walk here only blocks the Wails RPC and reads
			// the entire tree twice.
			continue
		}
		items = append(items, drive.UploadValidationItem{Name: info.Name(), Size: info.Size()})
	}
	return items, nil
}

// ListUploads lists upload jobs.
func (a *App) ListUploads() []model.UploadingUI {
	uploads := a.uploadQueue()
	if uploads == nil {
		return nil
	}
	return uploads.List()
}

// CancelUpload cancels an upload.
func (a *App) CancelUpload(id string) {
	a.runTransferCommand("取消上传", id, func() bool {
		if uploads := a.uploadQueue(); uploads != nil {
			uploads.Cancel(id)
			return true
		}
		return false
	})
}

// ClearUploads removes finished uploads.
func (a *App) ClearUploads() {
	a.runTransferCommand("清理已完成上传", "", func() bool {
		if uploads := a.uploadQueue(); uploads != nil {
			uploads.ClearCompleted()
			return true
		}
		return false
	})
}

// ResumeUpload restarts a paused or failed upload job.
func (a *App) ResumeUpload(id string) error {
	started := logActionStarted("继续上传", "transfer", "", "", "task_id", redactID(id))
	uploads := a.uploadQueue()
	if uploads == nil {
		err := errors.New("上传服务未启动")
		logActionFinished("继续上传", "transfer", "", "", started, err, "task_id", redactID(id))
		return err
	}
	err := uploads.Resume(id)
	logActionFinished("继续上传", "transfer", "", "", started, err, "task_id", redactID(id))
	return err
}

// ---- preview ----

// PreviewURL builds a proxied URL for a media file.
func (a *App) PreviewURL(userID, driveID, fileID string) (previewURL string, retErr error) {
	started := logActionStarted("打开文件预览", "pan", userID, driveID)
	defer func() { logActionFinished("打开文件预览", "pan", userID, driveID, started, retErr) }()
	mediaProxy := a.previewServer()
	if mediaProxy == nil {
		return "", fmt.Errorf("预览服务未启动")
	}
	u, retErr := drive.GetDownloadURL(userID, driveID, fileID, 3600)
	if retErr != nil {
		return "", retErr
	}
	name := ""
	if f, ferr := drive.GetFile(userID, driveID, fileID); ferr == nil && f != nil {
		name = f.Name
	}
	previewURL, retErr = mediaProxy.PlaybackURL(preview.PlaybackSource{
		URL:                 u.URL,
		Headers:             u.Headers,
		RequestAuth:         u.RequestAuth,
		AllowPrivateNetwork: u.AllowPrivateNetwork,
		Filename:            name,
	})
	return previewURL, retErr
}

// LocalPreviewURL builds a local file URL.
func (a *App) LocalPreviewURL(path string) string {
	mediaProxy := a.previewServer()
	if mediaProxy == nil {
		return ""
	}
	return mediaProxy.LocalURL(path)
}

// MediaProxy returns the internal server base URL.
func (a *App) MediaProxy() string {
	mediaProxy := a.previewServer()
	if mediaProxy == nil {
		return ""
	}
	return mediaProxy.BaseURL()
}
