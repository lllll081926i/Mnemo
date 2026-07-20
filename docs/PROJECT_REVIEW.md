# Mnemo 项目审查报告

> 审查日期：2026-07-20  
> 审查范围：产品进度、网盘能力、工程状态；环境依赖清理；Electron 内存主因  
> 基线：分支 `main`（相对 `origin/main` 当时 ahead 13）、`package.json` 版本 `0.1.1`、`localVersion` `5.0.7`

本文档合并两轮审查结论，供后续排期与首发前加固使用。实现细节以仓库代码为准；文中完成度为审查时点的主观估计。

---

## 目录

1. [产品与阶段](#1-产品与阶段)
2. [近期节奏](#2-近期节奏)
3. [网盘能力矩阵](#3-网盘能力矩阵)
4. [产品面进度](#4-产品面进度)
5. [工程与质量](#5-工程与质量)
6. [完成度总览](#6-完成度总览)
7. [建议优先级（功能/首发）](#7-建议优先级功能首发)
8. [环境依赖审查](#8-环境依赖审查)
9. [Electron 内存占用主因](#9-electron-内存占用主因)
10. [依赖与内存整改清单](#10-依赖与内存整改清单)
11. [一句话结论](#11-一句话结论)

---

## 1. 产品与阶段

**Mnemo** — 免费多网盘桌面文件管理器（Electron + Vue 3）。  
命名取自 Mnemosyne（记忆女神）：统一管理散落在各网盘的文件。

| 保留 | 已移出主产品线（见 `AGENTS.md`） |
|------|----------------------------------|
| 多网盘文件管理、Aria HTTP 下载、云离线、预览/播放、轻量分享 | 媒体库、媒体服务器、音乐库、图书/AI、本地 BT、WebDAV Server、付费/Pro、RSS 工具箱 |

**技术栈**：Electron 40 · Vue 3 · Vite 7 · Pinia · TypeScript · npm · Node ≥ 22.12

**体量（审查时点）**：约 **6.9 万行** TS/Vue（不含测试）

**阶段判断**：从旧多功能客户端收敛为「多网盘文件管理」后，已完成首发资料清理 + 多网盘 OAuth/能力路由扩展；处于 **可开发/可打包的 alpha → 首发前加固** 阶段。

文档体系：`README.md` / `README.en.md` / `AGENTS.md` / `DESIGN.md` / `CLAUDE.md` / `CONTRIBUTION.md`。

---

## 2. 近期节奏

约 **2026-07-19～07-20** 连续密集提交，主线清晰：

### 7/19

- 品牌与仓库元数据重置（Mnemo）
- 砍功能 / 收敛依赖 / 清理无入口能力
- 播放器精简、启动与延迟加载
- Windows 发布配置、确认式自动更新

### 7/20（当时未推送约 13 个 commit）

- 移除不再支持的网盘提供方
- **应用内多账号 OAuth 基础设施**
- 聚合登录、侧边栏与文件能力路由
- 多盘目录 / 预览 / 分享 / 文件操作补全
- Google Drive 文件操作、OAuth 凭证、aria2 启动竞态、标题栏拖动等

贡献结构：历史主干 `gaozhangmin` + 当前维护者近两日主力推进。

---

## 3. 网盘能力矩阵

统一入口：`src/utils/driveProvider.ts`（provider 元数据、能力位、侧边栏、userId/driveId 前缀）。

审查时登记 **13 个 provider**：

`aliyun` · `139` · `189` · `guangya` · `pikpak` · `quark` · `onedrive` · `dropbox` · `gdrive` · `nextcloud` · `gofile` · `webdav` · `s3` ·（及 `unknown`）

| 提供方 | 实现深度 | 能力位摘要 | 完成度观感 |
|--------|----------|------------|------------|
| **阿里云盘** | 最完整（约 7.9k LOC，模块最多） | 搜索 / 分享全链路 / 回收站 / 收藏 / 相册 / 加密 / 秒传等 | ★★★★★ 生产级 |
| **夸克** | auth / list / cmd / share / upload | 搜索 + 分享；`copy: false` | ★★★★☆ |
| **PikPak** | 完整 + offline | 分享 + 回收站视图 | ★★★★☆ |
| **光鸭** | 完整 + offline / search / share | 搜索 + 分享管理 | ★★★★☆ |
| **139 / 189** | 基础文件 + 上传 | 标准文件能力（扩展分享等较少） | ★★★☆☆ |
| **Dropbox** | 较完整（含 revisions / search / share / thumb） | 搜索 + 创建分享 | ★★★★☆ |
| **OneDrive** | 较完整（含 revisions / search / share） | 搜索 + 创建分享 | ★★★★☆ |
| **Google Drive** | 偏薄（约 368 LOC） | 搜索 + 分享 + 回收站位；share/upload 仍简 | ★★★☆☆ 骨架可跑 |
| **GoFile** | 很薄（约 231 LOC） | 创建分享位；share 偏直链 | ★★☆☆☆ |
| **Nextcloud** | 仅 auth（复用 WebDAV） | mountedStorage + direct 上传 | ★★★☆☆ |
| **WebDAV / S3** | `utils/*Client.ts` | mountedStorage + direct 上传 | ★★★☆☆ 基础可用 |

**结论**：

- 国内盘 + 阿里：主体已齐。
- 国际 OAuth 盘（OneDrive / Dropbox / Google Drive）：登录与路由已接，GDrive 最浅。
- 挂载存储（WebDAV / S3 / Nextcloud）：连接层有，与 pan UI 深度需联调。
- README 能力表与代码能力位大体一致；部分能力是「位已开、实现仍简」。

接入清单见 `AGENTS.md`（auth → list → download → search → ops → share → upload → menu → tests → build）。

---

## 4. 产品面进度

### 网盘 `pan/`

- 目录树 + 列表 + 批量操作 + 按能力裁剪菜单：主干成熟
- 侧边栏按 provider 能力生成（收藏 / 回收站 / 搜索 / 阿里多空间）
- 多账号切换与登录入口：近两日重点

### 传输 `down/` + Aria 主进程

- Aria2 多线程下载、任务分区、远程 Aria、限速等：产品声明齐全
- 近两日修复：aria2 启动竞态、Node 25 相关警告等

### 分享 `share/`

- 创建 / 导入 / 我的分享 / 历史：阿里系最全
- 其它盘按 `createShare` / `importShare` 等裁剪
- GDrive / GoFile 等仍偏「直链 / anyone 读」级

### 预览 / 播放 `layout/`

- 视频（Artplayer）、MPV、PDF / Office / 图片 / 代码等：在用
- **磁盘仍有残留**：`aisearch/`、`book-manager/`、`book-reader/`、`PageMusic.vue` 等
- 主进程仍有 `electron/main/reedy/`
- `src/rss/**` 整棵仍在（产品已声明移除入口）

### 设置 / 壳 / 更新

- 设计规范见 `DESIGN.md`（无卡片、语义 token、工作区壳）
- 确认式自动更新、Windows 发布、延迟加载首屏：已落地

### 登录 `user/UserLogin.vue`

- 多 provider 聚合登录 + 应用内 OAuth
- 审查时工作区有未提交改动：阿里 webview 监听清理、load 失败处理等（稳定性）

---

## 5. 工程与质量

| 项 | 状态 |
|----|------|
| 包管理 | 仅 npm / `package-lock.json` |
| 密钥 | `.env.local` → `scripts/generate-secrets.mjs` → `src/secrets.generated.ts`（gitignore） |
| 测试 | Vitest；`vitest.config.ts` 显式 include；多 provider 单测已挂 |
| 默认 `npm test` | 子集，非全量 E2E / 真实账号联调 |
| 格式 | single quotes、无分号、printWidth 260、LF |
| 发布 | `electron-builder`；`files: ["dist"]`；Windows 额外资源排除部分 mpv 路径 |

**缺口**：

- 无端到端真实账号冒烟表（各盘：登录 → 列表 → 传 → 预览 → 分享）
- GDrive / GoFile 测试深度低于 OneDrive / Dropbox
- 历史残骸增加阅读与误 import 风险
- `localVersion` 与 `package.json` 版本语义不一致，发布前需统一

---

## 6. 完成度总览

（主观估计，审查时点）

```
产品定位 / 文档          ████████████████████  ~95%
壳 / 设置 / 主题规范     ██████████████████░░  ~90%
阿里云盘全链路           ████████████████████  ~95%
国内其它盘               ███████████████░░░░░  ~75–85%
传输 (Aria)              ██████████████████░░  ~90%
预览播放                 █████████████████░░░  ~85%（残留未清）
国际盘 OD / Dropbox      ██████████████░░░░░░  ~70%
Google Drive             █████████░░░░░░░░░░░  ~45–55%
GoFile                   ██████░░░░░░░░░░░░░░  ~35%
WebDAV / S3 / Nextcloud  ████████████░░░░░░░░  ~60%
残骸清理                 ████░░░░░░░░░░░░░░░░  ~20%
发布 / 版本一致          ██████████████░░░░░░  ~70%
真实多盘联调验收         ████░░░░░░░░░░░░░░░░  ~20%
```

| 目标 | 就绪度 |
|------|--------|
| 功能可演示 | 约 **70–75%** |
| 对外稳定首发 | 约 **55–65%**（国际盘深度、残骸、联调、未推送大改） |

---

## 7. 建议优先级（功能/首发）

### P0 — 稳住当前大改

1. 收尾登录 webview / 窗口相关稳定性与测试
2. `npm run typecheck` + `npm test` + 至少 Windows `build:test`
3. 整理多 OAuth + 能力路由相关 commit 的 Release note 后再推送

### P1 — 按 AGENTS 清单补齐薄盘

4. **Google Drive**：断点上传、搜索与能力位对齐、分享/回收站、下载直链
5. **GoFile**：列表 / 权限边界、分享与下载一致性
6. **WebDAV / S3 / Nextcloud**：与 pan 菜单、上传队列、预览鉴权全路径验收

### P2 — 产品洁癖与发布

7. 删除或彻底隔离无入口代码：`src/rss/**`、`layout/book-*`、`aisearch`、`reedy`、无用入口
8. 统一版本号策略（`0.1.x` vs `localVersion`）
9. README 能力表与 `DriveProviderCapabilities` 对照，避免「位开了实现空」
10. 建立真实账号冒烟表

### P3 — 体验与质量

11. 扩展多账号边界测试（已有 `multiAccountBoundary` 等）
12. 登录页 / 新 provider 图标与侧栏视觉一致性
13. OAuth 失败提示用户可读性

**合理目标**：不宜继续盲目加盘；应 **收口登录与构建 → 按 checklist 做实已声明盘 → 清残骸 → 冒烟后打 0.1.x 预发布**。

---

## 8. 环境依赖审查

### 8.1 体量参考（审查时点开发机）

| 路径 | 约 |
|------|-----|
| `node_modules` | ~1.1 GB |
| `static/engine`（三端 aria2 / 相关资源） | ~55 MB |
| 其中 `dashjs` | ~125 MB |
| `ant-design-vue` | ~85 MB |
| `lucide-vue-next` | ~36 MB |
| `@arco-design` 相关 | ~35 MB |
| `pdfjs-dist` | ~37 MB |
| `hls.js` | ~25 MB |
| `@opendataloader/pdf` | ~24 MB |
| `electron`（开发依赖） | ~335 MB（正常，不进安装包逻辑同 builder 配置） |

安装包侧：`electron-builder.json` 的 `files: ["dist"]`，源码残骸默认 **不进安装包**，但污染开发与误用。

### 8.2 环境变量：可删除

产品已去掉 AI / 媒体库 / 订阅类能力，但模板仍残留：

| 变量 | 状态 |
|------|------|
| `TMDB_API_KEY` | 无业务读取（字幕 UI 仅文案可能提到 TMDB） |
| `BOXPLAYER_AI_API_URL` | 历史品牌残留 |
| `BOXPLAYER_SUPABASE_URL` | 同上 |
| `SUPABASE_PUBLISHABLE_KEY` | 同上 |
| `BOXPLAYER_SITE_URL` | 同上 |
| `MNEMO_AI_API_URL` | 仅在 `config.ts` 挂字段，**无业务调用** |

**建议删除**上述项（`.env.example` / `secrets.example.ts` / `generate-secrets.mjs` 等）。

**建议保留**：各网盘 OAuth 密钥、字幕相关 key、Apple 签名相关。

### 8.3 npm 包：确定可移除

| 包 | 结论 |
|----|------|
| **`markdown-it`**（及 `@types/markdown-it`） | 源码零引用；**2026-07-20 已卸载** |

### 8.4 npm 包：建议瘦身（仍被用，但不划算）

| 包 | 使用面 | 建议 |
|----|--------|------|
| **`dashjs`** | 仅 `PageVideo.vue` | 动态 `import()`；或确认无 DASH 后删除 |
| **`ant-design-vue`** | Tree / Tooltip / QRCode 等少量 | 迁 Arco 或轻量替代，去掉整库 |
| **`lucide-vue-next`** | 图标 | 按 icon 路径导入或 SVG sprite |
| **`@arco-design/web-vue` + 主题** | 全量 `app.use(ArcoVue)` + 整份 CSS | 按需组件 + 按需样式 |
| **`pdfjs-dist` / `hls.js` / `jassub`** | 预览 / 字幕 | 仅预览窗加载；控制 worker/locale 进主 chunk |
| **`@opendataloader/pdf`** | 主进程 Office→PDF | 确认链路；不用则删 |
| **`xlsx`** | `PageSheet` | 保留或换更轻预览解析 |
| **`lodash` 全量** | `xorWith` / `isEmpty` 等 | `lodash-es` 单函数或自写 util |
| **`@aws-sdk/*`** | S3 | 保持隔离，勿进主窗 eager graph |

### 8.5 代码残骸（非 npm，但污染依赖图）

| 路径 | 说明 |
|------|------|
| `src/rss/**` | 产品已移除入口 |
| `src/layout/aisearch`、`book-manager`、`book-reader` | 图书 / AI 遗留 |
| `electron/main/reedy/**` | 审查时未见引用 |
| `src/module/dlnacast`、`movie-db`、部分旧 webdav server / theme 等 | 部分已死 |
| `src/module/video-plugins/artplayer-plugin-libass` | 与 npm `artplayer-plugin-jassub` 双轨 |
| `static/crx` | 仅 dev 加载扩展 |
| `PageMusic` + `musicMetadata` | 音频预览窗仍可能打开；是否保留需产品决策 |

### 8.6 工程结构问题

- 大量运行时库写在 **`devDependencies`**，语义混乱。
- 主进程 Vite `external` 取 `package.json` 的 `dependencies`；若 `dependencies` 为空，external 列表为空，行为依赖打包结果。
- **双 UI 库**（Arco 全量 + Ant 局部）是依赖与 CSS 体积的第一结构性问题。

---

## 9. Electron 内存占用主因

### 9.1 空闲时进程结构

```
Electron Main
  ├─ Chromium 主进程 + IPC + session 改写
  ├─ MotrixApplication → aria2c 子进程（启动即起，常驻）
  ├─ Tray / 自动更新
  │
  ├─ Renderer #1  mainWindow
  │     同一入口：Vue + 全量 Arco + Pinia + PageMain 业务
  │
  ├─ Renderer #2  uploadWindow（10×10 隐藏）
  │     仍 load 完整 main 入口 + 全量 Arco/Vue
  │     实际只需 PageWorker 上传逻辑
  │
  └─ Renderer #3  downloadWindow（隐藏）
        同上

预览时 +N：WebOpenWindow 每次 new BrowserWindow
  → 再跑完整 Vue+Arco + 对应预览 SDK（hls/dash/jassub/pdf…）或外置 mpv
```

关键实现：

- `electron/main/core/window.ts`：`createUpload` / `createDownload` 使用与主窗相同的 `createElectronWindow(..., 'main', ...)`
- `src/main.ts`：`app.use(ArcoVue, {})` 全量注册
- `electron/main/launch.ts`：`disable-renderer-backgrounding`
- `createElectronWindow`：`backgroundThrottling: false`
- `ipcEvent`：`WebOpenWindow` → 新窗 + `setPage`

### 9.2 原因排序（对常驻占用）

| 优先级 | 原因 | 说明 |
|--------|------|------|
| **1** | **三窗复用完整渲染栈** | 上传/下载隐藏窗不需要完整 UI，却各开一个完整 Chromium renderer（常各 80–200MB+ 量级） |
| **2** | **全局禁止后台节流** | 隐藏窗与前台同级调度，不利于回收 |
| **3** | **webPreferences 偏重** | `nodeIntegration`、`webviewTag`、`sandbox: false`、`webSecurity: false`、`enableWebSQL` 等放大成本；预览窗同样配置 |
| **4** | **主窗启动即重业务** | 本地代理 server、Dexie 全量任务、定时器、拼音等；任务历史越长堆越大 |
| **5** | **预览 = 新窗 × 完整应用** | 多开预览线性涨；视频再叠 hls/dash/jassub 或 mpv 进程 |
| **6** | **启动即 aria2c** | 固定底座，合理但常驻 |
| **7** | **UI 依赖过重** | 全量 Arco CSS/组件 + 额外 Ant Design 局部 |
| **8** | **开发模式放大** | DevTools 自动打开、crx 扩展、Vite 未压缩 — 测内存勿当生产 |

**粗估空闲**：主进程 + 3×renderer + aria2 → 任务管理器 **400–800MB+** 常见；开发模式更高。  
**不是某一个库单独导致**，而是 **架构（多完整窗）× 禁止节流 × 重 UI 栈 × 预览复制窗**。

### 9.3 观测方式

```text
任务管理器 / Process Explorer：
  Mnemo.exe (main)
  Mnemo.exe (gpu)
  Mnemo.exe (renderer) × N   ← 空闲目标 N=1，而非 3
  aria2c.exe
```

---

## 10. 依赖与内存整改清单

### 内存 P0（结构，收益最大）

1. **Worker 轻入口**：upload/download 使用独立 `worker.html` + 无 Arco 的入口，或 UtilityProcess / 主进程队列
2. **恢复合理 throttling**：非传输关键窗允许 `backgroundThrottling: true`；全局 `disable-renderer-backgrounding` 改为可配置或移除
3. **预览窗减配**：无 webview、收敛 nodeIntegration、可考虑单例预览窗

### 依赖 P0

4. 删除过期 env + **`markdown-it`**
5. **`dashjs` 动态 import 或删除**
6. 规划去掉 **ant-design-vue**

### 内存 / 依赖 P1

7. Arco 按需注册，避免全量 `app.use(ArcoVue)`
8. 任务列表虚拟滚动 + 历史分页
9. aria2 **懒启动**（首次下载时再起）
10. 清理 `rss` / `reedy` / `book-*` / 死 `module/*`

### 验证目标

空闲 renderer **3 → 1** 后，常驻内存常见可降 **约 30–50%+**（视机器与任务量而定）。

---

## 11. 一句话结论

| 主题 | 结论 |
|------|------|
| **产品进度** | 已完成从旧「全家桶」到「纯多网盘文件管理」的收敛；国内主力盘 + 传输 + 预览壳成熟；国际盘骨架与 OAuth 已铺开，但 GDrive/GoFile 偏薄，残骸未清 |
| **首发** | 可演示约 70–75%；稳定首发约 55–65%。优先做实已声明盘与联调，而非继续加盘 |
| **环境依赖** | env 中 BoxPlayer/TMDB/Supabase/AI 与 **`markdown-it`** 可删；更大头是 **dashjs / 双 UI / lucide 全量** 需瘦身 |
| **内存主因** | **3 个 BrowserWindow 各跑完整 Vue+全量 Arco** + **禁止后台节流** + **启动即 aria2** + **预览再复制整窗** |

---

## 相关文档

| 文档 | 用途 |
|------|------|
| [README.md](../README.md) | 产品功能与安装 |
| [AGENTS.md](../AGENTS.md) | Agent / 贡献者工程约束与 provider 清单 |
| [DESIGN.md](../DESIGN.md) | 前端视觉与布局规范 |
| [CLAUDE.md](../CLAUDE.md) | Claude Code 项目指引 |
| [CONTRIBUTION.md](../CONTRIBUTION.md) | 贡献与环境配置 |

---

*本文档为审查快照。落地整改后，请同步更新完成度与清单状态，或追加「修订记录」小节。*
