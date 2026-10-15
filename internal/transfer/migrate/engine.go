// Package migrate implements cross-drive file migration: copy/move files
// between two accounts by streaming (source download → target upload).
package migrate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/drive/driveutil"
	"mnemo-go/internal/localfs"
	"mnemo-go/internal/logging"
	"mnemo-go/internal/model"
	"mnemo-go/internal/store"
	"mnemo-go/internal/transfer/dlengine"
)

// Job is a type alias for model.MigrateJob so existing callers (app layer,
// frontend events) continue to reference migrate.Job.
type Job = model.MigrateJob

// OnProgress is invoked per file.
type OnProgress func(j *Job)

// Engine runs migration jobs.
type Engine struct {
	downloadOptions func() (dlengine.Options, error)
	store           *store.Store
	onProgress      OnProgress
	persistMu       sync.Mutex
	// cancels stores per-job cancel funcs so Cancel(jobID) can abort a Run.
	cancelsMu sync.Mutex
	cancels   map[string]context.CancelFunc
	done      map[string]chan struct{}
	closed    bool
}

// NewEngine creates the migration engine. store may be nil (disables
// persistence and startup recovery).
func NewEngine(st *store.Store, onProgress OnProgress, options ...func(*Engine)) *Engine {
	engine := &Engine{
		store:      st,
		onProgress: onProgress,
		cancels:    make(map[string]context.CancelFunc),
		done:       make(map[string]chan struct{}),
	}
	for _, option := range options {
		option(engine)
	}
	return engine
}

func WithDownloadOptions(fn func() (dlengine.Options, error)) func(*Engine) {
	return func(engine *Engine) { engine.downloadOptions = fn }
}

func (e *Engine) sourceOptions() (dlengine.Options, error) {
	if e.downloadOptions != nil {
		return e.downloadOptions()
	}
	return dlengine.Options{}, nil
}

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
	e.closed = true
	cancels := make([]context.CancelFunc, 0, len(e.cancels))
	finished := make([]chan struct{}, 0, len(e.done))
	for _, cancel := range e.cancels {
		cancels = append(cancels, cancel)
	}
	for _, done := range e.done {
		finished = append(finished, done)
	}
	e.cancelsMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	for _, done := range finished {
		<-done
	}
}

// registerCancel stores the cancel func for a job and returns a derived
// context that is cancelled by Cancel(jobID) or parent cancellation.
func (e *Engine) registerCancel(parent context.Context, jobID string) (context.Context, context.CancelFunc, bool) {
	if parent == nil {
		parent = context.Background()
	}
	e.cancelsMu.Lock()
	if _, exists := e.cancels[jobID]; exists || e.closed {
		e.cancelsMu.Unlock()
		return nil, nil, false
	}
	ctx, cancel := context.WithCancel(parent)
	e.cancels[jobID] = cancel
	e.done[jobID] = make(chan struct{})
	e.cancelsMu.Unlock()
	return ctx, cancel, true
}

// releaseCancel drops the cancel entry for a finished job.
func (e *Engine) releaseCancel(jobID string) {
	e.cancelsMu.Lock()
	delete(e.cancels, jobID)
	if done, exists := e.done[jobID]; exists {
		close(done)
		delete(e.done, jobID)
	}
	e.cancelsMu.Unlock()
}

