# 新增网盘插件指南（PROVIDER_GUIDE）

Mnemo-Go 的网盘是插件：新增一个 Go 包，并登记模型常量、账号前缀、显示元数据和 blank import。
业务调用不增加中央 provider 判断。本指南说明插件契约与移植流程。

## 1. 契约（internal/drive）

### 接口 `drive.Driver`

```go
type Driver interface {
    ID() string
    Meta() drive.Meta
    Capabilities() drive.Capabilities
    RootID() string
    List(ctx, c drive.Context, dirID string, opts *drive.ListOptions) ([]model.File, error)
    // 其余方法以 driver.go 的实际契约为准
}
```

Provider 实现时**内嵌 `drive.BaseDriver`** 并覆盖支持的方法。`BaseDriver` 未提供默认实现的方法（如 `Mkdir`、`Rename`、`Move`、`Copy`、`Delete`）仍须显式实现；不支持的操作返回 `drive.NotSupported`，并关闭对应能力。新增包应加接口编译断言，避免只参考示例而漏掉实际契约：

```go
type Driver struct{ drive.BaseDriver }

var _ drive.Driver = (*Driver)(nil)

func (d *Driver) List(...) ([]model.File, error) { ... }   // 必实现
func (d *Driver) Search(...) ...                          // 可选
func (d *Driver) UploadOneFile(...) ...                   // 可选
```

### 新鲜文件元数据

需要绕过驱动内存池或 UI 占位详情时，实现 `drive.FreshFileDriver.GetFileFresh(ctx, c, fileID)`。`drive.GetFileFreshContext` 优先重新列出已知父目录；缺少父目录时使用驱动的新鲜详情接口。无法从远端确认文件的驱动必须返回错误，不能把占位名称、大小 0 或旧缓存当作可删除来源的依据。普通浏览仍可使用原有缓存。

移动迁移必须重新列出目标所属目录并匹配目标 ID，再核对类型、大小及共同哈希。冷启动恢复缺少父目录时保留来源，提示刷新目录；不得用同名文件猜测目标。

### 注册（init()）

```go
func init() {
    drive.Register(drive.Registration{
        ID:   "mydrive",
        Meta: drive.GetMeta("mydrive"),   // 需在 drive/meta.go 的 builtinMetas 登记
        Caps: drive.NewCapabilities("mydrive", map[string]bool{
            "search": true, "createShare": true, ...,
        }, func(c *drive.Capabilities) {
            c.SetHashes([]string{"md5"}, []string{"md5"}) // 秒传指纹
            c.SetUploadMode(drive.UploadModeQueue)
        }),
        Factory: func() drive.Driver { return &Driver{} },
        Auth:    authLogin,                    // 可选：登录流（Registration.Auth）
        Login:   drive.LoginConfig{Fields: []drive.LoginField{
            {Key: "username", Type: "text", Label: "账号", Required: true},
            {Key: "password", Type: "password", Label: "密码", Required: true},
        }},
    })
}
```

### 能力位
`drive.Capabilities` 与旧版 TS `DriveProviderCapabilities` 一一对应。
UI 菜单/操作按能力位裁剪；能力位声明了但 Driver 未实现的方法会返回
`ErrNotImplemented`，属实现缺陷（应由单测守护）。

### 收藏（云端优先，本地回退）

所有网盘都有本地收藏入口。只有同时实现原生收藏列表与修改功能时，才声明 `favorite: true` 并实现 `drive.RemoteFavorites`：

```go
type RemoteFavorites interface {
    SupportsRemoteFavorites(ctx context.Context, c Context) (bool, error)
    ListFavorites(ctx context.Context, c Context) ([]model.File, error)
}
```

另外覆盖 `Driver.Favorite`，返回远端确认成功的文件 ID。`ListFavorites` 必须读取完整分页，保留文件所属空间和父目录，异常时返回错误而不是部分列表。账号类型差异由插件内部处理（如 OneDrive 个人账号返回不支持）；权限不足、网络故障等不能作为“不支持”的依据。

