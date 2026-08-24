package transfer

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mnemo-go/internal/logging"
	"mnemo-go/internal/model"
	"mnemo-go/internal/store"
)

// MarkCleanupOnSuccess marks an application-owned temporary upload source.
// The source is removed only after the provider reports success; failed or
// paused jobs keep the file available for retry.
func (q *UploadQueue) MarkCleanupOnSuccess(id, localPath string) bool {
	localPath = strings.TrimSpace(localPath)
	if id == "" || localPath == "" {
		return false
	}
	attached := false
	marked := q.mutateJob(id, func(j *model.UploadingUI) {
		if filepath.Clean(j.Info.LocalFilePath) == filepath.Clean(localPath) {
			j.Info.CleanupLocalFile = true
			attached = true
		}
	})
	if !marked || !attached {
		return false
	}
	// A very small file may finish before the caller can attach the marker.
	// Check the terminal state once more so that this race does not leak the
	// application-owned temporary source.
	q.mu.Lock()
	completed := q.jobs[id]
	var snapshot *model.UploadingUI
	if completed != nil && completed.Upload.IsCompleted {
		copy := *completed
		snapshot = &copy
	}
	q.mu.Unlock()
	q.cleanupTemporarySource(snapshot)
	return true
}

// Wait waits for one upload to reach a terminal state. It is used by flows
// such as online text editing where reporting success before the remote write
// finishes would be misleading. Ordinary queue uploads remain asynchronous.
func (q *UploadQueue) Wait(id string, timeout time.Duration) (*model.UploadingUI, error) {
	if q == nil || strings.TrimSpace(id) == "" {
		return nil, errors.New("上传任务不存在")
	}
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		q.mu.Lock()
		job, ok := q.jobs[id]
		if ok && job != nil {
			snapshot := *job
			changed := q.changed
			q.mu.Unlock()
			switch {
			case snapshot.Upload.IsCompleted:
				return &snapshot, nil
			case snapshot.Upload.IsFailed:
				message := strings.TrimSpace(snapshot.Upload.FailedMessage)
				if message == "" {
					message = "远端上传失败"
				}
				return &snapshot, errors.New(message)
			case snapshot.Upload.IsStop:
				return &snapshot, errors.New("上传已停止")
			}
			select {
			case <-q.ctx.Done():
				return nil, errors.New("上传服务已停止")
			case <-deadline.C:
				return nil, errors.New("等待上传完成超时")
			case <-changed:
				continue
			}
		} else {
			q.mu.Unlock()
			return nil, errors.New("上传任务不存在")
		}
	}
}

func (q *UploadQueue) cleanupTemporarySource(j *model.UploadingUI) {
	if j == nil || !j.Info.CleanupLocalFile {
		return
	}
	path := filepath.Clean(strings.TrimSpace(j.Info.LocalFilePath))
	if path == "." || !isMnemoEditTemp(path) {
		logging.Warn("temporary upload cleanup skipped", "job_id", j.UploadID, "reason", "path outside managed temp directory")
		return
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		logging.Warn("temporary upload file cleanup failed", "job_id", j.UploadID, "error", err)
		return
	}
	if err := os.Remove(filepath.Dir(path)); err != nil && !os.IsNotExist(err) {
		logging.Debug("temporary upload directory cleanup skipped", "job_id", j.UploadID, "error", err)
	}
}

func isMnemoEditTemp(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	tmpRoot, err := filepath.Abs(os.TempDir())
	if err != nil || filepath.Dir(filepath.Dir(abs)) != tmpRoot {
		return false
	}
	dir := filepath.Dir(abs)
	return strings.HasPrefix(filepath.Base(dir), "mnemo_edit_")
}

// Cancel stops a job by canceling its provider request and marking it stopped.
func (q *UploadQueue) Cancel(id string) {
	q.mu.Lock()
	if run, ok := q.runs[id]; ok {
		run.cancel()
	}
	j, ok := q.jobs[id]
	if ok {
		j.Upload.IsStop = true
		j.Upload.IsDowning = false
		j.Upload.DownState = "stopped"
	}
	q.mu.Unlock()
	if ok {
		q.update(j)
	}
}

// Resume restarts a paused, failed or stopped job.
func (q *UploadQueue) Resume(id string) error {
	q.mu.Lock()
	j, ok := q.jobs[id]
	if !ok {
		q.mu.Unlock()
		return errors.New("上传任务不存在")
	}
	if _, active := q.runs[id]; active {
		q.mu.Unlock()
		return errUploadWorkerStopping
	}
	if j.Upload.IsDowning {
		q.mu.Unlock()
		return errors.New("任务正在上传中")
	}
	if j.Upload.IsCompleted {
		q.mu.Unlock()
		return errors.New("任务已完成")
	}
	// reset state and re-enqueue
	j.Upload.IsStop = false
	j.Upload.IsFailed = false
	j.Upload.FailedMessage = ""
	j.Upload.DownState = "queued"
	q.mu.Unlock()
	q.update(j)
	return q.startUpload(id)
}

// ClearCompleted removes finished uploads.
func (q *UploadQueue) ClearCompleted() {
	q.mu.Lock()
	for id, j := range q.jobs {
		if _, active := q.runs[id]; active {
			continue
		}
		if j.Upload.IsCompleted || j.Upload.IsFailed || j.Upload.IsStop {
			delete(q.jobs, id)
			delete(q.lastPersist, id)
			delete(q.generations, id)
			delete(q.lastProgress, id)
		}
	}
	q.signalChangedLocked()
	q.mu.Unlock()
	q.dirMu.Lock()
	q.dirIDs = map[string]string{}
	q.dirMu.Unlock()
	q.persistMu.Lock()
	_ = q.store.ClearUploadTasks()
	q.persistMu.Unlock()
}

// DownloadDir returns the configured download dir (helper).
func DownloadDir(st *store.Store) string {
	s, err := st.GetSettings()
	if err != nil || s.DownloadDir == "" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Downloads")
	}
	return s.DownloadDir
}

// Close cancels all in-flight uploads, waits for directory scanners/workers to
// stop touching queue persistence, and then saves the pre-cancel pending state.
func (q *UploadQueue) Close() {
	q.closeOnce.Do(func() {
		q.mu.Lock()
		q.closed = true
		var pending []*model.UploadingUI
		seen := make(map[string]struct{}, len(q.jobs))
		for _, j := range q.jobs {
			if !j.Upload.IsCompleted && !j.Upload.IsFailed && !j.Upload.IsStop {
				snapshot := *j
				pending = append(pending, &snapshot)
				seen[j.UploadID] = struct{}{}
			}
		}
		if q.cancel != nil {
			q.cancel()
		}
		for _, run := range q.runs {
			run.cancel()
		}
		q.mu.Unlock()

		q.workerWG.Wait()

		// A directory scanner may have passed its cancellation check immediately
		// before Close. Preserve any resulting queued item that was not part of
		// the first snapshot.
		q.mu.Lock()
		for _, j := range q.jobs {
			if _, ok := seen[j.UploadID]; ok || j.Upload.IsCompleted || j.Upload.IsFailed {
				continue
			}
			snapshot := *j
			pending = append(pending, &snapshot)
		}
		q.mu.Unlock()

		q.persistMu.Lock()
		for _, j := range pending {
			_ = q.store.SaveUploadTask(j)
		}
		q.persistMu.Unlock()
	})
}
