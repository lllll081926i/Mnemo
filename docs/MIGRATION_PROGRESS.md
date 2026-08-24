# Mnemo → Mnemo-Go 迁移进度报告

> 旧项目：`../Mnemo`（Electron + Vue3 + TypeScript + aria2c + mpv）
> 新项目：`Mnemo-Go`（Go + Wails v2 + Vue3 JS + 原生 Go 下载引擎 + HTML5 网页播放器）
> 最近更新：2026-08-21（全量源码调研 + 旧版对照 + 迁移回归验证 + 跨平台构建复核；08-20/08-21 两轮审查后同步修正过时声明，详见 [PROJECT_AUDIT_2026-08-20.md](PROJECT_AUDIT_2026-08-20.md) / [PROJECT_AUDIT_2026-08-21.md](PROJECT_AUDIT_2026-08-21.md)）
> 废弃说明：`gofile`、`gdrive`、`encryption` 已按需求移除/不再支持

---

## 一、整体迁移进度

| 维度 | 进度 | 说明 |
|------|:----:|------|
| 后端核心架构 | ~95% | drive/transfer/store/preview/netx/config 已迁移；08-20 审查 42 项修复（F-01~F-42）落地，安全边界加固 |
| 后端 Provider | ~95% | 13 个 provider 全部已移植且可注册，能力矩阵 P0/P1 全部关闭（见 [PROVIDER_STATUS.md](PROVIDER_STATUS.md)） |
| 前端视图与组件 | ~85% | 5 视图 + 16 组件完整，缺瀑布流/多音轨/任务详情抽屉 |
| 设计系统 | ~90% | 令牌系统更系统化，但移除多色包 |
| 文档 | ~75% | 核心文档齐备（含两轮审查报告、13 网盘详情）；旧 8 份辅助文档仍缺 |
| 测试 | ~70% | 后端 26 包单测/mock/e2e + 核心 `-race` 通过；前端 Vitest 16 用例；provider 深度不均、未覆盖真实云端账号 |
| **综合** | **≈88%** | 核心功能链路可用且经安全加固，处于“功能收尾+验证补强”阶段 |

---

## 二、后端模块迁移

| 模块 | 路径 | 状态 | 完成度 | 关键差距 |
|------|------|:----:|:------:|---------|
| app（绑定层） | internal/app/ | ✅ | ~95% | 无 TransferBall 独立绑定（已按需求移除悬浮球）、无 powerSaveBlocker；自动更新已实现（internal/updater + 设置项接入） |
| drive（契约层） | internal/drive/ | ✅ | ~95% | Go 侧 filecache 仍无 TTL/容量上限（前端目录缓存已有 10 分钟 TTL）；无搜索索引 |
| drive/providers | internal/drive/providers/ | ⚠️ | ~85% | 详见 [PROVIDER_STATUS.md](PROVIDER_STATUS.md) |
| netx | internal/netx/ | ✅ | ~80% | 无统一 Set-Cookie 中继、无下载代理穿透(CONNECT/DNS缓存/flow-enc)；上传限速令牌桶已实现（SetGlobalUploadRate + ProgressReader throttle） |
| store | internal/store/ | ✅ | ~95% | 原子 JSON + 上传会话 0700/0600；账号凭据 AES-256-GCM 加密（internal/vault），Windows 密钥绑定 DPAPI，非 Windows 保留 0600 兼容密钥文件 |
| transfer | internal/transfer/ | ✅ | ~90% | 原生 Go 分段下载器替代 aria2c；上传历史管理 API 未明确 |
| player | frontend/src/components/PlayerPanel.vue | ✅ | ~90% | HTML5 原生媒体 + 按需 HLS.js/dash.js；浏览器不支持的容器不承诺在线解码 |
| preview | internal/preview/ | ✅ | ~95% | 不透明播放会话、HLS/DASH 清单重写、字幕代理、Range 和签名刷新已覆盖 |
| sync | internal/sync/ | ✅ | ~85% | 已支持递归目录、大小/修改时间比较、快照、删除传播阈值保护、日志与定时调度；高级冲突合并仍有限 |
| migrate（跨账号/跨盘迁移） | internal/transfer/migrate/ | ✅ | ~85% | 同一网盘的不同账号与跨网盘均走同一迁移链路：源账号解析指纹、目标账号发起秒传，账号上下文和文件缓存严格隔离；按共同哈希秒传 → 流式 → 临时文件降级。仅明确 miss 可完整上传，目标状态不确定错误停止；checkpoint 写失败会传播且 move 删除源前必须确保持久化；Yike 继续显式排除秒传 |
| config/engine/e2e | — | ✅ | ~90% | 多个 provider 有 mock/e2e；真实云端账号验证仍需发布前按平台执行 |

