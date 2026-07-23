import { AppWindow, createElectronWindow } from './window'
import path from 'path'
import is from 'electron-is'
import { app, BrowserWindow, dialog, ipcMain, Menu, net, powerSaveBlocker, safeStorage, session, shell } from 'electron'
import { cpSync, existsSync, mkdirSync, readFileSync, readdirSync, rmSync, statSync, writeFileSync } from 'fs'
import { execFile } from 'child_process'
import { ShowError } from './dialog'
import { getAsarPath, getStaticPath, getUserDataPath } from '../utils/mainfile'
import { createHash } from 'crypto'
import { getMotrixApplicationRpcPort, parseElectronProxyRules, syncMotrixApplicationProxy } from '../aria/runtime'
import { embeddedMpvBridge } from '../mpv/embeddedMpvBridge'
import { convert as convertOpenDataLoaderPdf, type ConvertOptions as OpenDataLoaderPdfOptions } from '@opendataloader/pdf'
import { checkForUpdatesNow } from './autoUpdate'
import { isOAuthAuthorizationUrl, oauthCallbackServer, type OAuthCallbackTarget, type OAuthProvider } from './oauthServer'

let psbId: any

interface OAuthSessionOwner {
  provider: OAuthProvider
  redirectUri: string
  senderId: number
  target: OAuthCallbackTarget
}

const oauthSessionOwners = new Map<string, OAuthSessionOwner>()
const oauthWindows = new Map<string, BrowserWindow>()
let pikpakCaptchaWindow: BrowserWindow | undefined
const ipcEventsRegisteredKey = Symbol.for('mnemo.ipc-events-registered')

const closeOAuthWindow = (state: string) => {
  const window = oauthWindows.get(state)
  oauthWindows.delete(state)
  if (window && !window.isDestroyed()) window.close()
}

const closePikPakCaptchaWindow = () => {
  const window = pikpakCaptchaWindow
  pikpakCaptchaWindow = undefined
  if (window && !window.isDestroyed()) window.close()
}

const isPikPakCaptchaUrl = (value: string) => {
  try {
    const url = new URL(value)
    const hostname = url.hostname.toLowerCase()
    const trustedHost = hostname === 'mypikpak.com' || hostname.endsWith('.mypikpak.com') || hostname === 'mypikpak.net' || hostname.endsWith('.mypikpak.net')
    return url.protocol === 'https:' && trustedHost
  } catch {
    return false
  }
}

// 挑战页可能落在盾服务商域名或经跨域跳转，只要 https 就允许在验证窗口内加载
const isHttpsUrl = (value: string) => {
  try {
    return new URL(value).protocol === 'https:'
  } catch {
    return false
  }
}

const isPikPakCaptchaCallback = (value: string) => {
  try {
    const url = new URL(value)
    return url.protocol === 'xlaccsdk01:' && url.hostname === 'xbase.cloud' && url.pathname === '/callback'
  } catch {
    return false
  }
}

// 部分验证完成后，回调地址会直接携带换发后的 captcha token
const extractPikPakCallbackToken = (value: string): string => {
  try {
    const url = new URL(value)
    for (const key of ['captcha_token', 'captchaToken', 'token']) {
      const token = url.searchParams.get(key)
      if (token && token.length > 20) return token
    }
  } catch {}
  return ''
}

const isPikPakCaptchaReportUrl = (value: string) => {
  if (!isPikPakCaptchaUrl(value)) return false
  try {
    const url = new URL(value)
    return url.pathname === '/credit/v1/report'
  } catch {
    return false
  }
}

const findCaptchaToken = (value: unknown, depth = 0): string => {
  if (!value || depth > 4 || typeof value !== 'object') return ''
  const record = value as Record<string, unknown>
  if (typeof record.captcha_token === 'string' && record.captcha_token.length > 20) return record.captcha_token
  for (const child of Object.values(record)) {
    const token = findCaptchaToken(child, depth + 1)
    if (token) return token
  }
  return ''
}

