package app

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
	"mnemo-go/internal/store"
)

func TestGuangyaOfflineApplicationFlow(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const userID = "guangya_offline-app"
	const driveID = "offline-app"
	drive.SetTokenResolver(func(user, space string) (*model.TokenInfo, error) {
		if user != userID || space != driveID {
			t.Fatal("光鸭云离线任务调用了错误账号")
		}
		return &model.TokenInfo{AccessToken: "guangya-access", RefreshToken: `{"access_token":"guangya-access","refresh_token":"guangya-refresh","device_id":"fixed-device"}`}, nil
	})
	previous := netx.TestTransportHook
	t.Cleanup(func() { drive.SetTokenResolver(nil); netx.TestTransportHook = previous })
	deleted := false
	denyDelete := false
	netx.TestTransportHook = favoriteTestTransport(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != "api.guangyapan.com" || request.Method != http.MethodPost || request.Header.Get("Authorization") != "Bearer guangya-access" {
			t.Fatal("光鸭云离线请求使用了错误的服务或凭据")
		}
		body := ""
		switch request.URL.Path {
		case "/nd.bizcloudcollection.s/v1/resolve_res":
			body = `{"code":0,"data":{"url":"magnet:?xt=urn:btih:test"}}`
		case "/nd.bizcloudcollection.s/v1/create_task":
			body = `{"code":0,"data":{"taskId":"guangya-task","fileId":"guangya-file"}}`
		case "/nd.bizcloudcollection.s/v1/list_task":
			body = `{"code":0,"data":{"list":[{"taskId":"guangya-task","fileId":"guangya-file","status":5,"progress":"70","fileName":"测试资源","totalSize":"1024"}]}}`
		case "/nd.bizcloudcollection.s/v2/delete_task":
			var params struct {
				TaskIDs    []string `json:"taskIds"`
				DeleteFile bool     `json:"deleteFile"`
			}
			if err := json.NewDecoder(request.Body).Decode(&params); err != nil {
				t.Fatal(err)
			}
			deleted = len(params.TaskIDs) == 1 && params.TaskIDs[0] == "guangya-task" && !params.DeleteFile && !denyDelete
			body = `{"code":0,"data":{}}`
			if denyDelete {
				body = `{"code":403,"msg":"无权限"}`
			}
		default:
			t.Fatalf("未核实的光鸭接口：%s", request.URL)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})
	app := &App{store: st}
	task, err := app.OfflineDownload(userID, driveID, "magnet:?xt=urn:btih:test", "测试资源")
	if err != nil || task.ID != "guangya-task" || len(app.ListOfflineTasks(userID)) != 1 {
		t.Fatalf("应用未能创建并保存光鸭云离线任务：%+v %v", task, err)
	}
	tasks, err := app.RefreshOfflineTasks(userID, driveID)
	if err != nil || len(tasks) != 1 || tasks[0].Status != "partial" || tasks[0].Progress != 70 || tasks[0].FileSize != 1024 {
		t.Fatalf("应用未能刷新光鸭任务：%+v %v", tasks, err)
	}
	denyDelete = true
	if err := app.DeleteOfflineTask(userID, driveID, task.ID, false); err == nil || len(app.ListOfflineTasks(userID)) != 1 {
		t.Fatal("远端删除失败不应清除本地记录")
	}
	denyDelete = false
	if err := app.DeleteOfflineTask(userID, driveID, task.ID, false); err != nil || !deleted || len(app.ListOfflineTasks(userID)) != 0 {
		t.Fatalf("应用未能删除光鸭任务：%v", err)
	}
}
