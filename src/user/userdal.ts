import DB from '../utils/db'
import AliUser from '../aliapi/user'
import message from '../utils/message'
import useUserStore, { ITokenInfo } from './userstore'
import { useAppStore, useFootStore, useMyShareStore, usePanFileStore, usePanTreeStore, useSettingStore } from '../store'
import PanDAL from '../pan/pandal'
import DebugLog from '../utils/debuglog'
import { applyPikPakQuota, refreshPikPakAccessToken } from '../pikpak/auth'
import { refreshCloud189Token } from '../cloud189/auth'
import { fetchGuangyaUserInfo, refreshGuangyaAccessToken } from '../guangya/auth'
import { GetDriveType, isCloud139User, isCloud189User, isGuangyaUser, isNonAliyunProvider, isPikPakUser, isQuarkUser, isS3User, isWebDavUser } from '../aliapi/utils'
import { getWebDavConnection, getWebDavConnectionId } from '../utils/webdavClient'
import { getS3Connection, getS3ConnectionId } from '../utils/s3Client'
import { getDriveProviderSidebarEntries, resolveDriveProvider } from '../utils/driveProvider'
import { applyOneDriveQuota, refreshOneDriveAccessToken } from '../onedrive/auth'
import { applyDropboxQuota, refreshDropboxAccessToken } from '../dropbox/auth'
import { applyGoogleDriveQuota, refreshGoogleDriveAccessToken } from '../gdrive/auth'

export const UserTokenMap = new Map<string, ITokenInfo>()

export default class UserDAL {
  private static async ensureTokenReady(token: ITokenInfo): Promise<ITokenInfo | null> {
    try {
      const provider = resolveDriveProvider(token)
      if (provider === 'unknown') return null
      if (isWebDavUser(token)) {
        const connection = getWebDavConnection(getWebDavConnectionId(token.default_drive_id || token.user_id))
        return connection ? token : null
      }
      if (isS3User(token)) {
        const connection = getS3Connection(getS3ConnectionId(token.default_drive_id || token.user_id))
        return connection ? token : null
      }
      if (isPikPakUser(token)) {
        const expireTime = new Date(token.expire_time || 0).getTime()
        if (!token.access_token || (expireTime && expireTime <= Date.now())) {
          const refreshed = await refreshPikPakAccessToken(token)
          if (!refreshed?.access_token) return null
          this.SaveUserToken(refreshed)
          return refreshed
        }
        return token
      }
      if (isQuarkUser(token)) {
        token.default_drive_id = token.default_drive_id || 'quark'
        return token.user_id && token.access_token ? token : null
      }
      if (isCloud139User(token)) {
        token.default_drive_id = token.default_drive_id || 'cloud139'
        return token.user_id && token.access_token ? token : null
      }
      if (isCloud189User(token)) {
        token.default_drive_id = token.default_drive_id || 'cloud189'
        if (!token.open_api_access_token || !token.open_api_refresh_token) {
          const refreshed = await refreshCloud189Token(token)
          if (!refreshed?.user_id) return null
          this.SaveUserToken(refreshed)
          return refreshed
        }
        return token.user_id && token.refresh_token ? token : null
      }
      if (isGuangyaUser(token)) {
        token.default_drive_id = token.default_drive_id || 'guangya'
        const expireTime = new Date(token.expire_time || 0).getTime()
        if (!token.access_token || (expireTime && expireTime <= Date.now())) {
          const refreshed = await refreshGuangyaAccessToken(token)
          if (!refreshed?.access_token) return null
          this.SaveUserToken(refreshed)
          return refreshed
        }
        return token.user_id && token.access_token ? token : null
      }
      if (provider === 'onedrive' || provider === 'dropbox' || provider === 'gdrive') {
        const expireTime = new Date(token.expire_time || 0).getTime()
        if (!token.access_token || (expireTime && expireTime <= Date.now() + 60_000)) {
          const refreshed = provider === 'onedrive' ? await refreshOneDriveAccessToken(token) : provider === 'dropbox' ? await refreshDropboxAccessToken(token) : await refreshGoogleDriveAccessToken(token)
          if (!refreshed?.access_token) return null
          this.SaveUserToken(refreshed)
          return refreshed
        }
        return token.user_id ? token : null
      }
      if (provider === 'gofile') return token.user_id && token.access_token ? token : null
      if (isNonAliyunProvider(token)) return token.user_id ? token : null
      const ok = !!(token.user_id && (await AliUser.ApiTokenRefreshAccount(token, false)))
      return ok ? token : null
    } catch (err: any) {
      DebugLog.mSaveDanger('ensureTokenReady', err)
      return null
    }
  }

