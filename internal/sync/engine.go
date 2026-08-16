// Package sync implements two-way local ⇄ drive synchronization (config-driven).
package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/transfer/dlengine"
)

// Config describes one sync job.
type Config struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	UserID            string `json:"user_id"`
	DriveID           string `json:"drive_id"`
	LocalDir          string `json:"local_dir"`
	RemoteDir         string `json:"remote_dir"`
	RemoteName        string `json:"remote_name"`
	Direction         string `json:"direction"` // two-way | push | pull
	Enabled           bool   `json:"enabled"`
	IntervalMin       int    `json:"intervalMin"`       // scheduler interval in minutes; <=0 means no scheduling
	DeletePropagation bool   `json:"deletePropagation"` // propagate deletions across sides
}

// Entry is one synced file record.
type Entry struct {
	LocalPath  string `json:"localPath"`
	RemoteID   string `json:"remoteId"`
	RemoteName string `json:"remoteName"`
	Size       int64  `json:"size"`
	ModTime    int64  `json:"modTime"`
	Hash       string `json:"hash,omitempty"`
	IsDir      bool   `json:"isDir"`
}

// SnapshotStore persists sync snapshots (last-sync file lists) so the engine
// can detect cross-side deletions on the next run. The store package provides
// a JSON-backed implementation; the interface keeps the engine testable.
type SnapshotStore interface {
	SaveSyncSnapshot(id string, entries []Entry) error
	LoadSyncSnapshot(id string) ([]Entry, error)
	ClearSyncSnapshot(id string) error
}

// LogFunc is the logger callback for key sync events (start/finish/conflict/delete).
type LogFunc func(jobID, event, detail string)

// Engine executes sync jobs.
type Engine struct {
	onProgress func(jobID string, done, total int)
	snapshots  SnapshotStore
	logger     LogFunc
}

// NewEngine creates the sync engine.
// snapshots and logger are optional (nil-safe); snapshots are required for
// delete propagation.
func NewEngine(onProgress func(jobID string, done, total int), opts ...func(*Engine)) *Engine {
	e := &Engine{onProgress: onProgress}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// WithSnapshotStore sets the snapshot store on the engine.
func WithSnapshotStore(s SnapshotStore) func(*Engine) {
	return func(e *Engine) { e.snapshots = s }
}

// WithLogger sets the logger callback on the engine.
func WithLogger(fn LogFunc) func(*Engine) {
	return func(e *Engine) { e.logger = fn }
}

// log is a nil-safe helper.
func (e *Engine) log(jobID, event, detail string) {
	if e.logger != nil {
		e.logger(jobID, event, detail)
	}
}

// Run executes one sync job.
func (e *Engine) Run(ctx context.Context, cfg Config) error {
	e.log(cfg.ID, "start", fmt.Sprintf("direction=%s", cfg.Direction))
	var err error
	switch cfg.Direction {
	case "push":
		err = e.push(ctx, cfg)
	case "pull":
		err = e.pull(ctx, cfg)
	default:
		err = e.twoWay(ctx, cfg)
	}
	if err != nil {
		e.log(cfg.ID, "error", err.Error())
		return err
	}
	e.log(cfg.ID, "complete", "ok")
	return nil
}

// remoteTree recursively lists all files under remoteDir, preserving the
// relative path (slash-joined) in RemoteName. It recurses into IsDir entries
// returned by drive.ListDir so that nested subdirectories are not skipped.
func remoteTree(cfg Config) ([]Entry, error) {
	var out []Entry
	var walk func(parentID, relPrefix string) error
	walk = func(parentID, relPrefix string) error {
		files, err := drive.ListDir(cfg.UserID, cfg.DriveID, parentID, nil)
		if err != nil {
			return err
		}
		for _, f := range files {
			rel := f.Name
			if relPrefix != "" {
				rel = relPrefix + "/" + f.Name
			}
			if f.IsDir {
				// recurse into subdirectory, carrying the relative path forward
				if err := walk(f.FileID, rel); err != nil {
					return err
				}
				continue
			}
			out = append(out, Entry{
				RemoteID:   f.FileID,
				RemoteName: rel,
				Size:       f.Size,
				ModTime:    f.Time,
				IsDir:      false,
			})
		}
		return nil
	}
	if err := walk(cfg.RemoteDir, ""); err != nil {
		return nil, err
	}
	return out, nil
}

// push uploads newer local files to the drive, preserving nested directory
// structure on the remote side.
func (e *Engine) push(ctx context.Context, cfg Config) error {
	remoteFiles, err := remoteTree(cfg)
	if err != nil {
		return err
	}
	remoteByName := map[string]Entry{}
	for _, r := range remoteFiles {
		remoteByName[r.RemoteName] = r
	}

	var local []Entry
	err = filepath.Walk(cfg.LocalDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(cfg.LocalDir, path)
		local = append(local, Entry{
			LocalPath:  path,
			RemoteName: filepath.ToSlash(rel),
			Size:       info.Size(),
			ModTime:    info.ModTime().Unix(),
			IsDir:      false,
		})
		return nil
	})
	if err != nil {
		return err
	}

	// find local files missing or newer than remote
	needUpload := []Entry{}
	for _, l := range local {
		r, found := remoteByName[l.RemoteName]
		if !found {
			needUpload = append(needUpload, l)
			continue
		}
		// upload if local is newer (ModTime) or size differs
		if l.ModTime > r.ModTime || l.Size != r.Size {
			needUpload = append(needUpload, l)
		}
	}

	total := len(needUpload)
	for i, l := range needUpload {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err := e.uploadFile(ctx, cfg, l); err != nil {
			return err
		}
		if e.onProgress != nil {
			e.onProgress(cfg.ID, i+1, total)
		}
	}

	// delete propagation: files in the snapshot but absent locally now →
	// remove them from the remote side.
	if cfg.DeletePropagation && e.snapshots != nil {
		snap, _ := e.snapshots.LoadSyncSnapshot(cfg.ID)
		localByName := map[string]Entry{}
		for _, l := range local {
			localByName[l.RemoteName] = l
		}
		var toDelete []Entry
		for _, s := range snap {
			if _, exists := localByName[s.RemoteName]; !exists {
				toDelete = append(toDelete, s)
			}
		}
		if len(toDelete) > 0 {
			if ok := e.guardDelete(cfg.ID, len(toDelete), len(snap)); !ok {
				// safety threshold exceeded — skip deletions
			} else {
				e.propagateRemoteDeletes(ctx, cfg, toDelete)
			}
		}
	}

	// persist the new snapshot of local files for next run's delete detection
	if e.snapshots != nil {
		_ = e.snapshots.SaveSyncSnapshot(cfg.ID, local)
	}
	return nil
}

