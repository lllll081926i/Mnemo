# 操作失败原因契约

## 数据

下载任务和上传状态新增可选 `issue`：

```json
{
  "code": "PROVIDER_COOLDOWN",
  "message": "服务商要求冷却，请勿连续重试或反复登录",
  "action": "wait",
  "retryAt": 1792041600
}
```

`code` 标识原因，不替代任务的 `status`。`message` 使用固定安全文案，不复制原始响应、URL、路径或凭据。原有错误详情和脱敏日志保留。`action` 是建议路径，不是自动重试指令。`retryAt` 使用 Unix 秒，仅为服务商冷却参考时间，到时不代表限制一定解除。

旧任务没有 `issue` 时保留原有错误展示，不按文件名数字补造结构化原因。未知错误返回 `UNKNOWN`，不凭文字猜测账号失效。

## 分类证据

`drive.DescribeOperationError` 使用 `errors.Is`、`errors.As` 和公共错误方法，不按服务商名称分支：

| 证据 | 原因 | 建议路径 |
|---|---|---|
| 明确 `ErrUnauthorized` | `NEEDS_AUTH` | 重新授权原账号，再手动恢复 |
| HTTP 401/403 | `REMOTE_ACCESS_REJECTED` | 核对权限、链接有效期与账号限制 |
| 服务商 `RetryAfter()` 或 HTTP 429 | `PROVIDER_COOLDOWN` | 等待并核对，避免反复登录 |
| HTTP 409/412 | `BLOCKED_BY_CONFLICT` | 核对来源与目标，不能降级覆盖 |
| 本机 ENOSPC / Windows 磁盘已满 | `NO_LOCAL_SPACE` | 检查保存与临时目录所在磁盘 |
| 本机权限或路径错误 | `LOCAL_PERMISSION` / `LOCAL_NOT_FOUND` | 核对本机目录和占用 |
| 明确网盘不存在 / HTTP 404、410 | `REMOTE_NOT_FOUND` | 刷新目录，核对资源与链接 |
| 网络错误、超时、数据流中断 | `NETWORK_UNAVAILABLE` | 检查网络或代理，手动恢复 |
| HTTP 5xx | `REMOTE_UNAVAILABLE` | 核对状态后稍后恢复 |
| 明确能力未实现 | `UNSUPPORTED` | 核对当前账号能力 |
| 明确取消 | `USER_CANCELED` | 保留停止语义，不自动恢复 |
| 其他 | `UNKNOWN` | 保留任务和错误详情 |

HTTP 状态通过 `HTTPStatusCode() int` 获取。冷却通过已有 `RetryAfter() time.Duration`，或 `HTTPRetryAfter() string` 获取；后者支持标准秒数和 HTTP 日期。下载引擎和 OneDrive Graph 已接入；PikPak 限流/风控复用现有方法。其他驱动保留原协议，不把未经确认的字符串改成机器结论。

下载直链的 401/403 可能来自链接失效，不能据此断定整个账号失效。只有明确凭据失效的错误才能给出重新授权入口。

## 生命周期

- 失败原因随任务保存和事件发送。重启中断标记 `APP_RESTARTED`；直链未落盘来源标记 `SOURCE_REQUIRED`，不伪造来源链接。
- 用户暂停下载标记 `USER_PAUSED`。哈希错误标记 `INTEGRITY_MISMATCH`，最终文件保护不变。
- 恢复进入排队和成功时清除旧诊断。前端明确清除缺省诊断和旧上传错误，不因局部合并残留上次失败。
- 取消以类型和上下文判断。路径包含 `canceled` 不再被误判为用户暂停。
- 独立 `TaskIssue` 组件展示建议，提供网络设置和原账号重新授权入口；两个入口均不自动恢复任务或更换任务账号。
- 上传恢复仍取决于服务商和远端提交状态。写入重试前提示核对目标，不把本契约当成已完成上传对账或可安全重传的证明。

## 后续接入

当前覆盖下载、上传及相应任务详情。目录、同步、迁移与云离线尚未全部接入。完整诊断中心、账号级恢复调度、普通上传未知提交对账和所有驱动错误适配仍属独立待办，不能因引入 `issue` 就标为完成。
