package guangya

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

func mockOfflineHTTP(t *testing.T, handler guangyaRoundTripFunc) {
	previous := netx.TestTransportHook
	netx.TestTransportHook = handler
	t.Cleanup(func() { netx.TestTransportHook = previous })
}

func offlineAccount() drive.Context {
	return drive.Context{UserID: "guangya_offline", DriveID: "offline", Token: &model.TokenInfo{
		AccessToken: "access", RefreshToken: mustJSON(&Session{AccessToken: "access", RefreshToken: "refresh", DeviceID: "fixed-device", ClientID: clientID}),
	}}
}

func TestGuangyaOfflineCreateUsesResolvedResource(t *testing.T) {
	if !drive.Require(providerID).Caps.OfflineDownload || !(&Driver{}).Capabilities().OfflineDownload {
		t.Fatal("光鸭云离线已实现但没有开启")
	}
	for _, kind := range []string{"http", "magnet", "ed2k"} {
		t.Run(kind, func(t *testing.T) {
			link := "https://example.test/file"
			if kind == "magnet" {
				link = "magnet:?xt=urn:btih:test"
			} else if kind == "ed2k" {
				link = "ed2k://|file|test|1|hash|/"
			}
			resolvedLink := link
			if kind == "http" {
				resolvedLink = "https://example.test/resolved-file"
			}
			calls := 0
			mockOfflineHTTP(t, func(request *http.Request) (*http.Response, error) {
				calls++
				if request.URL.Host != "api.guangyapan.com" || request.Method != http.MethodPost || request.Header.Get("Authorization") != "Bearer access" || request.Header.Get("Did") != "fixed-device" {
					t.Fatal("云离线请求使用了错误服务或账号")
				}
				var body map[string]any
				if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				switch request.URL.Path {
				case "/nd.bizcloudcollection.s/v1/resolve_res":
					if body["url"] != link {
						t.Fatal("链接解析参数不正确")
					}
					resource := ""
					if kind == "magnet" {
						resource = `,"btResInfo":{"subfilesNum":3,"excludeIndices":[1],"subfiles":[{"fileIndex":0},{"isDir":true,"subfiles":[{"fileIndex":1},{"fileIndex":"2"}]}]}`
					}
					return guangyaResponse(request, 200, fmt.Sprintf(`{"code":0,"data":{"url":%q%s}}`, resolvedLink, resource)), nil
				case "/nd.bizcloudcollection.s/v1/create_task":
					if body["url"] != resolvedLink || body["parentId"] != "" || body["newName"] != "测试资源" {
						t.Fatalf("任务参数不正确：%+v", body)
					}
					if kind == "magnet" {
						indexes, ok := body["fileIndexes"].([]any)
						if !ok || len(indexes) != 2 || indexes[0] != float64(0) || indexes[1] != float64(2) {
							t.Fatalf("子文件索引错误：%+v", body)
						}
					} else if _, present := body["fileIndexes"]; present {
						t.Fatal("普通链接不应发送子文件索引")
					}
					return guangyaResponse(request, 200, `{"code":0,"data":{"taskId":"task","fileId":"file"}}`), nil
				}
				return nil, fmt.Errorf("意外路径 %s", request.URL.Path)
			})
			taskID, fileID, err := (&Driver{}).OfflineCreate(t.Context(), offlineAccount(), link, "测试资源", RootID)
			if err != nil || taskID != "task" || fileID != "file" || calls != 2 {
				t.Fatalf("创建任务失败：%s %s %v，请求=%d", taskID, fileID, err, calls)
			}
		})
	}
}