---

## 三、前端迁移

### 3.1 结构对比

| 旧项目（TS + 模块化） | 新项目（JS + 扁平化） |
|----------------------|----------------------|
| src/ 每个功能模块独立目录（pan/down/sync/share/migrate/setting/layout/components/store/transfer）| frontend/src/ 5 视图 + 16 组件 |
| TypeScript + Arco Design | 纯 JavaScript + 自研组件 |
| Pinia + vue-router | Composition API 无路由 |

### 3.2 缺失功能

| 功能 | 状态 | 说明 |
|------|:----:|------|
| 瀑布流/相册模式 | ❌ | 旧 PanRight 相册盘瀑布流，新 PanView 无 |
| 分享导入 UI | ✅ | 分享页按 importShare 能力筛选目标账号，支持解析、文件选择和目录转存 |
| 播放器字幕选择 | ✅ | 网页播放器原生 `<track>` + SRT→WebVTT 代理 |
| 播放列表 | ✅ | PlayerPanel 剧集/播放列表菜单（episodeFiles），对齐旧 MpvPlaylistPanel 的核心场景 |
| 多音轨 | ❌ | 旧 MpvSettingsPanel 能力，网页播放器结构性缺失 |
| 任务详情抽屉 | ❌ | 旧 TaskDetailDrawer，新项目无 |
| 调试/日志设置页 | ✅ | 日志级别 Info/Debug 设置已并入 SettingsView（对应后端分级日志 F-23） |
| 加密/私密标记 | ❌ | 已废弃加密（注意：与凭据加密存储 internal/vault 是两回事，后者已实现） |
| 前端测试 | ✅ | Vitest + jsdom：组件交互、mock Wails RPC、缓存逻辑共 16 用例通过；尚无页面级 E2E |

### 3.3 设计令牌

新设计令牌更系统化（动效标尺、圆角阶梯、glider 选中态），但主动移除多色包，主色从 `#4f46e5` 靛蓝改为 `#7c3aed` 品牌紫。

---

## 四、Provider 迁移矩阵（简表）

> 完整功能矩阵见 [PROVIDER_STATUS.md](PROVIDER_STATUS.md)

| Provider | 完成度 | P0 差距 |
|----------|:------:|---------|
| pikpak | ~98% | 主要迁移链路已完成；登录协议与旧版逐字节一致（见下方专项核对）；真实云端验证码、长时上传和大文件传输仍需发布前按账号验证 |
| aliopen | ~95% | (上传秒传探测、动态分片、断点复用、Live Photo 流回退已修复；转码清晰度仍按设计未接入) |
| pan123 | ~95% | (已修复；下载/预览点击快照与 `pan123meta` 恢复已补齐) |
| pan189 | ~95% | 无（最忠实移植） |
| pan139 | ~95% | (已修复) |
| lanzou | ~98% | 无（源码级移植） |
| ilanzou | ~100% | (已修复；登录续期、下载/视频预览、七牛上传与跨盘 MD5 秒传已闭环) |
| onedrive | ~95% | (已修复；rclone OAuth 凭据、账号选择、刷新、上传响应和预签名预览链路已覆盖定向回归) |
| dropbox | ~95% | (已修复；rclone OAuth 凭据、刷新保留、账号隔离、分享分页、上传 ID 校验和预签名预览链路已覆盖定向回归) |
| yike | ~90% | (已修复；按需求不参与跨盘秒传) |
| guangya | ~98% | 无 |
| webdav | ~95% | 已支持跨盘 `StreamUploader` 直接 PUT；通用 ETag 不作为内容哈希；分享/搜索为设计不支持 |
| s3 | ~95% | multipart 上传/复制已完成；ETag 不冒充 MD5，跨盘暂保留可重放 spool；分享/搜索/回收站为设计不支持 |

### 4.1 PikPak 登录逻辑与旧版一致性核对（2026-08-21）

对照 `../Mnemo/src/pikpak/auth.ts` 与 `internal/drive/providers/pikpak/auth.go` + `client.go` 逐项核实：

