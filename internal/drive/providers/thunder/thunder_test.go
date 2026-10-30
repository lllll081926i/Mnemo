package thunder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (transport roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func response(request *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}
}

func mockHTTP(t *testing.T, handler roundTripFunc) {
	previous := netx.TestTransportHook
	netx.TestTransportHook = handler
	t.Cleanup(func() { netx.TestTransportHook = previous })
}

func accountFor(user string) drive.Context {
	token := &model.TokenInfo{AccessToken: "access-" + user, TokenType: "Bearer"}
	cl := &client{token: token, session: &session{UserID: user, DeviceID: "0123456789abcdef0123456789abcdef", RefreshToken: "refresh-" + user}}
	cl.saveSession()
	return drive.Context{UserID: token.UserID, DriveID: user, TokenFrom: model.ProviderThunder, Token: token, TokenSnapshot: drive.CloneToken(token)}
}

func TestThunderRegistrationAndLogin(t *testing.T) {
	registered := drive.Require(model.ProviderThunder)
	caps := registered.Caps
	if !caps.Download || !caps.OfflineDownload || !caps.CloudVideoPreview || !caps.Copy || !caps.Move || !caps.Rename || !caps.RecycleBin || !caps.Upload || caps.Search || !caps.CreateShare || !caps.TrashView || !caps.TrashRestore || !caps.PermanentDelete || !caps.CancelCreatedShares || caps.SharePassword || caps.ImportShare || caps.UploadMode != drive.UploadModeQueue {
		t.Fatalf("迅雷能力声明与实现范围不符: %+v", caps)
	}
	if drive.ResolveProvider("thunder_123", "", "") != model.ProviderThunder || registered.Meta.RootKey != RootID || registered.Meta.Icon != "drive-icons/thunder.svg" {
		t.Fatal("迅雷账号命名空间或显示信息未注册")
	}
	mockHTTP(t, func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.String() != authHost+"/auth/token" || request.Header.Get("X-Device-Id") != "0123456789abcdef0123456789abcdef" || request.Header.Get("Authorization") != "" {
			t.Fatalf("错误的刷新请求: %s %s", request.Method, request.URL)
		}
		var body map[string]string
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["refresh_token"] != "initial" || body["grant_type"] != "refresh_token" || body["client_id"] != clientID || body["client_secret"] != clientSecret {
			t.Fatalf("错误的登录参数: %+v", body)
		}
		return response(request, 200, `{"access_token":"access-new","refresh_token":"refresh-new","sub":"123","expires_in":3600}`), nil
	})
	token, err := registered.Auth(context.Background(), drive.AuthRequest{Config: map[string]string{"refresh_token": "initial", "device_id": "0123456789abcdef0123456789abcdef"}})
	if err != nil {
		t.Fatal(err)
	}
	var stored session
	if json.Unmarshal([]byte(token.RefreshToken), &stored) != nil || token.UserID != "thunder_123" || token.AccessToken != "access-new" || stored.RefreshToken != "refresh-new" || stored.ExpiresAt <= 0 || token.ProviderRootID != RootID {
		t.Fatalf("登录会话不正确: %+v", token)
	}
}

func TestThunderRejectsIncompleteLoginWithoutRequests(t *testing.T) {
	mockHTTP(t, func(request *http.Request) (*http.Response, error) {
		t.Fatal("无效登录配置不得发请求")
		return nil, nil
	})
	for _, config := range []map[string]string{
		{}, {"refresh_token": "token"}, {"refresh_token": "token", "device_id": "invalid"},
		{"refresh_token": "token", "device_id": "0123456789abcdef0123456789abcdef", "captcha_sign": "1.sign"},
		{"refresh_token": "token", "device_id": "0123456789abcdef0123456789abcdef", "captcha_sign": "1.sign", "timestamp": "invalid"},
	} {
		if _, err := authLogin(context.Background(), drive.AuthRequest{Config: config}); err == nil {
			t.Fatal("不完整配置被接受")
		}
	}
}

