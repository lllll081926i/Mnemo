import { fetchCancellableProviderUpload } from '../utils/providerUpload'

const GRAPH_API_HOST = 'https://graph.microsoft.com/v1.0'
// Align with Graph simple-upload guidance and rclone's cautious single-part policy for business drives.
const SMALL_UPLOAD_LIMIT = 4 * 1024 * 1024
// Must be a multiple of 320 KiB per Microsoft Graph upload session rules.
const SESSION_CHUNK_SIZE = 10 * 1024 * 1024

const getOneDriveToken = async (user_id: string) => {
  const { default: UserDAL } = await import('../user/userdal')
  let token = UserDAL.GetUserToken(user_id)
  if (!token?.access_token) {
    const dbToken = await UserDAL.GetUserTokenFromDB(user_id)
    if (dbToken) token = dbToken
  }
  return token
}

const encodePathSegment = (value: string) => encodeURIComponent(value).replace(/%2F/g, '/')

export const buildOneDriveSmallUploadPath = (parentId: string, fileName: string): string => {
  const name = encodePathSegment(fileName)
  if (!parentId || parentId === 'onedrive_root') return `/me/drive/root:/${name}:/content`
  return `/me/drive/items/${encodeURIComponent(parentId)}:/${name}:/content`
}

export const buildOneDriveUploadSessionPath = (parentId: string, fileName: string): string => {
  const name = encodePathSegment(fileName)
  if (!parentId || parentId === 'onedrive_root') return `/me/drive/root:/${name}:/createUploadSession`
  return `/me/drive/items/${encodeURIComponent(parentId)}:/${name}:/createUploadSession`
}

export const toOneDriveConflictBehavior = (checkNameMode: string): 'fail' | 'replace' | 'rename' => {
  if (checkNameMode === 'auto_rename') return 'rename'
  if (checkNameMode === 'overwrite' || checkNameMode === 'ignore') return 'replace'
  return 'fail'
}

export const buildOneDriveUploadSessionBody = (checkNameMode: string) => ({
  item: {
    '@microsoft.graph.conflictBehavior': toOneDriveConflictBehavior(checkNameMode)
  }
})

export const apiOneDriveUploadBuffer = async (
  user_id: string,
  parentId: string,
  fileName: string,
  buff: Buffer
): Promise<{ file_id: string; error: string }> => {
  const token = await getOneDriveToken(user_id)
  if (!token?.access_token) return { file_id: '', error: '未登录 OneDrive' }
  if (buff.length > SMALL_UPLOAD_LIMIT) return { file_id: '', error: 'OneDrive 简单上传超过 4MB 限制，请使用会话分片上传' }
  const resp = await fetch(`${GRAPH_API_HOST}${buildOneDriveSmallUploadPath(parentId, fileName)}`, {
    method: 'PUT',
    headers: {
      Authorization: `Bearer ${token.access_token}`,
      'Content-Type': 'application/octet-stream'
    },
    body: new Uint8Array(buff)
  })
  const data = await resp.json().catch(() => undefined)
  if (!resp.ok || data?.error) return { file_id: '', error: data?.error?.message || '创建 OneDrive 文件失败' }
  return { file_id: data?.id || '', error: '' }
}

const readSlice = async (fileHandle: import('fs/promises').FileHandle, start: number, size: number): Promise<Buffer> => {
  const buff = Buffer.alloc(size)
  const read = await fileHandle.read(buff, 0, size, start)
  return buff.subarray(0, read.bytesRead)
}

const recordUploadProgress = async (uploadId: number, delta: number, pos: number) => {
  const { recordUploadProgress: record } = await import('../utils/uploadProgress')
  record(uploadId, delta, pos)
}