function buildOpenDataLoaderPdfOptions(data: any): OpenDataLoaderPdfOptions {
  const outputDir = String(data?.outputDir || '')
  const options: OpenDataLoaderPdfOptions = {
    outputDir,
    format: data?.format || 'json,html,markdown,text',
    quiet: true
  }
  const optionKeys: Array<keyof OpenDataLoaderPdfOptions> = [
    'password',
    'contentSafetyOff',
    'replaceInvalidChars',
    'tableMethod',
    'readingOrder',
    'markdownPageSeparator',
    'textPageSeparator',
    'htmlPageSeparator',
    'imageOutput',
    'imageFormat',
    'imageDir',
    'pages',
    'hybrid',
    'hybridMode',
    'hybridUrl',
    'hybridTimeout',
    'hybridHancomAiRegionlistStrategy',
    'hybridHancomAiOcrStrategy',
    'hybridHancomAiImageCache',
    'threads'
  ]
  for (const key of optionKeys) {
    const value = data?.[key]
    if (value !== undefined && value !== null && value !== '') (options as any)[key] = value
  }
  const booleanKeys: Array<keyof OpenDataLoaderPdfOptions> = ['sanitize', 'keepLineBreaks', 'useStructTree', 'markdownWithHtml', 'includeHeaderFooter', 'detectStrikethrough', 'hybridFallback', 'toStdout']
  for (const key of booleanKeys) {
    if (data?.[key] === true) (options as any)[key] = true
  }
  return options
}

export default class ipcEvent {
  private constructor() {}

  static handleEvents() {
    const runtime = globalThis as any
    if (runtime[ipcEventsRegisteredKey]) return
    this.handleWebToElectron()
    this.handleWebToElectronCB()
    this.handleShowContextMenu()
    this.handleWebShowOpenDialogSync()
    this.handleWebShowSaveDialogSync()
    this.handleWebShowItemInFolder()
    this.handleWebPlatformSync()
    this.handleAppPaths()
    this.handleWebSaveTheme()
    this.handleWebClearCookies()
    this.handleWebGetCookies()
    this.handleWebSetCookies()
    this.handleWebClearCache()
    this.handleWebReload()
    this.handleWebRelaunch()
    this.handleWebRelaunchAria()
    this.handleWebSetProgressBar()
    this.handleWebShutDown()
    this.handleWebSetProxy()
    this.handleWebOpenExternal()
    this.handlePikPakCaptcha()
    this.handleOAuthCallback()
    this.handleAutoUpdate()
    this.handleSafeStorage()
    this.handleOpenDataLoaderConvertPdf()
    this.handleWebOpenWindow()
    this.handlePowerSaveBlocker()
    this.handleMpvEmbedded()
    runtime[ipcEventsRegisteredKey] = true
  }

  private static handleAutoUpdate() {
    ipcMain.handle('AutoUpdate:check', () => checkForUpdatesNow())
  }

  private static handleWebOpenExternal() {
    ipcMain.handle('WebOpenExternal', async (_event, value: unknown) => {
      try {
        const url = new URL(String(value || '').trim())
        if (url.protocol !== 'https:' && url.protocol !== 'http:') return { ok: false, error: '仅允许打开 HTTP 或 HTTPS 链接' }
        await shell.openExternal(url.toString())
        return { ok: true }
      } catch (error: any) {
        return { ok: false, error: error?.message || '无法打开系统浏览器' }
      }
    })
  }

