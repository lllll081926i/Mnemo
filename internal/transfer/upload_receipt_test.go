package transfer

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
	"mnemo-go/internal/store"
)

func newReceiptQueue(testContext *testing.T) (*UploadQueue, string, string) {
	testContext.Helper()
	directory := testContext.TempDir()
	source := filepath.Join(directory, "源文件.txt")
	if err := os.WriteFile(source, []byte("upload receipt"), 0600); err != nil {
		testContext.Fatal(err)
	}
	storage, err := store.Open(directory)
	if err != nil {
		testContext.Fatal(err)
	}
	queue := NewUploadQueue(storage, nil)
	testContext.Cleanup(queue.Close)
	return queue, source, directory
}

func enqueueReceiptUpload(queue *UploadQueue, source string) string {
	job := queue.enqueue("account", "drive", "parent", "rename", source, "源文件.txt", 14)
	queue.workers.Wait()
	return job.UploadID
}

func TestUploadReceiptPersistedBeforeProviderAndReconciledWithoutResubmit(testContext *testing.T) {
	queue, source, _ := newReceiptQueue(testContext)
	var writes atomic.Int32
	queue.handlerResolver = func(string, string) (func(context.Context, *model.UploadingUI) error, error) {
		return func(ctx context.Context, job *model.UploadingUI) error {
			if err := job.BeginUploadAttempt(); err != nil {
				return err
			}
			saved, err := queue.store.ListUploadTasks()
			if err != nil || len(saved) != 1 || saved[0].Upload.Attempt.State != "pending" {
				testContext.Errorf("发送前缺少持久提交记录: %+v, %v", saved, err)
			}
			writes.Add(1)
			job.Upload.FileID = "remote-file"
			return io.ErrUnexpectedEOF
		}, nil
	}
	id := enqueueReceiptUpload(queue, source)
	if queue.List()[0].Upload.Attempt.State != "unknown" || queue.List()[0].Upload.Issue.Code != "UPLOAD_RESULT_UNKNOWN" {
		testContext.Fatalf("丢失回执没有阻止重发: %+v", queue.List())
	}
	hash, err := netx.HashFile(source, netx.HashSHA256)
	if err != nil {
		testContext.Fatal(err)
	}
	queue.verifyFile = func(ctx context.Context, user, drive, file string) (*model.File, error) {
		if user != "account" || drive != "drive" || file != "remote-file" {
			testContext.Errorf("复核作用域错误: %q %q %q", user, drive, file)
		}
		return &model.File{FileID: file, DriveID: drive, ParentFileID: "parent", Name: "源文件.txt", Size: 14, ContentHash: hash, ContentHashName: "SHA-256"}, nil
	}
	if err := queue.Resume(id); err != nil {
		testContext.Fatal(err)
	}
	if writes.Load() != 1 || !queue.List()[0].Upload.IsCompleted {
		testContext.Fatalf("复核重新提交了文件: writes=%d, %+v", writes.Load(), queue.List())
	}
	saved, err := queue.store.ListUploadTasks()
	if err != nil || saved[0].Upload.Attempt.State != "confirmed" || !saved[0].Upload.IsCompleted {
		testContext.Fatalf("复核没有持久化: %+v, %v", saved, err)
	}
}

func TestUnknownUploadRequiresExplicitReview(testContext *testing.T) {
	queue, source, _ := newReceiptQueue(testContext)
	var writes atomic.Int32
	queue.handlerResolver = func(string, string) (func(context.Context, *model.UploadingUI) error, error) {
		return func(ctx context.Context, job *model.UploadingUI) error {
			if err := job.BeginUploadAttempt(); err != nil {
				return err
			}
			if writes.Add(1) == 1 {
				return io.ErrUnexpectedEOF
			}
			return nil
		}, nil
	}
	id := enqueueReceiptUpload(queue, source)
	if err := queue.Resume(id); !errors.Is(err, errUploadReviewRequired) || writes.Load() != 1 {
		testContext.Fatalf("未知回执被直接重传: %v, writes=%d", err, writes.Load())
	}
	if err := queue.ResumeReviewed(id); err != nil {
		testContext.Fatal(err)
	}
	queue.workers.Wait()
	if writes.Load() != 2 || !queue.List()[0].Upload.IsCompleted {
		testContext.Fatalf("明确核对后未恢复: %+v, writes=%d", queue.List(), writes.Load())
	}
}

