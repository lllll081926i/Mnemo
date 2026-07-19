import { createHash } from 'crypto'
import path from 'path'
import mime from 'mime-types'
import type { IUploadingUI } from '../utils/dbupload'
import { quarkRequest } from './dirfilelist'
import { computeProviderFileHashes, fetchProviderUploadWithRetry, openProviderUploadFile, readProviderUploadSlice, recordProviderUploadProgress } from '../utils/providerUpload'
import { buildQuarkCompleteAuthMeta, buildQuarkCompleteXml, buildQuarkPartAuthMeta, buildQuarkUploadPreBody, type QuarkUploadPreData } from './uploadProtocol'
import { apiQuarkFileList } from './dirfilelist'
import { apiQuarkTrashBatch } from './filecmd'
import { resolveProviderUploadConflict } from '../utils/providerUploadConflict'

interface QuarkUploadPreResponse {
  data?: {
    task_id?: string
    finish?: boolean
    upload_id?: string
    obj_key?: string
    upload_url?: string
    fid?: string
    bucket?: string
    callback?: Record<string, any>
    auth_info?: string
  }
  metadata?: {
    part_size?: number
  }
}

interface QuarkUploadAuthResponse {
  data?: {
    auth_key?: string
  }
}

interface QuarkUploadHashResponse {
  data?: {
    finish?: boolean
    fid?: string
  }
}

interface QuarkUploadFinishResponse {
  data?: {
    fid?: string
  }
}

const isQuarkUploadError = (data: any) => !data || data.__error

const encodeQuarkObjectPath = (value: string) =>
  value
    .split('/')
    .map((part) => encodeURIComponent(part))
    .join('/')

