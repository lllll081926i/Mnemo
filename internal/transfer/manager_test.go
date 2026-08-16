package transfer

import (
	"sync"
	"testing"

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

	// verify the semaphore capacity matches MaxConcurrentDownloads (default 3)
	if cap(m.sem) != 3 {
		t.Errorf("expected sem cap 3, got %d", cap(m.sem))
	}
}

func TestManagerSetConcurrency(t *testing.T) {
	dir := t.TempDir()
	st, _ := store.Open(dir)
	m, _ := NewManager(st, dir, nil)
	defer m.Shutdown()

	m.SetConcurrency(5)
	if cap(m.sem) != 5 {
		t.Errorf("expected sem cap 5, got %d", cap(m.sem))
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
