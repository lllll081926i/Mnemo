import Aria2 from 'aria2-lib'
import axios from 'axios'
import DownDAL, { IAriaDownProgress, IStateDownFile } from '../down/DownDAL'
import message from './message'
import UserDAL from '../user/userdal'
import { useFootStore, useSettingStore } from '../store'
import DebugLog from './debuglog'
import Config from '../config'
import { listProviderDirChildren } from './providerDirList'

import path from 'path'
import fs from 'fs'
import { getRawUrl } from './proxyhelper'
import { callAriaClient, getAriaAddUriGid, isAriaDuplicateGidError } from './aria2Rpc'
import { buildAriaAddOptions } from '../down/integration/aria2AddOptions'
import { isDriveProviderSessionUsable } from './driveProvider'

export const localPwd = 'S4znWTaZYQi3cpRNb'

let Aria2cChangeing: boolean = false
let Aria2EngineLocal: Aria2 | undefined = undefined
let Aria2EngineRemote: Aria2 | undefined = undefined

let IsAria2cOnlineLocal: boolean = false

let Aria2cLocalRelaunchTime = 0

let IsAria2cOnlineRemote: boolean = false

let Aria2cRemoteRetryTime = 0

function GetAria() {
  if (useSettingStore().AriaIsLocal) return Aria2EngineLocal
  return Aria2EngineRemote
}

function SetAriaOnline(isOnline: boolean, ariaState: string = '') {
  if (!ariaState) ariaState = useSettingStore().ariaState
  if (ariaState == 'local') {
    IsAria2cOnlineLocal = isOnline
    let ariaInfo = isOnline ? '本地下载服务已连接' : '本地下载服务已断开'
    useFootStore().mSaveAriaInfo(ariaInfo)
  } else {
    IsAria2cOnlineRemote = isOnline
    let ariaInfo = isOnline ? '远程下载服务已连接' : '远程下载服务已断开'
    useFootStore().mSaveAriaInfo(ariaInfo)
  }
}

function CloseRemote() {
  if (IsAria2cOnlineRemote) {
    IsAria2cOnlineRemote = false
    if (Aria2EngineRemote) {
      try {
        Aria2EngineRemote.call('aria2.forceShutdown').catch(() => {})
      } catch {}
      try {
        Aria2EngineRemote.close()
      } catch {}
      Aria2EngineRemote = undefined
    }
  }
}

export function IsAria2cRemote() {
  return IsAria2cOnlineRemote
}

export async function AriaTest(https: boolean, host: string, port: number, secret: string) {
  const url = (https ? 'https://' : 'http://') + host + ':' + port.toString() + '/jsonrpc'
  return axios
    .post(
      url,
      { method: 'aria2.getGlobalStat', jsonrpc: '2.0', id: 'id' + Date.now(), params: ['token:' + secret] },
      {
        responseType: 'json',
        headers: {
          'Content-Type': 'application/json'
        },
        timeout: 4000
      }
    )
    .then(() => {
      return true
    })
    .catch(function(error) {
      if (error.response && error.response.data && error.response.data.error) {
        if (error.response.data.error.message == 'Unauthorized') {
          message.error('无法连接下载服务：密码不正确')
          return false
        }
      }
      if (error.message && error.message.indexOf('timeout of') >= 0) {
        message.error('无法连接下载服务：连接超时，请检查服务器地址和网络')
        return false
      }
      message.error('无法连接下载服务，请检查服务器地址、端口和网络')
      return false
    })
}

function Sleep(msTime: number) {
  return new Promise((resolve) =>
    setTimeout(
      () =>
        resolve({
          success: true,
          time: msTime
        }),
      msTime
    )
  )
}


