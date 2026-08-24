package app

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"mnemo-go/internal/updater"
)

// CheckUpdateResult is the payload returned to the frontend.
type CheckUpdateResult struct {
	Available bool   `json:"available"`
	Version   string `json:"version"`
	URL       string `json:"url"`
	Size      int64  `json:"size"`
	Notes     string `json:"notes"`
}

func cloneUpdateInfo(info *updater.Info) *updater.Info {
	if info == nil {
		return nil
	}
	copy := *info
	return &copy
}

// CheckUpdate queries GitHub for a newer release.
func (a *App) CheckUpdate() (*CheckUpdateResult, error) {
	info, err := updater.Check(a.appContext())
	if err != nil {
		return &CheckUpdateResult{Available: false}, err
	}
	a.updateMu.Lock()
	a.updateInfo = cloneUpdateInfo(info)
	a.updateMu.Unlock()
	if info == nil {
		return &CheckUpdateResult{Available: false}, nil
	}
	return &CheckUpdateResult{
		Available: true,
		Version:   info.Version,
		URL:       info.URL,
		Size:      info.Size,
		Notes:     info.Notes,
	}, nil
}

// DownloadUpdate downloads the update and emits "update:progress" events.
func (a *App) DownloadUpdate(downloadURL string) (string, error) {
	started := logActionStarted("下载更新", "settings", "", "", "url_host", urlHost(downloadURL))
	a.updateMu.Lock()
	info := cloneUpdateInfo(a.updateInfo)
	a.updateMu.Unlock()
	// The silent check already fetched and checksum-validated the release
	// metadata. Reuse it to avoid a second GitHub request at the moment the
	// user clicks “下载并安装”. A direct call without a cached check still
	// performs one fresh validation.
	if info == nil || (downloadURL != "" && downloadURL != info.URL) {
		checked, checkErr := updater.Check(a.appContext())
		if checkErr != nil {
			logActionFinished("下载更新", "settings", "", "", started, checkErr)
			return "", checkErr
		}
		info = checked
		if info != nil {
			a.updateMu.Lock()
			a.updateInfo = cloneUpdateInfo(info)
			a.updateMu.Unlock()
		}
	}
	if info == nil || info.URL == "" {
		err := fmt.Errorf("no update is available")
		logActionFinished("下载更新", "settings", "", "", started, err)
		return "", err
	}
	if downloadURL != "" && downloadURL != info.URL {
		err := fmt.Errorf("update URL does not match the latest release")
		logActionFinished("下载更新", "settings", "", "", started, err)
		return "", err
	}
	name := "mnemo-update"
	if parsed, parseErr := url.Parse(info.URL); parseErr == nil {
		base := filepath.Base(parsed.Path)
		if strings.HasSuffix(base, ".exe") {
			name += ".exe"
		} else if strings.HasSuffix(base, ".tar.gz") {
			name += ".tar.gz"
		} else if strings.HasSuffix(base, ".zip") {
			name += ".zip"
		} else if strings.HasSuffix(base, ".deb") {
			name += ".deb"
		}
	}
	dest := filepath.Join(updater.DownloadDir(a.dataDirectory()), name)
	ctx := a.appContext()
	go func() {
		_, downloadErr := updater.Download(ctx, info.URL, dest, func(p updater.Progress) {
			if p.Total <= 0 && info.Size > 0 {
				p.Total = info.Size
			}
			a.emit("update:progress", p)
		})
		if downloadErr != nil {
			a.emit("update:progress", map[string]any{"error": downloadErr.Error()})
			logActionFinished("下载更新", "settings", "", "", started, downloadErr,
				"url_host", urlHost(info.URL))
			return
		}
		ok, checksumErr := updater.VerifyChecksum(dest, info.SHA256)
		if checksumErr != nil || !ok {
			_ = os.Remove(dest)
			if checksumErr != nil {
				a.emit("update:progress", map[string]any{"error": fmt.Sprintf("update checksum verification failed: %v", checksumErr)})
				logActionFinished("下载更新", "settings", "", "", started, checksumErr,
					"url_host", urlHost(info.URL))
			} else {
				a.emit("update:progress", map[string]any{"error": "update checksum verification failed"})
				logActionFinished("下载更新", "settings", "", "", started,
					fmt.Errorf("update checksum verification failed"), "url_host", urlHost(info.URL))
			}
		} else {
			actualSize := info.Size
			if stat, statErr := os.Stat(dest); statErr == nil {
				actualSize = stat.Size()
			}
			a.emit("update:done", map[string]any{"path": dest, "size": actualSize, "version": info.Version})
			logActionFinished("下载更新", "settings", "", "", started, nil,
				"url_host", urlHost(info.URL), "size", actualSize, "version", info.Version)
		}
	}()
	return dest, nil
}

// ApplyUpdate launches the downloaded installer and quits.
func (a *App) ApplyUpdate(path string) error {
	started := logActionStarted("安装更新", "settings", "", "")
	if !updater.IsDownloadPath(a.dataDirectory(), path) {
		err := fmt.Errorf("invalid update path")
		logActionFinished("安装更新", "settings", "", "", started, err)
		return err
	}
	go func() {
		if err := updater.Apply(path); err != nil {
			a.emit("update:error", map[string]any{"error": err.Error()})
			logActionFinished("安装更新", "settings", "", "", started, err)
			return
		}
		// quit the app so the installer can replace files
		a.emit("update:applying", nil)
		logActionFinished("安装更新", "settings", "", "", started, nil)
	}()
	return nil
}