func TestThunderPaginationAndDownloadHeaders(t *testing.T) {
	account := accountFor("files")
	mockHTTP(t, func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("User-Agent") != apiUA || request.Header.Get("Authorization") != "Bearer access-files" {
			t.Fatal("业务请求缺少移动端身份")
		}
		switch request.URL.Path {
		case "/drive/v1/files":
			if request.URL.Query().Get("parent_id") != "" || request.URL.Query().Get("limit") != "100" || !strings.Contains(request.URL.Query().Get("filters"), "PHASE_TYPE_COMPLETE") {
				t.Fatalf("列表查询错误: %s", request.URL.RawQuery)
			}
			if request.URL.Query().Get("page_token") == "" {
				return response(request, 200, `{"files":[{"id":"folder","name":"目录","kind":"drive#folder","size":"0"}],"next_page_token":"next"}`), nil
			}
			return response(request, 200, `{"files":[{"id":"video","name":"测试.mp4","kind":"drive#file","size":4294967297}]}`), nil
		case "/drive/v1/files/video":
			return response(request, 200, `{"id":"video","name":"测试.mp4","size":"4294967297","web_content_link":"https://download.example/video?token=signature","medias":[{"is_origin":true,"link":{"url":"https://download.example/origin"}}]}`), nil
		default:
			return nil, fmt.Errorf("意外请求: %s", request.URL)
		}
	})
	driver := &Driver{}
	files, err := driver.List(context.Background(), account, RootID, nil)
	if err != nil || len(files) != 2 || !files[0].IsDir || files[1].Size != 4294967297 || files[1].ParentFileID != RootID {
		t.Fatalf("分页映射失败: %+v, %v", files, err)
	}
	link, err := driver.GetDownloadURL(context.Background(), account, "video", 0)
	if err != nil {
		t.Fatal(err)
	}
	if link.Size != 4294967297 || link.Headers["User-Agent"] != downloadUA || len(link.Headers) != 1 || link.DownloadMode != "redirect" {
		t.Fatalf("下载大小或鉴权隔离失败: %+v", link)
	}
	preview, err := driver.GetVideoPreview(context.Background(), account, "video")
	if err != nil || len(preview.Qualities) != 1 || !preview.Qualities[0].ForceProxy || preview.Qualities[0].URL != "https://download.example/origin" || len(preview.Qualities[0].Headers) != 1 {
		t.Fatalf("播放会话参数错误: %+v, %v", preview, err)
	}
}

func TestThunderRejectsRepeatedPagination(t *testing.T) {
	mockHTTP(t, func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/drive/v1/tasks" {
			return response(request, 200, `{"tasks":[],"next_page_token":"same"}`), nil
		}
		return response(request, 200, `{"files":[],"next_page_token":"same"}`), nil
	})
	if _, err := (&Driver{}).List(context.Background(), accountFor("loop"), RootID, nil); err == nil {
		t.Fatal("文件分页循环未终止")
	}
	if _, err := (&Driver{}).OfflineList(context.Background(), accountFor("loop")); err == nil {
		t.Fatal("离线分页循环未终止")
	}
}

func TestThunderCaptchaRetryIsBoundedAndPersisted(t *testing.T) {
	for _, repeat := range []bool{false, true} {
		t.Run(fmt.Sprintf("repeat_%t", repeat), func(t *testing.T) {
			account := accountFor("captcha")
			calls, exchanges := 0, 0
			mockHTTP(t, func(request *http.Request) (*http.Response, error) {
				if request.URL.Path == "/v1/shield/captcha/init" {
					exchanges++
					var body struct {
						Action   string            `json:"action"`
						Meta     map[string]string `json:"meta"`
						DeviceID string            `json:"device_id"`
					}
					if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					if body.Action != "GET:/drive/v1/files" || body.DeviceID != account.Token.DeviceID || body.Meta["user_id"] != "captcha" || body.Meta["captcha_sign"] != captchaSign(body.DeviceID, body.Meta["timestamp"]) {
						t.Fatalf("验证码签名参数错误: %+v", body)
					}
					return response(request, 200, `{"captcha_token":"captcha-new"}`), nil
				}
				calls++
				if calls == 1 || repeat {
					return response(request, 400, `{"error_code":9,"error":"captcha_invalid"}`), nil
				}
				if request.Header.Get("X-Captcha-Token") != "captcha-new" {
					t.Fatal("重试未携带新验证码 Token")
				}
				return response(request, 200, `{"files":[]}`), nil
			})
			_, err := (&Driver{}).ListPaged(context.Background(), account, RootID, "", nil)
			if repeat != (err != nil) || calls != 2 || exchanges != 1 {
				t.Fatalf("验证码重试失控: %d/%d, %v", calls, exchanges, err)
			}
			var stored session
			if json.Unmarshal([]byte(account.Token.RefreshToken), &stored) != nil || stored.CaptchaToken != "captcha-new" {
				t.Fatal("新验证码 Token 未保存到会话")
			}
		})
	}
}

