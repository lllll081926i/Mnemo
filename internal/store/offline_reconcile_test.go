package store

import (
	"testing"

	"mnemo-go/internal/model"
)

func TestOfflineReconcileRequiresCompleteListAndPreservesOtherScopes(testContext *testing.T) {
	storage, err := Open(testContext.TempDir())
	if err != nil {
		testContext.Fatal(err)
	}
	for _, task := range []model.OfflineTask{
		{ID: "gone", UserID: "user", DriveID: "one", Status: "running"},
		{ID: "gone", UserID: "other", DriveID: "one", Status: "running"},
		{ID: "gone", UserID: "user", DriveID: "two", Status: "running"},
	} {
		if err := storage.SaveOfflineTask(&task); err != nil {
			testContext.Fatal(err)
		}
	}
	for _, complete := range []bool{false, true} {
		result, err := storage.ReconcileOfflineTasks("user", "one", nil, complete)
		if err != nil || len(result) != 1 {
			testContext.Fatalf("对账丢失本地历史: %+v %v", result, err)
		}
		expected := "running"
		if complete {
			expected = "notFoundOnServer"
		}
		if result[0].Status != expected {
			testContext.Fatalf("对账错误推断完成或列表完整性: %+v", result)
		}
	}
	all, err := storage.ListOfflineTasks()
	if err != nil || len(all) != 3 {
		testContext.Fatalf("对账影响其他作用域: %+v %v", all, err)
	}
	for _, task := range all {
		if (task.UserID != "user" || task.DriveID != "one") && task.Status != "running" {
			testContext.Fatalf("其他作用域被修改: %+v", task)
		}
	}
}
