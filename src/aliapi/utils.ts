import { useSettingStore } from '../store'
import UserDAL from '../user/userdal'
import { IAliBatchResult } from './models'

import { SHA1, SHA256 } from 'crypto-js'
import { secp256k1 } from '@noble/curves/secp256k1.js'
import { IAliFileItem, IAliGetFileModel } from './alimodels'
import { decodeName, encodeName } from '../module/flow-enc/utils'
import path from 'path'
import mime from 'mime-types'
import { getEncPassword, getEncType } from '../utils/proxyhelper'

export function GetDriveID(user_id: string, drive: string): string {
  if (/^(webdav|s3|onedrive|dropbox|gdrive|gofile):/.test(drive || '')) return drive
  const token = UserDAL.GetUserToken(user_id)
  if (token) {
    if (isPikPakUser(user_id)) {
      return token.default_drive_id || 'pikpak'
    }
    if (isOneDriveUser(user_id) || isDropboxUser(user_id) || isGoogleDriveUser(user_id) || isGofileUser(user_id) || isWebDavUser(user_id) || isS3User(user_id)) {
      return token.default_drive_id || drive
    }
    if (drive.includes('backup')) {
      return token.backup_drive_id
    } else if (drive.includes('resource')) {
      return token.resource_drive_id
    } else if (drive.includes('pic')) {
      return token.pic_drive_id
    } else if (drive.includes('safe')) {
      return token.default_sbox_drive_id
    }
  }
  return ''
}

export function GetDriveType(user_id: string, drive_id: string): any {
  if ((drive_id || '').startsWith('webdav:')) return { title: 'WebDAV', name: 'webdav', key: '/' }
  if ((drive_id || '').startsWith('s3:')) {
    return { title: 'S3', name: 's3', key: '/' }
  }
  if ((drive_id || '').startsWith('onedrive:')) return { title: 'OneDrive', name: 'onedrive', key: 'onedrive_root' }
  if ((drive_id || '').startsWith('dropbox:')) return { title: 'Dropbox', name: 'dropbox', key: 'dropbox_root' }
  if ((drive_id || '').startsWith('gdrive:')) return { title: 'Google Drive', name: 'gdrive', key: 'gdrive_root' }
  if ((drive_id || '').startsWith('gofile:')) return { title: 'GoFile', name: 'gofile', key: 'gofile_root' }
  const token = UserDAL.GetUserToken(user_id)
  if (token) {
    if (isPikPakUser(user_id)) {
      return { title: '网盘文件', name: 'pikpak', key: 'pikpak_root' }
    }
    if (isOneDriveUser(user_id)) return { title: 'OneDrive', name: 'onedrive', key: 'onedrive_root' }
    if (isDropboxUser(user_id)) return { title: 'Dropbox', name: 'dropbox', key: 'dropbox_root' }
    if (isGoogleDriveUser(user_id)) return { title: 'Google Drive', name: 'gdrive', key: 'gdrive_root' }
    if (isGofileUser(user_id)) return { title: 'GoFile', name: 'gofile', key: 'gofile_root' }
    if (isWebDavUser(user_id)) return { title: 'WebDAV', name: 'webdav', key: '/' }
    if (isS3User(user_id)) return { title: 'S3', name: 's3', key: '/' }
    switch (drive_id) {
      case token.resource_drive_id:
        return { title: '网盘文件', name: 'resource', key: 'resource_root' }
      case token.backup_drive_id:
        return { title: token.resource_drive_id ? '备份空间' : '网盘文件', name: 'backup', key: 'backup_root' }
      case token.pic_drive_id:
        return { title: '相册', name: 'pic', key: 'pic_root' }
      case token.default_sbox_drive_id:
        return { title: '安全盘', name: 'safe', key: 'safe_root' }
      default:
        return { title: '网盘文件', name: 'backup', key: 'backup_root' }
    }
  }
  return { title: '网盘文件', name: 'backup', key: 'backup_root' }
}

