//go:build windows

package app

import (
	"bytes"
	"image"
	"image/png"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"mnemo-go/internal/logging"
	"mnemo-go/internal/model"
)

// 传输悬浮窗 Windows 实现：layered window（per-pixel alpha）+ 专属 UI 线程。
//
// 关键约束与升级：
//   - 尺寸收窄至 138×38 逻辑 px，更加小巧精致；
//   - 纯色实体背景（100% 不透明），支持跟随系统/应用的深色与浅色双套主题；
//   - 智能单双行排版：仅下载/仅上传时采用单行大字显示，同时有下载和上传时采用双行紧凑小字；
//   - Apple 风格弹性弹簧动效（Spring Ease-out + 果冻回弹 Jelly Pulse）；
//   - 拖拽过程永远保持固定物理尺寸（SWP_NOSIZE|SWP_NOZORDER|SWP_NOACTIVATE），位图与窗口尺寸同源，彻底杜绝累加形变。

const (
	floaterClassName = "MnemoTransferFloater"

	flLogiW      = 112 // 逻辑宽度 @96DPI（稍微加宽 10px，留足呼吸边距）
	flLogiH      = 34  // 逻辑高度 @96DPI
	flCardR      = 7   // 卡片圆角
	flLogoSize   = 18  // Logo 尺寸
	flLogoR      = 4   // Logo 圆角
	flLogoX      = 7   // Logo X 偏移
	flLogoY      = 8   // Logo Y 偏移
	flTextX      = 30  // 文字区域 X 起点
	flFontLarge  = 10  // 单行大字号（原 12 小 2 个字号，更加精巧）
	flFontSmall  = 8   // 双行小字号（原 9/10 小 2 个字号）
	flFontStatus = 10  // 状态提示字号（原 11/12 小 2 个字号）

	wmFloaterUpdate = 0x0400 + 71 // WM_APP + 71
	wmFloaterTheme  = 0x0400 + 72 // WM_APP + 72

	wmTimer       = 0x0113
	wmLButtonDown = 0x0201
	wmLButtonUp   = 0x0202
	wmRButtonUp   = 0x0205
	wmMouseMove   = 0x0200
	wmClose       = 0x0010
	wmDestroy     = 0x0002
	wmDPIChanged  = 0x02E0

	flTimerAnim       = 1
	flTimerFullscreen = 2
	flTimerJelly      = 3
	flAnimDuration    = 260.0 // ms（入场弹簧过渡）
	flJellyDuration   = 320.0 // ms（状态突变果冻弹动）
	flSlidePx         = 8.0   // 入场弹性位移
)

var (
	flUser32   = windows.NewLazySystemDLL("user32.dll")
	flGdi32    = windows.NewLazySystemDLL("gdi32.dll")
	flShell32  = windows.NewLazySystemDLL("shell32.dll")
	flKernel32 = windows.NewLazySystemDLL("kernel32.dll")

	pRegisterClassExW   = flUser32.NewProc("RegisterClassExW")
	pCreateWindowExW    = flUser32.NewProc("CreateWindowExW")
	pDefWindowProcW     = flUser32.NewProc("DefWindowProcW")
	pGetMessageW        = flUser32.NewProc("GetMessageW")
	pTranslateMessage   = flUser32.NewProc("TranslateMessage")
	pDispatchMessageW   = flUser32.NewProc("DispatchMessageW")
	pPostMessageW       = flUser32.NewProc("PostMessageW")
	pPostQuitMessage    = flUser32.NewProc("PostQuitMessage")
	pDestroyWindow      = flUser32.NewProc("DestroyWindow")
	pShowWindow         = flUser32.NewProc("ShowWindow")
	pSetWindowPos       = flUser32.NewProc("SetWindowPos")
	pGetCursorPos       = flUser32.NewProc("GetCursorPos")
	pGetWindowRect      = flUser32.NewProc("GetWindowRect")
	pSetCapture         = flUser32.NewProc("SetCapture")
	pReleaseCapture     = flUser32.NewProc("ReleaseCapture")
	pSetTimer           = flUser32.NewProc("SetTimer")
	pKillTimer          = flUser32.NewProc("KillTimer")
	pGetDC              = flUser32.NewProc("GetDC")
	pReleaseDC          = flUser32.NewProc("ReleaseDC")
	pUpdateLayeredWin   = flUser32.NewProc("UpdateLayeredWindow")
	pSetForegroundWin   = flUser32.NewProc("SetForegroundWindow")
	pCreatePopupMenu    = flUser32.NewProc("CreatePopupMenu")
	pAppendMenuW        = flUser32.NewProc("AppendMenuW")
	pTrackPopupMenuEx   = flUser32.NewProc("TrackPopupMenuEx")
	pDestroyMenu        = flUser32.NewProc("DestroyMenu")
	pGetSystemMetrics   = flUser32.NewProc("GetSystemMetrics")
	pGetDpiForWindow    = flUser32.NewProc("GetDpiForWindow")
	pGetForegroundWin   = flUser32.NewProc("GetForegroundWindow")
	pGetWindowLongW     = flUser32.NewProc("GetWindowLongW")
	pMonitorFromWindow  = flUser32.NewProc("MonitorFromWindow")
	pGetMonitorInfoW    = flUser32.NewProc("GetMonitorInfoW")
	pLoadCursorW        = flUser32.NewProc("LoadCursorW")
	pFillRect           = flUser32.NewProc("FillRect")
	pDrawTextW          = flUser32.NewProc("DrawTextW")
	pCreateDIBSection   = flGdi32.NewProc("CreateDIBSection")
	pCreateCompatibleDC = flGdi32.NewProc("CreateCompatibleDC")
	pSelectObject       = flGdi32.NewProc("SelectObject")
	pDeleteObject       = flGdi32.NewProc("DeleteObject")
	pDeleteDC           = flGdi32.NewProc("DeleteDC")
	pCreateFontW        = flGdi32.NewProc("CreateFontW")
	pCreateSolidBrush   = flGdi32.NewProc("CreateSolidBrush")
	pSetBkMode          = flGdi32.NewProc("SetBkMode")
	pSetTextColor       = flGdi32.NewProc("SetTextColor")
	pGetTextExtentW     = flGdi32.NewProc("GetTextExtentPoint32W")
	pGetModuleHandleW   = flKernel32.NewProc("GetModuleHandleW")
	pRtlMoveMemory      = flKernel32.NewProc("RtlMoveMemory")
	pQueryNotifState    = flShell32.NewProc("SHQueryUserNotificationState")
)