export async function AriaChangeToRemote() {
  if (Aria2cChangeing) return undefined
  Aria2cChangeing = true
  CloseRemote()
  try {
    const settingStore = useSettingStore()
    const host = settingStore.ariaUrl.split(':')[0]
    const port = parseInt(settingStore.ariaUrl.split(':')[1])
    const secret = settingStore.ariaPwd

    const options = { host, port, secure: settingStore.ariaHttps, secret, path: '/jsonrpc' }
    Aria2EngineRemote = new Aria2({ WebSocket: global.WebSocket, fetch: (...args: any[]) => (window.fetch as any)(...args), ...options })

    Aria2EngineRemote.on('close', () => {
      if (IsAria2cOnlineRemote && !Aria2cChangeing) {
        Aria2cRemoteRetryTime = 0 // 重置远程重试计数
        if (!settingStore.AriaIsLocal) {
          message.error('远程下载服务连接已断开，正在尝试重新连接')
          SetAriaOnline(false, 'remote')
          // 延迟 3 秒后自动重连
          setTimeout(() => {
            if (!IsAria2cOnlineRemote && !useSettingStore().AriaIsLocal) {
              AriaChangeToRemote()
            }
          }, 3000)
        }
      }
    })
    const _remoteNotifyRefresh = () => { AriaGetDowningList().catch(() => {}) }
    Aria2EngineRemote.on('onDownloadStart', _remoteNotifyRefresh)
    Aria2EngineRemote.on('onDownloadComplete', _remoteNotifyRefresh)
    Aria2EngineRemote.on('onDownloadError', _remoteNotifyRefresh)
    Aria2EngineRemote.on('onDownloadStop', _remoteNotifyRefresh)
    Aria2EngineRemote.on('onBtDownloadComplete', _remoteNotifyRefresh)
    await Sleep(500)
    await Aria2EngineRemote.open()
      .then(() => {
        Aria2cRemoteRetryTime = 0
        SetAriaOnline(true, 'remote')
      })
      .catch(() => {
        Aria2cRemoteRetryTime++
        SetAriaOnline(false, 'remote')
      })

    if (!IsAria2cOnlineRemote) {
      if (!settingStore.AriaIsLocal && Aria2cRemoteRetryTime % 10 == 1) message.error('无法连接远程下载服务，请检查服务器地址、端口和密码')
    } else {
      await AriaGlobalSpeed(); await AriaApplyAdvancedOptions()
    }
  } catch (e) {
    SetAriaOnline(false, 'remote')
  }
  Aria2cChangeing = false
  return IsAria2cOnlineRemote
}


export async function AriaChangeToLocal() {
  CloseRemote()
  if (Aria2cLocalRelaunchTime < 5) {
    try {
      let port = 16800
      if (Aria2EngineLocal == undefined) {
        port = window.WebRelaunchAria ? await window.WebRelaunchAria() : 16800
        const options = { host: '127.0.0.1', port, secure: false, secret: localPwd, path: '/jsonrpc' }
        Aria2EngineLocal = new Aria2({ WebSocket: global.WebSocket, fetch: (...args: any[]) => (window.fetch as any)(...args), ...options })
        Aria2EngineLocal.on('close', () => {
          IsAria2cOnlineLocal = false
          if (useSettingStore().AriaIsLocal) {
            Aria2cLocalRelaunchTime = 0 // 重置重试计数，允许重新连接
            if (Aria2cLocalRelaunchTime < 2) {
              message.error('本地下载服务连接已断开，正在尝试重新启动')
            }
            SetAriaOnline(false, 'local')
            // 延迟 2 秒后自动重连，避免频繁重试
            setTimeout(() => {
              if (!IsAria2cOnlineLocal && useSettingStore().AriaIsLocal) {
                AriaChangeToLocal()
              }
            }, 2000)
          }
        })
        const _notifyRefresh = () => { AriaGetDowningList().catch(() => {}) }
        Aria2EngineLocal.on('onDownloadStart', _notifyRefresh)
        Aria2EngineLocal.on('onDownloadComplete', _notifyRefresh)
        Aria2EngineLocal.on('onDownloadError', _notifyRefresh)
        Aria2EngineLocal.on('onDownloadStop', _notifyRefresh)
        Aria2EngineLocal.on('onBtDownloadComplete', _notifyRefresh)
        Aria2EngineLocal.setMaxListeners(0)
      }
      await Sleep(800)
      await Aria2EngineLocal.open()
        .then(() => {
          Aria2cLocalRelaunchTime = 0
          SetAriaOnline(true, 'local')
        })
        .catch(() => {
          SetAriaOnline(false, 'local')
          Aria2cLocalRelaunchTime++
          if (Aria2cLocalRelaunchTime < 2) {
            message.info('正在重新启动本地下载服务')
          }
        })
      if (!IsAria2cOnlineLocal) {
        if (Aria2cLocalRelaunchTime < 2) message.error('无法启动本地下载服务，请稍后重试')
      } else {
        await AriaGlobalSpeed(); await AriaApplyAdvancedOptions()
      }
    } catch (e) {
      SetAriaOnline(false, 'local')
    }
  } else {
    Aria2EngineLocal = undefined
  }
  return true
}


