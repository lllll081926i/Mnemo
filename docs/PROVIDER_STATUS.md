# 网盘功能支持矩阵（Provider Status）

> 数据来源：当前仓库 `internal/drive/providers/*`、统一能力注册表和本地自动化测试（最近更新：2026-08-20）。本次未读取父目录旧项目，也未使用真实网盘账号做在线验证。
> 废弃说明：`gofile`、`gdrive` 已按需求移除；`encryption`（加密文件名/加密流）不再支持，相关能力位不再纳入矩阵。
> 图示：✅ 已实现 · ⚠️ 部分/有差距 · ❌ 缺失/不支持 · ➖ 设计上不适用

---

## 一、在役网盘（13 个）

| # | Provider | 登录方式 | 能力实现 | 当前自动验证范围 | 已知限制 |
|---|----------|---------|:--------:|------------------|----------|
| 1 | pikpak | 账密 + 验证码 | ✅ 已注册 | 包内单测 + mock/e2e | 登录可能触发服务端风控；必须遵守冷却 |
| 2 | aliopen（阿里云盘） | refresh_token | ✅ 已注册 | mock/e2e | 包内覆盖仍不足 |
| 3 | pan123（123 云盘） | 账密 | ✅ 已注册 | 包内单测 + mock/e2e | 真实服务未在本轮验证 |
| 4 | pan189（天翼云盘） | 账密 + 验证码 | ✅ 已注册 | 包内单测 + mock/e2e | 部分回收站能力不完整 |
| 5 | pan139（139 云盘） | 手机号/邮箱 + 密码 / Authorization | ✅ 已注册 | mock/e2e | 容量接口未接入；包内覆盖不足 |
| 6 | lanzou（蓝奏云） | Cookie / 账密 | ✅ 已注册 | 包内单测 + mock/e2e | 服务端能力有限，无通用配额 |
| 7 | ilanzou（优享版蓝奏云） | 账密 | ✅ 已注册 | 包内单测 + mock/e2e | 无通用配额 |
| 8 | onedrive | OAuth PKCE | ✅ 已注册 | 包内单测 + mock/e2e | 真实 OAuth/服务未在本轮验证 |
| 9 | dropbox | OAuth PKCE | ✅ 已注册 | 包内单测 + mock/e2e | 真实 OAuth/服务未在本轮验证 |
| 10 | yike（一刻相册） | BDUSS / Cookie | ✅ 已注册 | mock/e2e | 不提供可靠配额，不参与跨盘秒传目标 |
| 11 | guangya（光鸭云盘） | 手机号 + 短信 / refresh_token | ✅ 已注册 | mock/e2e | 包内覆盖仍不足 |
| 12 | webdav | URL + 账密 | ✅ 已注册 | 本地 HTTP e2e | 仅 Basic Auth；配额取决于 RFC 4331 支持 |
| 13 | s3 | endpoint + AK/SK | ✅ 已注册 | mock/e2e | 无通用总容量接口；连接校验不证明写权限 |

“已注册/已实现”只表示统一驱动入口和对应代码路径存在，不代表所有真实服务商版本、权限模型和风控条件均已验证；可靠性应以测试范围和已知限制判断，不使用主观完成度百分比。

---

## 二、基础功能矩阵

### 2.1 登录与鉴权

| Provider | 账密 | OAuth PKCE | Cookie/Token | 短信验证码 | 验证码挑战 | Token 自动刷新 | RefreshAccount | 配额刷新 |
|----------|:----:|:----------:|:------------:|:----------:|:----------:|:--------------:|:--------------:|:--------:|
| pikpak | ✅ | ➖ | refresh_token | ➖ | ✅ | ✅ | ✅ | ✅ |
| aliopen | ➖ | ➖ | refresh_token | ➖ | ➖ | ✅ | ✅ | ✅ |
| pan123 | ✅ | ➖ | refresh_token | ➖ | ➖ | ✅(401重登) | ✅ | ✅ |
| pan189 | ✅ | ➖ | session | ➖ | ✅ | ✅ | ✅ | ✅ |
| pan139 | ✅ | ➖ | Authorization | ➖ | ➖ | ✅ | ✅ | ➖ |
| lanzou | ✅ | ➖ | ✅ | ➖ | ➖ | ✅(内联) | ✅ | ➖ |
| ilanzou | ✅ | ➖ | session | ➖ | ➖ | ✅(内联) | ✅ | ➖ |
| onedrive | ➖ | ✅ | refresh_token | ➖ | ➖ | ✅ | ✅ | ✅ |
| dropbox | ➖ | ✅ | refresh_token | ➖ | ➖ | ✅ | ✅ | ✅ |
| yike | ➖ | ➖ | BDUSS | ➖ | ➖ | ➖ | ✅(无配额) | ❌ |
| guangya | ➖ | ➖ | refresh_token | ✅ | ➖ | ✅ | ✅ | ✅ |
| webdav | ➖ | ➖ | 账密 | ➖ | ➖ | ➖ | ✅(可选配额) | ✅(RFC 4331) |
| s3 | ➖ | ➖ | AK/SK | ➖ | ➖ | ➖ | ➖ | ➖ |

