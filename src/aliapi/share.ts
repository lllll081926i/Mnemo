import { humanExpiration } from '../utils/format'
import { isPikPakUser } from './utils'
import { IAliShareItem } from './alimodels'
import { apiPikPakShareCreate } from '../pikpak/share'
import { getDriveProviderLabel, resolveDriveProvider } from '../utils/driveProvider'
import { apiOneDriveShareCreate } from '../onedrive/share'
import { apiDropboxShareCreate } from '../dropbox/share'
import { apiGoogleDriveCreateShare } from '../gdrive/share'
import { apiGofileCreateDirectLink } from '../gofile/share'
import { recordShareHistory } from '../share/sharehistory'

// 创建成功后写入本地分享记录（分享页聚合展示）
const recordShare = async (user_id: string, drive_id: string, item: IAliShareItem): Promise<IAliShareItem> => {
  try {
    const { default: UserDAL } = await import('../user/userdal')
    const token = UserDAL.GetUserToken(user_id)
    recordShareHistory({
      provider: resolveDriveProvider({ userId: user_id, driveId: drive_id }),
      user_id,
      account: token?.nick_name || token?.user_name || user_id,
      share_id: item.share_id,
      share_url: item.share_url,
      pass_code: item.share_pwd || '',
      share_name: item.share_name,
      file_count: item.file_id_list.length,
      expiration: item.expiration || ''
    })
  } catch {
    // 记录失败不影响分享本身
  }
  return item
}

const createProviderShareItem = (drive_id: string, file_id_list: string[], share_name: string, share_url: string, share_pwd = '', share_id = '', expiration = ''): IAliShareItem => ({
  created_at: '', creator: '', description: '', display_name: '', display_label: '', download_count: 0,
  drive_id, expiration, expired: false, file_id: file_id_list[0] || '', file_id_list, icon: 'iconwenjian',
  preview_count: 0, save_count: 0, share_id: share_id || share_url, share_msg: humanExpiration(expiration), full_share_msg: '',
  share_name: share_name || '分享链接', share_policy: '', share_pwd, share_url, status: '', updated_at: '',
  is_share_saved: false, share_saved: ''
})

export default class AliShare {
  static async ApiCreatShare(user_id: string, drive_id: string, expiration: string, share_pwd: string, share_name: string, file_id_list: string[]): Promise<string | IAliShareItem> {
    if (!user_id || !drive_id || file_id_list.length == 0) return '创建分享链接失败数据错误'
    if (isPikPakUser(user_id) || drive_id === 'pikpak') {
      const result = await apiPikPakShareCreate(user_id, file_id_list, !!share_pwd, expiration)
      if (result.error) return result.error
      return recordShare(user_id, drive_id, createProviderShareItem(drive_id, file_id_list, share_name, result.shareUrl, result.passCode || share_pwd || '', result.shareId, expiration))
    }
    const provider = resolveDriveProvider({ userId: user_id, driveId: drive_id })
    if (provider === 'onedrive') {
      const result = await apiOneDriveShareCreate(user_id, drive_id, file_id_list, share_name, expiration, share_pwd)
      if (!result.item) return result.error || '创建 OneDrive 分享链接失败'
      return recordShare(user_id, drive_id, result.item)
    }
    if (provider === 'dropbox') {
      const result = await apiDropboxShareCreate(user_id, drive_id, file_id_list, expiration, share_pwd, share_name)
      if (!result.item) return result.error || '创建 Dropbox 分享链接失败'
      return recordShare(user_id, drive_id, result.item)
    }
    if (provider === 'gdrive' || provider === 'gofile') {
      if (file_id_list.length !== 1) return `${provider === 'gdrive' ? 'Google Drive' : 'GoFile'} 分享链接一次只能选择一个文件或文件夹`
      try {
        const shareUrl = provider === 'gdrive' ? await apiGoogleDriveCreateShare(user_id, file_id_list[0]) : await apiGofileCreateDirectLink(user_id, file_id_list[0], expiration)
        return recordShare(user_id, drive_id, createProviderShareItem(drive_id, file_id_list, share_name, shareUrl, '', '', provider === 'gofile' ? expiration : ''))
      } catch (error: any) {
        return error?.message || '创建分享链接失败'
      }
    }
    return `${getDriveProviderLabel(provider)} 暂不支持创建分享链接`
  }
}
