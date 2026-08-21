package app

import (
	"sync"
	"time"

	"mnemo-go/internal/logging"
	"mnemo-go/internal/transfer"
)

// 传输悬浮窗控制器（平台无关部分）：聚合下载/上传速度，维护
// hidden → active → done/error → hidden 状态机，把帧推给平台原生视图。
// 视图懒创建：没有任何活动任务时不存在窗口与渲染资源。

type floaterPhase int

const (
	floaterHidden floaterPhase = iota
	floaterActive
	floaterPaused
	floaterDone
	floaterError
)

// floaterFrame 是推送给原生视图的一帧状态。
type floaterFrame struct {
	Phase floaterPhase
	Down  int64 // bytes/s
	Up    int64 // bytes/s
	Dark  bool  // 是否深色主题
}

// floaterHooks 由原生视图回调控制器/应用层。
type floaterHooks struct {
	onOpen func()         // 左键单击：唤起主窗口并跳到传输页
	onMove func(x, y int) // 拖拽结束：持久化位置（物理像素）
	onHide func()         // 右键「隐藏悬浮窗」：写入设置关闭
}

// floaterView 是平台原生悬浮窗接口（Windows 完整实现，其余平台为空实现）。
type floaterView interface {
	Present(f floaterFrame)
	SetSuppressed(suppressed bool)
	SetDark(isDark bool)
	Close()
}

const (
	floaterDoneHold   = 1200 * time.Millisecond // 传输完成后 1.2 秒优雅淡出
	floaterErrorHold  = 2500 * time.Millisecond
	floaterPausedHold = 2000 * time.Millisecond
	floaterPushEvery  = 250 * time.Millisecond // 速度刷新节流 4Hz
)

type floater struct {
	a    *App
	logo []byte

	mu         sync.Mutex
	down       map[string]int64
	up         map[string]int64
	live       map[string]bool
	paused     map[string]bool
	phase      floaterPhase
	sawError   bool // 本轮活跃期是否出现过失败
	sawPause   bool // 本轮活跃期是否暂停
	playerFS   bool // 播放器全屏抑制
	enabled    bool
	isDark     bool

	timer    *time.Timer // done/error/pause 自动隐藏
	throttle *time.Timer // 尾随刷新
	lastPush time.Time

	viewMu sync.Mutex
	view   floaterView
}

func newFloater(a *App, logo []byte) *floater {
	return &floater{
		a:       a,
		logo:    logo,
		down:    make(map[string]int64),
		up:      make(map[string]int64),
		live:    make(map[string]bool),
		paused:  make(map[string]bool),
		enabled: true,
		isDark:  true,
	}
}

// SetupFloater 注入 logo（PNG 字节）。仅保存，原生资源在首个活动任务时才创建。
func (a *App) SetupFloater(logo []byte) {
	a.stateMu.Lock()
	defer a.stateMu.Unlock()
	if a.floater == nil {
		a.floater = newFloater(a, logo)
	} else {
		a.floater.logo = logo
	}
}

// OnTaskEvent 由下载管理器/上传队列的事件回调驱动。
func (f *floater) OnTaskEvent(ev transfer.TaskEvent) {
	if f == nil {
		return
	}
	t := ev.Task
	key := ev.Kind + ":" + t.ID
	active := t.Status == "downloading" || t.Status == "uploading" || t.Status == "queued"
	paused := t.Status == "paused" || t.Status == "stopped"

	f.mu.Lock()
	if active {
		f.live[key] = true
		delete(f.paused, key)
		if ev.Kind == "upload" {
			f.up[key] = t.Speed
			delete(f.down, key)
		} else {
			f.down[key] = t.Speed
			delete(f.up, key)
		}
	} else {
		if f.live[key] && t.Status == "failed" {
			f.sawError = true
		} else if f.live[key] && paused {
			f.sawPause = true
		}
		if paused {
			f.paused[key] = true
		} else {
			delete(f.paused, key)
		}
		delete(f.live, key)
		delete(f.down, key)
		delete(f.up, key)
	}

	var downSum, upSum int64
	for _, v := range f.down {
		downSum += v
	}
	for _, v := range f.up {
		upSum += v
	}

	switch {
	case len(f.live) > 0:
		if f.phase != floaterActive {
			f.phase = floaterActive
			f.sawError = false
			f.sawPause = false
		}
		if f.timer != nil {
			f.timer.Stop()
			f.timer = nil
		}
	case f.phase == floaterActive:
		// 活跃任务刚清零：按本轮结局进入完成/暂停/出错态
		switch {
		case f.sawError:
			f.phase = floaterError
		case f.sawPause:
			f.phase = floaterPaused
		default:
			f.phase = floaterDone
		}
		hold := floaterDoneHold
		if f.phase == floaterError {
			hold = floaterErrorHold
		} else if f.phase == floaterPaused {
			hold = floaterPausedHold
		}
		f.timer = time.AfterFunc(hold, func() { f.transitionToHidden() })
	default:
		// 已是 done/error/paused/hidden，任务删除等事件不改变现状
	}
	phase := f.phase
	isDark := f.isDark
	f.mu.Unlock()

	f.push(floaterFrame{Phase: phase, Down: downSum, Up: upSum, Dark: isDark}, phase != floaterActive)
}

