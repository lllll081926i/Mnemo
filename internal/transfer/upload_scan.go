package transfer

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"mnemo-go/internal/model"
)

type uploadSource struct {
	localPath string
	relative  string
	size      int64
}

// scanUploadSources validates the entire selection before any job is queued.
// filepath.WalkDir does not follow directory symlinks; reject all symlinks and
// special files rather than accidentally uploading outside the selection or
// treating a partially readable directory as a successful upload.
func scanUploadSources(localPaths []string) ([]uploadSource, error) {
	if len(localPaths) == 0 {
		return nil, errors.New("未选择上传文件")
	}
	sources := make([]uploadSource, 0, len(localPaths))
	for _, input := range localPaths {
		if strings.TrimSpace(input) == "" {
			return nil, errors.New("上传路径不能为空")
		}
		root := filepath.Clean(input)
		info, err := os.Lstat(root)
		if err != nil {
			return nil, fmt.Errorf("无法读取待上传路径 %q: %w", root, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("不允许上传符号链接 %q，请选择实际文件", root)
		}
		if !info.IsDir() {
			if !info.Mode().IsRegular() {
				return nil, fmt.Errorf("不支持上传特殊文件 %q", root)
			}
			sources = append(sources, uploadSource{root, info.Name(), info.Size()})
			continue
		}
		err = filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return fmt.Errorf("遍历上传目录 %q: %w", current, walkErr)
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("上传目录包含符号链接 %q", current)
			}
			if entry.IsDir() {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return fmt.Errorf("检查上传文件 %q: %w", current, err)
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("上传目录包含特殊文件 %q", current)
			}
			rel, err := filepath.Rel(filepath.Dir(root), current)
			if err != nil {
				return fmt.Errorf("计算上传相对路径 %q: %w", current, err)
			}
			sources = append(sources, uploadSource{
				localPath: current, relative: filepath.ToSlash(rel), size: info.Size(),
			})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	if len(sources) == 0 {
		return nil, errors.New("选择的目录中没有可上传的普通文件")
	}
	return sources, nil
}

// AddFilesChecked returns a scan error rather than silently queuing an
// incomplete subset. No upload begins until the entire selection is valid.
func (q *UploadQueue) AddFilesChecked(userID, driveID, parentID, conflictPolicy string, localPaths []string) ([]*model.UploadingUI, error) {
	sources, err := scanUploadSources(localPaths)
	if err != nil {
		return nil, err
	}
	created := make([]*model.UploadingUI, 0, len(sources))
	for _, file := range sources {
		job := q.enqueue(userID, driveID, parentID, conflictPolicy, file.localPath, file.relative, file.size)
		if job == nil {
			return created, errors.New("上传服务已停止，部分待上传任务尚未入队")
		}
		created = append(created, job)
	}
	return created, nil
}
