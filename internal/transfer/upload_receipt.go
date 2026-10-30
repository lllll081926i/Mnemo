package transfer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"mnemo-go/internal/drive/driveutil"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

var errUploadReviewRequired = errors.New("上传结果尚未确认，请先核对目标目录；确认需要重新上传后，再选择核对后重试")

func uploadReviewIssue() *model.OperationIssue {
	return &model.OperationIssue{Code: "UPLOAD_RESULT_UNKNOWN", Message: "网盘可能已接收文件，不能直接重复上传", Action: "check_target"}
}

func validateUploadSource(job model.UploadingUI, attempt model.UploadAttempt) error {
	info, err := os.Lstat(job.Info.LocalFilePath)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() != job.Info.Size {
		return errors.New("上传源文件类型或大小已变化，请重新添加任务")
	}
	if attempt.State != "" && (info.Size() != attempt.Size || info.ModTime().UnixNano() != attempt.ModTime) {
		return errors.New("上传源文件已变化，不能复用原提交记录，请重新添加任务")
	}
	return nil
}

func (q *UploadQueue) beginUploadAttempt(ctx context.Context, id string, generation uint64, worker *model.UploadingUI) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateUploadSource(*worker, model.UploadAttempt{}); err != nil {
		return err
	}
	info, err := os.Lstat(worker.Info.LocalFilePath)
	if err != nil {
		return err
	}
	attempt := model.UploadAttempt{State: "pending", StartedAt: time.Now().UnixMilli(), Size: info.Size(), ModTime: info.ModTime().UnixNano(), ParentID: worker.Info.ParentFileID, Name: worker.Info.Name}
	q.persistMu.Lock()
	defer q.persistMu.Unlock()
	q.mu.Lock()
	defer q.mu.Unlock()
	job, run := q.jobs[id], q.runs[id]
	if job == nil || run == nil || run.generation != generation || ctx.Err() != nil {
		return context.Canceled
	}
	if job.Upload.Attempt.State != "" {
		return errUploadReviewRequired
	}
	snapshot := *job
	snapshot.Upload.Attempt = attempt
	if err := q.store.SaveUploadTask(&snapshot); err != nil {
		return fmt.Errorf("无法保存上传提交记录，尚未发送文件: %w", err)
	}
	job.Upload.Attempt = attempt
	worker.Upload.Attempt = attempt
	return nil
}

func (q *UploadQueue) finishUploadAttempt(id string, generation uint64, worker *model.UploadingUI, uploadErr error) error {
	q.persistMu.Lock()
	defer q.persistMu.Unlock()
	q.mu.Lock()
	defer q.mu.Unlock()
	job, run := q.jobs[id], q.runs[id]
	if job == nil || run == nil || run.generation != generation {
		return errUploadWorkerStopping
	}
	attempt := worker.Upload.Attempt
	attempt.State = "unknown"
	attempt.FileID = worker.Upload.FileID
	attempt.Name = worker.Info.Name
	if uploadErr == nil {
		attempt.State = "confirmed"
	}
	snapshot := *job
	snapshot.Upload.Attempt = attempt
	snapshot.Upload.FileID = worker.Upload.FileID
	snapshot.Upload.UploadID = worker.Upload.UploadID
	if err := q.store.SaveUploadTask(&snapshot); err != nil {
		attempt.State = "unknown"
		job.Upload.Attempt = attempt
		worker.Upload.Attempt = attempt
		return fmt.Errorf("网盘调用已结束，但上传回执保存失败，需核对目标目录: %w", err)
	}
	job.Upload.Attempt = attempt
	worker.Upload.Attempt = attempt
	return nil
}

func completeReconciledUpload(job *model.UploadingUI) {
	job.Upload.Attempt.State = "confirmed"
	job.Upload.IsDowning = false
	job.Upload.IsStop = false
	job.Upload.IsFailed = false
	job.Upload.IsCompleted = true
	job.Upload.DownState = "completed"
	job.Upload.DownSize = job.Info.Size
	job.Upload.DownProcess = 100
	job.Upload.DownSpeed = 0
	job.Upload.DownSpeedStr = ""
	job.Upload.Stage = ""
	job.Upload.StageProgress = 0
	job.Upload.FailedCode = 0
	job.Upload.FailedMessage = ""
	job.Upload.Issue = nil
}

