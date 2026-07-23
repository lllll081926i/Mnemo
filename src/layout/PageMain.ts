import { useAppStore, useSettingStore } from '../store'
import AppCache from '../utils/appcache'
import DownDAL from '../down/DownDAL'
import UploadDAL from '../transfer/uploaddal'

import UserDAL from '../user/userdal'
import DebugLog from '../utils/debuglog'
import UploadingDAL from '../transfer/uploadingdal'
import { Sleep } from '../utils/format'
import { createProxyServer } from '../utils/proxyhelper'
import { LoadPinyinLite } from '../utils/utils'

export function PageMain() {
  if (window.WinMsg) return
  window.WinMsg = WinMsg
  Promise.resolve()
    .then(async () => {
      if (!(await LoadPinyinLite())) DebugLog.mSaveWarning('Pinyin search support could not be loaded; exact file-name search remains available')
      if (!(await useSettingStore().WebSetProxy())) DebugLog.mSaveWarning('Proxy settings were not applied because the manual proxy is incomplete')
      // 创建代理server
      if (!window.MainProxyServer) {
        window.MainProxyHost = useSettingStore().debugProxyHost
        window.MainProxyPort = useSettingStore().debugProxyPort
        window.MainProxyServer = await createProxyServer(window.MainProxyPort)
        window.MainProxyServer.on('close', async () => {
          await Sleep(2000)
          window.MainProxyServer = await createProxyServer(window.MainProxyPort)
        })
      }
      // DebugLog.mSaveSuccess('Mnemo启动')
      // 加载数据库用户
      await UserDAL.aLoadFromDB().catch((err: any) => {
        DebugLog.mSaveDanger('UserDALLDB', err)
      })
    })
    .then(async () => {
      await Sleep(500)
      // 重新启动未完成的下载和上传任务
      await DownDAL.aReloadDowning().catch((err: any) => {
        DebugLog.mSaveDanger('aReloadDowning', err)
      })

      await DownDAL.aReloadDowned().catch((err: any) => {
        DebugLog.mSaveDanger('aReloadDowned', err)
      })

      await UploadingDAL.aReloadUploading().catch((err: any) => {
        DebugLog.mSaveDanger('aReloadUploading', err)
      })

      await UploadDAL.aReloadUploaded().catch((err: any) => {
        DebugLog.mSaveDanger('aReloadUploaded', err)
      })
      await Sleep(500)

      await AppCache.aLoadDirSize().catch((err: any) => {
        DebugLog.mSaveDanger('AppDirDALDB', err)
      })

      await AppCache.aLoadCacheSize().catch((err: any) => {
        DebugLog.mSaveDanger('AppCacheDALDB', err)
      })

      // 开启定时任务
      timeEventRunning = true
      setTimeout(timeEvent, 1000)
    })
    .catch((err: any) => {
      DebugLog.mSaveDanger('LoadSettingFromDB', err)
    })
}

export const WinMsg = async (arg: any) => {
  if (arg.cmd == 'MainUploadEvent') {
    if (arg.ReportList.length > 0 && arg.ReportList.length != arg.RunningKeys.length) {
      console.log('RunningKeys', arg)
    }
    if (arg.StopKeys.length > 0) console.log('StopKeys', arg)
    UploadingDAL.aUploadingEvent(arg.ReportList, arg.ErrorList, arg.SuccessList, arg.RunningKeys, arg.StopKeys, arg.LoadingKeys, arg.SpeedTotal)
  } else if (arg.cmd == 'MainUploadAppendFiles') {
    UploadingDAL.aUploadingAppendFiles(arg.TaskID, arg.UploadID, arg.CreatedDirID, arg.AppendList)
  }
}

let runTime = Math.floor(Date.now() / 1000)
let chkClearDownLogTime = 0
let chkTokenTime = 0

let timeEventRunning = false
export function stopTimeEvent() {
  timeEventRunning = false
}

/**
 * 时间事件，一但被调用每秒执行一次 <br/>
 * 可以理解为定时任务，根据不同的时间节点执行不同的任务
 */
function timeEvent() {
  const settingStore = useSettingStore()

  const nowTime = Math.floor(Date.now() / 1000)

  // 24小时重置
  if (nowTime - runTime > 60 * 60 * 24) {
    runTime = nowTime
  }

  // 自动清除上传下载日志，540s检查一次
  chkClearDownLogTime++
  if (nowTime - runTime > 60 && chkClearDownLogTime >= 540) {
    chkClearDownLogTime = 0
    UploadDAL.aClearUploaded().catch((err: any) => {
      DebugLog.mSaveDanger('aClearUploaded ', err)
    })
    DownDAL.aClearDowned().catch((err: any) => {
      DebugLog.mSaveDanger('aClearDowned ', err)
    })
  }

  // 自动刷新Token，600s检查一次
  chkTokenTime++
  if (nowTime - runTime > 10 && chkTokenTime >= 600) {
    chkTokenTime = 0
    UserDAL.aRefreshAllUserToken().catch((err: any) => {
      DebugLog.mSaveDanger('aRefreshAllUserToken', err)
    })
  }

  // 刷新下载速度
  DownDAL.aSpeedEvent().catch((err: any) => {
    DebugLog.mSaveDanger('aSpeedEvent', err)
  })

  // 没有下载和上传时触发自动关闭
  if (settingStore.downAutoShutDown == 2) {
    if (!DownDAL.QueryIsDowning() && !UploadingDAL.QueryIsUploading()) {
      settingStore.downAutoShutDown = 0
      useAppStore().appShutDown = true
    }
  }

  if (timeEventRunning) setTimeout(timeEvent, 1000)
}