func flCopyMemory(dst, src, n uintptr) {
	pRtlMoveMemory.Call(dst, src, n)
}

type flPoint struct{ X, Y int32 }
type flRect struct{ Left, Top, Right, Bottom int32 }
type flSize struct{ Cx, Cy int32 }

type flMsg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      flPoint
}

type flWndClassEx struct {
	Size      uint32
	Style     uint32
	WndProc   uintptr
	ClsExtra  int32
	WndExtra  int32
	Instance  uintptr
	Icon      uintptr
	Cursor    uintptr
	Brush     uintptr
	MenuName  *uint16
	ClassName *uint16
	IconSm    uintptr
}

type flBitmapInfo struct {
	Header struct {
		Size          uint32
		Width         int32
		Height        int32
		Planes        uint16
		BitCount      uint16
		Compression   uint32
		SizeImage     uint32
		XPelsPerMeter int32
		YPelsPerMeter int32
		ClrUsed       uint32
		ClrImportant  uint32
	}
	Masks [4]uint32
}

type flRGB struct{ R, G, B uint32 }

type flPalette struct {
	CardBg    flRGB
	Border    flRGB
	BorderA   float64
	TextMain  flRGB
	TextSub   flRGB
	DownArrow flRGB
	UpArrow   flRGB
	Done      flRGB
	Error     flRGB
	Pause     flRGB
}

var (
	flDarkTheme = flPalette{
		CardBg:    flRGB{22, 22, 28},    // 高质感深邃暗色
		Border:    flRGB{255, 255, 255}, // 细高光倒角描边
		BorderA:   0.16,
		TextMain:  flRGB{248, 248, 250}, // 主文字亮白
		TextSub:   flRGB{156, 163, 175}, // 弱化文字
		DownArrow: flRGB{167, 139, 250}, // 霓虹紫
		UpArrow:   flRGB{52, 211, 153},  // 翠绿
		Done:      flRGB{52, 211, 153},  // 成功绿
		Error:     flRGB{248, 113, 113}, // 失败红
		Pause:     flRGB{251, 191, 36},  // 暂停黄
	}
	flLightTheme = flPalette{
		CardBg:    flRGB{252, 252, 254}, // 珠光白
		Border:    flRGB{0, 0, 0},       // 细柔暗描边
		BorderA:   0.10,
		TextMain:  flRGB{17, 24, 39},    // 高清深灰黑
		TextSub:   flRGB{107, 114, 128}, // 次级灰
		DownArrow: flRGB{124, 58, 237},  // 品牌紫
		UpArrow:   flRGB{5, 150, 105},   // 森林绿
		Done:      flRGB{5, 150, 105},
		Error:     flRGB{220, 38, 38},
		Pause:     flRGB{217, 119, 6},
	}
)

// winFloater 为跨线程句柄；mu 保护共享状态，一切 Win32 对象只属于 UI 线程。
type winFloater struct {
	hooks  floaterHooks
	logo   []byte
	posX   int
	posY   int
	hasPos bool

	mu         sync.Mutex
	frame      floaterFrame
	suppressed bool
	isDark     bool

	hwnd atomic.Uintptr
	done atomic.Bool

	ui *flUI // 仅 UI 线程访问
}

var floaterWins sync.Map // hwnd → *winFloater

func newWinFloater(logo []byte, hooks floaterHooks, initial floaterFrame, suppressed bool, x, y int, hasPos bool) floaterView {
	w := &winFloater{
		hooks:      hooks,
		logo:       logo,
		posX:       x,
		posY:       y,
		hasPos:     hasPos,
		frame:      initial,
		suppressed: suppressed,
		isDark:     initial.Dark,
	}
	started := make(chan struct{})
	go w.run(started)
	<-started
	return w
}

func (w *winFloater) Present(f floaterFrame) {
	if w.done.Load() {
		return
	}
	w.mu.Lock()
	w.frame = f
	w.isDark = f.Dark
	w.mu.Unlock()
	if hwnd := w.hwnd.Load(); hwnd != 0 {
		pPostMessageW.Call(hwnd, wmFloaterUpdate, 0, 0)
	}
}

func (w *winFloater) SetSuppressed(suppressed bool) {
	if w.done.Load() {
		return
	}
	w.mu.Lock()
	w.suppressed = suppressed
	w.mu.Unlock()
	if hwnd := w.hwnd.Load(); hwnd != 0 {
		pPostMessageW.Call(hwnd, wmFloaterUpdate, 0, 0)
	}
}