  static async aLoadFromDB() {
    const tokenList = await DB.getUserAll()
    const defaultUser = await DB.getValueString('uiDefaultUser')
    UserTokenMap.clear()
    // 先把所有账号塞进 UserTokenMap，保证 UserInfo 历史账号列表 + 切换可用，
    // 不受“默认账号加载失败”影响
    for (const token of tokenList) {
      if (token?.user_id) UserTokenMap.set(token.user_id, token)
    }

    // 排序：默认账号优先；其次按 used_size（getUserAll 已倒序，即“最常用”）
    const orderedTokens: ITokenInfo[] = []
    if (defaultUser) {
      const idx = tokenList.findIndex((t) => t.user_id === defaultUser)
      if (idx >= 0) {
        orderedTokens.push(tokenList[idx])
        for (let i = 0; i < tokenList.length; i++) {
          if (i !== idx) orderedTokens.push(tokenList[i])
        }
      } else {
        orderedTokens.push(...tokenList)
      }
    } else {
      orderedTokens.push(...tokenList)
    }

    let hasLogin = false
    let defaultUserLoaded = false
    for (const token of orderedTokens) {
      if (hasLogin) {
        // 已经选出了一个账号成功登录；其余账号仅尝试静默刷新 + 签到，
        // 不影响 UserInfo 列表显示
        try {
          const prepared = await this.ensureTokenReady(token)
          if (prepared?.user_id) {
            await this.UserAutoSign(prepared)
          }
        } catch (err: any) {
          DebugLog.mSaveDanger('aLoadFromDB autoSign ' + (token?.user_id || ''), err)
        }
        continue
      }
      // 还未选出可登录账号：依次尝试。任意一步失败都 fallback 到下一个
      try {
        const prepared = await this.ensureTokenReady(token)
        if (!prepared?.user_id) continue
        await this.UserLogin(prepared)
        hasLogin = true
        if (defaultUser && prepared.user_id === defaultUser) defaultUserLoaded = true
      } catch (err: any) {
        DebugLog.mSaveDanger('aLoadFromDB userLogin ' + (token?.user_id || ''), err)
        // 失败的账号 token 已在 UserTokenMap 中（顶部统一塞过），
        // UserInfo.vue 仍可列出并支持手动切换
      }
    }
    if (defaultUser && !defaultUserLoaded) {
      console.log('aLoadFromDB defaultUser failed, fallback used. defaultUser=', defaultUser)
    }
    if (!hasLogin) {
      useUserStore().userShowLogin = true
    }
  }

