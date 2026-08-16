# Mnemo → Mnemo-Go 迁移进度报告

> 旧项目：`../Mnemo`（Electron + Vue3 + TypeScript + aria2c + mpv）
> 新项目：`Mnemo-Go`（Go + Wails v2 + Vue3 JS + 原生 Go 下载引擎 + mpv JSON IPC）
> 调研日期：2025-08（全量源码调研 + 旧版对照）
> 废弃说明：`gofile`、`gdrive`、`encryption` 已按需求移除/不再支持

---

## 一、整体迁移进度

| 维度 | 进度 | 说明 |
|------|:----:|------|
| 后端核心架构 | ~85% | drive/transfer/store/preview/player/netx/config 已迁移；迁移策略和同步引擎为简化版 |
| 后端 Provider | ~85% | 13 个 provider 全部已移植且可注册，但深度不一 |
| 前端视图与组件 | ~70% | 5 视图 + 16 组件完整，缺瀑布流/分享导入/字幕选择/播放列表 |
| 设计系统 | ~90% | 令牌系统更系统化，但移除多色包 |
| 文档 | ~65% | 3 核心文档 + 发版记录，旧 8 份辅助文档缺失（现已新增 PROVIDER_STATUS + 13 网盘详情） |
| 测试 | ~50% | 后端 21 个 Go 测试含 e2e；前端零测试 |
| **综合** | **~75%** | 核心功能链路可用，多处深度功能缺失 |

---

## 二、后端模块迁移

| 模块 | 路径 | 状态 | 完成度 | 关键差距 |
|------|------|:----:|:------:|---------|
| app（绑定层） | internal/app/ | ✅ | ~95% | 无 TransferBall 独立绑定、无 powerSaveBlocker、无 autoUpdate 逻辑 |
| drive（契约层） | internal/drive/ | ✅ | ~95% | filecache 无 TTL 过期、无搜索索引 |
| drive/providers | internal/drive/providers/ | ⚠️ | ~85% | 详见 [PROVIDER_STATUS.md](PROVIDER_STATUS.md) |
| netx | internal/netx/ | ✅ | ~80% | 无统一 Set-Cookie 中继、无下载代理穿透(CONNECT/DNS缓存/flow-enc)；上传限速令牌桶已实现（SetGlobalUploadRate + ProgressReader throttle） |
| store | internal/store/ | ✅ | ~90% | 无加密存储（已废弃加密，符合需求） |
| transfer | internal/transfer/ | ✅ | ~90% | 原生 Go 分段下载器替代 aria2c；上传历史管理 API 未明确 |
| player | internal/player/ | ⚠️ | ~60% | 无 texture bridge/overlay，mpv 独立窗口非应用内嵌入 |
| preview | internal/preview/ | ✅ | ~85% | 无 CONNECT 隧道/DNS 缓存/123 CDN 路由/proxyAccessToken |
| sync | internal/sync/ | ⚠️ | ~50% | 无 hash/timestamp diff、无快照持久化、无定时调度、无日志、pull 不递归子目录 |
| migrate（跨盘迁移） | internal/transfer/migrate/ | ⚠️ | ~40% | 仅 spool 临时文件中转，缺 server/stream/rapid 三级策略 |
| config/engine/e2e | — | ✅ | ~90% | 4 个 provider 有 e2e，9 个无 |

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
| 分享导入 UI | ❌ | 旧 ShareImportPanel，新项目无 |
| 播放器字幕选择 | ❌ | 旧 CloudSubtitlePickerModal，新项目无 |
| 播放列表 | ❌ | 旧 MpvPlaylistPanel，新项目无 |
| 多音轨 | ❌ | 旧 MpvSettingsPanel，新项目无 |
| 任务详情抽屉 | ❌ | 旧 TaskDetailDrawer，新项目无 |
| 调试/日志设置页 | ❌ | 旧 SettingDebug/SettingLog |
| 加密/私密标记 | ❌ | 已废弃加密 |
| 前端测试 | ❌ | 旧版有 vitest，新版无测试框架 |

