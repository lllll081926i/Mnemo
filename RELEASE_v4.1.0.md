# BoxPlayer v4.1.0 Release Notes

## ✨ 新功能 - Motrix 全功能集成

### 🔧 主进程下载基础设施（Motrix 移植）

- **Engine 进程管理**：主进程托管 aria2c 子进程，支持 PID 文件、会话续传、优雅退出
- **ConfigManager 双 Store**：`system.json` + `user.json` 分离，基于 electron-store 响应式配置
- **EngineClient**：主进程 aria2 RPC 客户端（基于 aria2-lib）
- **UPnP 端口映射**：BT 下载自动映射 NAT 端口（`enable-upnp` 开关）
- **防休眠管理**：下载进行中自动阻止系统休眠
- **协议注册**：`magnet://`、`mo://` 自动关联到 BoxPlayer
- **BT Tracker 12h 自动同步**：启动后每 12 小时从 ngosang/trackerslist 拉取最新 tracker
- **7 份平台差异化 aria2.conf**：darwin/linux/win32 × x64/arm64/armv7l/ia32 全覆盖

### 📥 渲染端功能增强

- **任务详情抽屉** (`TaskDetailDrawer.vue`)：显示 GID/总大小/进度/速度/做种数/连接数/InfoHash/保存路径/文件列表
- **Torrent 文件选择** (`TorrentFileSelector.vue`)：BT 任务可选择下载哪些文件
- **下载列表右键"任务详情"/"选择文件"** 快捷入口
- **aria2 实时事件驱动**：订阅 `onDownloadStart/Complete/Error/Stop/BtComplete` 事件，任务状态 100ms 内响应
- **拖拽添加任务** (`DragDropZone.vue`)：从浏览器地址栏拖拽 URL/magnet/torrent 文件到下载页面
- **速度仪表** (`Speedometer.vue`)：实时显示总下载速度

### ⚙️ 设置页增强

- **SettingAria.vue 新增**：
  - BT Tracker 编辑框（每行一个 URL）+ 立即同步按钮
  - 上传限速配置
  - 做种比例/时间配置
  - 自动恢复未完成任务开关
  - 浏览器扩展对接 RPC 地址展示
- **高级下载** 新设置分区（`SettingDownloadAdvanced.vue`）

### 🔔 系统集成

- **任务栏下载进度条**：下载中 macOS Dock / Windows 任务栏显示进度环
- **下载完成系统通知**：每个文件完成后弹出系统级通知，点击激活主窗口
- **协议关联捕获**：macOS `open-url`/`open-file` 捕获 magnet 链接和 .torrent 文件，自动打开下载对话框
- **批量操作** (`batchPauseSelected/batchResumeSelected/batchRemoveSelected`)：直接操作 aria2，不经过轮询延迟

## 📦 新增文件

- `shared/constants.ts` · `shared/configKeys.ts` · `shared/ua.ts`
- `shared/utils/index.ts` · `shared/utils/tracker.ts` · `shared/utils/rename.ts`
- `electron/main/aria/` — Logger / Context / ConfigManager / Engine / EngineClient / UPnPManager / EnergyManager / ProtocolManager / MotrixApplication
- `electron/main/core/protocol.ts`
- `src/down/motrix-integration/taskTypes.ts` · `aria2TaskApi.ts` · `tracker.ts` · `protocolPayload.ts`
- `src/down/TaskDetailDrawer.vue` · `TorrentFileSelector.vue`
- `src/components/DragDropZone.vue` · `src/components/Speedometer.vue`
- `src/setting/SettingDownloadAdvanced.vue`
- `static/engine/{darwin,linux,win32}/*/aria2.conf`（7 份）

## 🔗 依赖新增

- `electron-store@8.x`（配置持久化）
- `@motrix/nat-api@0.x`（UPnP 端口映射）
- `bittorrent-peerid@1.x`（Peer ID 解析）
