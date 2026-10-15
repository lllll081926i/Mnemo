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

	"mnemo-go/internal/config"
	"mnemo-go/internal/drive"
	"mnemo-go/internal/logging"
	"mnemo-go/internal/model"
	"mnemo-go/internal/store"
)

// UploadQueue manages upload jobs for queue-mode providers.
type UploadQueue struct {
	store           *store.Store
	mu              sync.Mutex
	dirMu           sync.Mutex
	jobs            map[string]*model.UploadingUI
	dirIDs          map[string]string
	lastPersist     map[string]progressPersistState
	lastProgress    map[string]time.Time
	onEvent         OnTaskEvent
	ctx             context.Context // root context, canceled on Close
	cancel          context.CancelFunc
	sem             chan struct{} // global concurrency slot
	runs            map[string]*uploadRun
	generations     map[string]uint64
	handlerResolver func(userID, driveID string) (func(context.Context, *model.UploadingUI) error, error)
	persistMu       sync.Mutex
	workers         sync.WaitGroup
	closeOnce       sync.Once
	speedSamples    map[string]uploadSpeedSample
}

type uploadSpeedSample struct {
	at   time.Time
	done int64
}

type uploadRun struct {
	generation uint64
	cancel     context.CancelFunc
	done       chan struct{}
}

var errUploadWorkerStopping = errors.New("上传任务仍在停止中，请稍后重试")

const maxUploadDirectoryCacheEntries = 1024

// NewUploadQueue creates the upload queue and restores persisted jobs.
func NewUploadQueue(st *store.Store, onEvent OnTaskEvent) *UploadQueue {
	maxConc := 2
	keepTasks := true
	if s, err := st.GetSettings(); err == nil {
		// reuse the concurrency setting as an upper bound for uploads too
		if s.MaxConcurrentDownloads > 0 {
			maxConc = s.MaxConcurrentDownloads
		}
		keepTasks = s.KeepTasks
	}
	rootCtx, rootCancel := context.WithCancel(context.Background())
	q := &UploadQueue{
		store:           st,
		jobs:            map[string]*model.UploadingUI{},
		dirIDs:          map[string]string{},
		lastPersist:     map[string]progressPersistState{},
		lastProgress:    map[string]time.Time{},
		onEvent:         onEvent,
		ctx:             rootCtx,
		cancel:          rootCancel,
		sem:             make(chan struct{}, maxConc),
		runs:            map[string]*uploadRun{},
		generations:     map[string]uint64{},
		speedSamples:    map[string]uploadSpeedSample{},
		handlerResolver: drive.QueueUploadHandler,
	}
	if !keepTasks {
		if err := st.ClearFinishedUploadTasks(); err != nil {
			logging.Warn("upload task history cleanup failed", "error", err)
		}
	}
	// restore persisted tasks as paused (user must resume manually)
	if list, err := st.ListUploadTasks(); err == nil {
		for i := range list {
			j := list[i]
			if !j.Upload.IsCompleted && !j.Upload.IsFailed {
				j.Upload.IsDowning = false
				j.Upload.IsStop = true
				j.Upload.DownState = "paused"
				j.Upload.Issue = &model.OperationIssue{Code: "APP_RESTARTED", Message: "应用退出中断了任务，上传恢复能力取决于服务商；先核对目标目录", Action: "check_target"}
			}
			j.Upload.DownSpeed = 0
			j.Upload.DownSpeedStr = ""
			q.jobs[j.UploadID] = &j
		}
	}
	return q
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
	snapshot := *j
	q.jobs[j.UploadID] = &snapshot
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
	snapshot := *j
	shouldPersist := q.shouldPersistLocked(snapshot)
	q.mu.Unlock()
	q.publishSnapshot(snapshot, shouldPersist)
	return true
}

