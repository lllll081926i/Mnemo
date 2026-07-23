import { AppWindow, createMainWindow, createTray } from './core/window'
import { app, ipcMain, session } from 'electron'
import { registerAutoUpdate } from './core/autoUpdate'
import is from 'electron-is'
import fixPath from 'fix-path'
import { release } from 'os'
import { getResourcesPath, getStaticPath } from './utils/mainfile'
import { existsSync, readFileSync, writeFileSync } from 'fs'
import { isAbsolute, join, resolve } from 'path'
import { EventEmitter } from 'node:events'
import exception from './core/exception'
import ipcEvent from './core/ipcEvent'
import MotrixApplication from './aria/MotrixApplication'

type UserToken = {
  access_token: string;
  open_api_access_token: string;
  user_id: string;
  tokenfrom?: string;
  refresh: boolean
}

const isHost = (hostname: string, domain: string) => hostname === domain || hostname.endsWith(`.${domain}`)

const parseRequestUrl = (url: string) => {
  try {
    const parsed = new URL(url)
    return { hostname: parsed.hostname.toLowerCase(), pathname: parsed.pathname }
  } catch {
    return { hostname: '', pathname: '' }
  }
}

export default class launch extends EventEmitter {
  private userToken: UserToken = {
    access_token: '',
    open_api_access_token: '',
    user_id: '',
    refresh: false
  }
  public motrixApp!: MotrixApplication

  constructor() {
    super()
    this.init()
  }

  init() {
    if (!is.mas()) {
      const gotSingleLock = app.requestSingleInstanceLock()
      if (!gotSingleLock) {
        app.exit()
        return
      }
      app.on('second-instance', (event, commandLine, workingDirectory) => {
        if (commandLine && commandLine.some((arg) => arg === 'exit' || arg === '--exit')) {
          this.hasExitArgv(commandLine)
          return
        }
        if (AppWindow.mainWindow && AppWindow.mainWindow.isDestroyed() == false) {
          if (AppWindow.mainWindow.isMinimized()) {
            AppWindow.mainWindow.restore()
          }
          AppWindow.mainWindow.show()
          AppWindow.mainWindow.focus()
        }
      })
    }
    this.start()
  }

  start() {
    exception.handler()
    this.setInitArgv()
    this.loadUserData()
    this.handleEvents()
    this.handleAppReady()
  }

  setInitArgv() {
    fixPath()
    if (release().startsWith('6.1')) {
      app.disableHardwareAcceleration()
    }
    app.commandLine.appendSwitch('wm-window-animations-disabled')
    app.commandLine.appendSwitch('enable-features', 'PlatformHEVCDecoderSupport')

    app.setName('Mnemo')
    process.title = 'Mnemo'
    if (is.windows()) app.setAppUserModelId('com.mnemo.app')
    this.hasExitArgv(process.argv)
  }

  hasExitArgv(args: string[]) {
    if (args && args.some((arg) => arg === 'exit' || arg === '--exit')) {
      app.exit()
    }
  }

  loadUserData() {
    const configPaths = [
      join(app.getPath('appData'), 'Mnemo', 'userdir.config'),
      getResourcesPath('userdir.config')
    ]
    for (const userData of configPaths) {
      try {
        if (!existsSync(userData)) continue
        const configData = readFileSync(userData, 'utf-8').trim()
        if (configData && isAbsolute(configData)) {
          app.setPath('userData', resolve(configData))
          return
        }
      } catch {
        // 尝试下一个配置位置
      }
    }
  }

  handleEvents() {
    ipcEvent.handleEvents()
    this.handleUserToken()
    this.handleAppActivate()
    this.handleAppWillQuit()
    this.handleAppWindowAllClosed()
  }

  handleAppReady() {
    app
      .whenReady()
      .then(() => {
        try {
          const localVersion = getResourcesPath('localVersion')
          if (localVersion && existsSync(localVersion)) {
            const version = readFileSync(localVersion, 'utf-8')
            if (app.getVersion() > version) {
              writeFileSync(localVersion, app.getVersion(), 'utf-8')
            }
          } else {
            writeFileSync(localVersion, app.getVersion(), 'utf-8')
          }
        } catch (err) {
        }
        session.defaultSession.webRequest.onBeforeSendHeaders((details, cb) => {
          const { hostname } = parseRequestUrl(details.url)
          const shouldBiliBili = isHost(hostname, 'bilibili.com')
          const shouldQQTv = hostname === 'v.qq.com' || hostname === 'video.qq.com'

          cb({
            cancel: false,
            requestHeaders: {
              ...details.requestHeaders,
              ...(shouldBiliBili && {
                Referer: 'https://www.bilibili.com/',
                Cookie: 'buvid_fp=4e5ab1b80f684b94efbf0d2f4721913e;buvid3=0679D9AB-1548-ED1E-B283-E0114517315E63379infoc;buvid4=990C4544-0943-1FBF-F13C-4C42A4EA97AA63379-024020214-83%2BAINcbQP917Ye0PjtrCg%3D%3D;',
                'user-agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36 Edg/121.0.0.0'
              }),
              ...(shouldQQTv && {
                Referer: 'https://m.v.qq.com/',
                Origin: 'https://m.v.qq.com',
                'user-agent': 'Mozilla/5.0 (Linux; Android 13; SM-G981B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Mobile Safari/537.36 Edg/121.0.0.0'
              }),
              'Accept-Language': 'zh-CN,zh;q=0.9'
            }
          })
        })
        this.motrixApp = new MotrixApplication()
        this.motrixApp.init().catch((err: any) => console.error('[MotrixApp] init failed', err))
        createMainWindow()
        createTray()
        registerAutoUpdate()

        if (!app.isPackaged) {
          setImmediate(() => {
            const defaultSessionExtensions = session.defaultSession.extensions
            const loadCrxExtension = defaultSessionExtensions?.loadExtension ? defaultSessionExtensions.loadExtension.bind(defaultSessionExtensions) : session.defaultSession.loadExtension.bind(session.defaultSession)
            loadCrxExtension(getStaticPath('crx'), { allowFileAccess: true }).catch((err: any) => {
              console.error('[launch] load crx extension failed', err)
            })
          })
        }
      })
      .catch((err: any) => {
        console.log(err)
      })
  }

  handleUserToken() {
    ipcMain.on('WebUserToken', (event, data) => {
      if (data.login) {
        this.userToken = data
      } else if (this.userToken.user_id == data.user_id) {
        this.userToken = data
      }
    })
  }

  handleAppActivate() {
    app.on('activate', () => {
      if (!AppWindow.mainWindow || AppWindow.mainWindow.isDestroyed()) createMainWindow()
      else {
        if (AppWindow.mainWindow.isMinimized()) AppWindow.mainWindow.restore()
        AppWindow.mainWindow.show()
        AppWindow.mainWindow.focus()
      }
    })
  }

  handleAppWillQuit() {
    let isQuitting = false
    app.on('will-quit', (event) => {
      if (isQuitting) return
      event.preventDefault()
      isQuitting = true
      void (async () => {
        try { await this.motrixApp?.quit() } catch {}
        try {
          if (AppWindow.appTray) {
            AppWindow.appTray.destroy()
            AppWindow.appTray = undefined
          }
        } catch {}
        app.quit()
      })()
    })
  }

  handleAppWindowAllClosed() {
    app.on('window-all-closed', () => {
      if (is.macOS()) {
        AppWindow.appTray?.destroy()
      } else {
        app.quit() // 未测试应该使用哪一个
      }
    })
  }

}