// ensureRemoteDir walks the slash-relative path components and creates each
// directory level on the remote drive as needed, returning the leaf FileID.
func ensureRemoteDir(cfg Config, relDir string) (string, error) {
	parentID := cfg.RemoteDir
	if relDir == "" || relDir == "." {
		return parentID, nil
	}
	parts := strings.Split(filepath.ToSlash(relDir), "/")
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		// check if it already exists under parentID
		existing, err := drive.ListDir(cfg.UserID, cfg.DriveID, parentID, nil)
		if err != nil {
			return "", err
		}
		found := ""
		for _, f := range existing {
			if f.IsDir && f.Name == part {
				found = f.FileID
				break
			}
		}
		if found != "" {
			parentID = found
			continue
		}
		res, err := drive.Mkdir(cfg.UserID, cfg.DriveID, parentID, part)
		if err != nil {
			return "", err
		}
		if res.Error != "" {
			return "", &driveError{op: "Mkdir", msg: res.Error}
		}
		parentID = res.FileID
	}
	return parentID, nil
}

type driveError struct {
	op  string
	msg string
}

func (e *driveError) Error() string { return e.op + ": " + e.msg }

func (e *Engine) uploadFile(ctx context.Context, cfg Config, entry Entry) error {
	// resolve/create remote parent directory so nested paths are preserved
	relDir := filepath.Dir(entry.RemoteName)
	parentID, err := ensureRemoteDir(cfg, relDir)
	if err != nil {
		return err
	}
	ui := &model.UploadingUI{
		UploadID: entry.LocalPath,
		Info: model.UploadInfo{
			LocalFilePath: entry.LocalPath,
			ParentFileID:  parentID,
			DriveID:       cfg.DriveID,
			Name:          filepath.Base(entry.LocalPath),
			Size:          entry.Size,
		},
	}
	handler, err := drive.QueueUploadHandler(cfg.UserID, cfg.DriveID)
	if err != nil {
		return err
	}
	return handler(ctx, ui)
}

// pull downloads remote files missing or changed locally, recursing into
// remote subdirectories and preserving relative path structure locally.
func (e *Engine) pull(ctx context.Context, cfg Config) error {
	remoteFiles, err := remoteTree(cfg)
	if err != nil {
		return err
	}
	remoteByName := map[string]Entry{}
	for _, r := range remoteFiles {
		remoteByName[r.RemoteName] = r
	}
	total := len(remoteFiles)
	for i, f := range remoteFiles {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		// preserve relative path structure locally
		localPath := filepath.Join(cfg.LocalDir, filepath.FromSlash(f.RemoteName))
		localModTime, localSize := int64(0), int64(-1)
		if info, statErr := os.Stat(localPath); statErr == nil {
			localModTime = info.ModTime().Unix()
			localSize = info.Size()
		}
		// only download if remote is newer or size differs
		if f.ModTime <= localModTime && f.Size == localSize {
			continue
		}
		u, err := drive.GetDownloadURL(cfg.UserID, cfg.DriveID, f.RemoteID, 14400)
		if err != nil {
			continue
		}
		// ensure local parent directory exists
		if mkErr := os.MkdirAll(filepath.Dir(localPath), 0o755); mkErr != nil {
			continue
		}
		if err := downloadTo(ctx, u, localPath); err != nil {
			continue
		}
		if e.onProgress != nil {
			e.onProgress(cfg.ID, i+1, total)
		}
	}

	// delete propagation: files in the snapshot but absent on the remote now →
	// remove them locally.
	if cfg.DeletePropagation && e.snapshots != nil {
		snap, _ := e.snapshots.LoadSyncSnapshot(cfg.ID)
		var toDelete []Entry
		for _, s := range snap {
			if _, exists := remoteByName[s.RemoteName]; !exists {
				toDelete = append(toDelete, s)
			}
		}
		if len(toDelete) > 0 {
			if ok := e.guardDelete(cfg.ID, len(toDelete), len(snap)); !ok {
				// safety threshold exceeded — skip deletions
			} else {
				e.propagateLocalDeletes(cfg, toDelete)
			}
		}
	}

	// persist the new snapshot of remote files for next run's delete detection
	if e.snapshots != nil {
		_ = e.snapshots.SaveSyncSnapshot(cfg.ID, remoteFiles)
	}
	return nil
}