> ✅ onedrive/dropbox 的 `RefreshAccount` 已实现：token 自动刷新 + 账号信息/配额拉取。

---

### 2.2 文件列表与搜索

| Provider | List | ListPaged | 游标分页 | 分页防环 | Search(云端) | 本地搜索索引 |
|----------|:----:|:---------:|:--------:|:--------:|:------------:|:------------:|
| pikpak | ✅ | ✅ | ✅ | ✅ | ❌(设计) | 由 ops 兜底 |
| aliopen | ✅ | ✅ | ✅ | ✅ | ✅ | — |
| pan123 | ✅ | ✅ | ❌(数字页码) | ✅ | ✅ | — |
| pan189 | ✅ | ✅ | ✅ | ➖ | ❌(设计) | — |
| pan139 | ✅ | ✅ | ✅ | ❌ | ❌(设计) | — |
| lanzou | ✅ | ➖ | ➖ | ➖ | ❌(设计) | — |
| ilanzou | ✅ | ➖ | ➖ | ➖ | ❌(设计) | — |
| onedrive | ✅ | ✅ | ✅ | ✅ | ✅ | — |
| dropbox | ✅ | ✅ | ✅ | ✅ | ✅ | — |
| yike | ✅ | ➖ | ✅ | ➖ | ❌(设计) | — |
| guangya | ✅ | ➖ | page翻页 | ➖ | ❌(设计) | — |
| webdav | ✅ | ➖ | ➖ | ➖ | ❌(设计) | — |
| s3 | ✅ | ✅ | ✅ | ✅ | ❌(设计) | — |

---

### 2.3 下载与视频预览

| Provider | GetDownloadURL | 下载模式 | 并发 | 视频预览 | 转码清晰度 | VIP/会员检测 | 链接过期检测 |
|----------|:--------------:|:--------:|:----:|:--------:|:----------:|:------------:|:------------:|
| pikpak | ✅ | redirect | ➖ | ✅ | ✅ | ✅(10m缓存) | ✅(提前60s) |
| aliopen | ✅ | redirect | ➖ | ✅(原画/Live Photo流) | ❌(设计未接转码) | ➖ | ✅(API expiration) |
| pan123 | ✅ | proxy | 1 | ✅ | ➖ | ➖ | ✅ |
| pan189 | ✅ | proxy | ➖ | ✅(伪预览) | ❌ | ➖ | ✅ |
| pan139 | ✅ | proxy | 1 | ✅ | ➖ | ➖ | ➖ |
| lanzou | ✅ | proxy | 1 | ✅ | ➖ | ➖ | ➖ |
| ilanzou | ✅ | proxy | 1 | ✅ | ➖ | ➖ | ➖ |
| onedrive | ✅ | redirect | ➖ | ✅(新增) | ➖ | ➖ | ➖ |
| dropbox | ✅ | redirect | ➖ | ✅(新增) | ➖ | ➖ | ✅(4h) |
| yike | ✅ | proxy | ➖ | ➖ | ➖ | ➖ | ➖ |
| guangya | ✅ | proxy | ➖ | ✅ | ➖ | ➖ | ➖ |
| webdav | ✅ | redirect | ➖ | ➖ | ➖ | ➖ | ➖ |
| s3 | ✅ | redirect | ➖ | ✅(原画/网页播放器) | ➖ | ➖ | ✅(4h预签名) |

---

### 2.4 上传

