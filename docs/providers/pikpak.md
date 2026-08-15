# PikPak 网盘功能详情

> 调研范围：`internal/drive/providers/pikpak/`（auth.go, client.go, pikpak.go, gcid.go, oss.go）
> 对照旧版：`../Mnemo/src/pikpak/`（8 文件）+ `src/drive/providers/pikpak.ts`
> 整体完成度：⚠️ ~75%

---

## 能力声明（pikpak.go:35-50）

| 能力 | 值 | 实现 |
|------|:--:|------|
| download | true | ✅ |
| offlineDownload | true | ⚠️(缺进度/删除) |
| createShare / shareExpiration / sharePassword / combinedShare / shareHistory | true | ✅ / ✅ / ✅ / 声明 / 声明 |
| importShare | true | ❌(未实现) |
| favorite | true | ✅ |
| trashView / trashRestore / trashPurge / trashClear | true | ✅ / ✅ / ⚠️(未实现) / ⚠️(未实现) |
| playbackHistory | true | ❌(未实现) |
| recycleBin / permanentDelete | true | ✅ / ✅ |

---

## 1. 登录（auth.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| 账密登录 | ✅ | `auth.go:167-196` signIn POST `/v1/auth/signin` | 无 |
| 验证码 init | ✅ | `auth.go:99-134` initCaptcha `/v1/shield/captcha/init` | ⚠️ 旧版有退避重试链 `[600,1200,2000]ms`，新版单次链式 |
| captcha_sign | ✅ | `client.go:43-52` 16 段 salt MD5 链 | 无 |
| device_id 持久化 | ✅ | `auth.go:58-91` UUID v4 写文件 | 存储介质不同（文件 vs localStorage） |
| refresh_token 存储 | ✅ | `auth.go:198-234` authSignIn | 无 |
| token 刷新 | ✅ | `auth.go:237-260` refreshToken `/v1/auth/token` | 无 |
| **速率限制处理** | ❌ | 无 | 🔴 旧版 `PikPakRateLimitError` + 30s 冷却完全缺失 |
| **API captcha token 注入** | ⚠️ | `auth.go:204` 登录流程已用 X-Captcha-Token；业务 API 续接不完整 | 🟡 业务 API 遇到二次挑战后的完整续接体系仍不完整 |

---

## 2. 文件列表（client.go / pikpak.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| List 全量分页 | ✅ | `client.go:147-163` 循环 ListPage + seen 去重 | 无 |
| ListPaged 单页 | ✅ | `pikpak.go:46-55` → `client.go:136-146` page_size=100 | 无 |
| **phase 过滤** | ❌ | `client.go:139` 仅 `{"trashed":%t}` | 🟡 不过滤未完成文件，可能列出上传/转码中的文件 |

---

## 3. 下载（client.go / pikpak.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| 直链获取 | ✅ | `client.go:194-210` GET `/drive/v1/files/{id}/download?redirect=false` | 路径不同（旧版从 detail 取最佳链接） |
| DownloadMode | redirect | `pikpak.go:64-74` | 无 |
| **Referer** | ❌ | `client.go:75-82` headers 无 Referer | 🟡 旧版 buildHeaders 设了 Referer |
| **最佳链接选择** | ❌ | 直接用 download 端点 | 🟡 旧版 `getPikPakBestDownloadLink` 按 fid 取 origin media link |
| **链接过期检测** | ❌ | 无 | 🟡 旧版 `pikPakItemLinksExpiring` 提前 60s 检测 X-Amz 签名 |
| **detail 缓存** | ❌ | 无 | 🟡 旧版 45s TTL 缓存 |

---

## 4. 视频预览（client.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| 转码清晰度 | ✅ | `client.go:223-272` PlayInfo GET `/drive/v1/files/{id}/video/play_info` | 路径不同（旧版从 detail medias 解析） |
| VIP 检测 | ✅ | `client.go:214-221` VipInfo `/drive/v1/privilege/vip` identity>0 | 🟡 无缓存，每次播放都请求 |
| 非会员 720p 限制 | ✅ | `client.go:252` height>720 跳过 | 无 |
| 原画流 | ✅ | `client.go:258-269` web_content_link / medias[].is_origin, ForceProxy | 无 |
| 清晰度 tier | ✅ | `client.go:278-290` QHD/FHD/HD/SD/LD | 无 |
| **stream type** | ⚠️ | `client.go:292-299` m3u8/dash/mp4 | 🟡 不检测 ts/mpegts |
| **duration** | ❌ | model.VideoPreview 未设 | 🟡 旧版从 medias/params 取 |
| **分辨率 fallback 解析** | ❌ | 仅依赖 tc.Height | 🟡 旧版正则解析 resolution_name |

---

