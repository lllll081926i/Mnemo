import nodehttps from 'https'
import type { ClientRequest } from 'http'
import path from 'path'
import { FileHandle } from 'fs/promises'
import type { IUploadingUI } from '../utils/dbupload'
import { Sleep } from '../utils/format'
import { resolveDropboxCommandPath } from './filecmd'

const DROPBOX_CONTENT_HOST = 'content.dropboxapi.com'
const SMALL_UPLOAD_LIMIT = 150 * 1024 * 1024
const SESSION_CHUNK_SIZE = 8 * 1024 * 1024

type DropboxUploadResponse = {
  id?: string
  path_display?: string
  name?: string
  error_summary?: string
}

type DropboxSessionStartResponse = {
  session_id?: string
  error_summary?: string
}

export const buildDropboxUploadPath = (parentId: string, fileName: string, parentDescription = '', parentPath = ''): string => {
  const parent = resolveDropboxCommandPath(parentId, parentDescription, parentPath)
  const cleanName = fileName.split(path.sep).join('/')
  if (!parent) return `/${cleanName}`
  return `${parent.replace(/\/+$/g, '')}/${cleanName}`
}

export const toDropboxWriteMode = (checkNameMode: string): { mode: 'add' | 'overwrite'; autorename: boolean } => {
  if (checkNameMode === 'auto_rename') return { mode: 'add', autorename: true }
  if (checkNameMode === 'refuse') return { mode: 'add', autorename: false }
  return { mode: 'overwrite', autorename: false }
}

export const buildDropboxCommitInfo = (uploadPath: string, checkNameMode: string) => {
  const writeMode = toDropboxWriteMode(checkNameMode)
  return {
    path: uploadPath,
    mode: writeMode.mode,
    autorename: writeMode.autorename,
    mute: false,
    strict_conflict: false
  }
}

export const buildDropboxUploadSessionCursor = (sessionId: string, offset: number) => ({
  session_id: sessionId,
  offset
})

const parseDropboxContentError = (data: string, fallback: string): string => {
  if (!data) return fallback
  try {
    const parsed = JSON.parse(data)
    return parsed?.error_summary || parsed?.error_description || parsed?.message || fallback
  } catch {
    return data || fallback
  }
}

const dropboxContentRequest = <T>(accessToken: string, endpoint: string, apiArg: any, body: Buffer, fallback: string, fileui?: IUploadingUI): Promise<{ data?: T; error: string }> => {
  return new Promise((resolve) => {
    if (fileui && !fileui.IsRunning) {
      resolve({ error: '已暂停' })
      return
    }
    let settled = false
    let monitor: ReturnType<typeof setInterval> | undefined
    const finish = (value: { data?: T; error: string }) => {
      if (settled) return
      settled = true
      if (monitor) clearInterval(monitor)
      resolve(value)
    }
    const req: ClientRequest = nodehttps.request({
      method: 'POST',
      hostname: DROPBOX_CONTENT_HOST,
      path: `/2${endpoint}`,
      headers: {
        Authorization: `Bearer ${accessToken}`,
        'Dropbox-API-Arg': JSON.stringify(apiArg),
        'Content-Type': 'application/octet-stream',
        'Content-Length': body.length
      }
    }, (res) => {
      let raw = ''
      res.on('data', (chunk: Buffer | string) => {
        raw += chunk.toString()
      })
      res.on('end', () => {
        if (res.statusCode && res.statusCode >= 200 && res.statusCode < 300) {
          try {
            const data = raw ? JSON.parse(raw) : {}
            finish({ data: data as T, error: '' })
          } catch {
            finish({ error: fallback })
          }
        } else {
          finish({ error: parseDropboxContentError(raw, fallback) })
        }
      })
    })
    req.on('error', (err: any) => finish({ error: fileui && !fileui.IsRunning ? '已暂停' : err?.message || fallback }))
    if (fileui) {
      monitor = setInterval(() => {
        if (!fileui.IsRunning) req.destroy(new Error('已暂停'))
      }, 100)
    }
    req.end(body)
  })
}

const readSlice = async (fileHandle: FileHandle, start: number, size: number): Promise<Buffer> => {
  const buff = Buffer.alloc(size)
  const read = await fileHandle.read(buff, 0, size, start)
  return buff.subarray(0, read.bytesRead)
}

const recordUploadProgress = async (uploadId: number, delta: number, pos: number) => {
  const { recordUploadProgress: record } = await import('../utils/uploadProgress')
  record(uploadId, delta, pos)
}

const uploadBufferWithRetry = async <T>(accessToken: string, endpoint: string, arg: any, body: Buffer, fallback: string, fileui?: IUploadingUI): Promise<{ data?: T; error: string }> => {
  let last = ''
  for (let i = 0; i < 3; i++) {
    if (fileui && !fileui.IsRunning) return { error: '已暂停' }
    const resp = await dropboxContentRequest<T>(accessToken, endpoint, arg, body, fallback, fileui)
    if (!resp.error) return resp
    last = resp.error
    if (fileui && !fileui.IsRunning) return { error: '已暂停' }
    await Sleep(800 * (i + 1))
  }
  return { error: last || fallback }
}