func TestThunderErrorsDoNotInvalidateValidAccounts(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusTooManyRequests} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			mockHTTP(t, func(request *http.Request) (*http.Response, error) {
				calls++
				return response(request, status, `{"error":"permission_denied"}`), nil
			})
			_, err := (&Driver{}).ListPaged(context.Background(), accountFor("limited"), RootID, "", nil)
			if err == nil || errors.Is(err, drive.ErrUnauthorized) || calls != 1 {
				t.Fatalf("权限或限流被当作授权失效: %v, %d", err, calls)
			}
		})
	}
}

func TestThunderConcurrentRefreshPersistsBeforeUnlock(t *testing.T) {
	account := accountFor("rotate")
	persisted := drive.CloneToken(account.Token)
	var storageMu sync.Mutex
	updates, refreshes := 0, 0
	drive.SetTokenResolver(func(_, _ string) (*model.TokenInfo, error) {
		storageMu.Lock()
		defer storageMu.Unlock()
		return drive.CloneToken(persisted), nil
	})
	drive.SetTokenUpdater(func(_, _ string, token *model.TokenInfo) error {
		storageMu.Lock()
		defer storageMu.Unlock()
		updates++
		persisted = drive.CloneToken(token)
		return nil
	})
	t.Cleanup(func() { drive.SetTokenResolver(nil); drive.SetTokenUpdater(nil) })
	mockHTTP(t, func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/v1/auth/token" {
			refreshes++
			var body map[string]string
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["refresh_token"] != "refresh-rotate" {
				t.Error("轮换后仍使用了错误的刷新 Token")
			}
			return response(request, 200, `{"access_token":"access-new","refresh_token":"refresh-new","user_id":"rotate","expires_in":3600}`), nil
		}
		if request.Header.Get("Authorization") != "Bearer access-new" {
			return response(request, 401, `{"error_code":16,"error":"unauthorized"}`), nil
		}
		return response(request, 200, `{"files":[]}`), nil
	})
	var group sync.WaitGroup
	for attempt := 0; attempt < 2; attempt++ {
		copy := account
		copy.Token = drive.CloneToken(account.Token)
		group.Add(1)
		go func() {
			defer group.Done()
			if _, err := (&Driver{}).ListPaged(context.Background(), copy, RootID, "", nil); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	if updates != 1 || refreshes != 1 || !strings.Contains(persisted.RefreshToken, "refresh-new") || persisted.DeviceID != account.Token.DeviceID {
		t.Fatalf("并发刷新未串行落库: 更新 %d，刷新 %d", updates, refreshes)
	}
}

func TestThunderRefreshExpiryAndRejectedCredentials(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		t.Run(fmt.Sprint(invalid), func(t *testing.T) {
			account := accountFor("expired")
			var stored session
			if err := json.Unmarshal([]byte(account.Token.RefreshToken), &stored); err != nil {
				t.Fatal(err)
			}
			stored.ExpiresAt = 1
			encoded, _ := json.Marshal(stored)
			account.Token.RefreshToken = string(encoded)
			calls := 0
			mockHTTP(t, func(request *http.Request) (*http.Response, error) {
				calls++
				if request.URL.Path == "/drive/v1/about" && !invalid {
					return response(request, 200, `{"quota":{"limit":"100","usage":"20"}}`), nil
				}
				if request.URL.Path != "/v1/auth/token" {
					t.Fatal("账号刷新不应请求其他接口")
				}
				if invalid {
					return response(request, 400, `{"error":"invalid_grant"}`), nil
				}
				return response(request, 200, `{"access_token":"access-new","refresh_token":"refresh-new","user_id":"expired","expires_in":3600}`), nil
			})
			_, err := (&Driver{}).RefreshAccount(context.Background(), account, account.Token)
			wantCalls := 2
			if invalid {
				wantCalls = 1
			}
			if calls != wantCalls || invalid != errors.Is(err, drive.ErrUnauthorized) || !invalid && (err != nil || account.Token.TotalSize != 100) {
				t.Fatalf("过期刷新重复或失效分类错误: %d, %v", calls, err)
			}
		})
	}
}

func TestThunderRejectsMissingLinksAndAsyncFailures(t *testing.T) {
	mockHTTP(t, func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/drive/v1/files/video" {
			return response(request, 200, `{"id":"video","name":"video.mp4","size":1}`), nil
		}
		if request.URL.Path == "/drive/v1/files:batchCopy" {
			return response(request, 200, `{"task":{"id":"copy-task","phase":"PHASE_TYPE_RUNNING"}}`), nil
		}
		return response(request, 200, `{"id":"copy-task","phase":"PHASE_TYPE_ERROR"}`), nil
	})
	account := accountFor("failures")
	if _, err := (&Driver{}).GetDownloadURL(context.Background(), account, "video", 0); err == nil {
		t.Fatal("空下载链接被当作成功")
	}
	if ids, err := (&Driver{}).Copy(context.Background(), account, []drive.FileRef{{ID: "video"}}, "folder", ""); err == nil || len(ids) != 0 {
		t.Fatal("异步失败被当作复制成功")
	}
}