UI 使用 `App.AddFavorite` / `App.RemoveFavorite` / `App.ListFavorites`；本地回退、云端快照、备份与账号隔离由 app/store 层负责。不要在 UI 再调用一次 `FavoriteFiles`，也不要在中央代码按 provider 写分支。逐盘现状见 [收藏核查表](PROVIDER_STATUS.md#收藏逐盘核查2026-09-10)。

### 下载源契约

`GetDownloadURL` 返回下载源，而非中央按网盘分支执行下载。插件必须返回必要的 `Headers`（包括专用 UA）、大小和可用的过期时间；`ExpireTime` 使用 Unix 毫秒，未知可为 0。签名 CDN 链接不要附带不必要的账号 Bearer token。

- `DownloadMode`、`ForceLocalProxy` 用于前端/播放策略，不隐含限制本地下载并发。
- `Concurrency > 0` 表示插件允许的请求并发上限；最终值不超过用户单文件设置。0 表示使用用户设置，而不是单请求。
- `RetrySingleConnection` 是不序列化到前端的可选恢复策略；分段请求遇到短期 HTTP 服务错误且常规重试耗尽后，降为单连接重试一次。权限、限流、长时间 Retry-After、校验和本地文件错误不降级。当前仅迅雷启用，普通下载与目录同步消费该策略。
- `AllowLastModifiedFallback` 是仅供后端消费的可选策略，当前仅光鸭启用。修改时间响应不一致且没有匹配的强 ETag 时，停止分段并废弃旧断点，以完整单流重新下载一次；不复用旧分片，不忽略 ETag 或长度变化。正常分段下载不限制并发。
- `ChunkSize > 0` 为显式分片大小；0 使用下载器自适应策略。没有真实协议/账号证据时，不擅自增减插件现有连接限制。
- `RequestAuth` 为请求绑定鉴权器，必须可为每次请求重新签名。Digest 需要保序时应同时声明 `Concurrency: 1`。
- 普通账号下载每次启动/恢复重新取得完整源；不能只缓存 URL 而丢弃头、鉴权器和策略。

普通下载、目录同步使用同一分片下载器。跨盘迁移管道使用同一下载 HTTP 超时/重试策略，但仍是顺序流，应用装配让这些路径共享下载限速器。逐盘配置及未实测的限制见 [下载引擎审计](DOWNLOAD_ENGINE_AUDIT_2026-10-04.md)。

### 装配方式
`internal/drive/providers/all.go` 用 blank import 引入新包即完成注册：

```go
import (
    _ "mnemo-go/internal/drive/providers/mydrive"
)
```

## 2. 模型（internal/model）

- `model.File`：统一列表/详情模型（字段与旧版一致）。
- `model.TokenInfo`：登录凭据；`tokenfrom/user_id` 命名空间隔离。
  挂载存储类（webdav/s3）用 `TokenInfo.Conn`。
- `model.UploadingUI`：上传任务视图，进度写 `ui.Upload`。

## 3. 会话与上下文

每次驱动调用都会拿到 `drive.Context`（含 userId/driveId/token）。
从 `ctx.Token` 读取凭据；需要刷新时实现 `RefreshAccount`。

云离线功能实现 `drive.OfflineDriver`，创建、查询、删除统一使用 `drive.Context` 和 `drive.OfflineTask`；不要让应用层依赖某个 provider 的私有返回类型。设备身份固定，轮换凭据在释放账号锁前通过 `drive.PersistToken` 保存。

完整实现离线契约时必须开启 `offlineDownload`，并添加编译断言。`registrycheck` 会逐盘核对注册开关、驱动开关及统一门面是否与实现一致；应用层验证开关传递到前端，前端验证支持账号确实显示入口，防止出现“已实现但未开放”。没有实现的网盘不能仅打开开关。

## 4. 登录流（Auth）

三种形态：

| 类型 | Auth 实现 | 示例 |
|---|---|---|
| OAuth PKCE | 本地 127.0.0.1 随机端口回调 | onedrive / dropbox |
| 账密/Token | 直接用表单字段调登录 API | pikpak / aliopen / pan139 |
| 挂载表单 | 不实现 Auth，表单填连接参数 | webdav / s3 |

`drive.AuthRequest.Config` 携带登录表单值；返回 `*model.TokenInfo`。

## 5. 文件 id 约定

- 简单盘：`file_id` 即服务端 id（pikpak/aliopen/onedrive/dropbox）。
- 路径盘：`file_id` 为路径（webdav/s3，根为 `/`）。
- 双盘盘：id 带作用域前缀（aliopen 的 `b:`/`r:`，根 `backup_root`/`resource_root`）。

## 6. 上传

`UploadOneFile(ctx, c, ui)` 接收完整上传任务：
- 小文件：单请求 PUT/POST。
- 大文件：分片/会话（onedrive upload session、dropbox session、aliopen part、
  pikpak GCID+OSS、pan139 chunk）。
- 进度写 `ui.Upload.DownSize/DownProcess`。

## 7. 移植自旧版（参考实现）

1. 读 `../Mnemo/src/drive/providers/<id>.ts`（驱动适配）+ `../Mnemo/src/<id>/*.ts`（API）。
2. 读同包内已移植 provider（`pikpak/`、`aliopen/` 是最完整的参考）。
3. 逐方法移植，字段/端点/加密算法必须与旧版一致。
4. 完成后：
   - `go build ./...`
   - `internal/drive/providers/all.go` 加 blank import
   - 为纯逻辑写单测（签名算法、分页、映射等）

## 8. 检查清单

- [ ] `internal/drive/meta.go` 的 `builtinMetas` 已登记（label/icon/rootKey）
- [ ] 能力位与旧版 `driveProviderCapabilities` 一致
- [ ] 登录流可用（Auth 或挂载表单）
- [ ] list / getDownloadUrl / 文件操作可用
- [ ] 上传/秒传按能力位实现
- [ ] `go build ./...` 通过，核心逻辑有单测
