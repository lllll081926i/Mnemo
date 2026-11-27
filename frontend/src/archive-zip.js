// 零依赖 HTTP Range ZIP 浏览器解析器：
// 仅按需读取末尾 EOCD + Central Directory，支持数百 MB ~ 数十 GB 的 ZIP 秒开，
// 并支持利用浏览器原生 DecompressionStream 按需解压提取单个文件。

const EOCD_SIG = 0x06054b50
const ZIP64_EOCD_LOC_SIG = 0x07064b50
const ZIP64_EOCD_SIG = 0x06064b50
const CD_ENTRY_SIG = 0x02014b50
const LOCAL_HEADER_SIG = 0x04034b50
const MAX_EOCD_TAIL = 65536 + 22 + 96
const MAX_CD_BYTES = 32 * 1024 * 1024
const MAX_ENTRIES = 50_000
const MAX_EXTRACT_BYTES = 128 * 1024 * 1024

function decodeFilename(bytes, isUtf8) {
  if (isUtf8) return new TextDecoder('utf-8', { fatal: false }).decode(bytes)
  try {
    const utf8 = new TextDecoder('utf-8', { fatal: true }).decode(bytes)
    return utf8
  } catch {
    try { return new TextDecoder('gbk').decode(bytes) } catch { return new TextDecoder('utf-8', { fatal: false }).decode(bytes) }
  }
}

function dosDateTime(dosDate, dosTime) {
  if (!dosDate && !dosTime) return 0
  const year = ((dosDate >> 9) & 0x7f) + 1980
  const month = Math.max(1, ((dosDate >> 5) & 0x0f))
  const day = Math.max(1, dosDate & 0x1f)
  const hour = (dosTime >> 11) & 0x1f
  const minute = (dosTime >> 5) & 0x3f
  const second = (dosTime & 0x1f) * 2
  const ts = Date.UTC(year, month - 1, day, hour, minute, second)
  return Number.isFinite(ts) ? Math.floor(ts / 1000) : 0
}

function readUint64(view, offset) {
  const lo = view.getUint32(offset, true)
  const hi = view.getUint32(offset + 4, true)
  return hi * 4294967296 + lo
}

async function fetchRange(url, start, end, signal) {
  const resp = await fetch(url, {
    headers: { Range: `bytes=${start}-${end}` },
    signal,
  })
  if (resp.status !== 206 && resp.status !== 200) {
    throw new Error(`读取压缩包分片失败（HTTP ${resp.status}）`)
  }
  const buf = new Uint8Array(await resp.arrayBuffer())
  if (resp.status === 200) {
    // 上游不支持 Range 返回完整包体时直接切片
    return { bytes: buf.subarray(start, Math.min(buf.length, end + 1)), total: buf.length, fullBuffer: buf }
  }
  const cr = resp.headers.get('Content-Range') || ''
  const match = cr.match(/bytes\s+(\d+)-(\d+)\/(\d+)/i)
  const total = match ? Number(match[3]) : 0
  return { bytes: buf, total, fullBuffer: null }
}

function parseZip64Extra(view, extraStart, extraLen, needUncomp, needComp, needOffset) {
  let pos = extraStart
  const limit = extraStart + extraLen
  let uncompressedSize = null
  let compressedSize = null
  let localHeaderOffset = null
  while (pos + 4 <= limit) {
    const id = view.getUint16(pos, true)
    const sz = view.getUint16(pos + 2, true)
    pos += 4
    if (pos + sz > limit) break
    if (id === 0x0001) {
      let cur = pos
      if (needUncomp && cur + 8 <= pos + sz) { uncompressedSize = readUint64(view, cur); cur += 8 }
      if (needComp && cur + 8 <= pos + sz) { compressedSize = readUint64(view, cur); cur += 8 }
      if (needOffset && cur + 8 <= pos + sz) { localHeaderOffset = readUint64(view, cur) }
      break
    }
    pos += sz
  }
  return { uncompressedSize, compressedSize, localHeaderOffset }
}

