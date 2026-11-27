package sync

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"mnemo-go/internal/drive"
)

// collapseTrackedRenames pairs unambiguous same-directory delete + create actions
// when both sides have matching non-empty content hash and identical file size.
// Using strict hash + size + same parent directory guarantees zero false positives
// and avoids re-uploading or re-downloading renamed files, while exempting true
// renames from tripping the mass-deletion safety guard.
func collapseTrackedRenames(cfg Config, changes []Change, caps drive.Capabilities) []Change {
	if len(changes) < 2 {
		return changes
	}
	type renameKey struct {
		dir  string
		size int64
		hash string
	}
	makeKey := func(p string, size int64, hash string) (renameKey, bool) {
		h := strings.ToLower(strings.TrimSpace(hash))
		if size <= 0 || h == "" {
			return renameKey{}, false
		}
		return renameKey{dir: path.Dir(p), size: size, hash: h}, true
	}

	delRemoteByKey := map[renameKey][]int{}
	addUploadByKey := map[renameKey][]int{}
	delLocalByKey := map[renameKey][]int{}
	addDownloadByKey := map[renameKey][]int{}

	for i, c := range changes {
		switch c.Action {
		case "delete-remote":
			if caps.Rename && c.Remote != nil && !c.Remote.IsDir {
				if k, ok := makeKey(c.Path, c.Remote.Size, c.Remote.Hash); ok {
					delRemoteByKey[k] = append(delRemoteByKey[k], i)
				}
			}
		case "upload":
			if caps.Rename && c.Local != nil && c.Remote == nil && !c.Local.IsDir {
				if k, ok := makeKey(c.Path, c.Local.Size, c.Local.Hash); ok {
					addUploadByKey[k] = append(addUploadByKey[k], i)
				}
			}
		case "delete-local":
			if c.Local != nil && !c.Local.IsDir {
				if k, ok := makeKey(c.Path, c.Local.Size, c.Local.Hash); ok {
					delLocalByKey[k] = append(delLocalByKey[k], i)
				}
			}
		case "download":
			if c.Remote != nil && c.Local == nil && !c.Remote.IsDir {
				if k, ok := makeKey(c.Path, c.Remote.Size, c.Remote.Hash); ok {
					addDownloadByKey[k] = append(addDownloadByKey[k], i)
				}
			}
		}
	}

	consumed := map[int]bool{}
	replacements := map[int]Change{}

	// 1. 本地改名 → 远端直接重命名（免重新上传）
	for k, delIdx := range delRemoteByKey {
		addIdx := addUploadByKey[k]
		if len(delIdx) == 1 && len(addIdx) == 1 {
			d, a := delIdx[0], addIdx[0]
			consumed[d] = true
			replacements[a] = Change{
				Path:     changes[a].Path,
				FromPath: changes[d].Path,
				Action:   "rename-remote",
				Local:    changes[a].Local,
				Remote:   changes[d].Remote,
			}
		}
	}

	// 2. 远端改名 → 本地直接重命名（免重新下载）
	for k, delIdx := range delLocalByKey {
		addIdx := addDownloadByKey[k]
		if len(delIdx) == 1 && len(addIdx) == 1 {
			d, a := delIdx[0], addIdx[0]
			consumed[d] = true
			replacements[a] = Change{
				Path:     changes[a].Path,
				FromPath: changes[d].Path,
				Action:   "rename-local",
				Local:    changes[d].Local,
				Remote:   changes[a].Remote,
			}
		}
	}

	if len(consumed) == 0 {
		return changes
	}
	out := make([]Change, 0, len(changes)-len(consumed))
	for i, c := range changes {
		if consumed[i] {
			continue
		}
		if rep, ok := replacements[i]; ok {
			out = append(out, rep)
		} else {
			out = append(out, c)
		}
	}
	return out
}

func (e *Engine) renameLocalEntry(cfg Config, c Change) error {
	srcPath, err := safeLocalPath(cfg.LocalDir, c.FromPath)
	if err != nil {
		return err
	}
	dstPath, err := safeLocalPath(cfg.LocalDir, c.Path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dstPath); err == nil {
		return fmt.Errorf("目标本地文件已存在: %s", c.Path)
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}
	return os.Rename(srcPath, dstPath)
}

func (e *Engine) renameRemoteEntry(ctx context.Context, cfg Config, c Change) error {
	if c.Remote == nil || c.Remote.RemoteID == "" {
		return fmt.Errorf("缺少远端文件 ID，无法重命名 %s", c.FromPath)
	}
	return drive.RenameContext(ctx, cfg.UserID, cfg.DriveID, c.Remote.RemoteID, path.Base(c.Path))
}
