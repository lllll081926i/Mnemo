import { useSettingStore } from '../store'
import UserDAL from '../user/userdal'
import { UploadAdd, UploadCmd, UploadReport } from './uiupload'

// eslint-disable-next-line no-unused-vars
let workerTimer: any
export function WorkerPage(type: string) {
  if (window.WinMsg) return

  if (type == 'upload') {
    window.WinMsg = WinMsgUpload
    window.Electron.ipcRenderer.send('uploadWorkerReady')
    const func = () => {
      try {
        UploadReport().catch()
      } catch {}
      workerTimer = setTimeout(func, 1000)
    }
    workerTimer = setTimeout(func, 6000)
    const element = document.createElement('div')
    const title = document.createElement('h3')
    title.className = 'workertitle'
    title.textContent = '上传进程'
    element.append(title)
    document.body.append(element)
  }
}

export const WinMsgUpload = function (arg: any) {
  try {
    if (arg.cmd == 'SettingRefresh') {
      useSettingStore().$reset()
    } else if (arg.cmd == 'ClearUserToken') {
      UserDAL.ClearUserTokenMap()
    } else if (arg.cmd == 'UploadAdd') {
      UploadAdd(arg.UploadList)
    } else if (arg.cmd == 'UploadCmd') {
      UploadCmd(arg.Command, arg.IsAll, arg.UploadIDList, arg.TaskIDList)
    }
  } catch {}
}