| Provider | UploadOneFile | 分片上传 | 整包上传 | 断点续传 | 秒传 | 冲突策略 | 进度回调 | 上传模式 |
|----------|:-------------:|:--------:|:--------:|:--------:|:----:|:--------:|:--------:|:--------:|
| pikpak | ✅ | ➖ | ✅(OSS PUT) | ✅ | ✅(GCID) | ✅(refuse/skip/rename/overwrite) | ✅ | queue |
| aliopen | ✅ | ✅(动态20MiB-5GiB) | ➖ | ✅ | ✅(SHA1/pre_hash) | ✅(refuse/rename/skip/overwrite) | ✅ | queue |
| pan123 | ✅ | ✅(16MB) | ➖ | ✅ | ✅(MD5) | ✅(1/2 映射) | ✅ | queue |
| pan189 | ✅ | ✅(10/20MB) | ➖ | ✅ | ✅(MD5) | ➖ | ✅ | queue |
| pan139 | ✅ | ✅(100/200MB预签名) | ➖ | ✅ | ✅(SHA-256) | ⚠️(服务端auto_rename) | ✅ | queue |
| lanzou | ✅ | ➖ | ✅(≤200MB) | ➖ | ➖ | ➖ | ➖ | queue |
| ilanzou | ✅ | ✅(8MB) | ✅(≤8MB) | ➖ | ✅(MD5) | ➖ | ✅ | queue |
| onedrive | ✅ | ✅(10MB session) | ✅(≤4MB) | ✅ | ➖ | ❌(固定rename) | ✅ | queue |
| dropbox | ✅ | ✅(8MB session) | ✅(≤150MB) | ➖ | ➖ | ❌(固定add) | ✅ | queue |
| yike | ✅ | ✅(4MB) | ➖ | ➖ | ✅ | ➖ | ✅ | queue |
| guangya | ✅ | ✅(OSS multipart) | ➖ | ➖ | ✅ | ➖ | ✅ | queue |
| webdav | ✅ | ➖ | ✅(PUT) | ➖ | ➖ | ✅ | ✅ | direct |
| s3 | ✅ | ✅(16MB multipart) | ✅(<64MiB PUT) | ➖ | ➖ | ✅ | ✅ | direct |

> ✅ aliopen/pan123/pan189/onedrive 已持久化上传会话；Dropbox 也保存远端 session 与已确认偏移。
> ✅ webdav/s3 的冲突策略与进度回调已实现（ConflictPolicy + ProgressReader）；S3 64MiB 以上上传自动使用 multipart，重名策略包含 `skip`。

---

### 2.5 文件操作

| Provider | Mkdir | Rename | Move | Copy | 批量任务轮询 |
|----------|:-----:|:------:|:----:|:----:|:------------:|
| pikpak | ✅ | ✅ | ✅ | ✅ | ✅(waitForTasks) |
| aliopen | ✅ | ✅ | ✅ | ✅ | ➖ |
| pan123 | ✅ | ✅ | ✅ | ❌(设计) | ➖ |
| pan189 | ✅ | ✅ | ✅ | ✅ | ✅ |
| pan139 | ✅ | ✅ | ✅ | ✅ | ➖ |
| lanzou | ✅ | ✅(文件) | ✅(仅文件) | ❌(设计) | ➖ |
| ilanzou | ✅ | ✅ | ✅ | ❌(设计) | ➖ |
| onedrive | ✅ | ✅ | ✅ | ✅ | ✅(monitor轮询) |
| dropbox | ✅ | ✅ | ✅ | ✅ | ➖ |
| yike | ✅(建相册) | ✅(仅相册) | ❌(设计) | ❌(设计) | ➖ |
| guangya | ✅ | ✅ | ✅ | ✅ | ➖ |
| webdav | ✅ | ✅ | ✅ | ✅ | ➖ |
| s3 | ✅ | ✅ | ✅ | ✅ | ➖ |

> ✅ s3 的 Rename/Move/Copy/Delete 均已实现递归处理（listAllUnder + 批量 DeleteObjects + copyRecursive）；大对象复制使用 multipart copy，CopySource 按路径段编码。

---

### 2.6 回收站

| Provider | Trash(移入) | Delete(永久) | Restore(恢复) | ListTrash(查看) | TrashPurge(清空) | TrashClear |
|----------|:-----------:|:------------:|:-------------:|:---------------:|:----------------:|:----------:|
| pikpak | ✅ | ✅ | ✅ | ✅ | ❌(声明已移除) | ❌(声明已移除) |
| aliopen | ✅ | ✅ | ❌(设计) | ❌(设计) | ➖ | ➖ |
| pan123 | ✅ | ✅ | ✅ | ✅ | ❌(声明已移除) | ❌(声明已移除) |
| pan189 | ✅ | ✅(清空回收站) | ❌**缺** | ❌(设计禁用) | ✅(Delete内联) | ✅(Delete内联) |
| pan139 | ✅ | ✅ | ➖(新版接口未验证) | ❌(设计) | ➖ | ➖ |
| lanzou | ➖ | ✅ | ➖ | ➖ | ➖ | ➖ |
| ilanzou | ➖ | ✅ | ➖ | ➖ | ➖ | ➖ |
| onedrive | ➖ | ✅(永久) | ➖ | ➖ | ➖ | ➖ |
| dropbox | ➖ | ✅ | ➖ | ➖ | ➖ | ➖ |
| yike | ➖ | ✅ | ➖ | ➖ | ➖ | ➖ |
| guangya | ➖ | ✅ | ➖ | ➖ | ➖ | ➖ |
| webdav | ➖ | ✅ | ➖ | ➖ | ➖ | ➖ |
| s3 | ➖ | ✅ | ➖ | ➖ | ➖ | ➖ |

