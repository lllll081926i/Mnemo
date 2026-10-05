package thunder

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

func TestThunderRefreshQuotaPreservesSessionAndPersists(t *testing.T) {
	for _, body := range []string{`{"quota":{"limit":"1099511627776","usage":"123456789"}}`, `{"quota":{"limit":1099511627776,"usage":123456789}}`} {
		t.Run(body, func(t *testing.T) {
			account := accountFor("quota")
			original := account.Token.RefreshToken
			updates := 0
			drive.SetTokenUpdater(func(user, driveID string, token *model.TokenInfo) error {
				updates++
				if user != account.UserID || token.TotalSize != 1099511627776 || token.UsedSize != 123456789 {
					t.Fatalf("容量未正确持久化: %s, %+v", user, token)
				}
				return nil
			})
			t.Cleanup(func() { drive.SetTokenUpdater(nil) })
			mockHTTP(t, func(request *http.Request) (*http.Response, error) {
				if request.Method != http.MethodGet || request.URL.Path != "/drive/v1/about" || request.Header.Get("Authorization") != "Bearer access-quota" {
					t.Fatalf("容量查询不应重复换取凭据: %s %s", request.Method, request.URL)
				}
				return response(request, 200, body), nil
			})
			token, err := (&Driver{}).RefreshAccount(t.Context(), account, account.Token)
			if err != nil || token.FreeSize != 1099511627776-123456789 || token.RefreshToken != original || updates != 1 {
				t.Fatalf("容量刷新失败: %+v, %v, updates=%d", token, err, updates)
			}
		})
	}
}

func TestThunderInvalidQuotaDoesNotErasePreviousCapacity(t *testing.T) {
	for _, body := range []string{`{}`, `{"quota":{}}`, `{"quota":{"limit":"100"}}`, `{"quota":{"limit":100,"usage":""}}`, `{"quota":{"limit":100,"usage":null}}`, `{"quota":{"limit":"bad","usage":2}}`, `{"quota":{"limit":100,"usage":-2}}`, `{"quota":{"limit":0,"usage":2}}`, `{"error_code":8,"error":"permission_denied"}`} {
		t.Run(body, func(t *testing.T) {
			account := accountFor("quota-failure")
			account.Token.TotalSize, account.Token.UsedSize, account.Token.FreeSize = 80, 20, 60
			mockHTTP(t, func(request *http.Request) (*http.Response, error) { return response(request, 200, body), nil })
			_, err := (&Driver{}).RefreshAccount(t.Context(), account, account.Token)
			if err == nil || errors.Is(err, drive.ErrUnauthorized) || account.Token.TotalSize != 80 || account.Token.UsedSize != 20 || account.Token.FreeSize != 60 {
				t.Fatalf("无效容量覆盖旧值或被误判为失效: %+v, %v", account.Token, err)
			}
		})
	}
}

func TestThunderQuotaLimitsAndRotation(t *testing.T) {
	account := accountFor("quota-rotate")
	calls, refreshes, updates := 0, 0, 0
	drive.SetTokenUpdater(func(_, _ string, token *model.TokenInfo) error {
		updates++
		if token.AccessToken != "rotated" || !strings.Contains(token.RefreshToken, "rotated-refresh") {
			t.Fatal("容量失败丢失轮换凭据")
		}
		return nil
	})
	t.Cleanup(func() { drive.SetTokenUpdater(nil) })
	mockHTTP(t, func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/v1/auth/token" {
			refreshes++
			return response(request, 200, `{"access_token":"rotated","refresh_token":"rotated-refresh","user_id":"quota-rotate","expires_in":3600}`), nil
		}
		calls++
		if calls == 1 {
			return response(request, 401, `{"error_code":16,"error":"unauthorized"}`), nil
		}
		return response(request, 429, `{"error":"too_many_requests"}`), nil
	})
	_, err := (&Driver{}).RefreshAccount(t.Context(), account, account.Token)
	if err == nil || errors.Is(err, drive.ErrUnauthorized) || calls != 2 || refreshes != 1 || updates != 1 {
		t.Fatalf("限流应保留账号与新凭据: %v, calls=%d refreshes=%d updates=%d", err, calls, refreshes, updates)
	}
}

