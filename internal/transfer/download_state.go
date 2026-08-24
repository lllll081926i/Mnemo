package transfer

import (
	"os"
	"time"

	"mnemo-go/internal/logging"
	"mnemo-go/internal/model"
)

// SetEventSink wires the event callback (used by the app layer).
func (m *Manager) SetEventSink(fn OnTaskEvent) {
	m.mu.Lock()
	m.onEvent = fn
	m.mu.Unlock()
}

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

func (m *Manager) loadPersisted() {
	list, err := m.store.ListDownloadTasks()
	if err != nil {
		logging.Warn("download task restore failed", "error", err)
		return
	}
	for i := range list {
		t := list[i]
		safeTask := safeDownloadTask(t)
		dirty := t.URL != "" || len(t.Headers) > 0 || t.Error != safeTask.Error
		t = safeTask
		if t.Status == "downloading" || t.Status == "queued" {
			t.Status = "paused" // interrupted on exit
			dirty = true
		}
		if t.UserID == "" && t.Status != "completed" && t.Status != "canceled" {
			t.Status = "failed"
			t.Error = "直链任务的地址和请求头不会落盘；应用重启后请重新添加任务"
			dirty = true
		}
		m.tasks[t.ID] = &t
		if dirty {
			if err := m.store.SaveDownloadTask(&t); err != nil {
				logging.Warn("download task security migration failed", "task_id", t.ID, "error", err)
			}
		}
	}
}

// List returns all tasks (active + persisted).
func (m *Manager) List() []model.DownloadTask {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]model.DownloadTask, 0, len(m.tasks))
	for _, t := range m.tasks {
		out = append(out, safeDownloadTask(*t))
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
	if t == nil {
		return
	}
	m.mu.Lock()
	if m.removed[t.ID] {
		m.mu.Unlock()
		return
	}
	m.tasks[t.ID] = t
	snapshot := *t
	shouldPersist := m.shouldPersistLocked(snapshot)
	event := m.onEvent
	m.mu.Unlock()
	if shouldPersist {
		// Store methods read-modify-write one JSON file. Serialize the whole
		// operation so concurrent task progress cannot lose another task's update.
		m.persistMu.Lock()
		persisted := safeDownloadTask(snapshot)
		if err := m.store.SaveDownloadTask(&persisted); err != nil {
			logging.Warn("download task persistence failed", "task_id", snapshot.ID, "status", snapshot.Status, "error", err)
		}
		m.persistMu.Unlock()
	}
	if event != nil {
		event(TaskEvent{Kind: "download", Task: safeDownloadTask(snapshot)})
	}
}

func safeDownloadTask(task model.DownloadTask) model.DownloadTask {
	task.URL = ""
	task.Headers = nil
	task.Error = logging.RedactText(task.Error)
	return task
}

func (m *Manager) shouldPersistLocked(task model.DownloadTask) bool {
	now := time.Now()
	previous, ok := m.lastPersist[task.ID]
	terminal := task.Status == "completed" || task.Status == "failed" || task.Status == "canceled" || task.Status == "paused"
	if !ok || previous.status != task.Status || terminal || now.Sub(previous.at) >= transferProgressPersistInterval {
		m.lastPersist[task.ID] = progressPersistState{status: task.Status, at: now}
		return true
	}
	return false
}
