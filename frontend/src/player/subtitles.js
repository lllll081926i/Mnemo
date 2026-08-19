// 字幕工具：SRT → WebVTT 文本转换，SUP(PGS 蓝光图形字幕) 解析与 canvas 渲染。
// ASS/SSA 特效字幕由 jassub（libass WASM）直接渲染，不经此模块。
// PGS 规范：90kHz 时间基；段头 13 字节（"PG" magic + PTS + DTS + 类型 + 长度）。

export function srtToVtt(text) {
  let body = String(text || '').replace(/^﻿/, '').trim()
  if (/^WEBVTT/i.test(body)) return body
  body = body.replace(/\r/g, '')
  body = body.replace(/(\d{2}:\d{2}:\d{2}),(\d{1,3})/g, '$1.$2')
  return 'WEBVTT\n\n' + body + '\n'
}

// ---------- SUP / PGS ----------

function newEpoch() {
  return { palette: [], objects: new Map() }
}

function ycbcrToRgba(y, cr, cb, a) {
  return [
    Math.max(0, Math.min(255, y + 1.402 * (cr - 128))),
    Math.max(0, Math.min(255, y - 0.344136 * (cb - 128) - 0.714136 * (cr - 128))),
    Math.max(0, Math.min(255, y + 1.772 * (cb - 128))),
    a,
  ]
}

function attachSet(set, epoch) {
  set.palette = epoch.palette
  set.comps = set.comps
    .map((c) => ({ x: c.x, y: c.y, obj: epoch.objects.get(c.objId) }))
    .filter((c) => c.obj)
  return set
}

// 解析为 display set 列表：{ pts(秒), w, h, palette, comps: [{x, y, obj:{w,h,chunks}}] }
export function parseSup(buffer) {
  const dv = new DataView(buffer)
  const sets = []
  let off = 0
  let epoch = newEpoch()
  let cur = null
  while (off + 13 <= dv.byteLength) {
    if (dv.getUint8(off) !== 0x50 || dv.getUint8(off + 1) !== 0x47) { off++; continue }
    const pts = dv.getUint32(off + 2, false) / 90000
    const type = dv.getUint8(off + 10)
    const size = dv.getUint16(off + 11, false)
    const p = off + 13
    if (p + size > dv.byteLength) break
    switch (type) {
      case 0x14: { // PDS 调色板
        const entries = []
        for (let i = p + 2; i + 5 <= p + size; i += 5) {
          entries[dv.getUint8(i)] = ycbcrToRgba(dv.getUint8(i + 1), dv.getUint8(i + 2), dv.getUint8(i + 3), dv.getUint8(i + 4))
        }
        epoch.palette = entries
        break
      }
      case 0x16: { // PCS 呈现合成
        const state = dv.getUint8(p + 7)
        if (state & 0x80) epoch = newEpoch()
        if (cur) { sets.push(attachSet(cur, epoch)); cur = null }
        cur = { pts, w: dv.getUint16(p, false), h: dv.getUint16(p + 2, false), comps: [] }
        const n = dv.getUint8(p + 10)
        let q = p + 11
        for (let i = 0; i < n && q + 8 <= p + size; i++) {
          const objId = dv.getUint16(q, false)
          const cropped = dv.getUint8(q + 2)
          cur.comps.push({ objId, x: dv.getUint16(q + 3, false), y: dv.getUint16(q + 5, false) })
          q += cropped ? 16 : 8
        }
        break
      }
      case 0x17: { // ODS 对象数据（RLE 位图，可分段）
        // 布局：objId(2) version(1) seqFlag(1) dataLen(3) [width(2) height(2) data]
        const objId = dv.getUint16(p, false)
        const seq = dv.getUint8(p + 3)
        const dataLen = ((dv.getUint8(p + 4) << 16) | dv.getUint16(p + 5, false))
        if (seq & 0x80) {
          epoch.objects.set(objId, {
            w: dv.getUint16(p + 7, false),
            h: dv.getUint16(p + 9, false),
            chunks: [new Uint8Array(buffer, p + 11, Math.max(0, dataLen - 4))],
          })
        } else {
          const o = epoch.objects.get(objId)
          if (o) o.chunks.push(new Uint8Array(buffer, p + 7, dataLen))
        }
        break
      }
      case 0x80: // END：一个 display set 结束（comps 为空代表清屏）
        if (cur) { sets.push(attachSet(cur, epoch)); cur = null }
        break
    }
    off = p + size
  }
  if (cur) sets.push(attachSet(cur, epoch))
  return sets
}

