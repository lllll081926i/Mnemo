// Package transfer manages download/upload tasks: the segmented download
// manager, the upload queue and cross-drive migration.
package transfer

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"mnemo-go/internal/logging"
	"mnemo-go/internal/model"
	"mnemo-go/internal/store"
	"mnemo-go/internal/transfer/dlengine"
)

// OnTaskEvent is called on every task state/progress change.
type OnTaskEvent func(ev TaskEvent)

// TaskEvent is pushed to the frontend via events.
type TaskEvent struct {
	Kind string             `json:"kind"` // download | upload | offline
	Task model.DownloadTask `json:"task"`
}

// Progress remains live in the UI, but task files only need a crash-recovery
// checkpoint every few seconds. Writing the whole JSON task list for every
// network tick creates unnecessary disk I/O and short-lived allocations.
const transferProgressPersistInterval = 5 * time.Second

type progressPersistState struct {
	status string
	at     time.Time
}

// Manager owns the active download queue.
type Manager struct {
	store              *store.Store
	mu                 sync.Mutex
	targetMu           sync.Mutex
	persistMu          sync.Mutex
	tasks              map[string]*model.DownloadTask
	cancels            map[string]context.CancelFunc
	removed            map[string]bool
	lastPersist        map[string]progressPersistState
	onEvent            OnTaskEvent
	dir                string
	stop               chan struct{}
	ctx                context.Context // root context for all downloads, canceled on Shutdown
	cancel             context.CancelFunc
	maxConcurrent      int
	activeConcurrent   int
	concurrencyChanged chan struct{}
	speedLimiter       *dlengine.SharedLimiter
	shutdownOnce       sync.Once
}

// NewManager creates a download manager.
func NewManager(st *store.Store, downloadDir string, onEvent OnTaskEvent) (*Manager, error) {
	if downloadDir == "" {
		home, _ := os.UserHomeDir()
		downloadDir = filepath.Join(home, "Downloads")
	}
	_ = os.MkdirAll(downloadDir, 0o755)
	maxConc := 3
	keepTasks := true
	settings, settingsErr := st.GetSettings()
	if settingsErr == nil {
		if settings.MaxConcurrentDownloads > 0 {
			maxConc = settings.MaxConcurrentDownloads
		}
		keepTasks = settings.KeepTasks
	}
	rootCtx, rootCancel := context.WithCancel(context.Background())
	initialRate := int64(0)
	if settingsErr == nil {
		initialRate = settings.MaxDownloadSpeed
	}
	m := &Manager{
		store:              st,
		tasks:              map[string]*model.DownloadTask{},
		cancels:            map[string]context.CancelFunc{},
		removed:            map[string]bool{},
		lastPersist:        map[string]progressPersistState{},
		onEvent:            onEvent,
		dir:                downloadDir,
		stop:               make(chan struct{}),
		ctx:                rootCtx,
		maxConcurrent:      maxConc,
		concurrencyChanged: make(chan struct{}),
		cancel:             rootCancel,
		speedLimiter:       dlengine.NewSharedLimiter(initialRate),
	}
	if !keepTasks {
		if err := st.ClearDownloadTasks(); err != nil {
			logging.Warn("download task history cleanup failed", "error", err)
		}
	}
	// restore persisted tasks
	m.loadPersisted()
	return m, nil
}
