package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"mnemo-go/internal/model"
)

func TestCachedIndexCompletenessAndInvalidation(testContext *testing.T) {
	storage, err := Open(testContext.TempDir())
	if err != nil {
		testContext.Fatal(err)
	}
	key := "webdav|user|drive|list|root|"
	files := []model.File{{FileID: "file", Name: "report.txt"}}
	if err := storage.SaveDirectoryCachePage(key, files, "next"); err != nil {
		testContext.Fatal(err)
	}
	for _, expectedPartial := range []bool{true, false} {
		if !expectedPartial {
			if err := storage.SaveDirectoryCache(key, files); err != nil {
				testContext.Fatal(err)
			}
		}
		for repeat := 0; repeat < 2; repeat++ {
			results, err := storage.SearchDirectoryCache("report")
			if err != nil || len(results) != 1 || results[0].Partial != expectedPartial {
				testContext.Fatalf("完整性或索引失效处理错误: %+v %v", results, err)
			}
		}
	}
}

func TestCacheReadErrorDoesNotBecomeEmptySuccess(testContext *testing.T) {
	storage, err := Open(testContext.TempDir())
	if err != nil {
		testContext.Fatal(err)
	}
	if err := os.MkdirAll(storage.path("cache"), 0o700); err != nil {
		testContext.Fatal(err)
	}
	if err := os.WriteFile(storage.path("cache/directories"), []byte("不是目录"), 0o600); err != nil {
		testContext.Fatal(err)
	}
	for repeat := 0; repeat < 2; repeat++ {
		if _, err := storage.SearchDirectoryCache("report"); err == nil {
			testContext.Fatal("目录读取失败被记为成功的空索引")
		}
	}
}

func TestShareHistoryIdentityIncludesDriveAndEmptyIDs(testContext *testing.T) {
	storage, err := Open(testContext.TempDir())
	if err != nil {
		testContext.Fatal(err)
	}
	for _, entry := range []model.ShareHistoryEntry{
		{AccountID: "user", DriveID: "one", ShareID: "same", ShareURL: "https://example/one"},
		{AccountID: "user", DriveID: "two", ShareID: "same", ShareURL: "https://example/two"},
		{AccountID: "user", DriveID: "one", ShareURL: "https://example/three"},
		{AccountID: "user", DriveID: "one", ShareURL: "https://example/four"},
	} {
		if err := storage.SaveShareHistory(entry); err != nil {
			testContext.Fatal(err)
		}
	}
	if err := storage.DeleteShareHistoryScoped("user", "one", "same", ""); err != nil {
		testContext.Fatal(err)
	}
	entries, err := storage.ListShareHistory("user")
	if err != nil || len(entries) != 3 {
		testContext.Fatalf("分享身份串空间或空 ID 覆盖: %+v %v", entries, err)
	}
	for _, entry := range entries {
		if entry.RemoteStatus != "unchecked" {
			testContext.Fatalf("本地记录误当远端确认: %+v", entry)
		}
	}
}

func TestDurableWritePreservesTargetOnFailureAndConcurrentWrites(testContext *testing.T) {
	path := filepath.Join(testContext.TempDir(), "state.json")
	if err := durableWrite(path, []byte(`{"value":"old"}`)); err != nil {
		testContext.Fatal(err)
	}
	if err := os.Mkdir(path+".tmp", 0o700); err != nil {
		testContext.Fatal(err)
	}
	if err := durableWrite(path, []byte(`{"value":"new"}`)); err == nil {
		testContext.Fatal("临时路径故障未传播")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != `{"value":"old"}` {
		testContext.Fatalf("故障破坏旧文件: %s %v", data, err)
	}
	other := filepath.Join(testContext.TempDir(), "concurrent.json")
	var workers sync.WaitGroup
	for worker := 0; worker < 30; worker++ {
		workers.Go(func() {
			if err := durableWrite(other, []byte(`{"value":"complete"}`)); err != nil {
				testContext.Error(err)
			}
		})
	}
	workers.Wait()
	data, err = os.ReadFile(other)
	if err != nil || !json.Valid(data) {
		testContext.Fatalf("并发写入损坏文件: %s %v", data, err)
	}
}
