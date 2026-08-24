package transfer

import (
	"context"
	"errors"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/logging"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

func (q *UploadQueue) startUpload(id string) error {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return errUploadQueueClosed
	}
	j, ok := q.jobs[id]
	if !ok {
		q.mu.Unlock()
		return errors.New("上传任务不存在")
	}
	if _, active := q.runs[id]; active {
		q.mu.Unlock()
		return errUploadWorkerStopping
	}
	q.generations[id]++
	generation := q.generations[id]
	ctx, cancel := context.WithCancel(q.ctx)
	q.runs[id] = &uploadRun{
		generation: generation,
		cancel:     cancel,
		done:       make(chan struct{}),
	}
	userID := j.UserID
	driveID := j.Info.DriveID
	q.workerWG.Add(1)
	q.mu.Unlock()

	go func() {
		defer q.workerWG.Done()
		q.runUpload(userID, driveID, id, generation, ctx)
	}()
	return nil
}

func (q *UploadQueue) finishUploadRun(id string, generation uint64) {
	q.mu.Lock()
	if run, ok := q.runs[id]; ok && run.generation == generation {
		delete(q.runs, id)
		delete(q.lastProgress, id)
		close(run.done)
	}
	q.mu.Unlock()
}

// runUpload executes one job through the provider's UploadOneFile. It waits
// for a global concurrency slot and runs under a cancelable context so Cancel
// can stop the provider request, not just flip a UI flag.
func (q *UploadQueue) runUpload(userID, driveID, id string, generation uint64, ctx context.Context) {
	defer q.finishUploadRun(id, generation)
	q.mu.Lock()
	run := q.runs[id]
	j, ok := q.jobs[id]
	if !ok || run == nil || run.generation != generation {
		q.mu.Unlock()
		return
	}
	initial := *j
	q.mu.Unlock()

	started := time.Now()
	logging.Info("upload worker started", "job_id", id, "name", initial.Info.Name, "size", initial.Info.Size)
	// wait for a dynamically resizable upload slot
	if err := q.gate.acquire(ctx); err != nil {
		logging.Debug("upload worker canceled before start", "job_id", id)
		return
	}
	defer q.gate.release()

	// The provider receives a worker-local copy. Only immutable progress samples
	// and the final result are merged back into the queue-owned task.
	worker := initial
	canStart := false
	if !q.mutateRunJob(id, generation, func(j *model.UploadingUI) {
		if j.Upload.IsStop || ctx.Err() != nil {
			return
		}
		j.Upload.IsDowning = true
		j.Upload.DownState = "uploading"
		worker = *j
		canStart = true
	}) || !canStart {
		logging.Debug("upload worker skipped", "job_id", id, "reason", "job already stopped")
		return
	}

	// Directory uploads are queued as files, but remote providers generally do
	// not interpret a slash in the file name as an implicit mkdir. Resolve the
	// path before invoking the provider and pass only the leaf name onward.
	relative := worker.Info.Path
	if relative == "" {
		relative = worker.Info.Name
	}
	relative = normalizeUploadPath(relative)
	if relative == "" {
		q.mutateRunJob(id, generation, func(j *model.UploadingUI) {
			j.Upload.IsDowning = false
			j.Upload.IsFailed = true
			j.Upload.DownState = "failed"
			j.Upload.FailedMessage = "上传路径无效"
		})
		logging.Warn("upload path validation failed", "job_id", id)
		return
	}
	worker.Info.Path = relative
	worker.Info.Name = uploadLeaf(relative)
	parentID := strings.TrimSpace(worker.Info.ParentFileID)
	provider := drive.ProviderOf(userID, driveID, "")
	if parentID == "" || drive.IsRootID(provider, parentID) {
		if root, rootErr := drive.RootID(userID, driveID); rootErr == nil && root != "" {
			parentID = root
		}
	}
	parentID, parentErr := q.ensureRemoteParent(ctx, userID, driveID, parentID, relative)
	if parentErr != nil {
		q.mutateRunJob(id, generation, func(j *model.UploadingUI) {
			j.Upload.IsDowning = false
			if errors.Is(ctx.Err(), context.Canceled) {
				j.Upload.IsStop = true
				j.Upload.DownState = "stopped"
			} else {
				j.Upload.IsFailed = true
				j.Upload.DownState = "failed"
			}
			j.Upload.FailedMessage = parentErr.Error()
		})
		logging.Warn("remote upload parent resolution failed", "job_id", id, "error", parentErr)
		return
	}
	worker.Info.ParentFileID = parentID
	q.mutateRunJob(id, generation, func(j *model.UploadingUI) {
		j.Info.Path = worker.Info.Path
		j.Info.Name = worker.Info.Name
		j.Info.ParentFileID = worker.Info.ParentFileID
	})

	// compute hash for providers that support 秒传 (optional, best-effort)
	if method := rapidHashMethod(drive.ProviderOf(userID, driveID, "")); method != "" {
		if h, err := netx.HashFile(worker.Info.LocalFilePath, netx.HashKind(method)); err == nil {
			worker.Info.ContentHash = h
			worker.Info.ContentHashAlgorithm = method
			if method == "sha1" {
				worker.Info.SHA1 = h
			}
		}
	}

	handler, err := q.handlerResolver(userID, driveID)
	if err != nil {
		q.mutateRunJob(id, generation, func(j *model.UploadingUI) {
			j.Upload.IsDowning = false
			j.Upload.IsFailed = true
			j.Upload.DownState = "failed"
			j.Upload.FailedMessage = err.Error()
		})
		logging.Warn("upload handler lookup failed", "job_id", id, "error", err)
		return
	}

	worker.ConfigureUploadRuntime(
		func(done int64, percent int) { q.reportRunProgress(id, generation, done, percent) },
		func() bool { return ctx.Err() != nil },
	)
	handlerErr := handler(ctx, &worker)
	q.mu.Lock()
	current := q.jobs[id]
	wasStopped := ctx.Err() != nil || (current != nil && current.Upload.IsStop)
	q.mu.Unlock()
	if handlerErr != nil {
		q.mutateRunJob(id, generation, func(j *model.UploadingUI) {
			cleanupLocalFile := j.Info.CleanupLocalFile
			j.Info = worker.Info
			j.Info.CleanupLocalFile = cleanupLocalFile || worker.Info.CleanupLocalFile
			j.Upload.FileID = worker.Upload.FileID
			j.Upload.UploadID = worker.Upload.UploadID
			j.Upload.IsDowning = false
			if wasStopped || errors.Is(ctx.Err(), context.Canceled) {
				j.Upload.IsStop = true
				j.Upload.DownState = "stopped"
			} else {
				j.Upload.IsFailed = true
				j.Upload.FailedCode = 1
				j.Upload.FailedMessage = handlerErr.Error()
				j.Upload.DownState = "failed"
			}
		})
		logging.Warn("upload worker failed", "job_id", id, "error", handlerErr, "duration", logging.Duration(started))
		return
	}
	if wasStopped {
		q.mutateRunJob(id, generation, func(j *model.UploadingUI) {
			cleanupLocalFile := j.Info.CleanupLocalFile
			j.Info = worker.Info
			j.Info.CleanupLocalFile = cleanupLocalFile || worker.Info.CleanupLocalFile
			j.Upload.FileID = worker.Upload.FileID
			j.Upload.UploadID = worker.Upload.UploadID
			j.Upload.IsDowning = false
			j.Upload.IsStop = true
			j.Upload.DownState = "stopped"
		})
		logging.Info("upload worker stopped", "job_id", id, "duration", logging.Duration(started))
		return
	}
	if q.mutateRunJob(id, generation, func(j *model.UploadingUI) {
		cleanupLocalFile := j.Info.CleanupLocalFile
		j.Info = worker.Info
		j.Info.CleanupLocalFile = cleanupLocalFile || worker.Info.CleanupLocalFile
		j.Upload.FileID = worker.Upload.FileID
		j.Upload.UploadID = worker.Upload.UploadID
		j.Upload.IsDowning = false
		j.Upload.IsStop = false
		j.Upload.IsFailed = false
		j.Upload.IsCompleted = true
		j.Upload.DownProcess = 100
		j.Upload.DownSize = j.Info.Size
		j.Upload.DownState = "completed"
		j.Upload.FailedCode = 0
		j.Upload.FailedMessage = ""
	}) {
		q.mu.Lock()
		completed := q.jobs[id]
		var snapshot *model.UploadingUI
		if completed != nil {
			copy := *completed
			snapshot = &copy
		}
		q.mu.Unlock()
		q.cleanupTemporarySource(snapshot)
	}
	logging.Info("upload worker finished", "job_id", id, "status", "completed", "duration", logging.Duration(started))
}

// rapidHashMethod returns a locally computable fingerprint kind supported by
// the provider's rapid-upload path.
func rapidHashMethod(provider string) string {
	caps := drive.RegistryCaps(provider)
	for _, m := range caps.RapidUploadHashes {
		normalized := strings.ToLower(strings.TrimSpace(m))
		switch normalized {
		case "sha1", "sha256", "md5":
			return normalized
		}
	}
	return ""
}