// saveJob persists the job to the store (if configured).
func (e *Engine) saveJob(job *Job) error {
	if e.store == nil || job == nil {
		return nil
	}
	e.persistMu.Lock()
	defer e.persistMu.Unlock()
	err := e.store.SaveMigrateJob(job)
	if err != nil {
		logging.Warn("migration checkpoint persistence failed", "job_id", job.ID, "error", err)
	}
	if err != nil {
		return errors.Join(errCheckpoint, err)
	}
	return nil
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
	seenIDs := make(map[string]bool, len(job.FileIDs))
	for _, fileID := range job.FileIDs {
		if strings.TrimSpace(fileID) == "" || seenIDs[fileID] {
			return errors.New("迁移文件 ID 不能为空或重复")
		}
		seenIDs[fileID] = true
	}
	if err := ValidateEndpoints(job.SrcUser, job.SrcDrive, job.DstUser, job.DstDrive); err != nil {
		return err
	}
	ctx, cancel, registered := e.registerCancel(ctx, job.ID)
	if !registered {
		return fmt.Errorf("migrate: job %q is already running or engine is closed", job.ID)
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
	job.Stage = ""
	job.StageName = ""
	job.StageProgress = 0
	if job.CreatedAt == 0 {
		job.CreatedAt = time.Now().Unix()
	}
	job.UpdatedAt = time.Now().Unix()
	if err := e.saveJob(job); err != nil {
		return e.finishCheckpointFailure(job, err)
	}
	e.emit(job)
	succeeded := completedTopLevel
	partial := false
	var failures []error
	for _, fileID := range job.FileIDs {
		if ctx.Err() != nil {
			return e.finishCanceled(job, ctx.Err())
		}
		if jobHasID(job.CompletedFileIDs, fileID) {
			continue
		}
		err := e.migrateOne(ctx, job, fileID)
		if err != nil {
			if errors.Is(err, errCheckpoint) {
				return e.finishCheckpointFailure(job, err)
			}
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				return e.finishCanceled(job, ctx.Err())
			}
			job.Failed += failureCount(err)
			failures = append(failures, fmt.Errorf("%s: %w", fileID, err))
			partial = partial || isPartialError(err)
			job.Message = err.Error()
		} else {
			succeeded++
		}
		job.Processed++
		job.UpdatedAt = time.Now().Unix()
		if err := e.saveJob(job); err != nil {
			return e.finishCheckpointFailure(job, err)
		}
		e.emit(job)
	}
	job.Status = migrationResultStatus(job.Failed, succeeded, partial)
	job.Stage = ""
	job.StageName = ""
	job.StageProgress = 0
	job.UpdatedAt = time.Now().Unix()
	if err := e.saveJob(job); err != nil {
		return e.finishCheckpointFailure(job, err)
	}
	e.emit(job)
	return errors.Join(failures...)
}

var errCheckpoint = errors.New("迁移恢复记录保存失败")

func (e *Engine) finishCheckpointFailure(job *Job, cause error) error {
	job.Status = "failed"
	job.Stage, job.StageName, job.StageProgress = "", "", 0
	job.Message = cause.Error()
	e.emit(job)
	return cause
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
	job.Stage = ""
	job.StageName = ""
	job.StageProgress = 0
	job.Message = cause.Error()
	job.UpdatedAt = time.Now().Unix()
	if err := e.saveJob(job); err != nil {
		cause = errors.Join(cause, err)
		job.Message = cause.Error()
	}
	e.emit(job)
	return cause
}

// migrateOne migrates a single file. It attempts strategies in order:
//  1. Rapid upload (hash-based秒传) if both source and target share a hash.
//  2. Stream pipe (source download Reader → target StreamUploader).
//  3. Spool (temp-file) fallback.
func (e *Engine) migrateOne(ctx context.Context, job *Job, fileID string) error {
	return e.migrateOneTo(ctx, job, fileID, job.DstParent)
}

