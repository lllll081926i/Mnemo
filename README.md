# Mnemo-Go

多网盘桌面文件管理器。

## 功能

- 13 个网盘统一管理：pikpak · onedrive · dropbox · pan123 · lanzou · ilanzou · pan139 · pan189 · yike · aliopen · guangya · webdav · s3
- 文件浏览、上传/下载、跨盘迁移、批量重命名
- 分段下载 + 断点续传
- 在线预览（文本/图片/视频/Markdown）
- 网页播放器播放（原生 HTML5 + HLS.js / dash.js，自适应流经本地会话代理）
- 双向同步引擎（两盘/本地↔网盘，定时调度）
- 分享链接管理
- 多账号切换、配额展示
- 深色/浅色主题跟随系统

## 技术栈

- **Go 1.25+** + **Wails v2**（Go 后端 + WebView2 前端）
- **Vue 3.5 + Vite 6**（纯 JavaScript，非 TypeScript）
- 原生 Go HTTP Range 分段下载器
- 纯 Go，无 cgo 外部依赖

## 开发

```bash
# 环境：Go ≥ 1.25、gcc（Windows）、Node ≥ 20
go mod tidy
cd frontend && npm install && cd ..
wails dev          # 热重载开发
wails build        # 产物 build/bin/
go test ./...      # 单元测试
```

## 目录

```
main.go                入口
internal/
  app/                 Wails 绑定层（前端调用的方法 + 事件）
  drive/               provider 插件契约 + ops 门面
  drive/providers/     13 个网盘插件（init() 注册）
  model/               统一数据模型
  netx/                HTTP 客户端/上传/哈希/限速
  store/               本地持久化（原子 JSON）
  transfer/            分段下载器 + 上传队列 + 跨盘迁移
  sync/                双向同步引擎
  preview/             本地 Range/播放会话代理（鉴权流、HLS/DASH、字幕）
frontend/              Vue 3 前端
docs/                  架构/加盘指南/设计规范
```

## 文档

- [架构说明](docs/ARCHITECTURE.md)
- [如何新增网盘插件](docs/PROVIDER_GUIDE.md)
- [界面设计规范](docs/DESIGN.md)
- [工程约束](AGENTS.md)

## 许可

GPL-3.0
