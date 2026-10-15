package transfer

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"
	"unicode/utf16"

	"mnemo-go/internal/config"
	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

func (m *Manager) AddDownloadFolder(userID, driveID string, file model.File, skipExisting bool) (*model.DownloadTask, bool, error) {
	if !file.IsDir {
		return nil, false, fmt.Errorf("请选择文件夹")
	}
	if file.FileID == "" || userID == "" {
		return nil, false, fmt.Errorf("缺少文件夹身份信息")
	}
	drive.RememberFile(userID, driveID, file)
	m.targetMu.Lock()
	defer m.targetMu.Unlock()
	m.mu.Lock()
	for _, task := range m.tasks {
		if task.IsDir && task.UserID == userID && task.DriveID == driveID && task.FileID == file.FileID && task.Status != "completed" && task.Status != "canceled" {
			resume := task.Status == "paused" || task.Status == "failed"
			result := safeDownloadTask(*task)
			m.mu.Unlock()
			if resume {
				m.Resume(task.ID)
			}
			return &result, skipExisting && !resume, nil
		}
	}
	m.mu.Unlock()
	dir, err := m.Directory()
	if err != nil {
		return nil, false, err
	}
	if err = config.EnsureDownloadDir(dir); err != nil {
		return nil, false, err
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, false, err
	}
	var target string
	for index := 0; ; index++ {
		name := safeFolderName(file.Name)
		if index > 0 {
			name = numberedDownloadName(name, index)
		}
		target = filepath.Join(dir, name)
		m.mu.Lock()
		occupied := m.downloadPathOccupiedLocked(target)
		m.mu.Unlock()
		if occupied {
			if skipExisting {
				return nil, true, nil
			}
			continue
		}
		if err = os.Mkdir(target, 0755); os.IsExist(err) {
			if skipExisting {
				return nil, true, nil
			}
			continue
		}
		if err != nil {
			return nil, false, err
		}
		break
	}
	task := &model.DownloadTask{
		ID: newID("folder"), UserID: userID, DriveID: driveID, Provider: drive.ProviderOf(userID, driveID, ""),
		FileID: file.FileID, SourcePath: file.Path, Name: file.Name, IsDir: true,
		LocalPath: target, Status: "queued", Created: time.Now().Unix(), Updated: time.Now().Unix(),
		FolderEntries: []model.DownloadEntry{{FileID: file.FileID, SourcePath: file.Path, Name: file.Name, IsDir: true, RelativePath: "."}},
	}
	if err := m.store.SaveDownloadTask(task); err != nil {
		return nil, false, err
	}
	m.update(task)
	result := safeDownloadTask(*task)
	m.startDownload(task)
	return &result, false, nil
}

func folderChildID(groupID, fileID string) string {
	return fmt.Sprintf("%s-%x", groupID, sha256.Sum256([]byte(fileID)))
}

func folderPath(root, relative string) (string, error) {
	if !filepath.IsAbs(root) || !filepath.IsLocal(relative) {
		return "", fmt.Errorf("下载目录路径不安全")
	}
	target := filepath.Join(root, relative)
	if runtime.GOOS == "windows" && len(utf16.Encode([]rune(target))) >= 240 {
		return "", fmt.Errorf("下载目标路径过长，请选择更短的下载目录：%s", target)
	}
	if len(target) >= 3900 {
		return "", fmt.Errorf("下载目标路径过长，请选择更短的下载目录：%s", target)
	}
	for current := target; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("下载目录不能包含符号链接：%s", current)
		}
		if current == filepath.Dir(current) {
			break
		}
	}
	return target, nil
}

func allocateFolderName(occupied map[string]bool, parent, name string) string {
	for index := 0; ; index++ {
		candidateName := safeFolderName(name)
		if index > 0 {
			candidateName = numberedDownloadName(candidateName, index)
		}
		candidate := filepath.Join(parent, candidateName)
		key := strings.ToLower(candidate)
		if !occupied[key] && !occupied[key+".part"] && !occupied[key+".state.json"] {
			occupied[key], occupied[key+".part"], occupied[key+".state.json"] = true, true, true
			return candidate
		}
	}
}

func safeFolderName(name string) string {
	name = safeName(strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return -1
		}
		return character
	}, name))
	characters := []rune(name)
	if len(characters) <= 60 {
		return name
	}
	extension := []rune(filepath.Ext(name))
	if len(extension) > 20 {
		extension = nil
	}
	return string(characters[:60-len(extension)]) + string(extension)
}

func (m *Manager) folderCheckpoint(task *model.DownloadTask) error {
	m.persistMu.Lock()
	defer m.persistMu.Unlock()
	m.mu.Lock()
	if m.removed[task.ID] || m.tasks[task.ID] != task {
		m.mu.Unlock()
		return context.Canceled
	}
	snapshot := *task
	snapshot.FolderEntries = append([]model.DownloadEntry(nil), task.FolderEntries...)
	m.mu.Unlock()
	return m.store.SaveDownloadTask(&snapshot)
}