func (e *Engine) migrateOneTo(ctx context.Context, job *Job, fileID, targetParent string) (resultErr error) {
	if jobHasID(job.CompletedFileIDs, fileID) {
		return nil
	}
	if job.Items == nil {
		job.Items = map[string]model.MigrateItem{}
	}
	previous := job.Items[fileID]
	if previous.CommitState == "confirmed" && !jobHasID(job.CopiedFileIDs, fileID) {
		if previous.TargetID == "" {
			return errors.New("目标已受理但缺少可查询的文件 ID，未重复传输；请先核对目标目录")
		}
		source, err := drive.GetFileFreshContext(ctx, job.SrcUser, job.SrcDrive, fileID)
		if err != nil {
			return err
		}
		if source == nil || !previous.SourceRecorded || source.Size != previous.Size || source.Time != previous.SourceTime || sourceHash(source) != previous.SourceHash {
			return errors.New("目标已提交，但来源已变化或缺少原始记录；未重复传输，已保留来源")
		}
		target, err := drive.GetFileFreshContext(ctx, job.DstUser, job.DstDrive, previous.TargetID)
		if err != nil {
			return err
		}
		if err := compareMoveTarget(source, target); err != nil {
			return fmt.Errorf("目标已提交但复核失败，未重复传输: %w", err)
		}
		if err := e.completeResource(ctx, job, source); err != nil {
			return err
		}
		previous.Status = "completed"
		job.Items[fileID] = previous
		return e.saveJob(job)
	}
	if previous.CommitState == "pending" || previous.CommitState == "unknown" {
		return fmt.Errorf("目标提交结果未知，已阻止重复传输；请核对目标文件后处理：%s", previous.Name)
	}
	item := model.MigrateItem{ID: fileID, Name: fileID, ParentID: targetParent, Status: "running", Verification: "unverified"}
	if job.Move && jobHasID(job.CopiedFileIDs, fileID) {
		item = previous
		item.ID, item.Status, item.Error = fileID, "running", ""
	}
	job.Items[fileID] = item
	defer func() {
		job.Stage = ""
		job.StageName = ""
		job.StageProgress = 0
		item.TargetID = job.Items[fileID].TargetID
		item.CommitState = job.Items[fileID].CommitState
		if verification := job.Items[fileID].Verification; verification == "remote-size" || verification == "remote-hash" {
			item.Verification = verification
		}
		if resultErr != nil {
			item.Status = "failed"
			item.Error = resultErr.Error()
			if errors.Is(resultErr, context.Canceled) {
				item.Status = "canceled"
			}
		} else {
			item.Status = "completed"
		}
		job.Items[fileID] = item
		if err := e.saveJob(job); err != nil {
			resultErr = errors.Join(resultErr, err)
		}
		e.emit(job)
	}()
	// resolve source file
	srcFile, err := drive.GetFileFreshContext(ctx, job.SrcUser, job.SrcDrive, fileID)
	if err != nil {
		return err
	}
	if srcFile == nil {
		return errors.New("migrate: source file is empty")
	}
	if job.Move && jobHasID(job.CopiedFileIDs, srcFile.FileID) {
		if !srcFile.IsDir && (!item.SourceRecorded || item.Size != srcFile.Size || item.SourceTime != srcFile.Time || item.SourceHash != sourceHash(srcFile)) {
			return newPartialMigrationError(errors.New("来源文件已变化或缺少原始记录，已保留来源，请核对后重新迁移"), 1)
		}
		job.Stage = "cleaning"
		job.StageName = srcFile.Name
		job.StageProgress = 0
		e.emit(job)
		if err := e.finalizeMove(ctx, job, srcFile); err != nil {
			return err
		}
		return e.markCompleted(job, srcFile.FileID)
	}
	item.Name = srcFile.Name
	item.Size = srcFile.Size
	item.IsDir = srcFile.IsDir
	item.SourceRecorded, item.SourceTime, item.SourceHash = true, srcFile.Time, sourceHash(srcFile)
	job.Items[fileID] = item
	e.emit(job)
	if srcFile.IsDir {
		job.Stage = "scanning"
		job.StageName = srcFile.Name
		job.StageProgress = 0
		e.emit(job)
		if err := e.migrateDir(ctx, job, srcFile, targetParent); err != nil {
			return err
		}
		return e.completeResource(ctx, job, srcFile)
	}
	if err := drive.ValidateUploadItemsContext(ctx, job.DstUser, job.DstDrive, []drive.UploadValidationItem{{Name: srcFile.Name, Size: srcFile.Size}}); err != nil {
		return err
	}
	// accumulate total bytes for progress
	job.TotalBytes += srcFile.Size
	progressStart := job.ProcessedBytes
	defer func() { e.emit(job) }()

	// 1) Try rapid upload (秒传).
	if migrated, err := e.tryRapidMigrate(ctx, job, srcFile, targetParent); migrated {
		if err == nil {
			completeBytes(job, progressStart, srcFile.Size)
			return e.completeResource(ctx, job, srcFile)
		}
		job.ProcessedBytes = progressStart
		return err
	}
	job.Stage = "uploading"
	job.StageName = srcFile.Name
	job.StageProgress = 0
	e.emit(job)

	// 2) Attempt streaming migration; fall back to spool on failure.
	if migrated, err := e.tryStreamMigrate(ctx, job, srcFile, targetParent); migrated {
		if err != nil {
			job.ProcessedBytes = progressStart
			return err
		}
		// streaming succeeded; handle move cleanup.
		item.Verification = "transferred-size"
		completeBytes(job, progressStart, srcFile.Size)
		return e.completeResource(ctx, job, srcFile)
	}
	// 3) streaming not supported by this provider — use spool.
	if err := e.spoolMigrate(ctx, job, srcFile, targetParent); err != nil {
		job.ProcessedBytes = progressStart
		return err
	}
	completeBytes(job, progressStart, srcFile.Size)
	item.Verification = "transferred-size"
	return e.completeResource(ctx, job, srcFile)
}