  private static handlePikPakCaptcha() {
    ipcMain.handle('PikPakCaptcha:open', async (event, value: unknown) => {
      const challengeUrl = String(value || '').trim()
      if (!isHttpsUrl(challengeUrl)) return { ok: false, error: 'PikPak 验证地址无效' }
      closePikPakCaptchaWindow()
      const captchaWindow = new BrowserWindow({
        parent: AppWindow.mainWindow,
        modal: true,
        show: true,
        width: 520,
        height: 720,
        minWidth: 420,
        minHeight: 560,
        title: 'PikPak 安全验证',
        icon: getStaticPath('icon_256x256.ico'),
        autoHideMenuBar: true,
        webPreferences: {
          nodeIntegration: false,
          contextIsolation: true,
          sandbox: true,
          devTools: false,
          webSecurity: true,
          partition: 'persist:pikpak-captcha'
        }
      })
      pikpakCaptchaWindow = captchaWindow
      captchaWindow.focus()
      captchaWindow.moveTop()
      let completed = false
      let callbackTimer: NodeJS.Timeout | undefined
      const reportRequests = new Set<string>()
      const completeCaptcha = (captchaToken = '') => {
        if (completed) return
        completed = true
        if (callbackTimer) clearTimeout(callbackTimer)
        if (!event.sender.isDestroyed()) event.sender.send('PikPakCaptcha:completed', { captchaToken })
        closePikPakCaptchaWindow()
      }
      const completeFromCallback = (callbackUrl: string) => {
        if (callbackTimer || completed) return
        const callbackToken = extractPikPakCallbackToken(callbackUrl)
        callbackTimer = setTimeout(() => completeCaptcha(callbackToken), 500)
      }
      const guardNavigation = (navigationEvent: Electron.Event, url: string) => {
        if (isPikPakCaptchaCallback(url)) {
          navigationEvent.preventDefault()
          completeFromCallback(url)
        } else if (!isHttpsUrl(url)) {
          navigationEvent.preventDefault()
        }
      }
      captchaWindow.webContents.on('will-navigate', guardNavigation)
      captchaWindow.webContents.on('will-redirect', guardNavigation)
      captchaWindow.webContents.setWindowOpenHandler(({ url }) => {
        if (isPikPakCaptchaCallback(url)) completeFromCallback(url)
        else if (isHttpsUrl(url)) void captchaWindow.loadURL(url)
        return { action: 'deny' }
      })
      const debuggerClient = captchaWindow.webContents.debugger
      try {
        debuggerClient.attach('1.3')
        debuggerClient.on('message', (_debuggerEvent, method, params) => {
          if (method === 'Network.responseReceived' && isPikPakCaptchaReportUrl(String(params?.response?.url || ''))) {
            reportRequests.add(String(params.requestId || ''))
            return
          }
          if (method !== 'Network.loadingFinished' || !reportRequests.delete(String(params?.requestId || ''))) return
          void debuggerClient.sendCommand('Network.getResponseBody', { requestId: params.requestId }).then((result: { body?: string; base64Encoded?: boolean }) => {
            const responseText = result.base64Encoded ? Buffer.from(result.body || '', 'base64').toString('utf8') : String(result.body || '')
            const captchaToken = findCaptchaToken(JSON.parse(responseText))
            if (captchaToken) completeCaptcha(captchaToken)
          }).catch(() => {})
        })
        await Promise.race([
          debuggerClient.sendCommand('Network.enable').catch(() => undefined),
          new Promise((resolve) => setTimeout(resolve, 300))
        ])
      } catch {}
      captchaWindow.once('ready-to-show', () => {
        captchaWindow.show()
        captchaWindow.focus()
        captchaWindow.moveTop()
      })
      captchaWindow.once('closed', () => {
        if (callbackTimer) clearTimeout(callbackTimer)
        if (debuggerClient.isAttached()) debuggerClient.detach()
        if (pikpakCaptchaWindow === captchaWindow) pikpakCaptchaWindow = undefined
      })
      try {
        await captchaWindow.loadURL(challengeUrl)
        if (!captchaWindow.isVisible()) captchaWindow.show()
        captchaWindow.focus()
        captchaWindow.moveTop()
        return { ok: true }
      } catch (error: any) {
        closePikPakCaptchaWindow()
        return { ok: false, error: error?.message || 'PikPak 验证页面加载失败' }
      }
    })
    ipcMain.handle('PikPakCaptcha:close', () => {
      closePikPakCaptchaWindow()
      return { ok: true }
    })
    ipcMain.handle('PikPakCaptcha:reset', async () => {
      closePikPakCaptchaWindow()
      // 清掉验证页面的持久会话（cookie/缓存的盾状态），配合新的设备 ID 全新开始
      await session.fromPartition('persist:pikpak-captcha').clearStorageData().catch(() => {})
      return { ok: true }
    })
  }