func (w *winFloater) SetDark(isDark bool) {
	if w.done.Load() {
		return
	}
	w.mu.Lock()
	w.isDark = isDark
	w.frame.Dark = isDark
	w.mu.Unlock()
	if hwnd := w.hwnd.Load(); hwnd != 0 {
		pPostMessageW.Call(hwnd, wmFloaterTheme, 0, 0)
	}
}

func (w *winFloater) Close() {
	if w.done.Swap(true) {
		return
	}
	if hwnd := w.hwnd.Load(); hwnd != 0 {
		pPostMessageW.Call(hwnd, wmClose, 0, 0)
	}
}

// ---------- UI 线程 ----------

type flUI struct {
	hwnd   uintptr
	scale  float64
	pxW    int32
	pxH    int32
	memDC  uintptr
	dib    uintptr
	oldBmp uintptr
	dibBits []byte

	stripDC   uintptr
	stripBmp  uintptr
	stripOld  uintptr
	stripBits []byte
	stripW    int32
	stripH    int32
	brushBlk  uintptr

	fontLarge  uintptr // 单行大字号
	fontSmall  uintptr // 双行小字号
	fontStatus uintptr // 状态字号

	logoPx   int
	logoTile []byte

	visible    bool
	animating  bool
	animFrom   float64
	animTo     float64
	animStart  time.Time
	alpha      float64

	jellyAnim  bool
	jellyStart time.Time
	jellyOffY  float64

	fullTmrOn bool
	osFull    bool

	dragging bool
	dragOff  flPoint
	moved    bool

	frame      floaterFrame
	lastPhase  floaterPhase
	suppressed bool
	isDark     bool
	baseX      int32
	baseY      int32
}

func (w *winFloater) run(started chan struct{}) {
	runtime.LockOSThread() // 消息泵必须钉在固定 OS 线程

	ui := &flUI{
		frame:      w.frame,
		lastPhase:  w.frame.Phase,
		suppressed: w.suppressed,
		isDark:     w.isDark,
	}
	if !ui.create(w) {
		logging.Warn("floater window creation failed; floater disabled")
		close(started)
		return
	}
	w.ui = ui
	w.hwnd.Store(ui.hwnd)
	close(started)

	ui.applyVisibility()
	ui.render()

	var msg flMsg
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
	ui.destroy()
}

func (ui *flUI) create(w *winFloater) bool {
	className, err := windows.UTF16PtrFromString(floaterClassName)
	if err != nil {
		return false
	}
	hInst, _, _ := pGetModuleHandleW.Call(0)
	cursor, _, _ := pLoadCursorW.Call(0, uintptr(32512)) // IDC_ARROW

	wcex := flWndClassEx{
		Size:      uint32(unsafe.Sizeof(flWndClassEx{})),
		WndProc:   windows.NewCallback(floaterWndProc),
		Instance:  hInst,
		Cursor:    cursor,
		ClassName: className,
	}
	if r, _, _ := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wcex))); r == 0 {
		// 允许已注册
	}

	const (
		wsExLayered     = 0x00080000
		wsExTopmost     = 0x00000008
		wsExToolwindow  = 0x00000080
		wsExNoActivate  = 0x08000000
		wsPopUp         = 0x80000000
	)
	hwnd, _, _ := pCreateWindowExW.Call(
		uintptr(wsExLayered|wsExTopmost|wsExToolwindow|wsExNoActivate),
		uintptr(unsafe.Pointer(className)),
		0,
		uintptr(wsPopUp),
		0, 0, 0, 0,
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		return false
	}
	ui.hwnd = hwnd
	floaterWins.Store(hwnd, w)

	dpi, _, _ := pGetDpiForWindow.Call(hwnd)
	if dpi == 0 {
		dpi = 96
	}
	ui.setScale(float64(dpi) / 96.0)

	if w.hasPos {
		ui.baseX = int32(w.posX)
		ui.baseY = int32(w.posY)
	} else {
		sw, _, _ := pGetSystemMetrics.Call(0) // SM_CXSCREEN
		sh, _, _ := pGetSystemMetrics.Call(1) // SM_CYSCREEN
		ui.baseX = int32(sw) - ui.pxW - int32(24*ui.scale+0.5)
		ui.baseY = int32(sh) - ui.pxH - int32(72*ui.scale+0.5)
	}

	ui.clampBasePos()
	const swpNoSize = 0x0001
	const swpNoZOrder = 0x0004
	const swpNoActivate = 0x0010
	pSetWindowPos.Call(ui.hwnd, 0, uintptr(ui.baseX), uintptr(ui.baseY), uintptr(ui.pxW), uintptr(ui.pxH), swpNoSize|swpNoZOrder|swpNoActivate)
	return true
}

func (ui *flUI) setScale(s float64) {
	if s < 0.5 {
		s = 1.0
	}
	ui.scale = s
	ui.pxW = int32(float64(flLogiW)*s + 0.5)
	ui.pxH = int32(float64(flLogiH)*s + 0.5)
	ui.allocCanvas()
	ui.allocStrip()
	ui.makeFonts()
	ui.makeLogoTile()
}

func flBitmapInfoFor(w, h int32) flBitmapInfo {
	var bi flBitmapInfo
	bi.Header.Size = uint32(unsafe.Sizeof(bi.Header))
	bi.Header.Width = w
	bi.Header.Height = -h // top-down DIB
	bi.Header.Planes = 1
	bi.Header.BitCount = 32
	bi.Header.Compression = 3 // BI_BITFIELDS
	bi.Masks[0] = 0x00FF0000  // R
	bi.Masks[1] = 0x0000FF00  // G
	bi.Masks[2] = 0x000000FF  // B
	bi.Masks[3] = 0xFF000000  // A
	return bi
}