export function parseCentralDirectory(cdBytes, expectedCount = Infinity) {
  const view = new DataView(cdBytes.buffer, cdBytes.byteOffset, cdBytes.byteLength)
  const entries = []
  let pos = 0
  while (pos + 46 <= cdBytes.byteLength && entries.length < expectedCount && entries.length < MAX_ENTRIES) {
    if (view.getUint32(pos, true) !== CD_ENTRY_SIG) break
    const flags = view.getUint16(pos + 8, true)
    const method = view.getUint16(pos + 10, true)
    const dosTime = view.getUint16(pos + 12, true)
    const dosDate = view.getUint16(pos + 14, true)
    const crc32 = view.getUint32(pos + 16, true)
    let compressedSize = view.getUint32(pos + 20, true)
    let uncompressedSize = view.getUint32(pos + 24, true)
    const nameLen = view.getUint16(pos + 28, true)
    const extraLen = view.getUint16(pos + 30, true)
    const commentLen = view.getUint16(pos + 32, true)
    let localHeaderOffset = view.getUint32(pos + 42, true)
    const headerEnd = pos + 46 + nameLen + extraLen + commentLen
    if (headerEnd > cdBytes.byteLength) break

    const nameBytes = cdBytes.subarray(pos + 46, pos + 46 + nameLen)
    const rawName = decodeFilename(nameBytes, (flags & 0x0800) !== 0)
    const needUncomp = uncompressedSize === 0xffffffff
    const needComp = compressedSize === 0xffffffff
    const needOffset = localHeaderOffset === 0xffffffff
    if ((needUncomp || needComp || needOffset) && extraLen > 0) {
      const z64 = parseZip64Extra(view, pos + 46 + nameLen, extraLen, needUncomp, needComp, needOffset)
      if (z64.uncompressedSize !== null) uncompressedSize = z64.uncompressedSize
      if (z64.compressedSize !== null) compressedSize = z64.compressedSize
      if (z64.localHeaderOffset !== null) localHeaderOffset = z64.localHeaderOffset
    }

    const normalized = rawName.replace(/\\/g, '/').replace(/^\/+/, '')
    if (normalized && !normalized.split('/').includes('..')) {
      const isDir = normalized.endsWith('/')
      const cleanPath = isDir ? normalized.slice(0, -1) : normalized
      if (cleanPath) {
        entries.push({
          path: cleanPath,
          name: cleanPath.slice(cleanPath.lastIndexOf('/') + 1),
          isDir,
          method,
          crc32,
          compressedSize,
          size: uncompressedSize,
          localHeaderOffset,
          encrypted: (flags & 0x0001) !== 0,
          modifiedAt: dosDateTime(dosDate, dosTime),
        })
      }
    }
    pos = headerEnd
  }
  return entries
}

export async function readZipIndex(url, knownSize, signal) {
  let totalSize = Number(knownSize) || 0
  let fullBuffer = null
  if (totalSize <= 0) {
    const head = await fetch(url, { method: 'HEAD', signal })
    if (head.ok) totalSize = Number(head.headers.get('Content-Length')) || 0
  }
  if (totalSize <= 0) {
    const probe = await fetchRange(url, 0, 0, signal)
    totalSize = probe.total
    fullBuffer = probe.fullBuffer
  }
  if (totalSize < 22) throw new Error('压缩包文件不完整或不是有效的 ZIP 文件')

  const tailLen = Math.min(totalSize, MAX_EOCD_TAIL)
  const tailStart = totalSize - tailLen
  let tailBytes
  if (fullBuffer) {
    tailBytes = fullBuffer.subarray(tailStart, totalSize)
  } else {
    const tailRes = await fetchRange(url, tailStart, totalSize - 1, signal)
    if (tailRes.fullBuffer) {
      fullBuffer = tailRes.fullBuffer
      totalSize = fullBuffer.length
    }
    tailBytes = tailRes.bytes
  }

  const tailView = new DataView(tailBytes.buffer, tailBytes.byteOffset, tailBytes.byteLength)
  let eocdOffset = -1
  for (let i = tailBytes.byteLength - 22; i >= 0; i--) {
    if (tailView.getUint32(i, true) === EOCD_SIG) {
      const commentLen = tailView.getUint16(i + 20, true)
      if (i + 22 + commentLen === tailBytes.byteLength) {
        eocdOffset = i
        break
      }
    }
  }
  if (eocdOffset < 0) throw new Error('未能定位 ZIP 目录尾（可能已损坏或为分卷压缩包）')

  let entryCount = tailView.getUint16(eocdOffset + 10, true)
  let cdSize = tailView.getUint32(eocdOffset + 12, true)
  let cdOffset = tailView.getUint32(eocdOffset + 16, true)

  // 支持 ZIP64
  if (cdOffset === 0xffffffff || cdSize === 0xffffffff || entryCount === 0xffff) {
    const locPos = eocdOffset - 20
    if (locPos < 0 || tailView.getUint32(locPos, true) !== ZIP64_EOCD_LOC_SIG) {
      throw new Error('不支持的 ZIP64 格式')
    }
    const z64Offset = readUint64(tailView, locPos + 8)
    let z64Bytes
    if (z64Offset >= tailStart && z64Offset + 56 <= totalSize) {
      z64Bytes = tailBytes.subarray(z64Offset - tailStart, z64Offset - tailStart + 56)
    } else if (fullBuffer) {
      z64Bytes = fullBuffer.subarray(z64Offset, z64Offset + 56)
    } else {
      z64Bytes = (await fetchRange(url, z64Offset, z64Offset + 55, signal)).bytes
    }
    const z64View = new DataView(z64Bytes.buffer, z64Bytes.byteOffset, z64Bytes.byteLength)
    if (z64View.getUint32(0, true) !== ZIP64_EOCD_SIG) throw new Error('ZIP64 目录头校验失败')
    entryCount = readUint64(z64View, 32)
    cdSize = readUint64(z64View, 40)
    cdOffset = readUint64(z64View, 48)
  }

  if (cdSize <= 0 || cdSize > MAX_CD_BYTES) {
    throw new Error('压缩包目录过大，请下载到本地使用专业解压工具查看')
  }
  if (cdOffset + cdSize > totalSize) {
    throw new Error('压缩包目录越界，文件可能不完整')
  }

  let cdBytes
  if (cdOffset >= tailStart && cdOffset + cdSize <= totalSize) {
    cdBytes = tailBytes.subarray(cdOffset - tailStart, cdOffset - tailStart + cdSize)
  } else if (fullBuffer) {
    cdBytes = fullBuffer.subarray(cdOffset, cdOffset + cdSize)
  } else {
    cdBytes = (await fetchRange(url, cdOffset, cdOffset + cdSize - 1, signal)).bytes
  }

  const entries = parseCentralDirectory(cdBytes, entryCount)
  return {
    totalSize,
    entryCount,
    truncated: entryCount > MAX_ENTRIES,
    entries,
    fullBuffer,
  }
}

