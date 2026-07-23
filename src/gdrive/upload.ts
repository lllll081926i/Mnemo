import type { IUploadingUI } from '../utils/dbupload'
import { getGoogleDriveToken } from './client'
import { fetchCancellableProviderUpload, openProviderUploadFile, readProviderUploadSlice, recordProviderUploadProgress } from '../utils/providerUpload'

const RESUMABLE_URL = 'https://www.googleapis.com/upload/drive/v3/files?uploadType=resumable&supportsAllDrives=true'
const UPLOAD_CHUNK_SIZE = 8 * 1024 * 1024

export const buildGoogleDriveUploadContentHeaders = (size: number, start = 0, total = size): Record<string, string> => ({
  'Content-Length': String(size),
  'Content-Range': total > 0 ? `bytes ${start}-${start + size - 1}/${total}` : 'bytes */0'
})

// 分片上传失败时重试（网络抖动 / 服务端 5xx 等瞬时错误）；暂停或取消时立即退出，不做无谓重试
const uploadChunkWithRetry = async (fileui: IUploadingUI, uploadUrl: string, chunk: Buffer, start: number, total: number, isFinal: boolean, maxRetries = 3): Promise<Response> => {
  let lastError = 'Google Drive 上传分片失败'
  for (let attempt = 1; attempt <= maxRetries; attempt++) {
    if (!fileui.IsRunning) throw new DOMException('已暂停', 'AbortError')
    try {
      const response = await fetchCancellableProviderUpload(fileui, uploadUrl, { method: 'PUT', headers: buildGoogleDriveUploadContentHeaders(chunk.length, start, total), body: new Uint8Array(chunk) })
      // 中间分片成功返回 308 Resume Incomplete，最后一片返回 2xx
      if (isFinal ? response.ok : response.status === 308) return response
      const payload = await response.json().catch(() => ({}))
      lastError = payload?.error?.message || `Google Drive 上传失败 (${response.status})`
      // 4xx（429 除外）是请求本身的问题，重试也不会成功
      if (response.status !== 429 && response.status >= 400 && response.status < 500) break
    } catch (error: any) {
      if (error?.name === 'AbortError' || !fileui.IsRunning) throw error
      lastError = error?.message || lastError
    }
    if (attempt < maxRetries) await new Promise((resolve) => setTimeout(resolve, 1000 * attempt))
  }
  throw new Error(lastError)
}

export default class GoogleDriveUploadDisk {
  static async UploadOneFile(fileui: IUploadingUI): Promise<string> {
    if (fileui.encType) return 'Google Drive 暂不支持加密上传'
    const token = await getGoogleDriveToken(fileui.user_id)
    if (!token) return '找不到 Google Drive 上传凭证'
    const parentId = fileui.parent_file_id === 'gdrive_root' ? 'root' : fileui.parent_file_id
    const sessionResponse = await fetchCancellableProviderUpload(fileui, RESUMABLE_URL, {
      method: 'POST',
      headers: { Authorization: `Bearer ${token.access_token}`, 'Content-Type': 'application/json; charset=UTF-8', 'X-Upload-Content-Length': String(fileui.File.size) },
      body: JSON.stringify({ name: fileui.File.name, parents: [parentId] })
    })
    const uploadUrl = sessionResponse.headers.get('Location') || ''
    if (!sessionResponse.ok || !uploadUrl) return `Google Drive 上传会话创建失败 (${sessionResponse.status})`
    fileui.Info.uploadState = 'running'
    const opened = await openProviderUploadFile(fileui)
    if (!opened.handle) return opened.error
    try {
      const total = fileui.File.size
      if (total === 0) {
        const response = await fetchCancellableProviderUpload(fileui, uploadUrl, { method: 'PUT', headers: buildGoogleDriveUploadContentHeaders(0), body: new Uint8Array() })
        const payload = await response.json().catch(() => ({}))
        if (!response.ok || !payload?.id) return payload?.error?.message || `Google Drive 上传失败 (${response.status})`
        fileui.File.uploaded_file_id = payload.id
        fileui.File.uploaded_is_rapid = false
        return 'success'
      }
      let offset = 0
      while (offset < total) {
        if (!fileui.IsRunning) return '已暂停'
        const buff = await readProviderUploadSlice(opened.handle, offset, Math.min(UPLOAD_CHUNK_SIZE, total - offset))
        if (!buff.length) return '读取 Google Drive 上传文件失败'
        const isFinal = offset + buff.length >= total
        let response: Response
        try {
          response = await uploadChunkWithRetry(fileui, uploadUrl, buff, offset, total, isFinal)
        } catch (error: any) {
          return error?.message || 'Google Drive 上传分片失败'
        }
        offset += buff.length
        await recordProviderUploadProgress(fileui, buff.length, offset)
        if (isFinal) {
          const payload = await response.json().catch(() => ({}))
          if (!payload?.id) return 'Google Drive 上传响应缺少文件 ID'
          fileui.File.uploaded_file_id = payload.id
          fileui.File.uploaded_is_rapid = false
        }
      }
      return 'success'
    } finally {
      await opened.handle.close().catch(() => {})
    }
  }
}
