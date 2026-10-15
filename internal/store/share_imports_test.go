package store

import (
	"testing"

	"mnemo-go/internal/model"
)

func TestShareImportReceiptScopeAndAccountRemoval(testContext *testing.T) {
	storage, err := Open(testContext.TempDir())
	if err != nil {
		testContext.Fatal(err)
	}
	for _, identity := range [][2]string{{"one", "a"}, {"one", "b"}, {"two", "a"}} {
		err := storage.SaveShareImport(model.ShareImportReceipt{ID: "same", AccountID: identity[0], DriveID: identity[1], Items: []model.ShareImportItem{{SourceID: "file", Status: "submitted"}}})
		if err != nil {
			testContext.Fatal(err)
		}
	}
	if err := storage.ClearShareImportsForAccount("one"); err != nil {
		testContext.Fatal(err)
	}
	remaining, err := storage.ListShareImports("two", "a")
	if err != nil || len(remaining) != 1 || !remaining[0].HistorySaved {
		testContext.Fatalf("误清其他账号: %+v %v", remaining, err)
	}
	removed, err := storage.ListShareImports("one", "b")
	if err != nil || len(removed) != 0 {
		testContext.Fatal("未清理账号的所有空间")
	}
}
