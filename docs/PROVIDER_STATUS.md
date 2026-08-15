# 网盘功能支持矩阵（Provider Status）

> 数据来源：对 `internal/drive/providers/*` 全量源码调研 + 旧版 `../Mnemo/src/drive/providers/*` 对照（2025-08）。
> 废弃说明：`gofile`、`gdrive` 已按需求移除；`encryption`（加密文件名/加密流）不再支持，相关能力位不再纳入矩阵。
> 图示：✅ 已实现 · ⚠️ 部分/有差距 · ❌ 缺失/不支持 · ➖ 设计上不适用

---

## 一、在役网盘（13 个）

| # | Provider | 登录方式 | 文件数 | Go 测试 | 整体完成度 |
|---|----------|---------|--------|---------|-----------|
| 1 | pikpak | 账密 + 验证码 | 5 | ❌ | ⚠️ ~75% |
| 2 | aliopen（阿里云盘） | refresh_token | 1 | ❌ | ⚠️ ~70% |
| 3 | pan123（123 云盘） | 账密 | 2 | ✅ | ⚠️ ~80% |
| 4 | pan189（天翼云盘） | 账密 + 验证码 | 11 | ✅ | ✅ ~95% |
| 5 | pan139（139 云盘） | 手机号/邮箱 + 密码 / Authorization | 1 | ❌ | ✅ ~90% |
| 6 | lanzou（蓝奏云） | Cookie / 账密 | 12 | ✅ | ✅ ~98% |
| 7 | ilanzou（优享版蓝奏云） | 账密 | 8 | ✅ | ✅ ~95% |
| 8 | onedrive | OAuth PKCE | 4 | ❌ | ⚠️ ~70% |
| 9 | dropbox | OAuth PKCE | 4 | ❌ | ⚠️ ~70% |
| 10 | yike（一刻相册） | BDUSS / Cookie | 2 | ❌ | ✅ ~90% |
| 11 | guangya（光鸭云盘） | 手机号 + 短信 / refresh_token | 2 | ❌ | ✅ ~98% |
| 12 | webdav | URL + 账密 | 1 | ✅(e2e) | ⚠️ ~75% |
| 13 | s3 | endpoint + AK/SK | 1 | ❌ | ⚠️ ~70% |

---

## 二、基础功能矩阵

### 2.1 登录与鉴权

| Provider | 账密 | OAuth PKCE | Cookie/Token | 短信验证码 | 验证码挑战 | Token 自动刷新 | RefreshAccount | 配额刷新 |
|----------|:----:|:----------:|:------------:|:----------:|:----------:|:--------------:|:--------------:|:--------:|
| pikpak | ✅ | ➖ | refresh_token | ➖ | ✅ | ✅ | ✅ | ✅ |
| aliopen | ➖ | ➖ | refresh_token | ➖ | ➖ | ✅ | ✅ | ✅ |
| pan123 | ✅ | ➖ | refresh_token | ➖ | ➖ | ✅(401重登) | ✅ | ✅ |
| pan189 | ✅ | ➖ | session | ➖ | ✅ | ✅ | ✅ | ✅ |
| pan139 | ✅ | ➖ | Authorization | ➖ | ➖ | ✅ | ✅ | ✅ |
| lanzou | ✅ | ➖ | ✅ | ➖ | ➖ | ✅(内联) | ✅ | ➖ |
| ilanzou | ✅ | ➖ | session | ➖ | ➖ | ✅(内联) | ✅ | ➖ |
| onedrive | ➖ | ✅ | refresh_token | ➖ | ➖ | ❌**缺失** | ❌**缺失** | ❌**缺失** |
| dropbox | ➖ | ✅ | refresh_token | ➖ | ➖ | ❌**缺失** | ❌**缺失** | ❌**缺失** |
| yike | ➖ | ➖ | BDUSS | ➖ | ➖ | ➖ | ✅(无配额) | ❌ |
| guangya | ➖ | ➖ | refresh_token | ✅ | ➖ | ✅ | ✅ | ✅ |
| webdav | ➖ | ➖ | 账密 | ➖ | ➖ | ➖ | ➖ | ➖ |
| s3 | ➖ | ➖ | AK/SK | ➖ | ➖ | ➖ | ➖ | ➖ |

> ⚠️ onedrive/dropbox 的 `RefreshAccount` 完全缺失，token 过期后无法自动续期，登录后也不拉取账号信息与配额。这是两个 OAuth 盘的共性问题。

---