const graphPost = async (accessToken: string, path: string, body: any, fileui: import('../utils/dbupload').IUploadingUI): Promise<{ data?: any; error: string }> => {
  const resp = await fetchCancellableProviderUpload(fileui, `${GRAPH_API_HOST}${path}`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${accessToken}`,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(body)
  })
  const data = await resp.json().catch(() => undefined)
  if (!resp.ok || data?.error) return { error: data?.error?.message || '创建 OneDrive 上传会话失败' }
  return { data, error: '' }
}

const uploadChunk = async (uploadUrl: string, buff: Buffer, start: number, total: number, fileui: import('../utils/dbupload').IUploadingUI): Promise<{ data?: any; error: string }> => {
  const end = start + buff.length - 1
  const resp = await fetchCancellableProviderUpload(fileui, uploadUrl, {
    method: 'PUT',
    headers: {
      'Content-Length': String(buff.length),
      'Content-Range': `bytes ${start}-${end}/${total}`
    },
    body: new Uint8Array(buff)
  })
  const data = await resp.json().catch(() => undefined)
  if (!resp.ok || data?.error) return { error: data?.error?.message || `上传 OneDrive 分片失败 ${resp.status}` }
  return { data, error: '' }
}

// 分片上传失败时重试（网络抖动 / 服务端 5xx 等瞬时错误）；暂停或取消时立即退出，不做无谓重试
const uploadChunkWithRetry = async (uploadUrl: string, buff: Buffer, start: number, total: number, fileui: import('../utils/dbupload').IUploadingUI, maxRetries = 3): Promise<{ data?: any; error: string }> => {
  let last: { data?: any; error: string } = { error: '上传 OneDrive 分片失败' }
  for (let attempt = 1; attempt <= maxRetries; attempt++) {
    if (!fileui.IsRunning) return { error: '已暂停' }
    try {
      const result = await uploadChunk(uploadUrl, buff, start, total, fileui)
      if (!result.error) return result
      last = result
    } catch (error: any) {
      if (error?.name === 'AbortError' || !fileui.IsRunning) throw error
      last = { error: error?.message || '上传 OneDrive 分片失败' }
    }
    if (attempt < maxRetries) await new Promise((resolve) => setTimeout(resolve, 1000 * attempt))
  }
  return last
}

const uploadSmallFile = async (accessToken: string, fileHandle: import('fs/promises').FileHandle, fileui: import('../utils/dbupload').IUploadingUI): Promise<string> => {
  const buff = await readSlice(fileHandle, 0, fileui.File.size)
  const resp = await fetchCancellableProviderUpload(fileui, `${GRAPH_API_HOST}${buildOneDriveSmallUploadPath(fileui.parent_file_id || 'onedrive_root', fileui.File.name)}`, {
    method: 'PUT',
    headers: {
      Authorization: `Bearer ${accessToken}`,
      'Content-Type': 'application/octet-stream'
    },
    body: new Uint8Array(buff)
  })
  const data = await resp.json().catch(() => undefined)
  if (!resp.ok || data?.error) return data?.error?.message || '上传 OneDrive 文件失败'
  fileui.File.uploaded_file_id = data?.id || ''
  fileui.File.uploaded_is_rapid = false
  await recordUploadProgress(fileui.UploadID, buff.length, buff.length)
  return 'success'
}

const uploadSessionFile = async (accessToken: string, fileHandle: import('fs/promises').FileHandle, fileui: import('../utils/dbupload').IUploadingUI): Promise<string> => {
  const created = await graphPost(
    accessToken,
    buildOneDriveUploadSessionPath(fileui.parent_file_id || 'onedrive_root', fileui.File.name),
    buildOneDriveUploadSessionBody(fileui.check_name_mode),
    fileui
  )
  const uploadUrl = created.data?.uploadUrl || ''
  if (created.error || !uploadUrl) return created.error || '创建 OneDrive 上传会话失败'

  const total = fileui.File.size
  let offset = 0
  while (offset < total) {
    if (!fileui.IsRunning) return '已暂停'
    const size = Math.min(SESSION_CHUNK_SIZE, total - offset)
    let buff: Buffer | null = await readSlice(fileHandle, offset, size)
    const uploaded = await uploadChunkWithRetry(uploadUrl, buff, offset, total, fileui)
    const chunkLength = buff.length
    buff = null
    if (uploaded.error) return uploaded.error
    offset += chunkLength
    await recordUploadProgress(fileui.UploadID, chunkLength, offset)
    if (uploaded.data?.id) {
      fileui.File.uploaded_file_id = uploaded.data.id
      fileui.File.uploaded_is_rapid = false
    }
  }
  return 'success'
}

export default class OneDriveUploadDisk {
  static async UploadOneFile(fileui: import('../utils/dbupload').IUploadingUI): Promise<string> {
    const { default: UserDAL } = await import('../user/userdal')
    const token = await UserDAL.GetUserTokenFromDB(fileui.user_id)
    if (!token?.access_token) return '找不到上传token，请重试'
    if (fileui.encType) return 'OneDrive 暂不支持加密上传'

    const path = await import('path')
    const filePath = path.join(fileui.localFilePath, fileui.File.partPath)
    const { OpenFileHandle } = await import('../utils/filehelper')
    const opened = await OpenFileHandle(filePath)
    if (opened.error || !opened.handle) return opened.error || '打开文件失败'
    fileui.Info.uploadState = 'running'
    try {
      if (fileui.File.size <= SMALL_UPLOAD_LIMIT) {
        return await uploadSmallFile(token.access_token, opened.handle, fileui)
      }
      return await uploadSessionFile(token.access_token, opened.handle, fileui)
    } finally {
      await opened.handle.close().catch(() => {})
    }
  }
}