func TestGuangyaOfflineListFollowsPageAndMapsStatus(t *testing.T) {
	calls := 0
	mockOfflineHTTP(t, func(request *http.Request) (*http.Response, error) {
		calls++
		var body map[string]any
		json.NewDecoder(request.Body).Decode(&body)
		if request.URL.Path != "/nd.bizcloudcollection.s/v1/list_task" || body["pageSize"] != float64(50) || len(body["status"].([]any)) != 6 {
			t.Fatal("分页或状态筛选参数不正确")
		}
		if calls == 1 {
			return guangyaResponse(request, 200, `{"code":0,"data":{"list":[{"taskId":"running","status":1,"progress":"43.5","totalSize":"9007199254740993"}],"hasMore":true,"cursor":"next"}}`), nil
		}
		if body["page"] != float64(1) {
			t.Fatal("未按官方页码查询第二页")
		}
		return guangyaResponse(request, 200, `{"code":0,"data":{"list":[{"taskId":"done","status":"2","progress":80},{"taskId":"partial","status":5,"progress":60},{"taskId":"running","status":1}],"hasMore":false}}`), nil
	})
	tasks, err := (&Driver{}).OfflineList(t.Context(), offlineAccount())
	if err != nil || len(tasks) != 3 || calls != 2 {
		t.Fatalf("分页失败：%+v %v", tasks, err)
	}
	if tasks[0].Progress != 44 || tasks[0].FileSize != 9007199254740993 || tasks[1].Status != "completed" || tasks[1].Progress != 100 || tasks[2].Status != "partial" || tasks[2].Progress != 60 || tasks[2].Message == "" {
		t.Fatalf("状态、进度或字节数错误：%+v", tasks)
	}
}

func TestGuangyaOfflineStatusAndProgress(t *testing.T) {
	for _, testCase := range []struct {
		state, progress, status string
		want                    int
	}{
		{"0", "-5", "queued", 0}, {"1", "200", "running", 100}, {"2", "0", "completed", 100},
		{"3", "30", "failed", 30}, {"4", "40", "canceled", 40}, {"5", "50", "partial", 50}, {"99", "0", "unknown", 0},
	} {
		task, err := (offlineRemoteTask{TaskID: "task", Status: json.Number(testCase.state), Progress: json.Number(testCase.progress)}).model()
		if err != nil || task.Status != testCase.status || task.Progress != testCase.want {
			t.Fatalf("状态转换错误：%+v %v", task, err)
		}
	}
}

func TestGuangyaOfflineRejectsInvalidSubfileIndexes(t *testing.T) {
	for _, raw := range []string{
		`{"subfiles":[{}]}`,
		`{"subfiles":[{"fileIndex":-1}]}`,
		`{"subfiles":[{"fileIndex":0}],"excludeIndices":[0]}`,
		`{"subfiles":[{"fileIndex":0}],"excludeIndices":[-1]}`,
		`{"subfilesNum":1,"subfiles":[]}`,
	} {
		var resource offlineResource
		if err := json.Unmarshal([]byte(raw), &resource); err != nil {
			t.Fatal(err)
		}
		if _, err := resource.selectedIndexes(); err == nil {
			t.Fatalf("接受了无效子文件索引：%s", raw)
		}
	}
}

func TestGuangyaOfflineRejectsMalformedResponses(t *testing.T) {
	for _, testCase := range []struct {
		operation, response string
	}{
		{"create", `{"code":157,"msg":"空间不足"}`},
		{"create", `{"code":0,"data":{"url":"https://example.test/file"}}`},
		{"list", `{"code":403,"msg":"无权限"}`},
		{"list", `{"code":0}`},
		{"list", `{"code":0,"data":{"list":[{"status":1}]}}`},
		{"list", `{"code":0,"data":{"list":[{"taskId":"repeated","status":1}],"hasMore":true}}`},
		{"delete", `{"code":403,"msg":"无权限"}`},
		{"delete", `{}`},
	} {
		t.Run(testCase.operation+testCase.response, func(t *testing.T) {
			mockOfflineHTTP(t, func(request *http.Request) (*http.Response, error) {
				return guangyaResponse(request, 200, testCase.response), nil
			})
			var err error
			switch testCase.operation {
			case "create":
				_, _, err = (&Driver{}).OfflineCreate(t.Context(), offlineAccount(), "https://example.test/file", "", "")
			case "list":
				_, err = (&Driver{}).OfflineList(t.Context(), offlineAccount())
			case "delete":
				err = (&Driver{}).OfflineDelete(t.Context(), offlineAccount(), []string{"task"}, false)
			}
			if err == nil {
				t.Fatal("无效响应被当作成功")
			}
		})
	}
}

