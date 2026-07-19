import { createCipheriv } from 'crypto'
import type { IUploadingUI } from '../utils/dbupload'

const CLOUD189_DEFAULT_PART_SIZE = 10 * 1024 * 1024

export const getCloud189UploadPartSize = (fileSize: number) => {
  if (fileSize > CLOUD189_DEFAULT_PART_SIZE * 2 * 999) {
    return Math.max(5, Math.ceil(fileSize / 1999 / CLOUD189_DEFAULT_PART_SIZE)) * CLOUD189_DEFAULT_PART_SIZE
  }
  if (fileSize > CLOUD189_DEFAULT_PART_SIZE * 999) return CLOUD189_DEFAULT_PART_SIZE * 2
  return CLOUD189_DEFAULT_PART_SIZE
}

export const encodeCloud189UploadParams = (params: Record<string, string>) =>
  Object.keys(params)
    .sort()
    .map((key) => `${key}=${params[key]}`)
    .join('&')

export const encryptCloud189UploadParams = (params: Record<string, string>, sessionSecret: string) => {
  if (sessionSecret.length < 16) throw new Error('天翼云盘 SessionSecret 无效，请重新登录')
  const cipher = createCipheriv('aes-128-ecb', Buffer.from(sessionSecret.slice(0, 16)), null)
  cipher.setAutoPadding(true)
  return Buffer.concat([cipher.update(encodeCloud189UploadParams(params), 'utf8'), cipher.final()])
    .toString('hex')
    .toUpperCase()
}

export const encodeCloud189FileName = (name: string) => encodeURIComponent(name).replace(/%20/g, '+')

export const toCloud189UploadParentId = (parentId: string) => (parentId === 'cloud189_root' || parentId === '0' || parentId === '/' || parentId === '' ? '-11' : parentId)

export const buildCloud189InitUploadParams = (fileui: IUploadingUI, partSize: number) => ({
  parentFolderId: toCloud189UploadParentId(fileui.parent_file_id),
  fileName: encodeCloud189FileName(fileui.File.name.split(/[\\/]/).pop() || fileui.File.name),
  fileSize: String(fileui.File.size),
  sliceSize: String(partSize),
  lazyCheck: '1'
})

export const parseCloud189UploadHeaders = (headerText: string) => {
  const headers: Record<string, string> = {}
  const decoded = decodeURIComponent(headerText || '')
  for (const item of decoded.split('&')) {
    const index = item.indexOf('=')
    if (index > 0) headers[item.slice(0, index)] = item.slice(index + 1)
  }
  return headers
}
