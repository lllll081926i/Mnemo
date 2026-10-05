package app

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
	"mnemo-go/internal/store"
)

func TestVerifyMovedFileUsesRecordedSourceHash(testContext *testing.T) {
	for _, testCase := range []struct{ name, metadata, status string }{
		{"匹配", `"file":{"hashes":{"sha1Hash":"abc"}}`, "hash"},
		{"变化", `"file":{"hashes":{"sha1Hash":"def"}}`, "mismatch"},
		{"目录伪装为文件", `"folder":{"childCount":0}`, "mismatch"},
	} {
		testContext.Run(testCase.name, func(testContext *testing.T) {
			previous := netx.TestTransportHook
			testContext.Cleanup(func() { netx.TestTransportHook = previous; drive.SetTokenResolver(nil) })
			drive.SetTokenResolver(func(userID, _ string) (*model.TokenInfo, error) {
				if strings.Contains(userID, "source") {
					testContext.Fatal("移动后校验不应依赖已经删除的来源")
				}
				return &model.TokenInfo{AccessToken: "token", TokenFrom: "onedrive"}, nil
			})
			netx.TestTransportHook = favoriteTestTransport(func(request *http.Request) (*http.Response, error) {
				if !strings.HasSuffix(request.URL.Path, "/items/target") {
					testContext.Fatalf("未知目标查询: %s", request.URL)
				}
				body := fmt.Sprintf(`{"id":"target","name":"file","size":3,%s}`, testCase.metadata)
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
			})
			storage, err := store.Open(testContext.TempDir())
			if err != nil {
				testContext.Fatal(err)
			}
			job := &model.MigrateJob{ID: "verify", Status: "completed", Move: true, SrcUser: "onedrive_source", SrcDrive: "source", DstUser: "onedrive_target_" + testCase.name, DstDrive: "target", Items: map[string]model.MigrateItem{"source": {ID: "source", Status: "completed", TargetID: "target", Size: 3, SourceRecorded: true, SourceHash: "sha1:ABC"}}}
			if err := storage.SaveMigrateJob(job); err != nil {
				testContext.Fatal(err)
			}
			application := NewApp()
			application.store = storage
			results, err := application.VerifyMigration(job.ID)
			if err != nil || len(results) != 1 || results[0].Status != testCase.status {
				testContext.Fatalf("迁移校验错误: %+v, %v", results, err)
			}
		})
	}
}
