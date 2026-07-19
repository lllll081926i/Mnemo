import path from 'path'
import mime from 'mime-types'
import type { IUploadingUI } from '../utils/dbupload'
import UserDAL from '../user/userdal'
import { captchaSign, getPikPakClientId, pikpakAuthHeaders } from './auth'
import { computeProviderGcid, fetchProviderUploadWithRetry, openProviderUploadFile, parseProviderUploadResponse } from '../utils/providerUpload'
import { uploadOssFile } from '../utils/ossUpload'
import { buildPikPakUploadBody, toPikPakOssCredentials, type PikPakUploadCreateResponse } from './uploadProtocol'
import { apiPikPakFileList } from './dirfilelist'
import { apiPikPakTrashBatch } from './filecmd'
import { resolveProviderUploadConflict } from '../utils/providerUploadConflict'

const PIKPAK_FILES_URL = 'https://api-drive.mypikpak.com/drive/v1/files'

const createPikPakUpload = async (fileui: IUploadingUI, gcid: string): Promise<{ data?: PikPakUploadCreateResponse; error: string }> => {
  const token = await UserDAL.GetUserTokenFromDB(fileui.user_id)
  if (!token?.access_token) return { error: '找不到 PikPak 上传 token，请重新登录' }
  const fileName = path.basename(fileui.File.name)
  const timestamp = Date.now().toString()
  const captchaResponse = await fetchProviderUploadWithRetry(() =>
    fetch('https://user.mypikpak.com/v1/shield/captcha/init', {
      method: 'POST',
      headers: pikpakAuthHeaders(token),
      body: JSON.stringify({
        client_id: getPikPakClientId(),
        action: 'POST:/drive/v1/files',
        device_id: token.device_id || '',
        meta: {
          captcha_sign: captchaSign(token.device_id || '', timestamp),
          client_version: '1.47.1',
          package_name: 'com.pikcloud.pikpak',
          user_id: token.user_id.replace(/^pikpak_/, ''),
          timestamp
        }
      })
    })
  )
  const captcha = await parseProviderUploadResponse(captchaResponse)
  if (!captchaResponse.ok || captcha.data?.error || !captcha.data?.captcha_token) {
    return { error: captcha.data?.error_description || captcha.data?.message || captcha.data?.error || '获取 PikPak 上传验证信息失败' }
  }
  const headers = pikpakAuthHeaders(token) as Record<string, string>
  headers['X-Captcha-Token'] = captcha.data.captcha_token
  const response = await fetchProviderUploadWithRetry(() =>
    fetch(PIKPAK_FILES_URL, {
      method: 'POST',
      headers,
      body: JSON.stringify(buildPikPakUploadBody(fileui.parent_file_id, fileName, fileui.File.size, gcid))
    })
  )
  const { data } = await parseProviderUploadResponse(response)
  if (!response.ok || data?.error) return { error: data?.error_description || data?.message || data?.error || `创建 PikPak 上传任务失败 HTTP ${response.status}` }
  return { data, error: '' }
}

export default class PikPakUploadDisk {
  static async UploadOneFile(fileui: IUploadingUI): Promise<string> {
    if (fileui.encType) return 'PikPak 暂不支持加密上传'
    const conflictError = await resolveProviderUploadConflict(fileui, {
      findConflict: async (name) => {
        let pageToken = ''
        for (let page = 0; page < 20; page++) {
          const result = await apiPikPakFileList(fileui.user_id, fileui.parent_file_id, 100, pageToken)
          const item = result.items.find((entry) => entry.name === name)
          if (item) return { id: item.id, name: item.name }
          if (!result.nextPageToken) return undefined
          pageToken = result.nextPageToken
        }
        return undefined
      },
      removeConflict: async (item) => (await apiPikPakTrashBatch(fileui.user_id, [item.id])).includes(item.id)
    })
    if (conflictError) return conflictError
    fileui.Info.uploadState = 'hashing'
    const { gcid, error } = await computeProviderGcid(fileui)
    if (!gcid) return error || '计算 PikPak GCID 失败'
    if (!fileui.IsRunning) return '已暂停'

    const created = await createPikPakUpload(fileui, gcid)
    if (!created.data) return created.error || '创建 PikPak 上传任务失败'
    const fileId = created.data.file?.id || created.data.id || ''
    if (!fileId) return 'PikPak 上传响应缺少文件 ID'
    fileui.Info.up_file_id = fileId
    if (!created.data.resumable?.params) {
      fileui.File.uploaded_file_id = fileId
      fileui.File.uploaded_is_rapid = true
      return 'success'
    }

    const opened = await openProviderUploadFile(fileui)
    if (!opened.handle) return opened.error
    fileui.Info.uploadState = 'running'
    try {
      const contentType = mime.lookup(path.basename(fileui.File.name)) || 'application/octet-stream'
      const uploadError = await uploadOssFile(fileui, opened.handle, toPikPakOssCredentials(created.data.resumable.params), { contentType })
      if (uploadError) return uploadError
      fileui.File.uploaded_file_id = fileId
      fileui.File.uploaded_is_rapid = false
      return 'success'
    } finally {
      await opened.handle.close().catch(() => {})
    }
  }
}
