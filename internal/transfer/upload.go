package transfer

import (
	"context"
	"errors"
	"sync"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/logging"
	"mnemo-go/internal/model"
	"mnemo-go/internal/store"
)

// UploadQueue manages upload jobs for queue-mode providers.
type UploadQueue struct {
	store           *store.Store
	mu              sync.Mutex
	dirMu           sync.Mutex
	jobs            map[string]*model.UploadingUI
	dirIDs          map[string]string
	lastPersist     map[string]progressPersistState
	lastProgress    map[string]time.Time
	onEvent         OnTaskEvent
	ctx             context.Context // root context, canceled on Close
	cancel          context.CancelFunc
	gate            *uploadConcurrencyGate
	changed         chan struct{}
	runs            map[string]*uploadRun
	generations     map[string]uint64
	handlerResolver func(userID, driveID string) (func(context.Context, *model.UploadingUI) error, error)
	persistMu       sync.Mutex
	workerWG        sync.WaitGroup
	closeOnce       sync.Once
	closed          bool
}

type uploadRun struct {
	generation uint64
	cancel     context.CancelFunc
	done       chan struct{}
}

var errUploadWorkerStopping = errors.New("上传任务仍在停止中，请稍后重试")
var errUploadQueueClosed = errors.New("上传队列已关闭")

const maxUploadDirectoryCacheEntries = 1024

// NewUploadQueue creates the upload queue and restores persisted jobs.
func NewUploadQueue(st *store.Store, onEvent OnTaskEvent) *UploadQueue {
	maxConc := 2
	keepTasks := true
	if s, err := st.GetSettings(); err == nil {
		if s.MaxConcurrentUploads > 0 {
			maxConc = s.MaxConcurrentUploads
		}
		keepTasks = s.KeepTasks
	}
	rootCtx, rootCancel := context.WithCancel(context.Background())
	q := &UploadQueue{
		store:           st,
		jobs:            map[string]*model.UploadingUI{},
		dirIDs:          map[string]string{},
		lastPersist:     map[string]progressPersistState{},
		lastProgress:    map[string]time.Time{},
		onEvent:         onEvent,
		ctx:             rootCtx,
		cancel:          rootCancel,
		gate:            newUploadConcurrencyGate(maxConc),
		changed:         make(chan struct{}),
		runs:            map[string]*uploadRun{},
		generations:     map[string]uint64{},
		handlerResolver: drive.QueueUploadHandler,
	}
	if !keepTasks {
		if err := st.ClearUploadTasks(); err != nil {
			logging.Warn("upload task history cleanup failed", "error", err)
		}
	}
	// restore persisted tasks as paused (user must resume manually)
	if list, err := st.ListUploadTasks(); err == nil {
		for i := range list {
			j := list[i]
			if j.Upload.IsDowning {
				j.Upload.IsDowning = false
				j.Upload.DownState = "paused"
			}
			q.jobs[j.UploadID] = &j
		}
	}
	return q
}