  private static handleOAuthCallback() {
    ipcMain.handle('OAuth:begin', async (event, value: { provider?: string }) => {
      try {
        const provider = String(value?.provider || '') as OAuthProvider
        const target: OAuthCallbackTarget = {
          isDestroyed: () => event.sender.isDestroyed(),
          send: (channel, payload) => {
            oauthSessionOwners.delete(payload.state)
            closeOAuthWindow(payload.state)
            if (!event.sender.isDestroyed()) event.sender.send(channel, payload)
          }
        }
        const session = await oauthCallbackServer.begin(provider, target)
        oauthSessionOwners.set(session.state, { provider, redirectUri: session.redirectUri, senderId: event.sender.id, target })
        return { ok: true, ...session }
      } catch (error: any) {
        return { ok: false, error: error?.message || 'OAuth 回调服务启动失败' }
      }
    })
    ipcMain.handle('OAuth:open', async (event, value: { state?: string; url?: string }) => {
      const state = String(value?.state || '')
      const owner = oauthSessionOwners.get(state)
      if (!owner || owner.senderId !== event.sender.id) return { ok: false, error: 'OAuth 会话不存在或已过期' }
      const authUrl = String(value?.url || '')
      if (!isOAuthAuthorizationUrl(owner.provider, authUrl, state, owner.redirectUri)) return { ok: false, error: 'OAuth 授权地址校验失败' }
      closeOAuthWindow(state)
      const authWindow = new BrowserWindow({
        parent: AppWindow.mainWindow,
        modal: true,
        show: false,
        width: 520,
        height: 720,
        minWidth: 420,
        minHeight: 560,
        title: `${owner.provider} 登录`,
        icon: getStaticPath('icon_256x256.ico'),
        autoHideMenuBar: true,
        webPreferences: {
          nodeIntegration: false,
          contextIsolation: true,
          sandbox: true,
          devTools: false,
          webSecurity: true
        }
      })
      oauthWindows.set(state, authWindow)
      authWindow.webContents.setWindowOpenHandler(() => ({ action: 'deny' }))
      authWindow.once('ready-to-show', () => authWindow.show())
      authWindow.once('closed', () => {
        if (oauthWindows.get(state) !== authWindow) return
        oauthWindows.delete(state)
        oauthSessionOwners.delete(state)
        oauthCallbackServer.cancel(state, owner.target)
        if (!event.sender.isDestroyed()) {
          event.sender.send('OAuth:callback', { provider: owner.provider, state, code: '', error: 'window_closed', errorDescription: '登录窗口已关闭' })
        }
      })
      try {
        await authWindow.loadURL(authUrl)
        return { ok: true }
      } catch (error: any) {
        closeOAuthWindow(state)
        oauthSessionOwners.delete(state)
        oauthCallbackServer.cancel(state, owner.target)
        return { ok: false, error: error?.message || '加载 OAuth 登录页面失败' }
      }
    })
    ipcMain.handle('OAuth:cancel', (event, value: { state?: string }) => {
      const state = String(value?.state || '')
      const owner = oauthSessionOwners.get(state)
      if (!owner || owner.senderId !== event.sender.id) return { ok: false }
      oauthSessionOwners.delete(state)
      closeOAuthWindow(state)
      return { ok: oauthCallbackServer.cancel(state, owner.target) }
    })
  }

  private static handleSafeStorage() {
    ipcMain.handle('SafeStorage:encrypt', (_event, value: unknown) => {
      if (!safeStorage.isEncryptionAvailable()) throw new Error('系统安全存储不可用')
      return safeStorage.encryptString(String(value ?? '')).toString('base64')
    })
    ipcMain.handle('SafeStorage:decrypt', (_event, value: unknown) => {
      if (!safeStorage.isEncryptionAvailable()) throw new Error('系统安全存储不可用')
      return safeStorage.decryptString(Buffer.from(String(value || ''), 'base64'))
    })
    ipcMain.on('SafeStorage:encryptSync', (event, value: unknown) => {
      try {
        if (!safeStorage.isEncryptionAvailable()) throw new Error('系统安全存储不可用')
        event.returnValue = { ok: true, value: safeStorage.encryptString(String(value ?? '')).toString('base64') }
      } catch (error: any) {
        event.returnValue = { ok: false, error: error?.message || '加密失败' }
      }
    })
    ipcMain.on('SafeStorage:decryptSync', (event, value: unknown) => {
      try {
        if (!safeStorage.isEncryptionAvailable()) throw new Error('系统安全存储不可用')
        event.returnValue = { ok: true, value: safeStorage.decryptString(Buffer.from(String(value || ''), 'base64')) }
      } catch (error: any) {
        event.returnValue = { ok: false, error: error?.message || '解密失败' }
      }
    })
  }

  private static handleMpvEmbedded() {
    ipcMain.handle('MpvEmbedded:getCapability', async () => embeddedMpvBridge.getCapability())
    ipcMain.handle('MpvEmbedded:load', async (event, data) => embeddedMpvBridge.load(data || {}, event.sender))
    ipcMain.handle('MpvEmbedded:control', async (_event, data) => embeddedMpvBridge.control(data || {}))
    ipcMain.handle('MpvEmbedded:getStatus', async () => embeddedMpvBridge.getStatus())
  }