### 2.2 文件列表与搜索

| Provider | List | ListPaged | 游标分页 | 分页防环 | Search(云端) | 本地搜索索引 |
|----------|:----:|:---------:|:--------:|:--------:|:------------:|:------------:|
| pikpak | ✅ | ✅ | ✅ | ✅ | ❌(设计) | 由 ops 兜底 |
| aliopen | ✅ | ✅ | ✅ | ❌**缺** | ✅ | — |
| pan123 | ✅ | ⚠️(marker未用) | ❌ | ❌ | ✅ | — |
| pan189 | ✅ | ✅ | ✅ | ➖ | ❌(设计) | — |
| pan139 | ✅ | ✅ | ⚠️(参数未推进) | ❌ | ❌(设计) | — |
| lanzou | ✅ | ➖ | ➖ | ➖ | ❌(设计) | — |
| ilanzou | ✅ | ➖ | ➖ | ➖ | ❌(设计) | — |
| onedrive | ✅ | ✅ | ✅ | ✅ | ⚠️(无分页) | — |
| dropbox | ✅ | ✅ | ✅ | ✅ | ⚠️(无分页) | — |
| yike | ✅ | ➖ | ✅ | ➖ | ❌(设计) | — |
| guangya | ✅ | ➖ | page翻页 | ➖ | ❌(设计) | — |
| webdav | ✅ | ➖ | ➖ | ➖ | ❌(设计) | — |
| s3 | ✅ | ✅ | ✅ | ❌**缺** | ❌(设计) | — |

---

### 2.3 下载与视频预览

| Provider | GetDownloadURL | 下载模式 | 并发 | 视频预览 | 转码清晰度 | VIP/会员检测 | 链接过期检测 |
|----------|:--------------:|:--------:|:----:|:--------:|:----------:|:------------:|:------------:|
| pikpak | ✅ | redirect | ➖ | ✅ | ✅ | ✅(无缓存) | ❌**缺** |
| aliopen | ✅ | redirect | ➖ | ⚠️(仅原画) | ❌ | ➖ | ➖ |
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
| s3 | ✅ | redirect | ➖ | ➖ | ➖ | ➖ | ✅(4h预签名) |

---

### 2.4 上传

| Provider | UploadOneFile | 分片上传 | 整包上传 | 断点续传 | 秒传 | 冲突策略 | 进度回调 | 上传模式 |
|----------|:-------------:|:--------:|:--------:|:--------:|:----:|:--------:|:--------:|:--------:|
| pikpak | ✅ | ➖ | ✅(OSS PUT) | ❌ | ✅(GCID) | ❌**缺** | ⚠️(仅完成) | queue |
| aliopen | ✅ | ✅(固定10MB) | ➖ | ❌**缺** | ✅(SHA1) | ❌(固定ignore) | ✅ | queue |
| pan123 | ✅ | ✅(16MB) | ➖ | ❌**缺** | ✅(MD5) | ⚠️(duplicate) | ✅ | queue |
| pan189 | ✅ | ✅(10/20MB) | ➖ | ❌**缺** | ✅(MD5) | ➖ | ✅ | queue |
| pan139 | ✅ | ✅(5MB base64) | ➖ | ➖ | ➖ | ➖ | ✅ | queue |
| lanzou | ✅ | ➖ | ✅(≤200MB) | ➖ | ➖ | ➖ | ➖ | queue |
| ilanzou | ✅ | ✅(8MB) | ✅(≤8MB) | ➖ | ✅(MD5) | ➖ | ✅ | queue |
| onedrive | ✅ | ✅(10MB session) | ✅(≤4MB) | ❌**缺** | ➖ | ❌(固定rename) | ✅ | queue |
| dropbox | ✅ | ✅(8MB session) | ✅(≤150MB) | ➖ | ➖ | ❌(固定add) | ✅ | queue |
| yike | ✅ | ✅(4MB) | ➖ | ➖ | ✅ | ➖ | ✅ | queue |
| guangya | ✅ | ✅(OSS multipart) | ➖ | ➖ | ✅ | ➖ | ✅ | queue |
| webdav | ✅ | ➖ | ✅(PUT) | ➖ | ➖ | ❌**缺** | ❌**缺** | direct |
| s3 | ✅ | ❌**缺** | ✅(PUT) | ➖ | ➖ | ❌**缺** | ❌**缺** | direct |