func (ui *flUI) allocCanvas() {
	if ui.dib != 0 {
		pSelectObject.Call(ui.memDC, ui.oldBmp)
		pDeleteObject.Call(ui.dib)
	}
	if ui.memDC == 0 {
		ui.memDC, _, _ = pCreateCompatibleDC.Call(0)
	}
	bi := flBitmapInfoFor(ui.pxW, ui.pxH)
	var bits unsafe.Pointer
	dib, _, _ := pCreateDIBSection.Call(0, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if dib == 0 {
		return
	}
	ui.dib = dib
	ui.dibBits = unsafe.Slice((*byte)(bits), int(ui.pxW)*int(ui.pxH)*4)
	ui.oldBmp, _, _ = pSelectObject.Call(ui.memDC, dib)
}

func (ui *flUI) allocStrip() {
	if ui.stripBmp != 0 {
		pSelectObject.Call(ui.stripDC, ui.stripOld)
		pDeleteObject.Call(ui.stripBmp)
	}
	if ui.stripDC == 0 {
		ui.stripDC, _, _ = pCreateCompatibleDC.Call(0)
		pSetBkMode.Call(ui.stripDC, 1) // TRANSPARENT
		pSetTextColor.Call(ui.stripDC, 0x00FFFFFF)
	}
	ui.stripW = int32(float64(flLogiW-flTextX-4)*ui.scale + 0.5)
	ui.stripH = int32(24*ui.scale + 0.5)
	bi := flBitmapInfoFor(ui.stripW, ui.stripH)
	var bits unsafe.Pointer
	bmp, _, _ := pCreateDIBSection.Call(0, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp == 0 {
		return
	}
	ui.stripBmp = bmp
	ui.stripBits = unsafe.Slice((*byte)(bits), int(ui.stripW)*int(ui.stripH)*4)
	ui.stripOld, _, _ = pSelectObject.Call(ui.stripDC, bmp)
	if ui.brushBlk == 0 {
		ui.brushBlk, _, _ = pCreateSolidBrush.Call(0)
	}
}

func (ui *flUI) makeFont(px int, bold bool) uintptr {
	face, _ := windows.UTF16PtrFromString("Microsoft YaHei UI")
	weight := uintptr(400) // FW_NORMAL
	if bold {
		weight = uintptr(600) // FW_SEMIBOLD
	}
	h, _, _ := pCreateFontW.Call(
		uintptr(int32(-px)), 0, 0, 0,
		weight,
		0, 0, 0,
		uintptr(1), // DEFAULT_CHARSET
		0, 0,
		uintptr(5), // ANTIALIASED_QUALITY
		0,
		uintptr(unsafe.Pointer(face)),
	)
	return h
}

func (ui *flUI) makeFonts() {
	if ui.fontLarge != 0 {
		pDeleteObject.Call(ui.fontLarge)
	}
	if ui.fontSmall != 0 {
		pDeleteObject.Call(ui.fontSmall)
	}
	if ui.fontStatus != 0 {
		pDeleteObject.Call(ui.fontStatus)
	}
	ui.fontLarge = ui.makeFont(int(flFontLarge*ui.scale+0.5), true)
	ui.fontSmall = ui.makeFont(int(flFontSmall*ui.scale+0.5), false)
	ui.fontStatus = ui.makeFont(int(flFontStatus*ui.scale+0.5), true)
}

func (ui *flUI) makeLogoTile() {
	ui.logoPx = int(flLogoSize*ui.scale + 0.5)
	tile := make([]byte, ui.logoPx*ui.logoPx*4)
	ui.logoTile = tile

	w := floaterOwner(ui)
	if w == nil || len(w.logo) == 0 {
		return
	}
	src, err := png.Decode(bytes.NewReader(w.logo))
	if err != nil {
		logging.Warn("floater logo decode failed", "error", err)
		return
	}
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	r := float64(flLogoR) * ui.scale
	N := float64(ui.logoPx)

	for y := 0; y < ui.logoPx; y++ {
		for x := 0; x < ui.logoPx; x++ {
			d := rrectSDF(float64(x)+0.5, float64(y)+0.5, N, N, r)
			maskA := clamp01(0.5 - d)
			if maskA <= 0 {
				continue
			}
			u := (float64(x) + 0.5) / N * float64(sw)
			v := (float64(y) + 0.5) / N * float64(sh)
			cr, cg, cb, ca := sampleBilinear(src, u, v, sw, sh)
			aa := float64(ca) * maskA
			o := (y*ui.logoPx + x) * 4
			tile[o+0] = uint8(float64(cb) * aa / 255.0)
			tile[o+1] = uint8(float64(cg) * aa / 255.0)
			tile[o+2] = uint8(float64(cr) * aa / 255.0)
			tile[o+3] = uint8(aa)
		}
	}
}

func floaterOwner(ui *flUI) *winFloater {
	if ui == nil {
		return nil
	}
	v, _ := floaterWins.Load(ui.hwnd)
	w, _ := v.(*winFloater)
	return w
}

func (ui *flUI) destroy() {
	floaterWins.Delete(ui.hwnd)
	if ui.dib != 0 {
		pSelectObject.Call(ui.memDC, ui.oldBmp)
		pDeleteObject.Call(ui.dib)
	}
	if ui.memDC != 0 {
		pDeleteDC.Call(ui.memDC)
	}
	if ui.stripBmp != 0 {
		pSelectObject.Call(ui.stripDC, ui.stripOld)
		pDeleteObject.Call(ui.stripBmp)
	}
	if ui.stripDC != 0 {
		pDeleteDC.Call(ui.stripDC)
	}
	if ui.brushBlk != 0 {
		pDeleteObject.Call(ui.brushBlk)
	}
	if ui.fontLarge != 0 {
		pDeleteObject.Call(ui.fontLarge)
	}
	if ui.fontSmall != 0 {
		pDeleteObject.Call(ui.fontSmall)
	}
	if ui.fontStatus != 0 {
		pDeleteObject.Call(ui.fontStatus)
	}
}

// ---------- 消息处理 ----------

func floaterWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	v, _ := floaterWins.Load(hwnd)
	w, _ := v.(*winFloater)
	if w == nil || w.ui == nil {
		r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
		return r
	}
	ui := w.ui

	switch msg {
	case wmFloaterUpdate:
		oldPhase := ui.frame.Phase
		ui.syncState(w)
		// 状态突变（Active -> Done / Error / Paused）时触发一次果冻弹性回弹
		if oldPhase == floaterActive && (ui.frame.Phase == floaterDone || ui.frame.Phase == floaterError || ui.frame.Phase == floaterPaused) {
			ui.startJelly()
		}
		ui.applyVisibility()
		ui.render()
		return 0
	case wmFloaterTheme:
		ui.syncState(w)
		ui.render()
		return 0
	case wmTimer:
		ui.onTimer(wParam)
		return 0
	case wmLButtonDown:
		ui.dragging = true
		ui.moved = false
		var pt flPoint
		pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
		ui.dragOff.X = pt.X - ui.baseX
		ui.dragOff.Y = pt.Y - ui.baseY
		pSetCapture.Call(hwnd)
		return 0
	case wmMouseMove:
		if ui.dragging {
			var pt flPoint
			pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
			nx := pt.X - ui.dragOff.X
			ny := pt.Y - ui.dragOff.Y
			if nx != ui.baseX || ny != ui.baseY {
				ui.moved = true
				ui.baseX = nx
				ui.baseY = ny
				ui.present()
			}
		}
		return 0
	case wmLButtonUp:
		if ui.dragging {
			ui.dragging = false
			pReleaseCapture.Call()
			if ui.moved {
				ui.clampBasePos()
				ui.present()
				if w.hooks.onMove != nil {
					w.hooks.onMove(int(ui.baseX), int(ui.baseY))
				}
			} else {
				if w.hooks.onOpen != nil {
					w.hooks.onOpen()
				}
			}
		}
		return 0
	case wmRButtonUp:
		ui.showMenu(w)
		return 0
	case wmDPIChanged:
		newDPI := uint32(wParam >> 16)
		ui.setScale(float64(newDPI) / 96.0)
		if lParam != 0 {
			var rc flRect
			flCopyMemory(uintptr(unsafe.Pointer(&rc)), lParam, unsafe.Sizeof(rc))
			ui.baseX, ui.baseY = rc.Left, rc.Top
		}
		ui.render()
		return 0
	case wmClose:
		pDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

func (ui *flUI) syncState(w *winFloater) {
	w.mu.Lock()
	ui.frame = w.frame
	ui.suppressed = w.suppressed
	ui.isDark = w.isDark
	w.mu.Unlock()
}

func (ui *flUI) wantVisible() bool {
	return ui.frame.Phase != floaterHidden && !ui.suppressed && !ui.osFull
}

func (ui *flUI) applyVisibility() {
	want := ui.wantVisible()
	if want && !ui.visible && !ui.animating {
		ui.visible = true
		const swShowNA = 8
		pShowWindow.Call(ui.hwnd, swShowNA)
		ui.setFullscreenTimer(true)
		ui.startAnim(1)
	} else if !want && ui.visible && !ui.animating {
		ui.startAnim(0)
	} else if ui.animating {
		if want && ui.animTo == 0 {
			ui.visible = true
			ui.startAnim(1)
		} else if !want && ui.animTo == 1 {
			ui.startAnim(0)
		}
	}
}

func (ui *flUI) startAnim(to float64) {
	ui.animating = true
	ui.animFrom = ui.alpha
	ui.animTo = to
	ui.animStart = time.Now()
	pSetTimer.Call(ui.hwnd, flTimerAnim, 16, 0)
}

func (ui *flUI) startJelly() {
	ui.jellyAnim = true
	ui.jellyStart = time.Now()
	pSetTimer.Call(ui.hwnd, flTimerJelly, 16, 0)
}

func (ui *flUI) setFullscreenTimer(on bool) {
	if on == ui.fullTmrOn {
		return
	}
	ui.fullTmrOn = on
	if on {
		pSetTimer.Call(ui.hwnd, flTimerFullscreen, 1500, 0)
	} else {
		pKillTimer.Call(ui.hwnd, flTimerFullscreen)
	}
}

func (ui *flUI) onTimer(id uintptr) {
	switch int(id) {
	case flTimerAnim:
		t := float64(time.Since(ui.animStart).Milliseconds()) / flAnimDuration
		if t >= 1 {
			t = 1
			ui.animating = false
			pKillTimer.Call(ui.hwnd, flTimerAnim)
		}
		// Apple 级弹簧缓动（入场过冲 + 回弹）
		var ease float64
		if ui.animTo == 1 {
			// Spring overshoot: 1 - e^(-6t) * cos(2.8 * pi * t)
			ease = 1.0 - math.Exp(-6.0*t)*math.Cos(2.8*math.Pi*t)
		} else {
			// Cubic ease-in-out for fade out
			ease = 1.0 - t*t*(3.0-2.0*t)
		}
		ui.alpha = ui.animFrom + (ui.animTo-ui.animFrom)*ease
		if !ui.animating && ui.alpha <= 0 {
			ui.visible = false
			pShowWindow.Call(ui.hwnd, 0) // SW_HIDE
			ui.setFullscreenTimer(false)
		}
		ui.present()
	case flTimerJelly:
		t := float64(time.Since(ui.jellyStart).Milliseconds()) / flJellyDuration
		if t >= 1 {
			ui.jellyAnim = false
			ui.jellyOffY = 0
			pKillTimer.Call(ui.hwnd, flTimerJelly)
		} else {
			// Jelly Pulse 阻尼震颤: sin(2.5 * pi * t) * e^(-4t) * 4px
			ui.jellyOffY = math.Sin(t*2.5*math.Pi) * math.Exp(-4.0*t) * 4.0 * ui.scale
		}
		ui.present()
	case flTimerFullscreen:
		if ui.visible {
			full := ui.foregroundFullscreen()
			if full != ui.osFull {
				ui.osFull = full
				ui.applyVisibility()
			}
		}
	}
}

func (ui *flUI) foregroundFullscreen() bool {
	var state uint32
	if r, _, _ := pQueryNotifState.Call(uintptr(unsafe.Pointer(&state))); r == 0 && state == 3 {
		return true
	}
	fg, _, _ := pGetForegroundWin.Call()
	if fg == 0 {
		return false
	}
	var rc flRect
	if r, _, _ := pGetWindowRect.Call(fg, uintptr(unsafe.Pointer(&rc))); r == 0 {
		return false
	}
	const gwlStyle = ^uintptr(15) // -16
	style, _, _ := pGetWindowLongW.Call(fg, gwlStyle)
	const wsCaption = 0x00C00000
	if style&wsCaption != 0 {
		return false
	}
	mon, _, _ := pMonitorFromWindow.Call(fg, 2) // MONITOR_DEFAULTTONEAREST
	type monitorInfo struct {
		CbSize    uint32
		RcMonitor flRect
		RcWork    flRect
		DwFlags   uint32
	}
	mi := monitorInfo{CbSize: uint32(unsafe.Sizeof(monitorInfo{}))}
	if r, _, _ := pGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi))); r == 0 {
		return false
	}
	m := mi.RcMonitor
	return rc.Left <= m.Left && rc.Top <= m.Top && rc.Right >= m.Right && rc.Bottom >= m.Bottom
}

