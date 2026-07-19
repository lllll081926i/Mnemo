import { createHash, type Hash } from 'crypto'
import path from 'path'
import type { FileHandle } from 'fs/promises'
import type { IUploadingUI } from './dbupload'
import { OpenFileHandle } from './filehelper'
import { Sleep } from './format'

export type ProviderHashAlgorithm = 'md5' | 'sha1' | 'sha256'

const shouldRetryFileOpen = (error: string) => error.includes('同时打开文件过多') || error.includes('文件被其他程序占用') || error.includes('操作超时') || error.includes('IO错误')

export const getProviderUploadFilePath = (fileui: IUploadingUI) => path.join(fileui.localFilePath, fileui.File.partPath)

export const openProviderUploadFile = async (fileui: IUploadingUI): Promise<{ handle?: FileHandle; error: string }> => {
  const filePath = getProviderUploadFilePath(fileui)
  let lastError = ''
  for (let attempt = 0; attempt < 5; attempt++) {
    const opened = await OpenFileHandle(filePath)
    if (!opened.error && opened.handle) return { handle: opened.handle, error: '' }
    lastError = opened.error || '打开文件失败'
    if (!shouldRetryFileOpen(lastError) || attempt === 4) break
    await Sleep(400 * (attempt + 1))
  }
  return { error: lastError || '打开文件失败' }
}

export const readProviderUploadSlice = async (handle: FileHandle, start: number, size: number): Promise<Buffer> => {
  if (size <= 0) return Buffer.alloc(0)
  const buff = Buffer.alloc(size)
  let totalRead = 0
  while (totalRead < size) {
    const read = await handle.read(buff, totalRead, size - totalRead, start + totalRead)
    if (!read.bytesRead) break
    totalRead += read.bytesRead
  }
  return buff.subarray(0, totalRead)
}

export const recordProviderUploadProgress = async (fileui: IUploadingUI, delta: number, position: number) => {
  const { default: AliUploadDisk } = await import('../aliapi/uploaddisk')
  AliUploadDisk.RecordUploadProgress(fileui.UploadID, delta, position)
}

export const computeProviderFileHashes = async (fileui: IUploadingUI, algorithms: ProviderHashAlgorithm[]): Promise<{ hashes: Partial<Record<ProviderHashAlgorithm, string>>; error: string }> => {
  const opened = await openProviderUploadFile(fileui)
  if (!opened.handle) return { hashes: {}, error: opened.error }
  const hashers = new Map<ProviderHashAlgorithm, Hash>(algorithms.map((algorithm) => [algorithm, createHash(algorithm)]))
  try {
    let offset = 0
    const chunkSize = 4 * 1024 * 1024
    while (offset < fileui.File.size) {
      if (!fileui.IsRunning) return { hashes: {}, error: '已暂停' }
      const buff = await readProviderUploadSlice(opened.handle, offset, Math.min(chunkSize, fileui.File.size - offset))
      if (!buff.length) return { hashes: {}, error: '读取上传文件失败' }
      for (const hasher of hashers.values()) hasher.update(buff)
      offset += buff.length
    }
    const hashes: Partial<Record<ProviderHashAlgorithm, string>> = {}
    for (const [algorithm, hasher] of hashers) hashes[algorithm] = hasher.digest('hex')
    return { hashes, error: '' }
  } catch (error: any) {
    return { hashes: {}, error: error?.message || '计算文件哈希失败' }
  } finally {
    await opened.handle.close().catch(() => {})
  }
}

const getGcidChunkSize = (fileSize: number) => {
  if (fileSize <= 0x8000000) return 262144
  if (fileSize <= 0x10000000) return 524288
  if (fileSize <= 0x20000000) return 1048576
  return 2097152
}

export const computeProviderGcid = async (fileui: IUploadingUI): Promise<{ gcid: string; error: string }> => {
  const opened = await openProviderUploadFile(fileui)
  if (!opened.handle) return { gcid: '', error: opened.error }
  try {
    const chunkSize = getGcidChunkSize(fileui.File.size)
    const chunkHashes: Buffer[] = []
    let offset = 0
    while (offset < fileui.File.size) {
      if (!fileui.IsRunning) return { gcid: '', error: '已暂停' }
      const buff = await readProviderUploadSlice(opened.handle, offset, Math.min(chunkSize, fileui.File.size - offset))
      if (!buff.length) return { gcid: '', error: '读取上传文件失败' }
      chunkHashes.push(createHash('sha1').update(buff).digest())
      offset += buff.length
    }
    const gcid = createHash('sha1').update(Buffer.concat(chunkHashes)).digest('hex').toUpperCase()
    return { gcid, error: '' }
  } catch (error: any) {
    return { gcid: '', error: error?.message || '计算 GCID 失败' }
  } finally {
    await opened.handle.close().catch(() => {})
  }
}

export const fetchProviderUploadWithRetry = async (request: () => Promise<Response>, attempts = 3): Promise<Response> => {
  let lastError: any
  let lastResponse: Response | undefined
  for (let attempt = 0; attempt < attempts; attempt++) {
    try {
      const response = await request()
      lastResponse = response
      if (response.ok || (response.status !== 429 && response.status < 500)) return response
      if (attempt < attempts - 1) await response.arrayBuffer().catch(() => undefined)
    } catch (error) {
      lastError = error
    }
    if (attempt < attempts - 1) await Sleep(700 * (attempt + 1))
  }
  if (lastResponse) return lastResponse
  throw lastError || new Error('上传请求失败')
}

export const parseProviderUploadResponse = async (response: Response): Promise<{ data: any; text: string }> => {
  const text = await response.text().catch(() => '')
  if (!text) return { data: undefined, text: '' }
  try {
    return { data: JSON.parse(text), text }
  } catch {
    return { data: undefined, text }
  }
}
