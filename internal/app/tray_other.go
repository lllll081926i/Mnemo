//go:build !windows

package app

// AcquireSingleInstance 非 Windows 平台暂不限制实例数。
func AcquireSingleInstance(string) bool { return true }

// SetupTray 非 Windows 平台不启用托盘。
func (a *App) SetupTray([]byte) {}
