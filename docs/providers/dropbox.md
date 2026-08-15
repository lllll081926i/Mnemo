# Dropbox 功能详情

> 调研范围：`internal/drive/providers/dropbox/`（onedrive.go 驱动, auth.go, dropbox.go 客户端, util.go）
> 对照旧版：`../Mnemo/src/dropbox/` + `src/drive/providers/dropbox.ts`
> 整体完成度：⚠️ ~70%（与 OneDrive 共性问题：RefreshAccount 缺失）

---

## 能力声明（dropbox.go:32-39）

```
createShare, shareExpiration, sharePassword: true
```
> ProvideHashes 未声明（但实际映射了 content_hash）

---

## 1. 登录（auth.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| OAuth PKCE S256 | ✅ | `auth.go:27-57` 始终用 PKCE | 🟡 旧版条件性 PKCE（有 secret 时不传 challenge） |
| 本地回调 | ✅ | `auth.go:59-115` net.Listen + state 校验 | Go 版内置 |
| Token 交换 | ✅ | `auth.go:74-96` POST dbTokenURL code_verifier | 🟡 旧版有 secret 时用 client_secret |
| token_access_type=offline | ✅ | `auth.go:45` | 无 |
| app_key | ✅ | `auth.go:30-34` req.Config 或 dropbox_app_key | 🟡 无内置 fallback |
| **Token 刷新** | ❌ | 无 RefreshAccount 重写 | 🔴 旧版 refreshDropboxAccessToken |
| **账号信息/配额** | ❌ | 不调 get_current_account / get_space_usage | 🔴 旧版 applyDropboxAccount |

---

## 2. 文件列表（dropbox.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| List / ListPaged | ✅ | `dropbox.go:108-146` /files/list_folder + /continue | 无 |
| cursor 防环 | ✅ | `dropbox.go:138-142` seen | 无 |
| deleted 过滤 | ✅ | `dropbox.go:120-127` filterDeleted | 无 |
| limit=500 | ✅ | `dropbox.go:122` | 无 |

---

## 3. 下载（onedrive.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| GetDownloadURL | ✅ | `onedrive.go:103-117` TemporaryLink /files/get_temporary_link | 无 |
| 过期时间 | ✅ | 4h | Go 版更完善 |
| Authorization | ✅ | Bearer | 无 |

---

## 4. 视频预览（onedrive.go）

✅ `onedrive.go:119-131` 复用 GetDownloadURL origin。Go 新增。

---

## 5. 上传（dropbox.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| 小文件 ≤150MB | ✅ | `dropbox.go:354-373` UploadSmall /files/upload | 🟡 旧版阈值 8MB（Go 版更接近 API 上限） |
| Session upload | ✅ | `dropbox.go:375-410` start + append_v2 + finish | 无 |
| chunk 8MB | ✅ | `dropbox.go:17` | 无 |
| **429 重试** | ❌ | 无 Retry-After 处理 | 🟡 旧版 uploadBufferWithRetry 3 次重试 + 指数退避 |
| **暂停支持** | ❌ | 无 | 🟡 旧版 IsRunning 监控 100ms |
| 冲突策略 | ⚠️ | 硬编码 mode:add autorename:true | 🟡 旧版 add/overwrite + autorename |
| **父目录解析** | ⚠️ | 直接用 ParentFileID 作路径 | 🟡 旧版 resolveDropboxUploadParentPath 调 get_metadata |

---

## 6. 文件操作（dropbox.go / onedrive.go）

| 操作 | 状态 | Go 证据 | 差距 |
|------|:----:|---------|------|
| Mkdir | ✅ | `dropbox.go:187-206` /files/create_folder_v2 autorename:true | 🟡 旧版 autorename:false |
| Rename | ✅ | `dropbox.go:209-219` move_v2 | 🟡 多一次 Detail 调用 |
| Move | ✅ | `dropbox.go:222-227` move_v2 | 🟡 缺 allow_shared_folder |
| Copy | ✅ | `dropbox.go:230-234` copy_v2 | 🟡 缺 allow_shared_folder |
| 路径解析 | ⚠️ | `dropbox.go:438-450` resolveCommandPath 简单处理 | 🟡 旧版从 description 提取 dropbox_path: |

---

## 7. 回收站

| 操作 | 状态 | 说明 |
|------|:----:|------|
| Trash | ✅ | `dropbox.go:216` /files/delete_v2 |
| Restore | ❌ | NotSupported |
| TrashView | ❌ | 未设 |

---

## 8. 分享（dropbox.go / onedrive.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| CreateShare | ✅ | `dropbox.go:255-289` /sharing/create_shared_link_with_settings | 无 |
| 过期/密码 | ✅ | settings.expires / link_password | 无 |
| 已存在链接处理 | ✅ | `dropbox.go:268-284` list_shared_links + modify | 无 |
| **visibility 逻辑** | ⚠️ | `dropbox.go:258` 固定 public，有密码时仍 public+link_password | 🔴 应为 password |
| **list 分页** | ❌ | 只取第一页 | 🟡 旧版循环 cursor |
| **settings_error 处理** | ❌ | 不区分 | 🟡 旧版友好提示 |

---

## 9. 搜索（dropbox.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| Search | ✅ | `dropbox.go:149-171` /files/search_v2 max_results:1000 | 无 |
| **搜索分页** | ❌ | 只取第一页 | 🟡 旧版跟 search/continue_v2 |
| **过滤器** | ❌ | 无 | 🟡 旧版 filterDropboxSearchResults |
| file_status | ❌ | 无 | 🟡 旧版 active |

---

## 10. RefreshAccount

❌ **完全缺失**。

---

## 11. ProvideHashes

⚠️ `dropbox.go:489-491` mapItem 提取 content_hash → ContentHash="dropbox"，但 Caps 未声明。

---

## 12. 缺失功能

| 功能 | 状态 | 说明 |
|------|:----:|------|
| 版本历史 revisions | ❌ | 旧版有 list_revisions / restore |
| 缩略图 thumbnail | ❌ | 旧版有 get_thumbnail_v2，新版 mapItem Thumbnail 始终空 |

---

## P0 差距清单

1. 🔴 **Token 刷新完全缺失**：RefreshAccount 未实现
2. 🔴 **账号信息/配额缺失**
3. 🔴 **分享 visibility 逻辑错误**：有密码时应为 password 而非 public
4. 🟡 上传无 429 重试/暂停支持
5. 🟡 上传冲突策略单一
6. 🟡 搜索无分页、无过滤器
7. 🟡 分享 list 无分页、settings_error 无友好提示
8. 🟡 move/copy 缺 allow_shared_folder
9. 🟡 版本历史、缩略图完全缺失
10. 🟡 ProvideHashes 未在 Caps 声明
