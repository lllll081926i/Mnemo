# PikPak 网盘功能详情

> 调研范围：`internal/drive/providers/pikpak/`（auth.go, client.go, pikpak.go, gcid.go, oss.go）
> 当前证据：本仓库 `internal/drive/providers/pikpak`、统一能力注册表、本地自动化测试，以及 `D:/Code/Mnemo/Example/alist/drivers/pikpak` 的原版实现。
> 状态：✅ 统一驱动已注册；可靠性以自动验证范围和已知限制为准，见 [Provider 状态总表](../PROVIDER_STATUS.md)。

---

## 能力声明（auth.go:27-39）

| 能力 | 值 | 实现 |
|------|:--:|------|
| download | true | ✅ |
| offlineDownload | true | ✅ |
| createShare / shareExpiration / sharePassword / combinedShare / shareHistory | true | ✅ / ✅ / ✅ / 声明 / 声明 |
| importShare | true | ✅ `pikpak.go:390-475` ImportShareSession + `pikpak.go:477-500` SaveShare |
| favorite | true | ✅ |
| trashView / trashRestore / trashPurge / trashClear | true / true / — / — | ✅ / ✅ / 声明已移除 / 声明已移除 |
| playbackHistory | 未声明 | 声明已移除 |
| recycleBin / permanentDelete | true | ✅ / ✅ |
| ProvideHashes / RapidUploadHashes | gcid / gcid | ✅ 原生 GCID 元数据与服务端创建文件秒传 |

---

## 1. 登录（auth.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| 账密登录 | ✅ | `auth.go:167-196` signIn POST `/v1/auth/signin` | 无 |
| 验证码 init | ✅ | `auth.go:initCaptchaWithPrev` + `exchangeLoginCaptcha` `/v1/shield/captcha/init` | 已按旧版 `[600,1200,2000]ms` 链式换发，支持 context 取消 |
| captcha_sign | ✅ | `client.go:43-52` 16 段 salt MD5 链 | 无 |
| device_id 持久化 | ✅ | `auth.go:58-91` UUID v4 写文件 | 存储介质不同（文件 vs localStorage） |
| refresh_token 存储 | ✅ | `auth.go:198-234` authSignIn | 无 |
| token 刷新 | ✅ | `auth.go:237-260` refreshToken `/v1/auth/token` | 无 |
| **速率限制处理** | ✅ | `client.go:PikPakRateLimitError`，识别 429/too_many/rate_limit 和 `Retry-After` | 最短冷却 30 秒，并保留服务端更长冷却时间 |
| **API captcha token 注入** | ✅ | `auth.go:apiCaptchaToken` 按设备/账号/action 缓存并用 previousToken 换发；`client.go:jsonDo/get` 自动重试 | 无 |

---

## 2. 文件列表（client.go / pikpak.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| List 全量分页 | ✅ | `client.go:147-163` 循环 ListPage + seen 去重 | 无 |
| ListPaged 单页 | ✅ | `pikpak.go:58-68` → `client.go:416-437` limit=100 + page_token | 无 |
| **phase 过滤** | ✅ | `client.go:ListPage` 正常列表使用 `trashed.eq=false + phase.eq=PHASE_TYPE_COMPLETE` | 回收站使用 `parent_id=* + trashed.eq=true` |

---

## 3. 下载（client.go / pikpak.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| 直链获取 | ✅ | `client.go:DownloadURL` 优先从详情选择同 `fid` 的媒体链接，失败再请求 download 端点 | 无 |
| DownloadMode | redirect | `pikpak.go:64-74` | 无 |
| **Referer** | ✅ | `client.go:headers` / `auth.go:captchaHeaders` | 与旧版一致发送 `https://mypikpak.com/` 和 `X-Client-Id` |
| **最佳链接选择** | ✅ | `client.go:bestDownloadLink` 按同 `fid` 优先选择 origin media | 无 |
| **链接过期检测** | ✅ | `client.go:linksExpireSoon` 使用通用签名解析并提前 60 秒刷新 | 无 |
| **detail 缓存** | ✅ | `client.go:Detail` 按设备/账号/token 隔离，TTL 45 秒 | 无 |

