package app

import (
	"fmt"

	"mnemo-go/internal/store"
	"mnemo-go/internal/sync"
)

// ListSyncConfigs lists persisted sync jobs.
func (a *App) ListSyncConfigs() []sync.Config {
	st, err := a.storeOrError()
	if err != nil {
		return nil
	}
	list, err := st.ListSyncConfigs()
	if err != nil {
		return nil
	}
	return list
}

// SaveSyncConfig persists a sync job.
func (a *App) SaveSyncConfig(cfg sync.Config) error {
	st, err := a.storeOrError()
	if err != nil {
		return err
	}
	return st.SaveSyncConfig(cfg)
}

// DeleteSyncConfig removes a sync job.
func (a *App) DeleteSyncConfig(id string) error {
	st, err := a.storeOrError()
	if err != nil {
		return err
	}
	return st.DeleteSyncConfig(id)
}

// RunSync executes a sync job synchronously.
func (a *App) RunSync(id string) error {
	st, err := a.storeOrError()
	if err != nil {
		return err
	}
	cfg, err := st.GetSyncConfig(id)
	if err != nil {
		return err
	}
	eng := sync.NewEngine(
		func(jobID string, done, total int) {
			a.emit("sync:progress", map[string]any{"id": jobID, "done": done, "total": total})
		},
		sync.WithSnapshotStore(st),
		sync.WithLogger(func(jobID, event, detail string) {
			a.emit("sync:log", map[string]any{"id": jobID, "event": event, "detail": detail})
		}),
	)
	return eng.Run(a.appContext(), cfg)
}

// StartSyncScheduler launches the background scheduler for all enabled,
// interval-based sync jobs. The scheduler runs for the lifetime of the app
// (stopped by OnShutdown); the method returns an empty string on success
// so it is serializable through the Wails binding.
func (a *App) StartSyncScheduler() string {
	st, err := a.storeOrError()
	if err != nil {
		return fmt.Sprint(err)
	}
	stop := make(chan struct{})
	eng := sync.NewEngine(nil,
		sync.WithSnapshotStore(st),
		sync.WithLogger(func(jobID, event, detail string) {
			a.emit("sync:log", map[string]any{"id": jobID, "event": event, "detail": detail})
		}),
	)
	eng.StartScheduler(stop, st.ListSyncConfigs)
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

var _ = store.Settings{}