// ---------- 右键菜单 ----------

func (ui *flUI) showMenu(w *winFloater) {
	menu, _, _ := pCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer pDestroyMenu.Call(menu)
	s1, _ := windows.UTF16PtrFromString("显示主窗口")
	s2, _ := windows.UTF16PtrFromString("隐藏悬浮窗")
	pAppendMenuW.Call(menu, 0, 1, uintptr(unsafe.Pointer(s1)))
	pAppendMenuW.Call(menu, 0, 2, uintptr(unsafe.Pointer(s2)))

	var pt flPoint
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pSetForegroundWin.Call(ui.hwnd)
	const tpmReturnCmd = 0x0100
	const tpmNonNotify = 0x0080
	cmd, _, _ := pTrackPopupMenuEx.Call(menu, uintptr(tpmReturnCmd|tpmNonNotify), uintptr(pt.X), uintptr(pt.Y), ui.hwnd, 0)
	pPostMessageW.Call(ui.hwnd, 0, 0, 0)
	switch cmd {
	case 1:
		if w.hooks.onOpen != nil {
			w.hooks.onOpen()
		}
	case 2:
		if w.hooks.onHide != nil {
			w.hooks.onHide()
		}
	}
}

func (ui *flUI) clampBasePos() {
	mon, _, _ := pMonitorFromWindow.Call(ui.hwnd, 2)
	if mon == 0 {
		return
	}
	type monitorInfo struct {
		CbSize    uint32
		RcMonitor flRect
		RcWork    flRect
		DwFlags   uint32
	}
	mi := monitorInfo{CbSize: uint32(unsafe.Sizeof(monitorInfo{}))}
	if r, _, _ := pGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi))); r == 0 {
		return
	}
	w := mi.RcWork
	margin := int32(4 * ui.scale)
	if ui.baseX < w.Left+margin {
		ui.baseX = w.Left + margin
	}
	if ui.baseX+ui.pxW > w.Right-margin {
		ui.baseX = w.Right - margin - ui.pxW
	}
	if ui.baseY < w.Top+margin {
		ui.baseY = w.Top + margin
	}
	if ui.baseY+ui.pxH > w.Bottom-margin {
		ui.baseY = w.Bottom - margin - ui.pxH
	}
}

