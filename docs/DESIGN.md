# Mnemo-Go 前端设计规范

Mnemo-Go 的设计语言：**轻盈、流动、内容至上**。界面是一块安静的画布，文件与任务是唯一的主角；所有交互反馈通过柔和的色块流动与弹性动效传达，不使用生硬的线条、厚重的边框或装饰性元素。

## 设计原则

1. **内容为中心**：页面服务于文件、任务与设置项，不为装饰增加多余元素。
2. **零说明文案**：界面不放置介绍/引导/提示条；无法避免的说明必须极简（一行以内）。
3. **语义令牌唯一**：所有颜色、圆角、阴影、间距、动效一律引用 `design-tokens.css` 的语义变量，组件不硬编码任何颜色值或近似数值。
4. **层级用"轻"表达**：层级只由文字权重、间距、细分隔线（`--border-lighter`）与定位表达；不堆叠卡片、不加粗描边。
5. **一处一样**：同类控件在全局只有一套实现（见「控件规格」），页面 `<style scoped>` 只解决内容特有的排列。

## 图标与 Emoji 禁令

- **所有界面禁止使用 Emoji**（含带 `FE0F` 变体选择符的符号）。按钮、菜单、空状态、文件类型、状态标识一律不得使用 Emoji 字符。
- 功能图标统一使用 `frontend/src/components/UiIcon.vue`（内联 SVG、stroke 风格、`currentColor` 继承文字色）。新增图标先在 `UiIcon.vue` 的 `PATHS` 中登记命名，再以 `<UiIcon name="…" />` 引用。
- 允许少量纯排版符号：箭头 `↑ ↓`、勾号 `✓`、星级 `★`；其余图形一律走 SVG。
- 网盘 provider 图标使用 `frontend/src/assets/drive-icons/` 资源，经 `api.js` 的 `providerIconUrl()` 解析。

## 选中态与强调：流动色块（Selection Language）

**选中/激活的视觉表达只有一种：浅色块 + 主色文字。禁止使用任何形式的小线条指示器**（底部 2px 条、左侧 3px 条、右侧竖条等均不允许）。

- 色块底色：`color-mix(in srgb, var(--color-primary) 11-13%, transparent)`，圆角 `--radius-md`，
  可叠 1px 内描边 `color-mix(in srgb, var(--color-primary) 18-22%, transparent)`。
- **可移动的选中容器**（顶栏页签、分段控件）使用**滑动 glider**：一个绝对定位的圆角色块，跟随激活项改变 `transform: translateX()` 与 `width`，缓动用带轻微回弹的 `cubic-bezier(0.22, 1.25, 0.36, 1)`，时长 320ms——切换时色块"灵动地滑动过去"。
- **列表项选中**（菜单、树节点、下拉项）使用静态色块 + 入场微动效 `sel-in`（scale .97 → 1，spring 缓动）。
- 悬停态一律 `--bg-hover` 圆角底面，不位移、不加阴影（文件/任务行除外，见下）。

## 动效标尺

| 令牌 | 值 | 用途 |
| --- | --- | --- |
| `--motion-fast` | 140ms | 颜色、底色、透明度 |
| `--motion-normal` | 240ms | 位移、展开收起 |
| `--motion-slow` | 380ms | 弹层、页面级过渡 |
| `--motion-ease` | cubic-bezier(0.22, 1, 0.36, 1) | 通用 |
| `--motion-ease-out` | cubic-bezier(0.16, 1, 0.3, 1) | 入场 |
| `--motion-spring` | cubic-bezier(0.34, 1.3, 0.64, 1) | 按压回弹、选中入场 |
| glider 缓动 | cubic-bezier(0.22, 1.25, 0.36, 1) | 滑动色块（带回弹） |

规则：悬停不加位移（`translateY`）也不加阴影——只有可点击的卡片/按钮允许 `scale(.96-.98)` 按压反馈。页面切换用左右滑动（`page-slide-left/right`，12px 位移 + 淡入）。弹层入场统一 `ctx-in`（.12s）。尊重 `prefers-reduced-motion`。

## 排版与间距

- 基准字号 `14px`（body），界面正文 `13px`，辅助 `12px`，弱化说明 `11-11.5px`。
- 列表行高：文件行 `46px`（fileitem）、任务行 `52px`（taskrow）、树/菜单节点 `28-32px`。
- 圆角阶梯：`--radius-xs 4` / `sm 6` / `md 8` / `lg 10` / `xl 12` / `full 999`。
- 间距只取 `--space-1..6`（4/8/12/16/20/24）。

## 颜色

只使用语义变量：`--bg-base/surface/hover/subtle/elevated`、`--text-primary/secondary/tertiary/disabled`、`--border-light/lighter`、`--color-primary(+hover/active)`、`--color-success/warning/error`、`--listselectbg`、`--ring-focus`。暗色主题下同一套变量自动切换（`html.dark`）。强调色为固定品牌紫：浅色 `#7c3aed` / 深色 `#a78bfa`，不提供色包切换；压白字的实心按钮用 `--color-primary-strong`（#7c3aed，过 WCAG AA）。

## 控件规格

