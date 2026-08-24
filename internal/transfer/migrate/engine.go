// Package migrate implements cross-drive file migration: copy/move files
// between two accounts by streaming (source download → target upload).
package migrate

import (
	"context"
	"sync"

	"mnemo-go/internal/model"
	"mnemo-go/internal/store"
)

// Job is a type alias for model.MigrateJob so existing callers (app layer,
// frontend events) continue to reference migrate.Job.
type Job = model.MigrateJob

// OnProgress is invoked per file.
type OnProgress func(j *Job)

// Engine runs migration jobs.
type Engine struct {
	store      *store.Store
	onProgress OnProgress
	persistMu  sync.Mutex
	// cancels stores per-job cancel funcs so Cancel(jobID) can abort a Run.
	cancelsMu sync.Mutex
	cancels   map[string]context.CancelFunc
}

// NewEngine creates the migration engine. store may be nil (disables
// persistence and startup recovery).
func NewEngine(st *store.Store, onProgress OnProgress) *Engine {
	return &Engine{
		store:      st,
		onProgress: onProgress,
		cancels:    make(map[string]context.CancelFunc),
	}
}