func TestThunderAccountIsolationAndCancellation(t *testing.T) {
	mockHTTP(t, func(request *http.Request) (*http.Response, error) {
		t.Fatal("账号不匹配或已取消的请求不应发送")
		return nil, nil
	})
	account := accountFor("first")
	account.UserID = "thunder_second"
	if _, err := (&Driver{}).ListPaged(context.Background(), account, RootID, "", nil); err == nil {
		t.Fatal("不同账号的凭据被混用")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (&Driver{}).ListPaged(ctx, accountFor("cancel"), RootID, "", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("取消未生效: %v", err)
	}
}

func TestThunderOfflineAndFileOperations(t *testing.T) {
	account := accountFor("operations")
	deleted := false
	mockHTTP(t, func(request *http.Request) (*http.Response, error) {
		var body map[string]any
		if request.Body != nil {
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
		}
		switch request.URL.Path {
		case "/drive/v1/files":
			if body["kind"] == "drive#folder" {
				if body["parent_id"] != "" {
					t.Fatal("目录根 ID 未映射")
				}
				return response(request, 200, `{"id":"folder-created"}`), nil
			}
			if body["upload_type"] != "UPLOAD_TYPE_URL" || body["url"].(map[string]any)["url"] != "magnet:?xt=urn:btih:test" {
				t.Fatal("磁力离线参数错误")
			}
			return response(request, 200, `{"task":{"id":"offline-task","file_id":"offline-file"}}`), nil
		case "/drive/v1/tasks":
			if request.Method == http.MethodDelete {
				deleted = request.URL.Query().Get("task_ids") == "offline-task" && request.URL.Query().Get("delete_files") == "false"
				return response(request, http.StatusNoContent, ""), nil
			}
			return response(request, 200, `{"tasks":[{"id":"offline-task","file_id":"offline-file","file_name":"测试","file_size":"4294967297","progress":100,"phase":"PHASE_TYPE_COMPLETE"}]}`), nil
		case "/drive/v1/files:batchMove", "/drive/v1/files:batchCopy":
			if body["to"].(map[string]any)["parent_id"] != "folder-created" {
				t.Fatal("移动复制目标错误")
			}
			return response(request, 200, `{}`), nil
		case "/drive/v1/files/offline-file":
			if request.Method != http.MethodPatch || body["name"] != "重命名" {
				t.Fatal("重命名参数错误")
			}
			return response(request, http.StatusNoContent, ""), nil
		case "/drive/v1/files/offline-file/trash":
			return response(request, 200, `{}`), nil
		default:
			return nil, fmt.Errorf("意外操作: %s %s", request.Method, request.URL)
		}
	})
	driver := &Driver{}
	created, err := driver.Mkdir(context.Background(), account, RootID, "目录")
	if err != nil || created.FileID != "folder-created" {
		t.Fatalf("建目录失败: %+v, %v", created, err)
	}
	taskID, fileID, err := driver.OfflineCreate(context.Background(), account, "magnet:?xt=urn:btih:test", "测试", RootID)
	if err != nil || taskID != "offline-task" || fileID != "offline-file" {
		t.Fatalf("磁力提交失败: %s, %s, %v", taskID, fileID, err)
	}
	tasks, err := driver.OfflineList(context.Background(), account)
	if err != nil || len(tasks) != 1 || tasks[0].FileSize != 4294967297 || tasks[0].Progress != 100 {
		t.Fatalf("离线查询失败: %+v, %v", tasks, err)
	}
	if err := driver.OfflineDelete(context.Background(), account, []string{taskID}, false); err != nil || !deleted {
		t.Fatalf("离线删除失败: %v", err)
	}
	refs := []drive.FileRef{{ID: fileID}}
	if _, err := driver.Move(context.Background(), account, refs, created.FileID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := driver.Copy(context.Background(), account, refs, created.FileID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := driver.Rename(context.Background(), account, fileID, "重命名"); err != nil {
		t.Fatal(err)
	}
	if ids, err := driver.Trash(context.Background(), account, []string{fileID}); err != nil || len(ids) != 1 {
		t.Fatalf("移入回收站失败: %v, %v", ids, err)
	}
}
