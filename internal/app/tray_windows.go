//go:build windows

package app

import (
	"errors"
	"unsafe"

	"github.com/energye/systray"
	"golang.org/x/sys/windows"

	"mnemo-go/internal/logging"
)

// AcquireSingleInstance 通过命名互斥体保证单实例运行。已存在实例时激活其
// 主窗口并返回 false（调用方应直接退出）。
func AcquireSingleInstance(windowTitle string) bool {
	name, err := windows.UTF16PtrFromString(`Local\MnemoGo.SingleInstance`)
	if err != nil {
		return true
	}
	_, err = windows.CreateMutex(nil, false, name)
	switch {
	case err == nil:
		return true
	case errors.Is(err, windows.ERROR_ALREADY_EXISTS):
		logging.Info("another instance detected, activating existing window")
		activateExistingWindow(windowTitle)
		return false
	default:
		logging.Warn("single instance mutex failed", "error", err)
		return true
	}
}

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	procFindWindowW      = user32.NewProc("FindWindowW")
	procShowWindow       = user32.NewProc("ShowWindow")
	procBringWindowToTop = user32.NewProc("BringWindowToTop")
	procSetForeground    = user32.NewProc("SetForegroundWindow")
)

func activateExistingWindow(title string) {
	t, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(t)))
	if hwnd == 0 {
		logging.Warn("existing instance window not found", "title", title)
		return
	}
	const swRestore = 9
	procShowWindow.Call(hwnd, swRestore)
	procBringWindowToTop.Call(hwnd)
	procSetForeground.Call(hwnd)
}

// SetupTray 启动系统托盘图标：左键点击或菜单「显示」恢复主窗口，
// 「退出」强制退出整个应用。
func (a *App) SetupTray(icon []byte) {
	go systray.Run(func() {
		systray.SetIcon(icon)
		systray.SetTooltip("Mnemo")
		systray.SetOnClick(func(systray.IMenu) { a.ShowMainWindow() })
		systray.AddMenuItem("显示 Mnemo", "显示主窗口").Click(func() { a.ShowMainWindow() })
		systray.AddSeparator()
		systray.AddMenuItem("退出 Mnemo", "完全退出").Click(func() { a.ForceQuit() })
	}, func() {})
}
