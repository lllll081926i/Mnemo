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
	"mnemo-go/internal/netx"
	"mnemo-go/internal/store"
)

// Job is a type alias for model.MigrateJob so existing callers (app layer,
// frontend events) continue to reference migrate.Job.
type Job = model.MigrateJob

// OnProgress is invoked per file.
type OnProgress func(j *Job)

// Engine runs migration jobs.
type Engine struct {
	store      *store.Store
	onProgress OnProgress
	// cancels stores per-job cancel funcs so Cancel(jobID) can abort a Run.
	cancelsMu sync.Mutex
	cancels   map[string]context.CancelFunc
}

// NewEngine creates the migration engine. store may be nil (disables
// persistence and startup recovery).
func NewEngine(st *store.Store, onProgress OnProgress) *Engine {
	return &Engine{
		store:      st,
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

// saveJob persists the job to the store (if configured).
func (e *Engine) saveJob(job *Job) {
	if e.store == nil {
		return
	}
	_ = e.store.SaveMigrateJob(job)
}

// RecoverUnfinished marks any persisted jobs that were still running/pending
// at last shutdown as canceled. This aligns with the legacy behavior of not
// silently resuming interrupted migrations. Should be called once at startup.
func (e *Engine) RecoverUnfinished() {
	if e.store == nil {
		return
	}
	jobs, err := e.store.ListMigrateJobs()
	if err != nil {
		return
	}
	for i := range jobs {
		j := &jobs[i]
		switch j.Status {
		case "running", "pending", "":
			j.Status = "canceled"
			j.Message = "recovered: job was interrupted; marked canceled on startup"
			_ = e.store.SaveMigrateJob(j)
			e.emit(j)
		}
	}
}

// Run migrates the given files. Each file is streamed through a temp file
// (or directly piped when the target provider supports streaming — see
// tryStreamMigrate).
func (e *Engine) Run(ctx context.Context, job *Job) error {
	ctx = e.registerCancel(ctx, job.ID)
	defer e.releaseCancel(job.ID)

	job.Total = int64(len(job.FileIDs))
	job.Status = "running"
	e.saveJob(job)
	e.emit(job)
	partial := false
	for _, fileID := range job.FileIDs {
		if ctx.Err() != nil {
			job.Status = "canceled"
			e.saveJob(job)
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
		e.saveJob(job)
		e.emit(job)
	}
	if job.Status != "canceled" {
		if partial {
			job.Status = "partial"
		} else {
			job.Status = "completed"
		}
	}
	e.saveJob(job)
	e.emit(job)
	return nil
}

// migrateOne migrates a single file. It attempts strategies in order:
//  1. Rapid upload (hash-based秒传) if both source and target share a hash.
//  2. Stream pipe (source download Reader → target StreamUploader).
//  3. Spool (temp-file) fallback.
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

	// 1) Try rapid upload (秒传).
	if migrated, err := e.tryRapidMigrate(ctx, job, srcFile); migrated {
		if err == nil {
			return e.finalizeMove(ctx, job, srcFile)
		}
		// rapid upload failed — fall through to stream/spool.
	}

	// 2) Attempt streaming migration; fall back to spool on failure.
	if migrated, err := e.tryStreamMigrate(ctx, job, srcFile); migrated {
		if err != nil {
			// streaming failed — fall back to spool.
			return e.spoolMigrate(ctx, job, srcFile)
		}
		// streaming succeeded; handle move cleanup.
		return e.finalizeMove(ctx, job, srcFile)
	}
	// 3) streaming not supported by this provider — use spool.
	return e.spoolMigrate(ctx, job, srcFile)
}

// tryRapidMigrate attempts a hash-based秒传: if both the source provider can
// provide a hash and the target provider supports rapid upload with the same
// hash method, it resolves the source hash and calls RapidUploadByHash on the
// target.
//
// Returns (attempted, error):
//   - attempted=false when no common hash method exists (caller falls through).
//   - attempted=true with nil error on success.
//   - attempted=true with non-nil error when秒传 was tried but failed (caller
//     falls through to stream/spool).
func (e *Engine) tryRapidMigrate(ctx context.Context, job *Job, srcFile *model.File) (bool, error) {
	srcProvider := drive.ResolveProvider(job.SrcUser, job.SrcDrive, "")
	dstProvider := drive.ResolveProvider(job.DstUser, job.DstDrive, "")

	srcCaps := drive.RegistryCaps(srcProvider)
	dstCaps := drive.RegistryCaps(dstProvider)

	// find a common hash method supported by both source (provide) and target (rapid).
	method := commonHashMethod(srcCaps.ProvideHashes, dstCaps.RapidUploadHashes)
	if method == "" {
		return false, nil
	}

	// resolve the source file's hash (allowStream=true lets the provider
	// download+hash when no precomputed fingerprint exists).
	hash, err := drive.ResolveTransferHash(job.SrcUser, job.SrcDrive, srcFile.FileID, method, true)
	if err != nil || hash == "" {
		return true, fmt.Errorf("rapid: resolve hash %s failed: %w", method, err)
	}

	// attempt秒传 on the target.
	result, err := drive.RapidUploadByHash(job.DstUser, job.DstDrive, drive.RapidUploadRequest{
		ParentID:  job.DstParent,
		FileName:  srcFile.Name,
		Method:    method,
		Hash:      hash,
		Size:      srcFile.Size,
		Duplicate: 0, // rename
	})
	if err != nil {
		return true, fmt.Errorf("rapid: %w", err)
	}
	if result != nil && result.Reuse {
		job.ProcessedBytes += srcFile.Size
		return true, nil
	}
	if result != nil && result.FileID != "" {
		// some providers return a file id without setting Reuse=true.
		job.ProcessedBytes += srcFile.Size
		return true, nil
	}
	return true, fmt.Errorf("rapid: target did not accept hash %s", method)
}

// commonHashMethod returns the first hash method that appears in both the
// source's ProvideHashes and the target's RapidUploadHashes lists.
// Returns "" when no common method exists.
func commonHashMethod(provide, rapid []string) string {
	rapidSet := make(map[string]bool, len(rapid))
	for _, r := range rapid {
		rapidSet[r] = true
	}
	for _, p := range provide {
		if rapidSet[p] {
			return p
		}
	}
	return ""
}

// tryStreamMigrate attempts to pipe the source download stream directly
// into the target upload via io.Pipe, avoiding a local temp file.
//
// It returns (supported, error):
//   - supported=false when the target provider does not expose a
//     StreamUploader capability, in which case the caller falls back to
//     spoolMigrate.
//   - supported=true with a non-nil error when streaming was attempted but
//     failed; the caller may still fall back to spoolMigrate.
func (e *Engine) tryStreamMigrate(ctx context.Context, job *Job, srcFile *model.File) (bool, error) {
	streamUploader, err := drive.StreamUploadHandler(job.DstUser, job.DstDrive)
	if err != nil {
		// target provider does not implement StreamUploader.
		return false, nil
	}

	// resolve the source download URL.
	dl, err := drive.GetDownloadURL(job.SrcUser, job.SrcDrive, srcFile.FileID, 3600)
	if err != nil {
		return true, fmt.Errorf("stream: resolve download url: %w", err)
	}

	// set up the io.Pipe: the download goroutine writes to pw, the upload
	// reads from pr.
	pr, pw := io.Pipe()

	// goroutine: stream the download into the pipe writer.
	downloadErrCh := make(chan error, 1)
	go func() {
		defer pw.Close()
		downloadErrCh <- downloadToCounted(ctx, dl, pw, job)
	}()

	// upload from the pipe reader on the target.
	uploadErr := streamUploader(ctx, job.DstParent, srcFile.Name, srcFile.Size, pr)

	// if the upload failed, we must ensure the download goroutine finishes
	// (it may still be blocked writing to pw). Cancel the pipe reader side.
	if uploadErr != nil {
		_ = pr.CloseWithError(uploadErr)
	}

	// wait for the download goroutine to complete.
	downloadErr := <-downloadErrCh

	if uploadErr != nil {
		return true, fmt.Errorf("stream: upload: %w", uploadErr)
	}
	if downloadErr != nil {
		return true, fmt.Errorf("stream: download: %w", downloadErr)
	}

	job.ProcessedBytes += srcFile.Size
	return true, nil
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
	hc := netx.NewClient(0) // honors global proxy; no total timeout for large files
	resp, err := hc.Do(ctx, http.MethodGet, dl.URL, dl.Headers, nil)
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
