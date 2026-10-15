const fs = require('node:fs')
const path = require('node:path')
const crypto = require('node:crypto')
const childProcess = require('node:child_process')

function packageName(location) {
  return location.split('node_modules/').at(-1)
}

function validateSources(manifest, lock, policy) {
  const approved = new Map((policy.externalPackages || []).map(entry => [entry.name, entry]))
  const registries = new Set(policy.registries || [])
  const packages = lock.packages
  if (lock.lockfileVersion !== 3 || !packages?.['']) throw new Error('必须使用完整的 npm v3 锁文件')
  for (const field of ['dependencies', 'devDependencies', 'optionalDependencies']) {
    const expected = Object.entries(manifest[field] || {}).sort(([left], [right]) => left.localeCompare(right))
    const actual = Object.entries(packages[''][field] || {}).sort(([left], [right]) => left.localeCompare(right))
    if (JSON.stringify(expected) !== JSON.stringify(actual)) throw new Error(`清单与锁文件根依赖不一致：${field}`)
  }
  for (const field of ['name', 'version', 'license']) {
    if (manifest[field] !== packages[''][field]) throw new Error(`清单与锁文件根元数据不一致：${field}`)
  }
  for (const [name, version] of Object.entries({ ...manifest.dependencies, ...manifest.devDependencies, ...manifest.optionalDependencies })) {
    if (/^(?:https?:|git|file:|link:|workspace:|github:)/i.test(version)) {
      if (!approved.has(name) || approved.get(name).resolved !== version) throw new Error(`未批准的直接依赖来源：${name}`)
    }
    if (!packages[`node_modules/${name}`]) throw new Error(`锁文件缺少直接依赖：${name}`)
  }
  for (const [location, entry] of Object.entries(packages)) {
    if (!location) continue
    if (!location.startsWith('node_modules/') || location.includes('\\') || location.split('/').some(part => part === '..') || entry.link) throw new Error(`不允许本地替换依赖：${location}`)
    const name = packageName(location)
    if (!entry.version || !entry.resolved || !/^sha512-[A-Za-z0-9+/]{86}==$/.test(entry.integrity || '')) throw new Error(`依赖缺少固定版本、来源或 SHA-512：${name}`)
    const source = new URL(entry.resolved)
    if (source.protocol !== 'https:' || source.username || source.password || source.port || source.hash || source.search) throw new Error(`依赖来源必须为无凭据 HTTPS：${name}`)
    if (!registries.has(source.origin)) {
      const expected = approved.get(name)
      if (!expected || entry.version !== expected.version || entry.resolved !== expected.resolved || entry.integrity !== expected.integrity || entry.license !== expected.license) {
        throw new Error(`非注册表依赖与批准清单不符：${name}`)
      }
    } else if (approved.has(name)) {
      throw new Error(`专门批准的依赖不得静默切换来源：${name}`)
    }
  }
  for (const entry of approved.values()) {
    if (!packages[`node_modules/${entry.name}`]) throw new Error(`批准清单存在未使用依赖：${entry.name}`)
  }
  return Object.keys(packages).length - 1
}

function run(command, args, cwd) {
  return childProcess.execFileSync(command, args, { cwd, encoding: 'utf8', maxBuffer: 32 * 1024 * 1024 }).trim()
}

function digest(file) {
  return crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex')
}

function licenseFiles(directory, relative = '', depth = 0) {
  const result = []
  for (const entry of fs.readdirSync(path.join(directory, relative), { withFileTypes: true })) {
    const candidate = path.join(relative, entry.name)
    if (entry.isFile() && /^(?:licen[sc]e|copying|notice|copyright|authors)(?:[._-].*)?$/i.test(entry.name)) result.push(candidate)
    if (depth === 0 && entry.isFile() && /^readme(?:[._-].*)?$/i.test(entry.name) && /permission is hereby granted|redistribution and use in source and binary forms/i.test(fs.readFileSync(path.join(directory, candidate), 'utf8'))) result.push(candidate)
    if (entry.isDirectory() && depth < 3 && (depth > 0 || /^(?:licenses?|licences?|notices?)$/i.test(entry.name))) {
      result.push(...licenseFiles(directory, candidate, depth + 1))
    } else if (depth > 0 && entry.isFile() && !result.includes(candidate)) {
      result.push(candidate)
    }
  }
  return result.sort()
}

function archiveLicenses(directory, identity, output) {
  const folder = crypto.createHash('sha256').update(identity).digest('hex').slice(0, 24)
  const collected = []
  for (const file of licenseFiles(directory)) {
    const relative = path.join('licenses', folder, file)
    fs.mkdirSync(path.dirname(path.join(output, relative)), { recursive: true })
    fs.copyFileSync(path.join(directory, file), path.join(output, relative), fs.constants.COPYFILE_EXCL)
    collected.push({ path: relative.replaceAll(path.sep, '/'), sha256: digest(path.join(output, relative)) })
  }
  return collected
}

