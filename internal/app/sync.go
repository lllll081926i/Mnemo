package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"mnemo-go/internal/logging"
	"mnemo-go/internal/store"
	"mnemo-go/internal/sync"
)

type activeSyncRun struct {
	cancel    context.CancelFunc
	startedAt int64
	source    string
}

// syncRunMeta is emitted with lifecycle events so the file view can invalidate
// only the directory affected by a completed remote-writing sync.
// Keep the generic beginSyncRun helper below for callers/tests that do not
// have a persisted Config at hand.
type syncRunMeta struct {
	userID            string
	driveID           string
	remoteDir         string
	direction         string
	deletePropagation bool
}

func (a *App) isSyncRunning(id string) bool {
	a.syncRunMu.Lock()
	defer a.syncRunMu.Unlock()
	_, running := a.syncRuns[id]
	return running
}

func (a *App) beginSyncRun(parent context.Context, jobID, source string) (context.Context, func(error), error) {
	return a.beginSyncRunWithMeta(parent, jobID, source, syncRunMeta{})
}

func (a *App) beginSyncRunWithMeta(parent context.Context, jobID, source string, meta syncRunMeta) (context.Context, func(error), error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil, nil, errors.New("同步任务 ID 为空")
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	run := &activeSyncRun{cancel: cancel, startedAt: time.Now().Unix(), source: source}

	a.syncRunMu.Lock()
	if a.syncRuns == nil {
		a.syncRuns = make(map[string]*activeSyncRun)
	}
	if _, exists := a.syncRuns[jobID]; exists {
		a.syncRunMu.Unlock()
		cancel()
		return nil, nil, fmt.Errorf("同步任务正在运行: %s", jobID)
	}
	a.syncRuns[jobID] = run
	a.syncRunMu.Unlock()
	state := map[string]any{"id": jobID, "running": true, "source": source, "startedAt": run.startedAt}
	if meta.userID != "" {
		state["user_id"] = meta.userID
		state["drive_id"] = meta.driveID
		state["remote_dir"] = meta.remoteDir
		state["direction"] = meta.direction
		state["delete_propagation"] = meta.deletePropagation
	}
	a.emit("sync:state", state)

	finish := func(runErr error) {
		cancel()
		a.syncRunMu.Lock()
		if current := a.syncRuns[jobID]; current == run {
			delete(a.syncRuns, jobID)
		}
		a.syncRunMu.Unlock()
		status := "completed"
		if errors.Is(runErr, context.Canceled) {
			status = "canceled"
		} else if runErr != nil {
			status = "failed"
		}
		state := map[string]any{"id": jobID, "running": false, "status": status, "startedAt": run.startedAt}
		if meta.userID != "" {
			state["user_id"] = meta.userID
			state["drive_id"] = meta.driveID
			state["remote_dir"] = meta.remoteDir
			state["direction"] = meta.direction
			state["delete_propagation"] = meta.deletePropagation
		}
		a.emit("sync:state", state)
	}
	return ctx, finish, nil
}

func (a *App) executeSync(parent context.Context, cfg sync.Config, engine *sync.Engine, source string) (runErr error) {
	ctx, finish, err := a.beginSyncRunWithMeta(parent, cfg.ID, source, syncRunMeta{
		userID:            cfg.UserID,
		driveID:           cfg.DriveID,
		remoteDir:         cfg.RemoteDir,
		direction:         cfg.Direction,
		deletePropagation: cfg.DeletePropagation,
	})
	if err != nil {
		return err
	}
	defer func() { finish(runErr) }()
	return engine.Run(ctx, cfg)
}

func (a *App) newSyncEngine(st sync.SnapshotStore, progress bool) *sync.Engine {
	var onProgress func(jobID string, done, total int)
	if progress {
		onProgress = func(jobID string, done, total int) {
			a.emit("sync:progress", map[string]any{"id": jobID, "done": done, "total": total})
		}
	}
	return sync.NewEngine(onProgress,
		sync.WithSnapshotStore(st),
		sync.WithLogger(func(jobID, event, detail string) {
			a.handleSyncLog(jobID, event, detail)
		}),
	)
}

