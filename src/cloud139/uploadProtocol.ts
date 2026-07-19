import type { IUploadingUI } from '../utils/dbupload'

export interface Cloud139PartInfo {
  partNumber: number
  partSize: number
  parallelHashCtx: {
    partOffset: number
  }
}

export interface Cloud139UploadPartInfo {
  partNumber: number
  uploadUrl: string
}

export const toCloud139UploadParentId = (parentId: string) => (parentId === 'cloud139_root' || parentId === '0' || parentId === '' ? '/' : parentId)

export const getCloud139UploadPartSize = (fileSize: number) => (fileSize > 30 * 1024 * 1024 * 1024 ? 512 * 1024 * 1024 : 100 * 1024 * 1024)

export const buildCloud139PartInfos = (fileSize: number, partSize = getCloud139UploadPartSize(fileSize)): Cloud139PartInfo[] => {
  const parts: Cloud139PartInfo[] = []
  for (let offset = 0, partNumber = 1; offset < fileSize; offset += partSize, partNumber++) {
    parts.push({
      partNumber,
      partSize: Math.min(partSize, fileSize - offset),
      parallelHashCtx: { partOffset: offset }
    })
  }
  return parts
}

export const buildCloud139UploadCreateBody = (fileui: IUploadingUI, contentHash: string, partInfos: Cloud139PartInfo[]) => ({
  contentHash,
  contentHashAlgorithm: 'SHA256',
  contentType: 'application/octet-stream',
  parallelUpload: false,
  partInfos: partInfos.slice(0, 100),
  size: fileui.File.size,
  parentFileId: toCloud139UploadParentId(fileui.parent_file_id),
  name: fileui.File.name.split(/[\\/]/).pop() || fileui.File.name,
  type: 'file',
  fileRenameMode: 'auto_rename'
})