  private static handleWebToElectron() {
    ipcMain.on('WebToElectron', async (event, data) => {
      let mainWindow = AppWindow.mainWindow
      if (data.cmd && data.cmd === 'close') {
        if (mainWindow && !mainWindow.isDestroyed()) mainWindow.hide()
      } else if (data.cmd && data.cmd === 'relaunch') {
        if (mainWindow && !mainWindow.isDestroyed()) {
          mainWindow.destroy()
          mainWindow = undefined
        }
        try {
          app.relaunch({ args: process.argv.slice(1).concat(['--relaunch']) })
          app.exit(0)
        } catch {}
      } else if (data.cmd && data.cmd === 'exit') {
        if (mainWindow && !mainWindow.isDestroyed()) {
          mainWindow.destroy()
          mainWindow = undefined
        }
        try {
          app.exit(0)
        } catch {}
      } else if (data.cmd && data.cmd === 'minsize') {
        if (mainWindow && !mainWindow.isDestroyed()) mainWindow.minimize()
      } else if (data.cmd && data.cmd === 'maxsize') {
        if (mainWindow && !mainWindow.isDestroyed()) {
          if (mainWindow.isMaximized()) {
            mainWindow.unmaximize()
          } else {
            mainWindow.maximize()
          }
        }
      } else if (data.cmd && (Object.hasOwn(data.cmd, 'launchStart') || Object.hasOwn(data.cmd, 'launchStartShow'))) {
        const launchStart = data.cmd.launchStart
        const launchStartShow = data.cmd.launchStartShow
        const appName = path.basename(process.execPath)
        // 设置开机自启
        const settings: Electron.Settings = {
          openAtLogin: launchStart,
          path: process.execPath
        }
        // 显示主窗口
        if (is.macOS()) {
          settings.openAsHidden = !launchStartShow
        } else {
          settings.args = ['--processStart', `${appName}`, '--process-start-args', `"--hidden"`]
          !launchStartShow && settings.args.push('--openAsHidden')
        }
        app.setLoginItemSettings(settings)
      } else if (data.cmd && data.cmd === 'preventSleep') {
        if (data.flag) {
          if (psbId && powerSaveBlocker.isStarted(psbId)) {
            return
          }
          psbId = powerSaveBlocker.start('prevent-app-suspension')
        } else {
          if (typeof psbId === 'undefined' || !powerSaveBlocker.isStarted(psbId)) {
            return
          }
          powerSaveBlocker.stop(psbId)
          psbId = undefined
        }
      } else if (data.cmd && data.cmd === 'downloadProgress') {
        const win = AppWindow.mainWindow
        if (win && !win.isDestroyed()) {
          const progress = typeof data.progress === 'number' ? data.progress : -1
          win.setProgressBar(progress)
        }
      } else if (data.cmd && data.cmd === 'downloadCompleted') {
        const { Notification } = require('electron') as typeof import('electron')
        if (Notification.isSupported()) {
          const n = new Notification({
            title: '下载完成',
            body: data.fileName ? `${data.fileName} 已下载完成` : '文件下载完成'
          })
          n.on('click', () => {
            AppWindow.mainWindow?.show()
            AppWindow.mainWindow?.focus()
          })
          n.show()
        }
      } else {
        event.sender.send('ElectronToWeb', 'mainsenddata')
      }
    })
  }

  private static handleWebToElectronCB() {
    ipcMain.on('WebToElectronCB', (event, data) => {
      const mainWindow = AppWindow.mainWindow
      if (data.cmd && data.cmd === 'maxsize') {
        if (mainWindow && !mainWindow.isDestroyed()) {
          if (mainWindow.isMaximized()) {
            mainWindow.unmaximize()
            event.returnValue = 'unmaximize'
          } else {
            mainWindow.maximize()
            event.returnValue = 'maximize'
          }
        }
      } else {
        event.returnValue = 'backdata'
      }
    })
  }

  private static handleShowContextMenu() {
    ipcMain.on('show-context-menu', (event, params) => {
      const { showCut, showCopy, showPaste } = params
      const window = BrowserWindow.fromWebContents(event.sender)
      // 制作右键菜单
      let template: Array<Electron.MenuItemConstructorOptions> = [
        // 设置选项是否可见
        { role: 'selectAll', label: '全选' },
        { role: 'copy', label: '复制', visible: showCopy },
        { role: 'cut', label: '剪切', visible: showCut },
        { role: 'paste', label: '粘贴', visible: showPaste },
        { role: 'undo', label: '撤销' }
      ]
      // 显示菜单
      const contextMenu = Menu.buildFromTemplate(template)
      contextMenu.popup({ window })
    })
  }

  private static handleWebShowOpenDialogSync() {
    ipcMain.on('WebShowOpenDialogSync', (event, config) => {
      event.returnValue = dialog.showOpenDialogSync(AppWindow.mainWindow!, config) || []
    })
  }

