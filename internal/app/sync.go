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
	eng := sync.NewEngine(func(jobID string, done, total int) {
		a.emit("sync:progress", map[string]any{"id": jobID, "done": done, "total": total})
	})
	return eng.Run(context.Background(), cfg)
}

var _ = store.Settings{}
