# Mnemo-Go 架构

## 分层

```
frontend (Vue3, webview)
   │  Wails Binding (Go 方法直调 + Events.Emit 事件推送)
   ▼
internal/app      绑定层：账号/网盘/传输/播放/同步/设置
   ▼
internal/drive    ops 门面（唯一业务入口，禁止中央 if provider）
   ├─ registry / capabilities / meta
   └─ providers/  13 个插件包（pikpak/onedrive/.../webdav/s3）
internal/transfer  原生 Go 分段下载器 · 上传队列 · 跨盘迁移
internal/sync      双向同步
internal/preview   本地 Range/播放会话代理（鉴权流、HLS/DASH 与字幕）
internal/store     本地持久化（原子 JSON，多集合；读改写由 Store 互斥保护）
internal/netx      HTTP/上传/哈希/限速 工具
```

## 核心不变量

1. **插件化**：新增网盘 = 新增一个 `internal/drive/providers/<id>` 包，
   实现 `drive.Driver` 接口，`init()` 调用 `drive.Register(...)`；
   主程序只通过 blank import 装配。禁止在门面或 UI 里写 `if provider == ...`。
2. **能力裁剪**：每个 provider 声明 `Capabilities`（能力位），UI 菜单按位裁剪。
3. **统一模型**：所有盘输出 `model.File`；列表/搜索/回收站共用同一前端组件。
4. **认证分离**：`store` 只存账号凭据，刷新由各 provider `RefreshAccount` 负责，
   凭据以 `tokenfrom/user_id` 命名空间隔离。
5. **事件优先、限频轮询兜底**：任务进度和应用内状态变化优先通过 `Events.Emit` 推送；容量、云离线任务等无法稳定推送的外部状态使用去重、缓存和页面可见性保护后的低频轮询。

## 数据流

| 场景 | 路径 |
|------|------|
| 登录 | 前端 → app.Login* → provider.Auth → store.SaveAccount → 事件 account:changed |
| 列表 | app.ListDir → drive ops → driver.List → model.File[] |
| 下载 | app.Download → transfer/manager → driver.GetDownloadURL → dlengine 分段下载 |
| 上传 | app.Upload → transfer/upload → driver.UploadOneFile（queue/direct 按能力） |
| 播放 | app.PlayVideo → driver.GetVideoPreview → preview 播放会话代理 → HTML5 `<video>`；HLS/DASH 由按需加载的 HLS.js/dash.js 驱动 |
| 迁移 | app.Migrate → transfer/migrate（server/stream/spool 策略） |

## 依赖方向

`app → drive → netx/model/store`；`providers → drive/netx/model`；无反向依赖。
provider 之间互相独立，只能通过 `drive` 注册表暴露能力。