---

## 4. 视频预览（client.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| 转码清晰度 | ✅ | `client.go:223-272` PlayInfo GET `/drive/v1/files/{id}/video/play_info` | 路径不同（旧版从 detail medias 解析） |
| VIP 检测 | ✅ | `client.go:VipInfo` `/drive/v1/privilege/vip` identity>0，按账号缓存 10 分钟 | 无 |
| 非会员 720p 限制 | ✅ | `client.go:252` height>720 跳过 | 无 |
| 原画流 | ✅ | `client.go:258-269` web_content_link / medias[].is_origin, ForceProxy | 无 |
| 清晰度 tier | ✅ | `client.go:278-290` QHD/FHD/HD/SD/LD | 无 |
| **stream type** | ✅ | `client.go:streamType` | 支持 m3u8/dash/ts(mpegts)/mp4 |
| **duration** | ✅ | `client.go:fileDurationSeconds` 从详情 duration、medias.video.duration、params.duration 取最大值 | 毫秒值自动转秒 |
| **分辨率 fallback 解析** | ✅ | `client.go:resolutionHeight` 解析 `resolution_name/media_name/template_id` | 无 |

---

## 5. 上传（gcid.go / oss.go / pikpak.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| GCID 计算 | ✅ | `gcid.go` 分块 SHA1 → 拼接 → SHA1 → 40 位大写 hex | 已修复将 hex 字符串再次 `%x` 编码成 80 位的错误 |
| 分片大小策略 | ✅ | `gcid.go:10-20` 4 档 | 无 |
| 创建上传任务 | ✅ | `pikpak.go:295-322` POST `/drive/v1/files` RESUMABLE + PROVIDER_ALIYUN | 无 |
| 普通上传内置秒传 | ✅ | 创建文件时提交 GCID；仅 `file.phase=PHASE_TYPE_COMPLETE` 判定服务端命中 | 模糊响应会清理预创建对象并报错，不误报完成 |
| 跨盘 GCID 秒传 | ✅ | `upload.go:RapidUploadByHash` 使用 `/drive/v1/files` 原生 GCID 创建协议 | 仅 `file.phase=PHASE_TYPE_COMPLETE` 判定命中；明确未命中时清理预创建文件再回退 |
| 迁移源 GCID | ✅ | 列表/详情的 `file.hash` 映射为 `ContentHashName=gcid`；`ResolveTransferHash` 可从详情补取 | 不为缺失 GCID 的文件额外下载全文件，避免一次迁移重复下载 |
| OSS PUT | ✅ | `oss.go:18-89` HMAC-SHA1 签名流式 PUT | 无 |
| **同名冲突处理** | ✅ | `pikpak.go:prepareUploadTarget` 分页查找，支持 refuse/skip/rename/overwrite | overwrite 移入回收站后再上传 |
| **上传失败清理** | ✅ | `pikpak.go:cleanupPikPakUpload` OSS 失败后永久删除已创建文件 | 清理失败会附加错误返回 |
| **实时进度** | ✅ | `oss.go:driveutil.NewProgressReader` 按实际读取字节回调 | 同时继承全局代理/上传限速 |

---

## 6. 文件操作（client.go）

| 操作 | 状态 | Go 证据 | 差距 |
|------|:----:|---------|------|
| Mkdir | ✅ | `client.go` POST `/drive/v1/files`，`kind=drive#folder` | 无 |
| Rename | ✅ | `client.go` PATCH `/drive/v1/files/{id}` | 无 |
| Move | ✅ | `client.go` POST `/drive/v1/files:batchMove`，body 为 `ids + to.parent_id` | 异步任务统一等待完成 |
| Copy | ✅ | `client.go` POST `/drive/v1/files:batchCopy`，body 为 `ids + to.parent_id` | 异步任务统一等待完成 |
| **任务轮询等待** | ✅ | `client.go:526-555` waitForTasks 60×1s 轮询 | 无 |

---

## 7. 回收站（client.go / pikpak.go）

