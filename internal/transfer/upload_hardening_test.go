package transfer

import (
	"context"
	"testing"
	"time"

	"mnemo-go/internal/model"
	"mnemo-go/internal/store"
)

func TestClearCompletedPreservesFailedAndPausedUploads(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	q := NewUploadQueue(st, nil)
	defer q.Close()
	for _, job := range []model.UploadingUI{
		{UploadID: "done", Upload: model.UploadState{IsCompleted: true}},
		{UploadID: "failed", Upload: model.UploadState{IsFailed: true}},
		{UploadID: "paused", Upload: model.UploadState{IsStop: true}},
	} {
		q.update(&job)
	}
	q.ClearCompleted()
	remaining := map[string]bool{}
	for _, task := range q.List() {
		remaining[task.UploadID] = true
	}
	if remaining["done"] || !remaining["failed"] || !remaining["paused"] {
		t.Fatalf("completed cleanup removed retryable task: %+v", remaining)
	}
	saved, err := st.ListUploadTasks()
	if err != nil {
		t.Fatal(err)
	}
	kept := map[string]bool{}
	for _, task := range saved {
		kept[task.UploadID] = true
	}
	if kept["done"] || !kept["failed"] || !kept["paused"] {
		t.Fatalf("persisted upload task cleanup mismatch: %+v", kept)
	}
}

func TestLateCancellationCannotDowngradeConfirmedUpload(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	q := NewUploadQueue(st, nil)
	defer q.Close()
	done := make(chan struct{})
	q.handlerResolver = func(string, string) (func(context.Context, *model.UploadingUI) error, error) {
		return func(ctx context.Context, ui *model.UploadingUI) error {
			q.Cancel(ui.UploadID)
			close(done)
			return nil // Provider has confirmed remote success.
		}, nil
	}
	job := q.enqueue("account", "drive", "root", "rename", "source", "source", 0)
	if job == nil {
		t.Fatal("failed to enqueue")
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("provider was not called")
	}
	q.workers.Wait()
	list := q.List()
	if len(list) != 1 {
		t.Fatalf("tasks: %+v", list)
	}
	if !list[0].Upload.IsCompleted || list[0].Upload.IsStop {
		t.Errorf("confirmed success downgraded after cancellation: %+v", list[0].Upload)
	}
}
