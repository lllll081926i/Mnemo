// Package transfer manages download/upload tasks: the segmented download
// manager, the upload queue and cross-drive migration.
package transfer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/store"
	"mnemo-go/internal/transfer/dlengine"
)

// OnTaskEvent is called on every task state/progress change.
type OnTaskEvent func(ev TaskEvent)

// TaskEvent is pushed to the frontend via events.
type TaskEvent struct {
	Kind string             `json:"kind"` // download | upload | offline
	Task model.DownloadTask `json:"task"`
}

// Manager owns the active download queue.
type Manager struct {
	store   *store.Store
	mu      sync.Mutex
	tasks   map[string]*model.DownloadTask
	onEvent OnTaskEvent
	dir     string
	stop    chan struct{}
}

// NewManager creates a download manager.
func NewManager(st *store.Store, downloadDir string, onEvent OnTaskEvent) (*Manager, error) {
	if downloadDir == "" {
		home, _ := os.UserHomeDir()
		downloadDir = filepath.Join(home, "Downloads")
	}
	_ = os.MkdirAll(downloadDir, 0o755)
	m := &Manager{
		store:   st,
		tasks:   map[string]*model.DownloadTask{},
		onEvent: onEvent,
		dir:     downloadDir,
		stop:    make(chan struct{}),
	}
	// restore persisted tasks
	m.loadPersisted()
	return m, nil
}

// SetEventSink wires the event callback (used by the app layer).
func (m *Manager) SetEventSink(fn OnTaskEvent) { m.onEvent = fn }

func (m *Manager) loadPersisted() {
	list, err := m.store.ListDownloadTasks()
	if err != nil {
		return
	}
	for i := range list {
		t := list[i]
		if t.Status == "downloading" || t.Status == "queued" {
			t.Status = "paused" // interrupted on exit
		}
		m.tasks[t.ID] = &t
	}
}

// List returns all tasks (active + persisted).
func (m *Manager) List() []model.DownloadTask {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]model.DownloadTask, 0, len(m.tasks))
	for _, t := range m.tasks {
		out = append(out, *t)
	}
	return out
}

func (m *Manager) get(id string) (*model.DownloadTask, bool) {
	m.mu.Lock()
	t, ok := m.tasks[id]
	m.mu.Unlock()
	return t, ok
}

func (m *Manager) update(t *model.DownloadTask) {
	m.mu.Lock()
	m.tasks[t.ID] = t
	m.mu.Unlock()
	_ = m.store.SaveDownloadTask(t)
	if m.onEvent != nil {
		m.onEvent(TaskEvent{Kind: "download", Task: *t})
	}
}

// AddDownload enqueues a download from a drive file.
func (m *Manager) AddDownload(userID, driveID string, f model.File) (*model.DownloadTask, error) {
	provider := drive.ProviderOf(userID, driveID, "")
	t := &model.DownloadTask{
		ID:        newID("dl"),
		UserID:    userID,
		DriveID:   driveID,
		Provider:  provider,
		FileID:    f.FileID,
		Name:      f.Name,
		Size:      f.Size,
		Status:    "queued",
		LocalPath: filepath.Join(m.dir, safeName(f.Name)),
		Created:   time.Now().Unix(),
		Updated:   time.Now().Unix(),
	}
	m.update(t)
	go m.runDownload(t)
	return t, nil
}

// AddDownloadURL enqueues a download from a direct URL.
func (m *Manager) AddDownloadURL(name, url string, headers map[string]string) (*model.DownloadTask, error) {
	t := &model.DownloadTask{
		ID:        newID("dl"),
		Name:      name,
		URL:       url,
		Status:    "queued",
		LocalPath: filepath.Join(m.dir, safeName(name)),
		Created:   time.Now().Unix(),
		Updated:   time.Now().Unix(),
	}
	m.update(t)
	go m.runDownload(t)
	return t, nil
}