export async function AriaGlobalSpeed() {
  try {
    const settingStore = useSettingStore()
    const limit = settingStore.downGlobalSpeed.toString() + (settingStore.downGlobalSpeedM == 'MB' ? 'M' : 'K')
    await GetAria()?.call('aria2.changeGlobalOption', { 'max-overall-download-limit': limit }).catch((e: any) => {
      if (e && e.message == 'Unauthorized') message.error('下载服务密码不正确，请检查远程下载设置')
      IsAria2cOnlineLocal = false
    })
  } catch {
    SetAriaOnline(false)
  }
}

export async function AriaApplyAdvancedOptions(): Promise<boolean> {
  const client = GetAria()
  if (!client) return false
  try {
    const settingStore = useSettingStore()
    const options: Record<string, string> = {
      'max-connection-per-server': String(settingStore.ariaMaxConnectionPerServer || 16),
      'continue': 'true'
    }
    if (settingStore.ariaUserAgent) {
      options['user-agent'] = settingStore.ariaUserAgent
    }
    await client.call('aria2.changeGlobalOption', options)
    return true
  } catch {
    return false
  }
}

export async function AriaConnect() {
  const settingStore = useSettingStore()
  if (settingStore.AriaIsLocal) {
    if (!IsAria2cOnlineLocal || !Aria2EngineLocal) await AriaChangeToLocal()
    return IsAria2cOnlineLocal
  } else {
    if (!IsAria2cOnlineRemote || !Aria2EngineRemote) await AriaChangeToRemote()
    return IsAria2cOnlineRemote
  }
}


export async function AriaGetDowningList() {
  const multicall = [
    ['aria2.tellActive', ['gid', 'status', 'totalLength', 'completedLength', 'downloadSpeed', 'errorCode', 'errorMessage']],
    ['aria2.tellWaiting', 0, 1000, ['gid', 'status', 'totalLength', 'completedLength', 'downloadSpeed', 'errorCode', 'errorMessage']],
    ['aria2.tellStopped', 0, 1000, ['gid', 'status', 'totalLength', 'completedLength', 'downloadSpeed', 'errorCode', 'errorMessage']]
  ]
  try {
    const result: any = await GetAria()?.multicall(multicall)
    if (result) {
      let list: IAriaDownProgress[] = []
      let arr = result[0][0]
      list = list.concat(arr)
      arr = result[1][0]
      list = list.concat(arr)
      arr = result[2][0]
      list = list.concat(arr)
      DownDAL.mSpeedEvent(list || [])
      SetAriaOnline(true)
    }
  } catch (e: any) {
    DebugLog.mSaveLog('danger', 'AriaGetDowningList' + (e.message || ''), e)
    SetAriaOnline(false)
  }
}


export async function AriaDeleteList(list: string[]) {
  const multicall = []
  for (let i = 0, maxi = list.length; i < maxi; i++) {
    multicall.push(['aria2.forceRemove', list[i]])
    multicall.push(['aria2.removeDownloadResult', list[i]])
  }
  try {
    await GetAria()?.multicall(multicall)
    SetAriaOnline(true)
  } catch {
    SetAriaOnline(false)
  }
}


export async function AriaStopList(list: string[]) {
  const multicall = []
  for (let i = 0, maxi = list.length; i < maxi; i++) {
    multicall.push(['aria2.forcePause', list[i]])
  }
  try {
    await GetAria()?.multicall(multicall)
    SetAriaOnline(true)
  } catch {
    SetAriaOnline(false)
  }
}


export function AriaShoutDown() {
  try {
    const aria = GetAria()
    if (aria) {
      aria.call('aria2.forceShutdown').catch(() => {})
    }
  } catch {}
  // 断开 WebSocket
  CloseRemote()
  // 清理本地引擎引用
  if (Aria2EngineLocal) {
    try { Aria2EngineLocal.close() } catch {}
    Aria2EngineLocal = undefined
  }
  IsAria2cOnlineLocal = false
}

export async function AriaRawCall(method: string, ...args: any[]): Promise<any> {
  return GetAria()?.call(method, ...args)
}

