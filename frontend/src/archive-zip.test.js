import { describe, expect, it, vi } from 'vitest'
import { buildArchiveTree, extractZipEntry, readZipIndex } from './archive-zip'

function buildStoredZip(files) {
  const encoder = new TextEncoder()
  const localParts = []
  const centralParts = []
  let offset = 0
  for (const file of files) {
    const nameBytes = encoder.encode(file.name)
    const data = encoder.encode(file.content || '')
    const local = new Uint8Array(30 + nameBytes.length + data.length)
    const lv = new DataView(local.buffer)
    lv.setUint32(0, 0x04034b50, true)
    lv.setUint16(4, 20, true)
    lv.setUint16(6, 0x0800, true) // UTF-8
    lv.setUint16(8, 0, true) // store
    lv.setUint32(18, data.length, true)
    lv.setUint32(22, data.length, true)
    lv.setUint16(26, nameBytes.length, true)
    local.set(nameBytes, 30)
    local.set(data, 30 + nameBytes.length)
    localParts.push(local)

    const cd = new Uint8Array(46 + nameBytes.length)
    const cv = new DataView(cd.buffer)
    cv.setUint32(0, 0x02014b50, true)
    cv.setUint16(4, 20, true)
    cv.setUint16(6, 20, true)
    cv.setUint16(8, 0x0800, true)
    cv.setUint16(10, 0, true)
    cv.setUint32(20, data.length, true)
    cv.setUint32(24, data.length, true)
    cv.setUint16(28, nameBytes.length, true)
    cv.setUint32(42, offset, true)
    cd.set(nameBytes, 46)
    centralParts.push(cd)
    offset += local.length
  }
  const cdOffset = offset
  const cdSize = centralParts.reduce((sum, part) => sum + part.length, 0)
  const eocd = new Uint8Array(22)
  const ev = new DataView(eocd.buffer)
  ev.setUint32(0, 0x06054b50, true)
  ev.setUint16(8, files.length, true)
  ev.setUint16(10, files.length, true)
  ev.setUint32(12, cdSize, true)
  ev.setUint32(16, cdOffset, true)

  const total = cdOffset + cdSize + 22
  const out = new Uint8Array(total)
  let pos = 0
  for (const part of [...localParts, ...centralParts, eocd]) {
    out.set(part, pos)
    pos += part.length
  }
  return out
}

describe('HTTP Range ZIP 解析与提取', () => {
  it('仅通过 Range 读取 ZIP 目录并提取单个文件内容', async () => {
    const zip = buildStoredZip([
      { name: 'docs/readme.txt', content: 'Hello Mnemo Range ZIP!' },
      { name: 'docs/sub/note.md', content: '# 标题' },
    ])
    const fetchMock = vi.fn(async (_url, init = {}) => {
      const range = init.headers?.Range || ''
      const m = range.match(/bytes=(\d+)-(\d+)/)
      const start = Number(m[1])
      const end = Math.min(zip.length - 1, Number(m[2]))
      const slice = zip.slice(start, end + 1)
      return new Response(slice, {
        status: 206,
        headers: { 'Content-Range': `bytes ${start}-${end}/${zip.length}` },
      })
    })
    vi.stubGlobal('fetch', fetchMock)
    try {
      const index = await readZipIndex('http://127.0.0.1:9999/stream/1?t=abc', zip.length)
      expect(index.entries).toHaveLength(2)
      const tree = buildArchiveTree(index.entries)
      expect(tree.totalFiles).toBe(2)
      expect(tree.totalDirs).toBe(2)
      const target = index.entries.find(e => e.name === 'readme.txt')
      const blob = await extractZipEntry('http://127.0.0.1:9999/stream/1?t=abc', target, index.fullBuffer)
      const content = await new Promise((resolve, reject) => {
        const reader = new FileReader()
        reader.onload = () => resolve(String(reader.result))
        reader.onerror = reject
        reader.readAsText(blob)
      })
      expect(content).toBe('Hello Mnemo Range ZIP!')
    } finally {
      vi.unstubAllGlobals()
    }
  })
})
