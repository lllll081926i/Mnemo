package migrate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Cancel cancels the migration job with the given id.
// It is safe to call even if the job is unknown or already finished;
// in those cases it is a no-op.
func (e *Engine) Cancel(jobID string) {
	e.cancelsMu.Lock()
	cancel, ok := e.cancels[jobID]
	e.cancelsMu.Unlock()
	if ok {
		cancel()
	}
}

// CancelAll stops every active migration. It is used during application
// shutdown so no migration can continue after the Wails context is gone.
func (e *Engine) CancelAll() {
	e.cancelsMu.Lock()
	cancels := make([]context.CancelFunc, 0, len(e.cancels))
	for _, cancel := range e.cancels {
		cancels = append(cancels, cancel)
	}
	e.cancelsMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

// registerCancel stores the cancel func for a job and returns a derived
// context that is cancelled by Cancel(jobID) or parent cancellation.
func (e *Engine) registerCancel(parent context.Context, jobID string) (context.Context, context.CancelFunc, bool) {
	if parent == nil {
		parent = context.Background()
	}
	e.cancelsMu.Lock()
	if _, exists := e.cancels[jobID]; exists {
		e.cancelsMu.Unlock()
		return nil, nil, false
	}
	ctx, cancel := context.WithCancel(parent)
	e.cancels[jobID] = cancel
	e.cancelsMu.Unlock()
	return ctx, cancel, true
}

// releaseCancel drops the cancel entry for a finished job.
func (e *Engine) releaseCancel(jobID string) {
	e.cancelsMu.Lock()
	delete(e.cancels, jobID)
	e.cancelsMu.Unlock()
}

// saveJob persists the job to the store (if configured).
func (e *Engine) saveJob(job *Job) error {
	if e.store == nil || job == nil {
		return nil
	}
	e.persistMu.Lock()
	defer e.persistMu.Unlock()
	return e.store.SaveMigrateJob(job)
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
	if job == nil {
		return errors.New("migrate: nil job")
	}
	if job.ID == "" {
		return errors.New("migrate: job id is empty")
	}
	if err := ValidateEndpoints(job.SrcUser, job.SrcDrive, job.DstUser, job.DstDrive); err != nil {
		return err
	}
	ctx, cancel, registered := e.registerCancel(ctx, job.ID)
	if !registered {
		return fmt.Errorf("migrate: job %q is already running", job.ID)
	}
	defer func() {
		cancel()
		e.releaseCancel(job.ID)
	}()

	job.Total = int64(len(job.FileIDs))
	completedTopLevel := completedTopLevelCount(job)
	job.Processed = completedTopLevel
	job.Failed = 0
	// Byte counters describe the current run. Recomputing them prevents a
	// retry from double-counting bytes that completed before interruption.
	job.TotalBytes = 0
	job.ProcessedBytes = 0
	job.Status = "running"
	job.Message = ""
	if job.CreatedAt == 0 {
		job.CreatedAt = time.Now().Unix()
	}
	job.UpdatedAt = time.Now().Unix()
	if err := e.saveJob(job); err != nil {
		return fmt.Errorf("migrate: persist running job: %w", err)
	}
	e.emit(job)
	succeeded := completedTopLevel
	partial := false
	for _, fileID := range job.FileIDs {
		if ctx.Err() != nil {
			return e.finishCanceled(job, ctx.Err())
		}
		if jobHasID(job.CompletedFileIDs, fileID) {
			continue
		}
		err := e.migrateOne(ctx, job, fileID)
		if err != nil {
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				return e.finishCanceled(job, ctx.Err())
			}
			job.Failed += failureCount(err)
			partial = partial || isPartialError(err)
			job.Message = err.Error()
		} else {
			succeeded++
		}
		job.Processed++
		job.UpdatedAt = time.Now().Unix()
		if saveErr := e.saveJob(job); saveErr != nil {
			return fmt.Errorf("migrate: persist job progress: %w", saveErr)
		}
		e.emit(job)
	}
	job.Status = migrationResultStatus(job.Failed, succeeded, partial)
	job.UpdatedAt = time.Now().Unix()
	if err := e.saveJob(job); err != nil {
		return fmt.Errorf("migrate: persist final job state: %w", err)
	}
	e.emit(job)
	return nil
}

// ValidateEndpoints rejects migrations whose source and target identify the
// same mounted drive. A same-drive subtree copy can recursively discover the
// files it just created; callers should use the provider's Move/Copy operation
// instead, where the provider can enforce its own parent relationship rules.
func ValidateEndpoints(srcUser, srcDrive, dstUser, dstDrive string) error {
	srcUser = strings.TrimSpace(srcUser)
	srcDrive = strings.TrimSpace(srcDrive)
	dstUser = strings.TrimSpace(dstUser)
	dstDrive = strings.TrimSpace(dstDrive)
	if srcUser == "" && srcDrive == "" && dstUser == "" && dstDrive == "" {
		// Keep the engine's empty-file-list smoke test and programmatic no-op
		// jobs backwards-compatible; the app binding validates real requests.
		return nil
	}
	if srcUser == "" || srcDrive == "" || dstUser == "" || dstDrive == "" {
		return errors.New("migrate: source and target account are incomplete")
	}
	if srcUser == dstUser && srcDrive == dstDrive {
		return errors.New("migrate: source and target cannot be the same drive; use the drive move operation")
	}
	return nil
}

func (e *Engine) finishCanceled(job *Job, cause error) error {
	if cause == nil {
		cause = context.Canceled
	}
	job.Status = "canceled"
	job.Message = cause.Error()
	job.UpdatedAt = time.Now().Unix()
	if err := e.saveJob(job); err != nil {
		return errors.Join(cause, fmt.Errorf("migrate: persist canceled job: %w", err))
	}
	e.emit(job)
	return cause
}

func (e *Engine) emit(job *Job) {
	if e.onProgress != nil {
		e.onProgress(job)
	}
}

// partialError marks an error whose cause is a post-migration source-delete
// failure. The file was copied successfully but the move is incomplete.
type partialError string

func (p partialError) Error() string { return string(p) }

func isPartialError(err error) bool {
	var p partialError
	return errors.As(err, &p)
}

type migrationError struct {
	err      error
	failures int64
}

func (e *migrationError) Error() string { return e.err.Error() }
func (e *migrationError) Unwrap() error { return e.err }

func newMigrationError(err error, failures int64) error {
	if err == nil {
		return nil
	}
	if failures < 1 {
		failures = 1
	}
	return &migrationError{err: err, failures: failures}
}

func newPartialMigrationError(err error, failures int64) error {
	if err == nil {
		return nil
	}
	return newMigrationError(partialError(err.Error()), failures)
}

func migrationResultStatus(failed, succeeded int64, partial bool) string {
	if failed == 0 {
		return "completed"
	}
	if succeeded > 0 || partial {
		return "partial"
	}
	return "failed"
}

func failureCount(err error) int64 {
	var me *migrationError
	if errors.As(err, &me) && me.failures > 0 {
		return me.failures
	}
	return 1
}
