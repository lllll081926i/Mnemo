import type { IUploadingUI } from '../utils/dbupload'
import { getGofileRootId, getGofileUserToken } from './client'

const UPLOAD_URL = 'https://upload.gofile.io/uploadfile'

export default class GofileUploadDisk {
  static async UploadOneFile(fileui: IUploadingUI): Promise<string> {
    if (fileui.encType) return 'GoFile 暂不支持加密上传'
    const token = await getGofileUserToken(fileui.user_id)
    if (!token) return '找不到 GoFile 上传凭证'
    const path = await import('node:path')
    const fs = await import('node:fs') as any
    const localPath = path.join(fileui.localFilePath, fileui.File.partPath)
    const blob = fs.openAsBlob ? await fs.openAsBlob(localPath) : new Blob([await (await import('node:fs/promises')).readFile(localPath)])
    const form = new FormData()
    form.set('folderId', fileui.parent_file_id === 'gofile_root' ? await getGofileRootId(fileui.user_id) : fileui.parent_file_id)
    form.set('modTime', String(Math.floor(Date.now() / 1000)))
    form.set('file', blob, fileui.File.name)
    fileui.Info.uploadState = 'running'
    const response = await fetch(UPLOAD_URL, { method: 'POST', headers: { Authorization: `Bearer ${token.access_token}` }, body: form })
    const payload = await response.json().catch(() => ({}))
    if (!response.ok || payload?.status !== 'ok' || !payload?.data?.id) return payload?.status || `GoFile 上传失败 (${response.status})`
    fileui.File.uploaded_file_id = payload.data.id
    fileui.File.uploaded_is_rapid = false
    const { default: AliUploadDisk } = await import('../aliapi/uploaddisk')
    AliUploadDisk.RecordUploadProgress(fileui.UploadID, fileui.File.size, fileui.File.size)
    return 'success'
  }
}
