package drive

import (
	"context"
	"strings"
	"sync"
)

type targetLock struct {
	gate chan struct{}
	refs int
}

var targetLocks = struct {
	sync.Mutex
	entries map[string]*targetLock
}{entries: make(map[string]*targetLock)}

func LockTarget(ctx context.Context, userID, driveID, parentID, name string) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	key := strings.Join([]string{ProviderOf(userID, driveID, ""), userID, driveID, parentID, name}, "\x00")
	targetLocks.Lock()
	entry := targetLocks.entries[key]
	if entry == nil {
		entry = &targetLock{gate: make(chan struct{}, 1)}
		targetLocks.entries[key] = entry
	}
	entry.refs++
	targetLocks.Unlock()
	releaseReference := func() {
		targetLocks.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(targetLocks.entries, key)
		}
		targetLocks.Unlock()
	}
	select {
	case entry.gate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-entry.gate
			releaseReference()
			return nil, err
		}
		var once sync.Once
		return func() { once.Do(func() { <-entry.gate; releaseReference() }) }, nil
	case <-ctx.Done():
		releaseReference()
		return nil, ctx.Err()
	}
}
