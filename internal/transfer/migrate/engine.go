// Package migrate implements cross-drive file migration: copy/move files
// between two accounts by streaming (source download → target upload).
package migrate

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

// Job is one migration request.
type Job struct {
	ID       string   `json:"id"`
	SrcUser  string   `json:"srcUser"`
	SrcDrive string   `json:"srcDrive"`
	FileIDs  []string `json:"fileIDs"`
	DstUser  string   `json:"dstUser"`
	DstDrive string   `json:"dstDrive"`
	DstParent string  `json:"dstParent"`
	Move     bool     `json:"move"`
	// Live progress
	Total     int64 `json:"total"`
	Processed int64 `json:"processed"`
	Failed    int64 `json:"failed"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
}

// OnProgress is invoked per file.
type OnProgress func(j *Job)

// Engine runs migration jobs.
type Engine struct {
	onProgress OnProgress
}

// NewEngine creates the migration engine.
func NewEngine(onProgress OnProgress) *Engine {
	return &Engine{onProgress: onProgress}
}

// Run migrates the given files. Each file is streamed through a temp file.
func (e *Engine) Run(ctx context.Context, job *Job) error {
	job.Status = "running"
	e.emit(job)
	for _, fileID := range job.FileIDs {
		if ctx.Err() != nil {
			job.Status = "canceled"
			e.emit(job)
			return ctx.Err()
		}
		job.Processed++
		err := e.migrateOne(ctx, job, fileID)
		if err != nil {
			job.Failed++
			job.Message = err.Error()
		}
		e.emit(job)
	}
	if job.Status != "canceled" {
		job.Status = "completed"
	}
	e.emit(job)
	return nil
}

func (e *Engine) migrateOne(ctx context.Context, job *Job, fileID string) error {
	// resolve source file
	srcFile, err := drive.GetFile(job.SrcUser, job.SrcDrive, fileID)
	if err != nil {
		return err
	}
	if srcFile.IsDir {
		return e.migrateDir(ctx, job, srcFile)
	}
	// resolve download url
	dl, err := drive.GetDownloadURL(job.SrcUser, job.SrcDrive, fileID, 3600)
	if err != nil {
		return err
	}
	// stream to temp
	tmp, err := os.CreateTemp("", "mnemo-migrate-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if err := downloadTo(ctx, dl, tmp); err != nil {
		return err
	}
	_ = tmp.Sync()
	// upload to target
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return err
	}
	ui := &model.UploadingUI{
		UploadID: fileID,
		Info: model.UploadInfo{
			LocalFilePath: tmp.Name(), ParentFileID: job.DstParent,
			DriveID: job.DstDrive, Name: srcFile.Name, Size: srcFile.Size,
		},
	}
	handler, err := drive.QueueUploadHandler(job.DstUser, job.DstDrive)
	if err != nil {
		return err
	}
	if err := handler(ctx, ui); err != nil {
		return err
	}
	// optionally remove source
	if job.Move {
		_, _ = drive.DeleteBatch(job.SrcUser, job.SrcDrive, []drive.FileRef{{ID: fileID}})
	}
	return nil
}

// migrateDir recursively migrates a folder.
func (e *Engine) migrateDir(ctx context.Context, job *Job, dir *model.File) error {
	// create folder on target
	mk, err := drive.Mkdir(job.DstUser, job.DstDrive, job.DstParent, dir.Name)
	if err != nil {
		return err
	}
	subParent := dir.FileID
	targetParent := job.DstParent
	if mk != nil && mk.FileID != "" {
		targetParent = mk.FileID
	}
	// list source dir
	children, err := drive.ListDir(job.SrcUser, job.SrcDrive, dir.FileID, nil)
	if err != nil {
		return err
	}
	for i := range children {
		child := children[i]
		if ctx.Err() != nil {
			return ctx.Err()
		}
		subJob := *job
		subJob.DstParent = targetParent
		if child.IsDir {
			if err := e.migrateDir(ctx, &subJob, &child); err != nil {
				job.Failed++
			}
		} else {
			if err := e.migrateOne(ctx, &subJob, child.FileID); err != nil {
				job.Failed++
			}
			job.Processed++
			_ = subParent
			e.emit(job)
		}
	}
	if job.Move {
		_, _ = drive.DeleteBatch(job.SrcUser, job.SrcDrive, []drive.FileRef{{ID: dir.FileID, IsDir: boolPtr(true)}})
	}
	return nil
}

func (e *Engine) emit(job *Job) {
	if e.onProgress != nil {
		e.onProgress(job)
	}
}

func boolPtr(v bool) *bool { return &v }

// downloadTo streams a download url into a writer.
func downloadTo(ctx context.Context, dl *model.DownloadURL, w io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dl.URL, nil)
	if err != nil {
		return err
	}
	for k, v := range dl.Headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("migrate: http %d", resp.StatusCode)
	}
	_, err = io.Copy(w, resp.Body)
	return err
}
