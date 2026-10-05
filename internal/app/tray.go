package app

import (
	"context"
	"fmt"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"mnemo-go/internal/logging"
	"mnemo-go/internal/model"
)

type downloadIndicator struct {
	state   uintptr
	percent uint64
	label   string
}

func summarizeDownloads(tasks []model.DownloadTask) downloadIndicator {
	active, paused, failed := 0, 0, 0
	var done, total float64
	unknown := false
	for _, task := range tasks {
		switch task.Status {
		case "downloading", "queued":
			active++
		case "paused":
			paused++
			continue
		case "failed":
			failed++
			continue
		default:
			continue
		}
		if task.Size <= 0 {
			unknown = true
			continue
		}
		total += float64(task.Size)
		done += float64(max(0, min(task.Downloaded, task.Size)))
	}
	progress := downloadIndicator{label: "Mnemo · 无下载任务"}
	if total > 0 {
		progress.percent = uint64(done * 100 / total)
	}
	if active > 0 {
		progress.state = 2
		progress.label = fmt.Sprintf("Mnemo · 下载 %d%% · %d 个任务", progress.percent, active)
		if unknown {
			progress.state = 1
			progress.label = fmt.Sprintf("Mnemo · 正在下载 · %d 个任务", active)
		}
	} else if paused > 0 {
		progress.label = fmt.Sprintf("Mnemo · 已暂停 %d 个任务", paused)
	} else if failed > 0 {
		progress.label = fmt.Sprintf("Mnemo · %d 个下载失败", failed)
	}
	return progress
}

// ShowMainWindow 从托盘/隐藏状态恢复主窗口并置于前台。
func (a *App) ShowMainWindow() {
	ctx, ok := a.wailsContext()
	if !ok {
		// 唤起信号可能先于 OnStartup 到达（二次启动竞态），稍候重试
		for i := 0; i < 20 && !ok; i++ {
			time.Sleep(300 * time.Millisecond)
			ctx, ok = a.wailsContext()
		}
		if !ok {
			return
		}
	}
	logging.Debug("main window show requested")
	runtime.WindowUnminimise(ctx)
	runtime.WindowShow(ctx)
	// Windows 上前台焦点需要 always-on-top 抖动
	runtime.WindowSetAlwaysOnTop(ctx, true)
	runtime.WindowSetAlwaysOnTop(ctx, false)
}

// ForceQuit 绕过「关闭不退出」，真正退出应用（托盘菜单「退出」使用）。
func (a *App) ForceQuit() {
	a.forceQuit.Store(true)
	logging.Info("force quit requested", "source", "tray")
	if ctx, ok := a.wailsContext(); ok {
		runtime.Quit(ctx)
	}
}

// BeforeClose 实现 Wails OnBeforeClose：启用「关闭不退出」（默认）时隐藏
// 窗口到托盘而非退出；真正退出前若仍有下载任务，弹窗请用户确认。
// 返回 true 表示阻止关闭。
func (a *App) BeforeClose(ctx context.Context) bool {
	a.updateMu.Lock()
	applyingUpdate := a.updateApplying
	a.updateMu.Unlock()
	if applyingUpdate {
		return false
	}
	a.quitMu.Lock()
	confirmed, pending := a.quitConfirmed, a.quitPending
	a.quitMu.Unlock()
	if confirmed {
		return false
	}
	if pending {
		return true
	}
	realQuit := a.forceQuit.Load() || !TrayAvailable()
	if TrayAvailable() && !realQuit {
		if st, err := a.storeOrError(); err == nil {
			if s, serr := st.GetSettings(); serr == nil && !s.CloseToTrayEnabled() {
				realQuit = true
			}
		}
	}
	if !realQuit {
		logging.Info("window close intercepted, hiding to tray")
		runtime.WindowHide(ctx)
		return true
	}
	if a.confirmQuitWithActiveDownloads(ctx) {
		return true
	}
	return false
}

// confirmQuitWithActiveDownloads 有进行中的下载任务时请求前端确认；
// 返回 true 表示等待用户回复，暂时阻止退出。
func (a *App) confirmQuitWithActiveDownloads(ctx context.Context) bool {
	dl := a.downloadManager()
	if dl == nil {
		return false
	}
	active := activeDownloadCount(dl.List())
	if active == 0 {
		return false
	}
	if !a.beginQuitConfirmation() {
		return true
	}
	logging.Info("quit blocked for confirmation", "active_downloads", active)
	runtime.WindowUnminimise(ctx)
	runtime.WindowShow(ctx)
	runtime.EventsEmit(ctx, "app:quit-confirm", active)
	return true
}

func activeDownloadCount(tasks []model.DownloadTask) int {
	active := 0
	for _, task := range tasks {
		if task.Status == "queued" || task.Status == "downloading" {
			active++
		}
	}
	return active
}

func (a *App) beginQuitConfirmation() bool {
	a.quitMu.Lock()
	defer a.quitMu.Unlock()
	if a.quitPending || a.quitConfirmed {
		return false
	}
	a.quitPending = true
	return true
}

func (a *App) resolveQuitConfirmation(confirmed bool) bool {
	a.quitMu.Lock()
	defer a.quitMu.Unlock()
	if !a.quitPending {
		return false
	}
	a.quitPending = false
	a.quitConfirmed = confirmed
	a.forceQuit.Store(confirmed)
	return confirmed
}

func (a *App) RespondQuit(confirmed bool) {
	a.respondQuit(confirmed, runtime.Quit)
}

func (a *App) respondQuit(confirmed bool, requestQuit func(context.Context)) {
	if !a.resolveQuitConfirmation(confirmed) {
		return
	}
	logging.Info("quit confirmed by user")
	if ctx, ok := a.wailsContext(); ok {
		requestQuit(ctx)
	}
}