| 操作 | 状态 | Go 证据 | 差距 |
|------|:----:|---------|------|
| Trash | ✅ | `client.go` POST `/drive/v1/files:batchTrash` | 异步任务统一等待完成 |
| Delete | ✅ | `client.go` POST `/drive/v1/files:batchDelete` | 异步任务统一等待完成 |
| Restore | ✅ | `client.go` POST `/drive/v1/files:batchUntrash` | 异步任务统一等待完成 |
| ListTrash | ✅ | `pikpak.go:79-85` → `client.go:416-437` parent_id=* + `trashed.eq=true` + limit 分页 | 无 |
| TrashPurge | — | 声明已移除（auth.go Caps 不再声明） | — |
| TrashClear | — | 声明已移除（auth.go Caps 不再声明） | — |

---

## 8. 分享（client.go / pikpak.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| CreateShare | ✅ | `client.go` POST `/drive/v1/share` | body 使用 `share_to + expiration_days + pass_code_option` |
| 过期时间 | ✅ | `pikpakExpirationDays` 将天数或 RFC3339 日期转换为整数 | 永久分享发送 `-1` |
| 密码 | ✅ | `pass_code_option=REQUIRED`，响应兼容 `pass_code/passcode` | 无 |
| **分享导入 importShare** | ✅ | `pikpak.go` `/drive/v1/share` + `/drive/v1/share/detail` 分页 + `/drive/v1/share/restore` | 支持 `/s/{id}`、`sharing/share/{id}` 和 `share_id` 查询参数 |

---

## 9. 云离线下载（client.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| 提交任务 | ✅ | `client.go` POST `/drive/v1/files`，`upload_type=UPLOAD_TYPE_URL`，`url={url}` | 兼容任务 ID、文件 ID 两种响应 |
| 任务列表 | ✅ | `client.go` GET `/drive/v1/tasks?type=offline&limit=10000` | phase 过滤、`reference_resource` 和 `page_token` 分页均已覆盖 |
| **进度查询** | ✅ | `pikpak.go` OfflineFind → FindOfflineTask，按 taskID/fileID 查找 | 进度兼容小数和百分比 |
| **任务删除** | ✅ | `pikpak.go` OfflineDelete DELETE `/drive/v1/tasks?task_ids=...&delete_files=...` | 明确传递是否删除已下载文件 |
| 任务查找 | ✅ | `client.go` FindOfflineTask | 任务字段兼容 `id/task_id`、`file/reference_resource` |

---

## 10. 收藏（client.go / pikpak.go）

| 状态 | Go 证据 | 差距 |
|:----:|---------|------|
| ✅ | `client.go` POST `/drive/v1/files:star` 或 `:unstar`，body 为 `ids` | 无 |

---

## 11. 搜索

❌ 两版均不支持（设计如此），由 ops 层本地搜索索引兜底。

---

## 12. ProvideHashes / RapidUploadHashes

✅ 声明 `SetHashes(["gcid"], ["gcid"])`。PikPak 可作为 GCID 迁移源和目标；只有源文件元数据能提供有效 40 位 GCID 时才尝试秒传。

`RapidUploadByHash` 与原版普通上传共用 `/drive/v1/files` 原生协议。若服务端返回 `resumable`，表示未命中；实现会先删除该预创建对象，再进入流式或落盘上传回退，避免遗留半成品和同名冲突。

不支持把 MD5/SHA1 猜测为 GCID，也不会为了生成 GCID 先下载一次、随后回退上传再下载一次。

---

## 13. RefreshAccount（pikpak.go）

✅ `pikpak.go:211-228` 调 refreshToken + About 刷新配额。功能对齐。

---

## P0 差距清单

1. ✅ **API captcha token 续接**：业务 API 遇到 captcha_required/captcha_invalid 时按 action 换发、缓存并带 `X-Captcha-Token` 重试
2. ✅ **分享导入**：已实现（`pikpak.go:390-500` ImportShareSession + SaveShare）
3. ✅ **离线下载进度/删除**：已实现（`pikpak.go:544-560` OfflineFind + OfflineDelete）
4. ✅ **TrashPurge/TrashClear**：声明已移除（auth.go Caps 不再声明）
5. ✅ **PlaybackHistory**：声明已移除
6. ✅ **任务轮询等待**：已实现（`client.go:526-555` waitForTasks）
