import type { IUploadingUI } from '../utils/dbupload'
import { getGoogleDriveToken } from './client'
import { fetchCancellableProviderUpload, openProviderUploadFile, readProviderUploadSlice, recordProviderUploadProgress } from '../utils/providerUpload'

const RESUMABLE_URL = 'https://www.googleapis.com/upload/drive/v3/files?uploadType=resumable&supportsAllDrives=true'
const UPLOAD_CHUNK_SIZE = 8 * 1024 * 1024

export const buildGoogleDriveUploadContentHeaders = (size: number, start = 0, total = size): Record<string, string> => ({
  'Content-Length': String(size),
  'Content-Range': total > 0 ? `bytes ${start}-${start + size - 1}/${total}` : 'bytes */0'
})

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
        const response = await fetchCancellableProviderUpload(fileui, uploadUrl, { method: 'PUT', headers: buildGoogleDriveUploadContentHeaders(buff.length, offset, total), body: new Uint8Array(buff) })
        const isFinal = offset + buff.length >= total
        if ((!isFinal && response.status !== 308) || (isFinal && !response.ok)) {
          const payload = await response.json().catch(() => ({}))
          return payload?.error?.message || `Google Drive 上传失败 (${response.status})`
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
