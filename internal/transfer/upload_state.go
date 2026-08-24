package transfer

import (
	"time"

	"mnemo-go/internal/model"
)

// SetConcurrency updates the upload worker limit without canceling active
// uploads. If the limit shrinks, current workers finish normally and new
// workers wait until the active count falls below the new limit.
func (q *UploadQueue) SetConcurrency(limit int) {
	if q == nil || q.gate == nil {
		return
	}
	q.gate.setLimit(limit)
}

// List returns active upload jobs.
func (q *UploadQueue) List() []model.UploadingUI {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]model.UploadingUI, 0, len(q.jobs))
	for _, j := range q.jobs {
		out = append(out, *j)
	}
	return out
}

func (q *UploadQueue) get(id string) (*model.UploadingUI, bool) {
	q.mu.Lock()
	j, ok := q.jobs[id]
	q.mu.Unlock()
	return j, ok
}

func (q *UploadQueue) update(j *model.UploadingUI) {
	if j == nil || j.UploadID == "" {
		return
	}
	q.mu.Lock()
	q.jobs[j.UploadID] = j
	q.signalChangedLocked()
	snapshot := *j
	shouldPersist := q.shouldPersistLocked(snapshot)
	q.mu.Unlock()
	q.publishSnapshot(snapshot, shouldPersist)
}

// mutateJob applies a queue-owned change to a job that may not have an active
// worker. It is used for metadata that is attached immediately after enqueue,
// while preserving the same persistence and event behavior as update.
func (q *UploadQueue) mutateJob(id string, mutate func(*model.UploadingUI)) bool {
	if id == "" || mutate == nil {
		return false
	}
	q.mu.Lock()
	j, ok := q.jobs[id]
	if !ok || j == nil {
		q.mu.Unlock()
		return false
	}
	mutate(j)
	q.signalChangedLocked()
	snapshot := *j
	shouldPersist := q.shouldPersistLocked(snapshot)
	q.mu.Unlock()
	q.publishSnapshot(snapshot, shouldPersist)
	return true
}

func (q *UploadQueue) publishSnapshot(snapshot model.UploadingUI, shouldPersist bool) {
	if shouldPersist {
		q.persistMu.Lock()
		_ = q.store.SaveUploadTask(&snapshot)
		q.persistMu.Unlock()
	}
	if q.onEvent != nil {
		t := model.DownloadTask{
			ID: snapshot.UploadID, UserID: snapshot.UserID, DriveID: snapshot.Info.DriveID,
			ParentID: snapshot.Info.ParentFileID, Name: snapshot.Info.Name, Size: snapshot.Info.Size,
			Downloaded: snapshot.Upload.DownSize, Speed: snapshot.Upload.DownSpeed,
			Progress: snapshot.Upload.DownProcess, Status: uploadStatus(snapshot.Upload),
			Created: snapshot.Upload.DownTime, Updated: time.Now().Unix(),
		}
		q.onEvent(TaskEvent{Kind: "upload", Task: t})
	}
}

func (q *UploadQueue) mutateRunJob(id string, generation uint64, mutate func(*model.UploadingUI)) bool {
	q.mu.Lock()
	run := q.runs[id]
	j, ok := q.jobs[id]
	if !ok || run == nil || run.generation != generation {
		q.mu.Unlock()
		return false
	}
	mutate(j)
	q.signalChangedLocked()
	snapshot := *j
	shouldPersist := q.shouldPersistLocked(snapshot)
	q.mu.Unlock()
	q.publishSnapshot(snapshot, shouldPersist)
	return true
}

func (q *UploadQueue) signalChangedLocked() {
	close(q.changed)
	q.changed = make(chan struct{})
}

func (q *UploadQueue) reportRunProgress(id string, generation uint64, done int64, percent int) {
	now := time.Now()
	q.mu.Lock()
	run := q.runs[id]
	j, ok := q.jobs[id]
	if !ok || run == nil || run.generation != generation || j.Upload.IsStop {
		q.mu.Unlock()
		return
	}
	j.Upload.DownSize = done
	j.Upload.DownProcess = percent
	last := q.lastProgress[id]
	publish := last.IsZero() || now.Sub(last) >= 500*time.Millisecond || percent >= 100
	if publish {
		q.lastProgress[id] = now
		snapshot := *j
		shouldPersist := q.shouldPersistLocked(snapshot)
		q.mu.Unlock()
		q.publishSnapshot(snapshot, shouldPersist)
		return
	}
	q.mu.Unlock()
}

func (q *UploadQueue) shouldPersistLocked(job model.UploadingUI) bool {
	now := time.Now()
	status := uploadStatus(job.Upload)
	previous, ok := q.lastPersist[job.UploadID]
	terminal := job.Upload.IsCompleted || job.Upload.IsFailed || job.Upload.IsStop
	if !ok || previous.status != status || terminal || now.Sub(previous.at) >= transferProgressPersistInterval {
		q.lastPersist[job.UploadID] = progressPersistState{status: status, at: now}
		return true
	}
	return false
}

func uploadStatus(u model.UploadState) string {
	switch {
	case u.IsCompleted:
		return "completed"
	case u.IsFailed:
		return "failed"
	case u.IsStop:
		return "paused"
	case u.IsDowning:
		return "uploading"
	default:
		return "queued"
	}
}
