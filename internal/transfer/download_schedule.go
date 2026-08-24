package transfer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

// SetConcurrency changes the queue limit without replacing a semaphore under
// active work. Existing downloads keep their slot and queued work is woken to
// re-check the new limit, so settings updates cannot temporarily double the
// number of active transfers.
func (m *Manager) SetConcurrency(n int) {
	if n <= 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maxConcurrent = n
	m.notifyConcurrencyLocked()
}

func (m *Manager) acquireSlot(ctx context.Context) error {
	for {
		m.mu.Lock()
		if m.activeConcurrent < m.maxConcurrent {
			m.activeConcurrent++
			m.mu.Unlock()
			return nil
		}
		changed := m.concurrencyChanged
		m.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}

func (m *Manager) releaseSlot() {
	m.mu.Lock()
	if m.activeConcurrent > 0 {
		m.activeConcurrent--
	}
	m.notifyConcurrencyLocked()
	m.mu.Unlock()
}

func (m *Manager) notifyConcurrencyLocked() {
	close(m.concurrencyChanged)
	m.concurrencyChanged = make(chan struct{})
}

// addDownloadTask atomically assigns a collision-free final path and publishes
// the task. Serializing assignment through targetMu prevents two concurrent
// enqueue calls from selecting the same .part/.state/final file set.
func (m *Manager) addDownloadTask(t *model.DownloadTask, requestedName string) {
	m.targetMu.Lock()
	m.mu.Lock()
	t.LocalPath = m.nextDownloadPathLocked(safeName(requestedName))
	m.mu.Unlock()
	m.update(t)
	m.targetMu.Unlock()
}

// nextDownloadPathLocked must be called with m.mu held.
func (m *Manager) nextDownloadPathLocked(fileName string) string {
	for index := 0; ; index++ {
		candidateName := fileName
		if index > 0 {
			candidateName = numberedDownloadName(fileName, index)
		}
		candidate := filepath.Join(m.dir, candidateName)
		if !m.downloadPathOccupiedLocked(candidate) {
			return candidate
		}
	}
}

func (m *Manager) downloadPathOccupiedLocked(candidate string) bool {
	for _, task := range m.tasks {
		if task != nil && sameDownloadPath(task.LocalPath, candidate) {
			return true
		}
	}
	for _, path := range []string{candidate, candidate + ".part", candidate + ".state.json"} {
		if _, err := os.Lstat(path); err == nil || !os.IsNotExist(err) {
			return true
		}
	}
	return false
}

func sameDownloadPath(left, right string) bool {
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

func numberedDownloadName(fileName string, index int) string {
	ext := filepath.Ext(fileName)
	stem := strings.TrimSuffix(fileName, ext)
	if stem == "" {
		stem = fileName
		ext = ""
	}
	return fmt.Sprintf("%s (%d)%s", stem, index, ext)
}

// AddDownload enqueues a download from a drive file.
func (m *Manager) AddDownload(userID, driveID string, f model.File) (*model.DownloadTask, error) {
	provider := drive.ProviderOf(userID, driveID, "")
	// Keep the exact list-row snapshot available until the provider resolves
	// its authenticated URL. Some providers need metadata that is not present
	// in a later detail response.
	drive.RememberFile(userID, driveID, f)
	t := &model.DownloadTask{
		ID:       newID("dl"),
		UserID:   userID,
		DriveID:  driveID,
		Provider: provider,
		FileID:   f.FileID,
		Name:     f.Name,
		Size:     f.Size,
		Status:   "queued",
		Created:  time.Now().Unix(),
		Updated:  time.Now().Unix(),
	}
	m.addDownloadTask(t, f.Name)
	go m.runDownload(t)
	result := safeDownloadTask(*t)
	return &result, nil
}

// AddDownloadURL enqueues a download from a direct URL.
func (m *Manager) AddDownloadURL(name, url string, headers map[string]string) (*model.DownloadTask, error) {
	storedHeaders := make(map[string]string, len(headers))
	for key, value := range headers {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			storedHeaders[key] = value
		}
	}
	t := &model.DownloadTask{
		ID:      newID("dl"),
		Name:    name,
		URL:     url,
		Status:  "queued",
		Headers: storedHeaders,
		Created: time.Now().Unix(),
		Updated: time.Now().Unix(),
	}
	m.addDownloadTask(t, name)
	go m.runDownload(t)
	result := safeDownloadTask(*t)
	return &result, nil
}

func newID(prefix string) string {
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixNano(), time.Now().UnixMilli()%1000)
}

func safeName(name string) string {
	if name == "" {
		return "download"
	}
	out := []rune{}
	for _, r := range name {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', 0:
			continue
		default:
			out = append(out, r)
		}
	}
	res := strings.TrimRight(string(out), " .")
	if res == "" || res == "." || res == ".." {
		return "download"
	}
	// Keep names valid on Windows even when the same download is later moved
	// between platforms. Device names remain reserved with an extension.
	stem := res
	if dot := strings.IndexByte(stem, '.'); dot >= 0 {
		stem = stem[:dot]
	}
	switch strings.ToUpper(stem) {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return "_" + res
	}
	return res
}
