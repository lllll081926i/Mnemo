// Package migrate implements cross-drive file migration: copy/move files
// between two accounts by streaming (source download → target upload).
package migrate

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"

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
	// Byte-level progress (accumulated across files).
	TotalBytes     int64 `json:"totalBytes"`
	ProcessedBytes int64 `json:"processedBytes"`
	Status         string `json:"status"`
	Message        string `json:"message,omitempty"`
}

// OnProgress is invoked per file.
type OnProgress func(j *Job)

// Engine runs migration jobs.
type Engine struct {
	onProgress OnProgress
	// cancels stores per-job cancel funcs so Cancel(jobID) can abort a Run.
	cancelsMu sync.Mutex
	cancels   map[string]context.CancelFunc
}

// NewEngine creates the migration engine.
func NewEngine(onProgress OnProgress) *Engine {
	return &Engine{
		onProgress: onProgress,
		cancels:    make(map[string]context.CancelFunc),
	}
}

// Cancel cancels the migration job with the given id.
// It is safe to call even if the job is unknown or already finished;
// in those cases it is a no-op.
func (e *Engine) Cancel(jobID string) {
	e.cancelsMu.Lock()
	defer e.cancelsMu.Unlock()
	if cancel, ok := e.cancels[jobID]; ok {
		cancel()
		delete(e.cancels, jobID)
	}
}

// registerCancel stores the cancel func for a job and returns a derived
// context that is cancelled by Cancel(jobID) or parent cancellation.
func (e *Engine) registerCancel(parent context.Context, jobID string) context.Context {
	ctx, cancel := context.WithCancel(parent)
	e.cancelsMu.Lock()
	e.cancels[jobID] = cancel
	e.cancelsMu.Unlock()
	return ctx
}

// releaseCancel drops the cancel entry for a finished job.
func (e *Engine) releaseCancel(jobID string) {
	e.cancelsMu.Lock()
	delete(e.cancels, jobID)
	e.cancelsMu.Unlock()
}

// Run migrates the given files. Each file is streamed through a temp file
// (or directly piped when the target provider supports streaming — see
// tryStreamMigrate).
func (e *Engine) Run(ctx context.Context, job *Job) error {
	ctx = e.registerCancel(ctx, job.ID)
	defer e.releaseCancel(job.ID)

	job.Total = int64(len(job.FileIDs))
	job.Status = "running"
	e.emit(job)
	partial := false
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
			if isPartialError(err) {
				partial = true
			}
		}
		e.emit(job)
	}
	if job.Status != "canceled" {
		if partial {
			job.Status = "partial"
		} else {
			job.Status = "completed"
		}
	}
	e.emit(job)
	return nil
}

// migrateOne migrates a single file. It first attempts a streaming pipe
// (source download Reader → target upload Writer); if the target provider
// does not support streaming or the stream attempt fails, it falls back
// to the spool (temp-file) path.
func (e *Engine) migrateOne(ctx context.Context, job *Job, fileID string) error {
	// resolve source file
	srcFile, err := drive.GetFile(job.SrcUser, job.SrcDrive, fileID)
	if err != nil {
		return err
	}
	if srcFile.IsDir {
		return e.migrateDir(ctx, job, srcFile)
	}
	// accumulate total bytes for progress
	job.TotalBytes += srcFile.Size
	defer func() { e.emit(job) }()

	// Attempt streaming migration first; fall back to spool on failure.
	if migrated, err := e.tryStreamMigrate(ctx, job, srcFile); migrated {
		if err != nil {
			// streaming failed — fall back to spool.
			return e.spoolMigrate(ctx, job, srcFile)
		}
		// streaming succeeded; handle move cleanup.
		return e.finalizeMove(ctx, job, srcFile)
	}
	// streaming not supported by this provider — use spool.
	return e.spoolMigrate(ctx, job, srcFile)
}