// ListRunningSyncIDs returns both manually and scheduler-started jobs so a
// newly mounted frontend can reconstruct the current running state.
func (a *App) ListRunningSyncIDs() []string {
	a.syncRunMu.Lock()
	defer a.syncRunMu.Unlock()
	ids := make([]string, 0, len(a.syncRuns))
	for id := range a.syncRuns {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// CancelSync cancels an active manual or scheduled sync job. The registry is
// cleared only after the worker exits, preventing an immediate overlapping run.
func (a *App) CancelSync(id string) bool {
	started := logActionStarted("取消同步", "sync", "", "", "job_id", redactID(id))
	a.syncRunMu.Lock()
	run := a.syncRuns[id]
	a.syncRunMu.Unlock()
	if run == nil {
		logActionFinished("取消同步", "sync", "", "", started, errors.New("同步任务未运行"), "job_id", redactID(id))
		return false
	}
	run.cancel()
	logActionFinished("取消同步", "sync", "", "", started, nil, "job_id", redactID(id), "source", run.source)
	return true
}

func (a *App) cancelAllSyncRuns() {
	a.syncRunMu.Lock()
	runs := make([]*activeSyncRun, 0, len(a.syncRuns))
	for _, run := range a.syncRuns {
		runs = append(runs, run)
	}
	a.syncRunMu.Unlock()
	for _, run := range runs {
		run.cancel()
	}
}

// ListSyncConfigs lists persisted sync jobs.
func (a *App) ListSyncConfigs() []sync.Config {
	st, err := a.storeOrError()
	if err != nil {
		logging.Warn("sync configurations unavailable", "error", err)
		return nil
	}
	list, err := st.ListSyncConfigs()
	if err != nil {
		logging.Warn("sync configurations load failed", "error", err)
		return nil
	}
	return list
}

// SaveSyncConfig persists a sync job.
func (a *App) SaveSyncConfig(cfg sync.Config) (retErr error) {
	started := logActionStarted("保存同步任务", "sync", cfg.UserID, cfg.DriveID,
		"job_id", redactID(cfg.ID), "direction", cfg.Direction, "enabled", cfg.Enabled)
	defer func() {
		logActionFinished("保存同步任务", "sync", cfg.UserID, cfg.DriveID, started, retErr,
			"job_id", redactID(cfg.ID), "direction", cfg.Direction, "enabled", cfg.Enabled)
	}()
	if a.isSyncRunning(cfg.ID) {
		return fmt.Errorf("同步任务正在运行，请先停止: %s", cfg.ID)
	}
	st, retErr := a.storeOrError()
	if retErr != nil {
		return retErr
	}
	if retErr = st.SaveSyncConfig(cfg); retErr != nil {
		return retErr
	}
	return nil
}

// DeleteSyncConfig removes a sync job.
func (a *App) DeleteSyncConfig(id string) (retErr error) {
	started := logActionStarted("删除同步任务", "sync", "", "", "job_id", redactID(id))
	defer func() {
		logActionFinished("删除同步任务", "sync", "", "", started, retErr, "job_id", redactID(id))
	}()
	if a.isSyncRunning(id) {
		return fmt.Errorf("同步任务正在运行，请先停止: %s", id)
	}
	st, retErr := a.storeOrError()
	if retErr != nil {
		return retErr
	}
	if retErr = st.DeleteSyncConfig(id); retErr != nil {
		return retErr
	}
	return nil
}

// RunSync executes a sync job synchronously.
func (a *App) RunSync(id string) (retErr error) {
	started := logActionStarted("启动同步", "sync", "", "", "job_id", redactID(id))
	defer func() {
		logActionFinished("启动同步", "sync", "", "", started, retErr, "job_id", redactID(id))
	}()
	st, retErr := a.storeOrError()
	if retErr != nil {
		return retErr
	}
	cfg, retErr := st.GetSyncConfig(id)
	if retErr != nil {
		return retErr
	}
	eng := a.newSyncEngine(st, true)
	retErr = a.executeSync(a.appContext(), cfg, eng, "manual")
	return retErr
}

// StartSyncScheduler launches the background scheduler for all enabled,
// interval-based sync jobs. The scheduler runs for the lifetime of the app
// (stopped by OnShutdown); the method returns an empty string on success
// so it is serializable through the Wails binding.
func (a *App) StartSyncScheduler() string {
	st, err := a.storeOrError()
	if err != nil {
		logging.Error("sync scheduler startup failed", "error", err)
		return fmt.Sprint(err)
	}
	stop := make(chan struct{})
	eng := a.newSyncEngine(st, false)
	eng.StartScheduler(stop, st.ListSyncConfigs, func(ctx context.Context, cfg sync.Config) error {
		return a.executeSync(ctx, cfg, eng, "scheduler")
	})
	// keep the stop channel for Shutdown; a.schedStop is closed on shutdown
	a.stateMu.Lock()
	previous := a.schedStop
	if previous != nil {
		close(previous)
	}
	a.schedStop = stop
	a.stateMu.Unlock()
	return ""
}

func (a *App) handleSyncLog(jobID, event, detail string) {
	a.emit("sync:log", map[string]any{"id": jobID, "event": event, "detail": detail})
	args := []any{"page", "sync", "job_id", redactID(jobID), "event", event}
	if detail != "" {
		args = append(args, "detail", detail)
	}
	switch event {
	case "error", "scheduler_error":
		logging.Error("sync failed", args...)
	case "delete_error":
		logging.Error("sync deletion failed", args...)
	case "delete_cancelled":
		logging.Warn("sync deletion paused", args...)
	case "start":
		logging.Info("sync started", args...)
	case "complete":
		logging.Info("sync completed", args...)
	case "delete":
		logging.Info("sync deletion applied", args...)
	default:
		logging.Info("sync event", args...)
	}
}

var _ = store.Settings{}
