import path from 'path'
import { throttle } from './debounce'

export function copyToClipboard(text: string): void {
  window.Electron.clipboard.writeText(text, 'clipboard')
}
export function openExternal(url: string): void {
  try {
    const target = new URL(url)
    if (target.protocol === 'https:' || target.protocol === 'http:') window.Electron.shell.openExternal(target.toString())
  } catch {}
}

const ElectronPath = {
  AppUserData: '',
  AppPlatform: '',
  AppArch: '',
  AppExecPath: '',
  AppUserName: '',
  env: ''
}


function LoadElectronPath(): void {
  if (!ElectronPath.AppUserData) {
    ElectronPath.AppPlatform = process.platform
    ElectronPath.AppArch = process.arch
    ElectronPath.AppExecPath = process.execPath
    ElectronPath.env = JSON.stringify(process.env)
    ElectronPath.AppUserName = process.env.USERNAME || process.env.USER || ''
    if (window.WebPlatformSync) {
      window.WebPlatformSync((data: { appPath: string; execPath: string }) => {
        ElectronPath.AppUserData = data.appPath
        ElectronPath.AppExecPath = data.execPath
        window.Electron.WebPlatformSync = data
      })
    }

    window.Electron.ElectronPath = ElectronPath
  }
}

export function getUserData(): string {
  LoadElectronPath()
  return ElectronPath.AppUserData
}

export function getUserDataPath(fileName: string): string {
  try {
    LoadElectronPath()
    return path.join(ElectronPath.AppUserData, fileName) as string
  } catch {
    return ''
  }
}

export function getSystemDownloadsPath(): string {
  let downloadsPath = ''
  try {
    if (window.WebPlatformSync) {
      window.WebPlatformSync((data: { downloadsPath?: string }) => {
        downloadsPath = data?.downloadsPath || ''
      })
    }
  } catch {}
  return downloadsPath
}

let ProgressBarBy = ''
let ProgressBarValue = -1
let ProgressBarNew = -1
const setProgressBar = throttle(() => {
  ProgressBarValue = ProgressBarNew
  const mode = ProgressBarValue < 0 ? 'none' : ProgressBarBy == 'download' ? 'normal' : 'paused'
  if (window.WebSetProgressBar) window.WebSetProgressBar({ pro: ProgressBarValue, mode })
}, 5000)


export function SetProgressBar(value: number, by: string): void {
  if (value < 0) value = -1
  if (ProgressBarValue == value && ProgressBarBy == by) return

  ProgressBarNew = value
  ProgressBarBy = by
  if (value < 0 || (ProgressBarValue < 0 && value > 0)) {
    
    const mode = value < 0 ? 'none' : ProgressBarBy == 'download' ? 'normal' : 'paused'
    ProgressBarValue = value
    if (window.WebSetProgressBar) window.WebSetProgressBar({ pro: ProgressBarValue, mode: mode })
  } else {
    
    setProgressBar()
  }
}