  static async aRefreshAllUserToken() {
    const tokenList = await DB.getUserAll()
    const dateNow = new Date().getTime()
    for (let i = 0, maxi = tokenList.length; i < maxi; i++) {
      const token = tokenList[i]
      try {
        if (resolveDriveProvider(token) === 'unknown') continue
        if (isPikPakUser(token)) {
          const expireTime = new Date(token.expire_time || 0).getTime()
          if (expireTime && expireTime - dateNow <= 1000 * 60 * 5) {
            const refreshed = await refreshPikPakAccessToken(token)
            if (refreshed) {
              UserTokenMap.set(refreshed.user_id, refreshed)
              await DB.saveUser(refreshed)
            }
          }
          continue
        }
        if (isCloud139User(token)) {
          continue
        }
        if (isCloud189User(token)) {
          if (!token.open_api_access_token || !token.open_api_refresh_token) {
            const refreshed = await refreshCloud189Token(token)
            if (refreshed) {
              UserTokenMap.set(refreshed.user_id, refreshed)
              await DB.saveUser(refreshed)
            }
          }
          continue
        }
        if (isGuangyaUser(token)) {
          const expireTime = new Date(token.expire_time || 0).getTime()
          if (expireTime && expireTime - dateNow <= 1000 * 60 * 5) {
            const refreshed = await refreshGuangyaAccessToken(token)
            if (refreshed) {
              UserTokenMap.set(refreshed.user_id, refreshed)
              await DB.saveUser(refreshed)
            }
          }
          continue
        }
        const provider = resolveDriveProvider(token)
        if (provider === 'onedrive' || provider === 'dropbox' || provider === 'gdrive') {
          const expireTime = new Date(token.expire_time || 0).getTime()
          if (!token.access_token || (expireTime && expireTime - dateNow <= 1000 * 60 * 5)) {
            const refreshed = provider === 'onedrive' ? await refreshOneDriveAccessToken(token) : provider === 'dropbox' ? await refreshDropboxAccessToken(token) : await refreshGoogleDriveAccessToken(token)
            if (refreshed) {
              UserTokenMap.set(refreshed.user_id, refreshed)
              await DB.saveUser(refreshed)
            }
          }
          continue
        }
        if (isNonAliyunProvider(token)) {
          continue
        }
        const expire_time = new Date(token.expire_time).getTime()
        const session_expire_time = new Date(token.session_expires_in).getTime()
        // 自动刷新Token(过期前5分钟)
        if (expire_time - dateNow <= 1000 * 60 * 5) {
          await AliUser.ApiTokenRefreshAccount(token, false, true)
          await AliUser.OpenApiTokenRefreshAccount(token, false, true)
        }
        if (session_expire_time - dateNow <= 1000 * 60) {
          await AliUser.ApiSessionRefreshAccount(token, false, true)
        }
      } catch (err: any) {
        DebugLog.mSaveDanger('aRefreshAllUserToken', err)
      }
    }
  }

  static GetUserToken(user_id: string): ITokenInfo {
    if (user_id && UserTokenMap.has(user_id)) return UserTokenMap.get(user_id)!

    return {
      tokenfrom: 'unknown',
      access_token: '',
      refresh_token: '',

      session_expires_in: 0,
      open_api_token_type: '',
      open_api_access_token: '',
      open_api_refresh_token: '',
      open_api_expires_in: 0,

      expires_in: 0,
      token_type: '',
      user_id: '',
      user_name: '',
      avatar: '',
      nick_name: '',
      default_drive_id: '',
      default_sbox_drive_id: '',
      resource_drive_id: '',
      backup_drive_id: '',
      sbox_drive_id: '',
      role: '',
      status: '',
      expire_time: '',
      state: '',
      pin_setup: false,
      is_first_login: false,
      need_rp_verify: false,
      name: '',
      spu_id: '',
      is_expires: false,
      used_size: 0,
      total_size: 0,
      free_size: 0,
      space_expire: false,
      spaceinfo: '',
      vipname: '',
      vipexpire: '',
      vipIcon: '',
      pic_drive_id: '',
      device_id: '',
      signature: '',
      signInfo: {
        signMon: -1,
        signDay: -1
      }
    }
  }

  static async GetUserTokenFromDB(user_id: string) {
    if (!user_id) return undefined
    if (UserTokenMap.has(user_id)) return UserTokenMap.get(user_id)
    const user = await DB.getUser(user_id)
    if (user) UserTokenMap.set(user.user_id, user)
    return user
  }

  static async ClearUserTokenMap() {
    UserTokenMap.clear()
  }

