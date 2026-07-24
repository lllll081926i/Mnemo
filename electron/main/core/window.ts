import { app, BrowserWindow, ipcMain, Menu, MessageChannelMain, nativeImage, nativeTheme, screen, shell, Tray } from 'electron'
import { getAsarPath, getPreloadPath, getUserDataPath } from '../utils/mainfile'
import { loadAppNativeImage, loadTrayNativeImage } from '../utils/appIcon'
import { existsSync, readFileSync, writeFileSync } from 'fs'
import is from 'electron-is'
import { ShowErrorAndRelaunch } from './dialog'

const DEBUGGING = !app.isPackaged
export const ua = `Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Mnemo/${app.getVersion()} Chrome/120.0.0.0 Safari/537.36`
export const Referer = 'https://www.aliyundrive.com/'
export const AppWindow: {
  mainWindow: BrowserWindow | undefined
  uploadWindow: BrowserWindow | undefined
  appTray: Tray | undefined
  winWidth: number
  winHeight: number
  winTheme: string
} = {
  mainWindow: undefined,
  uploadWindow: undefined,
  appTray: undefined,
  winWidth: 0,
  winHeight: 0,
  winTheme: ''
}

let timerUpload: NodeJS.Timeout | undefined
const debounceUpload = (fn: any, wait: number) => {
  if (timerUpload) {
    clearTimeout(timerUpload)
  }
  timerUpload = setTimeout(() => {
    fn()
    timerUpload = undefined
  }, wait)
}
let timerResize: NodeJS.Timeout | undefined
const debounceResize = (fn: any, wait: number) => {
  if (timerResize) clearTimeout(timerResize)
  timerResize = setTimeout(() => {
    fn()
    timerResize = undefined
  }, wait)
}
nativeTheme.on('updated', () => {
  if (AppWindow.mainWindow && !AppWindow.mainWindow.isDestroyed())
    AppWindow.mainWindow.webContents.send('setTheme', {
      dark: nativeTheme.shouldUseDarkColors
    })
})

export function createMainWindow() {
  Menu.setApplicationMenu(null)
  try {
    const configJson = getUserDataPath('config.json')
    if (existsSync(configJson)) {
      const configData = JSON.parse(readFileSync(configJson, 'utf-8'))
      AppWindow.winWidth = configData.width
      AppWindow.winHeight = configData.height
    }
  } catch {}
  try {
    const themeJson = getUserDataPath('theme.json')
    if (existsSync(themeJson)) {
      const themeData = JSON.parse(readFileSync(themeJson, 'utf-8'))
      AppWindow.winTheme = themeData.theme
    }
  } catch {}
  if (AppWindow.winWidth <= 0) {
    try {
      const size = screen.getPrimaryDisplay().workAreaSize
      let width = size.width * 0.677
      const height = size.height * 0.866
      if (width > AppWindow.winWidth) AppWindow.winWidth = width
      if (AppWindow.winWidth > 990) AppWindow.winWidth = 990
      if (height > AppWindow.winHeight) AppWindow.winHeight = height
      if (AppWindow.winHeight > 680) AppWindow.winHeight = 680
    } catch {
      AppWindow.winWidth = 990
      AppWindow.winHeight = 680
    }
  }
  AppWindow.mainWindow = createElectronWindow(AppWindow.winWidth, AppWindow.winHeight, true, 'main', AppWindow.winTheme, true)

  const showMainPage = () => {
    if (!AppWindow.mainWindow || AppWindow.mainWindow.isDestroyed()) return
    AppWindow.mainWindow.webContents.send('setPage', { page: 'PageMain' })
    AppWindow.mainWindow.webContents.send('setTheme', { dark: nativeTheme.shouldUseDarkColors })
  }

  AppWindow.mainWindow.on('resize', () => {
    debounceResize(function () {
      try {
        if (AppWindow.mainWindow && !AppWindow.mainWindow.isMaximized() && !AppWindow.mainWindow.isMinimized() && !AppWindow.mainWindow.isFullScreen()) {
          const s = AppWindow.mainWindow!.getSize()
          const configJson = getUserDataPath('config.json')
          writeFileSync(configJson, `{"width":${s[0].toString()},"height": ${s[1].toString()}}`, 'utf-8')
        }
      } catch {}
    }, 3000)
  })

  AppWindow.mainWindow.on('close', (event) => {
    if (is.macOS()) {
      // donothing
    } else {
      event.preventDefault()
      AppWindow.mainWindow?.hide()
    }
  })

  AppWindow.mainWindow.webContents.once('dom-ready', showMainPage)
  AppWindow.mainWindow.webContents.once('did-finish-load', showMainPage)
  setTimeout(showMainPage, 500)
  setTimeout(showMainPage, 1500)

  AppWindow.mainWindow.on('ready-to-show', function () {
    showMainPage()
    AppWindow.mainWindow!.setTitle('Mnemo')
    if (is.windows() && process.argv && process.argv.join(' ').indexOf('--openAsHidden') < 0) {
      AppWindow.mainWindow!.show()
    } else if (is.macOS() && !app.getLoginItemSettings().wasOpenedAsHidden) {
      AppWindow.mainWindow!.show()
    }
    if (is.linux()) {
      AppWindow.mainWindow!.show()
    }
  })

  AppWindow.mainWindow.webContents.on('render-process-gone', function (event, details) {
    if (details.reason == 'crashed' || details.reason == 'oom' || details.reason == 'killed') {
      ShowErrorAndRelaunch('(⊙o⊙)？Mnemo遇到错误崩溃了', details.reason)
    }
  })

}