func TestThunderTrashListsAllPagesAndDeletedTime(t *testing.T) {
	calls := 0
	mockHTTP(t, func(request *http.Request) (*http.Response, error) {
		calls++
		query := request.URL.Query()
		if request.Method != http.MethodGet || request.URL.Path != "/drive/v1/files" || query.Get("filters") != `{"trashed":{"eq":true}}` || query.Has("parent_id") {
			t.Fatalf("回收站不能附带目录或完成阶段过滤: %s", request.URL)
		}
		if query.Get("page_token") == "" {
			return response(request, 200, `{"files":[{"id":"nested-file","parent_id":"nested","name":"文件","size":"42","trashed":true,"delete_time":"2026-10-05T01:00:00Z"}],"next_page_token":"page-2"}`), nil
		}
		return response(request, 200, `{"files":[{"id":"folder","name":"目录","kind":"drive#folder","trashed":true}]}`), nil
	})
	files, err := (&Driver{}).ListTrash(t.Context(), accountFor("trash"), nil)
	if err != nil || calls != 2 || len(files) != 2 || files[0].Size != 42 || files[0].ParentFileID != "nested" || files[0].Time != 1791162000 || !files[1].IsDir {
		t.Fatalf("回收站列表不完整: %+v, %v", files, err)
	}
}

func TestThunderTrashRejectsUnsafePages(t *testing.T) {
	for _, body := range []string{`{"files":[{"id":"live","name":"未删除","trashed":false}]}`, `{"files":[{"id":"same","name":"重复","trashed":true}],"next_page_token":"repeated"}`} {
		t.Run(body, func(t *testing.T) {
			mockHTTP(t, func(request *http.Request) (*http.Response, error) { return response(request, 200, body), nil })
			if files, err := (&Driver{}).ListTrash(t.Context(), accountFor("trash-invalid"), nil); err == nil || len(files) != 0 {
				t.Fatalf("异常回收站响应被当作完整列表: %+v, %v", files, err)
			}
		})
	}
}

func TestThunderRestoreAndPurgeReceipts(t *testing.T) {
	for _, action := range []string{"restore", "purge"} {
		t.Run(action, func(t *testing.T) {
			mutations, polls := 0, 0
			mockHTTP(t, func(request *http.Request) (*http.Response, error) {
				if request.URL.Path == "/drive/v1/tasks/first-task" {
					polls++
					return response(request, 200, `{"id":"first-task","phase":"PHASE_TYPE_COMPLETE"}`), nil
				}
				if request.Method == http.MethodGet {
					id := strings.TrimPrefix(request.URL.Path, "/drive/v1/files/")
					return response(request, 200, fmt.Sprintf(`{"id":%q,"name":"已删除","trashed":true}`, id)), nil
				}
				mutations++
				if action == "restore" && (request.Method != http.MethodPatch || !strings.HasSuffix(request.URL.Path, "/untrash")) || action == "purge" && request.Method != http.MethodDelete {
					t.Fatalf("操作端点不正确: %s %s", request.Method, request.URL)
				}
				if mutations == 1 {
					return response(request, 200, `{"task":{"id":"first-task","phase":"PHASE_TYPE_RUNNING"}}`), nil
				}
				return response(request, 403, `{"error":"permission_denied"}`), nil
			})
			var done []string
			var err error
			if action == "restore" {
				done, err = (&Driver{}).Restore(t.Context(), accountFor(action), []string{"first", "second"})
			} else {
				done, err = (&Driver{}).Delete(t.Context(), accountFor(action), []drive.FileRef{{ID: "first"}, {ID: "second"}})
			}
			if err == nil || !reflect.DeepEqual(done, []string{"first"}) || polls != 1 || mutations != 2 {
				t.Fatalf("操作部分失败未保留实际回执: %v, %v, polls=%d mutations=%d", done, err, polls, mutations)
			}
		})
	}
}

