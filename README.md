# Mnemo-Go

Mnemo 的 Go + Wails v2 重写版 —— 多网盘桌面文件管理器。

> 旧版 Electron 工程位于 `../Mnemo`（封存参考，不再修改）；`../Example` 仅作实现参考（含 rclone Go 源码），不参与构建。

## 技术栈

- **Go 1.26** + **Wails v2**（桌面壳：Go 后端 + WebView2 前端）
- **Vue 3 + Vite + TypeScript**（全新精简前端，样式 token 复用旧版）
- **aria2c** 多线程下载引擎（懒启动）
- **mpv** 进程 JSON IPC 内嵌播放
- 纯 Go 无 cgo 外部依赖（现代 `net/http`、纯 JSON 本地存储）

## 支持的网盘（13 个）

pikpak · onedrive · dropbox · pan123 · lanzou · ilanzou · pan139 · pan189 · yike · aliopen · guangya · webdav · s3

> gofile、gdrive 已按需求移除。

## 开发

```bash
# 环境：Go ≥ 1.25、gcc（Windows）、Node ≥ 20
go mod tidy
wails dev          # 热重载开发
wails build        # 产物 build/bin/
go test ./...      # 单元测试
```

## 目录

```
main.go                    入口（Wails 应用装配）
internal/
  app/                     Wails 绑定层（前端调用的全部方法 + 事件）
  drive/                   provider 插件契约（接口/能力/元数据/注册表）+ ops 门面
  drive/providers/         13 个网盘插件（每个一个包，init() 注册）
  model/                   统一数据模型（文件/账号/分享/任务/设置）
  netx/                    HTTP 客户端/上传/哈希/限速
  store/                   本地持久化（账号/设置/标签/收藏/任务，原子 JSON）
  transfer/                aria2 引擎 + 上传队列 + 跨盘迁移
  player/                  mpv JSON IPC 桥
  sync/                    双向同步引擎
  preview/                 本地 Range 代理（鉴权流/预览）
  engine/                  内嵌引擎二进制（aria2c/mpv）释放
frontend/                  Vue 3 前端（Wails webview）
docs/                      架构/加盘指南/设计规范
```

## 文档

- [架构说明](docs/ARCHITECTURE.md)
- [如何新增网盘插件](docs/PROVIDER_GUIDE.md)
- [界面设计规范](docs/DESIGN.md)
- [工程约束](AGENTS.md)

## 许可

GPL-3.0（继承旧版 Mnemo 协议）。
