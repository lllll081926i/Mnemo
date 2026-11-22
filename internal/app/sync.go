package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	stdsync "sync"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/logging"
	"mnemo-go/internal/model"
	"mnemo-go/internal/store"
	"mnemo-go/internal/sync"
	"mnemo-go/internal/transfer/dlengine"
)

type activeSyncRun struct {
	cancel    context.CancelFunc
	startedAt int64
	source    string
}

func (a *App) isSyncRunning(id string) bool {
	a.syncRunMu.Lock()
	defer a.syncRunMu.Unlock()
	_, running := a.syncRuns[id]
	return running
}

func (a *App) beginSyncRun(parent context.Context, jobID, source string) (context.Context, func(error), error) {
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
	if a.syncStopping {
		a.syncRunMu.Unlock()
		cancel()
		return nil, nil, errors.New("同步已取消或应用正在退出")
	}
	if err := ctx.Err(); err != nil {
		a.syncRunMu.Unlock()
		cancel()
		return nil, nil, err
	}
	if a.syncRuns == nil {
		a.syncRuns = make(map[string]*activeSyncRun)
	}
	if _, exists := a.syncRuns[jobID]; exists {
		a.syncRunMu.Unlock()
		cancel()
		return nil, nil, fmt.Errorf("同步任务正在运行: %s", jobID)
	}
	a.syncRuns[jobID] = run
	a.syncRunWG.Add(1)
	a.syncRunMu.Unlock()
	a.emit("sync:state", map[string]any{"id": jobID, "running": true, "source": source, "startedAt": run.startedAt})

	var finishOnce stdsync.Once
	finish := func(runErr error) {
		finishOnce.Do(func() {
			defer a.syncRunWG.Done()
			cancel()
			status := "completed"
			if errors.Is(runErr, context.Canceled) {
				status = "canceled"
			} else if runErr != nil {
				status = "failed"
			}
			a.emit("sync:state", map[string]any{"id": jobID, "running": false, "status": status, "issue": drive.DescribeOperationError(runErr)})
			a.syncRunMu.Lock()
			if current := a.syncRuns[jobID]; current == run {
				delete(a.syncRuns, jobID)
			}
			a.syncRunMu.Unlock()
		})
	}
	return ctx, finish, nil
}

func (a *App) executeSync(parent context.Context, cfg sync.Config, engine *sync.Engine, source string) (runErr error) {
	return a.executeSyncRun(parent, cfg.ID, source, func(ctx context.Context) error {
		return engine.Run(ctx, cfg)
	})
}

// 所有执行入口共用运行记录；完成记录落盘后才允许退出。
func (a *App) executeSyncRun(parent context.Context, jobID, source string, execute func(context.Context) error) (runErr error) {
	ctx, finish, err := a.beginSyncRun(parent, jobID, source)
	if err != nil {
		return err
	}
	st, storeErr := a.storeOrError()
	if storeErr != nil {
		finish(storeErr)
		return storeErr
	}
	defer func() {
		status := "completed"
		if runErr != nil {
			status = "failed"
		}
		if errors.Is(runErr, context.Canceled) {
			status = "canceled"
		}
		if err := st.SaveSyncRunState(jobID, store.SyncRunState{Status: status, Issue: drive.DescribeOperationError(runErr)}); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("同步结果记录保存失败，请核对目录后处理: %w", err))
		}
		finish(runErr)
	}()
	if err := st.SaveSyncRunState(jobID, store.SyncRunState{Status: "running"}); err != nil {
		return err
	}
	return execute(ctx)
}

func (a *App) ListSyncRunStates() (map[string]store.SyncRunState, error) {
	st, err := a.storeOrError()
	if err != nil {
		return nil, err
	}
	states, err := st.ListSyncRunStates()
	if err != nil {
		return nil, err
	}
	for id, state := range states {
		if state.Status == "running" && !a.isSyncRunning(id) {
			state.Status = "interrupted"
			state.Issue = &model.OperationIssue{Code: "RESTART_INTERRUPTED", Message: "上次同步未记录完成结果，请重新预览差异后手动同步。", Action: "manual_resume"}
			states[id] = state
		}
	}
	return states, nil
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
		sync.WithDownloadOptions(a.downloadOptions),
		sync.WithLogger(func(jobID, event, detail string) {
			a.handleSyncLog(jobID, event, detail)
		}),
	)
}