func (m *Manager) scanDownloadFolder(ctx context.Context, task *model.DownloadTask) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		m.mu.Lock()
		position := -1
		var entry model.DownloadEntry
		for index, item := range task.FolderEntries {
			if item.IsDir && !item.Scanned {
				position, entry = index, item
				break
			}
		}
		m.mu.Unlock()
		if position < 0 {
			return nil
		}
		directory, err := folderPath(task.LocalPath, entry.RelativePath)
		if err != nil {
			return err
		}
		if err = os.MkdirAll(directory, 0755); err != nil {
			return err
		}
		cached, _ := drive.CachedFile(task.UserID, task.DriveID, entry.FileID)
		cached.FileID, cached.Path, cached.Name, cached.IsDir = entry.FileID, entry.SourcePath, entry.Name, true
		drive.RememberFile(task.UserID, task.DriveID, cached)
		files, err := drive.ListDirAllContext(ctx, task.UserID, task.DriveID, entry.FileID, nil)
		if err != nil {
			return fmt.Errorf("读取文件夹 %s 失败：%w", entry.Name, err)
		}
		for _, file := range files {
			if file.FileID != "" {
				drive.RememberFile(task.UserID, task.DriveID, file)
			}
		}
		m.mu.Lock()
		seen := make(map[string]bool, len(task.FolderEntries))
		occupied := make(map[string]bool, len(task.FolderEntries)*3)
		for _, item := range task.FolderEntries {
			seen[item.FileID] = true
			key := strings.ToLower(item.RelativePath)
			occupied[key], occupied[key+".part"], occupied[key+".state.json"] = true, true, true
		}
		for _, file := range files {
			if file.FileID == "" {
				err = fmt.Errorf("目录返回了没有身份信息的项目")
				break
			}
			if seen[file.FileID] {
				continue
			}
			if len(task.FolderEntries) >= 100000 {
				err = fmt.Errorf("目录超过十万项，请分批下载")
				break
			}
			seen[file.FileID] = true
			relative := allocateFolderName(occupied, entry.RelativePath, file.Name)
			task.FolderEntries = append(task.FolderEntries, model.DownloadEntry{FileID: file.FileID, ParentFileID: entry.FileID, SourcePath: file.Path, Name: file.Name, RelativePath: relative, Size: file.Size, IsDir: file.IsDir, ContentHash: file.ContentHash, ContentHashName: file.ContentHashName})
			if !file.IsDir {
				task.TotalFiles++
				task.Size += max(0, file.Size)
			}
		}
		if err == nil {
			task.FolderEntries[position].Scanned = true
		}
		m.mu.Unlock()
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if err = m.folderCheckpoint(task); err != nil {
			return err
		}
		m.update(task)
	}
}

