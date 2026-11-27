// 双栏目录对比引擎（O(N) 哈希表比对，零深拷贝，支持状态汇总）

function sameTrustedHash(left, right) {
  if (!left || !right) return null
  const leftAlgo = String(left.content_hash_name || '').trim().toLowerCase()
  const rightAlgo = String(right.content_hash_name || '').trim().toLowerCase()
  const leftHash = String(left.content_hash || '').trim().toLowerCase()
  const rightHash = String(right.content_hash || '').trim().toLowerCase()
  if (!leftAlgo || !rightAlgo || !leftHash || !rightHash || leftAlgo !== rightAlgo) return null
  return leftHash === rightHash
}

export function compareDirectoryLists(leftFiles = [], rightFiles = []) {
  const leftMap = new Map()
  const rightMap = new Map()
  for (const file of leftFiles) {
    if (file?.name) leftMap.set(file.name, file)
  }
  for (const file of rightFiles) {
    if (file?.name) rightMap.set(file.name, file)
  }

  const names = new Set([...leftMap.keys(), ...rightMap.keys()])
  const rows = []
  const counts = { leftOnly: 0, rightOnly: 0, different: 0, hashMatch: 0, sameMeta: 0 }

  for (const name of names) {
    const left = leftMap.get(name) || null
    const right = rightMap.get(name) || null
    let status
    let detail
    if (left && !right) {
      status = 'leftOnly'
      detail = '仅左栏存在'
    } else if (!left && right) {
      status = 'rightOnly'
      detail = '仅右栏存在'
    } else if (!!left.isDir !== !!right.isDir) {
      status = 'different'
      detail = '类型不同（文件 / 文件夹）'
    } else if (left.isDir) {
      status = 'sameMeta'
      detail = '同名文件夹（未递归扫描）'
    } else {
      const hashResult = sameTrustedHash(left, right)
      if (hashResult === true) {
        status = 'hashMatch'
        detail = `${String(left.content_hash_name).toUpperCase()} 哈希一致`
      } else if (hashResult === false) {
        status = 'different'
        detail = `${String(left.content_hash_name).toUpperCase()} 哈希不一致`
      } else if (left.size !== right.size) {
        status = 'different'
        detail = '文件大小不同'
      } else {
        status = 'sameMeta'
        detail = '名称与大小一致（未比对哈希）'
      }
    }
    counts[status]++
    rows.push({ name, status, detail, left, right, isDir: !!(left?.isDir || right?.isDir) })
  }

  const order = { leftOnly: 0, rightOnly: 1, different: 2, sameMeta: 3, hashMatch: 4 }
  rows.sort((a, b) => (order[a.status] - order[b.status]) || (Number(b.isDir) - Number(a.isDir)) || a.name.localeCompare(b.name, 'zh-CN', { numeric: true }))
  return { rows, counts }
}