| 协议要素 | 一致性 |
|---|---|
| 端点（captcha/init、auth/signin、auth/token、drive/v1/about） | ✅ 完全一致 |
| 客户端身份（clientID/clientVersion/packageName/redirectURI/UA） | ✅ 逐字节一致 |
| captcha_sign 盐链（15 盐 MD5 链 + "1." 前缀） | ✅ 完全一致 |
| signin 请求形状（body 仅 client_id/username/password，验证码走 X-Captcha-Token 头） | ✅ 一致 |
| signin captcha meta 单字段（email/phone_number/username）+ redirect_uri 仅 signin 携带 | ✅ 一致 |
| 无滑块路径单次重试语义（captcha_required 复用 token / captcha_invalid 换新链） | ✅ 一致 |
| 设备 ID：按账号稳定 32-hex，MD5(小写账号) 为键持久化 | ✅ 一致（存储从 localStorage 改为应用数据目录文件） |
| 限流分类（429/关键词/Retry-After，最小 30s 冷却） | ✅ 一致且更完整（新增 access_prohibited 10 分钟风控分类与进程级按账号冷却门） |
| API captcha 缓存（设备+账号+action 维度、链式 previousToken） | ✅ 一致（缓存键额外加入 accessToken 指纹；TTL 固定 4m30s vs 旧版服务端 expiresIn-30s） |
| token 刷新保留语义（refresh_token/expires_in 缺失时保留旧值） | ✅ 一致 |

有意差异（均有代码注释说明，非回归）：

1. **滑块后确认策略**：旧版自动链式重试 3 次（600/1200/2000ms）；新版等待 1.5s 后仅发一次确认请求，失败则要求用户重新触发——避免自动重试循环延长服务端风控。
2. **设备 ID 生成**：旧版 UUIDv4 位格式；新版纯随机 32-hex（两者均被服务端接受；带连字符 UUID 才会被拒）。
3. **验证码窗口架构**：Electron BrowserWindow+IPC 桥 → Wails + 本地回调会话（internal/captcha），完成事件经 `pikpak:captcha:completed` 回传。
4. **accountID 优先级**：新版 user_id 字段 > sub > username；旧版 sub > JWT payload > username。

---

## 五、示例项目可借鉴要点

> 完整分析见各示例项目调研报告（agent 输出）

### 5.1 LitePan（Go + chi + SQLite + Vue3）
- 驱动能力接口组合（最小契约 + 可选接口 + 类型断言检测）
- 反射生成配置表单（struct tag → FieldSchema）
- 认证状态机（阶梯冷却 + 主动/被动双通道 + 网络错误豁免）
- 账号级请求间隔门 + configHash 实例复用
- LRU + TTL + 字节上限 + singleflight + 持久化元数据缓存
- 写后 mutationFence + 增量列表更新
- 下载 302 防缓存头 + Range 并发代理 + linkHolder 防重复刷新
- 上传双计数并发槽 + 断点续传 debounce 落库 + 关停 flush
- 速度平滑算法（滑动窗口 + EMA）
- 进程内事件总线 + 自动化规则引擎
- 跨盘路由自动推导（基于哈希能力两两匹配）
- 纯 Go SQLite 双连接池（写1读4 + WAL + busy_timeout）

### 5.2 AList（Go + Gin + GORM + 80+ 驱动）
- driver 三层分离：internal/driver（接口）vs drivers（实现）vs internal/op（操作）
- 能力即接口（Mkdir/Move/Copy/Remove/Put 独立小接口）+ Result 变体
- struct tag 配置反射（零样板代码）
- init() 自注册 + drivers/all.go 空白导入
- Link 三级降级（URL → RangeReadCloser → MFile）
- 离线下载 Tool 接口抽象（双模式 Run vs AddURL+Status）
- 智能转存路由 + PutURL 优化
- 四维限速 + 动态调整 + blockBurstLimiter
- op 层模式（缓存 + singleflight + hook + 增量更新）
- Reference 接口（同网盘多账号共享登录态）
- 包装器驱动（Crypt/Chunker/Alias 装饰器模式）

### 5.3 rclone（Go + 70+ 后端 + VFS）
- 窄核心接口（Fs 仅 5 方法，Object 仅 4 方法）
- Features.Fill() 自动发现 20+ 可选能力 + Mask 取交集
- RegInfo 注册 + Option 一元化（CLI/env/config/RPC）
- VFS 四级缓存（off/minimal/writes/full）+ WriteBack 延迟上传
- 三管道并行传输（checker → transfer → delete）+ March 锁步遍历
- noTraverse 大目标优化 + trackRenames 重命名检测
- Pacer 限流 + RetryAfter + 五维错误分类
- dircache 路径↔ID 双向缓存 + encoder 文件名编码

---

## 六、剩余工作优先级

### 🔴 P0（影响功能正确性）