export async function AriaAddUrl(file: IStateDownFile): Promise<string> {
  try {
    // 提交任务前先确保 Aria2 连接正常
    const connected = await AriaConnect()
    if (!connected) {
      if (useSettingStore().AriaIsLocal) {
        // 尝试重新连接本地 Aria2
        await AriaChangeToLocal()
        if (!IsAria2cOnlineLocal) return '本地下载服务未连接，请稍后重试'
      } else {
        return '远程下载服务未连接，请检查服务器设置'
      }
    }

    const info = file.Info
    const token = UserDAL.GetUserToken(info.user_id)
    const sourceType = info.sourceType || ''
    const isExternalSource = sourceType === 'url'
    if (!isExternalSource && !isDriveProviderSessionUsable(token, { userId: info.user_id, driveId: info.drive_id })) return '网盘登录已失效，请重新登录后再下载'
    if (info.isDir) {
      const dirFull = path.join(info.DownSavePath, info.name)
      if (!info.ariaRemote) {
        try {
          await fs.promises.mkdir(dirFull, { recursive: true })
        } catch (error: any) {
          const errorMap: Record<string, string> = {
            EPERM: '文件没有读取权限',
            EBUSY: '文件被占用或锁定中',
            EACCES: '文件没有读取权限'
          }
          const errorMessage = errorMap[error.code] || error.message
          DebugLog.mSaveLog('danger', 'AriaAddUrl创建文件夹失败：' + dirFull + ' ' + (error || ''), error)
          return errorMessage
        }
      }
      if (file.Down.IsStop) return '已暂停'
      try {
        const children = await listProviderDirChildren(info.user_id, info.drive_id, info.file_id)
        if (file.Down.IsStop) return '已暂停'
        if (children.length > 0) DownDAL.aAddDownload(children, dirFull, false, info.user_id)
      } catch (error: any) {
        DebugLog.mSaveLog('danger', 'AriaAddUrl 列出子文件失败：' + (error?.message || ''), error)
        return '解析子文件列表失败，稍后重试'
      }
      return 'downed'
    } else {
      const dirPath = info.DownSavePath
      const outFileName = info.ariaRemote ? info.name : info.name + '.td'
      const fileFull = path.join(dirPath, info.name)
      if (!info.ariaRemote) {
        try {
          const fileStat = await fs.promises.stat(fileFull)
          if (fileStat && fileStat.size == info.size) return 'downed'
          else return '本地存在重名文件，请手动删除'
        } catch (error: any) {
          const errorMap: Record<string, string> = {
            EPERM: '文件没有读取权限',
            EBUSY: '文件被占用或锁定中',
            EACCES: '文件没有读取权限'
          }
          const errorMessage = errorMap[error.code] || error.message
          if (errorMessage.indexOf('no such file') < 0) {
            DebugLog.mSaveLog(
              'danger',
              `AriaAddUrl访问文件失败：${fileFull} ${errorMessage || ''}`,
              error
            )
            return errorMessage
          }
          if (info.size == 0 && !isExternalSource) {
            try {
              await (await fs.promises.open(fileFull, 'w')).close()
              return 'downed'
            } catch {
              return '创建空文件失败'
            }
          }
        }
      }
      let downloadUrl = typeof file.Down.DownUrl === 'string' ? file.Down.DownUrl : ''
      downloadUrl = downloadUrl.trim()
      let resolvedDownloadHeaders: Record<string, string> = {}
      if (downloadUrl && downloadUrl.includes('x-oss-expires=')) {
        const expires = downloadUrl.split('x-oss-expires=')[1].split('&')[0]
        const lastTime = parseInt(expires) - Date.now() / 1000
        const needTime = (info.size + 1) / 1024 / 1024
        if (lastTime < 60 || lastTime < needTime + 60) {
          downloadUrl = ''
        }
      }
      if (!downloadUrl && !isExternalSource) {
        const durl = await getRawUrl(info.user_id, info.drive_id, info.file_id, info.encType)
        if (typeof durl == 'string') {
          console.warn('[aria2] getRawUrl failed', info.drive_id, info.file_id, durl)
          return `无法获取下载地址：${durl}`
        } else if (!durl.url && !durl.qualities?.length) {
          console.warn('[aria2] getRawUrl empty url', info.drive_id, info.file_id, durl)
          DebugLog.mSaveLog('danger', `${info.file_id} 生成下载链接失败, ${JSON.stringify(durl)}`, null)
          return '无法获取下载地址，请刷新文件列表后重试'
        }
        downloadUrl = durl.url || durl.qualities?.[0]?.url || ''
        if (durl.headers) {
          resolvedDownloadHeaders = durl.headers
        }
        file.Down.DownUrl = downloadUrl
      }
      if (!downloadUrl) {
        console.warn('[aria2] no downloadUrl before addUri', info.drive_id, info.file_id)
        return '无法获取下载地址，请刷新文件列表后重试'
      }
      const safeUrl = downloadUrl.replace(/\\u0026/g, '&')
      if (safeUrl !== downloadUrl) {
        console.warn('[aria2] normalize url', info.drive_id, info.file_id)
        downloadUrl = safeUrl
      }
      if (!/^https?:\/\//i.test(downloadUrl) || /\.torrent(?:[?#].*)?$/i.test(downloadUrl)) {
        console.warn('[aria2] invalid downloadUrl', info.drive_id, info.file_id, downloadUrl)
        return '下载地址无效，请刷新文件列表后重试'
      }
      console.log('[aria2] addUri', info.drive_id, info.file_id, { url: downloadUrl, sourceType })
      if (file.Down.IsStop) return '已暂停'
      const split = info.split || useSettingStore().downThreadMax
      const referer = info.referer || Config.referer
      const userAgent = info.userAgent || useSettingStore().ariaUserAgent || Config.downAgent
      const headers: string[] = []
      const downloadHeaders = {
        ...(info.downloadHeaders || {}),
        ...resolvedDownloadHeaders
      }
      for (const [key, value] of Object.entries(downloadHeaders)) {
        if (key && value) headers.push(`${key}: ${value}`)
      }
      headers.push(...(info.externalHeaders || []))
      if (userAgent) {
        headers.push(`User-Agent: ${userAgent}`)
      }
      const addOptions: any = buildAriaAddOptions({
        gid: info.GID,
        dir: dirPath,
        split,
        referer,
        userAgent,
        headers,
        outFileName,
        allProxy: info.allProxy
      })
      const client = GetAria()
      if (!client) return '下载服务未连接，请稍后重试'
      const multicall = [
        ['aria2.forceRemove', info.GID],
        ['aria2.removeDownloadResult', info.GID],
        ['aria2.addUri', [downloadUrl], addOptions]
      ]
      const result: any = await GetAria()?.multicall(multicall)
      console.log('[aria2] addUri result', info.drive_id, info.file_id, JSON.stringify(result))
      const addResult = result && result.length >= 3 ? result[2] : undefined
      const addGid = getAriaAddUriGid(addResult)
      if (addGid) {
        info.GID = addGid
        return 'success'
      }
      // GID 不存在时忽略清理错误，尝试单独 addUri
      let singleError: any = undefined
      let singleResult: any = await callAriaClient(client, 'aria2.addUri', [downloadUrl], addOptions, (error: unknown) => {
        singleError = error
      })
      const singleGid = getAriaAddUriGid(singleResult)
      if (singleGid) {
        info.GID = singleGid
        return 'success'
      }
      if (isAriaDuplicateGidError(singleResult) || isAriaDuplicateGidError(singleError)) {
        // GID 重复说明旧任务残留，先强制清理再重建
        await callAriaClient(client, 'aria2.forceRemove', info.GID)
        await callAriaClient(client, 'aria2.removeDownloadResult', info.GID)
        delete addOptions.gid
        singleError = undefined
        singleResult = await callAriaClient(client, 'aria2.addUri', [downloadUrl], addOptions, (error: unknown) => {
          singleError = error
        })
        const retryGid = getAriaAddUriGid(singleResult)
        if (retryGid) {
          info.GID = retryGid
          return 'success'
        }
        if (isAriaDuplicateGidError(singleResult) || isAriaDuplicateGidError(singleError)) {
          return 'success'
        }
        return '创建下载任务失败，稍后会自动重试' + ((singleResult && singleResult.message) || (singleError && singleError.message) || '')
      }
      if (!singleResult || singleResult.code) {
        delete addOptions.gid
        singleError = undefined
        singleResult = await callAriaClient(client, 'aria2.addUri', [downloadUrl], addOptions, (error: unknown) => {
          singleError = error
        })
        const fallbackGid = getAriaAddUriGid(singleResult)
        if (fallbackGid) {
          info.GID = fallbackGid
          return 'success'
        }
        return '创建下载任务失败，稍后会自动重试' + ((singleResult && singleResult.message) || (singleError && singleError.message) || (addResult && addResult.message) || '')
      }
    }
  } catch (e: any) {
    SetAriaOnline(false)
    DebugLog.mSaveLog('danger', 'AriaAddUrl' + (e.message || ''), e)
    SetAriaOnline(false)
    return Promise.resolve('创建下载任务失败，下载服务连接已断开：' + (e.message || '未知原因'))
  }
  return Promise.resolve('创建下载任务失败，请稍后重试')
}


