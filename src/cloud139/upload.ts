import type { FileHandle } from 'fs/promises'
import type { IUploadingUI } from '../utils/dbupload'
import UserDAL from '../user/userdal'
import { cloud139Request } from './dirfilelist'
import { computeProviderFileHashes, fetchProviderUploadWithRetry, openProviderUploadFile, readProviderUploadSlice, recordProviderUploadProgress } from '../utils/providerUpload'
import { buildCloud139PartInfos, buildCloud139UploadCreateBody, type Cloud139PartInfo, type Cloud139UploadPartInfo } from './uploadProtocol'
import { apiCloud139FileList } from './dirfilelist'
import { apiCloud139TrashBatch } from './filecmd'
import { resolveProviderUploadConflict } from '../utils/providerUploadConflict'

const uploadCloud139Parts = async (fileui: IUploadingUI, handle: FileHandle, allParts: Cloud139PartInfo[], uploadParts: Cloud139UploadPartInfo[]) => {
  const sorted = [...uploadParts].sort((a, b) => a.partNumber - b.partNumber)
  for (const uploadPart of sorted) {
    if (!fileui.IsRunning) return '已暂停'
    const part = allParts[uploadPart.partNumber - 1]
    if (!part || !uploadPart.uploadUrl) return `139 云盘分片 ${uploadPart.partNumber} 参数无效`
    const buff = await readProviderUploadSlice(handle, part.parallelHashCtx.partOffset, part.partSize)
    if (buff.length !== part.partSize) return `读取 139 云盘分片 ${uploadPart.partNumber} 失败`
    const response = await fetchProviderUploadWithRetry(() =>
      fetch(uploadPart.uploadUrl, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/octet-stream',
          Origin: 'https://yun.139.com',
          Referer: 'https://yun.139.com/'
        },
        body: new Uint8Array(buff)
      })
    )
    if (!response.ok) return `139 云盘分片上传失败 HTTP ${response.status}`
    await recordProviderUploadProgress(fileui, buff.length, part.parallelHashCtx.partOffset + buff.length)
  }
  return ''
}

export default class Cloud139UploadDisk {
  static async UploadOneFile(fileui: IUploadingUI): Promise<string> {
    if (fileui.encType) return '139 云盘暂不支持加密上传'
    const conflictError = await resolveProviderUploadConflict(fileui, {
      findConflict: async (name) => {
        const items = await apiCloud139FileList(fileui.user_id, fileui.parent_file_id, 1000)
        const item = items.find((entry) => String(entry.name || entry.contentName || '') === name)
        const id = String(item?.fileId || item?.contentID || '')
        return item && id ? { id, name } : undefined
      },
      removeConflict: async (item) => (await apiCloud139TrashBatch(fileui.user_id, [item.id])).includes(item.id)
    })
    if (conflictError) return conflictError
    fileui.Info.uploadState = 'hashing'
    const hashed = await computeProviderFileHashes(fileui, ['sha256'])
    const contentHash = hashed.hashes.sha256 || ''
    if (!contentHash) return hashed.error || '计算 139 云盘 SHA256 失败'
    if (!fileui.IsRunning) return '已暂停'

    const partInfos = buildCloud139PartInfos(fileui.File.size)
    let created: any
    try {
      created = await cloud139Request(fileui.user_id, '/file/create', buildCloud139UploadCreateBody(fileui, contentHash, partInfos))
    } catch (error: any) {
      return error?.message || '创建 139 云盘上传任务失败'
    }
    const uploadData = created?.data || created || {}
    const fileId = String(uploadData.fileId || '')
    const uploadId = String(uploadData.uploadId || '')
    fileui.Info.up_file_id = fileId
    fileui.Info.up_upload_id = uploadId
    if (uploadData.exist || uploadData.rapidUpload || (fileui.File.size === 0 && (!Array.isArray(uploadData.partInfos) || uploadData.partInfos.length === 0))) {
      if (!fileId) return '139 云盘秒传响应缺少文件 ID'
      fileui.File.uploaded_file_id = fileId
      fileui.File.uploaded_is_rapid = true
      await recordProviderUploadProgress(fileui, fileui.File.size, fileui.File.size)
      return 'success'
    }
    if (!Array.isArray(uploadData.partInfos) || uploadData.partInfos.length === 0) return '139 云盘上传任务未返回分片地址'
    if (!fileId || !uploadId) return '139 云盘上传任务缺少文件 ID 或上传 ID'

    const opened = await openProviderUploadFile(fileui)
    if (!opened.handle) return opened.error
    fileui.Info.uploadState = 'running'
    try {
      let error = await uploadCloud139Parts(fileui, opened.handle, partInfos, uploadData.partInfos)
      if (error) return error
      for (let offset = 100; offset < partInfos.length; offset += 100) {
        if (!fileui.IsRunning) return '已暂停'
        const batch = partInfos.slice(offset, offset + 100)
        const token = UserDAL.GetUserToken(fileui.user_id) || (await UserDAL.GetUserTokenFromDB(fileui.user_id))
        const account = String(token?.user_name || token?.nick_name || token?.user_id || '')
          .replace(/^cloud139_/, '')
          .replace(/^139云盘\s*/, '')
        let response: any
        try {
          response = await cloud139Request(fileui.user_id, '/file/getUploadUrl', {
            fileId,
            uploadId,
            partInfos: batch,
            commonAccountInfo: { account, accountType: 1 }
          })
        } catch (requestError: any) {
          return requestError?.message || '获取 139 云盘分片地址失败'
        }
        error = await uploadCloud139Parts(fileui, opened.handle, partInfos, response?.data?.partInfos || response?.partInfos || [])
        if (error) return error
      }
    } finally {
      await opened.handle.close().catch(() => {})
    }

    try {
      await cloud139Request(fileui.user_id, '/file/complete', {
        contentHash,
        contentHashAlgorithm: 'SHA256',
        fileId,
        uploadId
      })
    } catch (error: any) {
      return error?.message || '完成 139 云盘上传任务失败'
    }
    fileui.File.uploaded_file_id = fileId
    fileui.File.uploaded_is_rapid = false
    return 'success'
  }
}
