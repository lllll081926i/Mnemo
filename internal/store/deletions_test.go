package store

import (
	"mnemo-go/internal/model"
	"testing"
)

func TestDeletionHistoryIsolationAndRestart(testContext *testing.T) {
	directory := testContext.TempDir()
	storage, err := Open(directory)
	if err != nil {
		testContext.Fatal(err)
	}
	for _, identity := range [][2]string{{"one", "a"}, {"one", "b"}, {"two", "a"}} {
		if err := storage.SaveDeletionRecords([]model.DeletionRecord{{UserID: identity[0], DriveID: identity[1], FileID: "same", Name: "同名", IDKind: "driveItem", Status: "deleted"}}); err != nil {
			testContext.Fatal(err)
		}
	}
	reopened, err := Open(directory)
	if err != nil {
		testContext.Fatal(err)
	}
	if err := reopened.ClearDeletionRecords("one", "a"); err != nil {
		testContext.Fatal(err)
	}
	for _, identity := range [][2]string{{"one", "b"}, {"two", "a"}} {
		records, err := reopened.ListDeletionRecords(identity[0], identity[1])
		if err != nil || len(records) != 1 {
			testContext.Fatalf("跨账号或空间清除: %+v %v", records, err)
		}
	}
	if err := reopened.ClearDeletionRecordsForAccount("one"); err != nil {
		testContext.Fatal(err)
	}
	records, err := reopened.ListDeletionRecords("two", "a")
	if err != nil || len(records) != 1 {
		testContext.Fatal("账号清除误删其他账号")
	}
	if err := reopened.SaveDeletionRecords([]model.DeletionRecord{{UserID: "one", FileID: "same", IDKind: "recycleBinItem"}}); err == nil {
		testContext.Fatal("混入企业回收站 ID")
	}
}