function resolveUserTokenInfo(user: string | { user_id?: string; tokenfrom?: string }) {
  if (typeof user === 'string') {
    return {
      user_id: user,
      tokenfrom: UserDAL.GetUserToken(user)?.tokenfrom || ''
    }
  }
  return {
    user_id: user?.user_id || '',
    tokenfrom: user?.tokenfrom || ''
  }
}

/** Aliyun Drive is no longer a product login surface; keep helper for legacy guards only. */
export function isAliyunUser(_user?: string | { user_id?: string; tokenfrom?: string }): boolean {
  return false
}

export function isPikPakUser(user: string | { user_id?: string; tokenfrom?: string }): boolean {
  const { user_id, tokenfrom } = resolveUserTokenInfo(user)
  if (tokenfrom === 'pikpak') return true
  return user_id.startsWith('pikpak_')
}

export function isWebDavUser(user: string | { user_id?: string; tokenfrom?: string }): boolean {
  const { user_id, tokenfrom } = resolveUserTokenInfo(user)
  if (tokenfrom === 'webdav') return true
  return user_id.startsWith('webdav:')
}

export function isS3User(user: string | { user_id?: string; tokenfrom?: string }): boolean {
  const { user_id, tokenfrom } = resolveUserTokenInfo(user)
  if (tokenfrom === 's3') return true
  return user_id.startsWith('s3:')
}

export function isOneDriveUser(user: string | { user_id?: string; tokenfrom?: string }): boolean {
  const { user_id, tokenfrom } = resolveUserTokenInfo(user)
  return tokenfrom === 'onedrive' || user_id.startsWith('onedrive_')
}

export function isDropboxUser(user: string | { user_id?: string; tokenfrom?: string }): boolean {
  const { user_id, tokenfrom } = resolveUserTokenInfo(user)
  return tokenfrom === 'dropbox' || user_id.startsWith('dropbox_')
}

export function isGoogleDriveUser(user: string | { user_id?: string; tokenfrom?: string }): boolean {
  const { user_id, tokenfrom } = resolveUserTokenInfo(user)
  return tokenfrom === 'gdrive' || user_id.startsWith('gdrive_')
}

export function isGofileUser(user: string | { user_id?: string; tokenfrom?: string }): boolean {
  const { user_id, tokenfrom } = resolveUserTokenInfo(user)
  return tokenfrom === 'gofile' || user_id.startsWith('gofile_')
}

export function isNonAliyunProvider(user: string | { user_id?: string; tokenfrom?: string }): boolean {
  return (
    isPikPakUser(user) ||
    isOneDriveUser(user) ||
    isDropboxUser(user) ||
    isGoogleDriveUser(user) ||
    isGofileUser(user) ||
    isWebDavUser(user) ||
    isS3User(user)
  )
}

export function GetDeviceId(userId: string): string {
  const hash = SHA1(userId).toString()
  const variant = ((parseInt(hash[16], 16) & 0x3) | 0x8).toString(16)
  return `${hash.slice(0, 8)}-${hash.slice(8, 12)}-5${hash.slice(13, 16)}-${variant}${hash.slice(17, 20)}-${hash.slice(20, 32)}`
}

export function GetSignature(nonce: number, user_id: string, deviceId: string) {
  const toHex = (bytes: Uint8Array) => {
    const hashArray = Array.from(bytes) // convert buffer to byte array
    // convert bytes to hex string
    return hashArray.map((b) => b.toString(16).padStart(2, '0')).join('')
  }
  const toU8 = (wordArray: CryptoJS.lib.WordArray) => {
    const words = wordArray.words
    const sigBytes = wordArray.sigBytes
    // Convert
    const u8 = new Uint8Array(sigBytes)
    for (let i = 0; i < sigBytes; i++) {
      u8[i] = (words[i >>> 2] >>> (24 - (i % 4) * 8)) & 0xff
    }
    return u8
  }
  const privateKey = toU8(SHA256(user_id))
  const publicKey = '04' + toHex(secp256k1.getPublicKey(privateKey, true))
  const appId = ''
  const signature = toHex(secp256k1.sign(toU8(SHA256(`${appId}:${deviceId}:${user_id}:${nonce}`)), privateKey, { prehash: false, lowS: true })) + '01'
  return { signature, publicKey }
}

