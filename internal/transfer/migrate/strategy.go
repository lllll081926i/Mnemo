package migrate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/drive/driveutil"
	"mnemo-go/internal/model"
)

// tryRapidMigrate attempts a hash-based秒传: if both the source provider can
// provide a hash and the target provider supports rapid upload with the same
// hash method, it resolves the source hash and calls RapidUploadByHash on the
// target.
//
// Returns (attempted, error):
//   - attempted=false when no common hash method exists (caller falls through).
//   - attempted=true with nil error on success.
//   - attempted=true with a rapidFallbackError only for a definite miss or a
//     source-side hash failure; other errors stop migration because target
//     state may be ambiguous.
func (e *Engine) tryRapidMigrate(ctx context.Context, job *Job, srcFile *model.File, targetParent string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return true, err
	}
	srcProvider := drive.ResolveProvider(job.SrcUser, job.SrcDrive, "")
	dstProvider := drive.ResolveProvider(job.DstUser, job.DstDrive, "")
	if !rapidUploadAllowed(srcProvider, dstProvider) {
		return false, nil
	}

	srcCaps := drive.RegistryCaps(srcProvider)
	dstCaps := drive.RegistryCaps(dstProvider)

	// find a common hash method supported by both source (provide) and target (rapid).
	method := commonHashMethod(srcCaps.ProvideHashes, dstCaps.RapidUploadHashes)
	if method == "" {
		return false, nil
	}

	// Only use a fingerprint already exposed by source metadata/cache. Downloading
	// the complete source merely to probe rapid upload makes a miss download the
	// same file twice; the subsequent stream/spool strategy is the only path that
	// should consume source content.
	hash, err := drive.ResolveTransferHashContext(ctx, job.SrcUser, job.SrcDrive, srcFile.FileID, method, false)
	if err != nil {
		if isContextError(err) {
			return true, err
		}
		// Hash resolution happens entirely on the source side, so falling back
		// cannot duplicate a destination object.
		return true, newRapidFallbackError(fmt.Errorf("rapid: resolve hash %s failed: %w", method, err))
	}
	if err := ctx.Err(); err != nil {
		return true, err
	}
	if strings.TrimSpace(hash) == "" {
		return true, newRapidFallbackError(fmt.Errorf("rapid: resolve hash %s returned empty hash", method))
	}

	// attempt秒传 on the target.
	result, err := drive.RapidUploadByHashContext(ctx, job.DstUser, job.DstDrive, drive.RapidUploadRequest{
		ParentID:  targetParent,
		FileName:  srcFile.Name,
		Method:    method,
		Hash:      strings.TrimSpace(hash),
		Size:      srcFile.Size,
		Duplicate: 0, // rename
	})
	if err != nil {
		// Once the target API was called, a timeout/transport/response error is
		// ambiguous: the remote object may already exist. Never turn that into a
		// second full upload. Providers report a definite miss with a nil error
		// and Reuse=false instead.
		return true, fmt.Errorf("rapid: %w", err)
	}
	if result != nil && result.Reuse {
		return true, nil
	}
	if result != nil && result.FileID != "" {
		// some providers return a file id without setting Reuse=true.
		return true, nil
	}
	return true, newRapidFallbackError(fmt.Errorf("rapid: target did not accept hash %s", method))
}

// rapidFallbackError marks a failure that occurred before the target could
// have created an object, or an explicit target-side miss. Only these errors
// may safely fall through to stream/spool upload.
type rapidFallbackError struct{ err error }

func (e *rapidFallbackError) Error() string { return e.err.Error() }
func (e *rapidFallbackError) Unwrap() error { return e.err }

func newRapidFallbackError(err error) error {
	if err == nil {
		return nil
	}
	return &rapidFallbackError{err: err}
}

func canFallbackAfterRapid(err error) bool {
	var fallback *rapidFallbackError
	return errors.As(err, &fallback)
}

func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// rapidUploadAllowed keeps providers with incompatible transfer semantics out
// of the cross-drive rapid-upload path. Yike exposes MD5 metadata for its own
// uploads, but it does not implement a target-side rapid-upload API.
func rapidUploadAllowed(srcProvider, dstProvider string) bool {
	return srcProvider != model.ProviderYike && dstProvider != model.ProviderYike
}

// commonHashMethod returns the first hash method that appears in both the
// source's ProvideHashes and the target's RapidUploadHashes lists.
// Returns "" when no common method exists.
func commonHashMethod(provide, rapid []string) string {
	rapidSet := make(map[string]bool, len(rapid))
	for _, r := range rapid {
		if method := canonicalHashMethod(r); method != "" {
			rapidSet[method] = true
		}
	}
	for _, p := range provide {
		method := canonicalHashMethod(p)
		if method != "" && rapidSet[method] {
			return method
		}
	}
	return ""
}