export function AriaHashFile(downitem: IStateDownFile): { DownID: string; Check: boolean } {
  const DownID = downitem.DownID
  const dir = downitem.Info.DownSavePath
  const out = downitem.Info.ariaRemote ? downitem.Info.name : downitem.Info.name + '.td'
  const sha1 = downitem.Info.sha1
  const crc64 = downitem.Info.crc64

  const data = {
    DownID: DownID,
    inputfile: path.join(dir, out),
    movetofile: path.join(dir, downitem.Info.name),
    hash: crc64 ? 'crc64' : sha1 ? 'sha1' : '',
    check: crc64 || sha1 || ''
  }
  const expectedSize = Number(downitem.Info.size) || 0
  const isCompletedFile = (filePath: string) => {
    try {
      const stat = fs.statSync(filePath)
      return stat.isFile() && (expectedSize <= 0 || stat.size === expectedSize)
    } catch {
      return false
    }
  }
  let success = false
  if (data.inputfile == data.movetofile || isCompletedFile(data.movetofile)) {
    success = true
  } else {
    try {
      fs.renameSync(data.inputfile, data.movetofile)
      success = isCompletedFile(data.movetofile)
    } catch (e: any) {
      success = isCompletedFile(data.movetofile)
      if (!success) DebugLog.mSaveLog('danger', 'AriaRename file=' + data.inputfile + ' error=' + (e.message || ''), e)
    }
  }
  return { DownID, Check: success }
}