  static GetUserList() {
    const list: ITokenInfo[] = []
    // eslint-disable-next-line no-unused-vars
    for (const [_, token] of UserTokenMap) {
      list.push(token)
    }
    return list.sort((a, b) => (a.name || a.nick_name || a.user_name || a.user_id).localeCompare(b.name || b.nick_name || b.user_name || b.user_id))
  }

  static async GetUserListFromDB(): Promise<ITokenInfo[]> {
    const list = await DB.getUserAll()
    for (const token of list) {
      if (token.user_id && !UserTokenMap.has(token.user_id)) {
        UserTokenMap.set(token.user_id, token)
      }
    }
    return list.sort((a, b) => (a.name || a.nick_name || a.user_name || a.user_id).localeCompare(b.name || b.nick_name || b.user_name || b.user_id))
  }

  static async SaveUserToken(token: ITokenInfo, previousUserId: string = ''): Promise<boolean> {
    if (!token.user_id) return false
    try {
      const staleUserId = previousUserId && previousUserId !== token.user_id ? previousUserId : ''
      if (staleUserId) {
        UserTokenMap.delete(staleUserId)
        await DB.deleteUser(staleUserId)
      }
      UserTokenMap.set(token.user_id, token)
      await DB.saveUser(token)
      window.WinMsgToUpload?.({ cmd: 'ClearUserToken' })
      return true
    } catch (err: any) {
      DebugLog.mSaveDanger('SaveUserToken ' + token.user_id, err)
      return false
    }
  }

  static async UserLogin(token: ITokenInfo, isInteractive: boolean = false) {
    const loadingKey = 'userlogin_' + Date.now().toString()
    message.loading('加载用户信息中...', 0, loadingKey)
    const previousActiveUserId = useUserStore().user_id
    const previousDefaultUserId = await DB.getValueString('uiDefaultUser')
    const initialUserId = token.user_id
    if (initialUserId) UserTokenMap.set(initialUserId, token)
    try {
      if (isPikPakUser(token)) {
        await applyPikPakQuota(token)
      } else if (isQuarkUser(token)) {
        token.default_drive_id = token.default_drive_id || 'quark'
      } else if (isCloud139User(token)) {
        token.default_drive_id = token.default_drive_id || 'cloud139'
      } else if (isCloud189User(token)) {
        token.default_drive_id = token.default_drive_id || 'cloud189'
        if (!token.open_api_access_token || !token.open_api_refresh_token) {
          const refreshed = await refreshCloud189Token(token)
          if (refreshed) Object.assign(token, refreshed)
        }
      } else if (isGuangyaUser(token)) {
        token.default_drive_id = token.default_drive_id || 'guangya'
        const userInfo = await fetchGuangyaUserInfo(token).catch(() => null)
        const info = userInfo?.data || userInfo || null
        if (info) {
          token.user_name = info.username || info.phone_number || info.phoneNumber || token.user_name
          token.nick_name = info.nickname || info.nick_name || info.name || token.nick_name
          token.name = info.name || token.nick_name || token.name
          token.avatar = info.avatar || token.avatar
          token.used_size = Number(info.used_size || info.usedSize || token.used_size || 0)
          token.total_size = Number(info.total_size || info.totalSize || token.total_size || 0)
        }
      } else if (isWebDavUser(token)) {
        token.default_drive_id = token.default_drive_id || token.user_id
      } else if (isS3User(token)) {
        token.default_drive_id = token.default_drive_id || token.user_id
      } else if (resolveDriveProvider(token) === 'onedrive') {
        await applyOneDriveQuota(token)
      } else if (resolveDriveProvider(token) === 'dropbox') {
        await applyDropboxQuota(token)
      } else if (resolveDriveProvider(token) === 'gdrive') {
        await applyGoogleDriveQuota(token)
      } else if (!isNonAliyunProvider(token)) {
        await Promise.all([
          AliUser.ApiUserInfo(token),
          AliUser.ApiUserDriveInfo(token),
          AliUser.ApiUserPic(token),
          AliUser.ApiUserVip(token),
          AliUser.ApiSessionRefreshAccount(token, false),
          AliUser.OpenApiTokenRefreshAccount(token, false),
          UserDAL.UserAutoSign(token)
        ])
      }

      if (!token.user_id) throw new Error('账号信息缺少用户标识')
      if (initialUserId && token.user_id !== initialUserId) {
        UserTokenMap.delete(initialUserId)
        await DB.deleteUser(initialUserId)
      }
      if (!(await UserDAL.SaveUserToken(token))) throw new Error('账号信息保存失败')
      window.WebUserToken({
        user_id: token.user_id,
        name: token.user_name,
        access_token: token.access_token,
        open_api_access_token: token.open_api_access_token,
        tokenfrom: token.tokenfrom,
        login: true
      })
      await UserDAL.LoadPanData(token)

      await DB.saveValueString('uiDefaultUser', token.user_id)
      useUserStore().userLogin(token.user_id)
      PanDAL.aReLoadQuickFile(token.user_id)
      useAppStore().resetTab(useSettingStore().uiDefaultTab || 'pan')
      useMyShareStore().$reset()
      useFootStore().mSaveUserInfo(token)
      message.success('加载用户成功!', 2, loadingKey)
    } catch (err: any) {
      useFootStore().mSaveLoading('')
      await DB.saveValueString('uiDefaultUser', previousDefaultUserId)
      if (previousActiveUserId) useUserStore().userLogin(previousActiveUserId)
      else useUserStore().userLogOff()
      const previousToken = previousActiveUserId ? UserTokenMap.get(previousActiveUserId) : undefined
      if (previousToken) {
        window.WebUserToken({
          user_id: previousToken.user_id,
          name: previousToken.user_name,
          access_token: previousToken.access_token,
          open_api_access_token: previousToken.open_api_access_token,
          tokenfrom: previousToken.tokenfrom,
          login: true
        })
      }
      message.error('加载账号失败：' + (err?.message || '网盘数据解析失败'), 5, loadingKey)
      throw err
    }
  }

