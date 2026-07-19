import type { OssUploadCredentials } from '../utils/ossUpload'

export interface PikPakUploadParams {
  access_key_id?: string
  access_key_secret?: string
  bucket?: string
  endpoint?: string
  key?: string
  security_token?: string
}

export interface PikPakUploadCreateResponse {
  upload_type?: string
  resumable?: {
    params?: PikPakUploadParams
  }
  file?: {
    id?: string
  }
  id?: string
  error?: string
  error_description?: string
  message?: string
}

export const normalizePikPakOssEndpoint = (endpoint: string) => {
  const normalized = endpoint.replace(/^https?:\/\//, '').replace(/\/$/, '')
  return normalized.endsWith('.mypikpak.net') ? 'mypikpak.net' : normalized
}

export const buildPikPakUploadBody = (parentId: string, name: string, size: number, gcid: string) => ({
  kind: 'drive#file',
  name,
  size,
  hash: gcid,
  upload_type: 'UPLOAD_TYPE_RESUMABLE',
  objProvider: { provider: 'UPLOAD_TYPE_UNKNOWN' },
  parent_id: parentId === 'pikpak_root' || parentId.includes('root') ? undefined : parentId,
  folder_type: 'NORMAL'
})

export const toPikPakOssCredentials = (params: PikPakUploadParams): OssUploadCredentials => ({
  endpoint: normalizePikPakOssEndpoint(params.endpoint || ''),
  bucket: params.bucket || '',
  objectPath: params.key || '',
  accessKeyId: params.access_key_id || '',
  accessKeySecret: params.access_key_secret || '',
  securityToken: params.security_token || ''
})