// ---------- 渲染管线 ----------

func (ui *flUI) palette() flPalette {
	if ui.isDark {
		return flDarkTheme
	}
	return flLightTheme
}

func (ui *flUI) render() {
	if ui.dibBits == nil || ui.pxW <= 0 || ui.pxH <= 0 {
		return
	}
	for i := range ui.dibBits {
		ui.dibBits[i] = 0
	}

	pal := ui.palette()
	W := float64(ui.pxW)
	H := float64(ui.pxH)
	r := float64(flCardR) * ui.scale
	ga := ui.alpha
	if ga < 0 {
		ga = 0
	}
	if ga > 1 {
		ga = 1
	}

	// 100% 实体纯色卡片（非半透明）+ 抗锯齿边缘 + 细内沿描边
	for y := 0; y < int(ui.pxH); y++ {
		for x := 0; x < int(ui.pxW); x++ {
			d := rrectSDF(float64(x)+0.5, float64(y)+0.5, W, H, r)
			a := clamp01(0.5 - d)
			if a <= 0 {
				continue
			}
			o := (y*int(ui.pxW) + x) * 4
			// 卡片实体纯色填充（100% 不透明）
			ui.blendOver(ui.dibBits[o:], pal.CardBg, a*ga)
			// 精致内沿描边（1.2px 平滑过渡）
			ui.blendOver(ui.dibBits[o:], pal.Border, a*clamp01(d+1.2)*pal.BorderA*ga)
		}
	}

	// 绘制精致 Logo
	ls := ui.logoPx
	lx := int(float64(flLogoX)*ui.scale + 0.5)
	ly := int(float64(flLogoY)*ui.scale + 0.5)
	for y := 0; y < ls; y++ {
		for x := 0; x < ls; x++ {
			s := (y*ls + x) * 4
			sa := uint32(ui.logoTile[s+3])
			if sa == 0 {
				continue
			}
			dx, dy := lx+x, ly+y
			if dx < 0 || dy < 0 || dx >= int(ui.pxW) || dy >= int(ui.pxH) {
				continue
			}
			o := (dy*int(ui.pxW) + dx) * 4
			ui.blendOverPremul(ui.dibBits[o:], ui.logoTile[s:], ga)
		}
	}

	// 文字与状态排版
	tx := int(float64(flTextX)*ui.scale + 0.5)

	switch ui.frame.Phase {
	case floaterActive:
		hasDown := ui.frame.Down > 0
		hasUp := ui.frame.Up > 0

		if hasDown && hasUp {
			// 同时有下载与上传：双行紧凑小字上下排列
			ui.drawSpeedRow(tx, int(3*ui.scale+0.5), "↓", pal.DownArrow, ui.frame.Down, ui.fontSmall, pal.TextMain, pal.TextSub, ga)
			ui.drawSpeedRow(tx, int(17*ui.scale+0.5), "↑", pal.UpArrow, ui.frame.Up, ui.fontSmall, pal.TextMain, pal.TextSub, ga)
		} else if hasUp && !hasDown {
			// 仅有上传：单行大字
			ui.drawSpeedRow(tx, int(8*ui.scale+0.5), "↑", pal.UpArrow, ui.frame.Up, ui.fontLarge, pal.TextMain, pal.TextSub, ga)
		} else {
			// 仅有下载（或初始任务排队中）：单行大字
			ui.drawSpeedRow(tx, int(8*ui.scale+0.5), "↓", pal.DownArrow, ui.frame.Down, ui.fontLarge, pal.TextMain, pal.TextSub, ga)
		}
	case floaterDone:
		ui.drawStatusLine(tx, int(8*ui.scale+0.5), "✓ 传输完成", pal.Done, ga)
	case floaterError:
		ui.drawStatusLine(tx, int(8*ui.scale+0.5), "✕ 传输失败", pal.Error, ga)
	case floaterPaused:
		ui.drawStatusLine(tx, int(8*ui.scale+0.5), "⏸ 传输已暂停", pal.Pause, ga)
	}

	ui.present()
}