### 3.3 设计令牌

新设计令牌更系统化（动效标尺、圆角阶梯、glider 选中态），但主动移除多色包，主色从 `#4f46e5` 靛蓝改为 `#7c3aed` 品牌紫。

---

## 四、Provider 迁移矩阵（简表）

> 完整功能矩阵见 [PROVIDER_STATUS.md](PROVIDER_STATUS.md)

| Provider | 完成度 | P0 差距 |
|----------|:------:|---------|
| pikpak | ~85% | API captcha 续接(部分) |
| aliopen | ~85% | (已修复) |
| pan123 | ~90% | (已修复) |
| pan189 | ~95% | 无（最忠实移植） |
| pan139 | ~95% | (已修复) |
| lanzou | ~98% | 无（源码级移植） |
| ilanzou | ~95% | (已修复) |
| onedrive | ~85% | (已修复) |
| dropbox | ~85% | (已修复) |
| yike | ~90% | decryptYikeMd5 缺失 |
| guangya | ~98% | 无 |
| webdav | ~85% | rootPath 缺失 |
| s3 | ~85% | (已修复) |

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
2. ✅ **分享导入 importShare**：pikpak/aliopen/pan123 三个盘已实现
3. ✅ **pikpak API captcha 续接**：业务 API 检测 captcha_required 自动获取新 token 重试(jsonDo/get)
4. ✅ **aliopen CompleteUpload bug**：已传真实 upload_id
5. ✅ **s3 目录递归操作**：已实现 listAllUnder + 批量 DeleteObjects + copyRecursive
6. ✅ **s3 forcePathStyle 可配置**：改为 *bool，支持 sessionToken
7. ✅ **webdav/s3 上传冲突策略**：已实现 refuse/rename/overwrite + ConflictPolicy
8. ✅ **预览服务器安全边界**：会话令牌 + SSRF 防护 + 根目录限制
9. ✅ **生命周期与异常处理**：OnShutdown + 错误不静默 + panic 防护
10. ✅ **上传队列重写**：并发槽 + Cancel 真正取消 + Resume + 启动恢复
11. ✅ **下载设置生效**：并发限制 + 限速 + 代理运行时生效
12. ✅ **同步引擎语义**：递归子目录 + 嵌套结构 + ModTime 比较 + ctx 传递
13. ✅ **跨盘迁移**：partial 状态 + 字节进度 + Cancel + 流式预留

### 🟡 P1（影响健壮性）

14. ✅ **上传断点续传持久化**：aliopen/pan123/pan189/onedrive 已实现 session 存储
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

24. ⚠️ 限速/重试：aliopen 并发限速、dropbox 429 重试、pikpak 速率限制
25. ⚠️ 版本历史：onedrive/dropbox revisions
26. ⚠️ 缩略图：dropbox get_thumbnail_v2
27. ⚠️ 账号凭据系统级保护：DPAPI/钥匙串（需平台特定实现）
28. ⚠️ 前端缺失功能：瀑布流/字幕选择/播放列表/调试设置页
29. ⚠️ 前端测试框架
30. ⚠️ 测试覆盖率：9/13 provider 无单测
31. ✅ **上传进度回调：webdav/s3**：已实现 ProgressReader + 令牌桶限速（`progress.go:22`）
32. ✅ **MaxUploadSpeed 限速**：已实现 SetGlobalUploadRate + ProgressReader throttle（`app.go:128,328`）
33. ✅ **yike decryptYikeMd5**：已实现对齐 alist DecryptMd5
34. ⚠️ player 字幕选择 + 播放列表 + 多音轨
35. ⚠️ 前端测试框架搭建
36. ⚠️ 瀑布流/相册模式
37. ⚠️ 调试/日志设置页
