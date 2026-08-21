//go:build !windows

package app

// stubFloaterView：Linux/macOS 暂无原生悬浮窗实现（见 docs/FLOATER.md §2.6）。
type stubFloaterView struct{}

func (stubFloaterView) Present(floaterFrame) {}
func (stubFloaterView) SetSuppressed(bool)   {}
func (stubFloaterView) SetDark(bool)         {}
func (stubFloaterView) Close()               {}

func (f *floater) newView() floaterView { return stubFloaterView{} }