func (ui *flUI) drawSpeedRow(x, y int, arrow string, arrowCol flRGB, speed int64, font uintptr, textCol, weakCol flRGB, ga float64) {
	aw := ui.drawText(x, y, arrow, font, arrowCol, ga)
	spCol := textCol
	if speed <= 0 {
		spCol = weakCol
	}
	ui.drawText(x+aw+int(4*ui.scale+0.5), y, model.FormatSpeed(speed), font, spCol, ga)
}

func (ui *flUI) drawStatusLine(x, y int, text string, col flRGB, ga float64) {
	ui.drawText(x, y, text, ui.fontStatus, col, ga)
}

func (ui *flUI) drawText(dx, dy int, text string, font uintptr, col flRGB, ga float64) int {
	if ui.stripDC == 0 || ui.stripBmp == 0 || font == 0 {
		return 0
	}
	rc := flRect{0, 0, ui.stripW, ui.stripH}
	pFillRect.Call(ui.stripDC, uintptr(unsafe.Pointer(&rc)), ui.brushBlk)
	pSelectObject.Call(ui.stripDC, font)
	u16, err := windows.UTF16FromString(text)
	if err != nil || len(u16) == 0 {
		return 0
	}
	tp := &u16[0]
	const dtSingleLine = 0x0020
	const dtNoPrefix = 0x0800
	pDrawTextW.Call(ui.stripDC, uintptr(unsafe.Pointer(tp)), ^uintptr(0),
		uintptr(unsafe.Pointer(&rc)), uintptr(dtSingleLine|dtNoPrefix))

	var sz flSize
	pGetTextExtentW.Call(ui.stripDC, uintptr(unsafe.Pointer(tp)), uintptr(len(u16)-1), uintptr(unsafe.Pointer(&sz)))

	for y := 0; y < int(ui.stripH); y++ {
		dyY := dy + y
		if dyY < 0 || dyY >= int(ui.pxH) {
			continue
		}
		for x := 0; x < int(ui.stripW); x++ {
			dxX := dx + x
			if dxX < 0 || dxX >= int(ui.pxW) {
				continue
			}
			ma := uint32(ui.stripBits[(y*int(ui.stripW)+x)*4])
			if ma == 0 {
				continue
			}
			o := (dyY*int(ui.pxW) + dxX) * 4
			ui.blendOver(ui.dibBits[o:], col, float64(ma)/255.0*ga)
		}
	}
	return int(sz.Cx)
}