> ⚠️ 多个 queue 盘缺失断点续传持久化（aliopen/pan123/pan189/onedrive），进程重启后大文件无法恢复。
> ⚠️ webdav/s3 两个 direct 盘完全缺失冲突策略，上传直接覆盖同名文件。

---

### 2.5 文件操作

| Provider | Mkdir | Rename | Move | Copy | 批量任务轮询 |
|----------|:-----:|:------:|:----:|:----:|:------------:|
| pikpak | ✅ | ✅ | ✅ | ✅ | ❌**缺** |
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
| s3 | ✅ | ⚠️(不递归目录) | ⚠️(不递归) | ⚠️(不递归) | ➖ |

> ⚠️ s3 的 Rename/Move/Copy/Delete 仅处理单对象，不递归目录子对象；旧版会先 `listAllS3Objects` 再批量操作。

---

### 2.6 回收站

| Provider | Trash(移入) | Delete(永久) | Restore(恢复) | ListTrash(查看) | TrashPurge(清空) | TrashClear |
|----------|:-----------:|:------------:|:-------------:|:---------------:|:----------------:|:----------:|
| pikpak | ✅ | ✅ | ✅ | ✅ | ⚠️(声明未实现) | ⚠️(声明未实现) |
| aliopen | ✅ | ✅ | ❌(设计) | ❌(设计) | ➖ | ➖ |
| pan123 | ✅ | ✅ | ✅ | ✅ | ⚠️(声明未实现) | ⚠️(声明未实现) |
| pan189 | ✅ | ✅(清空回收站) | ❌**缺** | ❌(设计禁用) | ✅(Delete内联) | ✅(Delete内联) |
| pan139 | ✅ | ✅ | ✅ | ❌(设计) | ➖ | ➖ |
| lanzou | ➖ | ✅ | ➖ | ➖ | ➖ | ➖ |
| ilanzou | ➖ | ✅ | ➖ | ➖ | ➖ | ➖ |
| onedrive | ➖ | ✅(永久) | ➖ | ➖ | ➖ | ➖ |
| dropbox | ➖ | ✅ | ➖ | ➖ | ➖ | ➖ |
| yike | ➖ | ✅ | ➖ | ➖ | ➖ | ➖ |
| guangya | ➖ | ✅ | ➖ | ➖ | ➖ | ➖ |
| webdav | ➖ | ✅ | ➖ | ➖ | ➖ | ➖ |
| s3 | ➖ | ✅ | ➖ | ➖ | ➖ | ➖ |

> ⚠️ 能力声明与实现不符：pikpak/pan123 声明了 `trashPurge:true`/`trashClear:true` 但 Driver 无对应方法。
> ⚠️ pan189 的 `recycleBin:true` 但只支持删除到回收站 + 清空回收站，无法查看/恢复。

---

### 2.7 分享

| Provider | CreateShare | 过期时间 | 密码 | 分享导入(转存) | 分享历史 |
|----------|:-----------:|:--------:|:----:|:--------------:|:--------:|
| pikpak | ✅ | ✅ | ✅ | ❌**缺**(声明true) | ✅ |
| aliopen | ✅ | ✅ | ✅ | ❌**缺**(声明true) | ✅ |
| pan123 | ✅ | ✅ | ✅ | ❌**缺**(声明true) | ✅ |
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

> ⚠️ pikpak/aliopen/pan123 三个盘声明了 `importShare:true` 但完全未实现分享导入逻辑，能力声明与实现不符。
> ⚠️ dropbox 密码分享的 `requested_visibility` 应为 `password` 而非 `public`（潜在 bug）。

---

### 2.8 云离线下载

| Provider | 能力声明 | 提交任务 | 任务列表 | 进度查询 | 任务删除 |
|----------|:--------:|:--------:|:--------:|:--------:|:--------:|
| pikpak | ✅ | ✅ | ✅ | ❌**缺** | ❌**缺** |
| 其余 12 盘 | ❌(设计) | ➖ | ➖ | ➖ | ➖ |

> ⚠️ pikpak 的离线下载只实现了创建和列表，缺进度查询和任务删除，列表 `page_size=100` 可能漏查大量任务。

---

### 2.9 收藏与哈希

