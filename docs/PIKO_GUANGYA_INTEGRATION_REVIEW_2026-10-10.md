# Mnemo：Piko 借鉴、PikPak 收藏与光鸭云离线修复核查

> 审查分支：`audit/hardening-2026-10-10`；仅列已确实修改的功能与后续建议。不得把 GitHub Actions 模拟测试当作真实网盘端到端验收。

## 1. 已落实的行为修复

### PikPak 收藏

- **观察依据**：Piko 的 PikPak Kotlin SDK `StarEndpoint.kt` 明确记录其 2026-09-24 的实测：成功加星后，普通列表的 `starred` 和详情的 `starred` 可能仍为 `false`，真正星标存在于 `tags:[{"name":"STAR"}]`；星标操作使用 `POST /drive/v1/files:star` / `unstar`。
- **改动**：Mnemo 的 `File` 映射兼容 `STAR` 标签，保留 `starred` 旧字段以兼容不同版本的响应；`ListFavorites` 不再限制文件必须完成态，避免遗漏被收藏的其他阶段的文件或目录。
- **验证**：测试普通列表中 `starred=false+STAR tag`、普通布尔值、其他标签、收藏分页、全盘 `parent_id=*` 查询条件、star/unstar POST 路径。
- **不做**：不缓存伪造星标状态，也不在收藏列表 API 故障时用空数组覆盖已保存的收藏。

### 光鸭云离线

- **观察依据**：`DDSRem-Dev/guangyaclient` 的 `cloud_resolve_url`、`cloud_create_task`、`cloud_task_list` 均使用服务 `/nd.bizcloudcollection.s/v1/`；`cloud_task_list` 的请求参数为 `page`（从 0 开始）、`pageSize`、`status`，并非 `cursor`。
- **改动**：修复解析、创建与列表服务路由；切换为页码分页，提供重复页与最多 200 页的保护。删除端点采用同一服务前缀的 `v2/delete_task`，**该 v2 精确端点仍未获得独立实现交叉验证**。
- **验证**：使用模拟 HTTP 响应验证调用路径、鉴权、分页、任务状态、登录失效刷新、异常数据及远端删除失败时保留本地任务。
- **限制**：`newName`、选择子文件 `fileIndexes` 与远端异常状态在各账号/地区的支持仍需真实账号回归。不能在无验证时盲目重试创建任务，防止重复云任务。

## 2. Piko 值得借鉴但尚未完整实施的能力

| 能力 | Piko 的现有做法 / 线索 | Mnemo 设计方向 | 本次状态 |
| --- | --- | --- | --- |
| 全盘收藏一致性 | `listStarred` + STAR 系统标签 | 同时修正目录列表和收藏界面 | 已修改，待真实账号 |
| 快速访问 | `PinnedFolders`、`RecentFolders`、`QuickAccessState` | 不增加新权限，保留多账号独立固定目录 | 待实现 |
| My Pack 便捷入口 | `QuickAccessState.watchMyPacks` | 按账号发现 My Pack，支持一键跳转；不能硬编码 ID | 待实现 |
| 同目录列表缓存 | `FolderContentMemory`、`FolderIndex` | 账号+目录作用域缓存，写操作后失效并刷新 | 待实现 |
| 批量整理 | `BatchRenameState`、`DuplicateScanner` | 先只读预览，再批量执行，提供冲突与撤销边界 | 待实现 |
| 视频体验 | `PikoMediaProxy`、`PikoFileCachePool` | 重用 Mnemo 现有本地流代理，改进签名过期、CDN 故障恢复 | 待设计与测试 |
| 弱网下载 | `PikoDownloadCoordinator`、动态线路选择 | 在 Mnemo 现有 dlengine 上做有限重试、域名可信校验与熔断，**不重写已成熟下载器** | 待实现 |
| 磁力/BT 预览 | `InstantMagnetRepository`、`InstantSavePlan` | 不创建远端任务也能预览与选择；需服务端能力核实 | 待实现 |
| 传输面板 | `TransfersState`、`TaskRepository` | 统一速度、阶段、服务端确认状态和弱网恢复 | 待实现 |
| 更新可靠性 | Release 校验、打包烟雾测试 | 保留现有 Mnemo CI，增加安装包校验与可回滚检查 | 待实现 |

## 3. 版权与适用性

- Piko 主仓库：https://github.com/NihilDigit/piko （AGPL-3.0-or-later）；已检查仓库 README 和许可证。
- 参考了项目行为、架构与公开 API 描述，**不直接复制 AGPL 源码**到 Mnemo。
- 其 Kotlin Multiplatform 设计不能机械移植到 Mnemo 的 Go + Wails + Vue 架构；优先按现有架构原生实现。
- SDK：https://github.com/NihilDigit/pikpak-kotlin/blob/main/src/commonMain/kotlin/io/github/nihildigit/pikpak/StarEndpoint.kt
- 光鸭独立实现：https://github.com/DDSRem-Dev/guangyaclient/blob/master/guangyaclient/client.py

## 4. 真账号测试清单（尚待执行）

1. PikPak 两个账号分别星标与取消星标，在普通文件列表、全盘收藏、重登录后观察一致性。
2. PikPak 星标子文件夹与跨多层目录文件，确保分页、标签、删除/还原后的结果正确。
3. PikPak 无权访问或 API 限流时，不要错误清空本地收藏。
4. 光鸭 HTTP、HTTPS、magnet、ed2k 单任务解析、创建和状态轮询。
5. 光鸭超过 50 个云离线任务的跨页枚举，无重复、无遗漏。
6. 光鸭删除任务（仅删除记录 / 同时删除文件）分别测试远端是否真正完成。
7. 光鸭登录过期后的 token 续期、429 限流、400 参数错误与任务创建后响应丢失。
8. 无真实账号时以上仅能标记为 Mock 单测，不可称“全部功能正常”。
