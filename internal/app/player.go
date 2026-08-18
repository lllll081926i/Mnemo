package app

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/store"
)

// PlayVideo resolves playback sources and returns a streamable proxy URL for
// the HTML5 <video> element in the frontend.
func (a *App) PlayVideo(userID, driveID, fileID string) (*model.VideoPreview, error) {
	return a.playVideo(userID, driveID, fileID, "")
}

// PlayVideoQuality returns a streamable URL for a specific quality variant.
// An empty quality keeps the default origin-quality selection.
func (a *App) PlayVideoQuality(userID, driveID, fileID, quality string) (*model.VideoPreview, error) {
	return a.playVideo(userID, driveID, fileID, quality)
}

func (a *App) playVideo(userID, driveID, fileID, requestedQuality string) (*model.VideoPreview, error) {
	preview, streamURL, err := a.resolveVideoSource(userID, driveID, fileID, requestedQuality)
	if err != nil {
		return nil, err
	}
	preview.URL = streamURL
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

func videoFileName(a *App, userID, driveID, fileID string) string {
	if f, err := drive.GetFile(userID, driveID, fileID); err == nil && f != nil {
		return f.Name
	}
	return fileID
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

// GetPlayCursor returns the saved resume position.
func (a *App) GetPlayCursor(userID, driveID, fileID string) float64 {
	st, err := a.storeOrError()
	if err != nil {
		return 0
	}
	v, _ := st.GetPlayCursor(userID, driveID, fileID)
	return v
}