/** 显示已有主窗口，没有则创建。用于托盘点击与二次启动唤起。 */
export function showOrCreateMainWindow() {
  if (AppWindow.mainWindow && AppWindow.mainWindow.isDestroyed() == false) {
    const win = AppWindow.mainWindow
    if (win.isMinimized()) win.restore()
    if (!win.isVisible()) win.show()
    // Windows 上从托盘/二次启动唤起时，强制置前，避免焦点被原进程抢走
    if (is.windows()) {
      win.setAlwaysOnTop(true)
      win.show()
      win.focus()
      win.moveTop()
      setTimeout(() => {
        if (!win.isDestroyed()) win.setAlwaysOnTop(false)
      }, 200)
    } else {
      win.show()
      win.focus()
    }
    return
  }
  createMainWindow()
}

export function createTray() {
  const trayMenuTemplate = [
    {
      label: '显示主界面',
      click: function () {
        showOrCreateMainWindow()
      }
    },
    {
      label: '彻底退出并停止下载',
      click: function () {
        if (AppWindow.mainWindow) {
          AppWindow.mainWindow.destroy()
          AppWindow.mainWindow = undefined
        }
        app.quit()
      }
    }
  ]

  try {
    const trayImage = loadTrayNativeImage()
    if (trayImage.isEmpty()) {
      console.error('[mnemo] tray NativeImage empty — check static/images')
      return
    }
    AppWindow.appTray = new Tray(trayImage)
    AppWindow.appTray.setImage(trayImage)
    console.log('[mnemo] tray icon ok', trayImage.getSize())
  } catch (err) {
    console.error('[mnemo] createTray icon failed, skip tray', err)
    return
  }
  const contextMenu = Menu.buildFromTemplate(trayMenuTemplate)
  AppWindow.appTray.setToolTip('Mnemo')
  AppWindow.appTray.setContextMenu(contextMenu)
  AppWindow.appTray.on('click', () => {
    showOrCreateMainWindow()
  })
}

