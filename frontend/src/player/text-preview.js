const MAX_TEXT_PREVIEW_BYTES = 4 * 1024 * 1024
const limitMessage = '文本文件超过 4 MB，请下载到本地查看'

export async function readTextPreview(response) {
  if (!response.ok) throw new Error(`HTTP ${response.status} 加载失败`)
  const body = response.body
  if (Number(response.headers.get('Content-Length')) > MAX_TEXT_PREVIEW_BYTES) {
    void body?.cancel().catch(() => {})
    throw new Error(limitMessage)
  }
  if (!body) return new Uint8Array()
  const reader = body.getReader()
  const chunks = []
  let byteLength = 0
  try {
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      byteLength += value.byteLength
      if (byteLength > MAX_TEXT_PREVIEW_BYTES) {
        void reader.cancel().catch(() => {})
        throw new Error(limitMessage)
      }
      chunks.push(value)
    }
  } finally {
    reader.releaseLock()
  }
  if (chunks.length === 1) return chunks[0]
  const buffer = new Uint8Array(byteLength)
  let offset = 0
  for (const chunk of chunks) {
    buffer.set(chunk, offset)
    offset += chunk.byteLength
  }
  return buffer
}