export function EncodeEncName(user_id: string, name: string, isDir: boolean, encType: string, inputpassword: string = '') {
  let settingStore = useSettingStore()
  if (encType && settingStore.securityEncFileName) {
    // 加密名称
    const splitFolder = name.split('/')
    const securityPassword = getEncPassword(user_id, encType, inputpassword)
    const securityEncType = settingStore.securityEncType
    if (!isDir) {
      return splitFolder
        .map((name) => {
          let plainName = ''
          let basename = path.basename(name)
          let extname = path.extname(name)
          if (mime.lookup(extname) && !settingStore.securityEncFileNameHideExt) {
            plainName = basename.replace(extname, '')
          } else {
            plainName = basename
            extname = ''
          }
          return encodeName(securityPassword, securityEncType, plainName) + extname
        })
        .join('/')
    } else {
      return splitFolder.map((name) => encodeName(securityPassword, securityEncType, name)).join('/')
    }
  } else {
    return name
  }
}

export function DecodeEncName(user_id: string, item: IAliFileItem | IAliGetFileModel, inputpassword: string = '') {
  // 自动解密文件名
  let settingStore = useSettingStore()
  const securityFileNameAutoDecrypt = settingStore.securityFileNameAutoDecrypt
  const securityEncType = settingStore.securityEncType
  let ext = ''
  let mine_type = item.mime_type
  if ('file_extension' in item) {
    ext = item.file_extension || ''
  } else if ('ext' in item) {
    ext = item.ext
  }
  let name = item.name
  let description = item.description
  let need_decode = description && description.includes('mnemoEncrypt')
  if (need_decode && securityFileNameAutoDecrypt) {
    let encType = getEncType(item)
    let filename = item.name.replace(ext ? '.' + ext : '', '')
    let password = getEncPassword(user_id, encType, inputpassword)
    let realName = decodeName(password, securityEncType, filename) || item.name
    if (ext) {
      name = realName + '.' + ext
    } else if (path.extname(realName)) {
      // 修复加密后的扩展
      name = realName
      ext = path.extname(realName).replace('.', '')
      mine_type = mime.lookup(ext) || 'application/oct-stream'
    } else {
      name = realName
    }
    return { name: name, mine_type, ext }
  }
  return { name: item.name, mine_type, ext }
}

export type AsyncType = '解压' | '复制' | '导入分享' | '回收站还原' | '异步' | '放回收站' | '彻底删除'

export async function ApiBatch(_title: string, _batchList: string[], _user_id: string, _share_token: string): Promise<IAliBatchResult> {
  return { count: 0, async_task: [], reslut: [], error: [] }
}

export function ApiBatchMaker(_url: string, _idList: string[], _bodymake: (file_id: string) => any): string[] {
  return []
}

export function ApiBatchMaker2(_url: string, _idList: string[], _namelist: string[], _bodymake: (file_id: string, name: string) => any): string[] {
  return []
}

export async function ApiBatchSuccess(_title: string, batchList: string[], _user_id: string, _share_token: string): Promise<string[]> {
  return []
}

export async function ApiWaitAsyncTask(_title: string, _user_id: string, _taskList: IAliBatchResult['async_task']): Promise<void> {
}

export async function ApiGetAsyncTask(_user_id: string, _async_task_id: string): Promise<'running' | 'error' | 'success'> {
  return 'error'
}

export async function ApiGetAsyncTaskUnzip(_user_id: string, _drive_id: string, _file_id: string, _domain_id: string, _task_id: string): Promise<string> {
  return 'error'
}
