package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mnemo-go/internal/model"
	"mnemo-go/internal/store"
	syncengine "mnemo-go/internal/sync"
)

func TestSyncPreviewFailurePersistsState(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := syncengine.Config{ID: "preview", Direction: "invalid", LocalDir: t.TempDir()}
	if err := st.SaveSyncConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveSyncRunState(cfg.ID, store.SyncRunState{Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	application := &App{store: st}
	if err := application.RunSyncPlan(cfg.ID, "old-token", nil); err == nil {
		t.Fatal("无效计划必须失败")
	}
	states, err := st.ListSyncRunStates()
	if err != nil || states[cfg.ID].Status != "failed" || states[cfg.ID].Issue == nil {
		t.Fatalf("预览执行失败未落盘：%+v %v", states, err)
	}
}

func TestSyncRunPersistsLifecycleAndClearsOldIssue(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	application := &App{store: st}
	for _, result := range []error{errors.New("同步失败"), nil, context.Canceled} {
		err := application.executeSyncRun(context.Background(), "job", "preview", func(context.Context) error {
			states, err := st.ListSyncRunStates()
			if err != nil || states["job"].Status != "running" || states["job"].Issue != nil {
				t.Fatalf("同步写入前未保存运行状态：%+v %v", states, err)
			}
			return result
		})
		if !errors.Is(err, result) {
			t.Fatalf("执行错误发生变化：%v", err)
		}
		states, err := st.ListSyncRunStates()
		if err != nil {
			t.Fatal(err)
		}
		want := "completed"
		if result != nil {
			want = "failed"
		}
		if errors.Is(result, context.Canceled) {
			want = "canceled"
		}
		if states["job"].Status != want || (result == nil && states["job"].Issue != nil) {
			t.Fatalf("同步结束状态错误：%+v", states["job"])
		}
	}
}

func TestSyncRunRefusesWorkWhenStateCannotBeSaved(t *testing.T) {
	root := t.TempDir()
	st, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "sync_run_states.json"), 0700); err != nil {
		t.Fatal(err)
	}
	application := &App{store: st}
	called := false
	err = application.executeSyncRun(context.Background(), "job", "preview", func(context.Context) error { called = true; return nil })
	if err == nil || called || application.isSyncRunning("job") {
		t.Fatalf("运行状态保存失败后仍执行同步：called=%v err=%v", called, err)
	}
}

func TestShutdownWaitsForSyncFinalStateAndRejectsNewRuns(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	application := &App{store: st}
	started, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	runDone, shutdownDone := make(chan error, 1), make(chan struct{})
	defer close(release)
	go func() {
		runDone <- application.executeSyncRun(context.Background(), "job", "preview", func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			close(canceled)
			<-release
			return ctx.Err()
		})
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("同步未启动")
	}
	go func() { application.Shutdown(context.Background()); close(shutdownDone) }()
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("退出未取消同步")
	}
	if _, finish, err := application.beginSyncRun(context.Background(), "late", "manual"); err == nil {
		finish(nil)
		t.Fatal("退出时仍接受新同步")
	}
	if application.StartSyncScheduler() == "" {
		t.Fatal("退出时仍接受调度器启动")
	}
	select {
	case <-shutdownDone:
		t.Fatal("同步收尾前提前退出")
	case <-time.After(20 * time.Millisecond):
	}
	release <- struct{}{}
	select {
	case err := <-runDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("同步未退出")
	}
	select {
	case <-shutdownDone:
	case <-time.After(3 * time.Second):
		t.Fatal("应用未完成退出")
	}
	states, err := st.ListSyncRunStates()
	if err != nil || states["job"].Status != "canceled" {
		t.Fatalf("退出未保存同步取消记录：%+v %v", states, err)
	}
}

func TestSyncRunStateRestoresInterruptionWithoutAutomaticWrite(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveSyncRunState("job", store.SyncRunState{Status: "running"}); err != nil {
		t.Fatal(err)
	}
	application := &App{store: st}
	states, err := application.ListSyncRunStates()
	if err != nil || states["job"].Status != "interrupted" || states["job"].Issue == nil {
		t.Fatalf("没有恢复中断提示：%+v %v", states, err)
	}
	if err := st.SaveSyncRunState("job", store.SyncRunState{Status: "failed", Issue: &model.OperationIssue{Code: "NETWORK_UNAVAILABLE", Action: "network_settings"}}); err != nil {
		t.Fatal(err)
	}
	states, err = application.ListSyncRunStates()
	if err != nil || states["job"].Issue.Code != "NETWORK_UNAVAILABLE" {
		t.Fatalf("同步失败原因未持久化：%+v %v", states, err)
	}
	if err := st.SaveSyncRunState("job", store.SyncRunState{Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	states, err = application.ListSyncRunStates()
	if err != nil || states["job"].Issue != nil {
		t.Fatal("同步成功后没有清除旧原因")
	}
}
