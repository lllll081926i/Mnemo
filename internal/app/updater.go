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

// CheckUpdate queries GitHub for a newer release.
func (a *App) CheckUpdate() (*CheckUpdateResult, error) {
	info, err := updater.Check(context.Background())
	if err != nil {
		return &CheckUpdateResult{Available: false}, err
	}
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
	info, err := updater.Check(context.Background())
	if err != nil {
		return "", err
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
			a.emit("update:done", map[string]any{"path": dest})
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
