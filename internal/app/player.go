package app

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
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
	mu         sync.Mutex
	loadMu     sync.Mutex
	player     *player.Player
	active     bool
	generation uint64
	userID     string
	driveID    string
	fileID     string
	name       string
}

// PlayVideo resolves playback sources and hands the stream to mpv.
func (a *App) PlayVideo(userID, driveID, fileID string) (*model.VideoPreview, error) {
	return a.playVideo(userID, driveID, fileID, "")
}

// PlayVideoQuality loads a specific quality variant into the existing mpv
// window. An empty quality keeps the default origin-quality selection.
func (a *App) PlayVideoQuality(userID, driveID, fileID, quality string) (*model.VideoPreview, error) {
	return a.playVideo(userID, driveID, fileID, quality)
}

func (a *App) playVideo(userID, driveID, fileID, requestedQuality string) (*model.VideoPreview, error) {
	preview, streamURL, err := a.resolveVideoSource(userID, driveID, fileID, requestedQuality)
	if err != nil {
		return nil, err
	}
	if err := a.ensurePlayer(); err != nil {
		return nil, err
	}
	a.playerMu.Lock()
	pb := a.player
	var p *player.Player
	if pb != nil {
		p = pb.player
	}
	a.playerMu.Unlock()
	if pb == nil || p == nil {
		return nil, errors.New("播放器未启动")
	}
	pb.mu.Lock()
	if pb.player != p || !pb.active {
		pb.mu.Unlock()
		return nil, errors.New("播放器已停止")
	}
	pb.userID, pb.driveID, pb.fileID = userID, driveID, fileID
	pb.name = videoFileName(a, userID, driveID, fileID)
	pb.generation++
	generation := pb.generation
	pb.mu.Unlock()

	// Serializing loadfile commands prevents an older retry or quality switch
	// from overwriting a newer selection while its network request is pending.
	pb.loadMu.Lock()
	defer pb.loadMu.Unlock()
	pb.mu.Lock()
	current := pb.player == p && pb.active && pb.generation == generation
	pb.mu.Unlock()
	if !current {
		return nil, errors.New("播放器已停止")
	}
	if err := p.LoadFile(a.playerContext(), streamURL); err != nil {
		return nil, err
	}
	pb.mu.Lock()
	current = pb.player == p && pb.active && pb.generation == generation
	pb.mu.Unlock()
	if !current {
		return nil, errors.New("播放器已停止")
	}
	return preview, nil
}

func (a *App) resolveVideoSource(userID, driveID, fileID, requestedQuality string) (*model.VideoPreview, string, error) {
	preview, err := drive.GetVideoPreview(userID, driveID, fileID)
	if err != nil {
		return nil, "", err
	}
	if preview == nil || len(preview.Qualities) == 0 {
		return nil, "", errors.New("该文件无可用播放源")
	}
	quality, err := chooseVideoQuality(preview.Qualities, requestedQuality)
	if err != nil {
		return nil, "", err
	}
	if strings.TrimSpace(quality.URL) == "" {
		return nil, "", errors.New("该文件播放地址为空")
	}
	mediaProxy := a.previewServer()
	if mediaProxy == nil {
		return nil, "", errors.New("预览服务未启动")
	}
	headers := mergeHeaders(preview.Headers, quality.Headers)
	// Always use the local proxy. It provides Range support, CORS and one
	// consistent header path for all providers, including public URLs.
	return preview, mediaProxy.ProxyURL(quality.URL, headers, videoFileName(a, userID, driveID, fileID)), nil
}

func chooseVideoQuality(qualities []model.VideoQuality, requested string) (model.VideoQuality, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		for _, quality := range qualities {
			if strings.EqualFold(strings.TrimSpace(quality.Quality), requested) ||
				strings.EqualFold(strings.TrimSpace(quality.Value), requested) ||
				strings.EqualFold(strings.TrimSpace(quality.Label), requested) {
				return quality, nil
			}
		}
		return model.VideoQuality{}, fmt.Errorf("不支持的视频画质 %q", requested)
	}
	for _, quality := range qualities {
		if strings.EqualFold(quality.Quality, "origin") || strings.EqualFold(quality.Label, "原画") {
			return quality, nil
		}
	}
	return qualities[0], nil
}

func mergeHeaders(base, override map[string]string) map[string]string {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	out := make(map[string]string, len(base)+len(override))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range override {
		out[k] = v
	}
	return out
}

func (a *App) playerContext() context.Context {
	return a.appContext()
}

func videoFileName(a *App, userID, driveID, fileID string) string {
	if f, err := drive.GetFile(userID, driveID, fileID); err == nil && f != nil {
		return f.Name
	}
	return fileID
}