export function createElectronWindow(width: number, height: number, center: boolean, page: string, theme: string, devTools: boolean = true, backgroundThrottling: boolean = true) {
  const allowWorkerNodeIntegration = page === 'main'
  const win = new BrowserWindow({
    show: false,
    title: 'Mnemo',
    width: width,
    height: height,
    minWidth: width > 680 ? 680 : width,
    minHeight: height > 500 ? 500 : height,
    center: center,
    icon: loadAppNativeImage(),
    useContentSize: true,
    frame: false,
    transparent: false,
    hasShadow: width > 680,
    autoHideMenuBar: true,
    backgroundColor: theme && theme == 'dark' ? '#23232e' : '#ffffff',
    webPreferences: {
      spellcheck: false,
      devTools: DEBUGGING && devTools,
      webviewTag: false,
      nodeIntegration: true,
      nodeIntegrationInWorker: allowWorkerNodeIntegration,
      sandbox: false,
      webSecurity: false,
      allowRunningInsecureContent: false,
      contextIsolation: false,
      backgroundThrottling,
      enableWebSQL: false,
      disableBlinkFeatures: 'OutOfBlinkCors,SameSiteByDefaultCookies,CookiesWithoutSameSiteMustBeSecure',
      preload: getPreloadPath()
    }
  })
  win.setTitle('Mnemo')
  try {
    const appIcon = loadAppNativeImage()
    if (!appIcon.isEmpty()) win.setIcon(appIcon)
  } catch (err) {
    console.error('[mnemo] setIcon failed', err)
  }
  win.removeMenu()
  if (DEBUGGING) {
    const devUrl = process.env.VITE_DEV_SERVER_URL || ''
    win.loadURL(page === 'worker' ? new URL('worker.html', devUrl).toString() : devUrl, { userAgent: ua, httpReferrer: Referer })
  } else {
    const htmlFile = page === 'worker' ? 'worker.html' : 'index.html'
    win.loadURL('file://' + getAsarPath('dist/' + htmlFile), {
      userAgent: ua,
      httpReferrer: Referer
    })
  }

  const allowDevTools = DEBUGGING && devTools
  if (allowDevTools) {
    if (width < 100) {
      win.setSize(800, 680)
    }
    win.show()
    win.webContents.openDevTools({ mode: 'bottom' })
  }
  if (page == 'main2') {
    handleWinCmd(win)
  }
  if (allowDevTools) registerDevToolsShortcut(win.webContents)
  else disableDevTools(win.webContents)
  win.webContents.setWindowOpenHandler(({ url }) => {
    if (isSafeExternalUrl(url)) void shell.openExternal(url)
    return { action: 'deny' }
  })
  win.webContents.on('will-navigate', (e, url) => {
    const devServerUrl = process.env.VITE_DEV_SERVER_URL
    if (devServerUrl && isSameOrigin(url, devServerUrl)) return
    e.preventDefault()
    if (isSafeExternalUrl(url)) void shell.openExternal(url)
  })
  return win
}

const isSafeExternalUrl = (value: string) => {
  try {
    const protocol = new URL(value).protocol
    return protocol === 'https:' || protocol === 'http:'
  } catch {
    return false
  }
}

const isSameOrigin = (value: string, expected: string) => {
  try {
    return new URL(value).origin === new URL(expected).origin
  } catch {
    return false
  }
}

function registerDevToolsShortcut(webContent: Electron.WebContents) {
  webContent.on('before-input-event', (_, input: Electron.Input) => {
    if (input.type === 'keyDown' && input.control && input.shift && input.key === 'F12') {
      webContent.isDevToolsOpened() ? webContent.closeDevTools() : webContent.openDevTools({ mode: 'undocked' })
    }
  })
}

function disableDevTools(webContent: Electron.WebContents) {
  webContent.on('devtools-opened', () => webContent.closeDevTools())
  if (webContent.isDevToolsOpened()) webContent.closeDevTools()
}

let hasWindowCommandHandler = false