  static async LoadPanData(token: ITokenInfo) {
    console.warn('LoadPanData....')
    if (isPikPakUser(token)) {
      await PanDAL.aReLoadPikPakDrive(token)
      await PanDAL.aReLoadOneDirToShow(token.default_drive_id || 'pikpak', 'pikpak_root', true)
      return
    }
    if (isQuarkUser(token)) {
      await PanDAL.aReLoadQuarkDrive(token)
      await PanDAL.aReLoadOneDirToShow(token.default_drive_id || 'quark', 'quark_root', true)
      return
    }
    if (isCloud139User(token)) {
      await PanDAL.aReLoadCloud139Drive(token)
      await PanDAL.aReLoadOneDirToShow(token.default_drive_id || 'cloud139', 'cloud139_root', true)
      return
    }
    if (isCloud189User(token)) {
      await PanDAL.aReLoadCloud189Drive(token)
      await PanDAL.aReLoadOneDirToShow(token.default_drive_id || 'cloud189', 'cloud189_root', true)
      return
    }
    if (isGuangyaUser(token)) {
      await PanDAL.aReLoadGuangyaDrive(token)
      await PanDAL.aReLoadOneDirToShow(token.default_drive_id || 'guangya', 'guangya_root', true)
      return
    }
    if (isWebDavUser(token)) {
      await PanDAL.aReLoadWebDavDrive(token)
      await PanDAL.aReLoadOneDirToShow(token.default_drive_id || token.user_id, '/', true)
      return
    }
    if (isS3User(token)) {
      await PanDAL.aReLoadS3Drive(token)
      await PanDAL.aReLoadOneDirToShow(token.default_drive_id || token.user_id, '/', true)
      return
    }
    const provider = resolveDriveProvider(token)
    if (provider === 'onedrive' || provider === 'dropbox' || provider === 'gdrive' || provider === 'gofile') {
      await PanDAL.aReLoadProviderDrive(token)
      const rootKey = provider === 'onedrive' ? 'onedrive_root' : provider === 'dropbox' ? 'dropbox_root' : provider === 'gdrive' ? 'gdrive_root' : 'gofile_root'
      await PanDAL.aReLoadOneDirToShow(token.default_drive_id, rootKey, true)
      return
    }
    const settingStore = useSettingStore()
    const panTreeStore = usePanTreeStore()
    panTreeStore.mSaveUser(token.user_id, token.default_drive_id, token.resource_drive_id, token.backup_drive_id, token.pic_drive_id)

    const spaces = getDriveProviderSidebarEntries('aliyun', token, {
      hideResourceDrive: settingStore.securityHideResourceDrive,
      hideBackupDrive: settingStore.securityHideBackupDrive,
      hideAlbum: settingStore.securityHidePicDrive
    }).filter((entry) => entry.kind === 'space')
    const resourceSpace = spaces.find((entry) => entry.key === 'resource_root')
    const backupSpace = spaces.find((entry) => entry.key === 'backup_root')
    const preferredSpace = settingStore.uiShowPanRootFirst === 'backup' ? backupSpace || resourceSpace : resourceSpace || backupSpace
    const initialSpace = preferredSpace || spaces[0]
    const fallbackDriveId = initialSpace?.driveId || token.default_drive_id || token.resource_drive_id || token.backup_drive_id || token.default_sbox_drive_id || token.sbox_drive_id
    panTreeStore.drive_id = fallbackDriveId

    if (resourceSpace) {
      await PanDAL.aReLoadResourceDrive(token)
    }
    if (backupSpace && backupSpace.driveId !== resourceSpace?.driveId) {
      await PanDAL.aReLoadBackupDrive(token)
    }
    if (!fallbackDriveId) return
    const rootKey = initialSpace?.key || GetDriveType(token.user_id, fallbackDriveId).key
    await PanDAL.aReLoadOneDirToShow(fallbackDriveId, rootKey, true)
  }

