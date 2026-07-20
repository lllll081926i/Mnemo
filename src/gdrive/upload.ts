import type { IUploadingUI } from '../utils/dbupload'
import { getGoogleDriveToken } from './client'

const RESUMABLE_URL = 'https://www.googleapis.com/upload/drive/v3/files?uploadType=resumable&supportsAllDrives=true'

export default class GoogleDriveUploadDisk {
  static async UploadOneFile(fileui: IUploadingUI): Promise<string> {
    if (fileui.encType) return 'Google Drive 暂不支持加密上传'
    const token = await getGoogleDriveToken(fileui.user_id)
    if (!token) return '找不到 Google Drive 上传凭证'
    const path = await import('node:path')
    const fs = await import('node:fs') as any
    const localPath = path.join(fileui.localFilePath, fileui.File.partPath)
    const parentId = fileui.parent_file_id === 'gdrive_root' ? 'root' : fileui.parent_file_id
    const sessionResponse = await fetch(RESUMABLE_URL, {
      method: 'POST',
      headers: { Authorization: `Bearer ${token.access_token}`, 'Content-Type': 'application/json; charset=UTF-8', 'X-Upload-Content-Length': String(fileui.File.size) },
      body: JSON.stringify({ name: fileui.File.name, parents: [parentId] })
    })
    const uploadUrl = sessionResponse.headers.get('Location') || ''
    if (!sessionResponse.ok || !uploadUrl) return `Google Drive 上传会话创建失败 (${sessionResponse.status})`
    const blob = fs.openAsBlob ? await fs.openAsBlob(localPath) : new Blob([await (await import('node:fs/promises')).readFile(localPath)])
    fileui.Info.uploadState = 'running'
    const uploadResponse = await fetch(uploadUrl, { method: 'PUT', headers: { 'Content-Length': String(fileui.File.size) }, body: blob })
    const payload = await uploadResponse.json().catch(() => ({}))
    if (!uploadResponse.ok || !payload?.id) return payload?.error?.message || `Google Drive 上传失败 (${uploadResponse.status})`
    fileui.File.uploaded_file_id = payload.id
    fileui.File.uploaded_is_rapid = false
    const { default: AliUploadDisk } = await import('../aliapi/uploaddisk')
    AliUploadDisk.RecordUploadProgress(fileui.UploadID, fileui.File.size, fileui.File.size)
    return 'success'
  }
}
