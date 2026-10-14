package transfer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"mnemo-go/internal/model"
	"mnemo-go/internal/store"
)

func TestPR1ReviewClearCompletedKeepsActiveRecordConsistent(testContext *testing.T) {
	storage, err := store.Open(testContext.TempDir())
	if err != nil {
		testContext.Fatal(err)
	}
	queue := NewUploadQueue(storage, nil)
	defer queue.Close()
	queue.update(&model.UploadingUI{UploadID: "finishing", Upload: model.UploadState{IsCompleted: true}})
	_, cancel := context.WithCancel(context.Background())
	queue.runs["finishing"] = &uploadRun{cancel: cancel}
	if err := queue.ClearCompleted(); err != nil {
		testContext.Fatal(err)
	}
	saved, err := storage.ListUploadTasks()
	if err != nil {
		testContext.Fatal(err)
	}
	if len(saved) != len(queue.List()) {
		testContext.Fatalf("清理时内存保留 %d 项，持久化保留 %d 项", len(queue.List()), len(saved))
	}
}

func TestClearCompletedPreservesMemoryOnPersistenceFailure(testContext *testing.T) {
	storage, err := store.Open(testContext.TempDir())
	if err != nil {
		testContext.Fatal(err)
	}
	queue := NewUploadQueue(storage, nil)
	defer queue.Close()
	queue.update(&model.UploadingUI{UploadID: "done", Upload: model.UploadState{IsCompleted: true}})
	path := filepath.Join(storage.Dir(), "uploads.json")
	if err := os.Rename(path, path+".backup"); err != nil {
		testContext.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		testContext.Fatal(err)
	}
	if err := queue.ClearCompleted(); err == nil {
		testContext.Fatal("持久化失败必须返回错误")
	}
	if len(queue.List()) != 1 {
		testContext.Fatal("持久化失败不得删除内存记录")
	}
}

func TestClearCompletedOnlyRemovesInactiveSuccess(testContext *testing.T) {
	storage, err := store.Open(testContext.TempDir())
	if err != nil {
		testContext.Fatal(err)
	}
	queue := NewUploadQueue(storage, nil)
	defer queue.Close()
	jobs := []model.UploadingUI{
		{UploadID: "done", Upload: model.UploadState{IsCompleted: true}},
		{UploadID: "finishing", Upload: model.UploadState{IsCompleted: true}},
		{UploadID: "failed", Upload: model.UploadState{IsFailed: true}},
		{UploadID: "paused", Upload: model.UploadState{IsStop: true}},
	}
	for _, job := range jobs {
		queue.update(&job)
	}
	_, cancel := context.WithCancel(context.Background())
	queue.runs["finishing"] = &uploadRun{cancel: cancel}
	if err := queue.ClearCompleted(); err != nil {
		testContext.Fatal(err)
	}
	saved, err := storage.ListUploadTasks()
	if err != nil || len(saved) != 3 || len(queue.List()) != 3 {
		testContext.Fatalf("清理结果不一致: %v, %v", saved, err)
	}
	for _, job := range saved {
		if job.UploadID == "done" {
			testContext.Fatal("已完成且退出的任务未清理")
		}
	}
}
