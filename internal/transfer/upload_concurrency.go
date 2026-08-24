package transfer

import (
	"context"
	"sync"
)

// uploadConcurrencyGate is a dynamically resizable counting gate. A channel
// semaphore cannot safely shrink while slots are occupied, so upload workers
// wait on a change notification and re-check the current limit instead.
type uploadConcurrencyGate struct {
	mu      sync.Mutex
	limit   int
	active  int
	changed chan struct{}
}

func newUploadConcurrencyGate(limit int) *uploadConcurrencyGate {
	if limit < 1 {
		limit = 1
	}
	return &uploadConcurrencyGate{limit: limit, changed: make(chan struct{})}
}

func (g *uploadConcurrencyGate) acquire(ctx context.Context) error {
	for {
		g.mu.Lock()
		if g.active < g.limit {
			g.active++
			g.mu.Unlock()
			return nil
		}
		changed := g.changed
		g.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}

func (g *uploadConcurrencyGate) release() {
	g.mu.Lock()
	if g.active > 0 {
		g.active--
	}
	g.notifyLocked()
	g.mu.Unlock()
}

func (g *uploadConcurrencyGate) setLimit(limit int) {
	if limit < 1 {
		limit = 1
	}
	g.mu.Lock()
	g.limit = limit
	g.notifyLocked()
	g.mu.Unlock()
}

func (g *uploadConcurrencyGate) notifyLocked() {
	close(g.changed)
	g.changed = make(chan struct{})
}
