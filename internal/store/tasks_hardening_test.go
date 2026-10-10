package store

import (
	"testing"

	"mnemo-go/internal/model"
)

func TestOfflineTaskIDsIsolatedByAccountAndDrive(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	items := []model.OfflineTask{
		{ID: "same-task", UserID: "alice", DriveID: "main", Status: "running"},
		{ID: "same-task", UserID: "bob", DriveID: "main", Status: "running"},
		{ID: "same-task", UserID: "alice", DriveID: "other", Status: "running"},
	}
	for i := range items {
		if err := st.SaveOfflineTask(&items[i]); err != nil { t.Fatal(err) }
	}
	got, err := st.ListOfflineTasks()
	if err != nil || len(got) != 3 { t.Fatalf("saved tasks: %v, %v", got, err) }
	changed := model.OfflineTask{ID: "same-task", UserID: "alice", DriveID: "main", Status: "complete"}
	if err := st.SaveOfflineTask(&changed); err != nil { t.Fatal(err) }
	got, err = st.ListOfflineTasks()
	if err != nil || len(got) != 3 { t.Fatalf("updated tasks: %v, %v", got, err) }
	for _, task := range got {
		if task.UserID == "alice" && task.DriveID == "main" && task.Status != "complete" {
			t.Errorf("scoped update lost: %+v", task)
		}
		if !(task.UserID == "alice" && task.DriveID == "main") && task.Status != "running" {
			t.Errorf("other account unexpectedly changed: %+v", task)
		}
	}
	if err := st.DeleteOfflineTaskScoped("alice", "main", "same-task"); err != nil { t.Fatal(err) }
	got, err = st.ListOfflineTasks()
	if err != nil || len(got) != 2 { t.Fatalf("scoped delete: %v, %v", got, err) }
	for _, task := range got {
		if task.UserID == "alice" && task.DriveID == "main" { t.Fatal("deleted record still present") }
	}
}

func TestClearUploadTasksOnlyRemovesCompleted(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil { t.Fatal(err) }
	jobs := []model.UploadingUI{
		{UploadID: "done", Upload: model.UploadState{IsCompleted: true}},
		{UploadID: "failed", Upload: model.UploadState{IsFailed: true}},
		{UploadID: "paused", Upload: model.UploadState{IsStop: true}},
		{UploadID: "active", Upload: model.UploadState{IsDowning: true}},
	}
	for i := range jobs {
		if err := st.SaveUploadTask(&jobs[i]); err != nil { t.Fatal(err) }
	}
	if err := st.ClearUploadTasks(); err != nil { t.Fatal(err) }
	got, err := st.ListUploadTasks()
	if err != nil || len(got) != 3 { t.Fatalf("unexpected remaining tasks: %v, %v", got, err) }
	for _, task := range got {
		if task.UploadID == "done" { t.Fatal("completed task remains") }
	}
}
