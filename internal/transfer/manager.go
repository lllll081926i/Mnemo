// Package transfer manages download/upload tasks: the segmented download
// manager, the upload queue and cross-drive migration.
package transfer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	cancels map[string]context.CancelFunc
	onEvent OnTaskEvent
	dir     string
	stop    chan struct{}
	ctx     context.Context // root context for all downloads, canceled on Shutdown
	sem     chan struct{}    // concurrency semaphore (cap = MaxConcurrentDownloads)
}

// NewManager creates a download manager.
func NewManager(st *store.Store, downloadDir string, onEvent OnTaskEvent) (*Manager, error) {
	if downloadDir == "" {
		home, _ := os.UserHomeDir()
		downloadDir = filepath.Join(home, "Downloads")
	}
	_ = os.MkdirAll(downloadDir, 0o755)
	maxConc := 3
	if s, err := st.GetSettings(); err == nil && s.MaxConcurrentDownloads > 0 {
		maxConc = s.MaxConcurrentDownloads
	}
	m := &Manager{
		store:   st,
		tasks:   map[string]*model.DownloadTask{},
		cancels: map[string]context.CancelFunc{},
		onEvent: onEvent,
		dir:     downloadDir,
		stop:    make(chan struct{}),
		ctx:     context.Background(),
		sem:     make(chan struct{}, maxConc),
	}
	// restore persisted tasks
	m.loadPersisted()
	return m, nil
}

// SetEventSink wires the event callback (used by the app layer).
func (m *Manager) SetEventSink(fn OnTaskEvent) { m.onEvent = fn }

// SetDir updates the download directory at runtime. Subsequent downloads use
// the new directory; existing tasks keep their already-assigned LocalPath.
func (m *Manager) SetDir(dir string) {
	if dir == "" {
		return
	}
	_ = os.MkdirAll(dir, 0o755)
	m.mu.Lock()
	m.dir = dir
	m.mu.Unlock()
}

// SetConcurrency rebuilds the semaphore capacity. In-flight tasks are not
// interrupted; only future starts are affected.
func (m *Manager) SetConcurrency(n int) {
	if n <= 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// drain old sem and replace; in-flight acquirers will release into the old
	// channel they captured, so we only swap the reference.
	m.sem = make(chan struct{}, n)
}

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
	// wait for a concurrency slot before doing any work
	select {
	case m.sem <- struct{}{}:
		defer func() { <-m.sem }()
	case <-m.ctx.Done():
		// app shutting down before the task could start
		m.mu.Lock()
		t.Status = "paused"
		t.Updated = time.Now().Unix()
		m.mu.Unlock()
		m.update(t)
		return
	}

	ctx, cancel := context.WithCancel(m.ctx)
	m.mu.Lock()
	m.cancels[t.ID] = cancel
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.cancels, t.ID)
		m.mu.Unlock()
	}()

	url := t.URL
	var headers map[string]string
	s, _ := m.store.GetSettings()
	opts := dlengine.Options{Concurrency: concurrencyFromSettings(s)}
	if s.MaxDownloadSpeed > 0 {
		opts.MaxSpeed = s.MaxDownloadSpeed
	}
	if url == "" && t.UserID != "" {
		u, err := drive.GetDownloadURL(t.UserID, t.DriveID, t.FileID, 14400)
		if err != nil {
			m.mu.Lock()
			t.Status = "failed"
			t.Error = err.Error()
			t.Updated = time.Now().Unix()
			m.mu.Unlock()
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
		m.mu.Lock()
		t.Status = "failed"
		t.Error = "无法获取下载地址"
		t.Updated = time.Now().Unix()
		m.mu.Unlock()
		m.update(t)
		return
	}

	m.mu.Lock()
	t.URL = url
	t.Status = "downloading"
	t.Updated = time.Now().Unix()
	m.mu.Unlock()
	opts.Headers = headers
	m.update(t)

	err := dlengine.Download(ctx, opts, url, t.LocalPath, func(p dlengine.Progress) {
		m.mu.Lock()
		t.Downloaded = p.Downloaded
		t.Speed = p.Speed
		t.Progress = p.Percent
		t.Updated = time.Now().Unix()
		m.mu.Unlock()
		m.update(t)
	})

	m.mu.Lock()
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) || strings.Contains(err.Error(), "canceled") {
			// user paused or canceled
			if t.Status != "canceled" {
				t.Status = "paused"
			}
		} else {
			t.Status = "failed"
			t.Error = err.Error()
		}
	} else {
		t.Status = "completed"
		t.Downloaded = t.Size
		t.Progress = 100
	}
	t.Updated = time.Now().Unix()
	m.mu.Unlock()
	m.update(t)
}

