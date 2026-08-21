# 已知问题与排查指南 (Known Issues)

本文档持续记录在真实网络、第三方服务端风控或特定环境下的已知限制、错误日志特征与应对解决方案。

---

## 一、Dropbox 授权后 Token 握手超时 (Network / Proxy)

### 1.1 现象描述
在系统浏览器中完成 Dropbox OAuth 授权后，页面成功重定向回本机监听的随机端口（`127.0.0.1:0`），但客户端界面提示登录失败，并在应用日志中输出如下错误：

```text
[WARN] HTTP request failed | target=api.dropboxapi.com/oauth2/token error="Post \"https://api.dropboxapi.com/oauth2/token\": dial tcp 65.49.26.99:443: connectex: A connection attempt failed because the connected party did not properly respond after a period of time, or established connection failed because connected host has failed to respond."
[WARN] provider login failed | provider=dropbox error="Post \"https://api.dropboxapi.com/oauth2/token\": dial tcp 65.49.26.99:443: connectex: A connection attempt failed..." duration=1m19.159s
[WARN] provider login RPC failed | scope=login error="Post \"https://api.dropboxapi.com/oauth2/token\": dial tcp 65.49.26.99:443: connectex: A connection attempt failed..." provider=dropbox
```

### 1.2 根本原因
- Dropbox 官方 API 域名（`api.dropboxapi.com` / `65.49.26.99`）在大陆网络环境下无法直连访问。
- 本地浏览器端因具备系统/浏览器层面的代理扩展可正常完成第一阶段网页授权，但在回调本机后，应用后台向 Dropbox 服务器换取 Access Token 时走的是原生 TCP 连接，直连握手被墙超时。

### 1.3 规避与解决方案
1. 打开应用「设置」页 ->「网络设置」-> 填写「网络代理」地址（如 `http://127.0.0.1:7890` 或 SOCKS5 代理）；
2. 保持代理客户端处于开启状态后重新发起 Dropbox 登录；
3. 或开启系统全局 TUN/TAP 模式代理。

---

## 二、PikPak 验证码通过后提示服务端风控 (Provider Risk Control)

### 1.1 现象描述
在登录 PikPak 账号时，应用已成功弹出并由用户完成了官方腾讯滑块人机验证（`txCaptcha.html`），但在提交登录请求时依然提示登录失败，日志输出：

```text
[WARN] provider login RPC failed | scope=login error="pikpak: captcha_required url=https://user.mypikpak.com/captcha/v2/txCaptcha.html token=[REDACTED] session=b76392d98f93266b46ea3ae4a557c3d68149" provider=pikpak
[WARN] PikPak sign-in request failed | source=internal/drive/providers/pikpak/auth.go:509 error="PikPak access was prohibited by provider risk control; retry later"
[WARN] provider login failed | provider=pikpak error="PikPak access was prohibited by provider risk control; retry later" duration=1.595s
[WARN] login form submit failed | scope=login error="PikPak access was prohibited by provider risk control; retry later" provider=pikpak
```

### 1.2 根本原因
- PikPak 官方服务端针对频繁尝试登录、数据中心代理节点 IP（机房 IP）、或者异常设备指纹启用了高敏感度的服务端风控（Risk Control）；
- 当 IP/设备命中黑名单或风控阈值时，即使在前端成功通过了人机滑块，后端的 `sign-in` 请求依然会被服务端拒绝并返回风控错误。

### 1.3 规避与解决方案
1. **停止短时间高频重试**：触发风控后，服务端通常会有数分钟至数小时的 IP 惩罚期，切勿连续点击登录；
2. **更换干净的网络出口**：切换至家庭宽带原生 IP、手机热点，或使用纯净的住宅代理节点重试；
3. **网页端先行验证**：先在浏览器端登录 PikPak 官网，确认账号本身未被官方封禁。

---

## 三、WebDAV 各服务商预设图标与识别状态

| 服务商预设 | 图标资源 | 识别特征 | 状态 |
| :--- | :--- | :--- | :---: |
| 坚果云 (Jianguoyun) | `drive-icons/jianguoyun.svg` | `jianguoyun.com` / 名称含「坚果云」 | ✅ 正常识别 |
| InfiniCLOUD | `drive-icons/infinitycloud.svg` | `infini-cloud.net` / 名称含「InfiniCLOUD」 | ✅ 正常识别 |
| Nextcloud | `drive-icons/nextcloud.svg` | `nextcloud` / `remote.php/dav` | ✅ 正常识别 |
| ownCloud | `drive-icons/owncloud.svg` | `owncloud` | ✅ 正常识别 |
| Seafile | `drive-icons/seafile.svg` | `seafile` / `seafdav` | ✅ 正常识别 |
| OpenList / AList | `drive-icons/openlist.svg` | `openlist` / `alist` | ✅ 正常识别 |
| 群晖 Synology | `drive-icons/synology.svg` | `:5006` / 名称含「群晖」「Synology」 | ✅ 正常识别 |
| Koofr | `drive-icons/koofr.svg` | `koofr.net` / 名称含「Koofr」 | ✅ 正常识别 |
| Yandex Disk | `drive-icons/yandex.svg` | `yandex.com` / `yandex.ru` | ✅ 正常识别 |
| pCloud (EU/US) | `pcloud-eu.svg` / `pcloud-us.svg` | `ewebdav.pcloud.com` / `webdav.pcloud.com` | ✅ 正常识别 |
| 自定义 WebDAV | `drive-icons/webdav.svg` | 通用兜底 | ✅ 正常 |
