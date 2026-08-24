package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/logging"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
	"mnemo-go/internal/store"
)

// GetSettings loads app settings.
func (a *App) GetSettings() store.Settings {
	st, err := a.storeOrError()
	if err != nil {
		return store.Settings{}
	}
	s, _ := st.GetSettings()
	return s
}

// SaveSettings persists settings and applies runtime-relevant changes.
func (a *App) SaveSettings(s store.Settings) (retErr error) {
	started := logActionStarted("保存设置", "settings", "", "",
		"download_dir_configured", strings.TrimSpace(s.DownloadDir) != "",
		"proxy_configured", strings.TrimSpace(s.Proxy) != "",
		"max_concurrent_downloads", s.MaxConcurrentDownloads,
		"max_concurrent_uploads", s.MaxConcurrentUploads,
		"max_upload_speed", s.MaxUploadSpeed)
	defer func() {
		logActionFinished("保存设置", "settings", "", "", started, retErr,
			"max_concurrent_downloads", s.MaxConcurrentDownloads,
			"max_concurrent_uploads", s.MaxConcurrentUploads,
			"max_upload_speed", s.MaxUploadSpeed)
	}()
	st, retErr := a.storeOrError()
	if retErr != nil {
		return retErr
	}
	if s.LogLevel == "" {
		s.LogLevel = "info"
	}
	if retErr = logging.SetLevel(s.LogLevel); retErr != nil {
		return retErr
	}
	if retErr = st.SetSettings(s); retErr != nil {
		return retErr
	}
	dl := a.downloadManager()
	uploads := a.uploadQueue()
	mediaProxy := a.previewServer()
	// apply download directory at runtime
	if s.DownloadDir != "" && dl != nil {
		dl.SetDir(s.DownloadDir)
		if mediaProxy != nil {
			mediaProxy.SetRoots(s.DownloadDir)
		}
	}
	// apply concurrency limit at runtime
	if s.MaxConcurrentDownloads > 0 && dl != nil {
		dl.SetConcurrency(s.MaxConcurrentDownloads)
	}
	if s.MaxConcurrentUploads > 0 && uploads != nil {
		uploads.SetConcurrency(s.MaxConcurrentUploads)
	}
	// apply proxy globally (affects netx clients + download engine)
	netx.SetGlobalProxy(s.Proxy)
	// apply upload speed cap at runtime (direct uploads via ProgressReader)
	netx.SetGlobalUploadRate(s.MaxUploadSpeed)
	return nil
}

// GetLogPath returns the active persistent log file path.
func (a *App) GetLogPath() string { return logging.Path() }

// LogFrontend forwards structured frontend diagnostics into the same
// persistent log used by backend operations. The message and fields are
// sanitized by the logging package before they are written.
func (a *App) LogFrontend(level, scope, message string, fields map[string]string) {
	args := make([]any, 0, len(fields)*2+2)
	args = append(args, "scope", scope)
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := fields[key]
		args = append(args, key, value)
	}
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "error":
		logging.Error(message, args...)
	case "warning", "warn":
		logging.Warn(message, args...)
	case "debug":
		logging.Debug(message, args...)
	default:
		logging.Info(message, args...)
	}
}

// ClearLogs removes the active log and starts a fresh file.
func (a *App) ClearLogs() error {
	err := logging.Clear()
	if err != nil {
		logging.Warn("清理日志失败", "page", "settings", "error", err)
		return err
	}
	// This line intentionally comes after Clear so it remains as the first
	// actionable entry in the newly created log file.
	logging.Info("清理日志完成", "page", "settings")
	return nil
}

// ExportLogs copies the active log to a user-selected destination and returns
// the destination path. The dialog is intentionally native for all platforms.
func (a *App) ExportLogs() (destination string, retErr error) {
	started := logActionStarted("导出日志", "settings", "", "")
	defer func() {
		if destination == "" && retErr == nil {
			logging.Info("导出日志已取消", "page", "settings")
			return
		}
		fields := []any{}
		if destination != "" {
			fields = append(fields, "file_name", filepath.Base(destination))
		}
		logActionFinished("导出日志", "settings", "", "", started, retErr, fields...)
	}()
	path := logging.Path()
	if path == "" {
		return "", errors.New("persistent logging is not initialized")
	}
	ctx, ok := a.wailsContext()
	if !ok {
		return "", errors.New("application UI is not initialized")
	}
	destination, retErr = runtime.SaveFileDialog(ctx, runtime.SaveDialogOptions{
		Title:           "Export Mnemo Log",
		DefaultFilename: "mnemo.log",
		Filters:         []runtime.FileFilter{{DisplayName: "Log files (*.log)", Pattern: "*.log"}},
	})
	if retErr != nil || destination == "" {
		return "", retErr
	}
	data, retErr := os.ReadFile(path)
	if retErr != nil {
		return "", retErr
	}
	if retErr = os.WriteFile(destination, data, 0o600); retErr != nil {
		return "", retErr
	}
	return destination, nil
}

// GetDirectoryCache returns a persisted directory snapshot. The key is
// supplied by the frontend and includes provider/account/mode/ directory
// identity, so snapshots from different accounts cannot be mixed.
func (a *App) GetDirectoryCache(key string) []model.File {
	st, err := a.storeOrError()
	if err != nil || strings.TrimSpace(key) == "" {
		return nil
	}
	list, err := st.LoadDirectoryCache(key)
	if err != nil {
		return nil
	}
	return list
}

// SaveDirectoryCache stores one directory snapshot under the install-local
// data/cache directory.
func (a *App) SaveDirectoryCache(key string, files []model.File) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("缓存键为空")
	}
	st, err := a.storeOrError()
	if err != nil {
		return err
	}
	return st.SaveDirectoryCache(key, files)
}

// DeleteDirectoryCache invalidates one persisted directory snapshot.
func (a *App) DeleteDirectoryCache(key string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("缓存键为空")
	}
	st, err := a.storeOrError()
	if err != nil {
		return err
	}
	return st.DeleteDirectoryCache(key)
}

// ClearCache removes directory/list cache only; account credentials and
// transfer/playback history remain intact.
func (a *App) ClearCache() error {
	started := logActionStarted("清理缓存", "settings", "", "")
	st, err := a.storeOrError()
	if err != nil {
		logActionFinished("清理缓存", "settings", "", "", started, err)
		return err
	}
	if err := st.ClearCache(); err != nil {
		logActionFinished("清理缓存", "settings", "", "", started, err)
		return err
	}
	drive.ClearFileMetaCache()
	a.emit("cache:cleared", nil)
	logActionFinished("清理缓存", "settings", "", "", started, nil)
	return nil
}
