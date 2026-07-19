import { createHmac } from 'crypto'
import type { FileHandle } from 'fs/promises'
import type { IUploadingUI } from './dbupload'
import { fetchProviderUploadWithRetry, readProviderUploadSlice, recordProviderUploadProgress } from './providerUpload'

export interface OssUploadCredentials {
  endpoint: string
  bucket: string
  objectPath: string
  accessKeyId: string
  accessKeySecret: string
  securityToken?: string
}

export interface OssUploadOptions {
  partSize?: number
  singlePutThreshold?: number
  contentType?: string
}

const normalizeEndpoint = (endpoint: string) => (endpoint.startsWith('http') ? endpoint : `https://${endpoint}`)

const encodeObjectPath = (objectPath: string) =>
  objectPath
    .split('/')
    .map((part) => encodeURIComponent(part))
    .join('/')

export const buildOssCanonicalResource = (credentials: OssUploadCredentials, query = '') => {
  return `/${credentials.bucket}/${credentials.objectPath}${query ? `?${query}` : ''}`
}

export const buildOssAuthorization = (method: string, credentials: OssUploadCredentials, date: string, contentType = '', query = '') => {
  const ossHeaders = credentials.securityToken ? `x-oss-security-token:${credentials.securityToken}\n` : ''
  const canonical = `${method}\n\n${contentType}\n${date}\n${ossHeaders}${buildOssCanonicalResource(credentials, query)}`
  const signature = createHmac('sha1', credentials.accessKeySecret).update(canonical).digest('base64')
  return `OSS ${credentials.accessKeyId}:${signature}`
}

export const buildOssObjectUrl = (credentials: OssUploadCredentials, query = '') => {
  const endpoint = new URL(normalizeEndpoint(credentials.endpoint))
  if (!endpoint.hostname.startsWith(`${credentials.bucket}.`)) endpoint.hostname = `${credentials.bucket}.${endpoint.hostname}`
  endpoint.pathname = `${endpoint.pathname.replace(/\/$/, '')}/${encodeObjectPath(credentials.objectPath)}`
  endpoint.search = query ? `?${query}` : ''
  return endpoint.toString()
}

const ossFetch = (method: string, credentials: OssUploadCredentials, query: string, contentType: string, body?: BodyInit) =>
  fetchProviderUploadWithRetry(() => {
    const date = new Date().toUTCString()
    const headers: Record<string, string> = {
      Date: date,
      Authorization: buildOssAuthorization(method, credentials, date, contentType, query)
    }
    if (contentType) headers['Content-Type'] = contentType
    if (credentials.securityToken) headers['x-oss-security-token'] = credentials.securityToken
    return fetch(buildOssObjectUrl(credentials, query), { method, headers, body })
  })

const parseUploadId = (xml: string) => xml.match(/<UploadId>([^<]+)<\/UploadId>/)?.[1] || ''

const uploadOssSingleObject = async (fileui: IUploadingUI, handle: FileHandle, credentials: OssUploadCredentials, contentType: string) => {
  const buff = await readProviderUploadSlice(handle, 0, fileui.File.size)
  if (buff.length !== fileui.File.size) return '读取 OSS 上传文件失败'
  const response = await ossFetch('PUT', credentials, '', contentType, new Uint8Array(buff))
  if (!response.ok) return `OSS 上传失败 HTTP ${response.status}`
  await recordProviderUploadProgress(fileui, buff.length, buff.length)
  return ''
}

const abortOssMultipart = async (credentials: OssUploadCredentials, uploadId: string) => {
  if (!uploadId) return
  await ossFetch('DELETE', credentials, `uploadId=${encodeURIComponent(uploadId)}`, '').catch(() => undefined)
}

const uploadOssMultipart = async (fileui: IUploadingUI, handle: FileHandle, credentials: OssUploadCredentials, partSize: number, contentType: string) => {
  const initResponse = await ossFetch('POST', credentials, 'uploads', '')
  const initText = await initResponse.text().catch(() => '')
  if (!initResponse.ok) return `初始化 OSS 分片上传失败 HTTP ${initResponse.status}`
  const uploadId = parseUploadId(initText)
  if (!uploadId) return 'OSS 未返回分片上传 ID'
  fileui.Info.up_upload_id = uploadId

  const parts: Array<{ partNumber: number; eTag: string }> = []
  let completed = false
  try {
    let offset = 0
    let partNumber = 1
    while (offset < fileui.File.size) {
      if (!fileui.IsRunning) return '已暂停'
      const buff = await readProviderUploadSlice(handle, offset, Math.min(partSize, fileui.File.size - offset))
      if (!buff.length) return '读取 OSS 上传分片失败'
      const query = `partNumber=${partNumber}&uploadId=${encodeURIComponent(uploadId)}`
      const response = await ossFetch('PUT', credentials, query, contentType, new Uint8Array(buff))
      if (!response.ok) return `OSS 分片上传失败 HTTP ${response.status}`
      const eTag = (response.headers.get('etag') || '').replace(/"/g, '')
      if (!eTag) return 'OSS 分片响应缺少 ETag'
      parts.push({ partNumber, eTag })
      offset += buff.length
      await recordProviderUploadProgress(fileui, buff.length, offset)
      partNumber += 1
    }

    const xml = `<?xml version="1.0" encoding="UTF-8"?><CompleteMultipartUpload>${parts.map((part) => `<Part><PartNumber>${part.partNumber}</PartNumber><ETag>${part.eTag}</ETag></Part>`).join('')}</CompleteMultipartUpload>`
    const response = await ossFetch('POST', credentials, `uploadId=${encodeURIComponent(uploadId)}`, 'application/xml', xml)
    if (!response.ok) return `OSS 分片合并失败 HTTP ${response.status}`
    completed = true
    return ''
  } finally {
    if (!completed) await abortOssMultipart(credentials, uploadId)
  }
}

export const uploadOssFile = async (fileui: IUploadingUI, handle: FileHandle, credentials: OssUploadCredentials, options: OssUploadOptions = {}) => {
  if (!credentials.endpoint || !credentials.bucket || !credentials.objectPath || !credentials.accessKeyId || !credentials.accessKeySecret) return 'OSS 上传凭证不完整'
  const contentType = options.contentType || 'application/octet-stream'
  const singlePutThreshold = options.singlePutThreshold ?? 10 * 1024 * 1024
  if (fileui.File.size <= singlePutThreshold) return uploadOssSingleObject(fileui, handle, credentials, contentType)
  return uploadOssMultipart(fileui, handle, credentials, options.partSize || 8 * 1024 * 1024, contentType)
}
