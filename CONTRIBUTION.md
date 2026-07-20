### Mnemov3版本源码帮助

v3采用 ts + vue3 + vite + electron 模板开发

#### 1.下载源代码

```
https://github.com/lllll081926i/Mnemo.git
```

#### 2.打开代码目录，安装依赖

```cmd
npm install
npm config set registry https://registry.npmmirror.com
```

#### 3.环境配置

##### 3.1 配置网盘 API 密钥

以 `.env.example` 为模板创建 `.env.local`，按需配置现有提供方的密钥：

```dotenv
ALIYUN_APP_ID=
ALIYUN_APP_SECRET=
PIKPAK_CLIENT_ID=
PIKPAK_CLIENT_SECRET=
GUANGYA_CLIENT_ID=
CLOUD189_APP_ID=
```

然后生成本地密钥模块：

```cmd
npm run secrets:generate
```

生成的 `src/secrets.generated.ts` 和 `.env.local` 均不会提交到 Git。请从对应服务商的官方开发者平台获取密钥，并妥善保管。

#### 4.开发调试运行

```cmd
npm run dev
```

执行命令后会调起electron窗口，配合vscode正常开发调试即可

#### 5.打包

```cmd
npm run build:electron
```

#### 6.密钥生成命令

项目提供了便捷的配置管理命令：

```cmd
npm run secrets:generate
```

#### 7.macOS 签名打包（可选）

如需在 macOS 上进行代码签名，需要在 `.env.local` 中配置 Apple 相关信息，然后使用：

```cmd
npm run build:mac:signed
```

或者构建所有平台（包含签名版本）：

```cmd
npm run build:all
```

#### 8.项目结构说明

```
├── electron/           # Electron 主进程和预加载脚本
├── src/
│   ├── components/     # Vue 组件
│   ├── config.ts       # 通用运行配置
│   ├── store/         # Pinia 状态管理
│   ├── utils/         # 工具函数
│   ├── aliapi/        # 阿里云盘 API
│   ├── pikpak/        # PikPak API
│   ├── quark/         # 夸克网盘 API
│   ├── cloud139/      # 中国移动云盘 API
│   ├── cloud189/      # 天翼云盘 API
│   └── guangya/       # 光鸭 API
├── scripts/           # 构建和配置脚本
├── .env.example       # 环境变量示例文件（已弃用）
├── vite.config.ts     # Vite 配置
└── package.json       # 项目依赖配置
```

#### 9.常见问题

**Q: 启动时提示网盘 API 配置错误？**
A: 请检查 `.env.local` 中相应配置，并重新执行 `npm run secrets:generate`

**Q: 为什么仓库里没有真实密钥？**
A: 真实密钥只保存在 `.env.local` 和本地生成的 `src/secrets.generated.ts` 中，不进入 Git

**Q: 如何添加新的网盘支持？**
A: 参考现有网盘 API 实现，在对应目录下添加新的 API 模块

**Q: 打包后的应用无法正常使用网盘功能？**
A: 确保构建环境已注入所需密钥，并在构建前生成 `src/secrets.generated.ts`

#### 10.贡献代码

欢迎提交 Issue 和 Pull Request！

1. Fork 本仓库
2. 创建功能分支 (`git checkout -b feature/amazing-feature`)
3. 提交更改 (`git commit -m 'Add some amazing feature'`)
4. 推送到分支 (`git push origin feature/amazing-feature`)
5. 创建 Pull Request
