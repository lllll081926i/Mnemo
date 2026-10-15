package app

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
	"mnemo-go/internal/store"
)

func shareImportTestApp(testContext *testing.T) (*App, *drive.ShareImportSession) {
	testContext.Helper()
	storage, err := store.Open(testContext.TempDir())
	if err != nil {
		testContext.Fatal(err)
	}
	drive.SetTokenResolver(func(_, _ string) (*model.TokenInfo, error) {
		return &model.TokenInfo{AccessToken: "token", DeviceID: "device"}, nil
	})
	previous := netx.TestTransportHook
	testContext.Cleanup(func() { drive.SetTokenResolver(nil); netx.TestTransportHook = previous })
	return &App{store: storage}, &drive.ShareImportSession{Provider: model.ProviderPikpak, ShareID: "share", PassCodeToken: "secret-pass-token", Password: "secret-password", Files: []drive.ShareImportFile{{FileID: "one"}, {FileID: "two"}}}
}

func shareImportResponse(request *http.Request, body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Request: request, Body: io.NopCloser(strings.NewReader(body))}
}

func TestShareImportCheckpointDeduplicationAndTaskVerification(testContext *testing.T) {
	application, session := shareImportTestApp(testContext)
	posts := 0
	queries := map[string]int{}
	netx.TestTransportHook = favoriteTestTransport(func(request *http.Request) (*http.Response, error) {
		if request.Method == http.MethodPost {
			posts++
			receipts, err := application.store.ListShareImports("pikpak_receipts", "space")
			if err != nil || len(receipts) != 1 || receipts[0].Items[0].Status != "pending" {
				testContext.Fatalf("远端写入前缺少检查点: %+v %v", receipts, err)
			}
			return shareImportResponse(request, `{"tasks":[{"id":"task-one"},{"task_id":"task-two"}]}`), nil
		}
		if request.Method != http.MethodGet || !strings.HasPrefix(request.URL.Path, "/drive/v1/tasks/") {
			testContext.Fatalf("意外请求: %s %s", request.Method, request.URL)
		}
		identity := strings.TrimPrefix(request.URL.Path, "/drive/v1/tasks/")
		queries[identity]++
		return shareImportResponse(request, `{"id":"`+identity+`","phase":"PHASE_TYPE_COMPLETE"}`), nil
	})
	receipt, err := application.SaveImportedShareDetailed("pikpak_receipts", "space", session, []string{"one", "two"}, "root")
	if err != nil || !receipt.HistorySaved || receipt.Items[0].Status != "submitted" || len(receipt.Items[0].TaskIDs) != 2 {
		testContext.Fatalf("受理回执错误: %+v %v", receipt, err)
	}
	duplicate, err := application.SaveImportedShareDetailed("pikpak_receipts", "space", session, []string{"two", "one"}, "*")
	if err != nil || duplicate.ID != receipt.ID || posts != 1 {
		testContext.Fatalf("同批次或根目录别名导致重复提交: %+v %v %d", duplicate, err, posts)
	}
	if _, err := application.SaveImportedShareDetailed("pikpak_receipts", "space", session, []string{"one"}, "root"); err == nil || posts != 1 {
		testContext.Fatal("子集选择重复提交")
	}
	reopened, err := store.Open(application.store.Dir())
	if err != nil {
		testContext.Fatal(err)
	}
	application = &App{store: reopened}
	if _, err := application.SaveImportedShareDetailed("pikpak_receipts", "space", session, []string{"one", "two"}, "root"); err != nil || posts != 1 {
		testContext.Fatal("重启后重复提交")
	}
	verified, err := application.RefreshShareImportReceipt("pikpak_receipts", "space", receipt.ID)
	if err != nil || verified.Items[0].Status != "completed" || verified.Items[1].Status != "completed" || queries["task-one"] != 1 || queries["task-two"] != 1 {
		testContext.Fatalf("批次任务没有完整对账或重复查询: %+v %v %+v", verified, err, queries)
	}
	if _, err := application.RefreshShareImportReceipt("pikpak_other", "space", receipt.ID); err == nil {
		testContext.Fatal("跨账号访问转存回执")
	}
	data, err := os.ReadFile(filepath.Join(reopened.Dir(), "share_imports.json"))
	if err != nil || strings.Contains(string(data), "secret-") || strings.Contains(string(data), "passCodeToken") {
		testContext.Fatalf("转存账本泄露会话凭据: %v", err)
	}
}

