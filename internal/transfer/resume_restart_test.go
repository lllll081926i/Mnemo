package transfer

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"mnemo-go/internal/model"
	"mnemo-go/internal/store"
)

func TestAllProviderTasksRestoreCheckpointProgress(t *testing.T) {
	providers := []string{"pikpak", "onedrive", "dropbox", "pan123", "lanzou", "ilanzou", "pan139", "pan189", "aliopen", "guangya", "thunder", "webdav", "s3"}
	for _, provider := range providers {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%v", provider, stream), func(t *testing.T) {
				root := t.TempDir()
				storage, err := store.Open(root)
				if err != nil {
					t.Fatal(err)
				}
				task := &model.DownloadTask{ID: "task", UserID: provider + "-account", DriveID: "drive", FileID: "file", Name: "file.bin", LocalPath: filepath.Join(root, "file.bin"), Status: "downloading", Size: 10000, Speed: 500, Stage: "connecting"}
				if err := storage.SaveDownloadTask(task); err != nil {
					t.Fatal(err)
				}
				resourceID := task.UserID + "\x00" + task.DriveID + "\x00" + task.FileID
				saved := map[string]any{"total": 10000, "resource_hash": fmt.Sprintf("%x", sha256.Sum256([]byte(resourceID))), "etag": `"stable"`}
				partLength := 10000
				if stream {
					saved["stream"], saved["stream_bytes"] = true, 6000
					partLength = 6000
				} else {
					saved["chunk"], saved["done"], saved["partial_bytes"] = 5000, []bool{true, false}, []int64{0, 1000}
				}
				data, err := json.Marshal(saved)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(task.LocalPath+".part", bytes.Repeat([]byte{1}, partLength), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(task.LocalPath+".state.json", data, 0600); err != nil {
					t.Fatal(err)
				}
				manager, err := NewManager(storage, root, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer manager.Shutdown()
				restored, ok := manager.get(task.ID)
				if !ok || restored.Status != "paused" || restored.Downloaded != 6000 || restored.Progress != 60 || restored.Size != 10000 || restored.Speed != 0 || restored.Stage != "" {
					t.Fatalf("重启恢复未使用有效检查点：%+v", restored)
				}
				persisted, err := storage.ListDownloadTasks()
				if err != nil || len(persisted) != 1 || persisted[0].Downloaded != 6000 || persisted[0].Progress != 60 {
					t.Fatalf("恢复后的进度没有落盘：%v, %+v", err, persisted)
				}
			})
		}
	}
}