func canonicalHashMethod(method string) string {
	return strings.ToLower(strings.TrimSpace(method))
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
func (e *Engine) tryStreamMigrate(ctx context.Context, job *Job, srcFile *model.File, targetParent string) (bool, error) {
	streamUploader, err := drive.StreamUploadHandlerContext(ctx, job.DstUser, job.DstDrive)
	if err != nil {
		if errors.Is(err, drive.ErrNotImplemented) {
			// target provider does not implement StreamUploader.
			return false, nil
		}
		return true, fmt.Errorf("stream: resolve target uploader: %w", err)
	}

	uploadErr, downloadErr := pipeMigration(
		func(w io.Writer) error { return downloadMigrationSource(ctx, job, srcFile, w) },
		func(r io.Reader) error { return streamUploader(ctx, targetParent, srcFile.Name, srcFile.Size, r) },
	)
	if uploadErr != nil {
		return true, fmt.Errorf("stream: upload: %w", uploadErr)
	}
	if downloadErr != nil {
		return true, fmt.Errorf("stream: download: %w", downloadErr)
	}

	return true, nil
}

// pipeMigration connects one producer to one uploader and guarantees that an
// uploader which returns before consuming EOF cannot leave the producer
// goroutine blocked forever.
func pipeMigration(download func(io.Writer) error, upload func(io.Reader) error) (uploadErr, downloadErr error) {
	pr, pw := io.Pipe()

	downloadErrCh := make(chan error, 1)
	go func() {
		defer pw.Close()
		downloadErrCh <- download(pw)
	}()

	uploadErr = upload(pr)
	// A buggy/short-circuiting uploader may return before consuming the whole
	// reader. Always close the reader before waiting so the producer cannot
	// remain blocked forever in PipeWriter.Write.
	if uploadErr != nil {
		_ = pr.CloseWithError(uploadErr)
	} else {
		_ = pr.Close()
	}

	// if the upload failed, we must ensure the download goroutine finishes
	// (it may still be blocked writing to pw). Cancel the pipe reader side.
	// wait for the download goroutine to complete.
	downloadErr = <-downloadErrCh
	return uploadErr, downloadErr
}

// spoolMigrate downloads the source to a temp file then uploads it to the
// target. This is the legacy/fallback path.
func (e *Engine) spoolMigrate(ctx context.Context, job *Job, srcFile *model.File, targetParent string) error {
	// Resolve the target account before downloading a potentially large source
	// file. This is local setup only, but makes a deleted/misconfigured target
	// fail immediately instead of wasting source bandwidth and disk space.
	handler, err := drive.QueueUploadHandlerContext(ctx, job.DstUser, job.DstDrive)
	if err != nil {
		return migrationTargetUploadError(job, srcFile, err)
	}

	// stream to temp
	tmp, err := os.CreateTemp("", "mnemo-migrate-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if err := downloadMigrationSource(ctx, job, srcFile, tmp); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("migrate: flush temporary download: %w", err)
	}
	actualSize, err := migrationSpoolSize(tmp, srcFile.Size)
	if err != nil {
		return fmt.Errorf("migrate: source %q: %w", srcFile.Name, err)
	}
	updateMigrationFileSize(job, srcFile, actualSize)
	if err := drive.ValidateUploadItemsContext(ctx, job.DstUser, job.DstDrive, []drive.UploadValidationItem{{
		Name: srcFile.Name,
		Size: actualSize,
	}}); err != nil {
		return migrationTargetUploadError(job, srcFile, err)
	}
	// upload to target
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return err
	}
	ui := &model.UploadingUI{
		UploadID: srcFile.FileID,
		Info: model.UploadInfo{
			LocalFilePath: tmp.Name(), ParentFileID: targetParent,
			DriveID: job.DstDrive, Name: srcFile.Name, Size: srcFile.Size,
			ConflictPolicy: driveutil.ConflictPolicyRename,
		},
	}
	if err := handler(ctx, ui); err != nil {
		return migrationTargetUploadError(job, srcFile, err)
	}
	return nil
}

// migrationSpoolSize returns the local file length that will actually be
// handed to the destination provider. A positive size from source metadata is
// authoritative enough to detect truncated or substituted HTTP responses; a
// zero/negative value is treated as unknown because several providers omit it
// from list/detail responses.
func migrationSpoolSize(file *os.File, expectedSize int64) (int64, error) {
	if file == nil {
		return 0, errors.New("migrate: temporary download file is nil")
	}
	info, err := file.Stat()
	if err != nil {
		return 0, fmt.Errorf("migrate: stat temporary download: %w", err)
	}
	actualSize := info.Size()
	if expectedSize > 0 && actualSize != expectedSize {
		return 0, fmt.Errorf("migrate: download size mismatch: expected %d bytes, got %d bytes", expectedSize, actualSize)
	}
	return actualSize, nil
}

func migrationProgressSize(size int64) int64 {
	if size < 0 {
		return 0
	}
	return size
}

// updateMigrationFileSize makes progress and destination metadata agree with
// the downloaded spool. It is intentionally called only after the source size
// check has succeeded, so a failed/truncated download cannot distort progress.
func updateMigrationFileSize(job *Job, srcFile *model.File, actualSize int64) {
	if srcFile == nil {
		return
	}
	actualSize = migrationProgressSize(actualSize)
	declaredSize := migrationProgressSize(srcFile.Size)
	if job != nil {
		job.TotalBytes += actualSize - declaredSize
	}
	srcFile.Size = actualSize
}

func migrationTargetUploadError(job *Job, srcFile *model.File, err error) error {
	if err == nil {
		return nil
	}
	provider := "目标网盘"
	if job != nil {
		if resolved := strings.TrimSpace(drive.ResolveProvider(job.DstUser, job.DstDrive, "")); resolved != "" {
			provider = resolved
		}
	}
	name := "文件"
	if srcFile != nil && strings.TrimSpace(srcFile.Name) != "" {
		name = fmt.Sprintf("文件 %q", srcFile.Name)
	}
	return fmt.Errorf("migrate: %s 上传到 %s 失败: %w", name, provider, err)
}