func TestUploadReconciliationRejectsWeakOrMismatchedEvidence(testContext *testing.T) {
	for _, mismatch := range []string{"hash-missing", "hash-wrong", "parent", "drive", "identity", "name", "size", "source-changed"} {
		testContext.Run(mismatch, func(testContext *testing.T) {
			queue, source, _ := newReceiptQueue(testContext)
			queue.handlerResolver = func(string, string) (func(context.Context, *model.UploadingUI) error, error) {
				return func(ctx context.Context, job *model.UploadingUI) error {
					if err := job.BeginUploadAttempt(); err != nil {
						return err
					}
					job.Upload.FileID = "remote"
					return io.ErrUnexpectedEOF
				}, nil
			}
			id := enqueueReceiptUpload(queue, source)
			hash, err := netx.HashFile(source, netx.HashSHA256)
			if err != nil {
				testContext.Fatal(err)
			}
			file := model.File{FileID: "remote", DriveID: "drive", ParentFileID: "parent", Name: "源文件.txt", Size: 14, ContentHash: hash, ContentHashName: "sha256"}
			switch mismatch {
			case "hash-missing":
				file.ContentHash = ""
			case "hash-wrong":
				file.ContentHash = "wrong"
			case "parent":
				file.ParentFileID = "other-parent"
			case "drive":
				file.DriveID = "other-drive"
			case "identity":
				file.FileID = "other-id"
			case "name":
				file.Name = "other-name"
			case "size":
				file.Size++
			case "source-changed":
				if err := os.Chtimes(source, time.Now(), time.Now().Add(time.Hour)); err != nil {
					testContext.Fatal(err)
				}
			}
			queue.verifyFile = func(context.Context, string, string, string) (*model.File, error) { return &file, nil }
			if err := queue.Resume(id); err == nil || queue.List()[0].Upload.IsCompleted {
				testContext.Fatalf("不可靠证据被当成成功: %+v, %v", queue.List(), err)
			}
		})
	}
}

func TestUploadReceiptFailurePreventsRemoteRequest(testContext *testing.T) {
	queue, source, directory := newReceiptQueue(testContext)
	if err := os.Mkdir(filepath.Join(directory, "uploads.json"), 0700); err != nil {
		testContext.Fatal(err)
	}
	var writes atomic.Int32
	queue.handlerResolver = func(string, string) (func(context.Context, *model.UploadingUI) error, error) {
		return func(ctx context.Context, job *model.UploadingUI) error {
			if err := job.BeginUploadAttempt(); err != nil {
				return err
			}
			writes.Add(1)
			return nil
		}, nil
	}
	enqueueReceiptUpload(queue, source)
	if writes.Load() != 0 || !queue.List()[0].Upload.IsFailed {
		testContext.Fatalf("本地记录失败仍然发送了文件: %+v", queue.List())
	}
}

func TestUploadReceiptSurvivesRestartAndDisabledHistory(testContext *testing.T) {
	queue, source, _ := newReceiptQueue(testContext)
	settings := store.DefaultSettings()
	settings.KeepTasks = false
	if err := queue.store.SetSettings(settings); err != nil {
		testContext.Fatal(err)
	}
	info, err := os.Stat(source)
	if err != nil {
		testContext.Fatal(err)
	}
	for _, state := range []string{"pending", "unknown", "confirmed"} {
		job := model.UploadingUI{UploadID: state, Info: model.UploadInfo{LocalFilePath: source, Size: 14}, Upload: model.UploadState{IsFailed: true, Attempt: model.UploadAttempt{State: state, Size: 14, ModTime: info.ModTime().UnixNano()}}}
		if err := queue.store.SaveUploadTask(&job); err != nil {
			testContext.Fatal(err)
		}
	}
	restored := NewUploadQueue(queue.store, nil)
	defer restored.Close()
	jobs := restored.List()
	if len(jobs) != 2 {
		testContext.Fatalf("关闭历史时丢失待核对记录: %+v", jobs)
	}
	for _, job := range jobs {
		if job.Upload.Attempt.State != "unknown" || job.Upload.Issue.Code != "UPLOAD_RESULT_UNKNOWN" {
			testContext.Fatalf("重启后没有保留防重: %+v", job)
		}
	}
}