| Provider | Favorite | ProvideHashes | RapidUploadHashes | ResolveTransferHash |
|----------|:--------:|:-------------:|:-----------------:|:-------------------:|
| pikpak | ✅ | ❌(未声明) | ❌(未声明) | ➖ |
| aliopen | ➖ | ✅(sha1) | ✅(sha1) | ✅ |
| pan123 | ➖ | ✅(md5) | ✅(md5) | ✅ |
| pan189 | ➖ | ✅(md5) | ✅(md5) | ✅ |
| pan139 | ➖ | ❌(未声明) | ➖ | ➖ |
| lanzou | ➖ | ❌ | ❌ | ➖ |
| ilanzou | ➖ | ✅(md5) | ✅(md5) | ➖ |
| onedrive | ➖ | ✅(sha1/quickXor) | ➖ | ➖ |
| dropbox | ➖ | ✅(dropbox) | ➖ | ➖ |
| yike | ➖ | ✅(md5) | ➖ | ➖ |
| guangya | ➖ | ✅(md5) | ➖ | ✅ |
| webdav | ➖ | ➖ | ➖ | ➖ |
| s3 | ➖ | ➖ | ➖ | ➖ |

> ⚠️ 多个盘实际计算了哈希但未在 Capabilities 声明，导致跨盘秒传路由无法识别：ilanzou(md5)、onedrive(sha1/quickXor)、dropbox(content_hash)、pikpak(GCID)。

---

## 三、关键差距汇总（按优先级）

### 🔴 P0 — 影响功能正确性

| # | 差距 | 影响范围 | 状态 | 详情 |
|---|------|---------|:----:|------|
| 1 | onedrive/dropbox `RefreshAccount` 完全缺失 | 2 盘 | ✅已修复 | 已实现 token 刷新 + 账号信息/配额 |
| 2 | 分享导入 `importShare` 声明未实现 | 3 盘(pikpak/aliopen/pan123) | ✅已修复 | ShareImportDriver 接口 + 三个盘完整实现 |
| 3 | pikpak API captcha token 续接不完整 | 1 盘 | ⚠️部分 | 登录已用 X-Captcha-Token，业务 API 二次挑战续接仍不完整 |
| 4 | aliopen `CompleteUpload` 传空 `upload_id` | 1 盘 | ✅已修复 | 传真实 upload_id |
| 5 | s3 目录递归操作完全缺失 | 1 盘 | ✅已修复 | listAllUnder + 批量 DeleteObjects + copyRecursive |
| 6 | s3 `forcePathStyle` 硬编码不可配置 | 1 盘 | ✅已修复 | 改为 *bool 可配置，支持 sessionToken |

### 🟡 P1 — 影响健壮性

| # | 差距 | 影响范围 | 状态 | 详情 |
|---|------|---------|:----:|------|
| 7 | 上传断点续传持久化缺失 | 4 盘 | ⚠️未修复 | aliopen/pan123/pan189/onedrive 进程重启后大文件无法恢复 |
| 8 | webdav/s3 上传冲突策略完全缺失 | 2 盘 | ✅已修复 | refuse/rename/overwrite + ConflictPolicy 字段 |
| 9 | 能力声明与实现不符 | 多盘 | ✅已修复 | pikpak/pan123 的 trashPurge/trashClear/playbackHistory 改为 false |
| 10 | 哈希能力声明缺失 | 4 盘 | ✅已修复 | ilanzou/onedrive/dropbox 已声明 SetHashes |
| 11 | pikpak 离线下载进度/删除缺失 | 1 盘 | ✅已修复 | RefreshOfflineTasks + DeleteOfflineTask 绑定 |
| 12 | pan139 ListPage 分页参数未推进 | 1 盘 | ✅已修复 | pageNum/startNumber 随 marker 递增 |
| 13 | onedrive/dropbox 搜索无分页 | 2 盘 | ⚠️未修复 | 只取第一页结果 |
| 14 | pikpak batch 操作无任务轮询 | 1 盘 | ⚠️未修复 | move/copy/trash 操作未等待完成就返回 |

### 🟢 P2 — 体验优化

| # | 差距 | 影响范围 | 详情 |
|---|------|---------|------|
| 15 | 限速/重试缺失 | 多盘 | aliopen 无并发限速、dropbox 上传无 429 重试、pikpak 无速率限制处理 |
| 16 | 版本历史缺失 | 2 盘 | onedrive/dropbox 旧版有 revisions，新版缺失 |
| 17 | 缩略图缺失 | 1 盘 | dropbox 旧版有 `get_thumbnail_v2`，新版缺失 |
| 18 | 上传进度回调缺失 | 2 盘 | webdav/s3 direct 上传无进度回调 |
| 19 | yike decryptYikeMd5 缺失 | 1 盘 | 旧版有 md5 解密，新版直接取字段，可能取到错误值 |

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