> ✅ 能力声明已修正：pikpak/pan123 已将 trashPurge/trashClear 改为不声明。
> ⚠️ pan189 的 `recycleBin:true` 但只支持删除到回收站 + 清空回收站，无法查看/恢复。

---

### 2.7 分享

| Provider | CreateShare | 过期时间 | 密码 | 分享导入(转存) | 分享历史 |
|----------|:-----------:|:--------:|:----:|:--------------:|:--------:|
| pikpak | ✅ | ✅ | ✅ | ✅ | ✅ |
| aliopen | ✅ | ✅ | ✅ | ✅ | ✅ |
| pan123 | ✅ | ✅ | ✅ | ✅ | ✅ |
| pan189 | ❌(设计禁用) | ➖ | ➖ | ➖ | ➖ |
| pan139 | ❌(设计) | ➖ | ➖ | ➖ | ➖ |
| lanzou | ✅ | ➖ | ✅(API返回) | ➖ | ✅ |
| ilanzou | ❌(设计) | ➖ | ➖ | ➖ | ➖ |
| onedrive | ✅ | ✅ | ✅ | ➖ | ➖ |
| dropbox | ✅ | ✅ | ✅ | ➖ | ➖ |
| yike | ❌(设计) | ➖ | ➖ | ➖ | ➖ |
| guangya | ❌(设计) | ➖ | ➖ | ➖ | ➖ |
| webdav | ➖ | ➖ | ➖ | ➖ | ➖ |
| s3 | ➖ | ➖ | ➖ | ➖ | ➖ |

> ✅ pikpak/aliopen/pan123 三个盘均已实现分享导入（ImportShareSession + SaveShare）。
> ✅ 分享页已接入导入入口：仅展示声明 importShare 的账号，解析会话、文件选择和保存目录均绑定目标账号；不支持导入的网盘不会显示入口。
> ✅ dropbox 密码分享的 `requested_visibility` 已修复为 `password`。

---

### 2.8 云离线下载

| Provider | 能力声明 | 提交任务 | 任务列表 | 进度查询 | 任务删除 |
|----------|:--------:|:--------:|:--------:|:--------:|:--------:|
| pikpak | ✅ | ✅ | ✅ | ✅(RefreshOffline) | ✅(DeleteOffline) |
| 其余 12 盘 | ❌(设计) | ➖ | ➖ | ➖ | ➖ |

> ✅ pikpak 的离线下载已实现进度查询和任务删除（OfflineFind + OfflineDelete）；列表使用 `limit=10000`、phase 过滤和 `page_token` 分页，不会因固定 100 条上限漏查任务。

---

### 2.9 收藏与哈希

| Provider | Favorite | ProvideHashes | RapidUploadHashes | ResolveTransferHash |
|----------|:--------:|:-------------:|:-----------------:|:-------------------:|
| pikpak | ✅ | ❌(未声明) | ❌(未声明) | ➖ |
| aliopen | ➖ | ✅(sha1) | ✅(sha1) | ✅ |
| pan123 | ➖ | ✅(md5) | ✅(md5) | ✅ |
| pan189 | ➖ | ✅(md5) | ✅(md5) | ✅ |
| pan139 | ➖ | ✅(sha256) | ✅(sha256) | ✅ |
| lanzou | ➖ | ❌ | ❌ | ➖ |
| ilanzou | ➖ | ✅(md5) | ✅(md5) | ✅ |
| onedrive | ➖ | ✅(sha1/quickXor) | ➖ | ➖ |
| dropbox | ➖ | ✅(dropbox) | ➖ | ➖ |
| yike | ➖ | ✅(md5) | ➖ | ➖ |
| guangya | ➖ | ✅(md5) | ➖ | ✅ |
| webdav | ➖ | ➖ | ➖ | ➖ |
| s3 | ➖ | ➖ | ➖ | ➖ |

> ✅ ilanzou 已实现 `RapidUploadByHash` + `ResolveTransferHash`，并与 onedrive/dropbox 的哈希声明一起纳入跨盘秒传能力；pikpak 通过 GCID 实现秒传但未声明 hash 类型。

