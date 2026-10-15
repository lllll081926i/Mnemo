# 依赖归档与发布验证

## 许可范围

Mnemo 自有代码按照根目录 `LICENSE` 的 GNU GPL v3 分发，包元数据使用 `GPL-3.0-only`。第三方依赖保留各自许可证和署名，不因被打包而改为 Mnemo 的许可证。自动扫描不是法律审查，也不是所有依赖许可兼容性的结论。

发布流程生成 `Mnemo-compliance.tar.gz`，包含：

- 根许可证 `LICENSE`。
- `dependency-inventory.json`：源码提交、输入文件 SHA-256、Go 模块整体依赖和前端生产依赖的版本、来源、完整性与原始许可文件位置。
- `THIRD_PARTY_NOTICES.txt`：可读清单与未找到原始许可文本的警告。
- `licenses/`：原样复制的 LICENSE、NOTICE、COPYING 等文件；README 含许可正文时也归档。
- Go 和前端的 CycloneDX SBOM。检测到的许可证标识属于证据，不替代原始许可文件。

清单包含整体依赖图，不代表每个平台安装包精确链接的组件集合。未安装的可选平台组件明确标记为未安装，不假称已归档其许可文件。所有平台包同时携带归档，Windows 位于安装目录 `licenses/`，Linux 原生包位于 `/usr/share/doc/mnemo/`，便携包位于 `licenses/`，macOS 位于应用资源目录 `licenses/`。

`@napi-rs/canvas` 的 1.0.9 平台包缺少许可正文时，使用同版本源码提交 `b2723ffae4e74e8c9df752902b137ec4061530e9` 的原始 LICENSE；来源与 SHA-256 固定在 `third_party/canvas-1.0.9/provenance.json`，归档前核对摘要，并在清单中记录补充来源。

`abslink@1.2.2`、`lfa-ponyfill@1.1.1`、`throughput@1.0.2` 的发布包仍未包含可识别的许可正文，复核上游默认分支也没有独立 LICENSE。保留版本、声明许可证和缺项告警，不编造署名或用通用许可证正文冒充上游原文。这三项分发义务仍需上游提供许可证据或人工确认。其他平台可选包以实际归档为准。

## 依赖来源和安装

运行以下检查，无需安装额外 Node 依赖：

```powershell
node --test tools/supply-chain.test.cjs
node tools/supply-chain.cjs check
```

`tools/supply-chain-policy.json` 明确允许的注册表和外部 tarball。检查要求 v3 锁文件、固定版本和 SHA-512，拒绝本地替换、弱摘要、带凭据的地址和未知外部来源。根依赖与元数据必须和包清单一致。SheetJS 的版本、地址、摘要和声明许可证必须全部匹配批准项，不能静默切回另一个包源。

前端和 Wails 的安装入口均禁用依赖安装脚本。原因之一是 `lfa-ponyfill` 的 postinstall 会从网络更新字体列表；构建使用锁定 tarball 自带的列表，不在安装阶段改写它。需要启用新的安装脚本时，必须先审查具体脚本及其网络输入。

云端在安装前检查来源，再运行 `npm ci --ignore-scripts`。Node 固定为 24.22.0；Go 固定为 1.27.2，设置 `GOTOOLCHAIN=local`，不自动下载另一套工具链。前端完整依赖审计和 Go 可达漏洞审计均属于质量门槛。`npm audit` 对外部 tarball 的覆盖有限，零公告不等于所有外部代码已经安全审计。

已用 Node 24.22.0 验证干净安装、测试、生产构建，以及已缓存依赖的 `npm ci --offline --ignore-scripts` 后重建。首次构建仍需要依赖源；这不是完整离线供应镜像，也不承诺跨平台安装包逐字节可重复。来源不可用时允许失败，不自动换源或放宽摘要。

生成本机归档时，使用空输出目录：

```powershell
$env:GOTOOLCHAIN = 'local'
go mod download all
node tools/supply-chain.cjs collect <空目录>
```

归档拒绝覆盖已有资产。完整 CI 另外生成 SBOM，并检查锁文件没有被构建改写。AppImage 打包工具执行前校验固定 SHA-256；上游 continuous 附件变化会使构建失败，需要重新核对后更新摘要。

## 发布来源证明

SHA-256 能检测安装包与校验清单是否一致，不能单独证明发布者身份。应用内更新目前核对官方仓库、文件大小和 SHA-256，**没有内置验证发布者签名**。更新弹窗明确显示这条边界。

新发布流程为所有安装包、合规归档和 SHA-256 清单生成 GitHub Artifact Attestation。只有标签发布任务获得写权限和 OIDC 权限；普通 PR 不获得这些权限。证明生成失败时不公开 Release。该流程尚需下一次云端构建验收，不为旧版本补称存在证明。

先取得预期版本的源码提交 SHA。下载对应安装包后，用 GitHub CLI 独立核验，例如：

```powershell
gh attestation verify .\Mnemo-windows-x64-Setup.exe --repo lllll081926i/Mnemo --signer-workflow lllll081926i/Mnemo/.github/workflows/release.yml --source-ref refs/tags/vX.Y.Z --source-digest <预期提交SHA> --deny-self-hosted-runners
```

替换版本、文件名和预期提交。验证失败时不要运行安装包。证明绑定文件摘要、仓库、工作流、源码引用和提交；不能把结果当作 Windows Authenticode、macOS Developer ID 或公证。拥有仓库和工作流控制权的攻击者仍在此信任模型内，来源证明不替代代码审查和发布权限保护。

参考：[GitHub CLI 验证说明](https://cli.github.com/manual/gh_attestation_verify)、[GitHub 官方证明 Action](https://github.com/actions/attest)、[CycloneDX Go 工具](https://github.com/CycloneDX/cyclonedx-gomod)、[npm SBOM](https://docs.npmjs.com/cli/v11/commands/npm-sbom)。
