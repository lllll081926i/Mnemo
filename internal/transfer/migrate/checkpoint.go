package migrate

import (
	"context"
	"fmt"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

// completeResource records a successful target copy before attempting source
// deletion for a move. If cleanup is interrupted or rejected, a later retry
// performs only the cleanup and never uploads a second destination copy.
func (e *Engine) completeResource(ctx context.Context, job *Job, srcFile *model.File) error {
	if job.Move {
		if err := e.markCopied(job, srcFile.FileID); err != nil {
			// Never delete the source until the durable "target exists"
			// checkpoint has been confirmed.
			return fmt.Errorf("migrate: persist copied checkpoint for %q: %w", srcFile.Name, err)
		}
		if err := e.finalizeMove(ctx, job, srcFile); err != nil {
			return err
		}
	}
	if err := e.markCompleted(job, srcFile.FileID); err != nil {
		return fmt.Errorf("migrate: persist completed checkpoint for %q: %w", srcFile.Name, err)
	}
	return nil
}

// finalizeMove deletes the source file in move mode. If deletion fails it
// returns a partialError so the caller records a partial (not completed)
// status and surfaces a message — the migration is effectively a copy in
// that case.
func (e *Engine) finalizeMove(ctx context.Context, job *Job, srcFile *model.File) error {
	if !job.Move {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	isDir := srcFile.IsDir
	refs := []drive.FileRef{{ID: srcFile.FileID, IsDir: &isDir}}
	provider := drive.ProviderOf(job.SrcUser, job.SrcDrive, "")
	caps := drive.RegistryCaps(provider)
	var (
		removed []string
		err     error
	)
	if caps.RecycleBin {
		removed, err = drive.TrashBatchContext(ctx, job.SrcUser, job.SrcDrive, []string{srcFile.FileID})
	} else {
		removed, err = drive.DeleteBatchContext(ctx, job.SrcUser, job.SrcDrive, refs)
	}
	if err != nil {
		msg := fmt.Sprintf("move: source upload succeeded but source cleanup failed for %q: %v", srcFile.Name, err)
		return newMigrationError(partialError(msg), 1)
	}
	if !containsID(removed, srcFile.FileID) {
		msg := fmt.Sprintf("move: source cleanup returned no matching id for %q", srcFile.Name)
		return newMigrationError(partialError(msg), 1)
	}
	return nil
}

func containsID(ids []string, want string) bool {
	return jobHasID(ids, want)
}

func jobHasID(ids []string, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return false
	}
	for _, id := range ids {
		if strings.TrimSpace(id) == want {
			return true
		}
	}
	return false
}

func completedTopLevelCount(job *Job) int64 {
	if job == nil {
		return 0
	}
	var completed int64
	for _, id := range job.FileIDs {
		if jobHasID(job.CompletedFileIDs, id) {
			completed++
		}
	}
	return completed
}

func removeJobID(ids []string, want string) []string {
	out := ids[:0]
	for _, id := range ids {
		if !jobHasID([]string{id}, want) {
			out = append(out, id)
		}
	}
	return out
}

// markCopied persists the point at which a destination resource is durable.
func (e *Engine) markCopied(job *Job, fileID string) error {
	if job == nil || jobHasID(job.CopiedFileIDs, fileID) {
		return nil
	}
	previousUpdatedAt := job.UpdatedAt
	job.CopiedFileIDs = append(job.CopiedFileIDs, fileID)
	job.UpdatedAt = time.Now().Unix()
	if err := e.saveJob(job); err != nil {
		job.CopiedFileIDs = removeJobID(job.CopiedFileIDs, fileID)
		job.UpdatedAt = previousUpdatedAt
		return err
	}
	e.emit(job)
	return nil
}

// markCompleted persists a resource that no longer needs any transfer or
// source-cleanup work. It is deliberately called for nested files too.
func (e *Engine) markCompleted(job *Job, fileID string) error {
	if job == nil || jobHasID(job.CompletedFileIDs, fileID) {
		return nil
	}
	previousCompleted := append([]string(nil), job.CompletedFileIDs...)
	previousCopied := append([]string(nil), job.CopiedFileIDs...)
	previousUpdatedAt := job.UpdatedAt
	job.CompletedFileIDs = append(job.CompletedFileIDs, fileID)
	job.CopiedFileIDs = removeJobID(job.CopiedFileIDs, fileID)
	job.UpdatedAt = time.Now().Unix()
	if err := e.saveJob(job); err != nil {
		job.CompletedFileIDs = previousCompleted
		job.CopiedFileIDs = previousCopied
		job.UpdatedAt = previousUpdatedAt
		return err
	}
	e.emit(job)
	return nil
}

func (e *Engine) markTargetDirectory(job *Job, sourceID, targetID string) error {
	if job == nil || strings.TrimSpace(sourceID) == "" || strings.TrimSpace(targetID) == "" {
		return nil
	}
	if job.TargetDirectoryIDs == nil {
		job.TargetDirectoryIDs = make(map[string]string)
	}
	if job.TargetDirectoryIDs[sourceID] == targetID {
		return nil
	}
	previousTarget, hadPrevious := job.TargetDirectoryIDs[sourceID]
	previousUpdatedAt := job.UpdatedAt
	job.TargetDirectoryIDs[sourceID] = targetID
	job.UpdatedAt = time.Now().Unix()
	if err := e.saveJob(job); err != nil {
		if hadPrevious {
			job.TargetDirectoryIDs[sourceID] = previousTarget
		} else {
			delete(job.TargetDirectoryIDs, sourceID)
		}
		job.UpdatedAt = previousUpdatedAt
		return err
	}
	e.emit(job)
	return nil
}
