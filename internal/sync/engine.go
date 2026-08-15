// Package sync implements two-way local ⇄ drive synchronization (config-driven).
package sync

import (
	"context"
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
	ID        string `json:"id"`
	Name      string `json:"name"`
	UserID    string `json:"user_id"`
	DriveID   string `json:"drive_id"`
	LocalDir  string `json:"local_dir"`
	RemoteDir string `json:"remote_dir"`
	Direction string `json:"direction"` // two-way | push | pull
	Enabled   bool   `json:"enabled"`
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

// Engine executes sync jobs.
type Engine struct {
	onProgress func(jobID string, done, total int)
}

// NewEngine creates the sync engine.
func NewEngine(onProgress func(jobID string, done, total int)) *Engine {
	return &Engine{onProgress: onProgress}
}

// Run executes one sync job.
func (e *Engine) Run(ctx context.Context, cfg Config) error {
	switch cfg.Direction {
	case "push":
		return e.push(ctx, cfg)
	case "pull":
		return e.pull(ctx, cfg)
	default:
		return e.twoWay(ctx, cfg)
	}
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

// TODO: delete propagation (remote→local and local→remote) is not yet
// implemented. It requires a persisted snapshot of the last sync state to
// distinguish "file was deleted on one side" from "never existed". This is
// intentionally out of scope for the minimal fix.

var _ = time.Now
var _ = strings.TrimSpace
