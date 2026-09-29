package transfer

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/store"
)

type blockingURLDriver struct {
	drive.Driver
	entered     chan context.Context
	release     chan struct{}
	files       map[string][]model.File
	listCalls   int
	downloadURL string
}

func (d *blockingURLDriver) ListPaged(ctx context.Context, c drive.Context, dirID, marker string, opts *drive.ListOptions) (*drive.DirPage, error) {
	d.listCalls++
	return &drive.DirPage{Items: d.files[dirID]}, nil
}

var downloadTestDriver *blockingURLDriver

func init() {
	drive.Register(drive.Registration{ID: "webdav", Factory: func() drive.Driver { return downloadTestDriver }})
}

func TestRetryChildUpdatesFailedFolderSummary(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(st, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Shutdown()
	parent := &model.DownloadTask{ID: "folder-retry", IsDir: true, Status: "failed", TotalFiles: 2, FailedFiles: 1}
	first := &model.DownloadTask{ID: "first", GroupID: parent.ID, Status: "completed", Downloaded: 10}
	second := &model.DownloadTask{ID: "second", GroupID: parent.ID, Status: "failed", Downloaded: 5}
	manager.mu.Lock()
	manager.tasks[parent.ID], manager.tasks[first.ID], manager.tasks[second.ID] = parent, first, second
	manager.mu.Unlock()
	manager.refreshFolderSummary(parent.ID)
	if parent.CompletedFiles != 1 || parent.FailedFiles != 1 || parent.Status != "failed" {
		t.Fatalf("部分失败统计不正确: %+v", parent)
	}
	manager.mu.Lock()
	second.Status, second.Downloaded = "completed", 10
	manager.mu.Unlock()
	manager.refreshFolderSummary(parent.ID)
	if parent.Status != "completed" || parent.CompletedFiles != 2 || parent.FailedFiles != 0 || parent.Progress != 100 {
		t.Fatalf("重试完成后文件夹状态不正确: %+v", parent)
	}
}

func TestFolderScanKeepsEmptyDirectoriesAndRestoresCheckpoint(t *testing.T) {
	driver := &blockingURLDriver{files: map[string][]model.File{
		"cloud-folder": {
			{FileID: "empty", Name: "空目录", IsDir: true},
			{FileID: "nested", Name: "内容", IsDir: true},
			{FileID: "first", Name: "README.txt", Size: 4},
			{FileID: "second", Name: "readme.txt", Size: 6},
		},
		"nested": {{FileID: "traversal", Name: "../outside", Size: 1}},
	}}
	downloadTestDriver = driver
	root := t.TempDir()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(st, root, nil)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "folder")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}
	task := &model.DownloadTask{ID: "folder-checkpoint", UserID: "webdav:test", DriveID: "webdav:test", FileID: "cloud-folder", Name: "folder", LocalPath: target, IsDir: true, Status: "paused", FolderEntries: []model.DownloadEntry{{FileID: "cloud-folder", Name: "folder", RelativePath: ".", IsDir: true}}}
	manager.mu.Lock()
	manager.tasks[task.ID] = task
	manager.mu.Unlock()
	if err := manager.scanDownloadFolder(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if task.TotalFiles != 3 {
		t.Fatalf("scanned %d files, want 3", task.TotalFiles)
	}
	if _, err := os.Stat(filepath.Join(target, "空目录")); err != nil {
		t.Fatal(err)
	}
	if driver.listCalls != 3 {
		t.Fatalf("listed %d directories, want 3", driver.listCalls)
	}
	manager.Shutdown()
	restored, err := NewManager(st, root, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Shutdown()
	reloaded, ok := restored.get(task.ID)
	if !ok {
		t.Fatal("folder checkpoint was not restored")
	}
	if err := restored.scanDownloadFolder(context.Background(), reloaded); err != nil {
		t.Fatal(err)
	}
	if driver.listCalls != 3 || reloaded.TotalFiles != 3 {
		t.Fatal("resume listed or queued the same directory twice")
	}
	paths := map[string]bool{}
	for _, entry := range reloaded.FolderEntries {
		if entry.IsDir {
			continue
		}
		path, err := folderPath(target, entry.RelativePath)
		if err != nil {
			t.Fatal(err)
		}
		if paths[strings.ToLower(path)] {
			t.Fatalf("duplicate local path: %s", path)
		}
		paths[strings.ToLower(path)] = true
		if !strings.HasPrefix(path, target+string(filepath.Separator)) {
			t.Fatalf("escaped download root: %s", path)
		}
	}
	if _, err := folderPath(target, filepath.Join("..", "outside")); err == nil {
		t.Fatal("relative traversal was accepted")
	}
}

func TestFolderSkipExistingDoesNotEnqueue(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "folder"), 0755); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(st, root, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Shutdown()
	task, skipped, err := manager.AddDownloadFolder("webdav:test", "webdav:test", model.File{FileID: "folder-id", Name: "folder", IsDir: true}, true)
	if err != nil || !skipped || task != nil || len(manager.List()) != 0 {
		t.Fatalf("task=%v skipped=%v err=%v", task, skipped, err)
	}
}

