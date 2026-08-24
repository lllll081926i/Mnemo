package dlengine

import (
	"context"
	"sync"
	"time"
)

// speedLimiter is a token-bucket style throttle (per second).
type speedLimiter struct {
	mu     sync.Mutex
	rate   int64
	window int64
	start  time.Time
}

func newSpeedLimiter(rate int64) *speedLimiter {
	return &speedLimiter{rate: rate, start: time.Now()}
}

func (l *speedLimiter) waitN(n int64) {
	_ = l.Wait(context.Background(), n)
}

func (l *speedLimiter) Wait(ctx context.Context, n int64) error {
	if n <= 0 {
		return nil
	}
	l.mu.Lock()
	if l.rate <= 0 {
		l.mu.Unlock()
		return nil
	}
	now := time.Now()
	// slide the window: if more than 1s has passed since the last reset,
	// reset the accumulator and the start timestamp so the limiter keeps
	// tracking the recent rate rather than a lifetime average that drifts.
	if now.Sub(l.start) >= time.Second {
		l.start = now
		l.window = 0
	}
	l.window += n
	elapsed := now.Sub(l.start).Seconds()
	allowed := float64(l.rate) * elapsed
	wait := time.Duration(0)
	if float64(l.window) > allowed {
		wait = time.Duration((float64(l.window) - allowed) / float64(l.rate) * float64(time.Second))
	}
	l.mu.Unlock()
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SharedLimiter is a process-wide cap shared by all downloads in a Manager.
type SharedLimiter struct{ speedLimiter }

func NewSharedLimiter(rate int64) *SharedLimiter {
	return &SharedLimiter{speedLimiter: *newSpeedLimiter(rate)}
}

func (l *SharedLimiter) SetRate(rate int64) {
	if l == nil {
		return
	}
	l.mu.Lock()
	if l.rate != rate {
		l.rate = rate
		l.window = 0
		l.start = time.Now()
	}
	l.mu.Unlock()
}
