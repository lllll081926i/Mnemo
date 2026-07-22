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
    this.start()
    if (is.mas()) return
    const gotSingleLock = app.requestSingleInstanceLock()
    if (!gotSingleLock) {
      app.exit()
    } else {
      app.on('second-instance', (event, commandLine, workingDirectory) => {
        if (commandLine && commandLine.join(' ').indexOf('exit') >= 0) {
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
    app.commandLine.appendSwitch('ignore-connections-limit', 'bj29-enet.cn-beijing.data.alicloudccp.com,bj29-hz.cn-hangzhou.data.alicloudccp.com,bj29.cn-beijing.data.alicloudccp.com,alicloudccp.com,api.aliyundrive.com,aliyundrive.com,api.alipan.com,alipan.com')
    app.commandLine.appendSwitch('wm-window-animations-disabled')
    app.commandLine.appendSwitch('enable-features', 'PlatformHEVCDecoderSupport')

    app.name = 'Mnemo'
    if (is.windows()) {
      app.setAppUserModelId('com.mnemo.app')
    }
    this.hasExitArgv(process.argv)
  }

  hasExitArgv(args) {
    if (args && args.join(' ').indexOf('exit') >= 0) {
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
          const { hostname, pathname } = parseRequestUrl(details.url)
          const shouldGieeReferer = isHost(hostname, 'gitee.com')
          const shouldBiliBili = isHost(hostname, 'bilibili.com')
          const shouldQQTv = hostname === 'v.qq.com' || hostname === 'video.qq.com'
          const shouldAliPanOrigin = isHost(hostname, 'aliyundrive.com') || isHost(hostname, 'alipan.com')
          const shouldAliReferer = !shouldQQTv && !shouldBiliBili && !shouldGieeReferer && (!details.referrer || details.referrer.trim() === '' || /(\/localhost:)|(^file:\/\/)|(\/127.0.0.1:)/.exec(details.referrer) !== null)
          const shouldToken = shouldAliPanOrigin && pathname.includes('download')
          const shouldOpenApiToken = shouldAliPanOrigin && (pathname.includes('/adrive/v1.0') || pathname.includes('/adrive/v1.1'))
          const forbidUrl = details.url.includes('younoyes') || details.url.includes('onatoshi')
          const hasAuthorizationHeader = Object.keys(details.requestHeaders || {}).some((key) => key.toLowerCase() === 'authorization')
          const fallbackAccessToken = this.userToken?.access_token || ''
          const fallbackOpenApiToken = this.userToken?.open_api_access_token || ''
          const baseRequestHeaders = { ...details.requestHeaders }

          cb({
            cancel: false,
            requestHeaders: {
              ...baseRequestHeaders,
              ...(shouldGieeReferer && {
                Referer: 'https://gitee.com/',
                'user-agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36 Edg/121.0.0.0'
              }),
              ...(shouldAliPanOrigin && {
                Origin: 'https://www.aliyundrive.com',
                'user-agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36 Edg/121.0.0.0'
              }),
              ...(shouldAliReferer && {
                Referer: 'https://www.aliyundrive.com/',
                'user-agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36 Edg/121.0.0.0'
              }),
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
              ...(forbidUrl && {
                'user-agent': 'SenPlayer'
              }),
              ...(shouldToken && !hasAuthorizationHeader && fallbackAccessToken && {
                Authorization: fallbackAccessToken,
                'user-agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36 Edg/121.0.0.0'
              }),
              ...(shouldOpenApiToken && !hasAuthorizationHeader && fallbackOpenApiToken && {
                Authorization: 'Bearer ' + fallbackOpenApiToken,
                'user-agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36 Edg/121.0.0.0'
              }),
              ...(shouldAliPanOrigin && {
                'X-Canary': 'client=windows,app=adrive,version=v4.12.0'
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
    app.on('will-quit', async () => {
      try { await this.motrixApp?.quit() } catch {}
      try {
        if (AppWindow.appTray) {
          AppWindow.appTray.destroy()
          AppWindow.appTray = undefined
        }
      } catch {}
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