func TestThunderPurgeRejectsLiveFileBeforeAnyDeletion(t *testing.T) {
	deletions := 0
	mockHTTP(t, func(request *http.Request) (*http.Response, error) {
		if request.Method == http.MethodDelete {
			deletions++
			return response(request, 204, ""), nil
		}
		id := strings.TrimPrefix(request.URL.Path, "/drive/v1/files/")
		return response(request, 200, fmt.Sprintf(`{"id":%q,"name":"文件","trashed":%v}`, id, id == "trashed")), nil
	})
	done, err := (&Driver{}).Delete(t.Context(), accountFor("purge-preflight"), []drive.FileRef{{ID: "trashed"}, {ID: "live"}})
	if err == nil || len(done) != 0 || deletions != 0 {
		t.Fatalf("永久删除预检未保护正常文件: %v, %v, deletions=%d", done, err, deletions)
	}
}

func TestThunderCreateAndCancelShare(t *testing.T) {
	account := accountFor("sharing")
	created, canceled := false, false
	mockHTTP(t, func(request *http.Request) (*http.Response, error) {
		var body map[string]json.RawMessage
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if request.Method != http.MethodPost {
			t.Fatal("分享操作应使用 POST")
		}
		if request.URL.Path == "/drive/v1/share" {
			created = true
			if string(body["file_ids"]) != `["file","folder"]` || string(body["expiration_days"]) != `"7"` || string(body["share_to"]) != `"copy"` || string(body["restore_limit"]) != `"-1"` {
				t.Fatalf("分享参数错误: %s", body)
			}
			return response(request, 200, `{"share_id":"sharing-id","share_url":"https://pan.xunlei.com/s/link?pwd=a1b2","pass_code":"a1b2","share_status":"OK"}`), nil
		}
		if request.URL.Path != "/drive/v1/share/delete" || string(body["share_id"]) != `"sharing-id"` || string(body["space"]) != `""` {
			t.Fatalf("撤销必须使用云端分享 ID: %s %s", request.URL, body)
		}
		canceled = true
		return response(request, 204, ""), nil
	})
	params := drive.ShareParams{FileIDs: []string{"file", "folder"}, ShareName: "资源", Expiration: time.Now().Add(7 * 24 * time.Hour).Format(time.RFC3339)}
	item, err := (&Driver{}).CreateShare(t.Context(), account, params)
	if err != nil || item.SharePwd != "a1b2" || item.ShareID != "sharing-id" || item.ShareName != "资源" || !created {
		t.Fatalf("分享创建失败: %+v, %v", item, err)
	}
	if err := (&Driver{}).CancelShare(t.Context(), account, model.ShareHistoryEntry{ShareID: item.ShareID, Provider: model.ProviderThunder}); err != nil || !canceled {
		t.Fatalf("分享未远端撤销: %v", err)
	}
}

func TestThunderInvalidShareParametersDoNotSendRequests(t *testing.T) {
	mockHTTP(t, func(request *http.Request) (*http.Response, error) {
		t.Fatal("无效分享不应发送请求")
		return nil, nil
	})
	for _, params := range []drive.ShareParams{{}, {FileIDs: []string{"same", "same"}}, {FileIDs: []string{RootID}}, {FileIDs: []string{"file"}, Password: "abcd"}, {FileIDs: []string{"file"}, Expiration: "30"}, {FileIDs: []string{"file"}, Expiration: "bad"}, {FileIDs: []string{"file"}, Expiration: "2020-01-01"}} {
		if _, err := (&Driver{}).CreateShare(t.Context(), accountFor("invalid-share"), params); err == nil {
			t.Fatalf("无效分享被接受: %+v", params)
		}
	}
}

func TestThunderMalformedShareReceiptsAreNotSuccess(t *testing.T) {
	for _, body := range []string{`{}`, `{"share_id":"id"}`, `{"share_id":"id","share_url":"https://pan.xunlei.com.attacker.test/s/id"}`, `{"share_id":"id","share_url":"https://pan.xunlei.com/s/id","share_status":"AUDITING"}`, `{"error_code":8,"error":"permission_denied"}`} {
		t.Run(body, func(t *testing.T) {
			mockHTTP(t, func(request *http.Request) (*http.Response, error) { return response(request, 200, body), nil })
			if item, err := (&Driver{}).CreateShare(t.Context(), accountFor("bad-share"), drive.ShareParams{FileIDs: []string{"file"}}); err == nil || item != nil {
				t.Fatalf("无效分享被当作成功: %+v, %v", item, err)
			}
		})
	}
}