func (m *Manager) runFolderDownload(task *model.DownloadTask) {
	m.mu.Lock()
	if m.tasks[task.ID] != task || task.Status != "queued" || m.cancels[task.ID] != nil || m.ctx.Err() != nil {
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancels[task.ID] = cancel
	task.Status, task.Stage = "downloading", "scanning"
	m.mu.Unlock()
	var workErr error
	defer func() {
		interrupted := ctx.Err() != nil
		cancel()
		m.mu.Lock()
		if task.Status == "downloading" {
			if interrupted {
				task.Status = "paused"
			} else if workErr != nil {
				task.Status, task.Error = "failed", workErr.Error()
			}
		}
		task.Speed = 0
		delete(m.cancels, task.ID)
		removed := m.tasks[task.ID] != task
		if removed {
			delete(m.removed, task.ID)
		}
		m.mu.Unlock()
		if !removed {
			m.update(task)
		}
		if workErr != nil && !removed {
			m.controlFolderChildren(task.ID, "pause")
		}
	}()
	m.update(task)
	workErr = m.scanDownloadFolder(ctx, task)
	if workErr != nil {
		return
	}
	m.mu.Lock()
	task.ScanDone, task.Stage = true, ""
	entries := append([]model.DownloadEntry(nil), task.FolderEntries...)
	m.mu.Unlock()
	if workErr = m.folderCheckpoint(task); workErr != nil {
		return
	}
	retried := map[string]bool{}
	refreshedParents := map[string]bool{}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if workErr = ctx.Err(); workErr != nil {
			return
		}
		m.mu.Lock()
		active, completed, failed := 0, 0, 0
		var downloaded, speed int64
		for _, child := range m.tasks {
			if child.GroupID != task.ID {
				continue
			}
			downloaded += child.Downloaded
			if child.Status == "downloading" {
				speed += child.Speed
			}
			if child.Status == "queued" || child.Status == "downloading" || m.cancels[child.ID] != nil {
				active++
			}
			if child.Status == "completed" {
				completed++
			}
			if child.Status == "failed" || child.Status == "canceled" {
				failed++
			}
		}
		task.CompletedFiles, task.FailedFiles, task.Downloaded, task.Speed = completed, failed, downloaded, speed
		if task.Size > 0 {
			task.Progress = int(min(99, downloaded*100/task.Size))
		}
		limit := m.maxConcurrent
		m.mu.Unlock()
		for _, entry := range entries {
			if entry.IsDir || active >= limit {
				continue
			}
			id := folderChildID(task.ID, entry.FileID)
			m.mu.Lock()
			child := m.tasks[id]
			eligible := child == nil || ((child.Status == "paused" || child.Status == "failed") && !retried[id] && m.cancels[id] == nil)
			m.mu.Unlock()
			if !eligible {
				continue
			}
			if entry.ParentFileID != "" && !refreshedParents[entry.ParentFileID] {
				if _, cached := drive.CachedFile(task.UserID, task.DriveID, entry.FileID); !cached {
					if listed, listErr := drive.ListDirAllContext(ctx, task.UserID, task.DriveID, entry.ParentFileID, nil); listErr == nil {
						for _, file := range listed {
							drive.RememberFile(task.UserID, task.DriveID, file)
						}
					}
				}
				refreshedParents[entry.ParentFileID] = true
			}
			var target string
			target, workErr = folderPath(task.LocalPath, entry.RelativePath)
			if workErr != nil {
				return
			}
			if workErr = os.MkdirAll(filepath.Dir(target), 0755); workErr != nil {
				return
			}
			if child == nil {
				for _, candidate := range []string{target, target + ".part", target + ".state.json"} {
					if _, err := os.Lstat(candidate); !os.IsNotExist(err) {
						workErr = fmt.Errorf("目标路径已被其他文件占用：%s", candidate)
						return
					}
				}
			}
			m.mu.Lock()
			if ctx.Err() != nil || task.Status != "downloading" {
				m.mu.Unlock()
				workErr = context.Canceled
				return
			}
			if child != nil && (m.tasks[id] != child || (child.Status != "paused" && child.Status != "failed") || m.cancels[id] != nil) {
				m.mu.Unlock()
				continue
			}
			if child == nil {
				child = &model.DownloadTask{ID: id, GroupID: task.ID, UserID: task.UserID, DriveID: task.DriveID, Provider: task.Provider, FileID: entry.FileID, SourcePath: entry.SourcePath, Name: entry.Name, LocalPath: target, Size: entry.Size, Created: time.Now().Unix(), ContentHash: entry.ContentHash, ContentHashName: entry.ContentHashName}
				m.tasks[id] = child
			}
			child.Status, child.Error = "queued", ""
			child.Updated = time.Now().Unix()
			persisted := safeDownloadTask(*child)
			m.mu.Unlock()
			m.persistMu.Lock()
			workErr = m.store.SaveDownloadTask(&persisted)
			m.persistMu.Unlock()
			if workErr != nil {
				m.mu.Lock()
				child.Status = "paused"
				m.mu.Unlock()
				return
			}
			m.mu.Lock()
			m.lastPersist[id] = progressPersistState{status: child.Status, at: time.Now()}
			m.mu.Unlock()
			m.update(child)
			retried[id] = true
			active++
			m.startDownload(child)
		}
		m.mu.Lock()
		finished := active == 0 && completed+failed == task.TotalFiles && task.Status == "downloading"
		if finished {
			if failed > 0 {
				task.Status, task.Error = "failed", fmt.Sprintf("%d 个文件未完成，继续可重试", failed)
			} else {
				task.Status, task.Progress = "completed", 100
			}
		}
		m.mu.Unlock()
		m.update(task)
		if finished {
			return
		}
		select {
		case <-ctx.Done():
			workErr = ctx.Err()
			return
		case <-ticker.C:
		}
	}
}

func (m *Manager) controlFolderChildren(id, action string) {
	m.mu.Lock()
	parent := m.tasks[id]
	var ids []string
	if parent != nil && parent.IsDir {
		if action == "remove" {
			parent.Status = "canceled"
			if cancel := m.cancels[id]; cancel != nil {
				cancel()
			}
		}
		for _, child := range m.tasks {
			if child.GroupID == id && (action == "remove" || child.Status != "completed") {
				ids = append(ids, child.ID)
			}
		}
	}
	m.mu.Unlock()
	for _, childID := range ids {
		switch action {
		case "pause":
			m.Pause(childID)
		case "cancel":
			m.Cancel(childID)
		case "remove":
			m.Remove(childID)
		}
	}
}

func validateFolderChild(task *model.DownloadTask, parent *model.DownloadTask) error {
	if parent == nil || !parent.IsDir {
		return fmt.Errorf("文件夹下载任务不存在")
	}
	relative, err := filepath.Rel(parent.LocalPath, task.LocalPath)
	if err != nil || strings.TrimSpace(relative) == "." {
		return fmt.Errorf("文件下载路径不安全")
	}
	_, err = folderPath(parent.LocalPath, relative)
	return err
}
