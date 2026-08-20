package app

import (
	"context"
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
	info, err := updater.Check(context.Background())
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
	a.updateMu.Lock()
	info := cloneUpdateInfo(a.updateInfo)
	a.updateMu.Unlock()
	// The silent check already fetched and checksum-validated the release
	// metadata. Reuse it to avoid a second GitHub request at the moment the
	// user clicks “下载并安装”. A direct call without a cached check still
	// performs one fresh validation.
	if info == nil || (downloadURL != "" && downloadURL != info.URL) {
		checked, checkErr := updater.Check(context.Background())
		if checkErr != nil {
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
		return "", fmt.Errorf("no update is available")
	}
	if downloadURL != "" && downloadURL != info.URL {
		return "", fmt.Errorf("update URL does not match the latest release")
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
	go func() {
		_, downloadErr := updater.Download(context.Background(), info.URL, dest, func(p updater.Progress) {
			if p.Total <= 0 && info.Size > 0 {
				p.Total = info.Size
			}
			a.emit("update:progress", p)
		})
		if downloadErr != nil {
			a.emit("update:progress", map[string]any{"error": downloadErr.Error()})
			return
		}
		ok, checksumErr := updater.VerifyChecksum(dest, info.SHA256)
		if checksumErr != nil || !ok {
			_ = os.Remove(dest)
			if checksumErr != nil {
				a.emit("update:progress", map[string]any{"error": fmt.Sprintf("update checksum verification failed: %v", checksumErr)})
			} else {
				a.emit("update:progress", map[string]any{"error": "update checksum verification failed"})
			}
		} else {
			actualSize := info.Size
			if stat, statErr := os.Stat(dest); statErr == nil {
				actualSize = stat.Size()
			}
			a.emit("update:done", map[string]any{"path": dest, "size": actualSize, "version": info.Version})
		}
	}()
	return dest, nil
}

// ApplyUpdate launches the downloaded installer and quits.
func (a *App) ApplyUpdate(path string) error {
	if !updater.IsDownloadPath(a.dataDirectory(), path) {
		return fmt.Errorf("invalid update path")
	}
	go func() {
		if err := updater.Apply(path); err != nil {
			a.emit("update:error", map[string]any{"error": err.Error()})
			return
		}
		// quit the app so the installer can replace files
		a.emit("update:applying", nil)
	}()
	return nil
}