  private static handleWebShowSaveDialogSync() {
    ipcMain.on('WebShowSaveDialogSync', (event, config) => {
      event.returnValue = dialog.showSaveDialogSync(AppWindow.mainWindow!, config) || ''
    })
  }

  private static handleWebShowItemInFolder() {
    ipcMain.on('WebShowItemInFolder', (event, fullPath) => {
      for (let i = 0; i < 5; i++) {
        if (existsSync(fullPath)) break
        if (fullPath.lastIndexOf(path.sep) > 0) {
          fullPath = fullPath.substring(0, fullPath.lastIndexOf(path.sep))
        } else return
      }
      if (fullPath.length > 2) shell.showItemInFolder(fullPath)
    })
  }

  private static handleWebPlatformSync() {
    ipcMain.on('WebPlatformSync', (event) => {
      const asarPath = app.getAppPath()
      const appPath = app.getPath('userData')
      event.returnValue = {
        platform: process.platform,
        arch: process.arch,
        version: process.version,
        execPath: process.execPath,
        appPath: appPath,
        userDataPath: appPath,
        downloadsPath: app.getPath('downloads'),
        defaultUserDataPath: path.join(app.getPath('appData'), 'Mnemo'),
        asarPath: asarPath,
        argv0: process.argv0
      }
    })
  }

  private static handleAppPaths() {
    ipcMain.handle('AppPaths:get', () => ({
      userDataPath: app.getPath('userData'),
      downloadsPath: app.getPath('downloads'),
      defaultUserDataPath: path.join(app.getPath('appData'), 'Mnemo')
    }))
    ipcMain.handle('AppPaths:setUserData', (_event, value: unknown) => {
      const target = String(value || '').trim()
      const defaultUserDataPath = path.resolve(path.join(app.getPath('appData'), 'Mnemo'))
      const configPath = path.join(defaultUserDataPath, 'userdir.config')
      const currentUserDataPath = path.resolve(app.getPath('userData'))
      if (target && !path.isAbsolute(target)) return { ok: false, error: '请选择完整的用户数据目录路径' }
      const normalizeForCompare = (value: string) => {
        const resolvedValue = path.resolve(value)
        return process.platform === 'win32' ? resolvedValue.toLowerCase() : resolvedValue
      }
      const samePath = (left: string, right: string) => normalizeForCompare(left) === normalizeForCompare(right)
      const selectedPath = target ? path.resolve(target) : defaultUserDataPath
      const resolved = samePath(selectedPath, defaultUserDataPath) ? defaultUserDataPath : selectedPath
      const isNestedPath = (candidate: string, parent: string) => {
        const relative = path.relative(normalizeForCompare(parent), normalizeForCompare(candidate))
        return !!relative && !relative.startsWith('..' + path.sep) && relative !== '..' && !path.isAbsolute(relative)
      }
      if (path.parse(resolved).root === resolved) return { ok: false, error: '请选择具体文件夹，不能把磁盘根目录作为用户数据目录' }
      if (!samePath(resolved, currentUserDataPath) && (isNestedPath(resolved, currentUserDataPath) || isNestedPath(currentUserDataPath, resolved))) {
        return { ok: false, error: '新的用户数据目录不能与当前目录互相包含，请选择其他文件夹' }
      }
      try {
        mkdirSync(resolved, { recursive: true })
        if (!samePath(resolved, currentUserDataPath) && existsSync(currentUserDataPath)) {
          const skippedNames = new Set(['singletoncookie', 'singletonlock', 'singletonsocket', 'lock', 'lockfile', 'engine.pid'])
          const skippedDirectories = new Set(['cache', 'code cache', 'gpucache', 'dawncache', 'shadercache', 'grshadercache', 'crashpad'])
          cpSync(currentUserDataPath, resolved, {
            recursive: true,
            force: true,
            errorOnExist: false,
            filter: (source) => {
              if (samePath(source, configPath)) return false
              const relative = path.relative(currentUserDataPath, source)
              if (!relative) return true
              const parts = relative.split(path.sep).map((part) => part.toLowerCase())
              return !parts.some((part) => skippedDirectories.has(part)) && !skippedNames.has(path.basename(source).toLowerCase())
            }
          })
        }
        mkdirSync(path.dirname(configPath), { recursive: true })
        writeFileSync(configPath, resolved, 'utf-8')
        return { ok: true, path: resolved, requiresRestart: !samePath(resolved, app.getPath('userData')) }
      } catch (error: any) {
        return { ok: false, error: error?.message ? `无法移动用户数据：${error.message}` : '无法移动用户数据，请检查文件夹权限和磁盘剩余空间' }
      }
    })
  }