func TestShareImportLostResponseDoesNotResubmit(testContext *testing.T) {
	application, session := shareImportTestApp(testContext)
	posts := 0
	netx.TestTransportHook = favoriteTestTransport(func(request *http.Request) (*http.Response, error) {
		posts++
		return nil, errors.New("lost response with secret-provider-token")
	})
	receipt, err := application.SaveImportedShareDetailed("pikpak_receipts", "space", session, []string{"one"}, "root")
	if err != nil || receipt.Items[0].Status != "unknown" || !receipt.HistorySaved {
		testContext.Fatalf("未知提交未保存: %+v %v", receipt, err)
	}
	if _, err := application.SaveImportedShareDetailed("pikpak_receipts", "space", session, []string{"one"}, "root"); err != nil || posts != 1 {
		testContext.Fatal("丢失回执后重复创建")
	}
	records, err := application.ListShareImportReceipts("pikpak_receipts", "space")
	if err != nil || strings.Contains(records[0].Message, "secret-provider") {
		testContext.Fatal("持久化了原始服务端错误")
	}
}

func TestShareImportCheckpointAndFinalSaveFailures(testContext *testing.T) {
	for _, finalFailure := range []bool{false, true} {
		testContext.Run(map[bool]string{false: "写前失败", true: "回执保存失败"}[finalFailure], func(testContext *testing.T) {
			application, session := shareImportTestApp(testContext)
			blocker := filepath.Join(application.store.Dir(), "share_imports.json.tmp")
			if !finalFailure {
				if err := os.Mkdir(blocker, 0o700); err != nil {
					testContext.Fatal(err)
				}
			}
			posts := 0
			netx.TestTransportHook = favoriteTestTransport(func(request *http.Request) (*http.Response, error) {
				posts++
				if err := os.Mkdir(blocker, 0o700); err != nil {
					testContext.Fatal(err)
				}
				return shareImportResponse(request, `{"task_id":"task"}`), nil
			})
			receipt, err := application.SaveImportedShareDetailed("pikpak_receipts", "space", session, []string{"one"}, "root")
			if !finalFailure {
				if err == nil || posts != 0 {
					testContext.Fatal("检查点失败仍提交")
				}
				return
			}
			if err != nil || receipt.HistorySaved || !reflect.DeepEqual(receipt.Items[0].TaskIDs, []string{"task"}) {
				testContext.Fatalf("本地失败丢弃远端回执: %+v %v", receipt, err)
			}
			if _, err := application.SaveImportedShareDetailed("pikpak_receipts", "space", session, []string{"one"}, "root"); err != nil || posts != 1 {
				testContext.Fatal("最终保存失败后重复提交")
			}
		})
	}
}

func TestInvalidShareSelectionDoesNotCreateCheckpoint(testContext *testing.T) {
	application, session := shareImportTestApp(testContext)
	for _, selection := range [][]string{{"one", "one"}, {"outside"}, {""}, nil} {
		if _, err := application.SaveImportedShareDetailed("pikpak_receipts", "space", session, selection, "root"); err == nil {
			testContext.Fatal("无效选择被受理")
		}
	}
	session.PassCodeToken = ""
	if _, err := application.SaveImportedShareDetailed("pikpak_receipts", "space", session, []string{"one"}, "root"); err == nil {
		testContext.Fatal("缺少会话凭据仍受理")
	}
	receipts, err := application.ListShareImportReceipts("pikpak_receipts", "space")
	if err != nil || len(receipts) != 0 {
		testContext.Fatalf("无效请求污染账本: %+v %v", receipts, err)
	}
}

func TestConcurrentOverlappingShareImportsOnlySubmitOnce(testContext *testing.T) {
	application, session := shareImportTestApp(testContext)
	posts := 0
	netx.TestTransportHook = favoriteTestTransport(func(request *http.Request) (*http.Response, error) {
		posts++
		return shareImportResponse(request, `{"task_id":"task"}`), nil
	})
	var workers sync.WaitGroup
	for index := range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			selection := []string{"one"}
			if index%2 == 0 {
				selection = append(selection, "two")
			}
			_, _ = application.SaveImportedShareDetailed("pikpak_receipts", "space", session, selection, "root")
		}()
	}
	workers.Wait()
	if posts != 1 {
		testContext.Fatalf("并发重叠选择发生重复提交: %d", posts)
	}
}