func (q *UploadQueue) publishSnapshot(snapshot model.UploadingUI, shouldPersist bool) {
	if shouldPersist {
		q.persistMu.Lock()
		q.mu.Lock()
		current := q.jobs[snapshot.UploadID]
		if current == nil {
			q.mu.Unlock()
			q.persistMu.Unlock()
			return
		}
		snapshot = *current
		q.mu.Unlock()
		if err := q.store.SaveUploadTask(&snapshot); err != nil {
			logging.Warn("upload task persistence failed", "job_id", snapshot.UploadID, "error", err)
		}
		q.persistMu.Unlock()
	}
	if q.onEvent != nil {
		t := model.DownloadTask{
			Stage: snapshot.Upload.Stage, StageProgress: snapshot.Upload.StageProgress,
			ID: snapshot.UploadID, Name: snapshot.Info.Name, Size: snapshot.Info.Size,
			Downloaded: snapshot.Upload.DownSize, Speed: snapshot.Upload.DownSpeed,
			Progress: snapshot.Upload.DownProcess, Status: uploadStatus(snapshot.Upload),
			UserID: snapshot.UserID, DriveID: snapshot.Info.DriveID, Error: snapshot.Upload.FailedMessage,
			Issue:   snapshot.Upload.Issue,
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
	snapshot := *j
	shouldPersist := q.shouldPersistLocked(snapshot)
	q.mu.Unlock()
	q.publishSnapshot(snapshot, shouldPersist)
	return true
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
	j.Upload.Stage = "uploading"
	j.Upload.StageProgress = percent
	if sample, exists := q.speedSamples[id]; exists {
		if done < sample.done {
			j.Upload.DownSpeed = 0
			q.speedSamples[id] = uploadSpeedSample{at: now, done: done}
		} else if elapsed := now.Sub(sample.at); elapsed >= 500*time.Millisecond {
			j.Upload.DownSpeed = int64(float64(done-sample.done) / elapsed.Seconds())
			q.speedSamples[id] = uploadSpeedSample{at: now, done: done}
		}
	} else {
		q.speedSamples[id] = uploadSpeedSample{at: now, done: done}
	}
	j.Upload.DownSpeedStr = model.FormatBytes(j.Upload.DownSpeed) + "/s"
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

func (q *UploadQueue) reportRunStage(id string, generation uint64, stage string, percent int) {
	q.mutateRunJob(id, generation, func(j *model.UploadingUI) {
		j.Upload.Stage = stage
		j.Upload.StageProgress = percent
	})
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

// AddFiles keeps the original public API for legacy callers. Use
// AddFilesChecked when an RPC needs a user-visible error.
func (q *UploadQueue) AddFiles(userID, driveID, parentID, conflictPolicy string, localPaths []string) []*model.UploadingUI {
	jobs, err := q.AddFilesChecked(userID, driveID, parentID, conflictPolicy, localPaths)
	if err != nil {
		logging.Warn("upload source validation failed", "error", err)
	}
	return jobs
}

func (q *UploadQueue) enqueue(userID, driveID, parentID, conflictPolicy, localPath, name string, size int64) *model.UploadingUI {
	if q.ctx.Err() != nil {
		return nil
	}
	relative := normalizeUploadPath(name)
	if relative == "" {
		relative = normalizeUploadPath(filepath.Base(localPath))
	}
	j := &model.UploadingUI{
		UploadID: newID("up"),
		UserID:   userID,
		Info: model.UploadInfo{
			LocalFilePath: localPath, ParentFileID: parentID,
			DriveID: driveID, Path: relative, Name: uploadLeaf(relative), Size: size,
			SizeStr:        model.FormatBytes(size),
			IsDir:          false,
			ConflictPolicy: conflictPolicy,
		},
		Upload: model.UploadState{
			DownState: "queued", DownTime: time.Now().Unix(), DownSize: 0,
		},
	}
	q.update(j)
	if err := q.startUpload(j.UploadID); err != nil {
		logging.Warn("upload worker could not start", "job_id", j.UploadID, "error", err)
	}
	return j
}

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
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		q.mu.Lock()
		job, ok := q.jobs[id]
		if ok && job != nil {
			snapshot := *job
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
		} else {
			q.mu.Unlock()
			return nil, errors.New("上传任务不存在")
		}

		select {
		case <-q.ctx.Done():
			return nil, errors.New("上传服务已停止")
		case <-deadline.C:
			return nil, errors.New("等待上传完成超时")
		case <-tick.C:
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

func (q *UploadQueue) startUpload(id string) error {
	q.mu.Lock()
	if q.ctx.Err() != nil {
		q.mu.Unlock()
		return errors.New("上传服务已停止")
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
	q.workers.Add(1)
	q.mu.Unlock()

	go q.runUpload(userID, driveID, id, generation, ctx)
	return nil
}

func (q *UploadQueue) finishUploadRun(id string, generation uint64) {
	q.mu.Lock()
	if run, ok := q.runs[id]; ok && run.generation == generation {
		delete(q.runs, id)
		delete(q.lastProgress, id)
		delete(q.speedSamples, id)
		close(run.done)
	}
	q.mu.Unlock()
}

// normalizeUploadPath keeps the relative path captured by a directory walk
// portable and rejects traversal segments before they can become remote
// directory names.
func normalizeUploadPath(raw string) string {
	raw = strings.ReplaceAll(strings.TrimSpace(raw), "\\", "/")
	parts := strings.Split(strings.Trim(raw, "/"), "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			return ""
		}
		out = append(out, part)
	}
	return strings.Join(out, "/")
}

func uploadLeaf(relative string) string {
	if i := strings.LastIndex(relative, "/"); i >= 0 {
		return relative[i+1:]
	}
	return relative
}

// ensureRemoteParent creates or reuses every directory in a walked local
// path. The cache is scoped by account, drive and initial remote parent, so
// switching accounts or mounted drives cannot reuse another account's ids.
func (q *UploadQueue) ensureRemoteParent(ctx context.Context, userID, driveID, baseParent, relative string) (string, error) {
	relative = normalizeUploadPath(relative)
	parts := strings.Split(relative, "/")
	if relative == "" || len(parts) <= 1 {
		return baseParent, nil
	}

	current := baseParent
	for _, segment := range parts[:len(parts)-1] {
		key := strings.Join([]string{userID, driveID, baseParent, current, segment}, "\x00")
		found, err := func() (string, error) {
			release, err := drive.LockTarget(ctx, userID, driveID, current, segment)
			if err != nil {
				return "", err
			}
			defer release()
			q.dirMu.Lock()
			cached := q.dirIDs[key]
			q.dirMu.Unlock()
			if cached != "" {
				if item, infoErr := drive.GetFileFreshContext(ctx, userID, driveID, cached); infoErr == nil && item != nil && item.IsDir {
					return cached, nil
				}
				q.dirMu.Lock()
				delete(q.dirIDs, key)
				q.dirMu.Unlock()
			}

			items, err := drive.ListDirAllContext(ctx, userID, driveID, current, nil)
			if err != nil {
				return "", fmt.Errorf("查找远端目录 %q 失败: %w", segment, err)
			}
			for _, item := range items {
				if item.IsDir && item.Name == segment {
					return item.FileID, nil
				}
			}
			result, err := drive.MkdirContext(ctx, userID, driveID, current, segment)
			if err != nil {
				return "", fmt.Errorf("创建远端目录 %q 失败: %w", segment, err)
			}
			if result == nil || result.FileID == "" {
				return "", fmt.Errorf("创建远端目录 %q 未返回目录 id", segment)
			}
			return result.FileID, nil
		}()
		if err != nil {
			return "", err
		}
		q.dirMu.Lock()
		if len(q.dirIDs) >= maxUploadDirectoryCacheEntries {
			q.dirIDs = map[string]string{}
		}
		q.dirIDs[key] = found
		q.dirMu.Unlock()
		current = found
	}
	return current, nil
}

// runUpload executes one job through the provider's UploadOneFile. It waits
// for a global concurrency slot and runs under a cancelable context so Cancel
// can stop the provider request, not just flip a UI flag.
func (q *UploadQueue) runUpload(userID, driveID, id string, generation uint64, ctx context.Context) {
	defer q.workers.Done()
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
	// wait for a slot
	select {
	case q.sem <- struct{}{}:
		defer func() { <-q.sem }()
	case <-ctx.Done():
		logging.Debug("upload worker canceled before start", "job_id", id)
		return
	}

	// The provider receives a worker-local copy. Only immutable progress samples
	// and the final result are merged back into the queue-owned task.
	worker := initial
	canStart := false
	if !q.mutateRunJob(id, generation, func(j *model.UploadingUI) {
		if j.Upload.IsStop || ctx.Err() != nil {
			return
		}
		j.Upload.IsDowning = true
		j.Upload.DownSpeed = 0
		j.Upload.DownSpeedStr = ""
		j.Upload.DownState = "uploading"
		j.Upload.Issue = nil
		j.Upload.Stage = "uploading"
		j.Upload.StageProgress = 0
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
	if strings.Contains(relative, "/") {
		q.reportRunStage(id, generation, "preparing", 0)
	}
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
			j.Upload.Issue = drive.DescribeOperationError(parentErr)
		})
		logging.Warn("remote upload parent resolution failed", "job_id", id, "error", parentErr)
		return
	}
	worker.Info.ParentFileID = parentID
	q.mutateRunJob(id, generation, func(j *model.UploadingUI) {
		j.Info.Path = worker.Info.Path
		j.Info.Name = worker.Info.Name
		j.Info.ParentFileID = worker.Info.ParentFileID
		j.Upload.Stage = "uploading"
		j.Upload.StageProgress = 0
	})

	handler, err := q.handlerResolver(userID, driveID)
	if err != nil {
		q.mutateRunJob(id, generation, func(j *model.UploadingUI) {
			j.Upload.IsDowning = false
			j.Upload.IsFailed = true
			j.Upload.DownState = "failed"
			j.Upload.FailedMessage = err.Error()
			j.Upload.Issue = drive.DescribeOperationError(err)
		})
		logging.Warn("upload handler lookup failed", "job_id", id, "error", err)
		return
	}

	worker.ConfigureUploadRuntime(
		func(done int64, percent int) { q.reportRunProgress(id, generation, done, percent) },
		func() bool { return ctx.Err() != nil },
	)
	worker.ConfigureUploadStage(func(stage string, percent int) { q.reportRunStage(id, generation, stage, percent) })
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
				j.Upload.Issue = drive.DescribeOperationError(handlerErr)
				j.Upload.DownState = "failed"
			}
		})
		logging.Warn("upload worker failed", "job_id", id, "error", handlerErr, "duration", logging.Duration(started))
		return
	}
	// A nil handler error is a confirmed remote success. Cancellation
	// requested just as the provider returns must not turn it into a paused
	// job that the user could upload a second time.
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
		j.Upload.DownSpeed = 0
		j.Upload.DownSpeedStr = ""
		j.Upload.DownProcess = 100
		j.Upload.DownSize = j.Info.Size
		j.Upload.DownState = "completed"
		j.Upload.Stage = ""
		j.Upload.StageProgress = 0
		j.Upload.FailedCode = 0
		j.Upload.FailedMessage = ""
		j.Upload.Issue = nil
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
		j.Upload.Issue = &model.OperationIssue{Code: "USER_CANCELED", Message: "上传由用户停止，恢复前请核对目标目录", Action: "check_target"}
		j.Upload.DownSpeed = 0
		j.Upload.DownSpeedStr = ""
		snapshot := *j
		shouldPersist := q.shouldPersistLocked(snapshot)
		q.mu.Unlock()
		q.publishSnapshot(snapshot, shouldPersist)
		return
	}
	q.mu.Unlock()
}

