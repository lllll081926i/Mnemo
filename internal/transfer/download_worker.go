package transfer

import (
	"context"
	"errors"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/logging"
	"mnemo-go/internal/model"
	"mnemo-go/internal/transfer/dlengine"
)

func (m *Manager) runDownload(t *model.DownloadTask) {
	started := time.Now()
	logging.Info("download worker started", "task_id", t.ID, "name", t.Name)
	// wait for a concurrency slot before doing any work
	if err := m.acquireSlot(m.ctx); err != nil {
		// app shutting down before the task could start
		m.mu.Lock()
		shouldPause := !m.removed[t.ID] && m.tasks[t.ID] == t
		if shouldPause {
			t.Status = "paused"
			t.Updated = time.Now().Unix()
		}
		m.mu.Unlock()
		if shouldPause {
			m.update(t)
		}
		logging.Debug("download worker paused before start", "task_id", t.ID, "reason", err)
		return
	}
	defer m.releaseSlot()

	m.mu.Lock()
	if m.removed[t.ID] || m.tasks[t.ID] != t || t.Status != "queued" || m.ctx.Err() != nil {
		m.mu.Unlock()
		logging.Debug("download worker skipped", "task_id", t.ID, "reason", "task no longer queued")
		return
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancels[t.ID] = cancel
	t.Status = "downloading"
	t.Updated = time.Now().Unix()
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.cancels, t.ID)
		removed := m.removed[t.ID]
		cleanup := removed || t.Status == "canceled"
		if removed {
			delete(m.removed, t.ID)
		}
		m.mu.Unlock()
		if cleanup {
			removeDownloadTemp(t)
		}
	}()

	m.update(t)

	url := t.URL
	headers := t.Headers
	var requestAuth model.RequestAuthenticator
	s, _ := m.store.GetSettings()
	opts := dlengine.Options{Concurrency: concurrencyFromSettings(s)}
	if s.MaxDownloadSpeed > 0 {
		opts.MaxSpeed = 0
		m.speedLimiter.SetRate(s.MaxDownloadSpeed)
	} else {
		m.speedLimiter.SetRate(0)
	}
	opts.Limiter = m.speedLimiter
	if url == "" && t.UserID != "" {
		u, err := drive.GetDownloadURL(t.UserID, t.DriveID, t.FileID, 14400)
		if err != nil {
			m.mu.Lock()
			if !m.removed[t.ID] && t.Status != "canceled" && t.Status != "paused" {
				t.Status = "failed"
				t.Error = err.Error()
			}
			t.Updated = time.Now().Unix()
			m.mu.Unlock()
			m.update(t)
			logging.Warn("download URL resolution failed", "task_id", t.ID, "error", err)
			return
		}
		if u == nil {
			m.mu.Lock()
			if !m.removed[t.ID] && t.Status != "canceled" && t.Status != "paused" {
				t.Status = "failed"
				t.Error = "下载地址为空"
			}
			t.Updated = time.Now().Unix()
			m.mu.Unlock()
			m.update(t)
			logging.Warn("download URL resolution returned empty result", "task_id", t.ID)
			return
		}
		url = u.URL
		headers = u.Headers
		requestAuth = u.RequestAuth
		m.mu.Lock()
		if t.Size == 0 {
			t.Size = u.Size
		}
		m.mu.Unlock()
		if u.ForceLocalProxy || u.DownloadMode == "proxy" {
			opts.Concurrency = 1
			if u.Concurrency > 1 {
				opts.Concurrency = u.Concurrency
			}
		}
	}
	if url == "" {
		m.mu.Lock()
		if !m.removed[t.ID] && t.Status != "canceled" && t.Status != "paused" {
			t.Status = "failed"
			t.Error = "无法获取下载地址"
		}
		t.Updated = time.Now().Unix()
		m.mu.Unlock()
		m.update(t)
		logging.Warn("download URL missing", "task_id", t.ID)
		return
	}

	m.mu.Lock()
	if t.Status == "canceled" || t.Status == "paused" || m.removed[t.ID] {
		m.mu.Unlock()
		return
	}
	t.URL = url
	t.Updated = time.Now().Unix()
	opts.ExpectedSize = t.Size
	m.mu.Unlock()
	opts.Headers = headers
	opts.RequestAuth = requestAuth
	m.update(t)
	logging.Info("download started", "task_id", t.ID)

	progress := func(p dlengine.Progress) {
		m.mu.Lock()
		t.Downloaded = p.Downloaded
		if p.Total > 0 {
			t.Size = p.Total
		}
		t.Speed = p.Speed
		t.Progress = p.Percent
		t.Updated = time.Now().Unix()
		m.mu.Unlock()
		m.update(t)
	}
	err := dlengine.Download(ctx, opts, url, t.LocalPath, progress)
	// Signed URLs from provider APIs can expire while a long download is
	// running. Re-resolve once for account-backed tasks and reuse the .part
	// file; direct URL tasks have no provider context and remain unchanged.
	if err != nil && t.UserID != "" && isExpiredDownloadError(err) && ctx.Err() == nil {
		if fresh, refreshErr := drive.GetDownloadURL(t.UserID, t.DriveID, t.FileID, 14400); refreshErr == nil && fresh != nil && fresh.URL != "" {
			url = fresh.URL
			opts.Headers = fresh.Headers
			opts.RequestAuth = fresh.RequestAuth
			m.mu.Lock()
			t.URL = url
			if fresh.Size > 0 {
				t.Size = fresh.Size
			}
			opts.ExpectedSize = t.Size
			m.mu.Unlock()
			if fresh.ForceLocalProxy || fresh.DownloadMode == "proxy" {
				opts.Concurrency = 1
				if fresh.Concurrency > 1 {
					opts.Concurrency = fresh.Concurrency
				}
			}
			m.update(t)
			err = dlengine.Download(ctx, opts, url, t.LocalPath, progress)
		}
	}

	m.mu.Lock()
	removed := m.removed[t.ID]
	if err != nil {
		if t.Status == "canceled" {
			// Cancel has already set the terminal state.
		} else if errors.Is(ctx.Err(), context.Canceled) || strings.Contains(err.Error(), "canceled") {
			// user paused or canceled
			if t.Status != "canceled" && !removed {
				t.Status = "paused"
			}
		} else if !removed {
			t.Status = "failed"
			t.Error = err.Error()
		}
	} else {
		if !removed && t.Status != "canceled" && t.Status != "paused" {
			t.Status = "completed"
			if t.Size <= 0 && t.Downloaded > 0 {
				t.Size = t.Downloaded
			}
			t.Downloaded = t.Size
			t.Progress = 100
		}
	}
	t.Updated = time.Now().Unix()
	m.mu.Unlock()
	m.update(t)
	logging.Info("download worker finished", "task_id", t.ID, "status", t.Status, "error_present", t.Error != "", "duration", logging.Duration(started))
}

func isExpiredDownloadError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "http 401") || strings.Contains(msg, "http 403") ||
		strings.Contains(msg, "range http 401") || strings.Contains(msg, "range http 403")
}