const quarkUploadObjectUrl = (pre: QuarkUploadPreData) => {
  const uploadHost = String(pre.upload_url || '')
    .replace(/^https?:\/\//, '')
    .replace(/\/$/, '')
  return `https://${pre.bucket}.${uploadHost}/${encodeQuarkObjectPath(pre.obj_key || '')}`
}

const uploadQuarkPart = async (fileui: IUploadingUI, pre: QuarkUploadPreData, contentType: string, partNumber: number, buff: Buffer) => {
  const date = new Date().toUTCString()
  const auth = await quarkRequest<QuarkUploadAuthResponse>(
    fileui.user_id,
    'file/upload/auth',
    {
      method: 'POST',
      body: JSON.stringify({
        auth_info: pre.auth_info,
        auth_meta: buildQuarkPartAuthMeta(pre, contentType, date, partNumber),
        task_id: pre.task_id
      })
    },
    {},
    true
  )
  if (isQuarkUploadError(auth) || !(auth as QuarkUploadAuthResponse).data?.auth_key) return { eTag: '', error: (auth as any)?.message || '获取夸克分片授权失败' }
  const url = `${quarkUploadObjectUrl(pre)}?partNumber=${partNumber}&uploadId=${encodeURIComponent(pre.upload_id || '')}`
  const response = await fetchProviderUploadWithRetry(() =>
    fetch(url, {
      method: 'PUT',
      headers: {
        Authorization: (auth as QuarkUploadAuthResponse).data!.auth_key!,
        'Content-Type': contentType,
        Referer: 'https://pan.quark.cn/',
        'x-oss-date': date,
        'x-oss-user-agent': 'aliyun-sdk-js/6.6.1 Chrome 98.0.4758.80 on Windows 10 64-bit'
      },
      body: new Uint8Array(buff)
    })
  )
  if (!response.ok) return { eTag: '', error: `夸克分片上传失败 HTTP ${response.status}` }
  const eTag = response.headers.get('etag') || ''
  return { eTag, error: eTag ? '' : '夸克分片响应缺少 ETag' }
}

const commitQuarkUpload = async (fileui: IUploadingUI, pre: QuarkUploadPreData, eTags: string[]) => {
  const xml = buildQuarkCompleteXml(eTags)
  const contentMd5 = createHash('md5').update(xml).digest('base64')
  const callbackBase64 = Buffer.from(JSON.stringify(pre.callback || {})).toString('base64')
  const date = new Date().toUTCString()
  const auth = await quarkRequest<QuarkUploadAuthResponse>(
    fileui.user_id,
    'file/upload/auth',
    {
      method: 'POST',
      body: JSON.stringify({
        auth_info: pre.auth_info,
        auth_meta: buildQuarkCompleteAuthMeta(pre, contentMd5, callbackBase64, date),
        task_id: pre.task_id
      })
    },
    {},
    true
  )
  if (isQuarkUploadError(auth) || !(auth as QuarkUploadAuthResponse).data?.auth_key) return (auth as any)?.message || '获取夸克合并授权失败'
  const response = await fetchProviderUploadWithRetry(() =>
    fetch(`${quarkUploadObjectUrl(pre)}?uploadId=${encodeURIComponent(pre.upload_id || '')}`, {
      method: 'POST',
      headers: {
        Authorization: (auth as QuarkUploadAuthResponse).data!.auth_key!,
        'Content-MD5': contentMd5,
        'Content-Type': 'application/xml',
        Referer: 'https://pan.quark.cn/',
        'x-oss-callback': callbackBase64,
        'x-oss-date': date,
        'x-oss-user-agent': 'aliyun-sdk-js/6.6.1 Chrome 98.0.4758.80 on Windows 10 64-bit'
      },
      body: xml
    })
  )
  return response.ok ? '' : `夸克分片合并失败 HTTP ${response.status}`
}

export default class QuarkUploadDisk {
  static async UploadOneFile(fileui: IUploadingUI): Promise<string> {
    if (fileui.encType) return '夸克网盘暂不支持加密上传'
    const conflictError = await resolveProviderUploadConflict(fileui, {
      findConflict: async (name) => {
        for (let page = 1; page <= 20; page++) {
          const result = await apiQuarkFileList(fileui.user_id, fileui.parent_file_id, 100, page)
          const item = result.items.find((entry) => entry.file_name === name)
          if (item) return { id: item.fid, name: item.file_name }
          if (page * 100 >= result.total) return undefined
        }
        return undefined
      },
      removeConflict: async (item) => (await apiQuarkTrashBatch(fileui.user_id, [item.id])).includes(item.id)
    })
    if (conflictError) return conflictError
    fileui.Info.uploadState = 'hashing'
    const hashed = await computeProviderFileHashes(fileui, ['md5', 'sha1'])
    if (hashed.error) return hashed.error
    if (!fileui.IsRunning) return '已暂停'

    const name = path.basename(fileui.File.name)
    const contentType = mime.lookup(name) || 'application/octet-stream'
    const pre = await quarkRequest<QuarkUploadPreResponse>(
      fileui.user_id,
      'file/upload/pre',
      {
        method: 'POST',
        body: JSON.stringify(buildQuarkUploadPreBody(fileui.parent_file_id, name, fileui.File.size, contentType))
      },
      {},
      true
    )
    if (isQuarkUploadError(pre) || !(pre as QuarkUploadPreResponse).data?.task_id) return (pre as any)?.message || '创建夸克上传任务失败'
    const preData = (pre as QuarkUploadPreResponse).data!
    fileui.Info.up_upload_id = preData.upload_id || ''
    fileui.Info.up_file_id = preData.fid || ''

    const hashResult = await quarkRequest<QuarkUploadHashResponse>(
      fileui.user_id,
      'file/update/hash',
      {
        method: 'POST',
        body: JSON.stringify({ md5: hashed.hashes.md5, sha1: hashed.hashes.sha1, task_id: preData.task_id })
      },
      {},
      true
    )
    if (isQuarkUploadError(hashResult)) return (hashResult as any)?.message || '夸克秒传校验失败'
    const hashData = (hashResult as QuarkUploadHashResponse).data
    if (hashData?.finish || preData.finish) {
      const fileId = hashData?.fid || preData.fid || ''
      if (!fileId) return '夸克秒传响应缺少文件 ID'
      fileui.File.uploaded_file_id = fileId
      fileui.File.uploaded_is_rapid = true
      await recordProviderUploadProgress(fileui, fileui.File.size, fileui.File.size)
      return 'success'
    }

    if (!preData.upload_id || !preData.upload_url || !preData.bucket || !preData.obj_key) return '夸克上传任务缺少分片参数'
    const opened = await openProviderUploadFile(fileui)
    if (!opened.handle) return opened.error
    fileui.Info.uploadState = 'running'
    const eTags: string[] = []
    try {
      const partSize = Number((pre as QuarkUploadPreResponse).metadata?.part_size || 8 * 1024 * 1024)
      let offset = 0
      let partNumber = 1
      while (offset < fileui.File.size) {
        if (!fileui.IsRunning) return '已暂停'
        const buff = await readProviderUploadSlice(opened.handle, offset, Math.min(partSize, fileui.File.size - offset))
        if (!buff.length && fileui.File.size > 0) return '读取夸克上传分片失败'
        let uploaded = { eTag: '', error: '夸克分片上传失败' }
        for (let attempt = 0; attempt < 3; attempt++) {
          uploaded = await uploadQuarkPart(fileui, preData, contentType, partNumber, buff)
          if (!uploaded.error) break
        }
        if (uploaded.error) return uploaded.error
        eTags.push(uploaded.eTag)
        offset += buff.length
        await recordProviderUploadProgress(fileui, buff.length, offset)
        partNumber += 1
      }
    } finally {
      await opened.handle.close().catch(() => {})
    }

    const commitError = await commitQuarkUpload(fileui, preData, eTags)
    if (commitError) return commitError
    const finished = await quarkRequest<QuarkUploadFinishResponse>(
      fileui.user_id,
      'file/upload/finish',
      {
        method: 'POST',
        body: JSON.stringify({ obj_key: preData.obj_key, task_id: preData.task_id })
      },
      {},
      true
    )
    if (isQuarkUploadError(finished)) return (finished as any)?.message || '完成夸克上传任务失败'
    const fileId = (finished as QuarkUploadFinishResponse).data?.fid || preData.fid || ''
    if (!fileId) return '夸克上传完成响应缺少文件 ID'
    fileui.File.uploaded_file_id = fileId
    fileui.File.uploaded_is_rapid = false
    return 'success'
  }
}
