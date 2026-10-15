package store

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"mnemo-go/internal/model"
)

func TestOfflineHistoryMigratesAtOpen(testContext *testing.T) {
	directory := testContext.TempDir()
	path := filepath.Join(directory, offlineFile)
	raw := []byte(`[{"id":"one","user_id":"alice","url":"https://user:password@example.test/private-token/file?sig=secret#secret"}]`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		testContext.Fatal(err)
	}
	storage, err := Open(directory)
	if err != nil {
		testContext.Fatal(err)
	}
	items, err := storage.ListOfflineTasks()
	if err != nil || len(items) != 1 || items[0].URL != "https://example.test" {
		testContext.Fatalf("迁移结果错误: %+v %v", items, err)
	}
	first, err := os.ReadFile(path)
	if err != nil || bytes.Contains(first, []byte("secret")) || bytes.Contains(first, []byte("private-token")) {
		testContext.Fatal("磁盘仍有敏感链接", err)
	}
	if _, err := Open(directory); err != nil {
		testContext.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(first, second) {
		testContext.Fatal("重复迁移改变记录", err)
	}
}

func TestOfflineMigrationFailurePreservesOriginal(testContext *testing.T) {
	for _, brokenJSON := range []bool{false, true} {
		directory := testContext.TempDir()
		original := []byte(`[{"id":"one","url":"https://example.test?sig=secret"}]`)
		if brokenJSON {
			original = []byte(`{"broken":`)
		} else if err := os.Mkdir(filepath.Join(directory, offlineFile+".tmp"), 0o700); err != nil {
			testContext.Fatal(err)
		}
		if !brokenJSON {
			if err := os.WriteFile(filepath.Join(directory, offlineFile+".tmp", "occupied"), []byte("test"), 0o600); err != nil {
				testContext.Fatal(err)
			}
		}
		path := filepath.Join(directory, offlineFile)
		if err := os.WriteFile(path, original, 0o600); err != nil {
			testContext.Fatal(err)
		}
		storage, err := Open(directory)
		if err != nil {
			testContext.Fatal("离线历史损坏不应阻止其他存储服务", err)
		}
		if _, err := storage.ListOfflineTasks(); err == nil {
			testContext.Fatal("读取接口未返回迁移错误")
		}
		if _, err := storage.GetSettings(); err != nil {
			testContext.Fatal("迁移失败影响了设置服务", err)
		}
		remaining, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(remaining, original) {
			testContext.Fatal("迁移失败破坏原始记录", err)
		}
	}
}

func TestOfflineStoreSanitizesAndRequiresExactScope(testContext *testing.T) {
	storage, err := Open(testContext.TempDir())
	if err != nil {
		testContext.Fatal(err)
	}
	for _, driveID := range []string{"", "other"} {
		input := model.OfflineTask{ID: "same", UserID: "alice", DriveID: driveID, URL: "https://example.test/private?sig=secret"}
		if err := storage.SaveOfflineTask(&input); err != nil {
			testContext.Fatal(err)
		}
		if input.URL != "https://example.test/private?sig=secret" {
			testContext.Fatal("保存修改调用方原链接")
		}
	}
	if err := storage.DeleteOfflineTaskScoped("", "", "same"); err == nil {
		testContext.Fatal("允许无账号作用域删除")
	}
	if err := storage.DeleteOfflineTaskScoped("alice", "", "same"); err != nil {
		testContext.Fatal(err)
	}
	items, err := storage.ListOfflineTasks()
	if err != nil || len(items) != 1 || items[0].DriveID != "other" || items[0].URL != "https://example.test" {
		testContext.Fatalf("作用域或脱敏错误: %+v %v", items, err)
	}
}
