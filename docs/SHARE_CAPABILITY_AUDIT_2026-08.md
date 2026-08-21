# 分享能力汇总（2026-08）

## 结论

13 个在役网盘中，10 个已开放创建入口并有真实远端请求实现与协议级回归；优享蓝奏、一刻相册、WebDAV 保持关闭，不把“可解析他人分享”误报为“可创建分享”。

| 网盘 | 创建方式 | 范围 | 有效期 | 提取码 | 协议回归 | 外部联机验收 |
|---|---|---|---|---|---|---|
| PikPak | 原生分享 API | 多项 | 服务端支持 | 自定义 | ✅ | 待指定测试文件；不重复登录以避免风控 |
| 阿里云盘 | Open API | 多项 | 服务端支持 | 自定义 | ✅ | 待指定测试文件 |
| 123 云盘 | 原生 API | 多项 | 服务端支持 | 自定义 | ✅ | 待指定测试文件 |
| 天翼云盘 | 原生 API（个人云） | 单项 | 永久 / 1 / 7 天 | 服务端生成 | ✅ | 待指定测试文件 |
| 139 云盘 | 个人云 outlink API | 多项 | 永久 / 1 / 7 天 | 服务端生成 | ✅ | 待指定测试文件 |
| 蓝奏云 | 原生文件/文件夹分享接口 | 单项 | 服务端规则 | 服务端生成 | ✅ | 待指定测试文件 |
| OneDrive | Microsoft Graph `createLink` | 单项 | 受账号策略限制 | 受账号策略限制 | ✅ | 待指定测试文件 |
| Dropbox | Sharing API | 单项 | 受套餐/策略限制 | 受套餐/策略限制 | ✅ | 待指定测试文件 |
| 光鸭云盘 | 原生资源分享 API | 多项 | 永久 / 1 / 7 / 30 天 | 自定义 | ✅ | 待指定测试文件 |
| S3 | `GetObject` 预签名 URL | 单项 | 1 / 7 天 | 不支持 | ✅ | 待指定测试对象 |
| 优享蓝奏 | 未开放 | — | — | — | — | 未找到稳定创建协议 |
| 一刻相册 | 未开放 | — | — | — | — | 产品有分享功能，未找到可靠 API |
| WebDAV | 未开放 | — | — | — | — | RFC 未定义通用公开分享 API |

## 本轮实现

- 新增 139、天翼个人云、光鸭和 S3 的创建路径。
- 统一返回账号、文件、链接、提取码、有效期；多项分享保留文件/目录类型。
- 有效期下拉按网盘能力收敛；单项网盘不能从任意入口发起多项分享。
- 蓝奏云、139、天翼的提取码由服务端生成，前端不再给出无效的自定义输入。
- S3 仅提供临时预签名访问，不写入本地历史，避免长期保存携带签名的 URL。

## 验收口径

协议回归使用本地 HTTP 传输替身，校验请求方法、URL、鉴权、签名、请求体、响应解析和失败分支；它证明应用实际会发出对应的远端请求，但不替代真实账号验收。

真实账号验收会创建公开链接，可能留下链接、访问码和测试文件。应在每个已登录账号中指定一个可公开的空测试目录（或统一授权创建 `Mnemo 分享测试` 文件夹）后执行：上传无敏感文本 → 创建分享 → 无登录打开链接 → 校验提取码/有效期 → 清理测试文件和可撤销的分享。

## 公开依据

- [Microsoft Graph `driveItem:createLink`](https://learn.microsoft.com/zh-cn/graph/api/driveitem-createlink?view=graph-rest-1.0)：OneDrive 的匿名链接、密码和有效期受账号与管理员策略约束。
- [Dropbox `create_shared_link_with_settings`](https://www.dropbox.com/developers/documentation/http/documentation#sharing-create_shared_link_with_settings)：创建共享链接及其可选设置。
- [阿里云盘 PDS CreateShareLink](https://help.aliyun.com/zh/pds/drive-and-photo-service-dev/developer-reference/api-pds-2022-03-01-createsharelink)：创建包含文件列表的分享链接。
- [天翼开放接口说明](https://id.189.cn/html/api_detail_173.html)：`createShareLink.action` 接口。
- [AWS S3 预签名 URL](https://docs.aws.amazon.com/AmazonS3/latest/userguide/ShareObjectPreSignedURL.html)：临时访问授权；SigV4 SDK URL 最长七天，临时凭据可能更早失效。
- [WebDAV RFC 4918](https://www.rfc-editor.org/info/rfc4918/)：协议没有通用的公开分享、提取码和过期时间创建标准。
- [139 云盘公开协议参考](https://greasyfork.org/zh-CN/scripts/476388) 与 [光鸭公开客户端](https://raw.githubusercontent.com/DDSRem-Dev/guangyaclient/master/guangyaclient/client.py)：仅用于交叉核对未公开协议，已由本地回归覆盖请求形状。
