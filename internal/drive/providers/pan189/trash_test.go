package pan189

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

func TestTrashListAndRestoreBothSpaces(t *testing.T) {
	previous := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = previous })
	session := &Session{
		SessionKey: "personal-key", SessionSecret: "personal-secret",
		FamilySessionKey: "family-key", FamilySessionSecret: "family-secret", FamilyID: "42",
	}
	account := drive.Context{UserID: "pan189:trash", DriveID: "pan189:trash", Token: &model.TokenInfo{
		AccessToken: session.SessionKey, RefreshToken: mustJSON(session),
	}}
	var restoredSpaces []string
	netx.TestTransportHook = pan189AuthRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/listRecycleBinFiles.action":
			family := request.URL.Query().Get("family") == "true"
			if family {
				if request.Header.Get("SessionKey") != "family-key" || request.URL.Query().Get("familyId") != "42" {
					t.Fatalf("家庭回收站未使用家庭签名或家庭 ID")
				}
				return pan189AuthResponse(request, 200, nil, `{"count":1,"folderList":[{"id":202,"name":"已删目录","lastOpTime":"2026-09-24 10:00:00"}],"fileList":[]}`), nil
			}
			if request.Header.Get("SessionKey") != "personal-key" {
				t.Fatalf("个人回收站未使用个人签名")
			}
			return pan189AuthResponse(request, 200, nil, `{"count":1,"fileList":[{"id":101,"name":"已删文件.txt","size":12,"md5":"abc"}],"folderList":[]}`), nil
		case "/batch/createBatchTask.action":
			body, err := io.ReadAll(request.Body)
			if err != nil {
				return nil, err
			}
			form, err := url.ParseQuery(string(body))
			if err != nil || form.Get("type") != "RESTORE" {
				t.Fatalf("恢复任务参数无效：%v，%v", form, err)
			}
			var entries []fileRefItem
			if err := json.Unmarshal([]byte(form.Get("taskInfos")), &entries); err != nil || len(entries) != 1 {
				t.Fatalf("恢复条目无效：%v，%v", entries, err)
			}
			if request.Header.Get("SessionKey") == "family-key" {
				if form.Get("familyId") != "42" || entries[0].FileID != "202" || entries[0].IsFolder != 1 {
					t.Fatalf("家庭恢复参数无效：%v，%v", form, entries)
				}
				restoredSpaces = append(restoredSpaces, spaceFamily)
			} else {
				if entries[0].FileID != "101" || entries[0].FileName != "已删文件.txt" {
					t.Fatalf("个人恢复参数无效：%v", entries)
				}
				restoredSpaces = append(restoredSpaces, spacePersonal)
			}
			return pan189AuthResponse(request, 200, nil, `{"taskId":"task-1"}`), nil
		case "/batch/checkBatchTask.action":
			return pan189AuthResponse(request, 200, nil, `{"taskStatus":4}`), nil
		default:
			return nil, fmt.Errorf("意外请求：%s", request.URL.Path)
		}
	})
	driver := &Driver{}
	items, err := driver.ListTrash(t.Context(), account, nil)
	if err != nil || len(items) != 2 || items[0].FileID != pan189FileID(spacePersonal, "101") || items[1].FileID != pan189FileID(spaceFamily, "202") || !items[1].IsDir {
		t.Fatalf("回收站列表=%+v，错误=%v", items, err)
	}
	ids, err := driver.Restore(t.Context(), account, []string{items[0].FileID, items[1].FileID})
	if err != nil || len(ids) != 2 || strings.Join(restoredSpaces, ",") != "personal,family" {
		t.Fatalf("恢复结果=%v，空间=%v，错误=%v", ids, restoredSpaces, err)
	}
}
