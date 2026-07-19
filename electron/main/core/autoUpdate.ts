import { app, dialog } from 'electron'
import { autoUpdater } from 'electron-updater'
import type { UpdateInfo } from 'electron-updater'
import is from 'electron-is'

const UPDATE_CHECK_DELAY_MS = 8000

type AutoUpdateLogger = Pick<typeof console, 'info' | 'warn'>
type AutoUpdateDialog = {
  showMessageBox: (options: Electron.MessageBoxOptions) => Promise<{ response: number }>
}
type AutoUpdaterAdapter = {
  autoDownload: boolean
  autoInstallOnAppQuit: boolean
  allowPrerelease: boolean
  on: (event: 'update-available' | 'update-downloaded' | 'error', listener: (value: any) => void) => unknown
  checkForUpdates: () => Promise<unknown>
  downloadUpdate: () => Promise<unknown>
  quitAndInstall: (isSilent?: boolean, isForceRunAfter?: boolean) => void
}
type AutoUpdateControllerOptions = {
  updater: AutoUpdaterAdapter
  dialog: AutoUpdateDialog
  logger: AutoUpdateLogger
  currentVersion: string
  isPackaged: boolean
  isMas?: boolean
}

export function createAutoUpdateController(options: AutoUpdateControllerOptions) {
  const { updater, dialog, logger, currentVersion, isPackaged, isMas = false } = options

  if (isMas) return

  if (!isPackaged) return

  updater.autoDownload = false
  updater.autoInstallOnAppQuit = false
  updater.allowPrerelease = currentVersion.includes('-')

  let acceptedVersion = ''
  let isPromptOpen = false
  let isDownloading = false
  let isInstalling = false

  updater.on('update-available', (info: UpdateInfo) => {
    if (isPromptOpen || isDownloading || isInstalling) return
    isPromptOpen = true
    logger.info('[auto-update] update available', info.version)
    dialog
      .showMessageBox({
        type: 'info',
        title: '检测到新版本',
        message: `检测到新版本 ${info.version}，是否下载并安装？`,
        detail: '确认后将自动下载并启动安装程序，安装完成后会重新打开 Mnemo。',
        buttons: ['下载并安装', '暂不更新'],
        defaultId: 0,
        cancelId: 1,
        noLink: true
      })
      .then(({ response }) => {
        isPromptOpen = false
        if (response !== 0) {
          logger.info('[auto-update] update declined', info.version)
          return
        }
        acceptedVersion = info.version
        isDownloading = true
        logger.info('[auto-update] downloading accepted update', info.version)
        return updater.downloadUpdate().catch((err: unknown) => {
          acceptedVersion = ''
          isDownloading = false
          logger.warn('[auto-update] download failed', err)
        })
      })
      .catch((err: unknown) => {
        isPromptOpen = false
        logger.warn('[auto-update] update prompt failed', err)
      })
  })

  updater.on('update-downloaded', (info: UpdateInfo) => {
    if (isInstalling || !acceptedVersion || info.version !== acceptedVersion) return
    isInstalling = true
    logger.info('[auto-update] update downloaded, starting installer', info.version)
    try {
      updater.quitAndInstall(false, true)
    } catch (err: unknown) {
      isInstalling = false
      logger.warn('[auto-update] installer launch failed', err)
    }
  })

  updater.on('error', (err: unknown) => {
    logger.warn('[auto-update] updater error', err)
  })

  setTimeout(() => {
    updater.checkForUpdates().catch((err: unknown) => {
      logger.warn('[auto-update] check failed', err)
    })
  }, UPDATE_CHECK_DELAY_MS)
}

export function registerAutoUpdate() {
  createAutoUpdateController({
    updater: autoUpdater,
    dialog,
    logger: console,
    currentVersion: app.getVersion(),
    isPackaged: app.isPackaged,
    isMas: is.mas()
  })
}

export async function checkForUpdatesNow() {
  if (!app.isPackaged || is.mas()) return { ok: false, error: '当前环境不支持自动更新检查' }
  try {
    const result = await autoUpdater.checkForUpdates()
    return { ok: true, version: result?.updateInfo?.version || app.getVersion() }
  } catch (error: any) {
    return { ok: false, error: error?.message || '检查更新失败' }
  }
}