  static async UserLogOff(user_id: string): Promise<boolean> {
    await DB.deleteUser(user_id)
    UserTokenMap.delete(user_id)

    let newUserID = ''
    for (const token of [...UserTokenMap.values()]) {
      try {
        const prepared = await this.ensureTokenReady(token)
        if (prepared?.user_id) {
          await this.UserLogin(prepared)
          newUserID = prepared.user_id
          break
        }
      } catch (err: any) {
        DebugLog.mSaveDanger('UserLogOff fallback ' + (token.user_id || ''), err)
      }
    }
    if (!newUserID) {
      useUserStore().userLogOff()
      usePanTreeStore().$reset()
      usePanFileStore().$reset()
      useUserStore().userShowLogin = true
    }
    return newUserID != ''
  }

  static async UserClearFromDB(user_id: string): Promise<void> {
    await DB.deleteUser(user_id)
    UserTokenMap.delete(user_id)
  }

  static async UserChange(user_id: string): Promise<boolean> {
    if (!UserTokenMap.has(user_id)) return false
    const token = UserTokenMap.get(user_id)!
    const prepared = await this.ensureTokenReady(token)
    if (!prepared?.user_id) {
      message.warning('该账号需要重新登陆[' + token.name + ']')
      return false
    }
    try {
      await this.UserLogin(prepared)
      return true
    } catch (err: any) {
      DebugLog.mSaveDanger('UserChange ' + user_id, err)
      message.warning('切换账号失败[' + (token.nick_name || token.user_name || token.name) + ']')
      return false
    }
  }