function decodeObject(obj, palette) {
  const { w, h } = obj
  const out = new Uint8ClampedArray(w * h * 4)
  let len = 0
  for (const c of obj.chunks) len += c.length
  const data = new Uint8Array(len)
  let at = 0
  for (const c of obj.chunks) { data.set(c, at); at += c.length }
  let di = 0, x = 0, y = 0
  const run = (count, rgba) => {
    for (let i = 0; i < count && y < h; i++) {
      if (rgba) {
        const p = (y * w + x) * 4
        out[p] = rgba[0]; out[p + 1] = rgba[1]; out[p + 2] = rgba[2]; out[p + 3] = rgba[3]
      }
      if (++x >= w) { x = 0; y++ }
    }
  }
  while (di < data.length && y < h) {
    const b = data[di++]
    if (b !== 0) { run(1, palette[b]); continue }
    if (di >= data.length) break
    const n = data[di++]
    if (n === 0) { x = 0; y++; continue }
    const flag = n & 0xC0
    if (flag === 0) { run(n, null); continue }
    if (flag === 0x40) { run(((n & 0x3F) << 8) | data[di++], null); continue }
    if (flag === 0x80) { run(n & 0x3F, palette[data[di++]]); continue }
    const count = ((n & 0x3F) << 8) | data[di++]
    run(count, palette[data[di++]])
  }
  const img = new ImageData(out, w, h)
  const tile = document.createElement('canvas')
  tile.width = w; tile.height = h
  tile.getContext('2d').putImageData(img, 0, 0)
  return tile
}

// SUP 渲染器：把某个时刻的 display set 合成后绘制到覆盖 canvas 的指定 16:9 框内，
// 框外与无字幕像素保持透明。
export class SupRenderer {
  constructor(canvas) {
    this.canvas = canvas
    this.ctx = canvas.getContext('2d')
    this.sets = []
    this.composed = new Map()
    this.active = -2
  }

  load(sets) {
    this.sets = Array.isArray(sets) ? sets : []
    this.composed.clear()
    this.active = -2
    this.clear()
  }

  stop() {
    this.load([])
  }

  clear() {
    this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height)
  }

  composeSet(index) {
    const cached = this.composed.get(index)
    if (cached) return cached
    const set = this.sets[index]
    const comp = document.createElement('canvas')
    comp.width = set.w || 1920
    comp.height = set.h || 1080
    const cctx = comp.getContext('2d')
    for (const c of set.comps) {
      try {
        cctx.drawImage(decodeObject(c.obj, set.palette), c.x, c.y)
      } catch { /* 单个对象损坏不影响其余 */ }
    }
    if (this.composed.size > 40) this.composed.clear()
    this.composed.set(index, comp)
    return comp
  }

  // box：{x, y, w, h}，canvas 像素坐标，锁定 16:9；无 box 或该时刻无字幕则清空
  renderAt(time, box) {
    let index = -1
    for (let i = 0; i < this.sets.length; i++) {
      if (this.sets[i].pts <= time) index = i
      else break
    }
    this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height)
    if (index < 0 || !box || !this.sets[index].comps.length) { this.active = -2; return }
    this.active = index
    this.ctx.drawImage(this.composeSet(index), box.x, box.y, box.w, box.h)
  }
}
