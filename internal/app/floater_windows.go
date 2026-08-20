//go:build windows

package app

import (
	"bytes"
	"image"
	"image/png"
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
// 关键约束（docs/FLOATER.md §1.5）：
//   - 拖拽只 SetWindowPos(..., SWP_NOSIZE|SWP_NOZORDER|SWP_NOACTIVATE)，永不改尺寸；
//     尺寸唯一计算点是 logicalSize×dpiScale（创建与 WM_DPICHANGED 时），
//     位图与窗口同源——以此根除「越拖越大」。
//   - 窗口常驻但隐藏；仅 Present 非隐藏帧时淡入显示，隐藏时无动画定时器、零渲染。

const (
	floaterClassName = "MnemoTransferFloater"

	flLogiW    = 176 // 逻辑尺寸 @96DPI（紧凑卡片）
	flLogiH    = 52
	flCardR    = 7
	flLogoSize = 36
	flLogoR    = 5
	flTextX    = 52
	flFontRow  = 12 // 速度行字号（逻辑 px）
	flFontBig  = 13 // 完成/出错态字号

	wmFloaterUpdate = 0x0400 + 71 // WM_APP + 71

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
	flAnimDuration    = 220.0 // ms
	flSlidePx         = 6.0
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

// flCopyMemory 从系统提供的指针拷贝定长数据（WM_DPICHANGED 的建议矩形）。
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

// flBitmapInfo：头部 + 4 个位掩码（BI_BITFIELDS 含 alpha 通道）。
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

var (
	flColCard   = flRGB{23, 25, 33}    // 卡底（alpha 235/255）
	flColBorder = flRGB{255, 255, 255} // 描边（alpha ≤ 18/255）
	flColText   = flRGB{232, 234, 242} // 主文字
	flColWeak   = flRGB{139, 144, 163} // 弱化（无速度）
	flColDown   = flRGB{167, 139, 250} // 下载箭头（主题紫）
	flColUp     = flRGB{110, 231, 183} // 上传箭头
	flColDone   = flRGB{52, 211, 153}  // 完成
	flColError  = flRGB{248, 113, 113} // 出错
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

	hwnd atomic.Uintptr
	done atomic.Bool

	ui *flUI // 仅 UI 线程访问
}

var floaterWins sync.Map // hwnd → *winFloater

func newWinFloater(logo []byte, hooks floaterHooks, initial floaterFrame, suppressed bool, x, y int, hasPos bool) floaterView {
	w := &winFloater{hooks: hooks, logo: logo, posX: x, posY: y, hasPos: hasPos, frame: initial, suppressed: suppressed}
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

func (w *winFloater) Close() {
	if w.done.Swap(true) {
		return
	}
	if hwnd := w.hwnd.Load(); hwnd != 0 {
		pPostMessageW.Call(hwnd, wmClose, 0, 0)
	}
}

// ---------- UI 线程 ----------

// flUI 持有全部 GDI 资源，仅在 UI 线程访问。
type flUI struct {
	hwnd  uintptr
	scale float64
	pxW   int32
	pxH   int32

	dib     uintptr // 画布 DIB section（BGRA 预乘，top-down）
	dibBits []byte
	memDC   uintptr
	oldBmp  uintptr

	stripDC   uintptr // 文字 mask strip（黑底白字，亮度作 alpha）
	stripBits []byte
	stripBmp  uintptr
	stripOld  uintptr
	stripW    int32
	stripH    int32
	brushBlk  uintptr
	fontRow   uintptr
	fontBig   uintptr

	logoTile []byte
	logoPx   int

	visible   bool
	alpha     float64
	animating bool
	animStart time.Time
	animFrom  float64
	animTo    float64
	osFull    bool // 前台独占全屏
	fullTmrOn bool

	dragging bool
	dragOff  flPoint
	moved    bool

	frame      floaterFrame
	suppressed bool
	baseX      int32
	baseY      int32
}

func (w *winFloater) run(started chan struct{}) {
	runtime.LockOSThread() // 与托盘相同：消息泵必须钉在固定 OS 线程

	ui := &flUI{frame: w.frame, suppressed: w.suppressed}
	if !ui.create(w) {
		logging.Warn("floater window creation failed; floater disabled")
		close(started)
		return
	}
	w.ui = ui
	w.hwnd.Store(ui.hwnd)
	close(started)

	var msg flMsg
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if r == 0 || int32(r) == -1 { // 0 = WM_QUIT
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
		return false
	}

	title, _ := windows.UTF16PtrFromString("Mnemo Floater")
	const (
		wsPopup        = 0x80000000
		wsExLayered    = 0x00080000
		wsExTopmost    = 0x00000008
		wsExToolWindow = 0x00000080
		wsExNoActivate = 0x08000000
	)
	hwnd, _, _ := pCreateWindowExW.Call(
		uintptr(wsExLayered|wsExTopmost|wsExToolWindow|wsExNoActivate),
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		uintptr(wsPopup),
		0, 0, 0, 0, // 尺寸/位置由 setScale/placeInitial 确定（尺寸唯一计算点）
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
	ui.placeInitial(w)
	return true
}

// setScale 按 DPI 重建尺寸相关资源（画布/文字 strip/字体/logo 瓦片）。
// 窗口尺寸唯一计算点：logical × scale。
func (ui *flUI) setScale(scale float64) {
	ui.scale = scale
	ui.pxW = int32(flLogiW*scale + 0.5)
	ui.pxH = int32(flLogiH*scale + 0.5)
	ui.allocCanvas()
	ui.allocStrip()
	ui.makeFonts()
	ui.makeLogoTile()
	pSetWindowPos.Call(ui.hwnd, 0, 0, 0, uintptr(ui.pxW), uintptr(ui.pxH),
		uintptr(0x0002|0x0004|0x0010)) // SWP_NOMOVE|SWP_NOZORDER|SWP_NOACTIVATE
}

func (ui *flUI) placeInitial(w *winFloater) {
	if w.hasPos {
		ui.baseX = int32(w.posX)
		ui.baseY = int32(w.posY)
	} else {
		// 默认：主显示器右下角，留 24 逻辑 px 边距
		cx, _, _ := pGetSystemMetrics.Call(0) // SM_CXSCREEN
		cy, _, _ := pGetSystemMetrics.Call(1) // SM_CYSCREEN
		m := int32(24*ui.scale + 0.5)
		ui.baseX = int32(cx) - ui.pxW - m
		ui.baseY = int32(cy) - ui.pxH - m
	}
	pSetWindowPos.Call(ui.hwnd, 0, uintptr(ui.baseX), uintptr(ui.baseY), 0, 0,
		uintptr(0x0001|0x0004|0x0010)) // SWP_NOSIZE|SWP_NOZORDER|SWP_NOACTIVATE
}

func flBitmapInfoFor(w, h int32) flBitmapInfo {
	bi := flBitmapInfo{}
	bi.Header.Size = 40
	bi.Header.Width = w
	bi.Header.Height = -h // top-down
	bi.Header.Planes = 1
	bi.Header.BitCount = 32
	bi.Header.Compression = 3 // BI_BITFIELDS
	bi.Masks = [4]uint32{0x00FF0000, 0x0000FF00, 0x000000FF, 0xFF000000}
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
	ui.stripW = int32(float64(flLogiW-flTextX-10)*ui.scale + 0.5)
	ui.stripH = int32(20*ui.scale + 0.5)
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

func (ui *flUI) makeFont(px int) uintptr {
	face, _ := windows.UTF16PtrFromString("Microsoft YaHei UI")
	h, _, _ := pCreateFontW.Call(
		uintptr(int32(-px)), 0, 0, 0,
		uintptr(400), // FW_NORMAL
		0, 0, 0,
		uintptr(1), // DEFAULT_CHARSET
		0, 0,
		uintptr(5), // ANTIALIASED_QUALITY：灰度，mask 用亮度作 alpha
		0,
		uintptr(unsafe.Pointer(face)),
	)
	return h
}

func (ui *flUI) makeFonts() {
	if ui.fontRow != 0 {
		pDeleteObject.Call(ui.fontRow)
	}
	if ui.fontBig != 0 {
		pDeleteObject.Call(ui.fontBig)
	}
	ui.fontRow = ui.makeFont(int(flFontRow*ui.scale + 0.5))
	ui.fontBig = ui.makeFont(int(flFontBig*ui.scale + 0.5))
}

// makeLogoTile 解码 PNG → 双线性缩放到 40s×40s → 圆角遮罩 → 预乘 BGRA。
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
	for y := 0; y < ui.logoPx; y++ {
		for x := 0; x < ui.logoPx; x++ {
			ma := rrectAlpha(float64(x)+0.5, float64(y)+0.5, float64(ui.logoPx), float64(ui.logoPx), r)
			if ma <= 0 {
				continue
			}
			fx := (float64(x)+0.5)*float64(sw)/float64(ui.logoPx) - 0.5
			fy := (float64(y)+0.5)*float64(sh)/float64(ui.logoPx) - 0.5
			r8, g8, b8, a8 := bilinearRGBA(src, b.Min.X, b.Min.Y, sw, sh, fx, fy)
			aa := uint32(float64(a8>>8)*ma + 0.5)
			o := (y*ui.logoPx + x) * 4
			tile[o+0] = uint8(uint32(b8>>8) * aa / 255)
			tile[o+1] = uint8(uint32(g8>>8) * aa / 255)
			tile[o+2] = uint8(uint32(r8>>8) * aa / 255)
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
	for _, h := range []uintptr{ui.brushBlk, ui.fontRow, ui.fontBig} {
		if h != 0 {
			pDeleteObject.Call(h)
		}
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
		ui.syncState(w)
		ui.applyVisibility()
		ui.render()
		return 0
	case wmTimer:
		ui.onTimer(wParam)
		return 0
	case wmLButtonDown:
		var pt flPoint
		pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
		var rc flRect
		pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
		ui.dragOff = flPoint{X: pt.X - rc.Left, Y: pt.Y - rc.Top}
		ui.dragging = true
		ui.moved = false
		pSetCapture.Call(hwnd)
		return 0
	case wmMouseMove:
		if ui.dragging {
			var pt flPoint
			pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
			nx, ny := pt.X-ui.dragOff.X, pt.Y-ui.dragOff.Y
			if abs32(nx-ui.baseX) > 4 || abs32(ny-ui.baseY) > 4 {
				ui.moved = true
			}
			ui.baseX, ui.baseY = nx, ny
			// 只移动，不改尺寸——「越拖越大」的硬性防线
			pSetWindowPos.Call(hwnd, 0, uintptr(nx), uintptr(ny), 0, 0,
				uintptr(0x0001|0x0004|0x0010)) // SWP_NOSIZE|SWP_NOZORDER|SWP_NOACTIVATE
		}
		return 0
	case wmLButtonUp:
		if ui.dragging {
			ui.dragging = false
			pReleaseCapture.Call()
			if ui.moved {
				x, y := int(ui.baseX), int(ui.baseY)
				if w.hooks.onMove != nil {
					go w.hooks.onMove(x, y)
				}
			} else if w.hooks.onOpen != nil {
				go w.hooks.onOpen()
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
			// WM_DPICHANGED 的 lParam 是系统给出的建议 RECT 指针
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

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

// ---------- 状态同步 / 可见性 / 动画 ----------

func (ui *flUI) syncState(w *winFloater) {
	w.mu.Lock()
	ui.frame = w.frame
	ui.suppressed = w.suppressed
	w.mu.Unlock()
}

func (ui *flUI) wantVisible() bool {
	return ui.frame.Phase != floaterHidden && !ui.suppressed && !ui.osFull
}

func (ui *flUI) applyVisibility() {
	want := ui.wantVisible()
	if want && !ui.visible && !ui.animating {
		ui.visible = true
		pShowWindow.Call(ui.hwnd, 8) // SW_SHOWNOACTIVATE
		ui.startAnim(1)
		ui.setFullscreenTimer(true)
	} else if !want && ui.visible && !ui.animating {
		ui.startAnim(0)
	} else if ui.animating {
		// 动画中目标翻转
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
		ease := 1 - (1-t)*(1-t)*(1-t) // ease-out cubic
		ui.alpha = ui.animFrom + (ui.animTo-ui.animFrom)*ease
		if !ui.animating && ui.alpha <= 0 {
			ui.visible = false
			pShowWindow.Call(ui.hwnd, 0) // SW_HIDE
			ui.setFullscreenTimer(false)
		}
		ui.render()
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

// foregroundFullscreen 判定前台是否存在全屏窗口：
// 1) 独占全屏（D3D）：SHQueryUserNotificationState == QUNS_RUNNING_D3D_FULL_SCREEN
// 2) 无边框全屏：前台窗口覆盖其所在显示器的全部区域且无标题栏
// 任一成立即隐藏悬浮球，退出全屏后自动恢复。
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
		return false // 带标题栏的窗口不算全屏
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
	pSetForegroundWin.Call(ui.hwnd) // TrackPopupMenu 前台焦点惯例
	const tpmReturnCmd = 0x0100
	const tpmNonNotify = 0x0080
	cmd, _, _ := pTrackPopupMenuEx.Call(menu, uintptr(tpmReturnCmd|tpmNonNotify), uintptr(pt.X), uintptr(pt.Y), ui.hwnd, 0)
	pPostMessageW.Call(ui.hwnd, 0, 0, 0) // WM_NULL：菜单退出惯例
	switch cmd {
	case 1:
		if w.hooks.onOpen != nil {
			go w.hooks.onOpen()
		}
	case 2:
		if w.hooks.onHide != nil {
			go w.hooks.onHide()
		}
	}
}

// ---------- 渲染 ----------

func (ui *flUI) render() {
	if ui.dib == 0 {
		return
	}
	clear(ui.dibBits)
	ga := ui.alpha
	if ga <= 0 {
		ui.present()
		return
	}

	W, H := float64(ui.pxW), float64(ui.pxH)
	r := float64(flCardR) * ui.scale

	// 卡片 + 描边
	for y := 0; y < int(ui.pxH); y++ {
		for x := 0; x < int(ui.pxW); x++ {
			d := rrectSDF(float64(x)+0.5, float64(y)+0.5, W, H, r)
			a := clamp01(0.5 - d)
			if a <= 0 {
				continue
			}
			o := (y*int(ui.pxW) + x) * 4
			ui.blendOver(ui.dibBits[o:], flColCard, a*(235.0/255.0)*ga)
			// 1px 内沿描边
			ui.blendOver(ui.dibBits[o:], flColBorder, a*clamp01(d+1)*(18.0/255.0)*ga)
		}
	}

	// logo
	ls := ui.logoPx
	lx := int(8*ui.scale + 0.5)
	ly := int(8*ui.scale + 0.5)
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

	// 文字
	tx := float64(flTextX) * ui.scale
	switch ui.frame.Phase {
	case floaterActive:
		rowH := 20 * ui.scale
		ui.drawSpeedRow(int(tx), int(7*ui.scale+0.5), "↓", flColDown, ui.frame.Down, rowH, ga)
		ui.drawSpeedRow(int(tx), int(27*ui.scale+0.5), "↑", flColUp, ui.frame.Up, rowH, ga)
	case floaterDone:
		ui.drawStatusLine(int(tx), int(16*ui.scale+0.5), "下载完成", flColDone, ga)
	case floaterError:
		ui.drawStatusLine(int(tx), int(16*ui.scale+0.5), "下载出错", flColError, ga)
	}

	ui.present()
}

func (ui *flUI) drawSpeedRow(x, y int, arrow string, arrowCol flRGB, speed int64, rowH float64, ga float64) {
	aw := ui.drawText(x, y, arrow, ui.fontRow, arrowCol, ga)
	spCol := flColText
	if speed <= 0 {
		spCol = flColWeak
	}
	ui.drawText(x+aw+int(4*ui.scale+0.5), y, model.FormatSpeed(speed), ui.fontRow, spCol, ga)
}

func (ui *flUI) drawStatusLine(x, y int, text string, col flRGB, ga float64) {
	ui.drawText(x, y, text, ui.fontBig, col, ga)
}

// drawText 用 GDI 把文字画进黑底 strip，亮度作 alpha 合成到画布；返回文字宽度（px）。
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
	pDrawTextW.Call(ui.stripDC, uintptr(unsafe.Pointer(tp)), ^uintptr(0), // -1 = NUL 结尾
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
			ma := uint32(ui.stripBits[(y*int(ui.stripW)+x)*4]) // 灰度：取 B 通道
			if ma == 0 {
				continue
			}
			o := (dyY*int(ui.pxW) + dxX) * 4
			ui.blendOver(ui.dibBits[o:], col, float64(ma)/255.0*ga)
		}
	}
	return int(sz.Cx)
}

// present 把画布推给 layered window。
func (ui *flUI) present() {
	if ui.dib == 0 {
		return
	}
	screen, _, _ := pGetDC.Call(0)
	defer pReleaseDC.Call(0, screen)
	size := flSize{Cx: ui.pxW, Cy: ui.pxH}
	slide := int32(0)
	if ui.alpha < 1 {
		slide = int32((1 - ui.alpha) * flSlidePx * ui.scale)
	}
	ptDst := flPoint{X: ui.baseX, Y: ui.baseY + slide}
	ptSrc := flPoint{0, 0}
	blend := [4]byte{0, 0, 255, 1} // AC_SRC_OVER, 0, 255, AC_SRC_ALPHA
	pUpdateLayeredWin.Call(ui.hwnd, screen, uintptr(unsafe.Pointer(&ptDst)), uintptr(unsafe.Pointer(&size)),
		ui.memDC, uintptr(unsafe.Pointer(&ptSrc)), 0, uintptr(unsafe.Pointer(&blend)), 2) // ULW_ALPHA
}

// blendOver：在预乘 BGRA 像素上以 a（0..1）叠一个纯色。
func (ui *flUI) blendOver(dst []byte, c flRGB, a float64) {
	if a <= 0 {
		return
	}
	if a > 1 {
		a = 1
	}
	ca := uint32(a*255 + 0.5)
	inv := 255 - ca
	dst[0] = uint8((c.B*ca + uint32(dst[0])*inv) / 255)
	dst[1] = uint8((c.G*ca + uint32(dst[1])*inv) / 255)
	dst[2] = uint8((c.R*ca + uint32(dst[2])*inv) / 255)
	dst[3] = uint8((ca*255 + uint32(dst[3])*inv) / 255)
}

// blendOverPremul：叠一个已预乘的 BGRA 像素，附加全局透明度 ga。
func (ui *flUI) blendOverPremul(dst, src []byte, ga float64) {
	g := uint32(ga*255 + 0.5)
	sb := uint32(src[0]) * g / 255
	sg := uint32(src[1]) * g / 255
	sr := uint32(src[2]) * g / 255
	sa := uint32(src[3]) * g / 255
	inv := 255 - sa
	dst[0] = uint8(sb + uint32(dst[0])*inv/255)
	dst[1] = uint8(sg + uint32(dst[1])*inv/255)
	dst[2] = uint8(sr + uint32(dst[2])*inv/255)
	dst[3] = uint8(sa + uint32(dst[3])*inv/255)
}

// ---------- 几何 ----------

// rrectSDF 圆角矩形有向距离（像素中心坐标，<0 在内部）。
func rrectSDF(px, py, w, h, r float64) float64 {
	cx, cy := w/2, h/2
	qx := abs64(px-cx) - (cx - r)
	qy := abs64(py-cy) - (cy - r)
	ax, ay := qx, qy
	if ax < 0 {
		ax = 0
	}
	if ay < 0 {
		ay = 0
	}
	outside := sqrt64(ax*ax + ay*ay)
	inside := qx
	if qy > inside {
		inside = qy
	}
	if inside > 0 {
		inside = 0
	}
	return outside + inside - r
}

func rrectAlpha(px, py, w, h, r float64) float64 {
	return clamp01(0.5 - rrectSDF(px, py, w, h, r))
}

func abs64(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func sqrt64(v float64) float64 {
	// 避免 math 包导入链差异；牛顿迭代足够精度
	if v <= 0 {
		return 0
	}
	x := v
	for i := 0; i < 8; i++ {
		x = (x + v/x) / 2
	}
	return x
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

// bilinearRGBA 对源图做双线性采样（返回 16bit 通道值）。
func bilinearRGBA(src image.Image, ox, oy, sw, sh int, fx, fy float64) (r, g, b, a uint32) {
	x0 := int(fx)
	y0 := int(fy)
	tx := fx - float64(x0)
	ty := fy - float64(y0)
	sample := func(x, y int) (uint32, uint32, uint32, uint32) {
		if x < 0 {
			x = 0
		}
		if y < 0 {
			y = 0
		}
		if x >= sw {
			x = sw - 1
		}
		if y >= sh {
			y = sh - 1
		}
		return src.At(ox+x, oy+y).RGBA()
	}
	r00, g00, b00, a00 := sample(x0, y0)
	r10, g10, b10, a10 := sample(x0+1, y0)
	r01, g01, b01, a01 := sample(x0, y0+1)
	r11, g11, b11, a11 := sample(x0+1, y0+1)
	lerp := func(c00, c10, c01, c11 uint32) uint32 {
		top := float64(c00)*(1-tx) + float64(c10)*tx
		bot := float64(c01)*(1-tx) + float64(c11)*tx
		return uint32(top*(1-ty) + bot*ty)
	}
	return lerp(r00, r10, r01, r11), lerp(g00, g10, g01, g11), lerp(b00, b10, b01, b11), lerp(a00, a10, a01, a11)
}

// ---------- 平台接口入口 ----------

func (f *floater) newView() floaterView {
	x, y, hasPos := f.savedPosition()
	return newWinFloater(f.logo, f.hooks(), f.initialFrame(), f.suppressed(), x, y, hasPos)
}
