import { createHash } from 'crypto'
import path from 'path'
import type { FileHandle } from 'fs/promises'
import type { IUploadingUI } from '../utils/dbupload'
import { Sleep } from '../utils/format'
import { boxApiRequest, getBoxToken, toBoxId } from './dirfilelist'

const BOX_UPLOAD_HOST = 'https://upload.box.com/api/2.0'
const BOX_CHUNK_UPLOAD_THRESHOLD = 20 * 1024 * 1024
const BOX_FALLBACK_PART_SIZE = 8 * 1024 * 1024

interface BoxUploadPart {
  part_id: string
  offset: number
  size: number
  sha1: string
}

interface BoxUploadSession {
  id?: string
  part_size?: number
  session_endpoints?: {
    upload_part?: string
    commit?: string
    abort?: string
  }
}

export const toBoxConflictBehavior = (mode: string) => {
  if (mode === 'overwrite') return 'overwrite'
  if (mode === 'refuse') return 'refuse'
  return 'rename'
}

export const buildBoxSmallUploadAttributes = (parentId: string, name: string) => ({
  name,
  parent: { id: toBoxId(parentId) }
})

export const buildBoxUploadSessionPath = () => '/files/upload_sessions'

export const buildBoxUploadSessionBody = (parentId: string, name: string, size: number) => ({
  folder_id: toBoxId(parentId),
  file_name: name,
  file_size: size
})

export const buildBoxPartHeaders = (offset: number, size: number, total: number, digest: string) => ({
  Digest: `sha=${digest}`,
  'Content-Range': `bytes ${offset}-${offset + size - 1}/${total}`,
  'Content-Type': 'application/octet-stream'
})

export const buildBoxCommitBody = (parts: BoxUploadPart[]) => ({ parts })

export const apiBoxUploadBuffer = async (user_id: string, parentId: string, name: string, buff: Buffer, mode: string): Promise<{ file_id: string; error: string }> => {
  if (toBoxConflictBehavior(mode) === 'overwrite') return { file_id: '', error: 'Box 新建文件暂不支持覆盖同名文件' }
  const form = new FormData()
  form.set('attributes', JSON.stringify(buildBoxSmallUploadAttributes(parentId, name)))
  form.set('file', new Blob([new Uint8Array(buff)]), name)
  const data = await boxApiRequest<any>(
    user_id,
    `${BOX_UPLOAD_HOST}/files/content`,
    {
      method: 'POST',
      body: form
    },
    '上传 Box 文件失败'
  )
  const file = data?.entries?.[0]
  return { file_id: file?.id || '', error: file?.id ? '' : '上传 Box 文件失败' }
}

const readSlice = async (fileHandle: FileHandle, start: number, size: number): Promise<Buffer> => {
  const buff = Buffer.alloc(size)
  const read = await fileHandle.read(buff, 0, size, start)
  return buff.subarray(0, read.bytesRead)
}

const recordUploadProgress = async (uploadId: number, delta: number, pos: number) => {
  const { default: AliUploadDisk } = await import('../aliapi/uploaddisk')
  AliUploadDisk.RecordUploadProgress(uploadId, delta, pos)
}

const parseBoxUploadError = (data: any, fallback: string) => data?.context_info?.errors?.[0]?.message || data?.message || fallback

const boxUploadRequest = async <T>(accessToken: string, url: string, init: RequestInit, fallback: string): Promise<{ data?: T; status: number; retryAfter: number; error: string }> => {
  try {
    const resp = await fetch(url, {
      ...init,
      headers: {
        Authorization: `Bearer ${accessToken}`,
        ...((init.headers as Record<string, string>) || {})
      }
    })
    const text = await resp.text().catch(() => '')
    let data: any = undefined
    try {
      data = text ? JSON.parse(text) : undefined
    } catch {
      data = undefined
    }
    if (!resp.ok) return { status: resp.status, retryAfter: Number(resp.headers.get('retry-after') || 0), error: parseBoxUploadError(data, fallback) }
    return { data: data as T, status: resp.status, retryAfter: Number(resp.headers.get('retry-after') || 0), error: '' }
  } catch (error: any) {
    return { status: 0, retryAfter: 0, error: error?.message || fallback }
  }
}

const boxUploadRequestWithRetry = async <T>(accessToken: string, url: string, init: RequestInit, fallback: string) => {
  let last = { status: 0, retryAfter: 0, error: fallback } as { data?: T; status: number; retryAfter: number; error: string }
  for (let attempt = 0; attempt < 3; attempt++) {
    last = await boxUploadRequest<T>(accessToken, url, init, fallback)
    if (!last.error) return last
    await Sleep(800 * (attempt + 1))
  }
  return last
}

