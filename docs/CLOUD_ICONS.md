# 网盘与存储 SVG 图标库

日期：2026-10-30。

## 使用范围

图标保存在 `frontend/src/assets/drive-icons/`。目录由 `frontend/src/drive-icons.js` 维护，账号“自定义”窗口支持中文、英文与文件名搜索。内置图标由 Vite 打包，不依赖远程图标服务。

本轮新增 31 个 SVG：19 个品牌图标与 12 个原创通用图标。连同原有 24 个，共 55 个。补上原先未进入选择器的迅雷图标。新增图标不新增登录入口、网盘驱动或协议能力；不会恢复已移除的 Google Drive、GoFile、一刻相册或加密流功能。

打开 [图标明暗预览](cloud-icons-preview.html) 可查看全部图标。

## 项目参考

- [OpenList 前端](https://github.com/OpenListTeam/OpenList-Frontend)：检查文件类型图标与存储驱动的组织方式；Mnemo 延续“业务名称映射 + 本地图标”的现有契约。
- [Rclone Web UI](https://github.com/rclone/rclone-webui-react)：参考多存储管理项目的资产组织，不导入该项目代码。
- [Cloudreve 自定义前端说明](https://docs.cloudreve.org/en/usage/custom-frontend)：核对图标与静态资源本地内嵌的使用方式；Mnemo 不新增在线 CDN 依赖。
- [Simple Icons](https://github.com/simple-icons/simple-icons)：选取网盘、NAS 和对象存储品牌路径，补全外观选择范围。

## 品牌图标来源

19 个图标路径来自 Simple Icons，固定提交 [90646e4bf7f3b89df6c37e65baf3e7bc759a517c](https://github.com/simple-icons/simple-icons/tree/90646e4bf7f3b89df6c37e65baf3e7bc759a517c)。原路径统一放入 64×64 画布，添加品牌色圆角底板与白色标记。品牌形状没有重新描绘。上游图标库使用 CC0-1.0，许可全文见 [CC0 许可](licenses/simple-icons-CC0-1.0.txt)。品牌名称与标记仍归各自所有者所有，不代表合作关系。

| 本地图标 | 名称 | 路径来源 | 上游记录的品牌来源 |
| --- | --- | --- | --- |
| `box.svg` | Box | [Simple Icons](https://github.com/simple-icons/simple-icons/blob/90646e4bf7f3b89df6c37e65baf3e7bc759a517c/icons/box.svg) | [来源](https://www.box.com/en-gb/about-us/press) |
| `mega.svg` | MEGA | [Simple Icons](https://github.com/simple-icons/simple-icons/blob/90646e4bf7f3b89df6c37e65baf3e7bc759a517c/icons/mega.svg) | [来源](https://mega.io/media) |
| `protondrive.svg` | Proton Drive | [Simple Icons](https://github.com/simple-icons/simple-icons/blob/90646e4bf7f3b89df6c37e65baf3e7bc759a517c/icons/protondrive.svg) | [来源](https://proton.me/media/kit) |
| `icloud.svg` | iCloud | [Simple Icons](https://github.com/simple-icons/simple-icons/blob/90646e4bf7f3b89df6c37e65baf3e7bc759a517c/icons/icloud.svg) | [来源](https://commons.wikimedia.org/wiki/File:ICloud_logo.svg) |
| `tresorit.svg` | Tresorit | [Simple Icons](https://github.com/simple-icons/simple-icons/blob/90646e4bf7f3b89df6c37e65baf3e7bc759a517c/icons/tresorit.svg) | [来源](https://tresorit.com/press) |
| `filen.svg` | Filen | [Simple Icons](https://github.com/simple-icons/simple-icons/blob/90646e4bf7f3b89df6c37e65baf3e7bc759a517c/icons/filen.svg) | [来源](https://github.com/FilenCloudDienste/filen-drive-legacy/issues/166#issue-2139393162) |
| `backblaze.svg` | Backblaze B2 | [Simple Icons](https://github.com/simple-icons/simple-icons/blob/90646e4bf7f3b89df6c37e65baf3e7bc759a517c/icons/backblaze.svg) | [来源](https://www.backblaze.com/company/news.html) |
| `cloudflare.svg` | Cloudflare R2 | [Simple Icons](https://github.com/simple-icons/simple-icons/blob/90646e4bf7f3b89df6c37e65baf3e7bc759a517c/icons/cloudflare.svg) | [来源](https://www.cloudflare.com/logo/) |
| `wasabi.svg` | Wasabi | [Simple Icons](https://github.com/simple-icons/simple-icons/blob/90646e4bf7f3b89df6c37e65baf3e7bc759a517c/icons/wasabi.svg) | [来源](https://wasabi.com) |
| `minio.svg` | MinIO | [Simple Icons](https://github.com/simple-icons/simple-icons/blob/90646e4bf7f3b89df6c37e65baf3e7bc759a517c/icons/minio.svg) | [来源](https://min.io) |
| `digitalocean.svg` | DigitalOcean Spaces | [Simple Icons](https://github.com/simple-icons/simple-icons/blob/90646e4bf7f3b89df6c37e65baf3e7bc759a517c/icons/digitalocean.svg) | [来源](https://www.digitalocean.com/press/) |
| `qnap.svg` | QNAP 威联通 | [Simple Icons](https://github.com/simple-icons/simple-icons/blob/90646e4bf7f3b89df6c37e65baf3e7bc759a517c/icons/qnap.svg) | [来源](https://marketing.qnap.com/resource/qnap-logo-k100) |
| `truenas.svg` | TrueNAS | [Simple Icons](https://github.com/simple-icons/simple-icons/blob/90646e4bf7f3b89df6c37e65baf3e7bc759a517c/icons/truenas.svg) | [来源](https://www.truenas.com) |
| `hetzner.svg` | Hetzner Storage | [Simple Icons](https://github.com/simple-icons/simple-icons/blob/90646e4bf7f3b89df6c37e65baf3e7bc759a517c/icons/hetzner.svg) | [来源](https://www.hetzner.com) |
| `ovh.svg` | OVHcloud | [Simple Icons](https://github.com/simple-icons/simple-icons/blob/90646e4bf7f3b89df6c37e65baf3e7bc759a517c/icons/ovh.svg) | [来源](https://www.ovh.com/ca/en/newsroom/) |
| `syncthing.svg` | Syncthing | [Simple Icons](https://github.com/simple-icons/simple-icons/blob/90646e4bf7f3b89df6c37e65baf3e7bc759a517c/icons/syncthing.svg) | [来源](https://github.com/syncthing/syncthing/blob/bdfd0f0548d2f6fc4b5500690dbd383baa3b0561/assets/logo-only.svg) |
| `rclone.svg` | Rclone | [Simple Icons](https://github.com/simple-icons/simple-icons/blob/90646e4bf7f3b89df6c37e65baf3e7bc759a517c/icons/rclone.svg) | [来源](https://github.com/rclone/rclone/blob/8f1c309c8149a734ccc3a0d2ce185b936dbe783a/graphics/logo/svg/logo_symbol_color.svg) |
| `ceph.svg` | Ceph | [Simple Icons](https://github.com/simple-icons/simple-icons/blob/90646e4bf7f3b89df6c37e65baf3e7bc759a517c/icons/ceph.svg) | [来源](https://github.com/ceph/ceph/blob/b106a03dcddaee80493825e85bc5e399ab4d8746/src/pybind/mgr/dashboard/frontend/src/assets/Ceph_Logo.svg) |
| `scaleway.svg` | Scaleway | [Simple Icons](https://github.com/simple-icons/simple-icons/blob/90646e4bf7f3b89df6c37e65baf3e7bc759a517c/icons/scaleway.svg) | [来源](https://ultraviolet.scaleway.com) |

## 原创图标

以下图标在本项目直接使用 SVG 路径绘制，沿用项目许可。使用 64×64 画布、60×60 圆角底板、统一留白及描边。个人云、团队云、共享云分别用单人、多人、连接节点区分；NAS、传输协议与存储用途分别使用房屋、终端、目录网络、时钟、归档盒、存储桶和服务器形状。原创图标不冒充任何厂商标志。

| 文件 | 用途 |
| --- | --- |
| `cloud-personal.svg` | 个人云 |
| `cloud-team.svg` | 团队云 |
| `cloud-shared.svg` | 共享云 |
| `home-nas.svg` | 家庭 NAS |
| `sftp.svg` | SFTP |
| `ftp.svg` | FTP |
| `smb.svg` | SMB 网络共享 |
| `backup-storage.svg` | 备份存储 |
| `archive-storage.svg` | 归档存储 |
| `object-storage.svg` | 对象存储 |
| `remote-server.svg` | 远程服务器 |
| `local-folder.svg` | 本地文件夹 |

## 维护要求

- 图标只包含本地矢量图形，不加入脚本、远程引用、嵌入位图或字体依赖。
- 新增图标同步维护目录与来源说明。品牌图标保留真实形状，通用图标清楚标记为原创。
- 原有 24 个图标沿用现有文件；本次不改变其来源或许可声明。
