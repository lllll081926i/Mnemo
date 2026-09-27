# OneDrive 功能详情

> 调研范围：`internal/drive/providers/onedrive/`（onedrive.go, auth.go, graph.go, upload.go）
> 当前证据：本仓库 `internal/drive/providers/onedrive`、统一能力注册表和本地自动化测试；本次未读取父目录旧项目。
> 状态：✅ 统一驱动已注册；可靠性以自动验证范围和已知限制为准，见 [Provider 状态总表](../PROVIDER_STATUS.md)。

---

## 能力声明（onedrive.go:18-22）

```
favorite, search, createShare, shareExpiration, sharePassword, shareHistory: true
SetHashes(["sha1", "quickxorhash"], nil)
```
> trashView 未设；ProvideHashes 已声明；webEdit 已设，官网回收站入口实际指向 OneDrive 主页。

预览中的 DOCX/XLSX/PPTX 可从 Graph `webUrl` 在系统浏览器打开，由 OneDrive 官网提供编辑；PDF 仅可官网打开。浏览器需登录同一账号，应用不将修改后的二进制文档直接回传。Graph `/restore` 只对个人版已知文件 ID 可用；Graph 当前授权范围无法列出完整回收站，因此未在应用内显示虚假的可恢复列表。

`favorite` 是按账号类型启用的原生能力：工作/学校空间使用 Graph `/me/drive/following`、`/items/{id}/follow`、`/items/{id}/unfollow`；个人账号回退本地收藏。通过 `driveType` 识别并缓存类型，不将未知类型或请求失败当作可用。列表只保留当前挂载空间条目，支持分页、token 失效重试；关注操作必须返回对应文件 ID。依据：[Graph 关注权限](https://learn.microsoft.com/en-us/graph/api/driveitem-follow?view=graph-rest-1.0)、[取消关注](https://learn.microsoft.com/en-us/graph/api/driveitem-unfollow?view=graph-rest-1.0)。已通过模拟 HTTP 回归，未进行真实租户写入验证。

---

## 1. 登录（auth.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| OAuth PKCE S256 | ✅ | `auth.go` verifier+challenge | 包含 `prompt=select_account`，便于多账号登录 |
| 本地回调 | ✅ | `auth.go` 127.0.0.1:0 + state/来源校验 + 10min 超时 | Go 版内置回调服务器 |
| Token 交换 | ✅ | `auth.go` POST msTokenURL | 按实际凭据传递 `client_secret`，同时发送 `code_verifier` |
| client_id / secret | ✅ | 请求配置、release secrets 或内置 rclone 凭据 | 自定义 client_id 只在同时提供 secret 时使用 secret |
| **Token 刷新** | ✅ | `onedrive.go` 的 `RefreshAccount` 调 `refreshOneDriveToken` | 登录与刷新都会写入新的绝对 `expire_time` |
| **账号信息/配额** | ✅ | `onedrive.go:386-414` 获取 /me 和 /me/drive 配额 | 无 |

默认使用与旧版及 `Example/rclone` 一致的 OneDrive OAuth `client_id/client_secret`；发行版可通过 `secrets.json` 覆盖 client_id。登录后账号 ID 来自 Graph `/me`，默认 drive ID 使用 `onedrive:<drive-id>` 命名空间，避免多账号串用。

---

## 2. 文件列表（graph.go / onedrive.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| List / ListPaged | ✅ | `graph.go:282-314` + `onedrive.go:56-76` | 无 |
| $select 字段 | ✅ | `graph.go:270` downloadUrl/hashes/thumbnails | 无 |
| $expand=thumbnails | ✅ | `graph.go:272` | 无 |
| 分页防环 | ✅ | `graph.go:306-310` seen map | 无 |
| 可信 URL 校验 | ✅ | `graph.go:50-58` trustedGraphURL 含 sharepoint/1drv | Go 版更严格 |

---

## 3. 下载（onedrive.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| GetDownloadURL | ✅ | `onedrive.go:96-113` cl.Detail + DownloadURL | 🟡 多一次 Detail 调用（旧版从列表 item 取） |
| URL 来源 | ✅ | `graph.go:404-412` @microsoft.graph.downloadUrl → @content.downloadUrl → /content | Go 版多了 fallback |
| Authorization | ✅ | `onedrive.go:108-110` Bearer | 无 |
| 文件夹拒绝 | ✅ | `onedrive.go:102-104` | Go 版更健壮 |

---

## 4. 视频预览（onedrive.go）

✅ `onedrive.go:115-127` 复用 GetDownloadURL origin。Go 新增（旧版无独立模块）。

---

## 5. 上传（upload.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| 小文件 PUT ≤4MB | ✅ | `upload.go:43-56` rawPut /content | 无 |
| 大文件 session | ✅ | `upload.go:59-110` CreateUploadSession + 10MB chunked PUT + Content-Range | 无 |
| **断点续传** | ✅ | `upload.go:65-128` UploadSessionKey + LoadUploadSession + SaveUploadSession + querySessionPosition 恢复 | 会话键包含文件指纹与冲突策略，避免同名同大小文件串会话 |
| **416 处理** | ✅ | `upload.go:67-72` querySessionPosition 查询实际位置跳过已传 | 无 |
| **会话取消** | ✅ | `upload.go:128` ClearUploadSession | 无 |
| 冲突策略 | ✅ | `onedrive.go:319-347` 映射 refuse/skip/rename/overwrite；Graph 冲突码可识别 | 无 |
| **ignore/skip 模式** | ✅ | Graph `nameAlreadyExists`/同名错误返回成功跳过 | 与统一 `ConflictPolicy=skip` 对齐 |

---

## 6. 文件操作（graph.go / onedrive.go）

| 操作 | 状态 | Go 证据 | 差距 |
|------|:----:|---------|------|
| Mkdir | ✅ | `graph.go:249-261` /children conflictBehavior:rename | 无 |
| Rename | ✅ | `graph.go:264-272` PATCH name | 无 |
| Move | ✅ | `graph.go:281-284` PATCH parentReference | 无 |
| Copy | ✅ | `graph.go:287-340` /copy respond-async + monitor 轮询 10min | 无 |
| Copy 状态分类 | ✅ | `graph.go:342-375` 303/completed/failed | 🟡 旧版状态更细 |

---

## 7. 回收站

| 操作 | 状态 | 说明 |
|------|:----:|------|
| Trash | ✅ | Graph DELETE（移入回收站） |
| Restore | ⚠️ | Graph `/restore`，仅个人账号且需已知文件 ID |
| TrashView | ❌ | 普通 OneDrive 无已核实的 Graph 列表接口；提供官网入口 |

Graph 的 `fileStorageContainer` 回收站列表只面向 SharePoint Embedded，不能用普通 OneDrive 授权替代。工作/学校账号虽有 SharePoint REST 回收站接口，但现有 Graph 访问令牌的受众不是 SharePoint，不能直接复用；不在未增加独立授权的情况下静默调用。

### 第三方实现复核（2026-09-24）

- [SimpleList 的 `ListTrashAsync`](https://github.com/aiguoli/SimpleList/blob/main/SimpleList.Core/Services/OneDriveStorageProvider.cs#L213) 在 `/drives/{driveId}/root/children` 加 `includeDeletedItems=true`，再筛选 `deleted` 字段；其 `EmptyTrashAsync` 依赖这一列表。微软的 [Get driveItem 文档](https://learn.microsoft.com/en-us/graph/api/driveitem-get?view=graph-rest-1.0) 明确限定：该参数仅在**按 ID 获取 driveItem**时有效，其他目标会被忽略，且仅支持 OneDrive Personal。因此不能把该列表方案当作可靠的完整回收站接口，也不能照搬其清空逻辑。
- SimpleList 的 `SupportsTrash=true` 是无条件声明，源码没有回收站列表失败时的其他枚举路径；`ListTrashAsync` 也没有在 `SimpleList.Tests` 中得到实际回收站列表测试覆盖。本机同账号非空回收站的只读探测返回 0 条，故当前项目不能照其能力声明开放“应用内完整回收站”。
- 用户已在回收站添加文件后，本机唯一已登录的 OneDrive 个人账号只读探测仍返回“已删容量 0、普通根目录条目 5、`deleted` 条目 0”。用户提供的网页请求路径中的个人站点标识与本机账号标识后四位一致，排除了此前怀疑的账号不一致；Graph 的 `root/children` 路径确实未列出该账号已删项。不能仅凭 `quota.deleted=0` 断言官网回收站为空。诊断测试需显式设置 `MNEMO_LIVE_ONEDRIVE_READONLY=1` 才运行，不会删除或恢复文件。
- 用户复制的网页请求是 `SP.Web.GetContextWebThemeData`，用于获取站点主题信息，不是回收站数据接口；请求携带登录 Cookie，不能放进源码、日志或诊断材料，也不能作为应用鉴权方案。
- SimpleList 还实现了 `GET /items/{id}/content?format=pdf` 的 Office 文档转 PDF，以及独立的 `/items/{id}/thumbnails` 缩略图获取；前者可作为高保真文档预览的候选，但需考虑格式支持、响应重定向和缓存。其 `/items/{id}/permanentDelete` 是永久删除，不能因示例有此方法就自动提供回收站“清空”按钮。
- [CuteOne](https://github.com/Hackxiaoya/CuteOne) 的 `app/admin/drive/logic.py` 实现了 Graph 文件列表、新建、重命名和删除，但未找到回收站列表或恢复协议，不能作为回收站实现依据。
- [Graph beta 的 SharePoint 站点回收站列表](https://learn.microsoft.com/en-us/graph/api/recyclebin-list-items?view=graph-rest-beta) 提供 `GET /sites/{siteId}/recycleBin/items`，可作为工作/学校账号的**候选只读方案**；现有授权的 `Files.ReadWrite` 高于文档所列的 `Files.Read` 最低权限，但本机只有个人账号，尚未用企业租户验证 OneDrive 个人站点是否能列出。返回的 `recycleBinItem.id` 是删除事务 ID，不是可直接传给个人版 `/restore` 的 driveItem ID；Graph beta 的对应恢复文档只覆盖 SharePoint Embedded 容器，不能据此宣称普通 OneDrive 可站内恢复。
- 本机个人站点标识可从 Graph 获取，但 `GET /beta/sites/{siteId}/recycleBin/items` 的只读实测返回 HTTP 400 `BadRequest`，因此不能把此方案当作个人版替代接口。SimpleList 的 `permanentDelete` 也不能直接借用到个人版：[rclone 的 OneDrive 文档](https://rclone.org/onedrive/) 明确指出该 Graph 方法仅适用于工作/学校版及 SharePoint 文档库。
- 若未来要支持工作/学校账号的完整恢复，应单独获取面向 SharePoint 的访问令牌并验证 `/_api/web/recyclebin` 与恢复端点；不能把当前面向 Graph 的令牌发送给 SharePoint 域名。个人版可考虑仅维护“本应用删除记录”并凭已知 driveItem ID 查询和恢复，但必须明确标为不完整视图，不能伪装成官网全部回收站。

---

## 8. 分享（graph.go / onedrive.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| CreateShare | ✅ | `graph.go:378-402` /createLink type:view scope:anonymous | 无 |
| 过期时间 | ✅ | `graph.go:384-386` expirationDateTime | 无 |
| 密码 | ✅ | `graph.go:381-383` | 无 |

---

## 9. 搜索（graph.go）

| 子功能 | 状态 | Go 证据 | 差距 |
|--------|:----:|---------|------|
| Search | ✅ | `graph.go:317-325` /me/drive/root/search(q=...) | 无 |
| **搜索分页** | ✅ | `graph.go:238-261` 跟随 @odata.nextLink 循环 | 无 |
| **缩略图** | ❌ | URL 无 $expand=thumbnails | 🟡 旧版有 |
| **过滤器** | ❌ | 无 | 🟡 旧版 filterOneDriveSearchResults |
| **引号转义** | ✅ | OData 单引号加倍 + `url.PathEscape` | 无 |

---

## 10. RefreshAccount

✅ `onedrive.go:361-414` 调 refreshOneDriveToken 刷新 token，再获取 /me 和 /me/drive 配额。对齐。

---

## 11. ProvideHashes

✅ `onedrive.go:30` SetHashes(["sha1", "quickxorhash"], nil)。`graph.go:460-472` mapItem 提取 sha1Hash/quickXorHash → ContentHash。

---

## 12. 版本历史（revisions）

❌ 完全缺失。旧版有 apiOneDriveListVersions / apiOneDriveRestoreVersion。

---

## P0 差距清单

1. ✅ **Token 刷新已实现**（`onedrive.go` 的 `RefreshAccount`），登录与刷新都会写入新的绝对 `expire_time`
2. ✅ **账号信息/配额已实现**（`onedrive.go:386-414`）
3. ✅ **上传断点续传已实现**（`upload.go:65-128` UploadSession + querySessionPosition + ClearUploadSession）
4. ✅ **ProvideHashes 已声明**（`onedrive.go:30` SetHashes）
5. ✅ **搜索分页已实现**（`graph.go:238-261` 跟随 nextLink）
6. ✅ 上传冲突策略已覆盖 overwrite/refuse/rename/skip，并校验上传响应的文件 ID
7. 🟡 搜索无缩略图；关键词的 OData 单引号和 URL 路径转义已修复
8. 🟡 版本历史完全缺失
9. ✅ **OAuth 默认凭据与 client_secret**（`auth.go`）：沿用旧版/rclone，登录与刷新均使用同一组凭据；登录会显示账号选择。