func (m *Manager) runDownload(t *model.DownloadTask) {
	ctx := context.Background()
	url := t.URL
	var headers map[string]string
	opts := dlengine.Options{Concurrency: DefaultConcurrencyFromSettings()}
	if url == "" && t.UserID != "" {
		u, err := drive.GetDownloadURL(t.UserID, t.DriveID, t.FileID, 14400)
		if err != nil {
			t.Status = "failed"
			t.Error = err.Error()
			m.update(t)
			return
		}
		url = u.URL
		headers = u.Headers
		if t.Size == 0 {
			t.Size = u.Size
		}
		if u.ForceLocalProxy || u.DownloadMode == "proxy" {
			opts.Concurrency = 1
			if u.Concurrency > 1 {
				opts.Concurrency = u.Concurrency
			}
		}
	}
	if url == "" {
		t.Status = "failed"
		t.Error = "无法获取下载地址"
		m.update(t)
		return
	}
	t.URL = url
	t.Status = "downloading"
	t.Updated = time.Now().Unix()
	opts.Headers = headers
	m.update(t)

	err := dlengine.Download(ctx, opts, url, t.LocalPath, func(p dlengine.Progress) {
		t.Downloaded = p.Downloaded
		t.Speed = p.Speed
		t.Progress = p.Percent
		t.Updated = time.Now().Unix()
		m.update(t)
	})
	if err != nil {
		t.Status = "failed"
		t.Error = err.Error()
	} else {
		t.Status = "completed"
		t.Downloaded = t.Size
		t.Progress = 100
	}
	t.Updated = time.Now().Unix()
	m.update(t)
}

// Pause marks a task paused (downloads resume on restart; in-flight run is
// allowed to finish current chunk).
func (m *Manager) Pause(id string) {
	if t, ok := m.get(id); ok && (t.Status == "downloading" || t.Status == "queued") {
		t.Status = "paused"
		m.update(t)
	}
}

// Resume re-queues a paused task.
func (m *Manager) Resume(id string) {
	if t, ok := m.get(id); ok && t.Status == "paused" {
		t.Status = "queued"
		m.update(t)
		go m.runDownload(t)
	}
}

// Cancel stops and removes a task.
func (m *Manager) Cancel(id string) {
	if t, ok := m.get(id); ok {
		t.Status = "canceled"
		t.Updated = time.Now().Unix()
		m.update(t)
		_ = os.Remove(t.LocalPath + ".part")
		_ = os.Remove(t.LocalPath + ".state.json")
	}
}

// Remove hard-deletes a task record (from memory and the store) immediately.
// Used by the “已完成”列表删除：删除即从列表移除，不留下 canceled 记录。
func (m *Manager) Remove(id string) {
	if t, ok := m.get(id); ok {
		// 清理可能残留的分卷临时文件（不删已完成的成品文件）
		_ = os.Remove(t.LocalPath + ".part")
		_ = os.Remove(t.LocalPath + ".state.json")
	}
	m.mu.Lock()
	delete(m.tasks, id)
	m.mu.Unlock()
	_ = m.store.DeleteDownloadTask(id)
	if m.onEvent != nil {
		m.onEvent(TaskEvent{Kind: "download", Task: model.DownloadTask{ID: id, Status: "removed"}})
	}
}

// Prioritize boosts one task: pause every other active (downloading/queued)
// task and (re)start this one so it gets the full bandwidth.
// 引擎是全并发的（无队列），“优先”即暂停其他任务把带宽让给该任务。
func (m *Manager) Prioritize(id string) {
	t, ok := m.get(id)
	if !ok {
		return
	}
	m.mu.Lock()
	for oid, o := range m.tasks {
		if oid != id && (o.Status == "downloading" || o.Status == "queued") {
			o.Status = "paused"
			o.Updated = time.Now().Unix()
			_ = m.store.SaveDownloadTask(o)
			if m.onEvent != nil {
				m.onEvent(TaskEvent{Kind: "download", Task: *o})
			}
		}
	}
	m.mu.Unlock()
	if t.Status == "paused" || t.Status == "failed" || t.Status == "canceled" {
		t.Status = "queued"
		t.Error = ""
		t.Updated = time.Now().Unix()
		m.update(t)
		go m.runDownload(t)
	}
}

// ClearCompleted removes completed/canceled tasks.
func (m *Manager) ClearCompleted() {
	m.mu.Lock()
	for id, t := range m.tasks {
		if t.Status == "completed" || t.Status == "canceled" || t.Status == "failed" {
			delete(m.tasks, id)
		}
	}
	m.mu.Unlock()
	_ = m.store.ClearDownloadTasks()
}

// Shutdown stops background work.
func (m *Manager) Shutdown() { close(m.stop) }

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
	res := string(out)
	if res == "" || res == "." || res == ".." {
		return "download"
	}
	return res
}