func (q *UploadQueue) reconcileUpload(snapshot model.UploadingUI) error {
	if snapshot.Upload.Attempt.State != "confirmed" {
		if err := validateUploadSource(snapshot, snapshot.Upload.Attempt); err != nil {
			return err
		}
		attempt := snapshot.Upload.Attempt
		if attempt.FileID == "" {
			return errUploadReviewRequired
		}
		ctx, cancel := context.WithTimeout(q.ctx, 45*time.Second)
		defer cancel()
		file, err := q.verifyFile(ctx, snapshot.UserID, snapshot.Info.DriveID, attempt.FileID)
		if err != nil {
			return fmt.Errorf("目标文件复核失败，未重新上传: %w", err)
		}
		if file == nil || file.FileID != attempt.FileID || file.IsDir || file.Size != attempt.Size || file.Name != attempt.Name || file.ParentFileID != "" && file.ParentFileID != attempt.ParentID || file.DriveID != "" && file.DriveID != snapshot.Info.DriveID {
			return errUploadReviewRequired
		}
		kind := netx.HashKind(strings.ToLower(strings.ReplaceAll(file.ContentHashName, "-", "")))
		if file.ContentHash == "" || (kind != netx.HashMD5 && kind != netx.HashSHA1 && kind != netx.HashSHA256 && kind != "gcid") {
			return errUploadReviewRequired
		}
		var hash string
		if kind == "gcid" {
			hash, err = driveutil.ComputeGCID(ctx, snapshot.Info.LocalFilePath, nil)
		} else {
			hash, err = netx.HashFileWithProgressContext(ctx, snapshot.Info.LocalFilePath, kind, nil, attempt.Size)
		}
		if err != nil {
			return err
		}
		if err := validateUploadSource(snapshot, attempt); err != nil {
			return err
		}
		if !strings.EqualFold(hash, file.ContentHash) {
			return errUploadReviewRequired
		}
	}
	q.persistMu.Lock()
	q.mu.Lock()
	job := q.jobs[snapshot.UploadID]
	if job == nil || job.Upload.Attempt != snapshot.Upload.Attempt {
		q.mu.Unlock()
		q.persistMu.Unlock()
		return errUploadWorkerStopping
	}
	snapshot = *job
	completeReconciledUpload(&snapshot)
	if err := q.store.SaveUploadTask(&snapshot); err != nil {
		q.mu.Unlock()
		q.persistMu.Unlock()
		return fmt.Errorf("目标已核实，完成记录保存失败，未重新上传: %w", err)
	}
	*job = snapshot
	q.mu.Unlock()
	q.persistMu.Unlock()
	q.publishSnapshot(snapshot, false)
	q.cleanupTemporarySource(&snapshot)
	return nil
}

func (q *UploadQueue) resetReviewedUpload(snapshot model.UploadingUI) error {
	if err := validateUploadSource(snapshot, snapshot.Upload.Attempt); err != nil {
		return err
	}
	if snapshot.Upload.Attempt.State == "confirmed" {
		return errors.New("上传已确认完成，不允许再次提交")
	}
	q.persistMu.Lock()
	defer q.persistMu.Unlock()
	q.mu.Lock()
	defer q.mu.Unlock()
	job := q.jobs[snapshot.UploadID]
	if job == nil || job.Upload.Attempt != snapshot.Upload.Attempt || q.ctx.Err() != nil {
		return errUploadWorkerStopping
	}
	snapshot = *job
	snapshot.Upload.Attempt = model.UploadAttempt{}
	if err := q.store.SaveUploadTask(&snapshot); err != nil {
		return err
	}
	*job = snapshot
	return nil
}