function collectInventory(root, output) {
  if (fs.existsSync(output) && fs.readdirSync(output).length) throw new Error('归档目录必须为空，不覆盖已有资产')
  fs.mkdirSync(output, { recursive: true })
  const manifest = JSON.parse(fs.readFileSync(path.join(root, 'frontend/package.json'), 'utf8'))
  const lock = JSON.parse(fs.readFileSync(path.join(root, 'frontend/package-lock.json'), 'utf8'))
  const policy = JSON.parse(fs.readFileSync(path.join(root, 'tools/supply-chain-policy.json'), 'utf8'))
  validateSources(manifest, lock, policy)
  const components = []
  for (const [location, entry] of Object.entries(lock.packages)) {
    if (!location || entry.dev) continue
    const name = packageName(location)
    const directory = path.join(root, 'frontend', location)
    const installed = fs.existsSync(path.join(directory, 'package.json'))
    if (!installed && !entry.optional) throw new Error(`生产依赖未安装：${name}`)
    if (installed && JSON.parse(fs.readFileSync(path.join(directory, 'package.json'), 'utf8')).version !== entry.version) throw new Error(`已安装版本与锁文件不符：${name}`)
    const files = installed ? archiveLicenses(directory, `npm:${location}@${entry.version}`, output) : []
    let licenseSupplement
    if (installed && !files.length && /^@napi-rs\/canvas(?:-[a-z0-9-]+)?$/.test(name) && entry.version === '1.0.9') {
      const supplementDir = path.join(root, 'third_party/canvas-1.0.9')
      const provenance = JSON.parse(fs.readFileSync(path.join(supplementDir, 'provenance.json'), 'utf8'))
      if (digest(path.join(supplementDir, 'LICENSE')) !== provenance.sha256) throw new Error('Canvas 上游许可摘要不符')
      files.push(...archiveLicenses(supplementDir, `upstream:${name}@${entry.version}`, output))
      licenseSupplement = { source: provenance.source, revision: provenance.revision }
    }
    components.push({ ecosystem: 'npm', name, version: entry.version, source: entry.resolved, integrity: entry.integrity, declaredLicense: entry.license || 'UNKNOWN', installed, licenseFiles: files, licenseSupplement })
  }
  const modules = run('go', ['list', '-m', '-json', 'all'], root).split(/\r?\n(?=\{)/).map(document => JSON.parse(document))
  for (const entry of modules) {
    if (entry.Main) continue
    if (entry.Replace) throw new Error(`发布清单拒绝模块替换：${entry.Path}`)
    if (!entry.Dir || !entry.Sum) throw new Error(`Go 模块未完整下载：${entry.Path}；先运行 go mod download`)
    components.push({ ecosystem: 'go', name: entry.Path, version: entry.Version, integrity: entry.Sum, source: `https://${entry.Path}`, licenseFiles: archiveLicenses(entry.Dir, `go:${entry.Path}@${entry.Version}`, output) })
  }
  const inputs = ['go.mod', 'go.sum', 'frontend/package.json', 'frontend/package-lock.json', 'tools/supply-chain-policy.json', 'LICENSE', 'third_party/canvas-1.0.9/LICENSE', 'third_party/canvas-1.0.9/provenance.json'].map(file => ({ path: file, sha256: digest(path.join(root, file)) }))
  fs.copyFileSync(path.join(root, 'LICENSE'), path.join(output, 'LICENSE'), fs.constants.COPYFILE_EXCL)
  const inventory = { format: 'mnemo-dependency-inventory', version: 1, commit: run('git', ['rev-parse', 'HEAD'], root), projectVersion: manifest.version, projectLicense: 'GPL-3.0-only', scope: 'Go 模块整体依赖与前端生产依赖；不是每种平台二进制的精确链接清单', tools: { node: process.version, go: run('go', ['version'], root) }, inputs, components }
  fs.writeFileSync(path.join(output, 'dependency-inventory.json'), JSON.stringify(inventory, null, 2) + '\n', { flag: 'wx' })
  const notices = ['Mnemo 第三方依赖归档', 'Mnemo 自有代码按根目录 GNU GPL v3 分发；第三方代码保留各自许可证与署名。', `对应源代码：https://github.com/lllll081926i/Mnemo/tree/${inventory.commit}`, '自动扫描不等同于法律审查。没有找到许可证文本的组件明确标记待人工核对。', '', ...components.flatMap(component => [`${component.ecosystem}:${component.name}@${component.version}`, `来源：${component.source}`, `完整性：${component.integrity}`, `声明许可证：${component.declaredLicense || '见原始文件与 SBOM 检测证据'}`, ...component.licenseFiles.map(file => `许可证/署名：${file.path}`), ...(component.licenseFiles.length ? [] : ['警告：未归档许可证文本，需要人工核对；未安装的可选组件不代表当前平台已分发。']), ''])]
  fs.writeFileSync(path.join(output, 'THIRD_PARTY_NOTICES.txt'), notices.join('\n'), { flag: 'wx' })
  return inventory
}

if (require.main === module) {
  try {
    const root = path.resolve(__dirname, '..')
    if (process.argv[2] === 'collect' && process.argv[3]) {
      const inventory = collectInventory(root, path.resolve(process.argv[3]))
      console.log(`已归档 ${inventory.components.length} 个依赖；缺少原始许可文本 ${inventory.components.filter(component => component.installed !== false && !component.licenseFiles.length).length} 个，详见清单`)
    } else if (process.argv[2] === 'check') {
      const count = validateSources(JSON.parse(fs.readFileSync(path.join(root, 'frontend/package.json'), 'utf8')), JSON.parse(fs.readFileSync(path.join(root, 'frontend/package-lock.json'), 'utf8')), JSON.parse(fs.readFileSync(path.join(root, 'tools/supply-chain-policy.json'), 'utf8')))
      console.log(`依赖来源检查通过：${count} 项`)
    } else {
      throw new Error('用法：node tools/supply-chain.cjs check | collect <空目录>')
    }
  } catch (error) {
    console.error(error.message)
    process.exitCode = 1
  }
}

module.exports = { validateSources, licenseFiles, archiveLicenses, collectInventory }
