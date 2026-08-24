package drive

import (
	"context"
	"errors"
	"fmt"
)

// Mkdir creates a folder.
func Mkdir(userID, driveID, parentID, name string) (result *MkdirResult, err error) {
	return MkdirContext(context.Background(), userID, driveID, parentID, name)
}

// MkdirContext is the cancellation-aware folder creation variant.
func MkdirContext(ctx context.Context, userID, driveID, parentID, name string) (result *MkdirResult, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	return d.Mkdir(ctx, c, parentID, name)
}

// RenameBatch renames files one-to-one with names.
func RenameBatch(userID, driveID string, fileRefs []FileRef, names []string) (out []RenameResult, err error) {
	if len(fileRefs) != len(names) {
		return nil, errors.New("drive: rename refs and names length mismatch")
	}
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	out = make([]RenameResult, 0, len(fileRefs))
	var renameErrs []error
	for i, ref := range fileRefs {
		name := names[i]
		r, err := d.Rename(context.Background(), c, ref.ID, name)
		if err != nil {
			out = append(out, RenameResult{FileID: "", ParentFileID: "", Name: name})
			renameErrs = append(renameErrs, fmt.Errorf("file %s: %w", ref.ID, err))
			continue
		}
		if r != nil {
			out = append(out, *r)
			continue
		}
		out = append(out, RenameResult{FileID: "", ParentFileID: "", Name: name})
		renameErrs = append(renameErrs, fmt.Errorf("file %s: provider returned an empty rename result", ref.ID))
	}
	return out, errors.Join(renameErrs...)
}

// TrashBatch moves files to the recycle bin.
func TrashBatch(userID, driveID string, fileIDs []string) (ids []string, err error) {
	return TrashBatchContext(context.Background(), userID, driveID, fileIDs)
}

// TrashBatchContext is the cancellation-aware recycle-bin variant.
func TrashBatchContext(ctx context.Context, userID, driveID string, fileIDs []string) (ids []string, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	return d.Trash(ctx, c, fileIDs)
}

// DeleteBatch permanently deletes files (skips trash where implemented).
func DeleteBatch(userID, driveID string, fileIDs []FileRef) (ids []string, err error) {
	return DeleteBatchContext(context.Background(), userID, driveID, fileIDs)
}

// DeleteBatchContext permanently deletes files while honoring cancellation.
func DeleteBatchContext(ctx context.Context, userID, driveID string, fileIDs []FileRef) (ids []string, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	return d.Delete(ctx, c, fileIDs)
}

// RestoreBatch restores files from the recycle bin.
func RestoreBatch(userID, driveID string, fileIDs []string) (ids []string, err error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	return d.Restore(context.Background(), c, fileIDs)
}

// MoveBatch moves files to a target folder.
func MoveBatch(userID, driveID string, fileIDs []FileRef, toParentID, toParentDesc string) (ids []string, err error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	return d.Move(context.Background(), c, fileIDs, toParentID, toParentDesc)
}

// CopyBatch copies files into a target folder.
func CopyBatch(userID, driveID string, fileIDs []FileRef, toParentID, toParentDesc string) (ids []string, err error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	return d.Copy(context.Background(), c, fileIDs, toParentID, toParentDesc)
}

// FavoriteBatch sets/clears remote favorite on files.
func FavoriteBatch(userID, driveID string, favorite bool, fileIDs []string) (ids []string, err error) {
	d, c, err := driverAndCtx(userID, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { err = withTokenPersist(err, c) }()
	return d.Favorite(context.Background(), c, fileIDs, favorite)
}