export const apiDropboxUploadBuffer = async (
  accessToken: string,
  parentId: string,
  fileName: string,
  buff: Buffer,
  checkNameMode = 'refuse'
): Promise<{ file_id: string; error: string }> => {
  if (!accessToken) return { file_id: '', error: '找不到上传token，请重试' }
  const uploadPath = buildDropboxUploadPath(parentId || 'dropbox_root', fileName)
  const resp = await uploadBufferWithRetry<DropboxUploadResponse>(
    accessToken,
    '/files/upload',
    buildDropboxCommitInfo(uploadPath, checkNameMode),
    buff,
    '创建 Dropbox 文件失败'
  )
  if (resp.error) return { file_id: '', error: resp.error }
  return { file_id: resp.data?.id || resp.data?.path_display || uploadPath, error: '' }
}

const uploadSmallFile = async (accessToken: string, fileHandle: FileHandle, fileui: IUploadingUI, uploadPath: string): Promise<string> => {
  const buff = await readSlice(fileHandle, 0, fileui.File.size)
  const resp = await uploadBufferWithRetry<DropboxUploadResponse>(
    accessToken,
    '/files/upload',
    buildDropboxCommitInfo(uploadPath, fileui.check_name_mode),
    buff,
    '上传 Dropbox 文件失败',
    fileui
  )
  if (resp.error) return resp.error
  fileui.File.uploaded_file_id = resp.data?.id || resp.data?.path_display || uploadPath
  fileui.File.uploaded_is_rapid = false
  await recordUploadProgress(fileui.UploadID, buff.length, buff.length)
  return 'success'
}

const uploadSessionFile = async (accessToken: string, fileHandle: FileHandle, fileui: IUploadingUI, uploadPath: string): Promise<string> => {
  const total = fileui.File.size
  let offset = 0
  const firstSize = Math.min(SESSION_CHUNK_SIZE, total)
  const first = await readSlice(fileHandle, offset, firstSize)
  const started = await uploadBufferWithRetry<DropboxSessionStartResponse>(
    accessToken,
    '/files/upload_session/start',
    { close: false },
    first,
    '创建 Dropbox 上传会话失败',
    fileui
  )
  if (started.error || !started.data?.session_id) return started.error || '创建 Dropbox 上传会话失败'
  offset += first.length
  await recordUploadProgress(fileui.UploadID, first.length, offset)

  while (offset < total) {
    if (!fileui.IsRunning) return '已暂停'
    const size = Math.min(SESSION_CHUNK_SIZE, total - offset)
    const buff = await readSlice(fileHandle, offset, size)
    const isLast = offset + buff.length >= total
    const cursor = buildDropboxUploadSessionCursor(started.data.session_id, offset)
    const endpoint = isLast ? '/files/upload_session/finish' : '/files/upload_session/append_v2'
    const arg = isLast ? { cursor, commit: buildDropboxCommitInfo(uploadPath, fileui.check_name_mode) } : { cursor, close: false }
    const resp = await uploadBufferWithRetry<DropboxUploadResponse>(accessToken, endpoint, arg, buff, '上传 Dropbox 分片失败', fileui)
    if (resp.error) return resp.error
    offset += buff.length
    await recordUploadProgress(fileui.UploadID, buff.length, offset)
    if (isLast) {
      fileui.File.uploaded_file_id = resp.data?.id || resp.data?.path_display || uploadPath
      fileui.File.uploaded_is_rapid = false
    }
  }
  return 'success'
}

export default class DropboxUploadDisk {
  static async UploadOneFile(fileui: IUploadingUI): Promise<string> {
    const { default: UserDAL } = await import('../user/userdal')
    const token = await UserDAL.GetUserTokenFromDB(fileui.user_id)
    if (!token?.access_token) return '找不到上传token，请重试'
    if (fileui.encType) return 'Dropbox 暂不支持加密上传'

    const filePath = path.join(fileui.localFilePath, fileui.File.partPath)
    const uploadPath = buildDropboxUploadPath(fileui.parent_file_id || 'dropbox_root', fileui.File.name)
    fileui.Info.uploadState = 'running'
    const { OpenFileHandle } = await import('../utils/filehelper')
    const opened = await OpenFileHandle(filePath)
    if (opened.error || !opened.handle) return opened.error || '打开文件失败'
    try {
      if (fileui.File.size <= SMALL_UPLOAD_LIMIT) {
        return await uploadSmallFile(token.access_token, opened.handle, fileui, uploadPath)
      }
      return await uploadSessionFile(token.access_token, opened.handle, fileui, uploadPath)
    } finally {
      await opened.handle.close().catch(() => {})
    }
  }
}