  static async UserRefreshByUserFace(user_id: string, force: boolean): Promise<boolean> {
    const token = UserDAL.GetUserToken(user_id)
    if (!token || (!token.access_token && !isWebDavUser(token) && !isS3User(token))) {
      return false
    }
    const provider = resolveDriveProvider(token)
    if (provider === 'onedrive' || provider === 'dropbox' || provider === 'gdrive') {
      const expireTime = new Date(token.expire_time || 0).getTime()
      const shouldRefreshToken = force || !token.access_token || (expireTime && expireTime <= Date.now() + 5 * 60_000)
      const refreshed = shouldRefreshToken
        ? provider === 'onedrive'
          ? await refreshOneDriveAccessToken(token)
          : provider === 'dropbox'
            ? await refreshDropboxAccessToken(token)
            : await refreshGoogleDriveAccessToken(token)
        : token
      if (!refreshed?.access_token) return false
      if (!shouldRefreshToken) {
        if (provider === 'onedrive') await applyOneDriveQuota(refreshed)
        else if (provider === 'dropbox') await applyDropboxQuota(refreshed)
        else await applyGoogleDriveQuota(refreshed)
      }
      useUserStore().userLogin(refreshed.user_id)
      UserDAL.SaveUserToken(refreshed)
      return true
    }
    if (provider === 'gofile') {
      UserDAL.SaveUserToken(token)
      return true
    }
    let expires_in = new Date(token.expire_time).getTime() - token.expires_in * 1000
    let time = Date.now() - expires_in
    if (!force || time / 1000 < 600) {
      if (isPikPakUser(token)) {
        await applyPikPakQuota(token)
        UserDAL.SaveUserToken(token)
        return true
      } else if (isQuarkUser(token)) {
        token.default_drive_id = token.default_drive_id || 'quark'
        UserDAL.SaveUserToken(token)
        return true
      } else if (isNonAliyunProvider(token)) {
        // 已知非阿里云盘 provider 但未在上面 if 链命中,跳过 aliyun 兜底
        UserDAL.SaveUserToken(token)
        return true
      } else {
        // 仅刷新个人信息
        await Promise.all([AliUser.ApiUserInfo(token), AliUser.ApiUserPic(token), AliUser.ApiUserVip(token)])
        UserDAL.SaveUserToken(token)
        return true
      }
    } else {
      // 刷新token和session
      if (token.user_id) {
        if (isPikPakUser(token)) {
          const refreshed = await refreshPikPakAccessToken(token)
          if (!refreshed?.access_token) return false
          UserDAL.SaveUserToken(refreshed)
        } else if (isQuarkUser(token)) {
          token.default_drive_id = token.default_drive_id || 'quark'
          UserDAL.SaveUserToken(token)
        } else if (isNonAliyunProvider(token)) {
          // 已知非阿里云盘 provider 但未在上面 if 链命中,跳过 aliyun 兜底
          UserDAL.SaveUserToken(token)
        } else {
          const isToken = await AliUser.ApiTokenRefreshAccount(token, true)
          if (!isToken) return false
          await AliUser.ApiSessionRefreshAccount(token, true)
          await AliUser.OpenApiTokenRefreshAccount(token, true)
        }
      } else {
        return false
      }
      if (isPikPakUser(token)) {
        await applyPikPakQuota(token)
      } else if (isQuarkUser(token)) {
        token.default_drive_id = token.default_drive_id || 'quark'
      } else if (!isNonAliyunProvider(token)) {
        // 刷新用户信息
        await Promise.all([AliUser.ApiUserInfo(token), AliUser.ApiUserPic(token), AliUser.ApiUserVip(token)])
      }
      useUserStore().userLogin(token.user_id)
      UserDAL.SaveUserToken(token)
      return true
    }
  }

  static async UserAutoSign(token: ITokenInfo) {
    // 自动签到
    if (isNonAliyunProvider(token)) {
      UserDAL.SaveUserToken(token)
      return
    }
    if (token.user_id && useSettingStore().uiLaunchAutoSign) {
      const nowMonth = new Date().getMonth() + 1
      const nowDay = new Date().getDate()
      if (!token.signInfo) token.signInfo = { signMon: -1, signDay: -1 }
      const signInfo = token.signInfo
      if (signInfo.signMon !== nowMonth || signInfo.signDay !== nowDay) {
        const signDay = await AliUser.ApiUserSign(token)
        if (signDay) {
          signInfo.signMon = nowMonth
          signInfo.signDay = signDay
        }
      }
    }
    UserDAL.SaveUserToken(token)
  }
}