// Resume restarts a paused, failed or stopped job.
func (q *UploadQueue) Resume(id string) error {
	q.mu.Lock()
	if q.ctx.Err() != nil {
		q.mu.Unlock()
		return errors.New("上传服务已停止")
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
	j.Upload.Issue = nil
	j.Upload.DownState = "queued"
	snapshot := *j
	shouldPersist := q.shouldPersistLocked(snapshot)
	q.mu.Unlock()
	q.publishSnapshot(snapshot, shouldPersist)
	return q.startUpload(id)
}

// ClearCompleted removes finished uploads.
func (q *UploadQueue) ClearCompleted() error {
	q.persistMu.Lock()
	defer q.persistMu.Unlock()
	q.mu.Lock()
	defer q.mu.Unlock()
	var completedIDs []string
	for id, j := range q.jobs {
		if _, active := q.runs[id]; active {
			continue
		}
		if j.Upload.IsCompleted {
			completedIDs = append(completedIDs, id)
		}
	}
	if len(completedIDs) == 0 {
		return nil
	}
	if err := q.store.DeleteUploadTasks(completedIDs); err != nil {
		return err
	}
	for _, id := range completedIDs {
		delete(q.jobs, id)
		delete(q.lastPersist, id)
		delete(q.generations, id)
		delete(q.lastProgress, id)
		delete(q.speedSamples, id)
	}
	return nil
}

// DownloadDir returns the configured download dir (helper).
func DownloadDir(st *store.Store) (string, error) {
	s, err := st.GetSettings()
	if err != nil {
		return "", err
	}
	return config.ResolveDownloadDir(s.DownloadDir)
}

// Close cancels all in-flight uploads and persists pending state.
func (q *UploadQueue) Close() {
	q.closeOnce.Do(func() {
		q.mu.Lock()
		q.cancel()
		for _, run := range q.runs {
			run.cancel()
		}
		q.mu.Unlock()
		q.workers.Wait()
		q.mu.Lock()
		var pending []model.UploadingUI
		for _, job := range q.jobs {
			if !job.Upload.IsCompleted && !job.Upload.IsFailed {
				job.Upload.IsDowning = false
				job.Upload.IsStop = true
				job.Upload.DownState = "paused"
				job.Upload.DownSpeed = 0
				job.Upload.DownSpeedStr = ""
				pending = append(pending, *job)
			}
		}
		q.mu.Unlock()
		for _, job := range pending {
			q.publishSnapshot(job, true)
		}
	})
}