export function FormatAriaError(code: string, message: string): string {
  switch (code) {
    case '0':
      return ''
    case '1':
      return '下载失败，原因不明，请重试'
    case '2':
      return '连接下载地址超时，请稍后重试'
    case '3':
      return '下载地址不存在（404）'
    case '4':
      return '下载地址不存在（404）'
    case '5':
      return '下载速度过慢，任务已停止，请稍后重试'
    case '6':
      return '网络连接中断，请重试'
    case '7':
      return '下载任务被停止，请重新开始'
    case '8':
      return '服务器不支持断点续传，正在重新下载'
    case '9':
      return '保存位置的磁盘空间不足，请清理空间后重试'
    case '10':
      return '下载文件信息发生变化，请重新开始下载'
    case '11':
      return '相同的下载任务已经存在'
    case '12':
      return '相同的种子任务已经存在'
    case '13':
      return '同名文件已经存在，无法覆盖'
    case '14':
      return '无法保存下载文件，请检查同名文件或文件是否被占用'
    case '15':
      return '无法打开保存位置，请检查目录权限'
    case '16':
      return '无法创建下载文件，请检查目录权限和剩余空间'
    case '17':
      return '写入下载文件失败，请检查磁盘空间或文件权限'
    case '18':
      return '无法创建下载文件夹，请检查保存路径是否可用'
    case '19':
      return '无法找到服务器，请检查网络连接'
    case '20':
      return '无法解析磁力链接，请检查链接是否完整'
    case '21':
      return 'FTP 服务器不支持该操作'
    case '22':
      if (message.includes('403')) return '服务器拒绝访问（403），请检查链接权限'
      if (message.includes('503')) return '服务器暂时不可用（503），请稍后重试'
      return message || '服务器返回错误，请稍后重试'
    case '23':
      return '下载地址跳转失败，请检查链接是否有效'
    case '24':
      return '下载地址需要身份验证，请检查链接权限'
    case '25':
      return '种子文件格式无法识别'
    case '26':
      return '无法读取种子文件，请重新获取文件'
    case '27':
      return '磁力链接格式错误'
    case '28':
      return '下载参数无效，请重新创建任务'
    case '29':
      return '服务器繁忙，请稍后重试'
    case '30':
      return '下载服务通信参数错误，请重试'
    case '31':
      return '下载服务返回了无法识别的数据，请重试'
    case '32':
      return '文件校验失败，下载内容可能已损坏，请重试'
    default:
      return message || '下载失败，请重试'
  }
}