// tryStreamMigrate attempts to pipe the source download stream directly
// into the target upload via io.Pipe, avoiding a local temp file.
//
// It returns (supported, error):
//   - supported=false when the target provider does not expose a streaming
//     upload hook (currently no provider implements StreamUpload), in which
//     case the caller falls back to spoolMigrate.
//   - supported=true with a non-nil error when streaming was attempted but
//     failed; the caller may still fall back to spoolMigrate.
//
// TODO: once providers expose a StreamUpload(ctx, reader, info) capability,
// wire it here via drive.QueueStreamUploadHandler. Until then streaming is
// not available and this always returns (false, nil).
func (e *Engine) tryStreamMigrate(ctx context.Context, job *Job, srcFile *model.File) (bool, error) {
	// No provider currently implements a streaming-upload entry point that
	// accepts an io.Reader. The upload contract (UploadOneFile) requires a
	// LocalFilePath on disk, so we cannot pipe directly. Returning
	// supported=false makes the engine fall back to the spool path.
	//
	// When a streaming upload API is added, the implementation would look like:
	//
	//   dl, err := drive.GetDownloadURL(job.SrcUser, job.SrcDrive, srcFile.FileID, 3600)
	//   if err != nil { return true, err }
	//   pr, pw := io.Pipe()
	//   go func() {
	//       defer pw.Close()
	//       downloadTo(ctx, dl, pw)
	//   }()
	//   if err := streamUpload(ctx, pr, srcFile); err != nil {
	//       return true, err
	//   }
	//   job.ProcessedBytes += srcFile.Size
	//   return true, nil
	_ = ctx
	_ = job
	_ = srcFile
	return false, nil
}

// spoolMigrate downloads the source to a temp file then uploads it to the
// target. This is the legacy/fallback path.
func (e *Engine) spoolMigrate(ctx context.Context, job *Job, srcFile *model.File) error {
	// resolve download url
	dl, err := drive.GetDownloadURL(job.SrcUser, job.SrcDrive, srcFile.FileID, 3600)
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
	if err := downloadToCounted(ctx, dl, tmp, job); err != nil {
		return err
	}
	_ = tmp.Sync()
	// upload to target
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return err
	}
	ui := &model.UploadingUI{
		UploadID: srcFile.FileID,
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
	return e.finalizeMove(ctx, job, srcFile)
}

// finalizeMove deletes the source file in move mode. If deletion fails it
// returns a partialError so the caller records a partial (not completed)
// status and surfaces a message — the migration is effectively a copy in
// that case.
func (e *Engine) finalizeMove(ctx context.Context, job *Job, srcFile *model.File) error {
	job.ProcessedBytes += srcFile.Size
	if !job.Move {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	isDir := srcFile.IsDir
	if _, err := drive.DeleteBatch(job.SrcUser, job.SrcDrive, []drive.FileRef{{ID: srcFile.FileID, IsDir: &isDir}}); err != nil {
		msg := fmt.Sprintf("move: source upload succeeded but source delete failed for %q: %v", srcFile.Name, err)
		return partialError(msg)
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
	dirPartial := false
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
				if isPartialError(err) {
					dirPartial = true
				}
			}
		} else {
			if err := e.migrateOne(ctx, &subJob, child.FileID); err != nil {
				job.Failed++
				if isPartialError(err) {
					dirPartial = true
				}
			}
			job.Processed++
			_ = subParent
			e.emit(job)
		}
	}
	if job.Move {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		isDir := true
		if _, err := drive.DeleteBatch(job.SrcUser, job.SrcDrive, []drive.FileRef{{ID: dir.FileID, IsDir: &isDir}}); err != nil {
			msg := fmt.Sprintf("move: source folder migrated but delete failed for %q: %v", dir.Name, err)
			if dirPartial {
				return partialError(msg)
			}
			return partialError(msg)
		}
	}
	if dirPartial {
		return partialError("move: one or more source items could not be deleted after migration")
	}
	return nil
}

func (e *Engine) emit(job *Job) {
	if e.onProgress != nil {
		e.onProgress(job)
	}
}

func boolPtr(v bool) *bool { return &v }

// partialError marks an error whose cause is a post-migration source-delete
// failure. The file was copied successfully but the move is incomplete.
type partialError string

func (p partialError) Error() string { return string(p) }

func isPartialError(err error) bool {
	_, ok := err.(partialError)
	return ok
}

// downloadTo streams a download url into a writer.
func downloadTo(ctx context.Context, dl *model.DownloadURL, w io.Writer) error {
	return downloadToCounted(ctx, dl, w, nil)
}

// downloadToCounted is like downloadTo but accumulates the byte count into
// job.ProcessedBytes when job is non-nil.
func downloadToCounted(ctx context.Context, dl *model.DownloadURL, w io.Writer, job *Job) error {
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
	var counter io.Writer = w
	if job != nil {
		counter = &countingWriter{w: w, job: job, emit: func() {}}
	}
	_, err = io.Copy(counter, resp.Body)
	return err
}

// countingWriter wraps a writer and increments job.ProcessedBytes.
type countingWriter struct {
	w   io.Writer
	job *Job
	emit func()
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.job.ProcessedBytes += int64(n)
	return n, err
}