func TestGuangyaOfflineDeleteWaitsForFileTask(t *testing.T) {
	for _, deleteFiles := range []bool{false, true} {
		t.Run(fmt.Sprint(deleteFiles), func(t *testing.T) {
			calls := 0
			mockOfflineHTTP(t, func(request *http.Request) (*http.Response, error) {
				calls++
				var body map[string]any
				json.NewDecoder(request.Body).Decode(&body)
				if strings.HasSuffix(request.URL.Path, "/delete_task") {
					if body["deleteFile"] != deleteFiles || body["taskIds"].([]any)[0] != "task" {
						t.Fatal("任务删除参数错误")
					}
					return guangyaResponse(request, 200, `{"code":0,"data":{"deleteFileTaskId":"file-task"}}`), nil
				}
				if request.URL.Path != "/userres/v1/get_task_status" || body["taskId"] != "file-task" {
					t.Fatal("未查询关联的文件删除任务")
				}
				return guangyaResponse(request, 200, `{"code":0,"data":{"status":2}}`), nil
			})
			if err := (&Driver{}).OfflineDelete(t.Context(), offlineAccount(), []string{"task"}, deleteFiles); err != nil {
				t.Fatal(err)
			}
			if calls != 1 && !deleteFiles || calls != 2 && deleteFiles {
				t.Fatal("未正确区分仅删除记录和删除网盘文件")
			}
		})
	}
}

func TestGuangyaOfflineRefreshPersistsCredentials(t *testing.T) {
	account := offlineAccount()
	account.TokenSnapshot = drive.CloneToken(account.Token)
	persisted := false
	drive.SetTokenUpdater(func(userID, driveID string, token *model.TokenInfo) error {
		persisted = userID == account.UserID && driveID == account.DriveID && token.AccessToken == "new-access" && strings.Contains(token.RefreshToken, "new-refresh")
		return nil
	})
	t.Cleanup(func() { drive.SetTokenUpdater(nil) })
	calls := 0
	mockOfflineHTTP(t, func(request *http.Request) (*http.Response, error) {
		calls++
		if request.URL.Host == "account.guangyapan.com" {
			return guangyaResponse(request, 200, `{"access_token":"new-access","refresh_token":"new-refresh"}`), nil
		}
		if calls == 1 {
			return guangyaResponse(request, 401, `{}`), nil
		}
		if request.Header.Get("Authorization") != "Bearer new-access" || request.Header.Get("Did") != "fixed-device" {
			t.Fatal("续期后未保持账号与设备身份")
		}
		return guangyaResponse(request, 200, `{"code":0,"data":{"list":[]}}`), nil
	})
	if _, err := (&Driver{}).OfflineList(t.Context(), account); err != nil || calls != 3 || !persisted {
		t.Fatalf("刷新凭据没有持久化：%v，请求=%d，保存=%v", err, calls, persisted)
	}
}

func TestGuangyaOfflineCancellationAndRateLimit(t *testing.T) {
	calls := 0
	mockOfflineHTTP(t, func(request *http.Request) (*http.Response, error) {
		calls++
		return guangyaResponse(request, 429, `{"code":429,"msg":"请求频繁"}`), nil
	})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := (&Driver{}).OfflineList(ctx, offlineAccount()); !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatal("取消后仍发出了请求")
	}
	if _, _, err := (&Driver{}).OfflineCreate(t.Context(), offlineAccount(), "https://example.test/file", "", ""); err == nil || calls != 1 {
		t.Fatal("限流被忽略或重复登录")
	}
	if _, _, err := (&Driver{}).OfflineCreate(t.Context(), offlineAccount(), "file:///test", "", ""); err == nil || calls != 1 {
		t.Fatal("接受了不支持的链接")
	}
	if err := (&Driver{}).OfflineDelete(t.Context(), offlineAccount(), []string{""}, false); err == nil || calls != 1 {
		t.Fatal("空任务 ID 发出了请求")
	}
}