func (a *App) downloadOptions() (dlengine.Options, error) {
	if manager := a.downloadManager(); manager != nil {
		return manager.DownloadOptions()
	}
	return dlengine.Options{}, nil
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
	a.syncRunMu.Lock()
	run := a.syncRuns[id]
	a.syncRunMu.Unlock()
	if run == nil {
		return false
	}
	logging.Info("sync cancellation requested", "job_id", id, "source", run.source)
	run.cancel()
	return true
}

func (a *App) cancelAllSyncRuns() {
	a.syncRunMu.Lock()
	a.syncStopping = true
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
func (a *App) SaveSyncConfig(cfg sync.Config) error {
	if a.isSyncRunning(cfg.ID) {
		return fmt.Errorf("同步任务正在运行，请先停止: %s", cfg.ID)
	}
	st, err := a.storeOrError()
	if err != nil {
		return err
	}
	if err := st.SaveSyncConfig(cfg); err != nil {
		logging.Error("sync configuration save failed", "job_id", cfg.ID, "error", err)
		return err
	}
	logging.Info("sync configuration saved", "job_id", cfg.ID, "direction", cfg.Direction, "enabled", cfg.Enabled)
	return nil
}

// DeleteSyncConfig removes a sync job.
func (a *App) DeleteSyncConfig(id string) error {
	if a.isSyncRunning(id) {
		return fmt.Errorf("同步任务正在运行，请先停止: %s", id)
	}
	st, err := a.storeOrError()
	if err != nil {
		return err
	}
	if err := st.DeleteSyncConfig(id); err != nil {
		logging.Error("sync configuration removal failed", "job_id", id, "error", err)
		return err
	}
	logging.Info("sync configuration removed", "job_id", id)
	return nil
}

// RunSync executes a sync job synchronously.
func (a *App) RunSync(id string) error {
	st, err := a.storeOrError()
	if err != nil {
		return err
	}
	cfg, err := st.GetSyncConfig(id)
	if err != nil {
		logging.Warn("sync run rejected", "job_id", id, "error", err)
		return err
	}
	eng := a.newSyncEngine(st, true)
	return a.executeSync(a.appContext(), cfg, eng, "manual")
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
	a.stateMu.Lock()
	a.syncRunMu.Lock()
	stopping := a.syncStopping
	a.syncRunMu.Unlock()
	if stopping {
		a.stateMu.Unlock()
		return "应用正在退出，无法启动同步调度"
	}
	stop := make(chan struct{})
	eng := a.newSyncEngine(st, false)
	eng.StartScheduler(stop, st.ListSyncConfigs, func(ctx context.Context, cfg sync.Config) error {
		return a.executeSync(ctx, cfg, eng, "scheduler")
	})
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
	args := []any{"job_id", jobID, "event", event}
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

func (a *App) PreviewSync(id string) (*sync.Plan, error) {
	st, err := a.storeOrError()
	if err != nil {
		return nil, err
	}
	if a.isSyncRunning(id) {
		return nil, fmt.Errorf("同步正在运行，请先停止")
	}
	cfg, err := st.GetSyncConfig(id)
	if err != nil {
		return nil, err
	}
	return a.newSyncEngine(st, false).Preview(a.appContext(), cfg)
}

func (a *App) RunSyncPlan(id, token string, choices map[string]string) (runErr error) {
	if token == "" {
		return fmt.Errorf("请先预览同步计划")
	}
	st, err := a.storeOrError()
	if err != nil {
		return err
	}
	cfg, err := st.GetSyncConfig(id)
	if err != nil {
		return err
	}
	return a.executeSyncRun(a.appContext(), id, "preview", func(ctx context.Context) error {
		return a.newSyncEngine(st, true).ExecutePlan(ctx, cfg, token, choices)
	})
}
