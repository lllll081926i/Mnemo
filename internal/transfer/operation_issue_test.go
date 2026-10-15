package transfer

import (
	"context"
	"net"
	"testing"

	"mnemo-go/internal/model"
	"mnemo-go/internal/store"
)

func TestUploadIssuePersistsAndClearsAfterSuccess(testContext *testing.T) {
	storage, err := store.Open(testContext.TempDir())
	if err != nil {
		testContext.Fatal(err)
	}
	queue := NewUploadQueue(storage, nil)
	defer queue.Close()
	queue.handlerResolver = func(string, string) (func(context.Context, *model.UploadingUI) error, error) {
		return func(context.Context, *model.UploadingUI) error {
			return &net.OpError{Op: "write", Net: "tcp", Err: context.DeadlineExceeded}
		}, nil
	}
	job := queue.enqueue("account", "drive", "root", "rename", "source", "source", 0)
	if job == nil {
		testContext.Fatal("加入队列失败")
	}
	queue.workers.Wait()
	saved, err := storage.ListUploadTasks()
	if err != nil || len(saved) != 1 || saved[0].Upload.Issue == nil || saved[0].Upload.Issue.Code != "NETWORK_UNAVAILABLE" {
		testContext.Fatalf("上传诊断未持久化: %+v %v", saved, err)
	}
	queue.handlerResolver = func(string, string) (func(context.Context, *model.UploadingUI) error, error) {
		return func(context.Context, *model.UploadingUI) error { return nil }, nil
	}
	if err := queue.Resume(job.UploadID); err != nil {
		testContext.Fatal(err)
	}
	queue.workers.Wait()
	saved, err = storage.ListUploadTasks()
	if err != nil || len(saved) != 1 || !saved[0].Upload.IsCompleted || saved[0].Upload.Issue != nil {
		testContext.Fatalf("重试完成后残留旧诊断: %+v %v", saved, err)
	}
}