## 5. 上传（gcid.go / oss.go / pikpak.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| GCID 计算 | ✅ | `gcid.go:23-55` 分块 SHA1 → 拼接 → SHA1 → 大写 hex | 无 |
| 分片大小策略 | ✅ | `gcid.go:10-20` 4 档 | 无 |
| 创建上传任务 | ✅ | `pikpak.go:295-322` POST `/drive/v1/files` RESUMABLE + PROVIDER_ALIYUN | 无 |
| 秒传 | ✅ | `pikpak.go:325-327` res.Resumable==nil 时完成 | 无 |
| OSS PUT | ✅ | `oss.go:18-89` HMAC-SHA1 签名流式 PUT | 无 |
| **同名冲突处理** | ❌ | 无 | 🟡 旧版 `resolveProviderUploadConflict` 分页查找 + trash |
| **上传失败清理** | ❌ | 无 | 🟡 旧版上传失败时 trashDelete 清理残留 |
| **实时进度** | ⚠️ | `oss.go:82-85` 仅完成时设 100 | 🟡 旧版有实时进度 |

---

## 6. 文件操作（client.go）

| 操作 | 状态 | Go 证据 | 差距 |
|------|:----:|---------|------|
| Mkdir | ✅ | `client.go:305-318` POST `/files:create_folder` | 端点不同（旧版裸 /files POST） |
| Rename | ✅ | `client.go:330-333` POST `/files:batch_rename` | 端点不同（旧版 PATCH 单文件） |
| Move | ✅ | `client.go:345-348` POST `/files:batch_move` requests 包裹 | body 结构不同（旧版 to 包裹） |
| Copy | ✅ | `client.go:353-356` POST `/files:batch_copy` | 同 Move |
| **任务轮询等待** | ❌ | 无 | 🟡 旧版 `waitForPikPakTask` 60×1s 轮询，新版不等待完成 |

---

## 7. 回收站（client.go / pikpak.go）

| 操作 | 状态 | Go 证据 | 差距 |
|------|:----:|---------|------|
| Trash | ✅ | `client.go:338` POST `/files:batch_trash` | 无 |
| Delete | ✅ | `client.go:342` POST `/files:batch_delete` | 无 |
| Restore | ✅ | `client.go:346` POST `/files:batch_restore` | 端点不同（旧版 batchUntrash） |
| ListTrash | ✅ | `pikpak.go:58-63` listPages(trashed=true) | ⚠️ filters `{"trashed":true}` 旧版 `{"trashed":{"eq":true},"parent_id":"*"}`，可能列不正确 |
| TrashPurge | ❌ | 声明 true 但无实现 | 🔴 能力声明与实现不符 |
| TrashClear | ❌ | 声明 true 但无实现 | 🔴 同上 |

---

## 8. 分享（client.go / pikpak.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| CreateShare | ✅ | `client.go:366-381` POST `/drive/v1/share` | body 结构差异大（旧版 share_to + expiration_days + pass_code_option） |
| 过期时间 | ✅ | 直传 expiration 字符串 | 旧版转 expiration_days 整数 |
| 密码 | ✅ | 直传 pass_code | 无 |
| **分享导入 importShare** | ❌ | 声明 true 但无实现 | 🔴 旧版 3 步：getShareToken → listByShare → copy with share_token |

---

## 9. 云离线下载（client.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| 提交任务 | ✅ | `client.go:384-405` POST `/drive/v1/tasks?type=offline` | 端点+body 与旧版完全不同（旧版 /files + url 对象） |
| 任务列表 | ✅ | `client.go:560-570` GET `/drive/v1/tasks?type=offline&page_size=100` | ⚠️ 无 phase 过滤、无 reference_resource、page_size=100 vs 旧版 limit=10000 |
| **进度查询** | ❌ | 无 OfflineProcess | 🔴 旧版 `apiPikPakOfflineProcess` 按 taskId 查询 |
| **任务删除** | ❌ | 无 OfflineDelete | 🔴 旧版 DELETE `/tasks?task_ids=...` |
| 任务查找 | ✅ | `client.go:572-585` FindOfflineTask | 无 |

---

## 10. 收藏（client.go / pikpak.go）

| 状态 | Go 证据 | 差距 |
|:----:|---------|------|
| ✅ | `client.go:350` POST `/files:batch_star` starred 参数 | 端点不同（旧版 star/unstar 双端点） |

---

## 11. 搜索

❌ 两版均不支持（设计如此），由 ops 层本地搜索索引兜底。

---

## 12. ProvideHashes / RapidUploadHashes

❌ 能力声明未调用 SetHashes，ProvideHashes/RapidUploadHashes 为空。
> 注：PikPak 实际通过 UploadOneFile 中 GCID 匹配实现秒传（`pikpak.go:325-327`），但未暴露为独立 RapidUploadByHash 接口，也未声明 hash 类型。

---

## 13. RefreshAccount（pikpak.go）

✅ `pikpak.go:211-228` 调 refreshToken + About 刷新配额。功能对齐。

---

## P0 差距清单

1. 🔴 **API captcha token 续接不完整**：登录流程已用 X-Captcha-Token(`auth.go:204`)，但业务 API 遇到二次挑战后的完整续接体系仍不完整
2. 🔴 **分享导入**：声明 `importShare:true` 但 3 个 API 完全未迁移
3. 🔴 **离线下载进度/删除**：只实现创建和列表
4. 🟡 **TrashPurge/TrashClear**：声明 true 但无实现
5. 🟡 **PlaybackHistory**：声明 true 但无实现
