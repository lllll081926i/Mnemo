package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/engine"
	"mnemo-go/internal/model"
	"mnemo-go/internal/player"
	"mnemo-go/internal/store"
)

// playback wraps the mpv session and its state.
type playback struct {
	mu      sync.Mutex
	player  *player.Player
	active  bool
	userID  string
	driveID string
	fileID  string
	name    string
}

// PlayVideo resolves playback sources and hands the stream to mpv.
func (a *App) PlayVideo(userID, driveID, fileID string) (*model.VideoPreview, error) {
	preview, err := drive.GetVideoPreview(userID, driveID, fileID)
	if err != nil {
		return nil, err
	}
	// pick the best quality
	quality := preview.Qualities[0]
	for _, q := range preview.Qualities {
		if q.Quality == "Origin" {
			quality = q
			break
		}
	}
	streamURL := quality.URL
	if quality.ForceProxy || quality.Headers != nil || preview.Headers != nil {
		streamURL = a.preview.ProxyURL(quality.URL, quality.Headers)
	}
	if err := a.ensurePlayer(); err != nil {
		return nil, err
	}
	a.player.mu.Lock()
	a.player.userID, a.player.driveID, a.player.fileID, a.player.name = userID, driveID, fileID, preview.FileID
	a.player.mu.Unlock()
	if err := a.player.player.LoadFile(context.Background(), streamURL); err != nil {
		return nil, err
	}
	return preview, nil
}

func (a *App) ensurePlayer() error {
	a.playerMu.Lock()
	defer a.playerMu.Unlock()
	if a.player != nil && a.player.player != nil {
		return nil
	}
	if a.player == nil {
		a.player = &playback{}
	}
	mpvPath := engine.MpvPath(filepath.Join(a.dataDir, "engine"))
	if mpvPath == "" {
		return errors.New("未找到 mpv 播放器")
	}
	if _, err := os.Stat(mpvPath); err != nil {
		return errors.New("未找到 mpv 播放器")
	}
	p, err := player.Start(context.Background(), player.Options{MpvPath: mpvPath, ConfigDir: filepath.Join(a.dataDir, "mpv-config")})
	if err != nil {
		return err
	}
	a.player.player = p
	a.player.active = true
	return nil
}

// PausePlayer toggles pause.
func (a *App) PausePlayer(paused bool) error {
	if a.player == nil || a.player.player == nil {
		return errors.New("播放器未启动")
	}
	return a.player.player.Pause(paused)
}

// SeekPlayer seeks to seconds.
func (a *App) SeekPlayer(seconds float64) error {
	if a.player == nil || a.player.player == nil {
		return errors.New("播放器未启动")
	}
	return a.player.player.Seek(seconds)
}

// SetPlayerVolume sets volume.
func (a *App) SetPlayerVolume(v int) error {
	if a.player == nil || a.player.player == nil {
		return errors.New("播放器未启动")
	}
	return a.player.player.SetVolume(v)
}

// SetPlayerSpeed sets playback speed.
func (a *App) SetPlayerSpeed(speed float64) error {
	if a.player == nil || a.player.player == nil {
		return errors.New("播放器未启动")
	}
	return a.player.player.SetSpeed(speed)
}

// StopPlayer stops playback and closes mpv.
func (a *App) StopPlayer() error {
	if a.player != nil && a.player.player != nil {
		_ = a.player.player.Close()
		a.player.player = nil
		a.player.active = false
	}
	return nil
}

// SavePlayCursor persists playback position for resume.
func (a *App) SavePlayCursor(userID, driveID, fileID string, seconds float64) error {
	return a.store.SavePlayCursor(store.PlayCursor{
		UserID: userID, DriveID: driveID, FileID: fileID,
		Seconds: seconds, Updated: time.Now().Unix(),
	})
}

// GetPlayCursor returns the saved resume position.
func (a *App) GetPlayCursor(userID, driveID, fileID string) float64 {
	v, _ := a.store.GetPlayCursor(userID, driveID, fileID)
	return v
}
