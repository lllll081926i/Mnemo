//go:build windows

package app

import (
	"errors"
	"unsafe"

	"github.com/energye/systray"
	"golang.org/x/sys/windows"

	"mnemo-go/internal/logging"
)

// AcquireSingleInstance 通过命名互斥体保证单实例运行。已存在实例时通过命名
// 事件通知其唤起主窗口（兜底再用 FindWindow 跨进程置前）并返回 false。
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
		logging.Info("another instance detected, signaling existing window")
		if !signalExistingInstance() {
			activateExistingWindow(windowTitle)
		}
		return false
	default:
		logging.Warn("single instance mutex failed", "error", err)
		return true
	}
}

const showEventName = `Local\MnemoGo.ShowRequest`

// WatchShowRequests 第一实例监听「唤起主窗口」信号（二次启动/其他入口触发）。
func (a *App) WatchShowRequests() {
	name, err := windows.UTF16PtrFromString(showEventName)
	if err != nil {
		return
	}
	h, err := windows.CreateEvent(nil, 0, 0, name)
	if err != nil {
		logging.Warn("show-request event creation failed", "error", err)
		return
	}
	go func() {
		for {
			r, err := windows.WaitForSingleObject(h, windows.INFINITE)
			if r != windows.WAIT_OBJECT_0 || err != nil {
				return
			}
			a.ShowMainWindow()
		}
	}()
}

// signalExistingInstance 向已运行实例发送唤起信号；事件不存在（对方尚未
// 完成启动）时返回 false，由调用方走 FindWindow 兜底。
func signalExistingInstance() bool {
	name, err := windows.UTF16PtrFromString(showEventName)
	if err != nil {
		return false
	}
	h, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, name)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	return windows.SetEvent(h) == nil
}

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	procFindWindowW      = user32.NewProc("FindWindowW")
	procShowWindow       = user32.NewProc("ShowWindow")
	procIsIconic         = user32.NewProc("IsIconic")
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
	// SW_RESTORE 只对最小化有效；隐藏（托盘）窗口需要 SW_SHOW
	cmd := uintptr(5) // SW_SHOW
	if iconic, _, _ := procIsIconic.Call(hwnd); iconic != 0 {
		cmd = 9 // SW_RESTORE
	}
	procShowWindow.Call(hwnd, cmd)
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
