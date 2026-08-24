package migrate

import (
	"context"
	"errors"

	"mnemo-go/internal/drive"
)

// migrateOne migrates a single file. It attempts strategies in order:
//  1. Rapid upload (hash-based秒传) if both source and target share a hash.
//  2. Stream pipe (source download Reader → target StreamUploader).
//  3. Spool (temp-file) fallback when streaming is unsupported. A failed
//     stream is not automatically retried because the target may already have
//     created a partial object or upload session.
func (e *Engine) migrateOne(ctx context.Context, job *Job, fileID string) error {
	return e.migrateOneTo(ctx, job, fileID, job.DstParent)
}

func (e *Engine) migrateOneTo(ctx context.Context, job *Job, fileID, targetParent string) error {
	if jobHasID(job.CompletedFileIDs, fileID) {
		return nil
	}
	// resolve source file
	srcFile, err := drive.GetFileContext(ctx, job.SrcUser, job.SrcDrive, fileID)
	if err != nil {
		// A move may have deleted the source successfully and then failed to
		// persist the completed checkpoint. The durable copied checkpoint proves
		// the destination exists; a now-missing source therefore means cleanup is
		// complete and must not trigger another upload.
		if job.Move && jobHasID(job.CopiedFileIDs, fileID) && errors.Is(err, drive.ErrNotFound) {
			return e.markCompleted(job, fileID)
		}
		return err
	}
	if srcFile == nil {
		return errors.New("migrate: source file is empty")
	}
	if job.Move && jobHasID(job.CopiedFileIDs, srcFile.FileID) {
		if err := e.finalizeMove(ctx, job, srcFile); err != nil {
			return err
		}
		return e.markCompleted(job, srcFile.FileID)
	}
	if srcFile.IsDir {
		if err := e.migrateDir(ctx, job, srcFile, targetParent); err != nil {
			return err
		}
		return e.completeResource(ctx, job, srcFile)
	}
	// Accumulate only a non-negative declared size. Some providers do not know
	// a file's size until a detail/download response; spoolMigrate reconciles
	// that value with the actual local bytes before invoking the target upload.
	job.TotalBytes += migrationProgressSize(srcFile.Size)
	progressStart := job.ProcessedBytes
	defer func() { e.emit(job) }()
	// Reject unsupported names/sizes before rapid, stream or spool can mutate
	// the destination. Providers still validate again inside their upload
	// implementation because unknown source sizes may be reconciled later.
	if err := drive.ValidateUploadItemsContext(ctx, job.DstUser, job.DstDrive, []drive.UploadValidationItem{{
		Name: srcFile.Name,
		Size: migrationProgressSize(srcFile.Size),
	}}); err != nil {
		return migrationTargetUploadError(job, srcFile, err)
	}

	// 1) Try rapid upload (秒传).
	if migrated, err := e.tryRapidMigrate(ctx, job, srcFile, targetParent); migrated {
		if err == nil {
			completeBytes(job, progressStart, srcFile.Size)
			return e.completeResource(ctx, job, srcFile)
		}
		if isContextError(err) {
			job.ProcessedBytes = progressStart
			return err
		}
		if !canFallbackAfterRapid(err) {
			job.ProcessedBytes = progressStart
			return migrationTargetUploadError(job, srcFile, err)
		}
		// The hash was unavailable or the target explicitly reported a miss;
		// neither case can have created an ambiguous destination object.
		job.ProcessedBytes = progressStart
	}

	// 2) Attempt streaming migration. Once an upload has been attempted, an
	// automatic spool retry is unsafe: several providers create the target
	// object before the stream body has completed.
	if migrated, err := e.tryStreamMigrate(ctx, job, srcFile, targetParent); migrated {
		if err != nil {
			job.ProcessedBytes = progressStart
			return migrationTargetUploadError(job, srcFile, err)
		}
		// streaming succeeded; handle move cleanup.
		completeBytes(job, progressStart, srcFile.Size)
		return e.completeResource(ctx, job, srcFile)
	}
	// 3) streaming not supported by this provider — use spool.
	if err := e.spoolMigrate(ctx, job, srcFile, targetParent); err != nil {
		job.ProcessedBytes = progressStart
		return err
	}
	completeBytes(job, progressStart, srcFile.Size)
	return e.completeResource(ctx, job, srcFile)
}