func (a *App) ensurePlayer() error {
	a.playerMu.Lock()
	defer a.playerMu.Unlock()
	if a.player != nil {
		pb := a.player
		pb.mu.Lock()
		existing := pb.player
		pb.mu.Unlock()
		if existing != nil && existing.Alive() {
			return nil
		}
		if existing != nil {
			_ = existing.Close()
		}
		pb.mu.Lock()
		if pb.player == existing {
			pb.player = nil
			pb.active = false
			pb.generation++
		}
		pb.mu.Unlock()
	}
	if a.player == nil {
		a.player = &playback{}
	}
	dataDir := a.dataDirectory()
	if dataDir == "" {
		return errors.New("应用尚未初始化")
	}
	engineDir := filepath.Join(dataDir, "engine")
	// Extract the platform-specific bundled engine on first playback. The
	// release workflow injects the matching binary before Wails embeds assets.
	if err := engine.Extract(engineDir); err != nil {
		return fmt.Errorf("mpv 引擎释放失败: %w", err)
	}
	mpvPath := engine.MpvPath(engineDir)
	if mpvPath == "" {
		return errors.New("未找到 mpv 播放器")
	}
	if _, err := os.Stat(mpvPath); err != nil {
		return errors.New("未找到 mpv 播放器")
	}
	p, err := player.Start(a.playerContext(), player.Options{
		MpvPath:   mpvPath,
		ConfigDir: filepath.Join(dataDir, "mpv-config"),
		Env:       engine.MpvEnv(engineDir),
	})
	if err != nil {
		return err
	}
	a.player.mu.Lock()
	a.player.player = p
	a.player.active = true
	a.player.generation++
	a.player.mu.Unlock()
	return nil
}

// PausePlayer toggles pause.
func (a *App) PausePlayer(paused bool) error {
	p, err := a.currentPlayer()
	if err != nil {
		return err
	}
	return p.Pause(paused)
}

// SeekPlayer seeks to seconds.
func (a *App) SeekPlayer(seconds float64) error {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
		return errors.New("播放位置无效")
	}
	p, err := a.currentPlayer()
	if err != nil {
		return err
	}
	return p.Seek(seconds)
}

// SetPlayerVolume sets volume.
func (a *App) SetPlayerVolume(v int) error {
	if v < 0 || v > 100 {
		return errors.New("音量必须在 0 到 100 之间")
	}
	p, err := a.currentPlayer()
	if err != nil {
		return err
	}
	return p.SetVolume(v)
}

// SetPlayerSpeed sets playback speed.
func (a *App) SetPlayerSpeed(speed float64) error {
	if math.IsNaN(speed) || math.IsInf(speed, 0) || speed <= 0 || speed > 16 {
		return errors.New("播放速度无效")
	}
	p, err := a.currentPlayer()
	if err != nil {
		return err
	}
	return p.SetSpeed(speed)
}

// GetPlayerState returns the current position and duration reported by mpv.
// Values are omitted while mpv has not loaded enough metadata yet.
func (a *App) GetPlayerState() (map[string]float64, error) {
	p, err := a.currentPlayer()
	if err != nil {
		return nil, err
	}
	state := make(map[string]float64, 2)
	for name, key := range map[string]string{"time-pos": "position", "duration": "duration"} {
		value, err := p.GetProperty(name)
		if err != nil {
			continue
		}
		if number, ok := value.(float64); ok && !math.IsNaN(number) && !math.IsInf(number, 0) && number >= 0 {
			state[key] = number
		}
	}
	if len(state) == 0 {
		return nil, errors.New("播放器尚未准备好")
	}
	return state, nil
}

// StopPlayer stops playback and closes mpv.
func (a *App) StopPlayer() error {
	a.playerMu.Lock()
	if a.player == nil || a.player.player == nil {
		a.playerMu.Unlock()
		return nil
	}
	pb := a.player
	pb.mu.Lock()
	p := pb.player
	pb.player = nil
	pb.active = false
	pb.generation++
	pb.mu.Unlock()
	a.playerMu.Unlock()
	_ = p.Close()
	return nil
}

// SavePlayCursor persists playback position for resume.
func (a *App) SavePlayCursor(userID, driveID, fileID string, seconds float64) error {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
		return errors.New("播放位置无效")
	}
	st, err := a.storeOrError()
	if err != nil {
		return err
	}
	return st.SavePlayCursor(store.PlayCursor{
		UserID: userID, DriveID: driveID, FileID: fileID,
		Seconds: seconds, Updated: time.Now().Unix(),
	})
}

func (a *App) currentPlayer() (*player.Player, error) {
	a.playerMu.Lock()
	defer a.playerMu.Unlock()
	if a.player == nil || a.player.player == nil {
		return nil, errors.New("播放器未启动")
	}
	return a.player.player, nil
}

// GetPlayCursor returns the saved resume position.
func (a *App) GetPlayCursor(userID, driveID, fileID string) float64 {
	st, err := a.storeOrError()
	if err != nil {
		return 0
	}
	v, _ := st.GetPlayCursor(userID, driveID, fileID)
	return v
}
