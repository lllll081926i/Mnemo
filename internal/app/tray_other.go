//go:build !windows

package app

// AcquireSingleInstance 非 Windows 平台暂不限制实例数。
func AcquireSingleInstance(string) bool { return true }

// WatchShowRequests 非 Windows 平台无唤起信号。
func (a *App) WatchShowRequests() {}

// SetupTray 非 Windows 平台不启用托盘。
func (a *App) SetupTray([]byte) {}