const uploadBoxSmallFile = async (fileHandle: FileHandle, fileui: IUploadingUI, fileName: string): Promise<string> => {
  const buff = await readSlice(fileHandle, 0, fileui.File.size)
  const result = await apiBoxUploadBuffer(fileui.user_id, fileui.parent_file_id, fileName, buff, fileui.check_name_mode)
  if (result.error) return result.error
  fileui.File.uploaded_file_id = result.file_id
  fileui.File.uploaded_is_rapid = false
  await recordUploadProgress(fileui.UploadID, buff.length, buff.length)
  return 'success'
}

const uploadBoxSessionFile = async (accessToken: string, fileHandle: FileHandle, fileui: IUploadingUI, fileName: string): Promise<string> => {
  const created = await boxUploadRequestWithRetry<BoxUploadSession>(
    accessToken,
    `${BOX_UPLOAD_HOST}${buildBoxUploadSessionPath()}`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(buildBoxUploadSessionBody(fileui.parent_file_id, fileName, fileui.File.size))
    },
    '创建 Box 上传会话失败'
  )
  const uploadPartUrl = created.data?.session_endpoints?.upload_part || ''
  const commitUrl = created.data?.session_endpoints?.commit || ''
  if (created.error || !uploadPartUrl || !commitUrl) return created.error || 'Box 上传会话缺少分片地址'

  const total = fileui.File.size
  const partSize = Number(created.data?.part_size || BOX_FALLBACK_PART_SIZE)
  const fileDigest = createHash('sha1')
  const parts: BoxUploadPart[] = []
  let offset = 0
  while (offset < total) {
    if (!fileui.IsRunning) return '已暂停'
    const buff = await readSlice(fileHandle, offset, Math.min(partSize, total - offset))
    if (!buff.length) return '读取 Box 上传分片失败'
    fileDigest.update(buff)
    const digest = createHash('sha1').update(buff).digest('base64')
    const uploaded = await boxUploadRequestWithRetry<{ part?: BoxUploadPart }>(
      accessToken,
      uploadPartUrl,
      {
        method: 'PUT',
        headers: buildBoxPartHeaders(offset, buff.length, total, digest),
        body: new Uint8Array(buff)
      },
      '上传 Box 分片失败'
    )
    const part = uploaded.data?.part
    if (uploaded.error || !part?.part_id) return uploaded.error || 'Box 分片响应无效'
    parts.push(part)
    offset += buff.length
    await recordUploadProgress(fileui.UploadID, buff.length, offset)
  }

  const commitBody = JSON.stringify(buildBoxCommitBody(parts))
  const digest = fileDigest.digest('base64')
  for (let attempt = 0; attempt < 6; attempt++) {
    const committed = await boxUploadRequest<{ entries?: Array<{ id?: string }> }>(
      accessToken,
      commitUrl,
      {
        method: 'POST',
        headers: { Digest: `sha=${digest}`, 'Content-Type': 'application/json' },
        body: commitBody
      },
      '提交 Box 上传失败'
    )
    if (committed.error) return committed.error
    const fileId = committed.data?.entries?.[0]?.id || ''
    if (fileId) {
      fileui.File.uploaded_file_id = fileId
      fileui.File.uploaded_is_rapid = false
      return 'success'
    }
    if (committed.status !== 202) return 'Box 上传提交响应无效'
    await Sleep(Math.max(1, committed.retryAfter) * 1000)
  }
  return 'Box 上传提交超时'
}

export default class BoxUploadDisk {
  static async UploadOneFile(fileui: IUploadingUI): Promise<string> {
    const token = await getBoxToken(fileui.user_id)
    if (!token?.access_token) return '找不到上传token，请重试'
    if (fileui.encType) return 'Box 暂不支持加密上传'

    const filePath = path.join(fileui.localFilePath, fileui.File.partPath)
    const fileName = path.basename(fileui.File.name)
    const { OpenFileHandle } = await import('../utils/filehelper')
    const opened = await OpenFileHandle(filePath)
    if (opened.error || !opened.handle) return opened.error || '打开文件失败'
    fileui.Info.uploadState = 'running'
    try {
      if (fileui.File.size <= BOX_CHUNK_UPLOAD_THRESHOLD) return await uploadBoxSmallFile(opened.handle, fileui, fileName)
      return await uploadBoxSessionFile(token.access_token, opened.handle, fileui, fileName)
    } finally {
      await opened.handle.close().catch(() => {})
    }
  }
}
