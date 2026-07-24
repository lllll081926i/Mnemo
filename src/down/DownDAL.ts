import { IAliGetFileModel } from '../aliapi/alimodels'
import path from 'path'
import TreeStore from '../store/treestore'
import { useDownedStore, useDowningStore, useFootStore, useSettingStore, useUserStore } from '../store'
import { ClearFileName } from '../utils/filehelper'
import {
  AriaAddUrl,
  AriaConnect,
  AriaDeleteList,
  AriaGetDowningList,
  AriaHashFile,
  AriaStopList,
  FormatAriaError,
  IsAria2cRemote
} from '../utils/aria2c'
import { humanSize, humanSizeSpeed } from '../utils/format'
import DBDown from '../utils/dbdown'
import fsPromises from 'fs/promises'
import { DecodeEncName } from '../aliapi/utils'
import { getEncType } from '../utils/proxyhelper'
import { SHA256 } from 'crypto-js'
import { shouldRemoveAriaStoppedResult } from '../utils/aria2Rpc'
import { resolveAriaProgressErrorState, resolveDownloadTaskbarProgress, resolveRestoredDownloadState } from './integration/downloadProgressState'
import { isDriveProviderRootId } from '../utils/driveProvider'
import { getSystemDownloadsPath } from '../utils/electronhelper'

export interface IStateDownFile {
  DownID: string
  Info: IStateDownInfo

  Down: {
    DownState: string
    DownTime: number
    DownSize: number
    DownSpeed: number
    DownSpeedStr: string
    DownProcess: number
    IsStop: boolean
    IsDowning: boolean
    IsCompleted: boolean
    IsFailed: boolean
    FailedCode: number
    FailedMessage: string

    AutoTry: number
    ManualRetryRequired?: boolean

    DownUrl: string
  }
}

export interface IStateDownInfo {

  GID: string
  user_id: string

  DownSavePath: string
  ariaRemote: boolean

  file_id: string
  drive_id: string

  name: string

  size: number
  sizestr: string
  icon: string
  isDir: boolean
  encType: string

  sha1: string

  crc64: string

  localFilePath?: string
  downloadHeaders?: Record<string, string>
  externalHeaders?: string[]
  referer?: string
  userAgent?: string
  allProxy?: string
  sourceType?: 'url'
  split?: number
  offlineProvider?: 'pikpak'
  offlineTaskId?: string
  offlineDirId?: string
}

export interface IAriaDownProgress {
  gid: string
  status: string
  totalLength: string
  completedLength: string
  downloadSpeed: string
  errorCode: string
  errorMessage: string
}

/** 存盘的时机：默认 10 时进行 */
let SaveTimeWait = 0
let completionSound: HTMLAudioElement | undefined
let batchPendingCount = 0
let batchPendingName = ''
let batchNotifyTimer: ReturnType<typeof setTimeout> | undefined

const playCompletionSound = () => {
  if (typeof Audio === 'undefined') return
  completionSound ||= new Audio('./audio/download_finished.mp3')
  if (!completionSound.paused) return
  completionSound.currentTime = 0
  void completionSound.play().catch(() => undefined)
}

const flushBatchDownloadNotify = () => {
  batchNotifyTimer = undefined
  const stillWorking = useDowningStore().ListDataRaw.some((item) => !item.Down.IsCompleted && !item.Down.IsStop && !item.Down.IsFailed)
  if (stillWorking) {
    batchNotifyTimer = setTimeout(flushBatchDownloadNotify, 800)
    return
  }
  const setting = useSettingStore()
  const count = batchPendingCount
  const name = batchPendingName
  batchPendingCount = 0
  batchPendingName = ''
  if (count <= 0) return
  if (setting.downFinishAudio) playCompletionSound()
  if (setting.downFinishNotify) {
    window.WebToElectron?.({
      cmd: 'downloadCompleted',
      fileName: count === 1 ? name : `共 ${count} 个文件`
    })
  }
}

