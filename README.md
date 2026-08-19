# Mnemo

Mnemo 是一个基于 Go + Wails 的跨平台多网盘桌面文件管理器。它把不同网盘统一为文件、传输、分享和同步工作区，并使用系统 WebView 播放网页媒体，不再依赖 mpv 或其他平台播放器进程。

## 主要能力

- 统一浏览：目录树、列表、搜索、排序、回收站、收藏、标签和批量操作。
- 传输管理：分段下载、断点续传、限速、队列上传、暂停/继续、任务即时状态和跨盘迁移。
- 网页预览：文本、Markdown、图片和视频预览；视频通过 HTML5、HLS.js、dash.js 及本地会话代理处理鉴权流。
- 分享与同步：创建和管理分享链接，支持本地与网盘之间的定时 push、pull、two-way 同步及删除保护。
- 账号体验：多账号切换、账号显示名、配额信息和上次使用网盘记忆。
- 登录安全：PikPak 验证码挑战内嵌登录页，验证完成后自动继续登录并带请求冷却；短信、图片验证码、OAuth、Cookie 和账号密码登录按网盘能力显示。
- 运维设置：英文结构化日志，默认 `warning` 等级，支持清除、导出和调整日志等级。

## 支持的平台

| 平台 | 架构 | 发布产物 |
| --- | --- | --- |
| Windows | x64、arm64 | 安装程序 `.exe`、压缩包 `.zip` |
| Linux | x64、arm64 | Debian 包 `.deb`、压缩包 `.tar.gz` |
| macOS | Apple Silicon（M 系列） | 磁盘映像 `.dmg`、压缩包 `.tar.gz` |

安装后的产品文件名固定为：Windows `Mnemo.exe`、Linux `Mnemo`、macOS `Mnemo.app`。发布文件只包含产品名和平台/架构标识，不在文件名中写入版本号，例如 `Mnemo-windows-x64-Setup.exe`；版本号只用于 Git tag、更新检查和安装器元数据。

Windows 需要 WebView2 Runtime；Linux 需要发行版提供的 GTK/WebKit 运行库；macOS 使用系统 WebKit。各平台均不需要额外安装 mpv。

## 已内置的网盘

`pikpak`、`onedrive`、`dropbox`、`pan123`、`lanzou`、`ilanzou`、`pan139`、`pan189`、`aliopen`、`guangya`、`webdav`、`s3` 和 `yike`。

其中 yike 已在登录页暂时隐藏，后端注册代码保留，待该平台重新开发完成后再恢复入口。WebDAV 登录页提供坚果云、InfiniCLOUD、Nextcloud 和自定义地址预设；OneDrive/Dropbox 的 OAuth 配置由发布环境注入，不将 secret 写入仓库。

## 快速开始

### 环境要求

- Go `1.25` 或更高版本。
- Node.js `20` 或更高版本及 npm。
- Wails CLI `v2.14` 或兼容版本。
- 原生构建所需的平台工具链；Windows 构建还需要 WebView2 相关 SDK/运行环境。

### 安装依赖与开发

```bash
go mod download
cd frontend
npm ci
cd ..

# 启动 Wails 开发模式
wails dev
```

### 构建与验证

```bash
# 构建当前平台，输出到 build/bin/
wails build -clean

# 前端生产构建
cd frontend && npm run build && cd ..

# Go 单元测试、集成测试与静态检查
go test ./...
go vet ./...

# 检查提交中的空白和冲突标记
git diff --check
```

发布流水线位于 `.github/workflows/release.yml`，通过 `v*` Git tag 或手动触发。流水线会分别构建 Windows x64/arm64、Linux x64/arm64 和 macOS arm64，并上传安装包、压缩包和 `SHA256SUMS.txt`。更新器会按当前平台/架构选择资产并校验 SHA-256；Linux/macOS 下载后需要用户手动安装。

## 项目结构

```text
main.go                  Wails 入口与窗口配置
internal/app/             前端绑定、事件、生命周期和应用服务
internal/drive/           网盘驱动契约、能力声明和统一操作门面
internal/drive/providers/ 各网盘插件，使用 init() 注册
internal/model/           统一文件、账号、传输和预览模型
internal/netx/            HTTP、代理、上传、哈希和限速工具
internal/store/           原子 JSON 持久化与账号/任务/设置存储
internal/transfer/        分段下载、上传队列和跨盘迁移
internal/sync/            本地与网盘同步调度及快照
internal/preview/         本地播放会话、Range、HLS/DASH 和字幕代理
internal/logging/         结构化日志、脱敏、轮转和导出
frontend/                 Vue 3 + Vite 前端
build/                    Wails 图标、清单和平台安装器模板
docs/                     架构、设计和网盘插件文档
```

新增网盘请遵循 [网盘插件指南](docs/PROVIDER_GUIDE.md)，不要在 `internal/app` 或前端堆叠 provider 特判。架构边界见 [架构说明](docs/ARCHITECTURE.md)，界面约束见 [设计规范](DESIGN.md)。

## 日志与问题排查

日志默认写入应用数据目录的 `logs/mnemo.log`，默认等级为 `warning`，并对 token、Cookie、密码、验证码和 URL 查询参数脱敏。出现登录或传输问题时，优先在设置中导出日志并保留发生时间、网盘类型和操作步骤；不要手工粘贴未脱敏的 token、Cookie 或验证码链接。

PikPak 返回 `too frequent` 或 `AccessProhibited` 时，说明服务端风控处于冷却状态。停止重复点击，等待界面提示的冷却时间后再进行一次登录；反复刷新验证码会延长限制。

## 文档

- [架构说明](docs/ARCHITECTURE.md)
- [网盘插件指南](docs/PROVIDER_GUIDE.md)
- [界面设计规范](DESIGN.md)
- [发布记录](docs/releases/)
- [工程约束](AGENTS.md)

## 许可证

GPL-3.0