func TestFolderDownloadCompletesWithEmptyDirectory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.ServeContent(writer, request, "file", time.Time{}, bytes.NewReader([]byte("hello")))
	}))
	defer server.Close()
	downloadTestDriver = &blockingURLDriver{downloadURL: server.URL, files: map[string][]model.File{
		"source": {{FileID: "empty", Name: "空目录", IsDir: true}, {FileID: "first", Name: "first.txt", Size: 5}, {FileID: "second", Name: "second.txt", Size: 5}},
	}}
	root := t.TempDir()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(st, root, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Shutdown()
	task, skipped, err := manager.AddDownloadFolder("webdav:test", "webdav:test", model.File{FileID: "source", Name: "下载目录", IsDir: true}, false)
	if err != nil || skipped {
		t.Fatalf("task=%v skipped=%v err=%v", task, skipped, err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		current, ok := manager.get(task.ID)
		if !ok {
			t.Fatal("folder task disappeared")
		}
		manager.mu.Lock()
		status, reason, completed := current.Status, current.Error, current.CompletedFiles
		manager.mu.Unlock()
		if status == "failed" {
			t.Fatal(reason)
		}
		if status == "completed" {
			if completed != 2 {
				t.Fatalf("completed %d files, want 2", completed)
			}
			for _, name := range []string{"first.txt", "second.txt"} {
				content, readErr := os.ReadFile(filepath.Join(task.LocalPath, name))
				if readErr != nil || string(content) != "hello" {
					t.Fatalf("%s: content=%q err=%v", name, content, readErr)
				}
			}
			if _, statErr := os.Stat(filepath.Join(task.LocalPath, "空目录")); statErr != nil {
				t.Fatal(statErr)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("folder download did not complete")
}
func (d *blockingURLDriver) GetDownloadURL(ctx context.Context, c drive.Context, id string, expires int) (*model.DownloadURL, error) {
	if d.downloadURL != "" {
		return &model.DownloadURL{URL: d.downloadURL + "/" + id, Size: 5}, nil
	}
	d.entered <- ctx
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-d.release:
		return nil, context.Canceled
	}
}
func TestPauseCancelsDownloadURLResolution(t *testing.T) {
	d := &blockingURLDriver{entered: make(chan context.Context, 1), release: make(chan struct{})}
	downloadTestDriver = d
	dir := t.TempDir()
	st, _ := store.Open(dir)
	m, err := NewManager(st, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Shutdown()
	task := &model.DownloadTask{ID: "cancel-url", UserID: "webdav:test", DriveID: "webdav:test", FileID: "file", Name: "file", Status: "queued"}
	if err := m.addDownloadTask(task, task.Name); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { defer close(done); m.runDownload(task) }()
	defer func() { close(d.release); <-done }()
	select {
	case <-d.entered:
	case <-time.After(time.Second):
		t.Fatal("resolver not entered")
	}
	m.mu.Lock()
	stage := task.Stage
	m.mu.Unlock()
	if stage != "resolving" {
		t.Fatalf("download stage = %q, want resolving", stage)
	}
	m.Pause(task.ID)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("paused resolver still occupies worker")
	}
}

func TestManagerConcurrency(t *testing.T) {
	dir := t.TempDir()
	st, _ := store.Open(dir)
	m, err := NewManager(st, dir, nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer m.Shutdown()

	m.mu.Lock()
	got := m.maxConcurrent
	m.mu.Unlock()
	if got != 3 {
		t.Errorf("expected max concurrency 3, got %d", got)
	}
}

func TestDownloadReportsUnreadableSettingsBeforeNetwork(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(st, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Shutdown()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	task := &model.DownloadTask{ID: "settings-error", Name: "file", URL: ":invalid", Status: "queued"}
	m.addDownloadTask(task, task.Name)
	m.runDownload(task)
	if task.Status != "failed" || !strings.Contains(task.Error, "读取下载设置失败") {
		t.Fatalf("status=%s error=%s", task.Status, task.Error)
	}
}

func TestManagerSetConcurrency(t *testing.T) {
	dir := t.TempDir()
	st, _ := store.Open(dir)
	m, _ := NewManager(st, dir, nil)
	defer m.Shutdown()

	m.SetConcurrency(5)
	m.mu.Lock()
	got := m.maxConcurrent
	m.mu.Unlock()
	if got != 5 {
		t.Errorf("expected max concurrency 5, got %d", got)
	}
}

func TestManagerConcurrencyCanShrinkWithoutOverlappingSlots(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(st, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := m.acquireSlot(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.acquireSlot(ctx); err != nil {
		t.Fatal(err)
	}
	m.SetConcurrency(1)

	acquired := make(chan struct{})
	errs := make(chan error, 1)
	go func() {
		if err := m.acquireSlot(ctx); err != nil {
			errs <- err
			return
		}
		close(acquired)
		m.releaseSlot()
	}()
	assertNotAcquired := func() {
		t.Helper()
		select {
		case <-acquired:
			t.Fatal("queued task acquired a slot above the reduced limit")
		case err := <-errs:
			t.Fatal(err)
		case <-time.After(30 * time.Millisecond):
		}
	}
	assertNotAcquired()
	m.releaseSlot()
	assertNotAcquired()
	m.releaseSlot()
	select {
	case <-acquired:
	case err := <-errs:
		t.Fatal(err)
	case <-time.After(time.Second):
		t.Fatal("queued task did not acquire a slot after active work released")
	}
}

func TestManagerSetDir(t *testing.T) {
	dir := t.TempDir()
	st, _ := store.Open(dir)
	m, _ := NewManager(st, dir, nil)
	defer m.Shutdown()

	newDir := dir + "/newdown"
	m.SetDir(newDir)
	m.mu.Lock()
	got := m.dir
	m.mu.Unlock()
	if got != filepath.Clean(newDir) {
		t.Errorf("expected dir %s, got %s", newDir, got)
	}
}

func TestManagerClearingDirectoryRestoresDefault(t *testing.T) {
	custom := t.TempDir()
	st, _ := store.Open(t.TempDir())
	m, err := NewManager(st, custom, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Shutdown()
	m.SetDir("")
	if m.dir == custom || !filepath.IsAbs(m.dir) {
		t.Fatalf("clearing directory did not restore an absolute system default: %q", m.dir)
	}
}

func TestDownloadRejectsUnavailableDirectoryBeforeEnqueue(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(blocked, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	st, _ := store.Open(t.TempDir())
	m, err := NewManager(st, blocked, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Shutdown()
	if _, err := m.AddDownloadURL("file.txt", ":invalid", nil); err == nil {
		t.Fatal("unavailable destination must be rejected before creating a download task")
	}
	if len(m.List()) != 0 {
		t.Fatal("unavailable destination created a task")
	}
}

func TestDownloadDirectoryChangeKeepsExistingTaskPaths(t *testing.T) {
	st, _ := store.Open(t.TempDir())
	first, second := t.TempDir(), t.TempDir()
	m, err := NewManager(st, first, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Shutdown()
	oldTask := &model.DownloadTask{ID: "old-directory", Status: "paused"}
	if err := m.addDownloadTask(oldTask, "original.txt"); err != nil {
		t.Fatal(err)
	}
	if err := m.SetDir(second); err != nil {
		t.Fatal(err)
	}
	newTask := &model.DownloadTask{ID: "new-directory", Status: "paused"}
	if err := m.addDownloadTask(newTask, "next.txt"); err != nil {
		t.Fatal(err)
	}
	if oldTask.LocalPath != filepath.Join(first, "original.txt") || newTask.LocalPath != filepath.Join(second, "next.txt") {
		t.Fatalf("old=%q new=%q", oldTask.LocalPath, newTask.LocalPath)
	}
	if err := m.SetDir("relative/path"); err == nil {
		t.Fatal("relative path accepted")
	}
	if current, _ := m.Directory(); current != second {
		t.Fatal("invalid path replaced working directory")
	}
}

func TestPerFileDownloadConcurrencyIsBounded(t *testing.T) {
	cases := []struct {
		configured int
		want       int
	}{
		{configured: 0, want: 3},
		{configured: 1, want: 1},
		{configured: 3, want: 3},
		{configured: 8, want: maxPerDownloadConnections},
	}
	for _, tc := range cases {
		if got := concurrencyFromSettings(store.Settings{MaxConcurrentDownloads: tc.configured}); got != tc.want {
			t.Errorf("concurrencyFromSettings(%d) = %d, want %d", tc.configured, got, tc.want)
		}
	}
}

func TestManagerShutdownCancelsContext(t *testing.T) {
	dir := t.TempDir()
	st, _ := store.Open(dir)
	m, _ := NewManager(st, dir, nil)

	// root context should be active before shutdown
	if m.ctx == nil {
		t.Fatal("ctx is nil")
	}
	select {
	case <-m.ctx.Done():
		t.Fatal("ctx should not be done before Shutdown")
	default:
	}
	m.Shutdown()
	select {
	case <-m.ctx.Done():
		// expected: ctx is done after Shutdown
	default:
		t.Error("ctx should be done after Shutdown")
	}
}

func TestManagerLoadPersistedMarksPaused(t *testing.T) {
	dir := t.TempDir()
	st, _ := store.Open(dir)
	// save a downloading task that should be restored as paused
	task := &model.DownloadTask{
		ID:     "dl-test1",
		UserID: "pikpak-test",
		Status: "downloading",
		Name:   "test.txt",
	}
	_ = st.SaveDownloadTask(task)

	m, err := NewManager(st, dir, nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer m.Shutdown()

	t2, ok := m.get("dl-test1")
	if !ok {
		t.Fatal("task not restored")
	}
	if t2.Status != "paused" {
		t.Errorf("expected paused, got %s", t2.Status)
	}
}

func TestRestoredDownloadPreservesSourcePath(t *testing.T) {
	drive.ClearFileMetaCache()
	t.Cleanup(drive.ClearFileMetaCache)
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	task := &model.DownloadTask{ID: "family-download", UserID: "account-one", DriveID: "pan139:one", FileID: "pan139:family:file-1", SourcePath: "/root/folder-1", Status: "paused"}
	if err := st.SaveDownloadTask(task); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(st, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Shutdown()
	restored, ok := manager.get(task.ID)
	if !ok || restored.SourcePath != task.SourcePath {
		t.Fatalf("来源路径未随任务恢复: %+v", restored)
	}
	rememberDownloadSource(restored)
	file, ok := drive.CachedFile(task.UserID, task.DriveID, task.FileID)
	if !ok || file.Path != task.SourcePath {
		t.Fatalf("来源路径未恢复到账号缓存: %+v", file)
	}
	if _, ok := drive.CachedFile("account-two", task.DriveID, task.FileID); ok {
		t.Fatal("来源路径泄漏到其他账号")
	}
}

func TestManagerKeepTasksDisabledClearsFinishedHistory(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	settings := store.DefaultSettings()
	settings.KeepTasks = false
	if err := st.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"completed", "failed", "canceled"} {
		if err := st.SaveDownloadTask(&model.DownloadTask{ID: "finished-" + status, Status: status}); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.SaveDownloadTask(&model.DownloadTask{ID: "paused", Status: "paused"}); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(st, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Shutdown()
	if _, ok := m.get("paused"); !ok {
		t.Fatal("paused task should remain resumable when history retention is disabled")
	}
	if _, ok := m.get("finished-completed"); ok {
		t.Fatal("completed task history was restored despite KeepTasks=false")
	}
	list, err := st.ListDownloadTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "paused" {
		t.Fatalf("finished task history was not cleared: %#v", list)
	}
}

func TestManagerProgressPersistenceIsThrottled(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(st, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Shutdown()

	task := &model.DownloadTask{ID: "dl-progress", Name: "payload.bin", Status: "downloading"}
	m.update(task)
	task.Downloaded = 128
	m.update(task)

	list, err := st.ListDownloadTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Downloaded != 0 {
		t.Fatalf("progress update should remain in memory before checkpoint: %#v", list)
	}

	m.mu.Lock()
	state := m.lastPersist[task.ID]
	state.at = time.Now().Add(-transferProgressPersistInterval)
	m.lastPersist[task.ID] = state
	m.mu.Unlock()
	m.update(task)

	list, err = st.ListDownloadTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Downloaded != 128 {
		t.Fatalf("checkpoint should persist latest progress: %#v", list)
	}
}

func TestManagerRemoveCompletedDoesNotRetainTombstone(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(st, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Shutdown()

	task := &model.DownloadTask{ID: "dl-removed", Name: "done.bin", Status: "completed"}
	m.update(task)
	m.Remove(task.ID)

	m.mu.Lock()
	_, retained := m.removed[task.ID]
	m.mu.Unlock()
	if retained {
		t.Fatal("completed task removal should not retain a tombstone")
	}

	list, err := st.ListDownloadTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("removed task was persisted again: %#v", list)
	}
}

func TestManagerRemovedQueuedTaskDoesNotReviveDuringShutdown(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(st, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Shutdown()

	task := &model.DownloadTask{ID: "dl-queued-removed", Name: "queued.bin", Status: "queued"}
	m.mu.Lock()
	m.tasks[task.ID] = task
	m.mu.Unlock()
	m.Remove(task.ID)
	m.Shutdown()
	m.runDownload(task)

	m.mu.Lock()
	_, revived := m.tasks[task.ID]
	_, retained := m.removed[task.ID]
	m.mu.Unlock()
	if revived || retained {
		t.Fatalf("removed queued task was retained or revived: revived=%v retained=%v", revived, retained)
	}
}

// TestManagerConcurrentAccess verifies no data race when multiple goroutines
// access the manager simultaneously.
func TestManagerConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	st, _ := store.Open(dir)
	m, _ := NewManager(st, dir, nil)
	defer m.Shutdown()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			m.SetConcurrency(n + 1)
			_ = m.List()
		}(i)
	}
	wg.Wait()
}

func TestSafeNameIsCrossPlatform(t *testing.T) {
	cases := map[string]string{
		"report.txt": "report.txt",
		"CON.txt":    "_CON.txt",
		"name. ":     "name",
		"../":        "download",
	}
	for input, want := range cases {
		if got := safeName(input); got != want {
			t.Errorf("safeName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestManagerAssignsUniqueDownloadTargets(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(st, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Shutdown()

	if err := os.WriteFile(filepath.Join(dir, "report.txt"), []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	first := &model.DownloadTask{ID: "dl-unique-1", Name: "report.txt", Status: "queued"}
	second := &model.DownloadTask{ID: "dl-unique-2", Name: "report.txt", Status: "queued"}
	m.addDownloadTask(first, first.Name)
	m.addDownloadTask(second, second.Name)

	if want := filepath.Join(dir, "report (1).txt"); first.LocalPath != want {
		t.Fatalf("first LocalPath = %q, want %q", first.LocalPath, want)
	}
	if want := filepath.Join(dir, "report (2).txt"); second.LocalPath != want {
		t.Fatalf("second LocalPath = %q, want %q", second.LocalPath, want)
	}
	if first.LocalPath == second.LocalPath {
		t.Fatal("same-name downloads share a target path")
	}
}

func TestManagerAvoidsOrphanedPartialTarget(t *testing.T) {
	dir := t.TempDir()
	st, _ := store.Open(dir)
	m, _ := NewManager(st, dir, nil)
	defer m.Shutdown()

	orphaned := filepath.Join(dir, "archive.zip.part")
	if err := os.WriteFile(orphaned, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	task := &model.DownloadTask{ID: "dl-partial", Name: "archive.zip", Status: "queued"}
	m.addDownloadTask(task, task.Name)
	if want := filepath.Join(dir, "archive (1).zip"); task.LocalPath != want {
		t.Fatalf("LocalPath = %q, want %q", task.LocalPath, want)
	}
}

func TestManagerDoesNotExposeDownloadSecrets(t *testing.T) {
	dir := t.TempDir()
	st, _ := store.Open(dir)
	var emitted TaskEvent
	m, _ := NewManager(st, dir, func(event TaskEvent) { emitted = event })
	defer m.Shutdown()

	task := &model.DownloadTask{
		ID:      "dl-secret",
		Name:    "secret.bin",
		Status:  "paused",
		URL:     "https://download.example/file?signature=secret",
		Headers: map[string]string{"Authorization": "Bearer secret"},
		Error:   "password=secret https://download.example/file?signature=secret",
	}
	m.addDownloadTask(task, task.Name)

	if emitted.Task.URL != "" || emitted.Task.Headers != nil || strings.Contains(emitted.Task.Error, "secret") {
		t.Fatalf("event exposed download secret: %#v", emitted.Task)
	}
	listed := m.List()
	if len(listed) != 1 || listed[0].URL != "" || listed[0].Headers != nil || strings.Contains(listed[0].Error, "secret") {
		t.Fatalf("List exposed download secret: %#v", listed)
	}
	persisted, err := st.ListDownloadTasks()
	if err != nil || len(persisted) != 1 || persisted[0].URL != "" || persisted[0].Headers != nil || strings.Contains(persisted[0].Error, "secret") {
		t.Fatalf("store exposed download secret: tasks=%#v err=%v", persisted, err)
	}
}
