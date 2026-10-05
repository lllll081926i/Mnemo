package transfer

import (
	"context"
	"sync"
	"testing"
	"time"

	"mnemo-go/internal/model"
	"mnemo-go/internal/store"
)

func TestUploadQueueRestoresQueuedAsPaused(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveUploadTask(&model.UploadingUI{UploadID: "queued", Upload: model.UploadState{DownState: "queued"}}); err != nil {
		t.Fatal(err)
	}
	queue := NewUploadQueue(st, nil)
	defer queue.Close()
	jobs := queue.List()
	if len(jobs) != 1 || !jobs[0].Upload.IsStop || jobs[0].Upload.DownState != "paused" {
		t.Fatalf("恢复任务显示排队但没有 worker：%+v", jobs)
	}
}

func TestUploadQueueCloseWaitsForWorker(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	queue := NewUploadQueue(st, nil)
	started, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	releaseWorker := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseWorker()
	queue.handlerResolver = func(string, string) (func(context.Context, *model.UploadingUI) error, error) {
		return func(ctx context.Context, ui *model.UploadingUI) error {
			close(started)
			<-ctx.Done()
			<-release
			return ctx.Err()
		}, nil
	}
	queue.enqueue("user", "drive", "parent", "rename", "file", "file", 3)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("worker 未启动")
	}
	closed := make(chan struct{})
	go func() { queue.Close(); close(closed) }()
	select {
	case <-closed:
		t.Fatal("Close 在 worker 收尾前返回")
	case <-time.After(50 * time.Millisecond):
	}
	releaseWorker()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("worker 已退出但 Close 未返回")
	}
	if err := queue.Resume(queue.List()[0].UploadID); err == nil {
		t.Fatal("已关闭的上传队列仍允许恢复")
	}
}

func TestUploadQueueProgressReportsSpeedAndFailure(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var event TaskEvent
	queue := NewUploadQueue(st, func(sample TaskEvent) { event = sample })
	defer queue.Close()
	queue.update(&model.UploadingUI{UploadID: "progress", UserID: "account", Info: model.UploadInfo{Size: 1000, DriveID: "drive"}, Upload: model.UploadState{IsDowning: true}})
	queue.runs["progress"] = &uploadRun{generation: 1, cancel: func() {}, done: make(chan struct{})}
	queue.speedSamples["progress"] = uploadSpeedSample{at: time.Now().Add(-time.Second), done: 0}
	queue.reportRunProgress("progress", 1, 500, 50)
	if event.Task.Speed <= 0 || event.Task.UserID != "account" || event.Task.DriveID != "drive" {
		t.Fatalf("缺少速率或账号信息：%+v", event)
	}
	queue.mutateRunJob("progress", 1, func(job *model.UploadingUI) {
		job.Upload.IsFailed = true
		job.Upload.IsDowning = false
		job.Upload.FailedMessage = "提交失败"
	})
	if event.Task.Error != "提交失败" || event.Task.Status != "failed" {
		t.Fatalf("失败事件缺少原因：%+v", event)
	}
	queue.finishUploadRun("progress", 1)
}

func TestUploadQueueOwnsSnapshotAndIgnoresStalePersistence(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	queue := NewUploadQueue(st, nil)
	defer queue.Close()
	input := &model.UploadingUI{UploadID: "snapshot", Info: model.UploadInfo{Name: "original"}, Upload: model.UploadState{DownState: "uploading", IsDowning: true}}
	queue.update(input)
	input.Info.Name = "caller-change"
	if queue.List()[0].Info.Name != "original" {
		t.Fatal("队列暴露了共享任务指针")
	}
	stale := queue.List()[0]
	queue.Cancel("snapshot")
	queue.publishSnapshot(stale, true)
	jobs, err := st.ListUploadTasks()
	if err != nil || len(jobs) != 1 || !jobs[0].Upload.IsStop {
		t.Fatalf("旧进度覆盖了暂停检查点：%+v, %v", jobs, err)
	}
}
