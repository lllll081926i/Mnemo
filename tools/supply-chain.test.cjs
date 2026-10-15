const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const os = require('node:os')
const { validateSources, licenseFiles, archiveLicenses } = require('./supply-chain.cjs')

function fixture() {
  const manifest = { dependencies: { xlsx: 'https://cdn.sheetjs.com/xlsx-0.20.3/xlsx-0.20.3.tgz' } }
  const policy = JSON.parse(fs.readFileSync(path.join(__dirname, 'supply-chain-policy.json'), 'utf8'))
  const entry = { ...policy.externalPackages[0] }
  const lock = { lockfileVersion: 3, packages: { '': structuredClone(manifest), 'node_modules/xlsx': entry } }
  return { manifest, policy, lock, entry }
}

test('批准的版本、来源与完整性必须同时匹配', () => {
  const { manifest, policy, lock } = fixture()
  assert.equal(validateSources(manifest, lock, policy), 1)
  for (const field of ['version', 'resolved', 'integrity', 'license']) {
    const changed = structuredClone(lock)
    changed.packages['node_modules/xlsx'][field] += 'changed'
    assert.throws(() => validateSources(manifest, changed, policy))
  }
})

test('不允许新增外部直链、弱摘要、伪造主机或本地路径', () => {
  for (const source of ['http://registry.npmjs.org/pkg.tgz', 'https://registry.npmjs.org.evil.test/pkg.tgz', 'https://user:secret@registry.npmjs.org/pkg.tgz', 'https://registry.npmjs.org/pkg.tgz?sig=secret', 'file:///tmp/pkg']) {
    const { manifest, policy, lock, entry } = fixture()
    lock.packages['node_modules/extra'] = { ...entry, resolved: source }
    assert.throws(() => validateSources(manifest, lock, policy))
  }
  const { manifest, policy, lock, entry } = fixture()
  lock.packages['node_modules/../../outside'] = entry
  assert.throws(() => validateSources(manifest, lock, policy))
  delete lock.packages['node_modules/../../outside']
  lock.packages['node_modules/extra'] = { ...entry, resolved: 'https://registry.npmjs.org/extra.tgz', integrity: 'sha1-weak' }
  assert.throws(() => validateSources(manifest, lock, policy))
  delete lock.packages['node_modules/extra']
  manifest.dependencies.extra = 'https://unknown.test/extra.tgz'
  assert.throws(() => validateSources(manifest, lock, policy))
})

test('清单与锁文件必须保持相同的依赖范围和元数据', () => {
  for (const field of ['dependencies', 'devDependencies', 'optionalDependencies']) {
    const { manifest, policy, lock } = fixture()
    lock.packages[''][field] = { other: '1.0.0' }
    assert.throws(() => validateSources(manifest, lock, policy), /根依赖不一致/)
  }
  for (const field of ['name', 'version', 'license']) {
    const { manifest, policy, lock } = fixture()
    lock.packages[''][field] = 'changed'
    assert.throws(() => validateSources(manifest, lock, policy), /根元数据不一致/)
  }
})

test('归档原始许可证与署名，不复制普通代码和凭据文件', context => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'mnemo-license-test-'))
  context.after(() => fs.rmSync(directory, { recursive: true, force: true }))
  const dependency = path.join(directory, 'dependency')
  const output = path.join(directory, 'output')
  fs.mkdirSync(path.join(dependency, 'LICENSES'), { recursive: true })
  fs.writeFileSync(path.join(dependency, 'LICENSE'), '许可证原文')
  fs.writeFileSync(path.join(dependency, 'NOTICE.txt'), '原始署名')
  fs.writeFileSync(path.join(dependency, 'LICENSES', 'MIT.txt'), '嵌套许可证')
  fs.writeFileSync(path.join(dependency, 'private.key'), '不可归档')
  fs.writeFileSync(path.join(dependency, 'index.js'), '代码')
  assert.equal(licenseFiles(dependency).length, 3)
  const archived = archiveLicenses(dependency, 'npm:example@1.0.0', output)
  assert.equal(archived.length, 3)
  for (const file of archived) {
    assert.match(file.sha256, /^[a-f0-9]{64}$/)
    assert.ok(fs.existsSync(path.join(output, file.path)))
  }
  assert.throws(() => archiveLicenses(dependency, 'npm:example@1.0.0', output))
})

test('归档 README 中的完整许可声明，不收集无关 README', context => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'mnemo-readme-license-'))
  context.after(() => fs.rmSync(directory, { recursive: true, force: true }))
  fs.writeFileSync(path.join(directory, 'README.md'), 'Permission is hereby granted, free of charge, to any person obtaining a copy...')
  fs.writeFileSync(path.join(directory, 'README.zh.md'), '这是普通使用说明，不含许可证正文')
  assert.deepEqual(licenseFiles(directory), ['README.md'])
  const output = path.join(directory, 'archive')
  const files = archiveLicenses(directory, 'npm:readme-license@1.0.0', output)
  assert.equal(files.length, 1)
  assert.equal(fs.readFileSync(path.join(output, files[0].path), 'utf8'), fs.readFileSync(path.join(directory, 'README.md'), 'utf8'))
})