function handleWinCmd(win: BrowserWindow) {
  if (hasWindowCommandHandler) return
  hasWindowCommandHandler = true
  ipcMain.on('WebToWindow', (event, data) => {
    const currentWin = BrowserWindow.fromWebContents(event.sender) || win
    if (data.cmd && data.cmd === 'close') {
      event.returnValue = 'close'
      if (currentWin && !currentWin.isDestroyed()) currentWin.close()
    } else if (data.cmd && data.cmd === 'minsize') {
      event.returnValue = 'minsize'
      if (currentWin && !currentWin.isDestroyed()) currentWin.minimize()
    } else if (data.cmd && data.cmd === 'top') {
      if (currentWin && !currentWin.isDestroyed()) {
        if (currentWin.isAlwaysOnTop()) {
          event.returnValue = 'untop'
          currentWin.setAlwaysOnTop(false)
        } else {
          event.returnValue = 'top'
          currentWin.setAlwaysOnTop(true, 'status')
        }
      } else {
        event.returnValue = 'missing'
      }
    } else if (data.cmd && data.cmd === 'maxsize') {
      if (currentWin && !currentWin.isDestroyed()) {
        if (currentWin.isMaximized()) {
          event.returnValue = 'unmaximize'
          currentWin.unmaximize()
        } else {
          event.returnValue = 'maximize'
          currentWin.maximize()
        }
      } else {
        event.returnValue = 'missing'
      }
    } else if (data.cmd && data.cmd === 'fullscreen') {
      if (currentWin && !currentWin.isDestroyed()) {
        const isFullScreen = currentWin.isFullScreen()
        currentWin.setFullScreen(!isFullScreen)
        event.returnValue = isFullScreen ? 'unfullscreen' : 'fullscreen'
      } else {
        event.returnValue = 'missing'
      }
    } else if (data.cmd && data.cmd === 'enterfullscreen') {
      if (currentWin && !currentWin.isDestroyed()) {
        if (!currentWin.isFullScreen()) currentWin.setFullScreen(true)
        event.returnValue = 'fullscreen'
      } else {
        event.returnValue = 'missing'
      }
    } else if (data.cmd && data.cmd === 'exitfullscreen') {
      if (currentWin && !currentWin.isDestroyed()) {
        if (currentWin.isFullScreen()) currentWin.setFullScreen(false)
        event.returnValue = 'unfullscreen'
      } else {
        event.returnValue = 'missing'
      }
    } else {
      event.returnValue = 'unknown'
    }
  })
}

function creatUploadPort() {
  debounceUpload(function () {
    if (AppWindow.mainWindow && AppWindow.uploadWindow && AppWindow.uploadWindow.isDestroyed() == false) {
      const { port1, port2 } = new MessageChannelMain()
      AppWindow.mainWindow.webContents.postMessage('setUploadPort', undefined, [port1])
      AppWindow.uploadWindow.webContents.postMessage('setPort', undefined, [port2])
    }
  }, 1000)
}

ipcMain.on('ensureUploadWorker', () => {
  createUpload()
})

ipcMain.on('uploadWorkerReady', (event) => {
  if (event.sender === AppWindow.uploadWindow?.webContents) creatUploadPort()
})

function createUpload() {
  if (AppWindow.uploadWindow && AppWindow.uploadWindow.isDestroyed() == false) return
  AppWindow.uploadWindow = createElectronWindow(10, 10, false, 'worker', 'dark', false, false)
  AppWindow.uploadWindow.setTitle('mnemo上传进程')

  AppWindow.uploadWindow.webContents.on('render-process-gone', function (event, details) {
    if (details.reason == 'crashed' || details.reason == 'oom' || details.reason == 'killed' || details.reason == 'integrity-failure') {
      AppWindow.mainWindow?.webContents.send('clearUploadPort')
      try {
        AppWindow.uploadWindow?.destroy()
      } catch {}
      AppWindow.uploadWindow = undefined
      createUpload()
    }
  })
  // AppWindow.uploadWindow.webContents.openDevTools({ mode: 'undocked' })
  AppWindow.uploadWindow.hide()
}