/** 下载完成提示：支持关闭、每文件/整批，并避免同一文件重复弹。 */
const notifyDownloadFinished = (fileName: string) => {
  const setting = useSettingStore()
  const mode = setting.downFinishNotifyMode === 'batch' ? 'batch' : 'each'

  if (mode === 'each') {
    if (setting.downFinishAudio) playCompletionSound()
    if (setting.downFinishNotify) window.WebToElectron?.({ cmd: 'downloadCompleted', fileName })
    return
  }

  batchPendingCount += 1
  if (batchPendingCount === 1) batchPendingName = fileName
  if (batchNotifyTimer) clearTimeout(batchNotifyTimer)
  batchNotifyTimer = setTimeout(flushBatchDownloadNotify, 700)
}

const buildAriaTaskGid = (file: IAliGetFileModel) => {
  const source = `${file.drive_id || ''}|${file.file_id || ''}|${file.size || 0}`
  return SHA256(source).toString().toLowerCase().replace(/[^0-9a-f]/g, '').slice(0, 16)
}

const buildUrlTaskGid = (source: string) => {
  return SHA256(source).toString().toLowerCase().replace(/[^0-9a-f]/g, '').slice(0, 16)
}

const isCompletedDowning = (downFile: IStateDownFile) => {
  return downFile.Down.IsCompleted && (downFile.Down.DownState === '已完成' || !!downFile.Info.offlineProvider)
}

export default class DownDAL {

  /**
   * 从DB中加载数据
   */
  static async aReloadDowning() {
    const downingStore = useDowningStore()
    if (downingStore.ListLoading) return
    downingStore.ListLoading = true
    await DBDown.deleteRemovedLocalBtTasks()
    const stateDownFiles = await DBDown.getDowningAll()
    // 意外中断的任务恢复排队；失败任务保留错误状态，等待用户手动重试。
    for (const stateDownFile of stateDownFiles) {
      stateDownFile.Down = resolveRestoredDownloadState(stateDownFile.Down)
    }
    downingStore.ListDataRaw = stateDownFiles
    downingStore.ListLoading = false
    downingStore.mRefreshListDataShow(true)
    DownDAL.syncTaskbarProgress()
  }

  static async aReloadDowned() {
    const downedStore = useDownedStore()
    if (downedStore.ListLoading) return
    downedStore.ListLoading = true
    const max = useSettingStore().debugDownedListMax
    const showlist = await DBDown.getDownedByTop(max)
    const count = await DBDown.getDownedTaskCount()
    downedStore.aLoadListData(showlist, count)
    downedStore.ListLoading = false
  }

  static async aClearDowned() {
    const max = useSettingStore().debugDownedListMax
    return await DBDown.deleteDownedOutCount(max)
  }

