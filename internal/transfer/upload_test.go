package transfer

import (
	"context"
	"errors"
	"testing"
	"time"

	"mnemo-go/internal/model"
	"mnemo-go/internal/store"
)

func waitUploadRunState(t *testing.T, q *UploadQueue, id string, active bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
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
