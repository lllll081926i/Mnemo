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
	"mnemo-go/internal/netx"
	"mnemo-go/internal/store"
)

// UploadQueue manages upload jobs for queue-mode providers.
type UploadQueue struct {
	store       *store.Store
	mu          sync.Mutex
	dirMu       sync.Mutex
	jobs        map[string]*model.UploadingUI
	dirIDs      map[string]string
	lastPersist map[string]progressPersistState
	onEvent     OnTaskEvent
	ctx         context.Context // root context, canceled on Close
	cancel      context.CancelFunc
	sem         chan struct{} // global concurrency slot
	cancels     map[string]context.CancelFunc
	persistMu   sync.Mutex
}

const maxUploadDirectoryCacheEntries = 1024

// NewUploadQueue creates the upload queue and restores persisted jobs.
func NewUploadQueue(st *store.Store, onEvent OnTaskEvent) *UploadQueue {
	maxConc := 2
	if s, err := st.GetSettings(); err == nil && s.MaxConcurrentDownloads > 0 {
		// reuse the concurrency setting as an upper bound for uploads too
		maxConc = s.MaxConcurrentDownloads
	}
	rootCtx, rootCancel := context.WithCancel(context.Background())
	q := &UploadQueue{
		store:       st,
		jobs:        map[string]*model.UploadingUI{},
		dirIDs:      map[string]string{},
		lastPersist: map[string]progressPersistState{},
		onEvent:     onEvent,
		ctx:         rootCtx,
		cancel:      rootCancel,
		sem:         make(chan struct{}, maxConc),
		cancels:     map[string]context.CancelFunc{},
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
	if j == nil || j.UploadID == "" {
		return
	}
	q.mu.Lock()
	q.jobs[j.UploadID] = j
	snapshot := *j
	shouldPersist := q.shouldPersistLocked(snapshot)
	q.mu.Unlock()
	if shouldPersist {
		q.persistMu.Lock()
		_ = q.store.SaveUploadTask(&snapshot)
		q.persistMu.Unlock()
	}
	if q.onEvent != nil {
		t := model.DownloadTask{
			ID: snapshot.UploadID, Name: snapshot.Info.Name, Size: snapshot.Info.Size,
			Downloaded: snapshot.Upload.DownSize, Speed: snapshot.Upload.DownSpeed,
			Progress: snapshot.Upload.DownProcess, Status: uploadStatus(snapshot.Upload),
			Created: snapshot.Upload.DownTime, Updated: time.Now().Unix(),
		}
		q.onEvent(TaskEvent{Kind: "upload", Task: t})
	}
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

// AddFiles enqueues local files/dirs into a parent folder.
func (q *UploadQueue) AddFiles(userID, driveID, parentID, conflictPolicy string, localPaths []string) []*model.UploadingUI {
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
				if j := q.enqueue(userID, driveID, parentID, conflictPolicy, path, name, fi.Size()); j != nil {
					created = append(created, j)
				}
				return nil
			})
		} else {
			if j := q.enqueue(userID, driveID, parentID, conflictPolicy, p, info.Name(), info.Size()); j != nil {
				created = append(created, j)
			}
		}
	}
	return created
}

func (q *UploadQueue) enqueue(userID, driveID, parentID, conflictPolicy, localPath, name string, size int64) *model.UploadingUI {
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
	go q.runUpload(userID, driveID, j)
	return j
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

	q.dirMu.Lock()
	defer q.dirMu.Unlock()
	current := baseParent
	for _, segment := range parts[:len(parts)-1] {
		key := strings.Join([]string{userID, driveID, baseParent, current, segment}, "\x00")
		if id := q.dirIDs[key]; id != "" {
			current = id
			continue
		}

		items, err := drive.ListDir(userID, driveID, current, nil)
		if err != nil {
			return "", fmt.Errorf("查找远端目录 %q 失败: %w", segment, err)
		}
		found := ""
		for _, item := range items {
			if item.IsDir && item.Name == segment {
				found = item.FileID
				break
			}
		}
		if found == "" {
			result, mkdirErr := drive.Mkdir(userID, driveID, current, segment)
			if mkdirErr != nil {
				return "", fmt.Errorf("创建远端目录 %q 失败: %w", segment, mkdirErr)
			}
			if result == nil || result.FileID == "" {
				if result != nil && result.Error != "" {
					return "", fmt.Errorf("创建远端目录 %q 失败: %s", segment, result.Error)
				}
				return "", fmt.Errorf("创建远端目录 %q 未返回目录 id", segment)
			}
			found = result.FileID
		}
		if len(q.dirIDs) >= maxUploadDirectoryCacheEntries {
			q.dirIDs = map[string]string{}
		}
		q.dirIDs[key] = found
		current = found
	}
	return current, nil
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

	// Directory uploads are queued as files, but remote providers generally do
	// not interpret a slash in the file name as an implicit mkdir. Resolve the
	// path before invoking the provider and pass only the leaf name onward.
	relative := j.Info.Path
	if relative == "" {
		relative = j.Info.Name
	}
	relative = normalizeUploadPath(relative)
	if relative == "" {
		j.Upload.IsDowning = false
		j.Upload.IsFailed = true
		j.Upload.DownState = "failed"
		j.Upload.FailedMessage = "上传路径无效"
		q.update(j)
		return
	}
	j.Info.Path = relative
	j.Info.Name = uploadLeaf(relative)
	parentID := strings.TrimSpace(j.Info.ParentFileID)
	provider := drive.ProviderOf(userID, driveID, "")
	if parentID == "" || drive.IsRootID(provider, parentID) {
		if root, rootErr := drive.RootID(userID, driveID); rootErr == nil && root != "" {
			parentID = root
		}
	}
	parentID, parentErr := q.ensureRemoteParent(ctx, userID, driveID, parentID, relative)
	if parentErr != nil {
		j.Upload.IsDowning = false
		if errors.Is(ctx.Err(), context.Canceled) {
			j.Upload.IsStop = true
			j.Upload.DownState = "stopped"
		} else {
			j.Upload.IsFailed = true
			j.Upload.DownState = "failed"
		}
		j.Upload.FailedMessage = parentErr.Error()
		q.update(j)
		return
	}
	j.Info.ParentFileID = parentID
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
	progressStopped := make(chan struct{})
	go func() {
		defer close(progressStopped)
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
	<-progressStopped
	q.mu.Lock()
	wasStopped := j.Upload.IsStop || ctx.Err() != nil
	q.mu.Unlock()
	if handlerErr != nil {
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
		q.update(j)
		return
	}
	if wasStopped {
		j.Upload.IsDowning = false
		j.Upload.DownState = "stopped"
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
			delete(q.lastPersist, id)
		}
	}
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

// Close cancels all in-flight uploads and persists pending state.
func (q *UploadQueue) Close() {
	if q.cancel != nil {
		q.cancel()
	}
	q.mu.Lock()
	var pending []*model.UploadingUI
	for _, c := range q.cancels {
		c()
	}
	for _, j := range q.jobs {
		if !j.Upload.IsCompleted && !j.Upload.IsFailed && !j.Upload.IsStop {
			snapshot := *j
			pending = append(pending, &snapshot)
		}
	}
	q.mu.Unlock()
	q.persistMu.Lock()
	for _, j := range pending {
		_ = q.store.SaveUploadTask(j)
	}
	q.persistMu.Unlock()
}
