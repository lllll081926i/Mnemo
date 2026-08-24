package transfer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/logging"
	"mnemo-go/internal/model"
)

// AddFiles enqueues local files/dirs into a parent folder.
func (q *UploadQueue) AddFiles(userID, driveID, parentID, conflictPolicy string, localPaths []string) []*model.UploadingUI {
	var created []*model.UploadingUI
	var directories []string
	for _, p := range localPaths {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if info.IsDir() {
			directories = append(directories, p)
		} else {
			if j := q.enqueue(userID, driveID, parentID, conflictPolicy, p, info.Name(), info.Size()); j != nil {
				created = append(created, j)
			}
		}
	}
	if len(directories) > 0 {
		// Directory enumeration may involve hundreds of thousands of entries or
		// a slow network-mounted source. Keep it off the Wails RPC path and emit
		// each discovered upload through the queue's existing task event.
		q.startDirectoryScan(userID, driveID, parentID, conflictPolicy, directories)
	}
	return created
}

func (q *UploadQueue) startDirectoryScan(userID, driveID, parentID, conflictPolicy string, directories []string) {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	q.workerWG.Add(1)
	q.mu.Unlock()
	go func() {
		defer q.workerWG.Done()
		q.scanAndEnqueueDirectories(userID, driveID, parentID, conflictPolicy, directories)
	}()
}

func (q *UploadQueue) scanAndEnqueueDirectories(userID, driveID, parentID, conflictPolicy string, directories []string) {
	for _, root := range directories {
		err := filepath.Walk(root, func(path string, fi os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			select {
			case <-q.ctx.Done():
				return q.ctx.Err()
			default:
			}
			if fi == nil || fi.IsDir() {
				return nil
			}
			rel, relErr := filepath.Rel(filepath.Dir(root), path)
			if relErr != nil {
				return relErr
			}
			name := strings.ReplaceAll(rel, "\\", "/")
			q.enqueue(userID, driveID, parentID, conflictPolicy, path, name, fi.Size())
			return nil
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			logging.Warn("upload directory scan failed", "directory", filepath.Base(root), "error", err)
		}
	}
}

func (q *UploadQueue) enqueue(userID, driveID, parentID, conflictPolicy, localPath, name string, size int64) *model.UploadingUI {
	if q.ctx.Err() != nil {
		return nil
	}
	relative := normalizeUploadPath(name)
	if relative == "" {
		relative = normalizeUploadPath(filepath.Base(localPath))
	}
	j := &model.UploadingUI{
		UploadID: newID("up"),
		UserID:   userID,
		Info: model.UploadInfo{
			LocalFilePath: localPath, ParentFileID: parentID,
			DriveID: driveID, Path: relative, Name: uploadLeaf(relative), Size: size,
			SizeStr:        model.FormatBytes(size),
			IsDir:          false,
			ConflictPolicy: conflictPolicy,
		},
		Upload: model.UploadState{
			DownState: "queued", DownTime: time.Now().Unix(), DownSize: 0,
		},
	}
	q.update(j)
	if err := q.startUpload(j.UploadID); err != nil {
		logging.Warn("upload worker could not start", "job_id", j.UploadID, "error", err)
	}
	return j
}

// normalizeUploadPath keeps the relative path captured by a directory walk
// portable and rejects traversal segments before they can become remote
// directory names.
func normalizeUploadPath(raw string) string {
	raw = strings.ReplaceAll(strings.TrimSpace(raw), "\\", "/")
	parts := strings.Split(strings.Trim(raw, "/"), "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			return ""
		}
		out = append(out, part)
	}
	return strings.Join(out, "/")
}

func uploadLeaf(relative string) string {
	if i := strings.LastIndex(relative, "/"); i >= 0 {
		return relative[i+1:]
	}
	return relative
}

// ensureRemoteParent creates or reuses every directory in a walked local
// path. The cache is scoped by account, drive and initial remote parent, so
// switching accounts or mounted drives cannot reuse another account's ids.
func (q *UploadQueue) ensureRemoteParent(ctx context.Context, userID, driveID, baseParent, relative string) (string, error) {
	relative = normalizeUploadPath(relative)
	parts := strings.Split(relative, "/")
	if relative == "" || len(parts) <= 1 {
		return baseParent, nil
	}

	q.dirMu.Lock()
	defer q.dirMu.Unlock()
	current := baseParent
	for _, segment := range parts[:len(parts)-1] {
		key := strings.Join([]string{userID, driveID, baseParent, current, segment}, "\x00")
		if id := q.dirIDs[key]; id != "" {
			current = id
			continue
		}

		items, err := drive.ListDirContext(ctx, userID, driveID, current, nil)
		if err != nil {
			return "", fmt.Errorf("查找远端目录 %q 失败: %w", segment, err)
		}
		found := ""
		for _, item := range items {
			if item.IsDir && item.Name == segment {
				found = item.FileID
				break
			}
		}
		if found == "" {
			result, mkdirErr := drive.MkdirContext(ctx, userID, driveID, current, segment)
			if mkdirErr != nil {
				return "", fmt.Errorf("创建远端目录 %q 失败: %w", segment, mkdirErr)
			}
			if result == nil || result.FileID == "" {
				if result != nil && result.Error != "" {
					return "", fmt.Errorf("创建远端目录 %q 失败: %s", segment, result.Error)
				}
				return "", fmt.Errorf("创建远端目录 %q 未返回目录 id", segment)
			}
			found = result.FileID
		}
		if len(q.dirIDs) >= maxUploadDirectoryCacheEntries {
			q.dirIDs = map[string]string{}
		}
		q.dirIDs[key] = found
		current = found
	}
	return current, nil
}