func (ui *flUI) present() {
	if ui.dib == 0 {
		return
	}
	screen, _, _ := pGetDC.Call(0)
	defer pReleaseDC.Call(0, screen)

	size := flSize{Cx: ui.pxW, Cy: ui.pxH}
	slide := int32(0)
	if ui.alpha < 1 {
		slide = int32((1.0 - ui.alpha) * flSlidePx * ui.scale)
	}
	jellyY := int32(ui.jellyOffY)

	ptDst := flPoint{X: ui.baseX, Y: ui.baseY + slide + jellyY}
	ptSrc := flPoint{0, 0}
	blend := [4]byte{0, 0, 255, 1} // AC_SRC_OVER, 0, 255, AC_SRC_ALPHA
	pUpdateLayeredWin.Call(ui.hwnd, screen, uintptr(unsafe.Pointer(&ptDst)), uintptr(unsafe.Pointer(&size)),
		ui.memDC, uintptr(unsafe.Pointer(&ptSrc)), 0, uintptr(unsafe.Pointer(&blend)), 2) // ULW_ALPHA
}

func (ui *flUI) blendOver(dst []byte, c flRGB, a float64) {
	if a <= 0 {
		return
	}
	if a > 1 {
		a = 1
	}
	srcR := uint32(float64(c.R)*a + 0.5)
	srcG := uint32(float64(c.G)*a + 0.5)
	srcB := uint32(float64(c.B)*a + 0.5)
	srcA := uint32(a*255.0 + 0.5)
	invA := 255 - srcA

	dst[0] = uint8(srcB + (uint32(dst[0])*invA+127)/255)
	dst[1] = uint8(srcG + (uint32(dst[1])*invA+127)/255)
	dst[2] = uint8(srcR + (uint32(dst[2])*invA+127)/255)
	dst[3] = uint8(srcA + (uint32(dst[3])*invA+127)/255)
}

func (ui *flUI) blendOverPremul(dst, src []byte, ga float64) {
	sa := float64(src[3]) * ga
	if sa <= 0 {
		return
	}
	invA := uint32(255.0 - sa + 0.5)
	k := ga
	dst[0] = uint8(uint32(float64(src[0])*k+0.5) + (uint32(dst[0])*invA+127)/255)
	dst[1] = uint8(uint32(float64(src[1])*k+0.5) + (uint32(dst[1])*invA+127)/255)
	dst[2] = uint8(uint32(float64(src[2])*k+0.5) + (uint32(dst[2])*invA+127)/255)
	dst[3] = uint8(uint32(sa+0.5) + (uint32(dst[3])*invA+127)/255)
}

func rrectSDF(px, py, w, h, r float64) float64 {
	hw, hh := w/2, h/2
	x := math.Abs(px-hw) - (hw - r)
	y := math.Abs(py-hh) - (hh - r)
	dx := math.Max(x, 0)
	dy := math.Max(y, 0)
	outside := math.Sqrt(dx*dx + dy*dy)
	inside := math.Min(math.Max(x, y), 0)
	return outside + inside - r
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func sampleBilinear(src image.Image, u, v float64, w, h int) (r, g, b, a uint32) {
	u = math.Max(0, math.Min(u, float64(w-1)))
	v = math.Max(0, math.Min(v, float64(h-1)))
	x0 := int(u)
	y0 := int(v)
	x1 := x0 + 1
	if x1 >= w {
		x1 = w - 1
	}
	y1 := y0 + 1
	if y1 >= h {
		y1 = h - 1
	}
	fx := u - float64(x0)
	fy := v - float64(y0)

	c00r, c00g, c00b, c00a := src.At(x0, y0).RGBA()
	c10r, c10g, c10b, c10a := src.At(x1, y0).RGBA()
	c01r, c01g, c01b, c01a := src.At(x0, y1).RGBA()
	c11r, c11g, c11b, c11a := src.At(x1, y1).RGBA()

	to8 := func(v uint32) float64 { return float64(v >> 8) }
	lerp := func(v00, v10, v01, v11 float64) uint32 {
		top := v00*(1-fx) + v10*fx
		bot := v01*(1-fx) + v11*fx
		return uint32(top*(1-fy) + bot*fy + 0.5)
	}

	r00, r10, r01, r11 := to8(c00r), to8(c10r), to8(c01r), to8(c11r)
	g00, g10, g01, g11 := to8(c00g), to8(c10g), to8(c01g), to8(c11g)
	b00, b10, b01, b11 := to8(c00b), to8(c10b), to8(c01b), to8(c11b)
	a00, a10, a01, a11 := to8(c00a), to8(c10a), to8(c01a), to8(c11a)

	return lerp(r00, r10, r01, r11), lerp(g00, g10, g01, g11), lerp(b00, b10, b01, b11), lerp(a00, a10, a01, a11)
}

// ---------- 平台接口入口 ----------

func (f *floater) newView() floaterView {
	x, y, hasPos := f.savedPosition()
	return newWinFloater(f.logo, f.hooks(), f.initialFrame(), f.suppressed(), x, y, hasPos)
}