// completeResource records a successful target copy before attempting source
// deletion for a move. If cleanup is interrupted or rejected, a later retry
// performs only the cleanup and never uploads a second destination copy.
func (e *Engine) completeResource(ctx context.Context, job *Job, srcFile *model.File) error {
	if job.Move {
		if err := e.markCopied(job, srcFile.FileID); err != nil {
			return newPartialMigrationError(fmt.Errorf("目标已复制，但恢复记录保存失败，已保留来源: %w", err), 1)
		}
		job.Stage = "cleaning"
		job.StageName = srcFile.Name
		job.StageProgress = 0
		e.emit(job)
		if err := e.finalizeMove(ctx, job, srcFile); err != nil {
			return err
		}
	}
	return e.markCompleted(job, srcFile.FileID)
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
func (e *Engine) tryRapidMigrate(ctx context.Context, job *Job, srcFile *model.File, targetParent string) (bool, error) {
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
	job.Stage = "hashing"
	job.StageName = srcFile.Name
	job.StageProgress = 0
	e.emit(job)

	// resolve the source file's hash (allowStream=true lets the provider
	// download+hash when no precomputed fingerprint exists).
	hash, err := drive.ResolveTransferHashContext(ctx, job.SrcUser, job.SrcDrive, srcFile.FileID, method, true)
	if err != nil {
		return true, fmt.Errorf("rapid: resolve hash %s failed: %w", method, err)
	}
	if strings.TrimSpace(hash) == "" {
		return true, fmt.Errorf("rapid: resolve hash %s returned empty hash", method)
	}
	job.Stage = "checking"
	job.StageProgress = 0
	e.emit(job)
	if err := e.setCommitState(job, srcFile.FileID, "pending"); err != nil {
		return true, err
	}

	// attempt秒传 on the target.
	result, err := drive.RapidUploadByHashContext(ctx, job.DstUser, job.DstDrive, drive.RapidUploadRequest{
		ParentID:  targetParent,
		FileName:  srcFile.Name,
		Method:    method,
		Hash:      hash,
		Size:      srcFile.Size,
		Duplicate: 0, // rename
	})
	if err != nil {
		return true, errors.Join(fmt.Errorf("秒传回执未确认，已停止自动降级: %w", err), e.setCommitState(job, srcFile.FileID, "unknown"))
	}
	if result != nil && result.Reuse {
		if job.Items != nil {
			item := job.Items[srcFile.FileID]
			item.TargetID = result.FileID
			job.Items[srcFile.FileID] = item
		}
		return true, e.setCommitState(job, srcFile.FileID, "confirmed")
	}
	if result == nil {
		return true, errors.Join(errors.New("秒传未返回受理回执，已停止自动降级"), e.setCommitState(job, srcFile.FileID, "unknown"))
	}
	if err := e.setCommitState(job, srcFile.FileID, "not_committed"); err != nil {
		return true, err
	}
	return false, nil
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
func (e *Engine) tryStreamMigrate(ctx context.Context, job *Job, srcFile *model.File, targetParent string) (bool, error) {
	streamUploader, err := drive.StreamUploadHandlerContext(ctx, job.DstUser, job.DstDrive)
	if err != nil {
		if errors.Is(err, drive.ErrNotImplemented) {
			return false, nil
		}
		return true, err
	}

	// resolve the source download URL.
	dl, err := drive.GetDownloadURLContext(ctx, job.SrcUser, job.SrcDrive, srcFile.FileID, 3600)
	if err != nil {
		return true, fmt.Errorf("stream: resolve download url: %w", err)
	}

	start := job.ProcessedBytes
	opts, err := e.sourceOptions()
	if err != nil {
		return true, err
	}
	if err := e.setCommitState(job, srcFile.FileID, "pending"); err != nil {
		return true, err
	}
	err = streamMigration(ctx, dl, job, func(reader io.Reader) error {
		return streamUploader(ctx, targetParent, srcFile.Name, srcFile.Size, reader)
	}, opts)
	if err == nil && srcFile.Size >= 0 && job.ProcessedBytes-start != srcFile.Size {
		err = fmt.Errorf("stream: size mismatch: expected %d, received %d", srcFile.Size, job.ProcessedBytes-start)
	}
	if err != nil {
		return true, errors.Join(fmt.Errorf("流式提交结果未确认，未再次上传或本机中转: %w", err), e.setCommitState(job, srcFile.FileID, "unknown"))
	}
	return true, e.setCommitState(job, srcFile.FileID, "confirmed")
}

func streamMigration(ctx context.Context, dl *model.DownloadURL, job *Job, upload func(io.Reader) error, options ...dlengine.Options) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// set up the io.Pipe: the download goroutine writes to pw, the upload
	// reads from pr.
	pr, pw := io.Pipe()
	defer pr.Close()

	// goroutine: stream the download into the pipe writer.
	downloadErrCh := make(chan error, 1)
	go func() {
		err := downloadToCounted(ctx, dl, pw, job, options...)
		_ = pw.CloseWithError(err)
		downloadErrCh <- err
	}()

	// upload from the pipe reader on the target.
	uploadErr := upload(pr)

	// An uploader may return before reading the body, even without an error.
	// Release blocked pipe writes and cancel any outstanding source request.
	_ = pr.CloseWithError(uploadErr)
	cancel()

	// wait for the download goroutine to complete.
	downloadErr := <-downloadErrCh

	if uploadErr != nil {
		return fmt.Errorf("stream: upload: %w", uploadErr)
	}
	if downloadErr != nil {
		return fmt.Errorf("stream: download: %w", downloadErr)
	}

	return nil
}

// spoolMigrate downloads the source to a temp file then uploads it to the
// target. This is the legacy/fallback path.
func (e *Engine) spoolMigrate(ctx context.Context, job *Job, srcFile *model.File, targetParent string) error {
	if err := requireSpoolSpace(os.TempDir(), srcFile.Size); err != nil {
		return err
	}
	job.Stage = "downloading"
	job.StageProgress = 0
	e.emit(job)
	// resolve download url
	dl, err := drive.GetDownloadURLContext(ctx, job.SrcUser, job.SrcDrive, srcFile.FileID, 3600)
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
	opts, err := e.sourceOptions()
	if err != nil {
		return err
	}
	if err := downloadToCounted(ctx, dl, tmp, job, opts); err != nil {
		return err
	}
	if stat, err := tmp.Stat(); err != nil {
		return err
	} else if srcFile.Size >= 0 && stat.Size() != srcFile.Size {
		return fmt.Errorf("migrate: size mismatch: expected %d, received %d", srcFile.Size, stat.Size())
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("本机中转文件落盘失败，未上传目标: %w", err)
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
	job.Stage = "local-upload"
	job.StageProgress = 0
	e.emit(job)
	ui.ConfigureUploadStage(func(stage string, percent int) {
		if job.Stage != stage || job.StageProgress != percent {
			job.Stage = stage
			job.StageProgress = percent
			e.emit(job)
		}
	})
	ui.ConfigureUploadRuntime(func(_ int64, percent int) {
		if job.Stage != "local-upload" || job.StageProgress != percent {
			job.Stage = "local-upload"
			job.StageProgress = percent
			e.emit(job)
		}
	}, func() bool { return ctx.Err() != nil })
	handler, err := drive.QueueUploadHandlerContext(ctx, job.DstUser, job.DstDrive)
	if err != nil {
		return err
	}
	if err := e.setCommitState(job, srcFile.FileID, "pending"); err != nil {
		return err
	}
	if err := handler(ctx, ui); err != nil {
		return errors.Join(fmt.Errorf("目标上传回执未确认，已保留来源: %w", err), e.setCommitState(job, srcFile.FileID, "unknown"))
	}
	if job.Items != nil {
		item := job.Items[srcFile.FileID]
		item.TargetID = ui.Upload.FileID
		job.Items[srcFile.FileID] = item
	}
	return e.setCommitState(job, srcFile.FileID, "confirmed")
}

func requireSpoolSpace(directory string, required int64) error {
	available, err := localfs.AvailableBytes(directory)
	if err != nil {
		return fmt.Errorf("无法检查本机中转空间，未开始下载: %w", err)
	}
	if required < 0 {
		return errors.New("来源文件大小未知，无法安全预留本机中转空间")
	}
	const reserve = 64 * 1024 * 1024
	if available < reserve || uint64(required) > available-reserve {
		return errors.New("本机临时盘空间不足，未开始中转；请释放空间或更改系统临时目录")
	}
	return nil
}

func (e *Engine) setCommitState(job *Job, fileID, state string) error {
	if job.Items == nil {
		job.Items = map[string]model.MigrateItem{}
	}
	item := job.Items[fileID]
	item.ID = fileID
	item.CommitState = state
	job.Items[fileID] = item
	return e.saveJob(job)
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
	if err := verifyMoveTarget(ctx, job, srcFile); err != nil {
		return newPartialMigrationError(err, 1)
	}
	if srcFile.IsDir {
		children, err := drive.ListDirAllContext(ctx, job.SrcUser, job.SrcDrive, srcFile.FileID, nil)
		if err != nil {
			return newPartialMigrationError(fmt.Errorf("清理来源前重新检查目录失败: %w", err), 1)
		}
		if len(children) != 0 {
			return newPartialMigrationError(errors.New("来源目录仍有文件，已保留目录，请核对新增或未迁移内容"), 1)
		}
	} else {
		current, err := drive.GetFileFreshContext(ctx, job.SrcUser, job.SrcDrive, srcFile.FileID)
		if err != nil {
			return newPartialMigrationError(fmt.Errorf("清理来源前重新检查文件失败: %w", err), 1)
		}
		if current == nil || current.IsDir || current.Size != srcFile.Size || current.Time != srcFile.Time || sourceHash(current) != sourceHash(srcFile) {
			return newPartialMigrationError(errors.New("来源文件在复制期间变化，已保留来源，请核对后重新迁移"), 1)
		}
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
			return newPartialMigrationError(err, 1)
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
			if errors.Is(err, context.Canceled) || errors.Is(err, errCheckpoint) {
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

func (e *Engine) emit(job *Job) {
	if e.onProgress != nil {
		e.onProgress(Snapshot(job))
	}
}

// partialError marks an error whose cause is a post-migration source-delete
// failure. The file was copied successfully but the move is incomplete.
type partialError string

func (p partialError) Error() string { return string(p) }

func isPartialError(err error) bool {
	var p partialError
	var partial *partialMigrationError
	return errors.As(err, &p) || errors.As(err, &partial)
}

type partialMigrationError struct{ cause error }

func (err *partialMigrationError) Error() string { return err.cause.Error() }
func (err *partialMigrationError) Unwrap() error { return err.cause }

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
	return newMigrationError(&partialMigrationError{cause: err}, failures)
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
	if job == nil {
		return nil
	}
	alreadyCopied := jobHasID(job.CopiedFileIDs, fileID)
	if !alreadyCopied {
		job.CopiedFileIDs = append(job.CopiedFileIDs, fileID)
	}
	job.UpdatedAt = time.Now().Unix()
	if err := e.saveJob(job); err != nil {
		if !alreadyCopied {
			job.CopiedFileIDs = removeJobID(job.CopiedFileIDs, fileID)
		}
		return err
	}
	e.emit(job)
	return nil
}

func sourceHash(file *model.File) string { return file.ContentHashName + ":" + file.ContentHash }

// markCompleted persists a resource that no longer needs any transfer or
// source-cleanup work. It is deliberately called for nested files too.
func (e *Engine) markCompleted(job *Job, fileID string) error {
	if job == nil || jobHasID(job.CompletedFileIDs, fileID) {
		return nil
	}
	job.CompletedFileIDs = append(job.CompletedFileIDs, fileID)
	job.CopiedFileIDs = removeJobID(job.CopiedFileIDs, fileID)
	job.UpdatedAt = time.Now().Unix()
	if err := e.saveJob(job); err != nil {
		return err
	}
	e.emit(job)
	return nil
}

func (e *Engine) markTargetDirectory(job *Job, sourceID, targetID string) error {
	if job == nil || strings.TrimSpace(sourceID) == "" || strings.TrimSpace(targetID) == "" {
		return errors.New("迁移目标目录记录缺少 ID")
	}
	if job.TargetDirectoryIDs == nil {
		job.TargetDirectoryIDs = make(map[string]string)
	}
	if job.TargetDirectoryIDs[sourceID] == targetID {
		return nil
	}
	job.TargetDirectoryIDs[sourceID] = targetID
	job.UpdatedAt = time.Now().Unix()
	if err := e.saveJob(job); err != nil {
		return err
	}
	e.emit(job)
	return nil
}

// downloadTo streams a download url into a writer.
func downloadTo(ctx context.Context, dl *model.DownloadURL, w io.Writer) error {
	return downloadToCounted(ctx, dl, w, nil)
}

// downloadToCounted is like downloadTo but accumulates the byte count into
// job.ProcessedBytes when job is non-nil.
func downloadToCounted(ctx context.Context, dl *model.DownloadURL, w io.Writer, job *Job, options ...dlengine.Options) error {
	if dl == nil || strings.TrimSpace(dl.URL) == "" {
		return errors.New("migrate: empty download url")
	}
	var opts dlengine.Options
	if len(options) > 0 {
		opts = options[0]
	}
	hc := dlengine.NewHTTPClient(opts)
	defer hc.CloseIdleConnections()
	req, err := dlengine.NewRequest(ctx, http.MethodGet, dl.URL, dlengine.Options{Headers: dl.Headers, RequestAuth: dl.RequestAuth})
	if err != nil {
		return err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// This path requests the entire file, without a Range or cache validator.
	// Partial, pending, and bodyless responses must never become target files.
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("migrate: http %d", resp.StatusCode)
	}
	var counter io.Writer = w
	if job != nil {
		counter = &countingWriter{w: w, job: job, emit: func() {}}
	}
	if opts.Limiter != nil {
		counter = &downloadLimitedWriter{ctx: ctx, writer: counter, limiter: opts.Limiter}
	}
	written, err := io.Copy(counter, resp.Body)
	if err == nil && dl.Size > 0 && written != dl.Size {
		return fmt.Errorf("migrate: 下载长度 %d 与文件大小 %d 不符", written, dl.Size)
	}
	return err
}

type downloadLimitedWriter struct {
	ctx     context.Context
	writer  io.Writer
	limiter dlengine.RateLimiter
}

func (writer *downloadLimitedWriter) Write(data []byte) (int, error) {
	if err := writer.limiter.Wait(writer.ctx, int64(len(data))); err != nil {
		return 0, err
	}
	return writer.writer.Write(data)
}

// countingWriter wraps a writer and increments job.ProcessedBytes.
type countingWriter struct {
	w    io.Writer
	job  *Job
	emit func()
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	if n > 0 {
		c.job.ProcessedBytes += int64(n)
	}
	return n, err
}

// completeBytes commits exactly one file's worth of progress after its target
// upload succeeds. Download strategies may have already reported incremental
// bytes, so assigning the final value avoids double-counting on success.
func completeBytes(job *Job, start, size int64) {
	if job == nil {
		return
	}
	if size < 0 {
		size = 0
	}
	job.ProcessedBytes = start + size
}