  private static handleWebSaveTheme() {
    ipcMain.on('WebSaveTheme', (event, data) => {
      try {
        const themeJson = getUserDataPath('theme.json')
        writeFileSync(themeJson, `{"theme":"${data.theme || ''}"}`, 'utf-8')
      } catch {}
    })
  }

  private static handleWebClearCookies() {
    ipcMain.on('WebClearCookies', (event, data) => {
      session.defaultSession.clearStorageData(data)
    })
  }

  private static handleWebGetCookies() {
    ipcMain.handle('WebGetCookies', async (event, data) => {
      return await session.defaultSession.cookies.get(data)
    })
  }

  private static handleWebSetCookies() {
    ipcMain.on('WebSetCookies', (event, data) => {
      for (let i = 0, maxi = data.length; i < maxi; i++) {
        const cookie = {
          url: data[i].url,
          name: data[i].name,
          value: data[i].value,
          domain: '.' + data[i].url.substring(data[i].url.lastIndexOf('/') + 1),
          secure: data[i].url.indexOf('https://') == 0,
          expirationDate: data[i].expirationDate
        }
        session.defaultSession.cookies.set(cookie).catch((err: any) => console.error(err))
      }
    })
  }

  private static handleWebClearCache() {
    ipcMain.on('WebClearCache', (event, data) => {
      if (data.cache) {
        session.defaultSession.clearCache()
        session.defaultSession.clearAuthCache()
      } else {
        session.defaultSession.clearStorageData(data)
      }
    })
  }

  private static handleWebReload() {
    ipcMain.on('WebReload', (event, data) => {
      if (AppWindow.mainWindow && !AppWindow.mainWindow.isDestroyed()) AppWindow.mainWindow.reload()
    })
  }

  private static handleWebRelaunch() {
    ipcMain.on('WebRelaunch', (event, data) => {
      app.relaunch()
      try {
        app.exit()
      } catch {}
    })
  }

  private static handleWebRelaunchAria() {
    ipcMain.handle('WebRelaunchAria', async (event, data) => {
      await (globalThis as any).motrixApplication?.ensureEngineReady?.()
      return getMotrixApplicationRpcPort()
    })
  }

  private static handleWebSetProgressBar() {
    ipcMain.on('WebSetProgressBar', (event, data) => {
      if (AppWindow.mainWindow && !AppWindow.mainWindow.isDestroyed()) {
        if (data.pro) {
          AppWindow.mainWindow.setProgressBar(data.pro, { mode: data.mode || 'normal' })
        } else AppWindow.mainWindow.setProgressBar(-1)
      }
    })
  }

  private static handleWebShutDown() {
    ipcMain.on('WebShutDown', (event, data) => {
      if (is.macOS()) {
        execFile('osascript', ['-e', 'tell application "System Events" to shut down'], (err: any) => {
          if (data.quitApp) {
            try {
              app.exit()
            } catch {}
          }
          if (err) {
            // donothing
          }
        })
      } else {
        let command = 'shutdown'
        const cmdArguments: string[] = []
        if (is.linux()) {
          if (data.sudo) {
            command = 'sudo'
            cmdArguments.push('shutdown')
          }
          cmdArguments.push('-h')
          cmdArguments.push('now')
        }
        if (is.windows()) {
          cmdArguments.push('/s', '/f', '/t', '0')
        }

        execFile(command, cmdArguments, (err: any) => {
          if (data.quitApp) {
            try {
              app.exit()
            } catch {}
          }
          if (err) {
            // donothing
          }
        })
      }
    })
  }

  private static handleWebSetProxy() {
    ipcMain.handle('WebSetProxy', async (_event, data: { mode?: 'system' | 'direct' | 'fixed_servers'; proxyUrl?: string }) => {
      const mode = data?.mode
      let ariaProxy = ''
      if (mode === 'fixed_servers' && data.proxyUrl) {
        await session.defaultSession.setProxy({ mode: 'fixed_servers', proxyRules: data.proxyUrl })
        ariaProxy = data.proxyUrl
      } else if (mode === 'direct') {
        await session.defaultSession.setProxy({ mode: 'direct' })
      } else {
        await session.defaultSession.setProxy({ mode: 'system' })
        ariaProxy = parseElectronProxyRules(await session.defaultSession.resolveProxy('https://api.aliyundrive.com'))
      }
      await session.defaultSession.closeAllConnections()
      await syncMotrixApplicationProxy(ariaProxy)
      return true
    })
  }

