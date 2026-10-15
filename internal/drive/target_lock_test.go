package drive

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestTargetLockCancellationAndIndependentTargets(testContext *testing.T) {
	release, err := LockTarget(testContext.Context(), "webdav:user", "drive", "/", "file")
	if err != nil {
		testContext.Fatal(err)
	}
	defer release()
	independent, err := LockTarget(testContext.Context(), "webdav:user", "drive", "/", "other")
	if err != nil {
		testContext.Fatal(err)
	}
	independent()
	ctx, cancel := context.WithCancel(testContext.Context())
	cancel()
	if acquired, err := LockTarget(ctx, "webdav:user", "drive", "/", "file"); !errors.Is(err, context.Canceled) || acquired != nil {
		testContext.Fatalf("取消未生效: %v", err)
	}
	release()
	release()
	targetLocks.Lock()
	defer targetLocks.Unlock()
	if len(targetLocks.entries) != 0 {
		testContext.Fatalf("目标锁未释放: %d", len(targetLocks.entries))
	}
}

func TestTargetLockSerializesSameDestination(testContext *testing.T) {
	var workers sync.WaitGroup
	var active atomic.Int32
	var overlap atomic.Bool
	for worker := 0; worker < 40; worker++ {
		workers.Go(func() {
			release, err := LockTarget(testContext.Context(), "webdav:user", "drive", "/", "same")
			if err != nil {
				testContext.Error(err)
				return
			}
			if active.Add(1) != 1 {
				overlap.Store(true)
			}
			active.Add(-1)
			release()
		})
	}
	workers.Wait()
	if overlap.Load() {
		testContext.Fatal("同一远端目标并发写入")
	}
}
