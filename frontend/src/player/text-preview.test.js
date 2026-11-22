import { describe, expect, it, vi } from 'vitest'
import { readTextPreview } from './text-preview'

describe('文本预览读取上限', () => {
  it('已知长度超限时不开始读取，直接取消响应', async () => {
    const cancel = vi.fn()
    const body = new ReadableStream({ cancel }, { highWaterMark: 0 })
    const reader = vi.spyOn(body, 'getReader')
    const response = new Response(body, { headers: { 'Content-Length': String(4 * 1024 * 1024 + 1) } })
    await expect(readTextPreview(response)).rejects.toThrow('文本文件超过 4 MB')
    expect(reader).not.toHaveBeenCalled()
    expect(cancel).toHaveBeenCalledOnce()
  })

  it('长度缺失时一旦超限便停止读取，不等待响应结束', async () => {
    const cancel = vi.fn()
    const chunk = new Uint8Array(1024 * 1024)
    const pull = vi.fn(controller => controller.enqueue(chunk))
    const body = new ReadableStream({ pull, cancel }, { highWaterMark: 0 })
    await expect(readTextPreview(new Response(body))).rejects.toThrow('文本文件超过 4 MB')
    expect(pull).toHaveBeenCalledTimes(5)
    expect(cancel).toHaveBeenCalledOnce()
    expect(body.locked).toBe(false)
  })

  it('保留跨分片的多字节字符和刚好达到上限的内容', async () => {
    const bytes = new Uint8Array(4 * 1024 * 1024)
    const suffix = new TextEncoder().encode('末行')
    bytes.set(suffix, bytes.length - suffix.length)
    const body = new ReadableStream({ start(controller) {
      controller.enqueue(bytes.subarray(0, bytes.length - 1))
      controller.enqueue(bytes.subarray(bytes.length - 1))
      controller.close()
    } })
    const result = await readTextPreview(new Response(body))
    expect(result.byteLength).toBe(bytes.length)
    expect(new TextDecoder().decode(result.subarray(-suffix.length))).toBe('末行')
  })
})