// Pause marks a task paused and cancels active download immediately.
func (m *Manager) Pause(id string) {
	m.mu.Lock()
	if cancel, ok := m.cancels[id]; ok {
		cancel()
	}
	t, ok := m.tasks[id]
	if ok && (t.Status == "downloading" || t.Status == "queued") {
		t.Status = "paused"
		t.Updated = time.Now().Unix()
		m.mu.Unlock()
		m.update(t)
		return
	}
	m.mu.Unlock()
}

// Resume re-queues a paused task.
func (m *Manager) Resume(id string) {
	m.mu.Lock()
	t, ok := m.tasks[id]
	if ok && (t.Status == "paused" || t.Status == "failed") {
		t.Status = "queued"
		t.Error = ""
		t.Updated = time.Now().Unix()
		m.mu.Unlock()
		m.update(t)
		go m.runDownload(t)
		return
	}
	m.mu.Unlock()
}

// Cancel stops and removes a task.
func (m *Manager) Cancel(id string) {
	m.mu.Lock()
	if cancel, ok := m.cancels[id]; ok {
		cancel()
	}
	t, ok := m.tasks[id]
	if ok {
		t.Status = "canceled"
		t.Updated = time.Now().Unix()
		m.mu.Unlock()
		m.update(t)
		_ = os.Remove(t.LocalPath + ".part")
		_ = os.Remove(t.LocalPath + ".state.json")
		return
	}
	m.mu.Unlock()
}

// Remove hard-deletes a task record (from memory and the store) immediately.
func (m *Manager) Remove(id string) {
	m.mu.Lock()
	if cancel, ok := m.cancels[id]; ok {
		cancel()
	}
	t, ok := m.tasks[id]
	if ok {
		_ = os.Remove(t.LocalPath + ".part")
		_ = os.Remove(t.LocalPath + ".state.json")
	}
	delete(m.tasks, id)
	m.mu.Unlock()
	_ = m.store.DeleteDownloadTask(id)
	if m.onEvent != nil {
		m.onEvent(TaskEvent{Kind: "download", Task: model.DownloadTask{ID: id, Status: "removed"}})
	}
}

// Prioritize boosts one task: pause every other active (downloading/queued)
// task and (re)start this one so it gets the full bandwidth.
func (m *Manager) Prioritize(id string) {
	m.mu.Lock()
	for oid, o := range m.tasks {
		if oid != id && (o.Status == "downloading" || o.Status == "queued") {
			if cancel, ok := m.cancels[oid]; ok {
				cancel()
			}
			o.Status = "paused"
			o.Updated = time.Now().Unix()
			_ = m.store.SaveDownloadTask(o)
			if m.onEvent != nil {
				m.onEvent(TaskEvent{Kind: "download", Task: *o})
			}
		}
	}
	t, ok := m.tasks[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	start := false
	if t.Status == "paused" || t.Status == "failed" || t.Status == "canceled" {
		t.Status = "queued"
		t.Error = ""
		t.Updated = time.Now().Unix()
		start = true
	}
	m.mu.Unlock()
	m.update(t)
	if start {
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
func (m *Manager) Shutdown() {
	// cancel all active downloads first
	m.mu.Lock()
	for _, c := range m.cancels {
		c()
	}
	m.mu.Unlock()
	close(m.stop)
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
	res := string(out)
	if res == "" || res == "." || res == ".." {
		return "download"
	}
	return res
}