func TestResolvedUploadParentIsNotCreatedAgainOnResume(testContext *testing.T) {
	queue, source, _ := newReceiptQueue(testContext)
	queue.handlerResolver = func(string, string) (func(context.Context, *model.UploadingUI) error, error) {
		return func(ctx context.Context, job *model.UploadingUI) error {
			if job.Info.ParentFileID != "resolved-folder" || job.Info.Name != "源文件.txt" {
				testContext.Errorf("恢复后目录重复嵌套: %+v", job.Info)
			}
			return job.BeginUploadAttempt()
		}, nil
	}
	job := &model.UploadingUI{UploadID: "resolved", UserID: "account", Info: model.UploadInfo{LocalFilePath: source, DriveID: "drive", ParentFileID: "resolved-folder", ParentResolved: true, Path: "folder/源文件.txt", Size: 14}, Upload: model.UploadState{IsStop: true}}
	queue.update(job)
	if err := queue.Resume(job.UploadID); err != nil {
		testContext.Fatal(err)
	}
	queue.workers.Wait()
	if !queue.List()[0].Upload.IsCompleted {
		testContext.Fatalf("已解析目录被重复创建: %+v", queue.List())
	}
}

func TestUploadFinalReceiptFailureKeepsSourceAndBlocksRetry(testContext *testing.T) {
	queue, source, directory := newReceiptQueue(testContext)
	path := filepath.Join(directory, "uploads.json")
	backup := filepath.Join(directory, "uploads.saved")
	queue.handlerResolver = func(string, string) (func(context.Context, *model.UploadingUI) error, error) {
		return func(ctx context.Context, job *model.UploadingUI) error {
			if err := job.BeginUploadAttempt(); err != nil {
				return err
			}
			job.Info.CleanupLocalFile = true
			job.Upload.FileID = "remote"
			if err := os.Rename(path, backup); err != nil {
				return err
			}
			return os.Mkdir(path, 0700)
		}, nil
	}
	id := enqueueReceiptUpload(queue, source)
	if queue.List()[0].Upload.IsCompleted || queue.List()[0].Upload.Attempt.State != "unknown" {
		testContext.Fatalf("未保存回执仍被标成完成: %+v", queue.List())
	}
	if _, err := os.Stat(source); err != nil {
		testContext.Fatalf("未保存回执时删除了源文件: %v", err)
	}
	if err := os.Remove(path); err != nil {
		testContext.Fatal(err)
	}
	if err := os.Rename(backup, path); err != nil {
		testContext.Fatal(err)
	}
	queue.verifyFile = func(context.Context, string, string, string) (*model.File, error) {
		return nil, errors.New("remote unavailable")
	}
	if err := queue.Resume(id); err == nil {
		testContext.Fatal("回执保存失败后直接重传")
	}
}

func TestConfirmedUploadReceiptCompletesAfterRestart(testContext *testing.T) {
	queue, _, _ := newReceiptQueue(testContext)
	job := model.UploadingUI{UploadID: "confirmed", Info: model.UploadInfo{Size: 14}, Upload: model.UploadState{IsDowning: true, Attempt: model.UploadAttempt{State: "confirmed"}}}
	if err := queue.store.SaveUploadTask(&job); err != nil {
		testContext.Fatal(err)
	}
	restored := NewUploadQueue(queue.store, nil)
	defer restored.Close()
	if len(restored.List()) != 1 || !restored.List()[0].Upload.IsCompleted || restored.List()[0].Upload.DownSize != 14 {
		testContext.Fatalf("已确认记录在重启后仍可重复提交: %+v", restored.List())
	}
}
