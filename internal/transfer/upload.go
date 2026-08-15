package transfer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
	"mnemo-go/internal/store"
)

// UploadQueue manages upload jobs for queue-mode providers.
type UploadQueue struct {
	store   *store.Store
	mu      sync.Mutex
	jobs    map[string]*model.UploadingUI
	onEvent OnTaskEvent
	ctx     context.Context // root context, canceled on Close
	sem     chan struct{}    // global concurrency slot
	cancels map[string]context.CancelFunc
}

// NewUploadQueue creates the upload queue and restores persisted jobs.
func NewUploadQueue(st *store.Store, onEvent OnTaskEvent) *UploadQueue {
	maxConc := 2
	if s, err := st.GetSettings(); err == nil && s.MaxConcurrentDownloads > 0 {
		// reuse the concurrency setting as an upper bound for uploads too
		maxConc = s.MaxConcurrentDownloads
	}
	q := &UploadQueue{
		store:   st,
		jobs:    map[string]*model.UploadingUI{},
		onEvent: onEvent,
		ctx:     context.Background(),
		sem:     make(chan struct{}, maxConc),
		cancels: map[string]context.CancelFunc{},
	}
	// restore persisted tasks as paused (user must resume manually)
	if list, err := st.ListUploadTasks(); err == nil {
		for i := range list {
			j := list[i]
			if j.Upload.IsDowning {
				j.Upload.IsDowning = false
				j.Upload.DownState = "paused"
			}
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
	q.mu.Lock()
	q.jobs[j.UploadID] = j
	q.mu.Unlock()
	_ = q.store.SaveUploadTask(j)
	if q.onEvent != nil {
		t := model.DownloadTask{
			ID: j.UploadID, Name: j.Info.Name, Size: j.Info.Size,
			Downloaded: j.Upload.DownSize, Speed: j.Upload.DownSpeed,
			Progress: j.Upload.DownProcess, Status: uploadStatus(j.Upload),
			Created: j.Upload.DownTime, Updated: time.Now().Unix(),
		}
		q.onEvent(TaskEvent{Kind: "upload", Task: t})
	}
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

// AddFiles enqueues local files/dirs into a parent folder.
func (q *UploadQueue) AddFiles(userID, driveID, parentID string, localPaths []string) []*model.UploadingUI {
	var created []*model.UploadingUI
	for _, p := range localPaths {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if info.IsDir() {
			// walk folder
			_ = filepath.Walk(p, func(path string, fi os.FileInfo, err error) error {
				if err != nil || fi.IsDir() {
					return nil
				}
				rel, _ := filepath.Rel(filepath.Dir(p), path)
				name := strings.ReplaceAll(rel, "\\", "/")
				if j := q.enqueue(userID, driveID, parentID, path, name, fi.Size()); j != nil {
					created = append(created, j)
				}
				return nil
			})
		} else {
			if j := q.enqueue(userID, driveID, parentID, p, info.Name(), info.Size()); j != nil {
				created = append(created, j)
			}
		}
	}
	return created
}

func (q *UploadQueue) enqueue(userID, driveID, parentID, localPath, name string, size int64) *model.UploadingUI {
	j := &model.UploadingUI{
		UploadID: newID("up"),
		UserID:   userID,
		Info: model.UploadInfo{
			LocalFilePath: localPath, ParentFileID: parentID,
			DriveID: driveID, Name: name, Size: size,
			SizeStr: model.FormatBytes(size),
			IsDir:   false,
		},
		Upload: model.UploadState{
			DownState: "queued", DownTime: time.Now().Unix(), DownSize: 0,
		},
	}
	q.update(j)
	go q.runUpload(userID, driveID, j)
	return j
}

// runUpload executes one job through the provider's UploadOneFile. It waits
// for a global concurrency slot and runs under a cancelable context so Cancel
// can stop the provider request, not just flip a UI flag.
func (q *UploadQueue) runUpload(userID, driveID string, j *model.UploadingUI) {
	// wait for a slot
	select {
	case q.sem <- struct{}{}:
		defer func() { <-q.sem }()
	case <-q.ctx.Done():
		return
	}

	ctx, cancel := context.WithCancel(q.ctx)
	q.mu.Lock()
	q.cancels[j.UploadID] = cancel
	q.mu.Unlock()
	defer func() {
		cancel()
		q.mu.Lock()
		delete(q.cancels, j.UploadID)
		q.mu.Unlock()
	}()

	// honor a stop requested while the job was queued
	q.mu.Lock()
	if j.Upload.IsStop {
		q.mu.Unlock()
		return
	}
	j.Upload.IsDowning = true
	j.Upload.DownState = "uploading"
	q.mu.Unlock()
	q.update(j)

	// compute hash for providers that support 秒传 (optional, best-effort)
	if method := rapidMethod2(drive.ProviderOf(userID, driveID, "")); method != "" {
		if h, err := netx.HashFile(j.Info.LocalFilePath, netx.HashKind(method)); err == nil {
			j.Info.SHA1 = h
		}
	}

	handler, err := drive.QueueUploadHandler(userID, driveID)
	if err != nil {
		j.Upload.IsDowning = false
		j.Upload.IsFailed = true
		j.Upload.FailedMessage = err.Error()
		q.update(j)
		return
	}

	// 上传中周期推送进度（handler 内只写字段，事件由这里泵出）
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(500 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				q.update(j)
			}
		}
	}()
	handlerErr := handler(ctx, j)
	close(done)
	if handlerErr != nil {
		j.Upload.IsDowning = false
		if errors.Is(ctx.Err(), context.Canceled) {
			j.Upload.IsStop = true
			j.Upload.DownState = "stopped"
		} else {
			j.Upload.IsFailed = true
			j.Upload.FailedCode = 1
			j.Upload.FailedMessage = handlerErr.Error()
			j.Upload.DownState = "failed"
		}
		q.update(j)
		return
	}
	j.Upload.IsDowning = false
	j.Upload.IsCompleted = true
	j.Upload.DownProcess = 100
	j.Upload.DownSize = j.Info.Size
	j.Upload.DownState = "completed"
	q.update(j)
}

// rapidMethod returns the fingerprint kind a provider supports for upload.
func rapidMethod2(provider string) string {
	caps := drive.RegistryCaps(provider)
	for _, m := range caps.RapidUploadHashes {
		if m == "sha1" || m == "md5" {
			return m
		}
	}
	return ""
}

// Cancel stops a job by canceling its provider request and marking it stopped.
func (q *UploadQueue) Cancel(id string) {
	q.mu.Lock()
	if c, ok := q.cancels[id]; ok {
		c()
		delete(q.cancels, id)
	}
	j, ok := q.jobs[id]
	q.mu.Unlock()
	if ok {
		j.Upload.IsStop = true
		j.Upload.IsDowning = false
		j.Upload.DownState = "stopped"
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
	userID := j.UserID
	driveID := j.Info.DriveID
	q.mu.Unlock()
	q.update(j)
	go q.runUpload(userID, driveID, j)
	return nil
}

// ClearCompleted removes finished uploads.
func (q *UploadQueue) ClearCompleted() {
	q.mu.Lock()
	for id, j := range q.jobs {
		if j.Upload.IsCompleted || j.Upload.IsFailed || j.Upload.IsStop {
			delete(q.jobs, id)
		}
	}
	q.mu.Unlock()
	_ = q.store.ClearUploadTasks()
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

// Close cancels all in-flight uploads and persists pending state.
func (q *UploadQueue) Close() {
	q.mu.Lock()
	for _, c := range q.cancels {
		c()
	}
	for _, j := range q.jobs {
		if !j.Upload.IsCompleted && !j.Upload.IsFailed && !j.Upload.IsStop {
			_ = q.store.SaveUploadTask(j)
		}
	}
	q.mu.Unlock()
}
