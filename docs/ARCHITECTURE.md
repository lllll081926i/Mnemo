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
   └─ providers/  14 个插件包（pikpak/onedrive/.../thunder/webdav/s3）
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

下载设置区分任务并发（默认 3）与单文件请求并发（默认 8，1–16）；插件的显式 `Concurrency` 是上限，预览代理模式不决定本地下载并发。分片使用有限 worker 与自适应大小，已有合法断点保留原布局；先同步数据再持久化分片完成位图，暂停/失败保存检查点，退出等待下载 worker 收尾。

分片断点同时记录已完成位图和未完成分片的有效字节前缀（`partial_bytes`）。短读、暂停或有界降并发重试保留已写入字节，从同一分片的中断偏移继续；进度按实际有效字节计数，不对百分比取历史最大值。加载断点时校验数组长度、偏移边界、完整分片重复计数及原有资源身份；旧断点没有前缀字段时仍可读取。完整单流重下仍废弃分片断点，不能混合资源或恢复失效数据。

完整单流及小文件使用同一状态文件中的 `stream`、`stream_bytes` 保存连续字节检查点，以实际响应的 ETag/修改时间确认身份。恢复使用条件 Range，核对起点、总长度和资源身份后再截断未确认尾部并追加；服务端返回完整响应时从头覆盖，不混合旧字节。分段与单流均每秒保存检查点，先同步文件再原子保存 JSON；正常暂停和退出再保存最终进度。任务列表恢复读取本地检查点的有效字节数，不能把预分配的 `.part` 文件长度当作下载量。新签名 URL 需要相同稳定资源 ID 与可确认的身份；弱 ETag 不能单独确认字节一致性。

插件可通过内部 `DownloadURL.RetrySingleConnection` 启用一次单连接恢复，当前仅迅雷启用。降级只处理分段请求的短期 HTTP 服务错误，复用下载客户端的 Retry-After 冷却与合法分片；权限、限流、身份/长度校验和本地文件错误保持失败。下载失败日志记录具体错误而非仅记录错误是否存在，任务详情提供脱敏后的原始错误。

光鸭通过内部 `DownloadURL.AllowLastModifiedFallback` 兼容下载探测与分段响应的修改时间差异。匹配的强 ETag 优先于修改时间；没有匹配强 ETag 时，仅修改时间差异允许停止全部分段、废弃分片断点并重新获取一条完整响应。恢复仍校验 ETag 和完整长度，不拼接不同响应的文件内容。

普通下载、目录同步与跨盘迁移共用下载设置供应器和全局令牌桶。大文件 HTTP 无总时限，网络阻塞读取默认 60 秒空闲超时，不计限速或目标上传背压；临时 HTTP 状态有界重试并协调单下载内的 Retry-After。迁移的流式路径仍为顺序流。完整配置、性能证据和公网待测项见 [下载引擎审计](DOWNLOAD_ENGINE_AUDIT_2026-10-04.md)。

## 依赖方向

`app → drive → netx/model/store`；`providers → drive/netx/model`；无反向依赖。
provider 之间互相独立，只能通过 `drive` 注册表暴露能力。

## 工作区增强

- `WorkspaceView.vue` 组合两个 `PanView` 实例，隔离目录位置、搜索输入、选中状态与键盘事件。原生拖入统一注册，通过落点路由到可见目标栏。盘内复制走 `drive.Copy`，跨账号复制进入迁移队列。
- `GlobalSearch.vue` 先检索持久化目录缓存，按需用两个并发请求调用支持搜索的账号。缓存结果保留账号、盘、父目录和更新时间；缓存结果最多 1000 项，在线每账号最多展示 500 项。停止搜索后丢弃过期结果，不继续排队请求。
- `workspace.js` 提供分享文案解析、账号状态和备份白名单。备份采用版本化 JSON，包含偏好与本地收藏；不读取或导出账号 token。恢复是逐项合并，失败会提示部分恢复并允许重试。
- `internal/sync/plan.go` 让预览与执行共用差异计划。执行前重新扫描并验证计划摘要，快照绑定账号、目录与方向；首次双向同步的同名文件按冲突处理。默认保留两份，也可选择本地或网盘。检测主要基于大小、时间与服务商提供的哈希元数据，不是全量内容扫描。
- 同步下载先写临时文件再替换，上传前后核对本地文件状态；删除数量超过有效快照一半时停止。旧快照没有作用范围信息时不会用于自动传播删除。
- 迁移 `items` 保存每个资源的结果与错误；目标接口返回明确文件 ID 时可按需比较目标大小和共同哈希，接口不返回 ID 时明确标为未校验。流式传输检查字节数，失败回退临时文件；校验操作不修改迁移检查点。

- 同步下载在替换前再次核对本地文件是否仍与计划一致，包括计划中不存在、下载期间新建的文件。远端哈希变化参与删除冲突判定；Windows 特殊路径及同步临时文件名不进入同步计划。
- 移动式迁移清理来源前，先复查目标 ID 对应的资源、类型、大小及双方可比较的哈希，再重新读取来源元数据；来源目录必须已经清空。旧复制检查点恢复时也不能跳过目标检查。缺少目标 ID、查询失败或恢复记录无法落盘时保留来源，报告未完成；复制期间新增或修改的内容也保留。退出等待全部迁移收尾，进度与启动响应使用独立快照。各网盘的元数据精度和条件删除能力不同，这些检查不能替代服务端事务锁。

## 工作区验证

### 下载完成提示音

- 两段音效保存为 `frontend/src/assets/audio/download-complete-1.mp3`、`download-complete-2.mp3`；旧音效不再参与构建。
- 设置提供“提示音 1 / 提示音 2”与试听，保留总开关。选择通过本地偏好 `downloadSoundType` 保存，并纳入偏好备份白名单；旧设置或无效值默认使用提示音 1。
- 设置试听和下载完成共用音效配置；切换音效时停止旧音频，下次下载完成读取最新选择。保留单文件及文件夹整体完成通知规则，子下载任务不重复响铃。

### 关闭与退出

- 启用关闭到托盘时，关闭主窗口仅隐藏；托盘退出或禁用关闭到托盘时才真正退出。无托盘的平台直接进入退出流程。
- 存在排队或下载中的任务时，后端阻止首次关闭并发送 `app:quit-confirm`（进行中的下载数量），前端使用应用内 `ConfirmModal`，默认保留继续下载的选择，不再使用原生消息框。
- 前端调用 `RespondQuit(bool)`：取消清除退出标记；确认设置已确认状态后重新请求 Wails 退出，关闭钩子不再重复拦截。重复关闭或重复回复不会反复弹窗；最终仍由 `OnShutdown` 释放资源和保存任务。

前端运行 `npm test`，后端运行 `go test ./...`。`frontend/workspace.browser.cjs` 用 Edge 无头浏览器和本地 mock 验证 20 账号、10000 文件、双栏焦点、分享、同步、搜索和备份。需要可用的 Playwright 模块；通过 `PLAYWRIGHT_MODULE` 指定安装位置，`QA_URL` 指定 Vite 地址，`QA_OUTPUT` 指定截图目录。脚本不连接真实网盘。
