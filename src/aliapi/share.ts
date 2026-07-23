import { humanExpiration } from '../utils/format'
import { isPikPakUser } from './utils'
import { IAliShareAnonymous, IAliShareFileItem, IAliShareItem } from './alimodels'
import type { IAliBatchResult } from './models'
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

export interface IAliShareFileResp {
  items: IAliShareFileItem[]
  itemsKey: Set<string>
  punished_file_count: number
  next_marker: string
  m_user_id: string
  m_share_id: string
  dirID: string
  dirName: string
}

export interface UpdateShareModel {
  share_id: string
  share_pwd: string
  expiration: string
  share_name: string
}

export default class AliShare {
  static async ApiShareFileCheckAvailable(_user_id: string, _drive_id: string, _file_id_list: string[]) {
    return []
  }

  static async ApiGetShareAnonymous(_share_id: string, _share_pwd = ''): Promise<IAliShareAnonymous> {
    return {
      shareinfo: {
        share_id: '',
        creator_id: '',
        creator_name: '',
        creator_phone: '',
        display_name: '',
        expiration: '',
        file_count: 0,
        share_name: '',
        created_at: '',
        updated_at: '',
        vip: '',
        is_photo_collection: false,
        album_id: ''
      },
      shareinfojson: '',
      error: '当前版本不支持导入第三方分享链接'
    }
  }

  static async ApiGetShareToken(_share_id: string, _pwd: string): Promise<string> {
    return '，当前版本不支持导入第三方分享链接'
  }

  static async ApiShareFileList(_share_id: string, _share_token: string, dirID: string): Promise<IAliShareFileResp> {
    return {
      items: [],
      itemsKey: new Set(),
      punished_file_count: 0,
      next_marker: '',
      m_user_id: '',
      m_share_id: '',
      dirID,
      dirName: ''
    }
  }

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

  static async ApiCreatShareBatch(_user_id: string, _drive_id: string, _expiration: string, _share_pwd: string, _file_id_list: string[]): Promise<IAliBatchResult> {
    return { count: 0, async_task: [], reslut: [], error: [] }
  }

  static async ApiCancelShareBatch(_user_id: string, _share_idList: string[]): Promise<string[]> {
    return []
  }

  static async ApiUpdateShareBatch(_user_id: string, _share_idList: string[], _expirationList: string[], _share_pwdList: string[], _share_nameList?: string[]): Promise<UpdateShareModel[]> {
    return []
  }

  static async ApiSaveShareFilesBatch(_share_id: string, _share_token: string, _user_id: string, _drive_id: string, _parent_file_id: string, _file_idList: string[]): Promise<string> {
    return '当前版本不支持转存第三方分享'
  }
}
