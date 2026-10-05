package app

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"mnemo-go/internal/model"
)

func TestQuitConfirmationAllowsNextClose(t *testing.T) {
	application := NewApp()
	if !application.beginQuitConfirmation() {
		t.Fatal("首次退出未进入确认状态")
	}
	if !application.BeforeClose(context.Background()) {
		t.Fatal("等待确认时没有阻止重复关闭")
	}
	application.RespondQuit(true)
	if application.BeforeClose(context.Background()) {
		t.Fatal("确认后退出仍被拦截")
	}
	if !application.forceQuit.Load() || application.beginQuitConfirmation() {
		t.Fatal("确认后退出标记不正确或重复弹窗")
	}
	application.RespondQuit(false)
	if application.BeforeClose(context.Background()) {
		t.Fatal("重复回复改变了已确认状态")
	}
}

func TestQuitCancellationResetsTrayRequest(t *testing.T) {
	application := NewApp()
	application.forceQuit.Store(true)
	application.beginQuitConfirmation()
	application.RespondQuit(false)
	if application.forceQuit.Load() || application.quitPending || application.quitConfirmed {
		t.Fatal("取消后未清除退出状态")
	}
	if !application.beginQuitConfirmation() {
		t.Fatal("取消后无法重新请求退出")
	}
}

func TestQuitConfirmationRequestsRuntimeOnce(t *testing.T) {
	application := NewApp()
	application.ctx = context.Background()
	application.beginQuitConfirmation()
	requests := 0
	requestQuit := func(ctx context.Context) {
		requests++
		if application.BeforeClose(ctx) {
			t.Fatal("实际退出请求仍被关闭钩子阻止")
		}
	}
	application.respondQuit(true, requestQuit)
	application.respondQuit(true, requestQuit)
	application.respondQuit(false, requestQuit)
	if requests != 1 {
		t.Fatalf("退出调用数量：%d", requests)
	}
}

func TestQuitCancellationDoesNotRequestRuntime(t *testing.T) {
	application := NewApp()
	application.ctx = context.Background()
	application.forceQuit.Store(true)
	application.beginQuitConfirmation()
	application.respondQuit(false, func(context.Context) { t.Fatal("取消退出却调用了退出接口") })
	if application.forceQuit.Load() || application.quitPending || application.quitConfirmed {
		t.Fatal("取消未清除退出状态")
	}
}

func TestQuitIgnoresUnsolicitedConfirmation(t *testing.T) {
	application := NewApp()
	application.RespondQuit(true)
	if application.forceQuit.Load() || application.quitConfirmed {
		t.Fatal("未请求确认时接受了退出回复")
	}
}

func TestQuitConfirmationDeduplicatesConcurrentRequests(t *testing.T) {
	application := NewApp()
	var requests atomic.Int32
	var group sync.WaitGroup
	for range 32 {
		group.Go(func() {
			if application.beginQuitConfirmation() {
				requests.Add(1)
			}
		})
	}
	group.Wait()
	if requests.Load() != 1 {
		t.Fatalf("重复确认请求数量：%d", requests.Load())
	}
}

func TestQuitWithoutDownloadsNeedsNoConfirmation(t *testing.T) {
	application := NewApp()
	application.forceQuit.Store(true)
	if application.BeforeClose(context.Background()) || application.quitPending {
		t.Fatal("无下载任务时退出被拦截")
	}
}

func TestUpdateQuitBypassesPendingConfirmation(t *testing.T) {
	application := NewApp()
	application.updateApplying = true
	application.beginQuitConfirmation()
	if application.BeforeClose(context.Background()) {
		t.Fatal("安装更新时退出被确认框拦截")
	}
}

func TestActiveDownloadCount(t *testing.T) {
	tasks := []model.DownloadTask{
		{Status: "queued"}, {Status: "downloading"}, {Status: "paused"},
		{Status: "completed"}, {Status: "failed"}, {Status: "canceled"},
	}
	if active := activeDownloadCount(tasks); active != 2 {
		t.Fatalf("进行中的下载数量：%d", active)
	}
}