| 控件 | 实现 | 规格 |
| --- | --- | --- |
| 按钮 | `.btn` / `.primary` / `.danger` / `.text` / `.sm` | 高 30px（sm 26px），圆角 `--radius-sm`，按压 scale(.97) |
| 幽灵按钮 | `.tbtn`（工具条内） | 无边框透明底，悬停出 `--bg-hover` 底面 |
| 圆形钮 | `.btn-circle` | 26px 圆钮，用于行内操作/选择，按压 scale(.9) |
| 输入框 | `.input` / `.textarea` | 高 30px，聚焦 `--ring-focus` 光环 |
| 下拉选择 | `UiSelect.vue` | **禁止原生 `<select>`**；按钮 + 浮层列表，选中项带勾，支持图标 |
| 开关 | `.switch` | 38×21 滑块，滑块位移用 `transform`（不用 left），按压滑块拉长 |
| 分段切换 | `SegTabs.vue` | 滑块 glider 式（`--seg-index` 驱动 translateX） |
| 徽标 | `.badge` | `.primary/.success/.warn/.error` |
| 模态框 | `Modal.vue` | `.modal-mask` 遮罩 + `.modal`，Esc 关闭 |
| 菜单 | `ContextMenu.vue` | `.ctx-menu` 浮层，`.ctx-item` 行，danger 红色 |
| 空状态 | `.workspace-empty-state` | 居中图标 + 标题 +（可选）一行描述 |
| 搜索框 | `.search-quick` | 填充式圆角，聚焦变宽 168→200px |
| Toast | `.toast` | 右上角浮层，左缘 3px 状态色，3.2s 自动消失 |

## 页面结构

### 应用壳（App.vue）
- 顶栏 `topbar` 高 **42px**，整块窗口拖动区（`--wails-draggable: drag`），控件 `no-drag`。
- 左侧页签组 `top-tabs`：pill 形按钮 + `.top-tab-glider` 滑动色块（JS 测量激活项 offsetLeft/offsetWidth，resize 时重算）；右侧为明暗切换与设置入口图标钮。
- 页面容器 `page-host`，切换用左右滑动过渡（按页签索引定方向）。
- 传输中右下角 `.transfer-ball` 悬浮速度球（可拖动固定尺寸，点击跳传输页）。

### 网盘页（PanView）
- 最左 `account-rail` 账号栏：**60px 窄图标栏，悬停 300ms 展开为 220px**（图标 36px 盒 + 名称/用量条渐显）；底部添加账号按钮；右键账号出菜单。
- 内部左侧 `pan-left`（216px）：收藏（可展开内联列表）+ 回收站 + 目录树（`TreeNode.vue` 递归懒加载，任意层级，悬停预览浮层）。
- 右侧 `pan-right`：`toppanbtns` 幽灵工具条 → `toppanarea` 信息条（全选/计数/排序列头/视图切换，高 40px）→ `file-list`。
- 文件行 `fileitem`：46px flex 行，文件名两行 clamp 悬停主色；网格视图 `griditem` 76px 图标底面。
- 搜索关键词用 `<mark class="hl">` 高亮，Esc 返回目录。

### 传输页（TransferView）
- 左侧 `down-side` 菜单（选中为流动色块）+ 账号筛选；右侧六分区列表。
- 任务行 `taskrow`：52px 平铺行——选择圆钮 34px / 图标 26px / 名称+副行 / 大小 76px / 进度 200px（状态文字+5px 细条）/ 速度 86px / 操作钮。
- 进度条 `progress-total` + `progress-current`（`.active/.succeed/.error` 三色）。

### 分享页（ShareView）
- `share-page` 单滚动容器：`share-toolbar`（搜索 + UiSelect 筛选 + 刷新）→ `share-group` 分组（网盘图标+账号+条数）→ `share-record` 记录行（名称/时间/链接 + 提取码 chip + 操作钮）。

### 同步页（SyncView）
- `syncpage` 容器：`syncpage-head` 标题栏 → `sync-task` 任务卡（名称+方向徽标、本地⇄网盘路径行、运行进度条、switch+操作钮组）。

### 设置页（SettingsView）
- 左 `settings-nav` 图标导航（选中为流动色块，滚动 spy）+ 右 `settings-body` 长页。
- 设置项 `sg-row` 平面行：132-168px 加粗标签列 + 控件区。
- 改动即时静默保存，底部保留「立即保存」。

### 视频播放（PlayerPanel.vue）
- 视频统一由网页播放器沉浸式覆盖层播放；原生 HTML5/WebView 解码 MP4、WebM、Ogg，HLS 使用 HLS.js，DASH 使用 dash.js。上游签名 URL 和鉴权头只保留在 Go 侧播放会话中。
- 控制层采用顶部文件信息栏 + 底部悬浮控制区：进度条、播放/暂停、快退/快进、时间、音量、清晰度、字幕、倍速、循环、画中画、截图与全屏均使用线条图标；窄屏将次要控件收进“更多”。
- 起播时按 `GetPlayCursor` 续播，播放中定时保存并在关闭/结束时调用 `SavePlayCursor`；短期签名地址失效时由会话代理刷新一次并保留 Range。
- MKV/AVI/WMV/FLV/RMVB 等浏览器无法稳定解码的容器明确提示下载，除非 provider 提供 HLS/DASH 转码源。

## 文件清单

| 文件 | 职责 |
| --- | --- |
| `frontend/src/styles/design-tokens.css` | 颜色/文字/边框/阴影/间距/动效语义变量（唯一事实源） |
| `frontend/src/styles/main.css` | 应用壳 + 全部共用控件与页面布局规则 |
| 页面 `<style scoped>` | 该页面独有的排布细节 |
