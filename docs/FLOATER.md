# 传输悬浮窗（Floater）需求与设计

> v1 · 2026-08-19 · 状态：已实现（Windows）

## 1. 需求

桌面级传输悬浮窗：一个**独立于主窗口的 OS 原生置顶小窗**，实时显示当前下载/上传聚合速度，可拖拽到屏幕任意位置。

### 1.1 外观

- 圆角矩形深色半透明卡片，**圆角克制**：卡片半径 8px、logo 半径 6px（逻辑像素 @96DPI，随显示器 DPI 缩放）
- 左侧：项目 logo（圆角矩形裁切），40×40
- 右侧两行速度：`↓ 12.3 MB/s` / `↑ 1.2 MB/s`，单位 B/s → KB/s → MB/s → GB/s 自适应
- 无速度活动的行弱化显示（灰色）；两项均无速度时卡片保持骨架（logo + 0 B/s）

### 1.2 可见性

| 条件 | 行为 |
|---|---|
| 无活动下载/上传任务 | 隐藏（且**不创建窗口**，零渲染开销） |
| 有活动任务 | 淡入显示，事件驱动刷新速度 |
| 前台存在独占全屏应用（`SHQueryUserNotificationState`） | 临时隐藏，退出全屏后恢复 |
| 应用内播放器全屏（前端 fullscreenchange 事件上报） | 临时隐藏，退出后恢复 |
| 下载任务全部完成 | 绿色「下载完成」态，停留 3s 后淡出隐藏 |
| 任务出错 | 红色「下载出错」态，停留 6s 或点击后淡出 |
| 主窗口关闭到托盘 | **保持显示**（这正是悬浮窗的意义） |
| 应用真正退出 | 销毁 |

### 1.3 交互

- **拖拽**：按住任意位置移动；结束后位置写入设置（下次启动原位恢复）
- **左键单击**：唤起主窗口并切换到「传输」页
- **右键菜单**：显示主窗口 / 隐藏悬浮窗（写入设置，设置页可重新开启）
- 窗口永不可聚焦（`WS_EX_NOACTIVATE`），不抢输入焦点、不出现在任务栏/Alt-Tab

### 1.4 性能约束

- 无任务时窗口与 GDI 资源**全部不创建**（懒初始化）；进入 idle 后隐藏并停掉所有定时器
- 渲染**事件驱动**：速度按 transfer 事件聚合，节流 4Hz 推送；无自由轮询
- 动画期才开 16ms 定时器（淡入/淡出/完成态），结束即停
- 内存占用目标 < 5MB（一张 32bpp DIB + logo 位图）；**不使用第二个 WebView**（单 WebView2 实例 ~100MB+，不可接受）

### 1.5 已踩过的坑（必须规避）

- **拖动时窗口越来越大**：上一轮实现的 bug。根因是拖拽循环里反复重设/累加窗口尺寸（含 DPI 换算漂移）。本版约束：
  - 拖拽只调 `SetWindowPos(..., SWP_NOSIZE | SWP_NOZORDER | SWP_NOACTIVATE)`，任何路径都不改尺寸
  - 尺寸只有唯一计算点：`logicalSize × dpiScale`，仅在窗口创建与 `WM_DPICHANGED` 时重算，位图与窗口尺寸同源
  - `WM_DPICHANGED` 只采纳系统建议矩形的**位置**，尺寸自行重算（不采纳建议尺寸，避免累积误差）
- Windows 上窗口消息循环与托盘一样必须 `runtime.LockOSThread()`，否则消息泵静默失效

## 2. 技术方案

### 2.1 窗口

Win32 layered window（per-pixel alpha），专属 UI 线程：

```
WS_EX_LAYERED | WS_EX_TOPMOST | WS_EX_TOOLWINDOW | WS_EX_NOACTIVATE
```

- 32bpp DIB section 作为画布，`UpdateLayeredWindow` 整体呈现（AC_SRC_ALPHA，预乘 alpha）
- 跨线程通信：聚合器 → `PostMessage(WM_APP_UPDATE)`，UI 线程持锁读状态重绘；不设反向调用

### 2.2 绘制（混合策略）

1. Go 侧用标准库图像操作生成卡底：圆角矩形（径向 alpha）、logo 解码 PNG 缩放 + 超椭圆裁切、上下行箭头（小多边形）
2. 文字（数字/单位/完成态文案）用 GDI 渲染到独立灰度 mask（黑底白字，`ANTIALIASED_QUALITY`），亮度作为 alpha 在 Go 侧合成进主 buffer——绕开 GDI 直写 32bpp DIB 不产生 alpha 的问题
3. 字体 `Microsoft YaHei UI`（中西文一致），字号 11/10.5px 逻辑
4. 配色沿用设计 token：卡底 `rgba(24,26,34,0.92)` + 1px `rgba(255,255,255,0.08)` 描边；文字主 `#e8eaf2`、弱 `#8b90a3`；下载箭头主题紫 `#a78bfa`、上传 `#6ee7b7`；完成 `#34d399`；出错 `#f87171`

### 2.3 状态机（`floater.go`，平台无关）

```
hidden ──有活动任务──▶ active ──全部完成──▶ done ──3s──▶ hidden(淡出)
  ▲                     │└──任一失败──▶ error ──6s/点击──▶ hidden(淡出)
  └─────暂时隐藏/设置关闭──┘
```

- 聚合器：`map[taskID]speed` 分下载/上传两桶，每次 transfer 事件更新求和；活动任务数降为 0 时按最后任务结局进 done/error
- 抑制源（任一成立即临时隐藏，不清状态）：设置关闭 / 独占全屏 / 播放器全屏

### 2.4 动画

- 淡入/淡出：alpha 0→1（或反向），ease-out cubic，220ms，伴随 6px 上浮位移
- 速度文本变化：直接重绘（4Hz 已足够平滑）
- done/error 态：卡片主题色描边 2px 呼吸一次（0.35→0 透明度衰减，600ms），文字切换

### 2.5 设置

`Settings.Floater *bool`（nil=默认开）、`FloaterX/FloaterY int` + `FloaterPos bool`（是否拖拽过）。设置页「基础」组「传输悬浮球」开关，保存即生效。

### 2.6 平台

- Windows：完整实现（`floater_windows.go`）
- Linux/macOS：`floater_other.go` 空实现（后续可用 GTK/NSPanel 补）

## 3. 实现清单

- `internal/app/floater.go`：控制器（速度聚合、hidden→active→done/error 状态机、4Hz 节流、设置接线）
- `internal/app/floater_windows.go`：Win32 layered window（懒创建、SDF 圆角渲染、GDI 文字 mask、淡入淡出动画、拖拽、右键菜单、全屏抑制）
- 前端：设置页「传输悬浮球」开关；播放器全屏事件上报；点击悬浮球跳转传输页；旧页面内悬浮球已移除
- 全屏检测双重保险：`SHQueryUserNotificationState`（独占全屏）+ 前台窗口覆盖整屏且无标题栏（无边框全屏，含本应用播放器全屏）