  /**
   * 添加到下载动作
   * @param fileList
   * @param savePath
   * @param needPanPath
   */
  static aAddDownload(fileList: IAliGetFileModel[], savePath: string, needPanPath: boolean, sourceUserId = '') {
    const userID = sourceUserId || useUserStore().user_id
    const settingStore = useSettingStore()
    savePath ||= settingStore.ariaState === 'remote' ? settingStore.ariaSavePath : getSystemDownloadsPath()
    if (!savePath) throw new Error('无法确定下载位置，请在设置中选择下载文件夹')

    if (savePath.endsWith('/') || savePath.endsWith('\\')) {
      savePath = savePath.substr(0, savePath.length - 1)
    }

    const downlist: IStateDownFile[] = []
    const dTime = Date.now()

    let cPid = ''
    let cPath = ''
    const ariaRemote = settingStore.ariaState == 'remote'
    const sep = settingStore.ariaSavePath.indexOf('/') >= 0 ? '/' : '\\'
    for (let f = 0; f < fileList.length; f++) {
      const file = fileList[f]
      const name = ClearFileName(DecodeEncName(userID, file).name)
      let fullPath = savePath
      if (needPanPath) {
        if (cPath != '' && cPid == file.parent_file_id) fullPath = cPath
        else {
          let cPath2 = savePath
          const plist = TreeStore.GetDirPath(file.drive_id, file.parent_file_id)
          for (let p = 0; p < plist.length; p++) {
            const pName = ClearFileName(plist[p].name)
            if (isDriveProviderRootId({ userId: userID, driveId: file.drive_id }, plist[p].file_id)) continue
            if (path.join(cPath2, pName, name).length > 250) break
            cPath2 = path.join(cPath2, pName)
          }
          cPid = file.parent_file_id
          cPath = cPath2
          fullPath = cPath2
        }
      }

      if (ariaRemote) {
        if (sep == '/') fullPath = fullPath.replace(/\\/g, '/')
        else fullPath = fullPath.replace(/\//g, '\\')
      }

      const gid = buildAriaTaskGid(file)

      let downloadurl = ''
      let crc64 = ''
      const downitem: IStateDownFile = {
        DownID: `${userID}|${file.drive_id}|${file.file_id}`,
        Info: {
          GID: gid,
          user_id: userID,
          DownSavePath: fullPath,
          ariaRemote: ariaRemote,
          file_id: file.file_id,
          drive_id: file.drive_id,
          name: name,
          size: file.size,
          sizestr: file.sizeStr,
          isDir: file.isDir,
          icon: file.icon,
          encType: getEncType(file),
          sha1: '',
          crc64: crc64
        },
        Down: {
          DownState: '队列中',
          DownTime: dTime + f,
          DownSize: 0,
          DownSpeed: 0,
          DownSpeedStr: '',
          DownProcess: 0,
          IsStop: false,
          IsDowning: false,
          IsCompleted: false,
          IsFailed: false,
          FailedCode: 0,
          FailedMessage: '',
          AutoTry: 0,
          DownUrl: downloadurl
        }
      }
      if (downitem.Info.ariaRemote && !downitem.Info.isDir) downitem.Info.icon = 'iconcloud-download'
      downlist.push(downitem)
    }
    useDowningStore().mAddDownload({ downlist })
  }

  static aAddUrlDownload(params: {
    user_id: string
    drive_id: string
    file_id: string
    url: string
    headers?: Record<string, string>
    savePath: string
    fileName: string
    fileSize?: number
    icon?: string
  }) {
    const settingStore = useSettingStore()
    const name = ClearFileName(params.fileName || 'media')
    const ariaRemote = settingStore.ariaState == 'remote'
    let fullPath = params.savePath
    if (fullPath.endsWith('/') || fullPath.endsWith('\\')) fullPath = fullPath.substr(0, fullPath.length - 1)
    if (ariaRemote) {
      const sep = settingStore.ariaSavePath.indexOf('/') >= 0 ? '/' : '\\'
      fullPath = sep == '/' ? fullPath.replace(/\\/g, '/') : fullPath.replace(/\//g, '\\')
    }
    const gid = buildUrlTaskGid(`${params.drive_id}|${params.file_id}|${params.url}|${params.fileSize || 0}`)
    const downitem: IStateDownFile = {
      DownID: `${params.user_id}|${params.drive_id}|${params.file_id}|${gid}`,
      Info: {
        GID: gid,
        user_id: params.user_id,
        DownSavePath: fullPath,
        ariaRemote,
        file_id: params.file_id,
        drive_id: params.drive_id,
        name,
        size: params.fileSize || 0,
        sizestr: params.fileSize ? humanSize(params.fileSize) : '',
        isDir: false,
        icon: params.icon || 'iconcloud-download',
        encType: '',
        sha1: '',
        crc64: '',
        downloadHeaders: params.headers || {}
      },
      Down: {
        DownState: '队列中',
        DownTime: Date.now(),
        DownSize: 0,
        DownSpeed: 0,
        DownSpeedStr: '',
        DownProcess: 0,
        IsStop: false,
        IsDowning: false,
        IsCompleted: false,
        IsFailed: false,
        FailedCode: 0,
        FailedMessage: '',
        AutoTry: 0,
        DownUrl: params.url
      }
    }
    useDowningStore().mAddDownload({ downlist: [downitem] })
  }

  static aAddExternalDownload(params: {
    source: string
    savePath: string
    fileName?: string
    split?: number
    userAgent?: string
    authorization?: string
    referer?: string
    cookie?: string
    allProxy?: string
  }) {
    const settingStore = useSettingStore()
    const userID = useUserStore().user_id || 'external'
    const ariaRemote = settingStore.ariaState == 'remote'
    let fullPath = params.savePath || (ariaRemote ? settingStore.ariaSavePath : (settingStore.downSavePath || getSystemDownloadsPath()))
    if (!fullPath) return { success: false, message: '请先选择保存目录' }
    if (fullPath.endsWith('/') || fullPath.endsWith('\\')) fullPath = fullPath.substr(0, fullPath.length - 1)

    if (ariaRemote) {
      const sep = settingStore.ariaSavePath.indexOf('/') >= 0 ? '/' : '\\'
      fullPath = sep == '/' ? fullPath.replace(/\\/g, '/') : fullPath.replace(/\//g, '\\')
    }

    const source = params.source.trim()
    if (!/^https?:\/\//i.test(source) || /\.torrent(?:[?#].*)?$/i.test(source)) {
      return { success: false, message: '仅支持 HTTP/HTTPS 下载链接，不支持 magnet 或种子文件' }
    }
    const inferredName = (() => {
      if (params.fileName?.trim()) return params.fileName.trim()
      try {
        const url = new URL(source)
        const name = decodeURIComponent(url.pathname.split('/').filter(Boolean).pop() || '')
        return name || 'URL 下载任务'
      } catch {
        return 'URL 下载任务'
      }
    })()
    const name = ClearFileName(inferredName)
    const gid = buildUrlTaskGid(source)
    const downitem: IStateDownFile = {
      DownID: `${userID}|external|${gid}`,
      Info: {
        GID: gid,
        user_id: userID,
        DownSavePath: fullPath,
        ariaRemote,
        file_id: gid,
        drive_id: 'external',
        name,
        size: 0,
        sizestr: '',
        isDir: false,
        icon: 'iconcloud-download',
        encType: '',
        sha1: '',
        crc64: '',
        externalHeaders: [
          params.authorization ? `Authorization: ${params.authorization}` : '',
          params.cookie ? `Cookie: ${params.cookie}` : ''
        ].filter(Boolean),
        referer: params.referer,
        userAgent: params.userAgent,
        allProxy: params.allProxy,
        sourceType: 'url',
        split: params.split
      },
      Down: {
        DownState: '队列中',
        DownTime: Date.now(),
        DownSize: 0,
        DownSpeed: 0,
        DownSpeedStr: '',
        DownProcess: 0,
        IsStop: false,
        IsDowning: false,
        IsCompleted: false,
        IsFailed: false,
        FailedCode: 0,
        FailedMessage: '',
        AutoTry: 0,
        DownUrl: source
      }
    }
    useDowningStore().mAddDownload({ downlist: [downitem] })
    return { success: true, message: '' }
  }

  /**
   * 速度事件动作
   */
  static async aSpeedEvent() {
    const downingStore = useDowningStore()
    const downedStore = useDownedStore()
    const settingStore = useSettingStore()

    const isOnline = await AriaConnect()

    if (isOnline && downingStore.ListDataRaw.length) {
      await AriaGetDowningList()
      const ariaRemote = IsAria2cRemote()
      const DowningList: IStateDownFile[] = downingStore.ListDataRaw
      const timeThreshold = Date.now() - 60 * 1000
      const downFileMax = settingStore.downFileMax
      const shouldSkipDown = (Down: any) => {
        return (
          Down.IsCompleted ||
          Down.IsStop ||
          Down.IsDowning ||
          Down.ManualRetryRequired ||
          (Down.IsFailed && timeThreshold <= Down.AutoTry)
        )
      }
      let addDowningCount = 0
      for (let i = 0; i < DowningList.length; i++) {
        const DownItem = DowningList[i]
        const { DownID, Info, Down } = DownItem
        if (Info.ariaRemote !== ariaRemote) continue
        if (isCompletedDowning(DownItem)) {
          // 将下载标记为已完成并添加到列表以供稍后处理
          const completedDownId = `${Date.now()}_${Down.DownTime}`
          // 删除已完成的下载并更新数据库
          DowningList.splice(i, 1)
          await DBDown.deleteDowning(DownID)
          // 将已完成的下载添加到下载文件列表中
          const downedData = JSON.parse(JSON.stringify({ DownID: completedDownId, Down, Info }))
          downedStore.ListDataRaw.unshift({ DownID: completedDownId, Down, Info })
          downedStore.mRefreshListDataShow(true)
          await DBDown.saveDowned(completedDownId, downedData)
          if (downedStore.ListSelected.has(completedDownId)) {
            downedStore.ListSelected.delete(completedDownId)
          }
          // 移除Aria2已完成的任务
          await AriaDeleteList([Info.GID])
          i--
        } else if ((addDowningCount + downingStore.ListDataDowningCount) < downFileMax && !shouldSkipDown(Down)) {
          addDowningCount++
          downingStore.mUpdateDownState(DownItem, 'start')
          let state = await AriaAddUrl(DownItem)
          downingStore.mUpdateDownState(DownItem, state)
        }
      }
    } else {
      useFootStore().mSaveDownTotalSpeedInfo('')
    }
    await DownDAL.aPikPakOfflineProgress()

    downingStore.mRefreshListDataShow(true)
    downedStore.mRefreshListDataShow(true)
  }

  /**
   * 速度事件方法
   */
  static mSpeedEvent(list: IAriaDownProgress[]) {
    const downingStore = useDowningStore()
    const settingStore = useSettingStore()
    const DowningList: IStateDownFile[] = downingStore.ListDataRaw
    const ariaRemote = !settingStore.AriaIsLocal

    const dellist: string[] = []
    const saveList: IStateDownFile[] = []

    let hasSpeed = 0

    for (const listItem of list) {
      try {
        const { gid, status, totalLength, completedLength, downloadSpeed, errorCode, errorMessage } = listItem
        const isComplete = status === 'complete'
        const isDowning = isComplete || status === 'active' || status === 'waiting'
        const isStop = status === 'paused' || status === 'removed'
        const isError = status === 'error'
        const downingItem: IStateDownFile | undefined = DowningList.find((item) => item.Info.ariaRemote === ariaRemote && item.Info.GID === gid)
        if (!downingItem) continue
        const { DownID, Down, Info } = downingItem
        const totalLengthInt = parseInt(totalLength) || 0
        Down.DownSize = parseInt(completedLength) || 0
        Down.DownSpeed = parseInt(downloadSpeed) || 0
        Down.DownSpeedStr = humanSize(Down.DownSpeed) + '/s'
        Down.DownProcess = Math.floor((Down.DownSize * 100) / (totalLengthInt + 1)) % 100
        const justFinished = isComplete && !Down.IsCompleted
        Down.IsCompleted = isComplete
        Down.IsDowning = isDowning
        const errorState = resolveAriaProgressErrorState({ status, errorCode, errorMessage }, FormatAriaError)
        Down.IsFailed = errorState.isFailed
        // 保护 '队列中' 状态不被 Aria2 'paused' 覆盖（用户刚点开始，aria2.unpause 尚未生效）
        if (Down.DownState !== '队列中') {
          Down.IsStop = isStop
        }
        Down.FailedCode = errorState.failedCode
        Down.FailedMessage = errorState.failedMessage
        if (justFinished) {
          downingStore.mUpdateDownState(downingItem, 'valid')
          const check = AriaHashFile(downingItem)
          if (check.Check) {
            downingStore.mUpdateDownState(downingItem, 'downed')
            notifyDownloadFinished(Info.name)
          } else {
            downingStore.mUpdateDownState(downingItem, 'error', '下载已完成，但文件未能保存到下载目录。请检查文件是否被占用或下载目录是否可写，然后重试')
          }
        } else if (isStop && Down.DownState !== '队列中') {
          downingStore.mUpdateDownState(downingItem, 'stop')
          if (shouldRemoveAriaStoppedResult(status)) dellist.push(gid)
        } else if (isError) {
          downingStore.mUpdateDownState(downingItem, 'error', Down.FailedMessage)
          if (shouldRemoveAriaStoppedResult(status)) dellist.push(gid)
        } else if (isDowning) {
          hasSpeed += Down.DownSpeed
          let lastTime = ((totalLengthInt - Down.DownSize) / (Down.DownSpeed + 1)) % 356400
          if (lastTime < 1) lastTime = 1
          // 进度条
          Down.DownState =
            `${Down.DownProcess}% ${(lastTime / 3600).toFixed(0).padStart(2, '0')}:${((lastTime % 3600) / 60)
              .toFixed(0)
              .padStart(2, '0')}:${(lastTime % 60).toFixed(0).padStart(2, '0')}`
          if (SaveTimeWait > 10) {
            saveList.push(downingItem)
          }
        }
        downingStore.mRefreshListDataShow(true)
      } catch {
        // Ignore any errors
      }
    }
    // 存盘时间
    SaveTimeWait = (SaveTimeWait + 1) % 11
    if (saveList.length) {
      DBDown.saveDownings(JSON.parse(JSON.stringify(saveList)))
    }
    if (dellist.length) {
      AriaDeleteList(dellist).then()
    }
    useFootStore().mSaveDownTotalSpeedInfo(hasSpeed && humanSizeSpeed(hasSpeed) || '')

    const totalCount = DowningList.filter((d) => !d.Down.IsCompleted).length
    const activeCount = DowningList.filter((d) => d.Down.IsDowning && !d.Down.IsCompleted).length
    const overallProgress = resolveDownloadTaskbarProgress(DowningList, settingStore.downSaveShowPro)
    window.WebToElectron?.({ cmd: 'downloadProgress', progress: overallProgress, activeCount, totalCount })
  }

  static async deleteDowning(isAll: boolean, deleteList: IStateDownFile[], gidList: string[]) {
    DownDAL.syncTaskbarProgress()
    // 处理待删除文件
    if (!isAll) {
      const downIDList = deleteList.map(item => item.DownID)
      // console.log('deleteDowning', deleteList)
      await DBDown.deleteDownings(JSON.parse(JSON.stringify(downIDList)))
    } else {
      await DBDown.deleteDowningAll()
    }
    // 停止aria2下载任务
    await AriaStopList(gidList)
    await AriaDeleteList(gidList)
    const pikpakTaskMap = new Map<string, string[]>()
    for (const downFile of deleteList) {
      if (!downFile.Info.offlineTaskId) continue
      if (downFile.Info.offlineProvider === 'pikpak') {
        const list = pikpakTaskMap.get(downFile.Info.user_id) || []
        list.push(downFile.Info.offlineTaskId)
        pikpakTaskMap.set(downFile.Info.user_id, list)
      }
    }
    if (pikpakTaskMap.size) {
      const { apiPikPakOfflineDelete } = await import('../pikpak/offline')
      for (const [userID, taskIds] of pikpakTaskMap) {
        await apiPikPakOfflineDelete(userID, taskIds)
      }
    }
    // 删除临时文件
    for (let downFile of deleteList) {
      let downInfo = downFile.Info
      if (downInfo.offlineProvider) continue
      if (downInfo.ariaRemote) continue
      try {
        if (!downInfo.isDir) {
          let filePath = path.join(downInfo.DownSavePath, downInfo.name)
          let tmpFilePath1 = filePath + '.td.aria2'
          let tmpFilePath2 = filePath + '.td'
          const tmpFilePath3 = filePath + '.td.json'
          await fsPromises.rm(tmpFilePath1, { recursive: true })
          await fsPromises.rm(tmpFilePath2, { recursive: true })
          await fsPromises.rm(tmpFilePath3, { recursive: true })
        }
      } catch (e) {
      }
    }
    DownDAL.syncTaskbarProgress()
  }

  static async deleteDowned(isAll: boolean, deleteList: IStateDownFile[]) {
    if (!isAll) {
      // 处理待删除状态
      const downIDList = deleteList
        .filter(list => list.Down.DownState === '待删除')
        .map(item => item.DownID)
      console.log('downedList', deleteList)
      await DBDown.deleteDowneds(JSON.parse(JSON.stringify(downIDList)))
    } else {
      await DBDown.deleteDownedAll()
    }
  }

  static async stopDowning(downList: IStateDownFile[], gidList: string[]) {
    DownDAL.syncTaskbarProgress()
    await DBDown.saveDownings(JSON.parse(JSON.stringify(downList)))
    await AriaStopList(gidList)
    DownDAL.syncTaskbarProgress()
  }

  static syncTaskbarProgress() {
    const list = useDowningStore().ListDataRaw
    const activeCount = list.filter((item) => item.Down.IsDowning && !item.Down.IsCompleted).length
    const totalCount = list.filter((item) => !item.Down.IsCompleted).length
    const progress = resolveDownloadTaskbarProgress(list, useSettingStore().downSaveShowPro)
    window.WebToElectron?.({ cmd: 'downloadProgress', progress, activeCount, totalCount })
  }

  static QueryIsDowning() {
    return useDowningStore().ListDataDowningCount > 0
  }

  static async aAddPikPakOfflineDownload(url: string, fileName: string, dirID: string | undefined) {
    const userID = useUserStore().user_id
    if (!userID) return { success: false, message: '请先登录' }
    const { apiPikPakOfflineCreate } = await import('../pikpak/offline')
    const resp = await apiPikPakOfflineCreate(userID, url, fileName, dirID)
    if (!resp.taskId && !resp.fileId) return { success: false, message: resp.error || '创建离线下载失败' }
    const taskId = String(resp.taskId || resp.fileId)
    const downitem: IStateDownFile = {
      DownID: `${userID}|pikpak_offline_${taskId}`,
      Info: {
        GID: `pikpak_offline_${taskId}`,
        user_id: userID,
        DownSavePath: '',
        ariaRemote: false,
        file_id: resp.fileId,
        drive_id: 'pikpak',
        name: fileName || url,
        size: 0,
        sizestr: '',
        icon: 'iconcloud-download',
        isDir: false,
        encType: '',
        sha1: '',
        crc64: '',
        offlineProvider: 'pikpak',
        offlineTaskId: taskId,
        offlineDirId: dirID || ''
      },
      Down: {
        DownState: '离线下载中',
        DownTime: Date.now(),
        DownSize: 0,
        DownSpeed: 0,
        DownSpeedStr: '',
        DownProcess: 0,
        IsStop: false,
        IsDowning: true,
        IsCompleted: false,
        IsFailed: false,
        FailedCode: 0,
        FailedMessage: '',
        AutoTry: 0,
        DownUrl: url
      }
    }
    useDowningStore().mAddDownload({ downlist: [downitem] })
    return { success: true, message: '' }
  }

  private static pikpakOfflineTick = 0

  static async aPikPakOfflineProgress() {
    const downingStore = useDowningStore()
    const list = downingStore.ListDataRaw
    if (!list.length) return
    DownDAL.pikpakOfflineTick = (DownDAL.pikpakOfflineTick + 1) % 5
    if (DownDAL.pikpakOfflineTick !== 0) return
    const { apiPikPakOfflineProcess } = await import('../pikpak/offline')
    const saveList: IStateDownFile[] = []
    for (let i = 0; i < list.length; i++) {
      const item = list[i]
      if (item.Info.offlineProvider !== 'pikpak' || !item.Info.offlineTaskId) continue
      if (item.Down.IsCompleted || item.Down.IsFailed) continue
      const info = await apiPikPakOfflineProcess(item.Info.user_id, item.Info.offlineTaskId, item.Info.file_id)
      if (info.error) {
        item.Down.IsFailed = true
        item.Down.IsDowning = false
        item.Down.DownState = '离线下载失败'
        item.Down.FailedMessage = info.error
        saveList.push(item)
        continue
      }
      const process = Math.max(0, Math.min(100, info.process))
      item.Down.DownProcess = process
      item.Down.DownSpeedStr = ''
      if (info.status === 2) {
        item.Down.IsCompleted = true
        item.Down.IsDowning = false
        item.Down.DownState = '离线下载完成'
        item.Down.DownProcess = 100
      } else if (info.status === 1) {
        item.Down.IsFailed = true
        item.Down.IsDowning = false
        item.Down.DownState = '离线下载失败'
      } else if (info.status === 3) {
        item.Down.IsDowning = true
        item.Down.DownState = `离线下载等待中 ${process}%`
      } else {
        item.Down.IsDowning = true
        item.Down.DownState = `离线下载中 ${process}%`
      }
      saveList.push(item)
    }
    if (saveList.length) {
      DBDown.saveDownings(JSON.parse(JSON.stringify(saveList)))
    }
  }

}
