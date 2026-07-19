import { createHash } from 'crypto'
import type { IUploadingUI } from '../utils/dbupload'
import UserDAL from '../user/userdal'
import { CLOUD189_USER_AGENT, CLOUD189_WEB_URL, cloud189ClientSuffix, cloud189SignatureHeaders } from './auth'
import { fetchProviderUploadWithRetry, openProviderUploadFile, parseProviderUploadResponse, readProviderUploadSlice, recordProviderUploadProgress } from '../utils/providerUpload'
import { buildCloud189InitUploadParams, encryptCloud189UploadParams, getCloud189UploadPartSize, parseCloud189UploadHeaders } from './uploadProtocol'
import { apiCloud189FileList } from './dirfilelist'
import { apiCloud189TrashBatch } from './filecmd'
import { resolveProviderUploadConflict } from '../utils/providerUploadConflict'

const CLOUD189_UPLOAD_URL = 'https://upload.cloud.189.cn'
const cloud189UploadRequest = async (user_id: string, action: string, params: Record<string, string>) => {
  let token = UserDAL.GetUserToken(user_id)
  if (!token?.open_api_access_token || !token?.open_api_refresh_token) token = (await UserDAL.GetUserTokenFromDB(user_id)) as any
  const sessionKey = token?.open_api_access_token || ''
  const sessionSecret = token?.open_api_refresh_token || token?.signature || ''
  if (!sessionKey || !sessionSecret) throw new Error('天翼云盘登录态无效，请重新登录')
  const url = `${CLOUD189_UPLOAD_URL}${action}`
  const encrypted = encryptCloud189UploadParams(params, sessionSecret)
  const query = new URLSearchParams({ ...cloud189ClientSuffix(), params: encrypted })
  const response = await fetchProviderUploadWithRetry(() =>
    fetch(`${url}?${query.toString()}`, {
      method: 'GET',
      headers: {
        Accept: 'application/json;charset=UTF-8',
        'User-Agent': CLOUD189_USER_AGENT,
        Referer: CLOUD189_WEB_URL,
        ...cloud189SignatureHeaders(sessionKey, sessionSecret, 'GET', url, encrypted)
      }
    })
  )
  const parsed = await parseProviderUploadResponse(response)
  if (!response.ok || (parsed.data?.code && parsed.data.code !== 'SUCCESS')) throw new Error(parsed.data?.msg || parsed.data?.message || `天翼云盘上传请求失败 HTTP ${response.status}`)
  return parsed.data || {}
}

const getCloud189UploadUrl = (response: any, partNumber: number) => {
  const uploadUrls = response?.uploadUrls || response?.data?.uploadUrls || response?.data || {}
  return uploadUrls[`partNumber_${partNumber}`] || {}
}

export default class Cloud189UploadDisk {
  static async UploadOneFile(fileui: IUploadingUI): Promise<string> {
    if (fileui.encType) return '天翼云盘暂不支持加密上传'
    if (fileui.File.size <= 0) return '天翼云盘暂不支持上传空文件'
    if (!fileui.IsRunning) return '已暂停'
    const conflictError = await resolveProviderUploadConflict(fileui, {
      replaceModes: ['ignore'],
      findConflict: async (name) => {
        const items = await apiCloud189FileList(fileui.user_id, fileui.parent_file_id, 1000)
        const item = items.find((entry) => String(entry.name || entry.fileName || '') === name)
        const id = String(item?.id || item?.fileId || '')
        return item && id ? { id, name } : undefined
      },
      removeConflict: async (item) => (await apiCloud189TrashBatch(fileui.user_id, [item.id])).includes(item.id)
    })
    if (conflictError) return conflictError

    const partSize = getCloud189UploadPartSize(fileui.File.size)
    let initialized: any
    try {
      initialized = await cloud189UploadRequest(fileui.user_id, '/person/initMultiUpload', buildCloud189InitUploadParams(fileui, partSize))
    } catch (error: any) {
      return error?.message || '创建天翼云盘上传任务失败'
    }
    const initData = initialized?.data || initialized || {}
    const uploadFileId = String(initData.uploadFileId || '')
    if (!uploadFileId) return '天翼云盘未返回上传文件 ID'
    fileui.Info.up_upload_id = uploadFileId

    const opened = await openProviderUploadFile(fileui)
    if (!opened.handle) return opened.error
    fileui.Info.uploadState = 'running'
    const fullMd5 = createHash('md5')
    const partMd5List: string[] = []
    try {
      let offset = 0
      let partNumber = 1
      while (offset < fileui.File.size) {
        if (!fileui.IsRunning) return '已暂停'
        const buff = await readProviderUploadSlice(opened.handle, offset, Math.min(partSize, fileui.File.size - offset))
        if (!buff.length) return '读取天翼云盘上传分片失败'
        fullMd5.update(buff)
        const partMd5 = createHash('md5').update(buff).digest()
        partMd5List.push(partMd5.toString('hex').toUpperCase())
        let uploadUrlResponse: any
        try {
          uploadUrlResponse = await cloud189UploadRequest(fileui.user_id, '/person/getMultiUploadUrls', {
            partInfo: `${partNumber}-${partMd5.toString('base64')}`,
            uploadFileId
          })
        } catch (error: any) {
          return error?.message || '获取天翼云盘分片地址失败'
        }
        const uploadData = getCloud189UploadUrl(uploadUrlResponse, partNumber)
        if (!uploadData.requestURL) return `天翼云盘未返回第 ${partNumber} 个分片地址`
        const response = await fetchProviderUploadWithRetry(() =>
          fetch(uploadData.requestURL, {
            method: 'PUT',
            headers: parseCloud189UploadHeaders(uploadData.requestHeader || ''),
            body: new Uint8Array(buff)
          })
        )
        if (!response.ok) return `天翼云盘分片上传失败 HTTP ${response.status}`
        offset += buff.length
        await recordProviderUploadProgress(fileui, buff.length, offset)
        partNumber += 1
      }
    } finally {
      await opened.handle.close().catch(() => {})
    }

    const fileMd5 = fullMd5.digest('hex').toUpperCase()
    const sliceMd5 = partMd5List.length > 1 ? createHash('md5').update(partMd5List.join('\n')).digest('hex').toUpperCase() : fileMd5
    let committed: any
    try {
      committed = await cloud189UploadRequest(fileui.user_id, '/person/commitMultiUploadFile', {
        uploadFileId,
        fileMd5,
        sliceMd5,
        lazyCheck: '1',
        isLog: '0',
        opertype: fileui.check_name_mode === 'overwrite' ? '3' : '1'
      })
    } catch (error: any) {
      return error?.message || '完成天翼云盘上传任务失败'
    }
    const file = committed?.file || committed?.data?.file || {}
    const fileId = String(file.userFileId || file.fileId || '')
    if (!fileId) return '天翼云盘上传完成响应缺少文件 ID'
    fileui.Info.up_file_id = fileId
    fileui.File.uploaded_file_id = fileId
    fileui.File.uploaded_is_rapid = false
    return 'success'
  }
}
