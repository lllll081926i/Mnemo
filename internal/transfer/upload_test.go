package transfer

import (
	"testing"
	"time"

	"mnemo-go/internal/model"
	"mnemo-go/internal/store"
)

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
