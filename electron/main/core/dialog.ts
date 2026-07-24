import { app, dialog } from 'electron'

export function ShowErrorAndRelaunch(title: string, errmsg: string) {
  dialog
    .showMessageBox({
      type: 'error',
      buttons: ['重新启动', '关闭'],
      defaultId: 0,
      cancelId: 1,
      title: title,
      message: '错误信息: ' + errmsg,
      detail: '渲染进程异常。可选择重新启动 Mnemo，或关闭应用。'
    })
    .then((result) => {
      if (result.response !== 0) {
        try {
          app.exit(0)
        } catch {}
        return
      }
      setTimeout(() => {
        app.relaunch()
        try {
          app.exit(0)
        } catch {}
      }, 100)
    })
    .catch(() => {})
}

export function ShowErrorAndExit(title: string, errmsg: string) {
  dialog
    .showMessageBox({
      type: 'error',
      buttons: ['确定'],
      title: title,
      message: '错误信息: ' + errmsg
    })
    .then(() => {
      setTimeout(() => {
        try {
          app.exit(0)
        } catch {}
      }, 100)
    })
    .catch(() => {})
}

export function ShowError(title: string, errmsg: string) {
  dialog
    .showMessageBox({
      type: 'error',
      buttons: ['确定'],
      title: title,
      message: '错误信息: ' + errmsg
    })
    .catch(() => {})
}