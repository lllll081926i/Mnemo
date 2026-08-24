package migrate

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

// migrateDir recursively migrates a folder.
func (e *Engine) migrateDir(ctx context.Context, job *Job, dir *model.File, targetParent string) error {
	// Reuse a target directory created by an earlier interrupted attempt. The
	// checkpoint is written immediately after Mkdir succeeds, before listing
	// children, so a retry cannot create a duplicate folder tree.
	if existing := strings.TrimSpace(job.TargetDirectoryIDs[dir.FileID]); existing != "" {
		targetParent = existing
	} else {
		mk, err := drive.MkdirContext(ctx, job.DstUser, job.DstDrive, targetParent, dir.Name)
		if err != nil {
			return newMigrationError(err, 1)
		}
		if mk == nil {
			return newMigrationError(errors.New("migrate: target folder creation returned empty result"), 1)
		}
		if strings.TrimSpace(mk.Error) != "" {
			return newMigrationError(fmt.Errorf("migrate: create target folder %q: %s", dir.Name, mk.Error), 1)
		}
		if strings.TrimSpace(mk.FileID) == "" {
			return newMigrationError(fmt.Errorf("migrate: create target folder %q returned empty id", dir.Name), 1)
		}
		targetParent = mk.FileID
		if err := e.markTargetDirectory(job, dir.FileID, targetParent); err != nil {
			return newPartialMigrationError(fmt.Errorf("migrate: target folder %q was created but its checkpoint could not be persisted: %w", dir.Name, err), 1)
		}
	}
	// list source dir
	children, err := drive.ListDirAllContext(ctx, job.SrcUser, job.SrcDrive, dir.FileID, nil)
	if err != nil {
		if errors.Is(err, context.Canceled) || ctx.Err() != nil {
			return err
		}
		return newPartialMigrationError(fmt.Errorf("migrate: target folder %q was created but source listing failed: %w", dir.Name, err), 1)
	}
	var childFailures int64
	var lastChildErr error
	for i := range children {
		child := children[i]
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if jobHasID(job.CompletedFileIDs, child.FileID) {
			continue
		}
		if err := e.migrateOneTo(ctx, job, child.FileID, targetParent); err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			childFailures += failureCount(err)
			lastChildErr = err
		}
		e.emit(job)
	}
	if childFailures > 0 {
		if lastChildErr == nil {
			lastChildErr = errors.New("migrate: one or more directory children failed")
		}
		return newPartialMigrationError(fmt.Errorf("migrate: directory %q was partially copied and has failed children: %w", dir.Name, lastChildErr), childFailures)
	}
	return nil
}
