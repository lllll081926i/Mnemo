import { S3Client } from '@aws-sdk/client-s3'
import { Upload } from '@aws-sdk/lib-storage'
import type { FileHandle } from 'fs/promises'
import type { IUploadingUI } from './dbupload'
import { recordProviderUploadProgress } from './providerUpload'

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
  contentType?: string
}

export const buildPikPakS3ClientConfig = (credentials: OssUploadCredentials) => ({
  endpoint: 'https://mypikpak.com',
  region: 'pikpak',
  credentials: {
    accessKeyId: credentials.accessKeyId,
    secretAccessKey: credentials.accessKeySecret,
    sessionToken: credentials.securityToken || undefined
  }
})

export const uploadOssFile = async (fileui: IUploadingUI, handle: FileHandle, credentials: OssUploadCredentials, options: OssUploadOptions = {}) => {
  if (!credentials.bucket || !credentials.objectPath || !credentials.accessKeyId || !credentials.accessKeySecret) return 'PikPak S3 上传凭证不完整'
  if (!fileui.IsRunning) return '已暂停'
  const client = new S3Client(buildPikPakS3ClientConfig(credentials))
  const upload = new Upload({
    client,
    params: {
      Bucket: credentials.bucket,
      Key: credentials.objectPath,
      Body: handle.createReadStream({ autoClose: false }),
      ContentLength: fileui.File.size,
      ContentType: options.contentType || 'application/octet-stream'
    },
    partSize: Math.max(5 * 1024 * 1024, options.partSize || 5 * 1024 * 1024),
    queueSize: 4,
    leavePartsOnError: false
  })
  let uploaded = 0
  upload.on('httpUploadProgress', (progress) => {
    const position = Math.min(fileui.File.size, Number(progress.loaded || 0))
    const delta = Math.max(0, position - uploaded)
    uploaded = Math.max(uploaded, position)
    if (delta > 0) void recordProviderUploadProgress(fileui, delta, uploaded)
    if (!fileui.IsRunning) void upload.abort()
  })
  try {
    await upload.done()
    if (!fileui.IsRunning) return '已暂停'
    if (uploaded < fileui.File.size) await recordProviderUploadProgress(fileui, fileui.File.size - uploaded, fileui.File.size)
    return ''
  } catch (error: any) {
    if (!fileui.IsRunning || error?.name === 'AbortError') return '已暂停'
    return error?.message || 'PikPak S3 上传失败'
  } finally {
    client.destroy()
  }
}
