import AliShareList from '../../aliapi/sharelist'
import DB from '../../utils/db'
import { humanDateTime, humanExpiration, Sleep } from '../../utils/format'
import message from '../../utils/message'
import useMyShareStore from './MyShareStore'
import useOtherShareStore, { IOtherShareLinkModel } from './OtherShareStore'
import { IID, ParseShareIDList } from '../../utils/shareurl'
import { RunBatch } from '../../aliapi/batch'
import AliShare from '../../aliapi/share'
import { IAliShareAnonymous } from '../../aliapi/alimodels'
import useMyTransferShareStore from './MyShareTransferStore'
import AliTransferShareList from '../../aliapi/transfersharelist'
import useShareHistoryStore from './ShareHistoryStore'
import useShareBottleFishStore from './ShareBottleFishStore'
import UserDAL from '../../user/userdal'
import type { ITokenInfo } from '../../user/userstore'
import { getDriveProviderCapabilities, getDriveProviderIcon, getDriveProviderLabel, type DriveProvider, type DriveProviderCapabilities } from '../../utils/driveProvider'
import type { IManagedShareItem } from './MyShareStore'
import DebugLog from '../../utils/debuglog'

export interface IShareAccountSummary {
  user_id: string
  name: string
  provider: DriveProvider
  providerLabel: string
  icon: string
  capabilities: DriveProviderCapabilities
}

export default class ShareDAL {

  static toShareAccount(token: ITokenInfo): IShareAccountSummary | undefined {
    const capabilities = getDriveProviderCapabilities({ tokenfrom: token.tokenfrom, userId: token.user_id, driveId: token.default_drive_id })
    if (!capabilities.manageCreatedShares && !capabilities.manageImportedShares && !capabilities.shareHistory) return undefined
    return {
      user_id: token.user_id,
      name: token.nick_name || token.user_name || token.name || token.user_id,
      provider: capabilities.provider,
      providerLabel: getDriveProviderLabel(capabilities.provider),
      icon: getDriveProviderIcon(capabilities.provider),
      capabilities
    }
  }

  static async getShareAccounts(): Promise<IShareAccountSummary[]> {
    const tokens = await UserDAL.GetUserListFromDB()
    return tokens.map((token) => ShareDAL.toShareAccount(token)).filter((account): account is IShareAccountSummary => !!account)
  }

  static async aReloadAllMyShare(force: boolean): Promise<IShareAccountSummary[]> {
    const accounts = await ShareDAL.getShareAccounts()
    const myshareStore = useMyShareStore()
    const accountIds = accounts.filter((account) => account.capabilities.manageCreatedShares).map((account) => account.user_id).sort()
    if (!force && accountIds.length == myshareStore.LoadedAccountIds.length && accountIds.every((id, index) => id == myshareStore.LoadedAccountIds[index])) return accounts
    if (myshareStore.ListLoading) return accounts
    myshareStore.ListLoading = true
    try {
      const successfulAccountIds: string[] = []
      const failedAccounts: string[] = []
      const lists = await Promise.all(
        accounts
          .filter((account) => account.capabilities.manageCreatedShares)
          .map(async (account) => {
            try {
              const resp = await AliShareList.ApiShareListAll(account.user_id)
              successfulAccountIds.push(account.user_id)
              return resp.items.map(
                (item): IManagedShareItem => ({
                  ...item,
                  account_id: account.user_id,
                  account_name: account.name,
                  account_provider: account.provider,
                  share_key: `${account.user_id}:${item.share_id}`
                })
              )
            } catch (error: any) {
              failedAccounts.push(account.name)
              DebugLog.mSaveWarning(`aReloadAllMyShare ${account.user_id}`, error)
              return []
            }
          })
      )
      myshareStore.aLoadListData(lists.flat(), successfulAccountIds)
      if (failedAccounts.length > 0) message.warning(`部分账号分享加载失败：${failedAccounts.join('、')}`)
    } finally {
      myshareStore.ListLoading = false
    }
    return accounts
  }

  static async aLoadFromDB(): Promise<void> {
    await ShareDAL.aReloadOtherShare()
  }


  static async aReloadMyShare(user_id: string, force: boolean): Promise<void> {
    if (!user_id) return
    await ShareDAL.aReloadAllMyShare(force)
  }

  static async aReloadShareHistory(user_id: string, force: boolean): Promise<void> {
    if (!user_id) return
    const shareHistoryStore = useShareHistoryStore()
    if (!force && shareHistoryStore.LoadedAccountId == user_id) return
    if (shareHistoryStore.ListLoading == true) return
    shareHistoryStore.ListLoading = true
    const resp = await AliShareList.ApiShareRecentListAll(user_id)
    shareHistoryStore.aLoadListData(resp.items, user_id)
    shareHistoryStore.ListLoading = false
  }

  static async aReloadShareBottleFish(user_id: string, force: boolean): Promise<void> {
    if (!user_id) return
    const shareBottleFishStore = useShareBottleFishStore()
    if (!force && shareBottleFishStore.ListDataRaw.length > 0) return
    if (shareBottleFishStore.ListLoading == true) return
    shareBottleFishStore.ListLoading = true
    const resp = await AliShareList.ApiShareBottleFishListAll(user_id)
    shareBottleFishStore.aLoadListData(resp.items)
    shareBottleFishStore.ListLoading = false
  }

