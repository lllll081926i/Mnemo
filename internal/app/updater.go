package app

import (
	"context"
	"path/filepath"

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
func (a *App) DownloadUpdate(url string) (string, error) {
	if url == "" {
		// resolve from CheckUpdate if not provided
		info, err := updater.Check(context.Background())
		if err != nil || info == nil {
			return "", err
		}
		url = info.URL
	}
	dest := filepath.Join(updater.DownloadDir(a.dataDir), "mnemo-update")
	go func() {
		_, err := updater.Download(context.Background(), url, dest, func(p updater.Progress) {
			a.emit("update:progress", p)
		})
		if err != nil {
			a.emit("update:progress", map[string]any{"error": err.Error()})
		} else {
			a.emit("update:done", map[string]any{"path": dest})
		}
	}()
	return dest, nil
}

// ApplyUpdate launches the downloaded installer and quits.
func (a *App) ApplyUpdate(path string) error {
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