// twoWay is a conservative mirror: upload new local, download missing remote.
func (e *Engine) twoWay(ctx context.Context, cfg Config) error {
	if err := e.push(ctx, cfg); err != nil {
		return err
	}
	return e.pull(ctx, cfg)
}

// downloadTo streams a download url to a local file via the segmented engine.
// It accepts the caller's context so downloads can be cancelled reliably.
func downloadTo(ctx context.Context, u *model.DownloadURL, path string) error {
	opts := dlengine.Options{}
	if u.ForceLocalProxy || u.DownloadMode == "proxy" {
		opts.Concurrency = 1
	}
	if u.Headers != nil {
		opts.Headers = u.Headers
	}
	// single-stream download for sync simplicity
	opts.Concurrency = 1
	return dlengine.Download(ctx, opts, u.URL, path, nil)
}

// guardDelete enforces the safety threshold: if the number of files to
// delete exceeds 50% of the snapshot total, deletions are cancelled to
// prevent catastrophic data loss from a transient listing failure.
func (e *Engine) guardDelete(jobID string, deleteCount, snapTotal int) bool {
	if snapTotal == 0 {
		return false
	}
	ratio := float64(deleteCount) / float64(snapTotal)
	if ratio > 0.5 {
		e.log(jobID, "delete_cancelled", fmt.Sprintf("delete count %d exceeds 50%% of snapshot %d", deleteCount, snapTotal))
		return false
	}
	return true
}

// propagateRemoteDeletes trashes/deletes the given remote files that no
// longer exist locally. It uses the drive trash batch for safety.
func (e *Engine) propagateRemoteDeletes(_ context.Context, cfg Config, toDelete []Entry) {
	ids := make([]string, 0, len(toDelete))
	names := make([]string, 0, len(toDelete))
	for _, d := range toDelete {
		if d.RemoteID != "" {
			ids = append(ids, d.RemoteID)
			names = append(names, d.RemoteName)
		}
	}
	if len(ids) == 0 {
		return
	}
	_, err := drive.TrashBatch(cfg.UserID, cfg.DriveID, ids)
	if err != nil {
		e.log(cfg.ID, "delete_error", fmt.Sprintf("remote trash failed: %v", err))
		return
	}
	e.log(cfg.ID, "delete", fmt.Sprintf("removed remote files: %v", names))
}

// propagateLocalDeletes removes local files that no longer exist on the
// remote side.
func (e *Engine) propagateLocalDeletes(cfg Config, toDelete []Entry) {
	names := make([]string, 0, len(toDelete))
	for _, d := range toDelete {
		localPath := filepath.Join(cfg.LocalDir, filepath.FromSlash(d.RemoteName))
		if err := os.Remove(localPath); err != nil {
			continue
		}
		names = append(names, d.RemoteName)
	}
	if len(names) > 0 {
		e.log(cfg.ID, "delete", fmt.Sprintf("removed local files: %v", names))
	}
}

// StartScheduler launches a background goroutine that periodically runs all
// enabled sync jobs at the given IntervalMin interval. It blocks until stop
// is closed. When IntervalMin <= 0 the scheduler is a no-op and returns
// immediately.
func (e *Engine) StartScheduler(stop <-chan struct{}, configs func() ([]Config, error)) {
	// Determine the minimum non-zero interval across all enabled configs; if
	// none qualify the scheduler does nothing.
	all, err := configs()
	if err != nil {
		return
	}
	minInterval := 0
	for _, c := range all {
		if c.Enabled && c.IntervalMin > 0 {
			if minInterval == 0 || c.IntervalMin < minInterval {
				minInterval = c.IntervalMin
			}
		}
	}
	if minInterval <= 0 {
		return
	}

	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ticker := time.NewTicker(time.Duration(minInterval) * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				cancel()
				return
			case <-ticker.C:
				runList, err := configs()
				if err != nil {
					continue
				}
				for _, c := range runList {
					if !c.Enabled || c.IntervalMin <= 0 {
						continue
					}
					_ = e.Run(ctx, c)
				}
			}
		}
	}()
}

var _ = time.Now
var _ = strings.TrimSpace
