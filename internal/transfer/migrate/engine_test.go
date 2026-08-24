package migrate

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/store"
)

func TestDownloadToIncludesUpstreamDetailAndRequestID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Dropbox-Request-Id", "dropbox-request-42")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error_summary":"path/not_found/..."}`))
	}))
	defer server.Close()

	err := downloadTo(context.Background(), &model.DownloadURL{URL: server.URL}, io.Discard)
	if err == nil {
		t.Fatal("downloadTo() succeeded, want upstream error")
	}
	message := err.Error()
	if !strings.Contains(message, "download http 400") || !strings.Contains(message, "path/not_found") || !strings.Contains(message, "dropbox-request-42") {
		t.Fatalf("download error lost upstream diagnostics: %q", message)
	}
}

func TestMigrationDownloadAuthFailuresAreTheOnlyRefreshableHTTPFailures(t *testing.T) {
	for _, testCase := range []struct {
		status int
		want   bool
	}{
		{status: http.StatusUnauthorized, want: true},
		{status: http.StatusForbidden, want: true},
		{status: http.StatusBadRequest, want: false},
		{status: http.StatusInternalServerError, want: false},
	} {
		err := &migrationDownloadHTTPError{statusCode: testCase.status}
		if got := retryableMigrationDownloadAuthFailure(err); got != testCase.want {
			t.Fatalf("retryableMigrationDownloadAuthFailure(%d) = %v, want %v", testCase.status, got, testCase.want)
		}
	}
}

func TestMigrationSpoolSizeUsesActualFileSizeAndRejectsMismatch(t *testing.T) {
	path := t.TempDir() + "/spool.bin"
	if err := os.WriteFile(path, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	if got, err := migrationSpoolSize(file, 0); err != nil || got != int64(len("payload")) {
		t.Fatalf("migrationSpoolSize(unknown) = %d, %v", got, err)
	}
	if _, err := migrationSpoolSize(file, 1); err == nil || !strings.Contains(err.Error(), "size mismatch") {
		t.Fatalf("migrationSpoolSize(mismatch) error = %v", err)
	}
}

func TestCommonHashMethod(t *testing.T) {
	cases := []struct {
		name string
		src  []string
		dst  []string
		want string
	}{
		{"both md5", []string{"md5"}, []string{"md5"}, "md5"},
		{"both sha1", []string{"sha1"}, []string{"sha1"}, "sha1"},
		{"no match", []string{"md5"}, []string{"sha1"}, ""},
		{"multi match", []string{"md5", "sha1"}, []string{"sha1", "md5"}, "md5"},
		{"canonical trim and case", []string{" SHA256 "}, []string{"sha256"}, "sha256"},
		{"ignore blank methods", []string{" ", " MD5"}, []string{"", "md5 "}, "md5"},
		{"empty src", []string{}, []string{"md5"}, ""},
	}
	for _, c := range cases {
		got := commonHashMethod(c.src, c.dst)
		if got != c.want {
			t.Errorf("%s: commonHashMethod(%v,%v) = %q, want %q", c.name, c.src, c.dst, got, c.want)
		}
	}
}

func TestRapidFallbackClassification(t *testing.T) {
	definiteMiss := newRapidFallbackError(errors.New("explicit miss"))
	if !canFallbackAfterRapid(definiteMiss) {
		t.Fatal("explicit miss must permit full-upload fallback")
	}
	if canFallbackAfterRapid(errors.New("target response lost")) {
		t.Fatal("ambiguous target error must not permit full-upload fallback")
	}
	if canFallbackAfterRapid(context.Canceled) || !isContextError(context.Canceled) || !isContextError(context.DeadlineExceeded) {
		t.Fatal("context cancellation/deadline must propagate instead of falling back")
	}
}

func TestRapidAndStreamSetupPropagateCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	e := NewEngine(nil, nil)
	if attempted, err := e.tryRapidMigrate(ctx, nil, nil, ""); !attempted || !errors.Is(err, context.Canceled) {
		t.Fatalf("tryRapidMigrate canceled = (%v, %v), want attempted context.Canceled", attempted, err)
	}
	job := &Job{DstUser: "webdav:test", DstDrive: model.ProviderWebdav}
	if attempted, err := e.tryStreamMigrate(ctx, job, &model.File{Name: "file.bin"}, "/"); !attempted || !errors.Is(err, context.Canceled) {
		t.Fatalf("tryStreamMigrate canceled setup = (%v, %v), want attempted context.Canceled", attempted, err)
	}
}

func TestPipeMigrationClosesEarlyReturningUploader(t *testing.T) {
	done := make(chan struct{})
	var uploadErr, downloadErr error
	go func() {
		defer close(done)
		uploadErr, downloadErr = pipeMigration(func(w io.Writer) error {
			_, err := io.CopyN(w, strings.NewReader(strings.Repeat("x", 4096)), 4096)
			return err
		}, func(io.Reader) error {
			return nil
		})
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pipeMigration deadlocked after uploader returned early")
	}
	if uploadErr != nil {
		t.Fatalf("upload error = %v, want nil", uploadErr)
	}
	if !errors.Is(downloadErr, io.ErrClosedPipe) {
		t.Fatalf("download error = %v, want io.ErrClosedPipe", downloadErr)
	}
}

func TestCheckpointPersistenceFailureRollsBackInMemoryState(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "migrate_jobs.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	e := NewEngine(st, nil)
	job := &Job{ID: "persist-failure"}
	if err := e.markCopied(job, "source-file"); err == nil {
		t.Fatal("markCopied succeeded despite persistence failure")
	}
	if len(job.CopiedFileIDs) != 0 {
		t.Fatalf("failed copied checkpoint leaked into memory: %#v", job.CopiedFileIDs)
	}
	if err := e.markTargetDirectory(job, "source-dir", "target-dir"); err == nil {
		t.Fatal("markTargetDirectory succeeded despite persistence failure")
	}
	if got := job.TargetDirectoryIDs["source-dir"]; got != "" {
		t.Fatalf("failed directory checkpoint leaked into memory: %q", got)
	}
	moveJob := &Job{ID: "move-persist-failure", Move: true}
	err = e.completeResource(context.Background(), moveJob, &model.File{FileID: "source-file", Name: "source.bin"})
	if err == nil || !strings.Contains(err.Error(), "persist copied checkpoint") {
		t.Fatalf("completeResource persistence error = %v", err)
	}
	if len(moveJob.CopiedFileIDs) != 0 {
		t.Fatalf("move advanced past failed durable checkpoint: %#v", moveJob.CopiedFileIDs)
	}
}

func TestRapidUploadAllowedExcludesYike(t *testing.T) {
	if rapidUploadAllowed(model.ProviderYike, "pan123") {
		t.Fatal("yike must not be used as a rapid-upload source")
	}
	if rapidUploadAllowed("pan123", model.ProviderYike) {
		t.Fatal("yike must not be used as a rapid-upload target")
	}
	if !rapidUploadAllowed("pan123", "pan189") {
		t.Fatal("compatible providers should remain eligible")
	}
}

// TestSameProviderDifferentAccountsUseSeparateContexts verifies the migration
// path that is easy to regress when source and destination share one provider
// id: the source fingerprint must be read with account A, while the rapid
// upload request must be submitted with account B. They must not be treated as
// an in-account move/copy merely because both endpoints are PikPak.
func TestSameProviderDifferentAccountsUseSeparateContexts(t *testing.T) {
	registerSameProviderRapidTestDriver()
	recorder := &sameProviderRapidRecorder{}
	sameProviderRapidTestRecorder = recorder
	t.Cleanup(func() { sameProviderRapidTestRecorder = nil })

	const (
		sourceUser  = "pikpak_rapid-source"
		sourceDrive = "pikpak:rapid-source-drive"
		targetUser  = "pikpak_rapid-target"
		targetDrive = "pikpak:rapid-target-drive"
	)
	drive.SetTokenResolver(func(userID, driveID string) (*model.TokenInfo, error) {
		switch {
		case userID == sourceUser && driveID == sourceDrive:
			return &model.TokenInfo{TokenFrom: model.ProviderPikpak, AccessToken: "source-session"}, nil
		case userID == targetUser && driveID == targetDrive:
			return &model.TokenInfo{TokenFrom: model.ProviderPikpak, AccessToken: "target-session"}, nil
		default:
			return nil, errors.New("unexpected account context")
		}
	})
	t.Cleanup(func() { drive.SetTokenResolver(nil) })

	job := &Job{
		SrcUser: sourceUser, SrcDrive: sourceDrive,
		DstUser: targetUser, DstDrive: targetDrive,
	}
	attempted, err := NewEngine(nil, nil).tryRapidMigrate(context.Background(), job, &model.File{
		FileID: "source-file", Name: "same-provider.bin", Size: 42,
	}, "target-parent")
	if err != nil || !attempted {
		t.Fatalf("tryRapidMigrate() = (%v, %v), want successful rapid attempt", attempted, err)
	}
	if recorder.sourceUser != sourceUser || recorder.sourceDrive != sourceDrive || recorder.sourceToken != "source-session" {
		t.Fatalf("source hash context = user=%q drive=%q token=%q, want source account", recorder.sourceUser, recorder.sourceDrive, recorder.sourceToken)
	}
	if recorder.targetUser != targetUser || recorder.targetDrive != targetDrive || recorder.targetToken != "target-session" {
		t.Fatalf("rapid upload context = user=%q drive=%q token=%q, want target account", recorder.targetUser, recorder.targetDrive, recorder.targetToken)
	}
	if recorder.method != "gcid" || recorder.hash != "0123456789ABCDEF0123456789ABCDEF01234567" || recorder.parentID != "target-parent" || recorder.fileName != "same-provider.bin" || recorder.size != 42 {
		t.Fatalf("rapid request = %+v, want source hash + target parent/name/size", recorder)
	}
}

func TestEngineCancelUnknown(t *testing.T) {
	e := NewEngine(nil, nil)
	// should not panic on unknown id
	e.Cancel("nonexistent")
}

func TestEngineRunEmptyFileIDs(t *testing.T) {
	e := NewEngine(nil, nil)
	job := &Job{ID: "t1", FileIDs: []string{}}
	err := e.Run(context.Background(), job)
	if err != nil {
		t.Errorf("Run with empty FileIDs: %v", err)
	}
	if job.Status != "completed" {
		t.Errorf("expected completed, got %s", job.Status)
	}
}

func TestEngineRunSkipsPersistedTopLevelCheckpoints(t *testing.T) {
	e := NewEngine(nil, nil)
	job := &Job{
		ID:               "resume-complete",
		FileIDs:          []string{"already-done"},
		CompletedFileIDs: []string{"already-done", "nested-file"},
		Status:           "canceled",
	}
	if err := e.Run(context.Background(), job); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if job.Status != "completed" {
		t.Fatalf("status = %q, want completed", job.Status)
	}
	if job.Processed != 1 || job.Total != 1 {
		t.Fatalf("processed/total = %d/%d, want 1/1", job.Processed, job.Total)
	}
}

func TestRecoveryCheckpointsAreIdempotent(t *testing.T) {
	e := NewEngine(nil, nil)
	job := &Job{ID: "checkpoint"}

	if err := e.markTargetDirectory(job, "source-dir", "target-dir"); err != nil {
		t.Fatal(err)
	}
	if err := e.markTargetDirectory(job, "source-dir", "target-dir"); err != nil {
		t.Fatal(err)
	}
	if err := e.markCopied(job, "source-file"); err != nil {
		t.Fatal(err)
	}
	if err := e.markCopied(job, "source-file"); err != nil {
		t.Fatal(err)
	}
	if err := e.markCompleted(job, "source-file"); err != nil {
		t.Fatal(err)
	}
	if err := e.markCompleted(job, "source-file"); err != nil {
		t.Fatal(err)
	}

	if got := job.TargetDirectoryIDs["source-dir"]; got != "target-dir" {
		t.Fatalf("target directory checkpoint = %q, want target-dir", got)
	}
	if len(job.CompletedFileIDs) != 1 || job.CompletedFileIDs[0] != "source-file" {
		t.Fatalf("completed checkpoints = %#v", job.CompletedFileIDs)
	}
	if len(job.CopiedFileIDs) != 0 {
		t.Fatalf("copied checkpoint must be cleared after source cleanup: %#v", job.CopiedFileIDs)
	}
}

func TestCompletedTopLevelCountIgnoresNestedCheckpoints(t *testing.T) {
	job := &Job{
		FileIDs:          []string{"root-a", "root-b"},
		CompletedFileIDs: []string{"root-a", "nested-a", "nested-b"},
	}
	if got := completedTopLevelCount(job); got != 1 {
		t.Fatalf("completed top-level count = %d, want 1", got)
	}
}

func TestEngineRejectsDuplicateActiveRun(t *testing.T) {
	e := NewEngine(nil, nil)
	_, cancel, registered := e.registerCancel(context.Background(), "active")
	if !registered {
		t.Fatal("first registration must succeed")
	}
	defer func() {
		cancel()
		e.releaseCancel("active")
	}()

	job := &Job{ID: "active"}
	if err := e.Run(context.Background(), job); err == nil {
		t.Fatal("duplicate active run must be rejected")
	}
	if job.Status != "" {
		t.Fatalf("duplicate run changed status to %q", job.Status)
	}
}

func TestValidateEndpointsRejectsSameDrive(t *testing.T) {
	if err := ValidateEndpoints("pikpak_user", "pikpak:drive", "pikpak_user", "pikpak:drive"); err == nil {
		t.Fatal("same source and target drive must be rejected")
	}
	if err := ValidateEndpoints("pikpak_user", "pikpak:drive", "pikpak_other", "pikpak:drive"); err != nil {
		t.Fatalf("different accounts should remain valid: %v", err)
	}
}

func TestEngineRunRejectsSameDriveBeforeChangingState(t *testing.T) {
	e := NewEngine(nil, nil)
	job := &Job{
		ID: "same-drive", SrcUser: "user", SrcDrive: "drive",
		DstUser: "user", DstDrive: "drive", FileIDs: []string{"file"},
	}
	if err := e.Run(context.Background(), job); err == nil {
		t.Fatal("same-drive migration should fail before provider access")
	}
	if job.Status != "" {
		t.Fatalf("job status changed after validation failure: %q", job.Status)
	}
}

func TestJobProcessedBytesTracking(t *testing.T) {
	j := &Job{ID: "t1", Total: 1000}
	j.ProcessedBytes = 500
	if j.ProcessedBytes != 500 {
		t.Error("ProcessedBytes not set")
	}
	j.Failed = 2
	if j.Failed != 2 {
		t.Error("Failed not set")
	}
}

func TestPartialMigrationErrorUnwrapsThroughMigrationError(t *testing.T) {
	err := newMigrationError(partialError("source cleanup failed"), 1)
	if !isPartialError(err) {
		t.Fatal("partial cleanup errors must remain identifiable after wrapping")
	}
	if failureCount(err) != 1 {
		t.Fatalf("failure count = %d, want 1", failureCount(err))
	}
}

func TestPartialDirectoryCopyCannotBeReportedAsTotalFailure(t *testing.T) {
	err := newPartialMigrationError(context.DeadlineExceeded, 3)
	if !isPartialError(err) {
		t.Fatal("a created target directory with failed children must be partial")
	}
	if failureCount(err) != 3 {
		t.Fatalf("failure count = %d, want 3", failureCount(err))
	}
	if got := migrationResultStatus(3, 0, isPartialError(err)); got != "partial" {
		t.Fatalf("migration status = %q, want partial", got)
	}
	if got := migrationResultStatus(1, 0, false); got != "failed" {
		t.Fatalf("migration status without copied output = %q, want failed", got)
	}
}

var sameProviderRapidTestRecorder *sameProviderRapidRecorder

func registerSameProviderRapidTestDriver() {
	if drive.IsRegistered(model.ProviderPikpak) {
		return
	}
	drive.Register(drive.Registration{
		ID:   model.ProviderPikpak,
		Meta: drive.GetMeta(model.ProviderPikpak),
		Caps: drive.NewCapabilities(model.ProviderPikpak, nil, func(caps *drive.Capabilities) {
			caps.SetHashes([]string{"gcid"}, []string{"gcid"})
		}),
		Factory: func() drive.Driver { return &sameProviderRapidDriver{recorder: sameProviderRapidTestRecorder} },
	})
}

type sameProviderRapidRecorder struct {
	sourceUser  string
	sourceDrive string
	sourceToken string
	targetUser  string
	targetDrive string
	targetToken string
	method      string
	hash        string
	parentID    string
	fileName    string
	size        int64
}

type sameProviderRapidDriver struct {
	drive.BaseDriver
	recorder *sameProviderRapidRecorder
}

func (d *sameProviderRapidDriver) ID() string       { return model.ProviderPikpak }
func (d *sameProviderRapidDriver) Meta() drive.Meta { return drive.GetMeta(model.ProviderPikpak) }
func (d *sameProviderRapidDriver) Capabilities() drive.Capabilities {
	return drive.RegistryCaps(model.ProviderPikpak)
}
func (d *sameProviderRapidDriver) RootID() string { return "root" }
func (d *sameProviderRapidDriver) List(context.Context, drive.Context, string, *drive.ListOptions) ([]model.File, error) {
	return nil, nil
}
func (d *sameProviderRapidDriver) Mkdir(context.Context, drive.Context, string, string) (*drive.MkdirResult, error) {
	return nil, drive.ErrNotImplemented
}
func (d *sameProviderRapidDriver) Rename(context.Context, drive.Context, string, string) (*drive.RenameResult, error) {
	return nil, drive.ErrNotImplemented
}
func (d *sameProviderRapidDriver) Delete(context.Context, drive.Context, []drive.FileRef) ([]string, error) {
	return nil, drive.ErrNotImplemented
}
func (d *sameProviderRapidDriver) Move(context.Context, drive.Context, []drive.FileRef, string, string) ([]string, error) {
	return nil, drive.ErrNotImplemented
}
func (d *sameProviderRapidDriver) Copy(context.Context, drive.Context, []drive.FileRef, string, string) ([]string, error) {
	return nil, drive.ErrNotImplemented
}

func (d *sameProviderRapidDriver) ResolveTransferHash(_ context.Context, c drive.Context, _ string, method string, _ bool) (string, error) {
	if d.recorder == nil {
		return "", errors.New("rapid test recorder is not initialized")
	}
	d.recorder.sourceUser = c.UserID
	d.recorder.sourceDrive = c.DriveID
	if c.Token != nil {
		d.recorder.sourceToken = c.Token.AccessToken
	}
	if method != "gcid" {
		return "", errors.New("unexpected transfer hash method")
	}
	return "0123456789ABCDEF0123456789ABCDEF01234567", nil
}

func (d *sameProviderRapidDriver) RapidUploadByHash(_ context.Context, c drive.Context, req drive.RapidUploadRequest) (*drive.RapidUploadResult, error) {
	if d.recorder == nil {
		return nil, errors.New("rapid test recorder is not initialized")
	}
	d.recorder.targetUser = c.UserID
	d.recorder.targetDrive = c.DriveID
	if c.Token != nil {
		d.recorder.targetToken = c.Token.AccessToken
	}
	d.recorder.method = req.Method
	d.recorder.hash = req.Hash
	d.recorder.parentID = req.ParentID
	d.recorder.fileName = req.FileName
	d.recorder.size = req.Size
	return &drive.RapidUploadResult{Reuse: true, FileID: "target-file", ParentID: req.ParentID}, nil
}
