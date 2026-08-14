# Mnemo-Go 工程约束

## 架构原则

1. **插件化**：新增网盘 = 新增 `internal/drive/providers/<id>` 包 + `all.go` blank import。
   禁止在门面或 UI 里写 `if provider == ...`。
2. **能力裁剪**：每个 provider 声明 `Capabilities`，UI 按位裁剪菜单。
3. **统一模型**：所有盘输出 `model.File`。
4. **身份隔离**：用户 id 以 `provider_` 或 `provider:` 命名空间前缀隔离。
5. **事件驱动**：进度/状态变化走 `Events.Emit`，前端不做轮询。

## 盘移植清单

在役 13 盘（`gofile`、`gdrive` 已移除）：

| 盘 | 登录方式 | 状态 |
|---|---------|------|
| PikPak | 账号+密码+滑块验证码 | ✅ |
| OneDrive | OAuth PKCE | ✅ |
| Dropbox | OAuth PKCE | ✅ |
| 123 云盘 | 账号+密码 | ✅ |
| 蓝奏云 | Cookie/Cred | ✅ |
| 优享版蓝奏云 | 账号+密码 | ✅ |
| 139 云盘 | 手机号+密码 / Authorization | ✅ |
| 天翼云盘 | 账号+密码（RSA+HMAC） | ✅ |
| 一刻相册 | BDUSS Cookie | ✅ |
| 阿里云盘 | Refresh Token | ✅ |
| 光鸭云盘 | 短信验证码 / Refresh Token | ✅ |
| WebDAV | 地址+账号+密码（挂载） | ✅ |
| S3 | 端点+密钥（挂载） | ✅ |

## 构建

```bash
# 环境
go version ≥ 1.25
gcc (mingw-w64, 仅 Windows)
node ≥ 20

# 开发
wails dev

# 测试
go test ./...

# 生产构建
wails build
```

## 测试

- 纯逻辑（加密/签名/分页/映射）写 `_test.go` 单测。
- provider 包测试覆盖能力位、注册、映射、login 函数。
- 集成测试：`go test ./...` 通过。

## 依赖

- 纯 Go 标准库优先（`net/http`、`crypto`、`encoding/xml`、`encoding/json`）。
- 必要外部依赖：`aws-sdk-go-v2`（S3）、`golang.org/x/sync`（errgroup）。
- 无 cgo 外部依赖（除 Wails 自身的 WebView2 loader，由 wails build 处理）。
- 引擎二进制（mpv）使用 `go:embed` 内嵌，运行时释放。

## 代码风格

- 遵循 Go 标准 `gofmt` + `go vet` 无警告。
- 字段 JSON tag 与旧版 TS 模型一致。
- 错误处理：返回 error 而非 panic（init 注册时 panic 除外）。
- 注册：`internal/drive/providers/all.go` 管理所有 provider 的 blank import。