// transitionToHidden 完成/出错/暂停态停留结束后淡出。
func (f *floater) transitionToHidden() {
	f.mu.Lock()
	if f.phase == floaterDone || f.phase == floaterError || f.phase == floaterPaused {
		f.phase = floaterHidden
	}
	isDark := f.isDark
	f.mu.Unlock()
	f.push(floaterFrame{Phase: floaterHidden, Dark: isDark}, true)
}

// DismissTerminal 用户在完成/出错/暂停态点击：立即隐藏并打开传输页。
func (f *floater) dismissTerminal() {
	f.mu.Lock()
	terminal := f.phase == floaterDone || f.phase == floaterError || f.phase == floaterPaused
	if terminal {
		f.phase = floaterHidden
		if f.timer != nil {
			f.timer.Stop()
			f.timer = nil
		}
	}
	isDark := f.isDark
	f.mu.Unlock()
	if terminal {
		f.push(floaterFrame{Phase: floaterHidden, Dark: isDark}, true)
	}
}

// SetDark 设置明暗主题（深色/浅色）。
func (f *floater) SetDark(isDark bool) {
	f.mu.Lock()
	if f.isDark == isDark {
		f.mu.Unlock()
		return
	}
	f.isDark = isDark
	phase := f.phase
	var downSum, upSum int64
	for _, v := range f.down {
		downSum += v
	}
	for _, v := range f.up {
		upSum += v
	}
	f.mu.Unlock()

	f.viewMu.Lock()
	v := f.view
	f.viewMu.Unlock()
	if v != nil {
		v.SetDark(isDark)
	}
	f.push(floaterFrame{Phase: phase, Down: downSum, Up: upSum, Dark: isDark}, true)
}

// SetPlayerFullscreen 前端播放器全屏变化时抑制/恢复悬浮窗。
func (f *floater) SetPlayerFullscreen(full bool) {
	f.mu.Lock()
	f.playerFS = full
	f.mu.Unlock()
	f.applySuppression()
}

// ApplySettings 设置保存后同步开关状态。
func (f *floater) ApplySettings(enabled bool) {
	f.mu.Lock()
	f.enabled = enabled
	f.mu.Unlock()
	f.applySuppression()
}

func (f *floater) applySuppression() {
	f.mu.Lock()
	suppressed := f.playerFS || !f.enabled
	f.mu.Unlock()
	f.viewMu.Lock()
	v := f.view
	f.viewMu.Unlock()
	if v != nil {
		v.SetSuppressed(suppressed)
	}
}

// push 把帧发给视图；活动态按 4Hz 节流（尾随保证最后一帧不丢）。
func (f *floater) push(frame floaterFrame, immediate bool) {
	if !immediate {
		f.mu.Lock()
		if d := time.Since(f.lastPush); d < floaterPushEvery {
			if f.throttle == nil {
				f.throttle = time.AfterFunc(floaterPushEvery-d, func() {
					f.mu.Lock()
					f.throttle = nil
					var down, up int64
					for _, v := range f.down {
						down += v
					}
					for _, v := range f.up {
						up += v
					}
					phase := f.phase
					f.mu.Unlock()
					f.push(floaterFrame{Phase: phase, Down: down, Up: up}, true)
				})
			}
			f.mu.Unlock()
			return
		}
		f.lastPush = time.Now()
		f.mu.Unlock()
	}

	f.viewMu.Lock()
	if f.view == nil {
		if frame.Phase == floaterHidden || !f.enabledSnapshot() {
			f.viewMu.Unlock()
			return // 隐藏态/已关闭时不创建窗口，零资源占用
		}
		f.view = f.newView()
	}
	v := f.view
	f.viewMu.Unlock()
	v.Present(frame)
}

// Close 应用退出时销毁视图。
func (f *floater) Close() {
	f.viewMu.Lock()
	v := f.view
	f.view = nil
	f.viewMu.Unlock()
	if v != nil {
		v.Close()
	}
}

// hooks 组装应用层回调。
func (f *floater) hooks() floaterHooks {
	return floaterHooks{
		onOpen: func() {
			f.dismissTerminal()
			f.a.ShowMainWindow()
			f.a.emit("nav:tab", "transfer")
		},
		onMove: func(x, y int) {
			st, err := f.a.storeOrError()
			if err != nil {
				return
			}
			if err := st.SetFloaterPosition(x, y); err != nil {
				logging.Warn("floater position persistence failed", "error", err)
			}
		},
		onHide: func() {
			st, err := f.a.storeOrError()
			if err != nil {
				return
			}
			if err := st.SetFloaterEnabled(false); err != nil {
				logging.Warn("floater disable persistence failed", "error", err)
			}
		},
	}
}

// initialFrame 返回视图创建时的初始帧。
func (f *floater) initialFrame() floaterFrame {
	f.mu.Lock()
	defer f.mu.Unlock()
	var down, up int64
	for _, v := range f.down {
		down += v
	}
	for _, v := range f.up {
		up += v
	}
	return floaterFrame{Phase: f.phase, Down: down, Up: up, Dark: f.isDark}
}

func (f *floater) suppressed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.playerFS || !f.enabled
}

func (f *floater) enabledSnapshot() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.enabled
}

func (f *floater) savedPosition() (x, y int, ok bool) {
	s := f.a.GetSettings()
	return s.FloaterX, s.FloaterY, s.FloaterPos
}
