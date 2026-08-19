package app

import (
	"context"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"mnemo-go/internal/logging"
)

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
// 窗口到托盘而非退出；返回 true 表示阻止关闭。
func (a *App) BeforeClose(ctx context.Context) bool {
	if a.forceQuit.Load() {
		return false
	}
	if st, err := a.storeOrError(); err == nil {
		if s, serr := st.GetSettings(); serr == nil && !s.CloseToTrayEnabled() {
			return false
		}
	}
	logging.Info("window close intercepted, hiding to tray")
	runtime.WindowHide(ctx)
	return true
}