1. ✅ **onedrive/dropbox RefreshAccount**：已实现 token 刷新 + 账号信息/配额
2. ✅ **分享导入 importShare**：pikpak/aliopen/pan123 三个盘已实现，分享页导入 UI 已接通
3. ✅ **pikpak API captcha 续接**：业务 API 检测 captcha_required 自动获取新 token 重试(jsonDo/get)
4. ✅ **aliopen 上传兼容**：真实 upload_id、pre_hash/proof_code、动态分片、断点会话复用和冲突策略已补齐
5. ✅ **s3 目录递归操作**：已实现 listAllUnder + 批量 DeleteObjects + copyRecursive
6. ✅ **s3 forcePathStyle 可配置**：改为 *bool，支持 sessionToken
7. ✅ **webdav/s3 上传冲突策略**：已实现 refuse/rename/overwrite + ConflictPolicy
8. ✅ **预览服务器安全边界**：会话令牌 + SSRF 防护 + 根目录限制
9. ✅ **生命周期与异常处理**：OnShutdown + 错误不静默 + panic 防护
10. ✅ **上传队列重写**：并发槽 + Cancel 真正取消 + Resume + 启动恢复
11. ✅ **下载设置生效**：并发限制 + 限速 + 代理运行时生效
12. ✅ **同步引擎语义**：递归子目录 + 嵌套结构 + ModTime 比较 + ctx 传递
13. ✅ **跨账号/跨盘迁移**：同一网盘不同账号会使用源账号读取指纹、目标账号发起秒传；partial 状态 + 字节进度 + Cancel + WebDAV 流式 PUT；秒传只使用规范化后的共同指纹能力，Yike 源/目标均显式排除；模糊目标错误不自动二次上传。若服务端不允许该指纹跨账号复用，则只在目标支持常规上传时安全降级为普通迁移

### 🟡 P1（影响健壮性）

14. ✅ **上传断点续传持久化**：aliopen/pan123/pan189/onedrive/dropbox 已实现 session 存储与身份隔离
15. ✅ **能力声明与实现不符**：pikpak/pan123 的 trashPurge/trashClear/playbackHistory 改 false
16. ✅ **哈希能力声明缺失**：ilanzou/onedrive/dropbox 已声明 SetHashes
17. ✅ **pikpak 离线下载进度/删除**：RefreshOfflineTasks + DeleteOfflineTask
18. ✅ **pan139 ListPage 分页参数推进**：pageNum/startNumber 随 marker 递增
19. ✅ **onedrive/dropbox 搜索分页**：跟随 nextLink/cursor 分页
20. ✅ **pikpak batch 操作任务轮询**：waitForTasks 轮询异步任务
21. ✅ **yike decryptYikeMd5**：已实现对齐 alist DecryptMd5
22. ✅ **sync 引擎高级特性**：快照持久化 + 定时调度 + 删除传播(含 50% 阈值保护) + 日志回调
23. ✅ **migrate 高级策略**：StreamUploader 流式管道 + 秒传路由(哈希匹配) + 任务持久化+重启恢复

### 🟢 P2（体验优化）

24. ⚠️ 限速/重试：✅ aliopen 并发限速/401 刷新/429 退避；✅ pikpak 登录/API 429 冷却识别；✅ dropbox 429 重试
25. ⚠️ 版本历史：onedrive/dropbox revisions
26. ⚠️ 缩略图：dropbox get_thumbnail_v2
27. ✅ **账号凭据系统级保护**：Windows 已实现 DPAPI 绑定密钥（`vault_os_windows.go`），旧 keyfile 自动迁移；macOS Keychain/Linux Secret Service 仍未接入，保留 0600 兼容文件
28. ⚠️ 前端缺失功能：瀑布流/多音轨/任务详情抽屉
29. ⚠️ 页面级 E2E 测试（组件级 Vitest 已落地）
30. ⚠️ 测试覆盖率：provider 深度覆盖仍不均衡，当前主要是 mock/e2e 关键链路，未覆盖真实云端账号
31. ✅ **上传进度回调：webdav/s3**：已实现 ProgressReader + 令牌桶限速（`progress.go:22`）
32. ✅ **MaxUploadSpeed 限速**：已实现 SetGlobalUploadRate + ProgressReader throttle（`app.go:128,328`）
33. ✅ **yike decryptYikeMd5**：已实现对齐 alist DecryptMd5
34. ⚠️ player 多音轨
35. ✅ **前端测试框架**：Vitest 已搭建并通过（见上表）
36. ⚠️ 瀑布流/相册模式
37. ⚠️ 调试/日志设置页
38. ✅ **123 云盘参数与上传策略兼容**：AList 下载参数兼容旧版 Base64 解码；upload_request 按 `ConflictPolicy` 映射 `duplicate`
