package transfer

import (
	"os"
	"time"

	"mnemo-go/internal/logging"
	"mnemo-go/internal/model"
)

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
	_, active := m.cancels[id]
	if ok && !active && (t.Status == "paused" || t.Status == "failed") {
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
	_, active := m.cancels[id]
	if cancel, ok := m.cancels[id]; ok {
		cancel()
	}
	t, ok := m.tasks[id]
	if ok {
		t.Status = "canceled"
		t.Updated = time.Now().Unix()
		m.mu.Unlock()
		m.update(t)
		if !active {
			removeDownloadTemp(t)
		}
		return
	}
	m.mu.Unlock()
}

// Remove hard-deletes a task record (from memory and the store) immediately.
func (m *Manager) Remove(id string) {
	m.mu.Lock()
	m.removed[id] = true
	delete(m.lastPersist, id)
	if cancel, ok := m.cancels[id]; ok {
		cancel()
	}
	t, ok := m.tasks[id]
	_, active := m.cancels[id]
	delete(m.tasks, id)
	// A waiting task cannot revive itself once it has been removed from tasks.
	// Keep the tombstone only for an actively running task, whose progress
	// callback may still be in flight until runDownload exits.
	if !active {
		delete(m.removed, id)
	}
	m.mu.Unlock()
	if err := m.store.DeleteDownloadTask(id); err != nil {
		logging.Warn("download task removal persistence failed", "task_id", id, "error", err)
	}
	if ok && !active {
		removeDownloadTemp(t)
	}
	if m.onEvent != nil {
		m.onEvent(TaskEvent{Kind: "download", Task: model.DownloadTask{ID: id, Status: "removed"}})
	}
}

// Prioritize boosts one task: pause every other active (downloading/queued)
// task and (re)start this one so it gets the full bandwidth.
func (m *Manager) Prioritize(id string) {
	m.mu.Lock()
	var paused []*model.DownloadTask
	for oid, o := range m.tasks {
		if oid != id && (o.Status == "downloading" || o.Status == "queued") {
			if cancel, ok := m.cancels[oid]; ok {
				cancel()
			}
			o.Status = "paused"
			o.Updated = time.Now().Unix()
			paused = append(paused, o)
		}
	}
	t, ok := m.tasks[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	start := false
	if _, active := m.cancels[id]; !active && (t.Status == "paused" || t.Status == "failed" || t.Status == "canceled") {
		t.Status = "queued"
		t.Error = ""
		t.Updated = time.Now().Unix()
		start = true
	}
	m.mu.Unlock()
	for _, p := range paused {
		m.update(p)
	}
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
			delete(m.lastPersist, id)
		}
	}
	m.mu.Unlock()
	if err := m.store.ClearDownloadTasks(); err != nil {
		logging.Warn("completed download task cleanup failed", "error", err)
	}
}

// Shutdown stops background work.
func (m *Manager) Shutdown() {
	m.shutdownOnce.Do(func() {
		// cancel the root context so goroutines waiting for a slot or in flight
		// unblock immediately, in addition to any in-flight task contexts.
		if m.cancel != nil {
			m.cancel()
		}
		m.mu.Lock()
		for _, c := range m.cancels {
			c()
		}
		m.mu.Unlock()
		close(m.stop)
	})
}

func removeDownloadTemp(t *model.DownloadTask) {
	if t == nil {
		return
	}
	_ = os.Remove(t.LocalPath + ".part")
	_ = os.Remove(t.LocalPath + ".state.json")
}