---

## 三、关键差距汇总（按优先级）

### 🔴 P0 — 影响功能正确性

| # | 差距 | 影响范围 | 状态 | 详情 |
|---|------|---------|:----:|------|
| 1 | onedrive/dropbox `RefreshAccount` 完全缺失 | 2 盘 | ✅已修复 | 已实现 token 刷新 + 账号信息/配额；缺省 `expires_in` 时保留旧值 |
| 2 | 分享导入 `importShare` 声明未实现 | 3 盘(pikpak/aliopen/pan123) | ✅已修复 | ShareImportDriver 接口 + 三个盘完整实现 |
| 3 | pikpak API captcha token 续接不完整 | 1 盘 | ✅已修复 | 按设备/账号/action 缓存 token，失败时 previousToken 换发并自动重试 |
| 4 | aliopen `CompleteUpload` 传空 `upload_id` | 1 盘 | ✅已修复 | 传真实 upload_id |
| 5 | s3 目录递归操作完全缺失 | 1 盘 | ✅已修复 | listAllUnder + 批量 DeleteObjects + copyRecursive |
| 6 | s3 `forcePathStyle` 硬编码不可配置 | 1 盘 | ✅已修复 | 改为 *bool 可配置，支持 sessionToken |

### 🟡 P1 — 影响健壮性

| # | 差距 | 影响范围 | 状态 | 详情 |
|---|------|---------|:----:|------|
| 7 | 上传断点续传持久化缺失 | 4 盘 | ✅已修复 | aliopen/pan123/pan189/onedrive 已实现 session 存储 |
| 8 | webdav/s3 上传冲突策略完全缺失 | 2 盘 | ✅已修复 | refuse/rename/overwrite + ConflictPolicy 字段 |
| 9 | 能力声明与实现不符 | 多盘 | ✅已修复 | pikpak/pan123 的 trashPurge/trashClear/playbackHistory 改为 false |
| 10 | 哈希能力声明缺失 | 4 盘 | ✅已修复 | ilanzou/onedrive/dropbox 已声明 SetHashes |
| 11 | pikpak 离线下载进度/删除缺失 | 1 盘 | ✅已修复 | RefreshOfflineTasks + DeleteOfflineTask 绑定 |
| 12 | pan139 ListPage 分页参数未推进 | 1 盘 | ✅已修复 | 改用新版 `pageInfo.pageCursor`，并防重复游标 |
| 13 | onedrive/dropbox 搜索无分页 | 2 盘 | ✅已修复 | 跟随 nextLink/cursor 分页 |
| 14 | pikpak batch 操作无任务轮询 | 1 盘 | ✅已修复 | waitForTasks 60s 超时轮询 |

### 🟢 P2 — 体验优化

| # | 差距 | 影响范围 | 详情 |
|---|------|---------|------|
| 15 | 限速/重试缺失 | 多盘 | aliopen 已覆盖并发限速、401 刷新和 429 退避；pikpak 已覆盖登录/API 429 冷却识别；Dropbox 已支持 429/Retry-After |
| 16 | 版本历史缺失 | 2 盘 | onedrive/dropbox 当前驱动未提供版本历史/恢复能力 |
| 17 | 缩略图缺失 | 1 盘 | dropbox 当前 `mapItem` 未填充缩略图 |
| 18 | 上传进度回调已实现 | 2 盘 | ✅ webdav/s3 已实现 ProgressReader + 令牌桶限速（`progress.go:22`） |
| 19 | yike decryptYikeMd5 | 1 盘 | ✅已实现；yike 按需求不参与跨盘秒传能力路由 |

---

## 四、文档导航

各网盘详细功能调研与 file:line 证据见：
- [docs/providers/pikpak.md](providers/pikpak.md)
- [docs/providers/aliopen.md](providers/aliopen.md)
- [docs/providers/pan123.md](providers/pan123.md)
- [docs/providers/pan189.md](providers/pan189.md)
- [docs/providers/pan139.md](providers/pan139.md)
- [docs/providers/lanzou.md](providers/lanzou.md)
- [docs/providers/ilanzou.md](providers/ilanzou.md)
- [docs/providers/onedrive.md](providers/onedrive.md)
- [docs/providers/dropbox.md](providers/dropbox.md)
- [docs/providers/yike.md](providers/yike.md)
- [docs/providers/guangya.md](providers/guangya.md)
- [docs/providers/webdav.md](providers/webdav.md)
- [docs/providers/s3.md](providers/s3.md)
