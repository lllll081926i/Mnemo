# Google Drive 接入研究：优先使用 OpenList 在线授权与刷新

研究日期：2026-10-30。项目基线：`ddd877e1b1c2ce155b68d0f01dc85abf02a7943f`（v0.4.7）。

## 1. 结论与当前状态

协议层面可以接入。采用 OpenList APIPages 完成网页授权和令牌刷新；文件操作由 Mnemo Go 后端直接请求 Google Drive API v3。

**当前公共共享应用存在阻断问题，不能把此方案标记为已可用。** 本次匿名探测中，`api.oplist.org` 和 `api.oplist.org.cn` 的刷新接口都返回 HTTP 500，正文为 `{"text":"The OAuth client was deleted."}`。匿名打开全球节点生成的 Google 授权链接，也在 Google 页面中检测到 `deleted_client` 及相同描述。Google 官方将该错误定义为 OAuth 客户端已删除，区别于用户令牌失效的 `invalid_grant`。[Google OAuth 错误说明](https://developers.google.com/identity/protocols/oauth2/web-server#errors)

这是本次网络环境与时间点的观测，不能据此断言全部 OpenList 部署均不可用。研究阶段未使用真实账号授权，未验证有效令牌刷新，未执行文件读写。研究提交 `baeed02` 仅新增文档；后续用户明确要求接入，驱动实现现已恢复，当前范围与验收状态见 [Google Drive 驱动说明](../providers/gdrive.md)。未发布新版本。

## 2. 上游基线与实际探测

源码阅读固定到以下提交，避免将后续上游变化混入结论。公共站点的实际部署提交未知。

| 来源 | 固定提交 | 核对内容 |
| --- | --- | --- |
| OpenList | `e72822f5390f4272c9fe9da8cabba3c1a270182f` | `drivers/google_drive/` 的配置、刷新、文件操作、上传 |
| OpenList-APIPages | `4322fd1f0f5e4e77454fd8a5edb89727854b0092` | Google 授权、回调、刷新包装与网页操作 |

探测时间为 2026-10-30 05:30—05:31 UTC，即香港时间 13:30—13:31。刷新请求使用固定的无效研究令牌 `mnemo-invalid-research-token`；该字符串不属于任何账号。请求经过本机系统代理。完整脱敏结果见 [探测证据](../review-evidence-2026-10-30/google-drive-openlist-probe.json)。

| 节点 | 授权入口 `/googleui/requests` | 刷新 `/googleui/renewapi` |
| --- | --- | --- |
| `https://api.oplist.org` | 200，生成 Google 授权 URL | 500，OAuth 客户端已删除 |
| `https://api.oplist.org.cn` | 200，生成 Google 授权 URL | 500，OAuth 客户端已删除 |
| `https://api-cn.oplist.org` | 当前环境网络请求未完成 | 当前环境网络请求未完成 |

APIPages README 顶部展示中国站点 `api.oplist.org.cn`，接口表格还保留 `api-cn.oplist.org`，两者存在文档差异。不能把旧域名自动作为可靠备用节点。[固定版本 README](https://github.com/OpenListTeam/OpenList-APIPages/blob/4322fd1f0f5e4e77454fd8a5edb89727854b0092/README.md)

`src/shares/refresh.ts` 在缺少访问令牌时提取上游 `error_description` 等字段，包装为 `{text: ...}` 和 HTTP 500。因此不能将此类 500 一律当作暂时网络故障重试。[刷新包装源码](https://github.com/OpenListTeam/OpenList-APIPages/blob/4322fd1f0f5e4e77454fd8a5edb89727854b0092/src/shares/refresh.ts)

## 3. 授权与刷新请求契约

### 3.1 首次授权

网页发起以下请求，使用服务端预设应用：

```http
GET /googleui/requests?server_use=true&driver_txt=googleui_go
Host: api.oplist.org
```

成功响应的 `text` 是 Google 授权 URL。关键参数为 `response_type=code`、`access_type=offline`、`prompt=consent`、随机 `state`，权限范围是 `https://www.googleapis.com/auth/drive`。回调固定到该部署的 `/googleui/callback`。

此请求还设置 `driver_txt`、`random_key`、`server_use` Cookie。必须让请求与回调在同一浏览器上下文完成。Go 后端取得 Google URL 后直接打开该 URL，会遗漏浏览器所需的 OpenList Cookie。

回调由 OpenList 服务端兑换令牌，再通过 `/#<Base64 JSON>` 返回其网页。已检查的 Google 路由没有桌面应用回传地址、一次性票据轮询或 Mnemo 本地回调参数，不能直接套用 OneDrive 的本地回调方式。[授权与回调源码](https://github.com/OpenListTeam/OpenList-APIPages/blob/4322fd1f0f5e4e77454fd8a5edb89727854b0092/src/driver/googleui_oa.ts)

首期建议流程：

1. 在 Mnemo 登录表单说明令牌接收方与完整 Drive 权限，复用现有 `LoginMethodSpec.Recipient/Notice/Consent`。
2. 打开 OpenList 官方 Token 网页，选择 Google Drive 与服务端预设参数。
3. 在系统浏览器完成 Google 授权。
4. 将网页显示的 `refresh_token` 导入 Mnemo 密码输入框。
5. 后端通过同一应用对应的在线接口刷新令牌，并读取账号资料，成功后保存账号。

不要求用户粘贴含令牌的完整回调链接。全自动回传需要上游提供明确协议，或在可信自建服务上实现；当前协议不能承诺自动回传。

静态检查还发现：共享应用分支没有执行自有应用分支中的同等 `state` 比对，随机值使用 `Math.random`。这是固定版本代码的观察，未验证线上漏洞。若自建并修改该服务，应统一校验 `state` 并使用安全随机源。

### 3.2 后续刷新

```http
GET /googleui/renewapi?server_use=true&driver_txt=googleui_go&refresh_ui=<URL编码后的刷新令牌>
Host: api.oplist.org
```

成功响应包含 `access_token`、`refresh_token`；错误通常包含 `text`。OpenList 驱动实际发送的是 `driver_txt`，README 刷新表格中的 `apps_types` 不能直接照抄。服务端保管共享应用密钥，Mnemo 不内置该密钥。[OpenList 刷新实现](https://github.com/OpenListTeam/OpenList/blob/e72822f5390f4272c9fe9da8cabba3c1a270182f/drivers/google_drive/util.go)

Mnemo 实现要求：

- 同时检查 HTTP 状态、JSON 结构和非空访问令牌，禁止仅凭回显的刷新令牌认定成功。
- 接口没有透传 `expires_in`，不虚构有效期。访问令牌首次遭遇 401 时，按账号合并刷新请求，最多刷新并重放一次。
- 保留原刷新令牌；响应给出新值时，在账号锁内通过 `drive.PersistToken` 保存后释放锁。
- 记录令牌对应的授权服务和应用模式。更换节点前核实共享应用身份，禁止将真实令牌静默发送给多个节点。
- GET 查询携带令牌，必须过滤请求日志、网络异常中的 URL 和通知详情；错误提示不拼接原始请求地址。
- 请求绑定 `context.Context`，设置超时；刷新端点不跟随到未知主机。

### 3.3 当前故障的处理

默认公共服务需先修复共享 OAuth 应用，并通过真实授权与刷新验收。另一条可行路径是自建 OpenList APIPages、配置自有 Google OAuth 应用，保留同款请求协议。自建仍需配置正确回调与 Google Drive 权限，不能依靠更换 URL 消除应用配置要求。

服务端应用错误应通过统一通知显示“OpenList 的 Google 授权服务当前不可用”，详情保留脱敏上游原因。用户令牌失效才提示重新授权。禁止无限刷新、循环打开浏览器或每次切换账号重复弹窗。

## 4. Google Drive 文件操作适配

OpenList 在线接口只负责令牌交换。目录、下载和上传仍需要客户端网络能够访问 Google API；在线刷新不能充当文件传输代理。[OpenList 驱动配置](https://github.com/OpenListTeam/OpenList/blob/e72822f5390f4272c9fe9da8cabba3c1a270182f/drivers/google_drive/meta.go)

| 功能 | Google API / 处理要求 |
| --- | --- |
| 目录与搜索 | `GET /drive/v3/files`，目录查询使用 `'<parentID>' in parents and trashed=false`；正确转义查询内容并处理完整分页 |
| 文件详情 | `GET /drive/v3/files/{id}`，按需读取父目录、大小、MIME、校验和与操作权限 |
| 普通文件下载 | `GET /drive/v3/files/{id}?alt=media`，使用 Bearer 请求头，支持 Range |
| 新建目录 | `POST /drive/v3/files`，MIME 为 `application/vnd.google-apps.folder` |
| 重命名与移动 | `PATCH /drive/v3/files/{id}`；移动使用实际源父目录，不能固定 `removeParents=root` |
| 移入回收站与恢复 | `PATCH /drive/v3/files/{id}`，分别设置 `trashed=true/false` |
| 回收站列表 | `files.list` 查询 `trashed=true`，保留所属空间并处理分页 |
| 彻底删除 | `DELETE /drive/v3/files/{id}`，只映射明确的永久删除操作 |
| 同盘复制 | Google 提供 `files.copy`；只有实现并验收后才开启能力，文件夹递归复制单独核验 |
| 云端收藏 | 完整实现 `starred=true` 列表与修改后再开启；此前沿用项目本地收藏 |
| 秒传 | 本次核对的接口没有发现“提交已知哈希免传内容”的契约；`RapidUploadHashes` 保持为空 |

回收站语义以 [Google 删除与恢复文档](https://developers.google.com/workspace/drive/api/guides/delete) 为准。OpenList 当前 `Remove` 调用永久删除，不能直接映射为 Mnemo 的普通删除。[OpenList 文件操作源码](https://github.com/OpenListTeam/OpenList/blob/e72822f5390f4272c9fe9da8cabba3c1a270182f/drivers/google_drive/driver.go)

共享盘请求按适用接口设置 `supportsAllDrives=true`，列表使用 `includeItemsFromAllDrives=true`。指定共享盘检索时配套 `corpora=drive` 与 `driveId`。若返回 `incompleteSearch=true`，不能把部分结果视为完整目录或用于删除来源判断。首期可以显式配置一个根目录；自动枚举共享盘另做空间模型验收。[共享盘说明](https://developers.google.com/workspace/drive/api/guides/enable-shareddrives)、[列表契约](https://developers.google.com/workspace/drive/api/reference/rest/v3/files/list)

快捷方式必须区分快捷方式 ID 与目标 ID。浏览可解析目标；重命名、移动、删除必须作用于用户选中的快捷方式。不能照搬上游将对象 ID 替换为目标 ID 的映射。

Google Docs、Sheets、Slides 需要 `files.export`，不能按普通二进制文件下载。该导出接口存在 10 MB 内容限制且不支持 Range。首期未完成导出格式、文件名、实际大小与下载器适配前，只提供网页打开并返回明确的导出不支持提示；不能将缺失大小当作零字节成功，也不能用于移动迁移后删除来源。[下载与导出说明](https://developers.google.com/workspace/drive/api/guides/manage-downloads)

## 5. 上传确认与恢复

使用 resumable upload。创建会话使用 `POST /upload/drive/v3/files?uploadType=resumable`，覆盖已有文件使用对应文件 ID 的 `PATCH`；传输使用响应 `Location`，不自行改写会话 URL。

建议普通文件创建前调用 `files.generateIds`，把预生成目标 ID 持久化，再发起写入。这样断线后可按同一 ID 核对，减少重复创建；409 仍须读取目标详情确认内容。Google Workspace 格式转换不适用这一策略。[预生成 ID 契约](https://developers.google.com/workspace/drive/api/reference/rest/v3/files/generateIds)

上传状态以远端确认结果为准：

- 非末块按 256 KiB 的整数倍发送，可采用 5 MiB；零字节文件单独处理。
- 308 表示未完成，按 `Range` 更新确认偏移，不能按本地发送量推进成功状态。
- 200/201 后解析文件 ID，再核对大小及可用校验和；缺少必要确认时继续核对。
- 断线后向原会话发送状态查询，读取服务端已收字节；不直接创建第二个文件。
- 会话失效时按已记录目标 ID 核对远端，再决定恢复动作；不能用同名文件判断成功。
- 覆盖使用原文件 ID，不采用“先删除再创建”；401 刷新后恢复原会话，不递归重启整个上传。

以上依据 [Google 上传协议](https://developers.google.com/workspace/drive/api/guides/manage-uploads)。上游分片实现未充分检查 308 与 `Range`，不适合直接移植为 Mnemo 的上传完成判断。

接入项目 `UploadAttemptDriver`：在本地检查完成后、首次远端写入前调用 `ui.BeginUploadAttempt()`；获得或预分配目标 ID 后及时保存。完成后的凭据持久化错误走 `upload:warning`，不使成功上传进入重传。真正无法确认的结果保留现有只读核对流程；任务错误全部进入统一通知。

## 6. 项目落点与实施顺序

| 落点 | 需要完成的工作 |
| --- | --- |
| `internal/drive/providers/gdrive/` | 新插件、在线令牌客户端、请求重试、分页与映射、文件操作、上传恢复 |
| `internal/drive/providers/all.go` | 空白导入注册；业务层继续只走 `drive/ops` |
| `internal/model/account.go`、`internal/drive/meta.go` | 恢复 provider 常量、账号前缀与显示元数据；授权服务配置随账号保存 |
| `internal/drive/auth.go` 现有声明式表单 | 令牌导入、授权网页入口和第三方接收方说明；复用阿里中转登录机制 |
| `internal/model/file.go` 与下载源契约 | 明确快捷方式标识及原生文档下载限制；优先插件内部解决，必要扩展再同步契约 |
| 注册、能力、模型与前端测试 | 更新明确禁止 `gdrive` 的旧断言；核对菜单与实际方法，不只增加图标 |
| `AGENTS.md`、`docs/PROVIDER_STATUS.md`、`docs/providers/gdrive.md` | 实现后同步支持范围和实测状态 |

实施分为三步：先完成在线令牌客户端与错误处理；再完成目录、普通文件下载与回收站；最后完成上传恢复与完整插件装配。协议测试可独立推进，默认公共授权上线必须等共享应用故障解除。自有应用或自建服务作为显式选择，不静默切换。

## 7. 验收门槛

本次已完成源码核对、匿名接口探测和方案对照，未完成以下实现验收：

- 在线接口：200 空令牌、500 文本错误、非 JSON、超时、取消、401 仅刷新一次、并发合并、令牌保存失败与 URL 脱敏。
- 文件操作：多页目录、引号与反斜线搜索、真实父目录移动、快捷方式删除、回收站分页和恢复、共享盘权限错误。
- 上传：零字节、跨块文件、308 部分接收、最后一块响应丢失、会话过期、401 恢复、409 核对、覆盖中断、完成后凭据保存失败。
- 下载：鉴权与 Range、令牌过期、本地预览代理；原生文档不进入零字节完成或不安全迁移流程。
- 真实账号：系统浏览器授权、首次刷新、重启后刷新、小文件上传下载校验、移入回收站与恢复；共享盘需具备对应账号单独核验。
- 工程：`go test ./...`、`go test -race ./...`、`go vet ./...`、前端测试与 `wails build`，能力声明和项目文档一致。

仅通过无效令牌探测不能替代真实账号验收。当前默认公共服务状态不满足发布条件。
