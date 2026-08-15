package app

import (
	"context"

	"mnemo-go/internal/sync"
	"mnemo-go/internal/store"
)

// ListSyncConfigs lists persisted sync jobs.
func (a *App) ListSyncConfigs() []sync.Config {
	list, err := a.store.ListSyncConfigs()
	if err != nil {
		return nil
	}
	return list
}

// SaveSyncConfig persists a sync job.
func (a *App) SaveSyncConfig(cfg sync.Config) error {
	return a.store.SaveSyncConfig(cfg)
}

// DeleteSyncConfig removes a sync job.
func (a *App) DeleteSyncConfig(id string) error {
	return a.store.DeleteSyncConfig(id)
}

// RunSync executes a sync job synchronously.
func (a *App) RunSync(id string) error {
	cfg, err := a.store.GetSyncConfig(id)
	if err != nil {
		return err
	}
	eng := sync.NewEngine(
		func(jobID string, done, total int) {
			a.emit("sync:progress", map[string]any{"id": jobID, "done": done, "total": total})
		},
		sync.WithSnapshotStore(a.store),
		sync.WithLogger(func(jobID, event, detail string) {
			a.emit("sync:log", map[string]any{"id": jobID, "event": event, "detail": detail})
		}),
	)
	return eng.Run(context.Background(), cfg)
}

// StartSyncScheduler launches the background scheduler for all enabled,
// interval-based sync jobs. The scheduler runs for the lifetime of the app
// (stopped by OnShutdown); the method returns an empty string on success
// so it is serializable through the Wails binding.
func (a *App) StartSyncScheduler() string {
	stop := make(chan struct{})
	eng := sync.NewEngine(nil,
		sync.WithSnapshotStore(a.store),
		sync.WithLogger(func(jobID, event, detail string) {
			a.emit("sync:log", map[string]any{"id": jobID, "event": event, "detail": detail})
		}),
	)
	eng.StartScheduler(stop, a.store.ListSyncConfigs)
	// keep the stop channel for Shutdown; a.schedStop is closed on shutdown
	a.schedStop = stop
	return ""
}

var _ = store.Settings{}