  static async aReloadMyTransferShare(user_id: string, force: boolean): Promise<void> {
    if (!user_id) return
    const myTransferShareStore = useMyTransferShareStore()
    if (!force && myTransferShareStore.ListDataRaw.length > 0) return
    if (myTransferShareStore.ListLoading == true) return
    myTransferShareStore.ListLoading = true
    const resp = await AliTransferShareList.ApiTransferShareListAll(user_id)
    myTransferShareStore.aLoadListData(resp.items)
    myTransferShareStore.ListLoading = false
  }

  static async aReloadMyShareUntilShareID(user_id: string, share_id: string): Promise<void> {
    if (!user_id) return
    const find = await AliShareList.ApiShareListUntilShareID(user_id, share_id)
    if (find) await ShareDAL.aReloadMyShare(user_id, true)
  }

  static async aReloadMyTransferShareUntilShareID(user_id: string, share_id: string): Promise<void> {
    if (!user_id) return
    const find = await AliTransferShareList.ApiTransferShareListUntilShareID(user_id, share_id, 20)
    if (find) await ShareDAL.aReloadMyTransferShare(user_id, true)
  }

  static async aReloadOtherShare(): Promise<void> {
    const othershareStore = useOtherShareStore()
    if (othershareStore.ListLoading) return
    othershareStore.ListLoading = true

    const shareList = await DB.getOtherShareAll()
    const timeNow = new Date().getTime()
    for (let i = 0, maxi = shareList.length; i < maxi; i++) {
      const item = shareList[i]
      if (item.updated_at) {
        const updated_at = new Date(item.updated_at).getTime()
        item.updated_at = humanDateTime(updated_at)
      }
      if (!item.expired) {
        if (item.share_msg != '已失效') item.share_msg = humanExpiration(item.expiration, timeNow)
        item.expired = item.share_msg == '过期失效'
      }
    }
    othershareStore.aLoadListData(shareList)
    await Sleep(200)
    othershareStore.ListLoading = false
  }


  static async SaveOtherShare(password: string, info: IAliShareAnonymous, refresh: boolean) {
    let share = await DB.getOtherShare(info.shareinfo.share_id)
    if (!share) {
      share = {
        share_id: info.shareinfo.share_id,
        share_name: info.shareinfo.share_id,
        description: '',
        share_pwd: password,
        expiration: '0',
        expired: false,
        created_at: '',
        updated_at: new Date().toISOString(),
        saved_at: '',
        saved_time: Date.now(),
        share_msg: ''
      }
    }
    share.share_name = info.shareinfo.display_name || info.shareinfo.share_id
    share.created_at = info.shareinfo.created_at || new Date().toISOString()
    share.updated_at = info.shareinfo.updated_at || new Date().toISOString()
    share.saved_at = humanDateTime(share.saved_time)

    if (info.error != '') {
      share.share_msg = '已失效'
      share.expired = false
    } else {
      share.expiration = info.shareinfo.expiration
      share.share_msg = humanExpiration(share.expiration)
      share.expired = share.share_msg == '过期失效'
    }
    await DB.saveOtherShare(share)
    if (!refresh) return
    await ShareDAL.aReloadOtherShare()
  }


  static async SaveOtherShareText(text: string): Promise<boolean> {
    const idList = ParseShareIDList(text)

    if (idList.length == 0) {
      message.error('解析分享链接失败，格式错误')
      return false
    }

    const savefunc = (one: IID) => {
      return AliShare.ApiGetShareAnonymous(one.id, one.pwd).then((info) => {
        if (info.error == '429') return
        return ShareDAL.SaveOtherShare(one.pwd, info, false)
      })
    }

    await RunBatch('解析分享链接', idList, 3, savefunc)
    await ShareDAL.aReloadOtherShare()
    return true
  }


  static async SaveOtherShareRefresh(): Promise<boolean> {
    const shareList = await DB.getOtherShareAll()

    if (shareList.length == 0) {
      return false
    }
    const savefunc = (share: IOtherShareLinkModel) => {
      return AliShare.ApiGetShareAnonymous(share.share_id, share.share_pwd).then((info) => {
        if (info.error == '429') return
        if (info.error != '') {
          share.expired = false
          share.share_msg = '已失效'
        } else {
          share.share_name = info.shareinfo.display_name
          share.expiration = info.shareinfo.expiration
          share.updated_at = info.shareinfo.updated_at
          share.share_msg = humanExpiration(share.expiration)
          share.expired = share.share_msg == '过期失效'
        }
        return DB.saveOtherShare(share)
      })
    }
    await RunBatch('更新状态', shareList, 3, savefunc)
    await ShareDAL.aReloadOtherShare()
    return true
  }


  static async DeleteOtherShare(selectKeys: string[]): Promise<void> {
    if (selectKeys) await DB.deleteOtherShareBatch(selectKeys)
    useOtherShareStore().mDeleteFiles(selectKeys)
  }


}
