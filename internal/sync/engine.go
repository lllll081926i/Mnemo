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

// push uploads newer local files to the drive.
func (e *Engine) push(ctx context.Context, cfg Config) error {
	remoteFiles, err := drive.ListDir(cfg.UserID, cfg.DriveID, cfg.RemoteDir, nil)
	if err != nil {
		return err
	}
	remoteByID := map[string]model.File{}
	for _, f := range remoteFiles {
		remoteByID[f.FileID] = f
	}
	var local []Entry
	err = filepath.Walk(cfg.LocalDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(cfg.LocalDir, path)
		local = append(local, Entry{LocalPath: path, RemoteName: filepath.ToSlash(rel), Size: info.Size(), ModTime: info.ModTime().Unix(), IsDir: false})
		return nil
	})
	if err != nil {
		return err
	}
	// find local files missing or newer on remote
	needUpload := []Entry{}
	for _, l := range local {
		found := false
		for _, r := range remoteFiles {
			if r.Name == filepath.Base(l.RemoteName) && r.Size == l.Size {
				found = true
				break
			}
		}
		if !found {
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

func (e *Engine) uploadFile(ctx context.Context, cfg Config, entry Entry) error {
	// build an upload UI job and run through the provider
	ui := &model.UploadingUI{
		UploadID: entry.LocalPath,
		Info: model.UploadInfo{
			LocalFilePath: entry.LocalPath,
			ParentFileID:  cfg.RemoteDir,
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

// pull downloads remote files missing/changed locally.
func (e *Engine) pull(ctx context.Context, cfg Config) error {
	remoteFiles, err := drive.ListDir(cfg.UserID, cfg.DriveID, cfg.RemoteDir, nil)
	if err != nil {
		return err
	}
	total := len(remoteFiles)
	for i, f := range remoteFiles {
		if f.IsDir {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		localPath := filepath.Join(cfg.LocalDir, f.Name)
		if _, err := os.Stat(localPath); err == nil {
			continue // exists
		}
		u, err := drive.GetDownloadURL(cfg.UserID, cfg.DriveID, f.FileID, 14400)
		if err != nil {
			continue
		}
		if err := downloadTo(u, localPath); err != nil {
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
func downloadTo(u *model.DownloadURL, path string) error {
	opts := dlengine.Options{}
	if u.ForceLocalProxy || u.DownloadMode == "proxy" {
		opts.Concurrency = 1
	}
	if u.Headers != nil {
		opts.Headers = u.Headers
	}
	// single-stream download for sync simplicity
	opts.Concurrency = 1
	return dlengine.Download(context.Background(), opts, u.URL, path, nil)
}

var _ = time.Now
var _ = strings.TrimSpace