  private static handleOpenDataLoaderConvertPdf() {
    const handler = async (_event: any, data: any) => {
      try {
        const inputPath = String(data?.inputPath || '')
        if (!inputPath) return { ok: false, error: '请选择要转换的 PDF 文件' }
        const baseName = path.basename(inputPath, path.extname(inputPath))
        const hash = createHash('sha1').update(inputPath).digest('hex').slice(0, 12)
        const outputDir = path.join(app.getPath('userData'), 'pdf-tools-output', `${baseName}-${hash}`)
        try {
          rmSync(outputDir, { recursive: true, force: true })
        } catch {}
        mkdirSync(outputDir, { recursive: true })
        const options = buildOpenDataLoaderPdfOptions({ ...data, outputDir })
        const stdout = await convertOpenDataLoaderPdf(inputPath, options)
        let body: any = stdout.trim()
        try {
          body = stdout ? JSON.parse(stdout) : {}
        } catch {}
        const fmtMap: { [k: string]: 'json' | 'html' | 'md' | 'text' } = {
          '.json': 'json',
          '.html': 'html',
          '.htm': 'html',
          '.md': 'md',
          '.markdown': 'md',
          '.txt': 'text'
        }
        const contents: { json: any; html: string; md: string; text: string; files: any[] } = {
          json: null,
          html: '',
          md: '',
          text: '',
          files: []
        }
        const walk = (dir: string): string[] => {
          const out: string[] = []
          try {
            for (const name of readdirSync(dir)) {
              const fp = path.join(dir, name)
              try {
                const st = statSync(fp)
                if (st.isDirectory()) out.push(...walk(fp))
                else if (st.isFile()) out.push(fp)
              } catch {}
            }
          } catch {}
          return out
        }
        const allFiles = walk(outputDir)
        for (const filePath of allFiles) {
          const ext = path.extname(filePath).toLowerCase()
          const kind = fmtMap[ext]
          const entry: any = { path: filePath, name: path.basename(filePath), format: kind || ext.slice(1) || 'unknown' }
          contents.files.push(entry)
          if (!kind) continue
          try {
            const text = readFileSync(filePath, 'utf8')
            entry.size = text.length
            if (kind === 'json' && !contents.json) {
              try {
                contents.json = JSON.parse(text)
              } catch {
                contents.json = text
              }
            } else if (kind === 'html' && !contents.html) contents.html = text
            else if (kind === 'md' && !contents.md) contents.md = text
            else if (kind === 'text' && !contents.text) contents.text = text
          } catch (e: any) {
            console.warn('[opendataloader:convertPdf] read file failed', filePath, e?.message)
          }
        }

        const merged = body && typeof body === 'object' ? { ...body, ...contents } : contents
        if (!contents.json && !contents.html && !contents.md && !contents.text) {
          console.warn('[opendataloader:convertPdf] empty output, files=', allFiles, 'stdout=', stdout.slice(0, 500))
        }
        return { ok: true, result: merged }
      } catch (err: any) {
        console.error('[opendataloader:convertPdf] error', err)
        return { ok: false, error: err?.message || 'OpenDataLoader PDF 转换失败' }
      }
    }
    ipcMain.handle('opendataloader:convertPdf', handler)
    ipcMain.handle('OpenDataLoader:convertPdf', handler)
  }

  private static handleWebOpenWindow() {
    let winWidth = AppWindow.winWidth
    if (winWidth < 1080) winWidth = 1080
    ipcMain.on('WebOpenWindow', (event, data) => {
      const win = createElectronWindow(winWidth, AppWindow.winHeight, true, 'main2', data.theme, true)
      win.on('ready-to-show', function () {
        win.webContents.send('setPage', data)
        win.setTitle('预览窗口')
        win.show()
      })
    })
  }

  private static powerSaveBlockerId: number | null = null

  private static handlePowerSaveBlocker() {
    ipcMain.on('setPowerSaveBlocker', (_event, enabled: boolean) => {
      if (enabled) {
        if (ipcEvent.powerSaveBlockerId === null) {
          ipcEvent.powerSaveBlockerId = powerSaveBlocker.start('prevent-display-sleep')
        }
      } else {
        if (ipcEvent.powerSaveBlockerId !== null) {
          powerSaveBlocker.stop(ipcEvent.powerSaveBlockerId)
          ipcEvent.powerSaveBlockerId = null
        }
      }
    })
  }

}