export function buildArchiveTree(entries) {
  const root = { path: '', name: '', isDir: true, children: [], childMap: new Map(), size: 0, compressedSize: 0 }
  let totalFiles = 0
  let totalDirs = 0

  function ensureDir(parts) {
    let cur = root
    let acc = ''
    for (const part of parts) {
      acc = acc ? `${acc}/${part}` : part
      let next = cur.childMap.get(part)
      if (!next) {
        next = { path: acc, name: part, isDir: true, children: [], childMap: new Map(), size: 0, compressedSize: 0, modifiedAt: 0 }
        cur.childMap.set(part, next)
        cur.children.push(next)
        totalDirs++
      }
      cur = next
    }
    return cur
  }

  for (const entry of entries) {
    const parts = entry.path.split('/').filter(Boolean)
    if (!parts.length) continue
    if (entry.isDir) {
      const dir = ensureDir(parts)
      if (entry.modifiedAt) dir.modifiedAt = entry.modifiedAt
    } else {
      const parent = ensureDir(parts.slice(0, -1))
      const name = parts[parts.length - 1]
      if (!parent.childMap.has(name)) {
        const node = { ...entry, name, isDir: false }
        parent.childMap.set(name, node)
        parent.children.push(node)
        totalFiles++
      }
    }
  }
  return { root, totalFiles, totalDirs }
}

export async function extractZipEntry(url, entry, fullBuffer, signal) {
  if (entry.isDir) throw new Error('无法提取目录')
  if (entry.encrypted) throw new Error('该条目已加密，暂不支持在线解压，请下载整个压缩包后使用本地解压工具')
  if (entry.method !== 0 && entry.method !== 8) {
    throw new Error(`不支持的压缩算法（Method ${entry.method}），请下载到本地解压`)
  }
  if (entry.size > MAX_EXTRACT_BYTES) {
    throw new Error('单个文件超过 128 MB，请下载整个压缩包到本地解压')
  }

  const headStart = entry.localHeaderOffset
  const headerSlice = fullBuffer
    ? fullBuffer.subarray(headStart, headStart + 30)
    : (await fetchRange(url, headStart, headStart + 29, signal)).bytes
  const hView = new DataView(headerSlice.buffer, headerSlice.byteOffset, headerSlice.byteLength)
  if (hView.getUint32(0, true) !== LOCAL_HEADER_SIG) {
    throw new Error('本地文件头校验失败')
  }
  const nameLen = hView.getUint16(26, true)
  const extraLen = hView.getUint16(28, true)
  const dataStart = headStart + 30 + nameLen + extraLen
  const dataEnd = dataStart + entry.compressedSize - 1

  const rawData = entry.compressedSize === 0
    ? new Uint8Array(0)
    : fullBuffer
      ? fullBuffer.subarray(dataStart, dataStart + entry.compressedSize)
      : (await fetchRange(url, dataStart, dataEnd, signal)).bytes

  if (entry.method === 0) {
    return new Blob([rawData])
  }
  if (typeof DecompressionStream === 'undefined') {
    throw new Error('当前环境不支持原生 Deflate 解压')
  }
  const stream = new Blob([rawData]).stream().pipeThrough(new DecompressionStream('deflate-raw'))
  return await new Response(stream).blob()
}
