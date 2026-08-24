package transfer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mnemo-go/internal/model"
	"mnemo-go/internal/store"
)

func waitUploadRunState(t *testing.T, q *UploadQueue, id string, active bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		q.mu.Lock()
		_, ok := q.runs[id]
		q.mu.Unlock()
		if ok == active {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("upload run %q active=%v did not become %v", id, !active, active)
}

func TestUploadQueueCloseWaitsForActiveWorker(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	q := NewUploadQueue(st, nil)
	started := make(chan struct{})
	stopped := make(chan struct{})
	q.handlerResolver = func(_, _ string) (func(context.Context, *model.UploadingUI) error, error) {
		return func(ctx context.Context, _ *model.UploadingUI) error {
			close(started)
			<-ctx.Done()
			close(stopped)
			return ctx.Err()
		}, nil
	}
	path := filepath.Join(t.TempDir(), "payload.bin")
	if err := os.WriteFile(path, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	q.enqueue("unknown_user", "unknown_drive", "root", "rename", path, "payload.bin", 7)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("upload worker did not start")
	}

	q.Close()
	select {
	case <-stopped:
	default:
		t.Fatal("Close returned before the active worker observed cancellation")
	}
}

func TestNewUploadQueueRestoresPaused(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	// save a job that looks "in-flight"
	j := &model.UploadingUI{
		UploadID: "up1",
		UserID:   "pikpak_test",
		Info: model.UploadInfo{
			Name:          "test.txt",
			Size:          100,
			LocalFilePath: "/tmp/test.txt",
			DriveID:       "pikpak_test",
		},
		Upload: model.UploadState{
			DownState: "uploading",
			IsDowning: true,
			DownTime:  1,
			UploadID:  "up1",
		},
	}
	_ = st.SaveUploadTask(j)

	q := NewUploadQueue(st, nil)
	if got := len(q.jobs); got != 1 {
		t.Fatalf("expected 1 restored job, got %d", got)
	}
	r := q.jobs["up1"]
	if r == nil {
		t.Fatal("job up1 not restored")
	}
	if r.Upload.IsDowning {
		t.Error("in-flight job should be restored as paused")
	}
	if r.Upload.DownState != "paused" {
		t.Errorf("expected paused, got %s", r.Upload.DownState)
	}
}

func TestUploadQueueKeepTasksDisabledClearsFinishedHistory(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	settings := store.DefaultSettings()
	settings.KeepTasks = false
	if err := st.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	for i, state := range []model.UploadState{{IsCompleted: true}, {IsFailed: true}, {IsStop: true}} {
		if err := st.SaveUploadTask(&model.UploadingUI{UploadID: fmt.Sprintf("finished-%d", i), Upload: state}); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.SaveUploadTask(&model.UploadingUI{UploadID: "paused", Upload: model.UploadState{DownState: "paused"}}); err != nil {
		t.Fatal(err)
	}
	q := NewUploadQueue(st, nil)
	defer q.Close()
	if _, ok := q.jobs["paused"]; !ok {
		t.Fatal("paused upload should remain resumable when history retention is disabled")
	}
	if _, ok := q.jobs["finished-0"]; ok {
		t.Fatal("finished upload history was restored despite KeepTasks=false")
	}
	list, err := st.ListUploadTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].UploadID != "paused" {
		t.Fatalf("finished upload history was not cleared: %#v", list)
	}
}

func TestUploadQueueCancelNotFound(t *testing.T) {
	dir := t.TempDir()
	st, _ := store.Open(dir)
	q := NewUploadQueue(st, nil)
	// should not panic on unknown id
	q.Cancel("nonexistent")
}

func TestUploadQueueResumeNotFound(t *testing.T) {
	dir := t.TempDir()
	st, _ := store.Open(dir)
	q := NewUploadQueue(st, nil)
	if err := q.Resume("nonexistent"); err == nil {
		t.Error("expected error resuming nonexistent job")
	}
}

func TestUploadQueueWaitReturnsTerminalResult(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	q := NewUploadQueue(st, nil)
	defer q.Close()
	job := &model.UploadingUI{UploadID: "up-wait", Upload: model.UploadState{DownState: "uploading", IsDowning: true}}
	q.update(job)
	resultCh := make(chan struct {
		job *model.UploadingUI
		err error
	}, 1)
	go func() {
		got, waitErr := q.Wait(job.UploadID, time.Second)
		resultCh <- struct {
			job *model.UploadingUI
			err error
		}{got, waitErr}
	}()
	time.Sleep(30 * time.Millisecond)
	if !q.mutateJob(job.UploadID, func(j *model.UploadingUI) {
		j.Upload.IsDowning = false
		j.Upload.IsCompleted = true
		j.Upload.DownState = "completed"
	}) {
		t.Fatal("failed to mark upload completed")
	}
	select {
	case result := <-resultCh:
		if result.err != nil || result.job == nil || !result.job.Upload.IsCompleted {
			t.Fatalf("Wait result = %#v, want completed job", result)
		}
	case <-time.After(time.Second):
		t.Fatal("Wait did not return after completion")
	}
}

func TestUploadQueueWaitReturnsProviderFailure(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	q := NewUploadQueue(st, nil)
	defer q.Close()
	job := &model.UploadingUI{UploadID: "up-wait-failed", Upload: model.UploadState{DownState: "failed", IsFailed: true, FailedMessage: "服务端拒绝"}}
	q.update(job)
	got, waitErr := q.Wait(job.UploadID, time.Second)
	if got == nil || waitErr == nil || waitErr.Error() != "服务端拒绝" {
		t.Fatalf("Wait failure = job %#v, err %v", got, waitErr)
	}
}

func TestUploadConcurrencyGateCanShrinkAndGrow(t *testing.T) {
	gate := newUploadConcurrencyGate(2)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := gate.acquire(ctx); err != nil {
		t.Fatal(err)
	}
	if err := gate.acquire(ctx); err != nil {
		t.Fatal(err)
	}
	gate.setLimit(1)
	blocked := make(chan error, 1)
	go func() { blocked <- gate.acquire(ctx) }()
	gate.release()
	select {
	case err := <-blocked:
		t.Fatalf("shrunk gate admitted a third worker: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	gate.setLimit(2)
	select {
	case err := <-blocked:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("grown gate did not wake a waiting worker")
	}
	gate.release()
	gate.release()
}

func TestUploadQueueDirectoryScanRunsAsynchronously(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 20; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("file-%02d.txt", i)), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	q := NewUploadQueue(st, nil)
	defer q.Close()
	// Avoid network work: scanning/enqueue behavior is the unit under test.
	q.handlerResolver = func(_, _ string) (func(context.Context, *model.UploadingUI) error, error) {
		return func(context.Context, *model.UploadingUI) error { return nil }, nil
	}
	created := q.AddFiles("unknown_user", "unknown_drive", "root", "overwrite", []string{root})
	if len(created) != 0 {
		t.Fatalf("directory scan blocked to return %d jobs synchronously", len(created))
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(q.List()) == 20 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("background directory scan produced %d jobs, want 20", len(q.List()))
}

func TestUploadEventIdentifiesChangedDirectory(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var emitted TaskEvent
	q := NewUploadQueue(st, func(event TaskEvent) { emitted = event })
	defer q.Close()

	q.publishSnapshot(model.UploadingUI{
		UploadID: "upload-event",
		UserID:   "account-1",
		Info: model.UploadInfo{
			DriveID:      "drive-1",
			ParentFileID: "folder-1",
			Name:         "done.txt",
		},
		Upload: model.UploadState{IsCompleted: true, DownState: "completed"},
	}, false)

	if emitted.Kind != "upload" || emitted.Task.Status != "completed" {
		t.Fatalf("upload event state = %#v", emitted)
	}
	if emitted.Task.UserID != "account-1" || emitted.Task.DriveID != "drive-1" || emitted.Task.ParentID != "folder-1" {
		t.Fatalf("upload event directory identity = %#v", emitted.Task)
	}
}

func TestUploadQueueCleansManagedCloudTextTempAfterSuccess(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	q := NewUploadQueue(st, nil)
	defer q.Close()

	tmpDir, err := os.MkdirTemp(os.TempDir(), "mnemo_edit_upload_test_")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)
	tmpPath := filepath.Join(tmpDir, "note.txt")
	if err := os.WriteFile(tmpPath, []byte("draft"), 0o600); err != nil {
		t.Fatal(err)
	}

	job := &model.UploadingUI{
		UploadID: "up-cleanup",
		Info:     model.UploadInfo{LocalFilePath: tmpPath, Name: "note.txt"},
		Upload:   model.UploadState{IsCompleted: true, DownState: "completed"},
	}
	q.update(job)
	if !q.MarkCleanupOnSuccess(job.UploadID, tmpPath) {
		t.Fatal("expected cleanup marker to attach")
	}
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Fatalf("managed temp source still exists, stat error: %v", err)
	}

	outside := filepath.Join(t.TempDir(), "keep.txt")
	if err := os.WriteFile(outside, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	outsideJob := &model.UploadingUI{
		UploadID: "up-no-cleanup",
		Info:     model.UploadInfo{LocalFilePath: outside, CleanupLocalFile: true},
	}
	q.cleanupTemporarySource(outsideJob)
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("unmanaged source should remain, stat error: %v", err)
	}
}

func TestUploadProgressPersistenceIsThrottled(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	q := NewUploadQueue(st, nil)
	defer q.Close()

	job := &model.UploadingUI{
		UploadID: "up-progress",
		Info:     model.UploadInfo{Name: "payload.bin", Size: 1024},
		Upload:   model.UploadState{DownState: "uploading", IsDowning: true},
	}
	q.update(job)
	job.Upload.DownSize = 256
	q.update(job)

	list, err := st.ListUploadTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Upload.DownSize != 0 {
		t.Fatalf("progress update should remain in memory before checkpoint: %#v", list)
	}

	q.mu.Lock()
	state := q.lastPersist[job.UploadID]
	state.at = time.Now().Add(-transferProgressPersistInterval)
	q.lastPersist[job.UploadID] = state
	q.mu.Unlock()
	q.update(job)

	list, err = st.ListUploadTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Upload.DownSize != 256 {
		t.Fatalf("checkpoint should persist latest progress: %#v", list)
	}
}

func TestUploadQueueResumeWaitsForCanceledWorkerExit(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	q := NewUploadQueue(st, nil)
	defer q.Close()

	started := make(chan int, 2)
	releaseFirst := make(chan struct{})
	calls := 0
	q.handlerResolver = func(_, _ string) (func(context.Context, *model.UploadingUI) error, error) {
		return func(ctx context.Context, _ *model.UploadingUI) error {
			calls++
			call := calls
			started <- call
			<-ctx.Done()
			if call == 1 {
				<-releaseFirst
			}
			return ctx.Err()
		}, nil
	}

	job := q.enqueue("test-user", "test-drive", "parent", "rename", "payload.bin", "payload.bin", 64)
	if job == nil {
		t.Fatal("enqueue returned nil")
	}
	select {
	case call := <-started:
		if call != 1 {
			t.Fatalf("first worker call = %d", call)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first upload worker did not start")
	}

	q.Cancel(job.UploadID)
	if err := q.Resume(job.UploadID); err == nil || !errors.Is(err, errUploadWorkerStopping) {
		t.Fatalf("Resume while old worker is exiting = %v", err)
	}
	select {
	case call := <-started:
		t.Fatalf("unexpected overlapping upload worker call %d", call)
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseFirst)
	waitUploadRunState(t, q, job.UploadID, false)
	if err := q.Resume(job.UploadID); err != nil {
		t.Fatalf("Resume after old worker exited: %v", err)
	}
	select {
	case call := <-started:
		if call != 2 {
			t.Fatalf("resumed worker call = %d", call)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("resumed upload worker did not start")
	}
	q.Cancel(job.UploadID)
	waitUploadRunState(t, q, job.UploadID, false)
}
