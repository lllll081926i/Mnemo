package transfer

import (
	"context"
	"sync"
	"testing"
	"time"

	"mnemo-go/internal/model"
	"mnemo-go/internal/store"
)

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
	if got != newDir {
		t.Errorf("expected dir %s, got %s", newDir, got)
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
