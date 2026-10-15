package app

import (
	"testing"

	"mnemo-go/internal/model"
	"mnemo-go/internal/store"
)

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
