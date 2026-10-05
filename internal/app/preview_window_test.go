package app

import (
	"encoding/json"
	"mnemo-go/internal/model"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDownloadIndicatorAggregatesBytesAndClearsFinishedTasks(t *testing.T) {
	for _, tc := range []struct {
		name    string
		tasks   []model.DownloadTask
		state   uintptr
		percent uint64
	}{
		{"weighted", []model.DownloadTask{{Status: "downloading", Size: 100, Downloaded: 50}, {Status: "queued", Size: 300}}, 2, 12},
		{"unknown", []model.DownloadTask{{Status: "downloading"}}, 1, 0},
		{"paused", []model.DownloadTask{{Status: "paused", Size: 100, Downloaded: 25}}, 0, 0},
		{"failed", []model.DownloadTask{{Status: "failed"}}, 0, 0},
		{"finished", []model.DownloadTask{{Status: "completed", Size: 100, Downloaded: 100}, {Status: "canceled"}}, 0, 0},
		{"finished_with_history", []model.DownloadTask{{Status: "completed", Size: 100, Downloaded: 100}, {Status: "failed"}, {Status: "paused", Size: 300, Downloaded: 150}}, 0, 0},
		{"active_with_history", []model.DownloadTask{{Status: "downloading", Size: 100, Downloaded: 50}, {Status: "paused", Size: 300, Downloaded: 300}, {Status: "failed"}, {Status: "completed", Size: 1000, Downloaded: 1000}}, 2, 50},
		{"active_with_unknown_paused", []model.DownloadTask{{Status: "downloading", Size: 100, Downloaded: 50}, {Status: "paused"}}, 2, 50},
		{"clamped", []model.DownloadTask{{Status: "downloading", Size: 100, Downloaded: 110}}, 2, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := summarizeDownloads(tc.tasks)
			if got.state != tc.state || got.percent != tc.percent {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestDownloadIndicatorReturnsToIdleAfterLastDownload(t *testing.T) {
	tasks := []model.DownloadTask{
		{Status: "downloading", Size: 100, Downloaded: 99},
		{Status: "completed", Size: 200, Downloaded: 200},
		{Status: "failed"},
		{Status: "paused", Size: 300, Downloaded: 150},
	}
	if got := summarizeDownloads(tasks); got.state != 2 || got.percent != 99 {
		t.Fatalf("下载中进度不正确：%+v", got)
	}
	tasks[0].Status, tasks[0].Downloaded = "completed", 100
	if got := summarizeDownloads(tasks); got.state != 0 || got.percent != 0 {
		t.Fatalf("最后一个下载完成后未清除任务栏进度：%+v", got)
	}
	if got := summarizeDownloads(tasks[:2]); got.label != "Mnemo · 无下载任务" {
		t.Fatalf("已完成记录未恢复空闲提示：%+v", got)
	}
}

func TestPreviewWindowRejectsUnrelatedOperationsAndAccounts(t *testing.T) {
	a := &App{}
	var seed PreviewWindowSeed
	seed.Account.UserID, seed.Account.DriveID = "user", "drive"
	for _, call := range []previewCall{
		{Method: "ListAccounts"},
		{Method: "RemoveAccount", Args: []json.RawMessage{json.RawMessage(`"user"`)}},
		{Method: "PreviewURL", Args: []json.RawMessage{json.RawMessage(`"another"`), json.RawMessage(`"drive"`), json.RawMessage(`"file"`)}},
		{Method: "CachedPreviewImageURL", Args: []json.RawMessage{json.RawMessage(`"another"`), json.RawMessage(`"drive"`), json.RawMessage(`{"file_id":"file"}`)}},
		{Method: "PreviewURL", Args: []json.RawMessage{json.RawMessage(`"user"`), json.RawMessage(`"drive"`)}},
	} {
		if a.previewInvoke(seed, call).Error == "" {
			t.Fatalf("accepted invalid call %s", call.Method)
		}
	}
	allowed := previewCall{Method: "CachedPreviewImageURL", Args: []json.RawMessage{json.RawMessage(`"user"`), json.RawMessage(`"drive"`), json.RawMessage(`{"file_id":"file"}`)}}
	if got := a.previewInvoke(seed, allowed).Error; got != "图片缓存不可用" {
		t.Fatalf("图片缓存方法未转发到主窗口: %q", got)
	}
}

func TestShowPreviewWindowsCoalescesRestoreRequests(t *testing.T) {
	a := &App{}
	process := &previewProcess{commands: make(chan string, 1)}
	a.previewWindows.Store(1, process)
	a.ShowPreviewWindows()
	a.ShowPreviewWindows()
	if len(process.commands) != 1 || <-process.commands != "show" {
		t.Fatal("restore request not delivered")
	}
}

func TestPreviewHostForwardsResultAndError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer session" {
			t.Error("missing session authentication")
		}
		var call previewCall
		_ = json.NewDecoder(r.Body).Decode(&call)
		if call.Method == "GetSettings" {
			_, _ = w.Write([]byte(`{"result":{"playbackResume":true}}`))
			return
		}
		_, _ = w.Write([]byte(`{"error":"expired"}`))
	}))
	defer server.Close()
	data, _ := json.Marshal(previewChildConfig{Endpoint: server.URL, Token: "session"})
	host, err := ReadPreviewHost(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	result, err := host.Invoke("GetSettings", nil)
	if err != nil || string(result) != `{"playbackResume":true}` {
		t.Fatalf("result=%s err=%v", result, err)
	}
	if _, err := host.Invoke("PreviewURL", nil); err == nil || err.Error() != "expired" {
		t.Fatalf("unexpected error %v", err)
	}
}
