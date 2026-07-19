import { AppWindow, createElectronWindow } from './window'
import path from 'path'
import is from 'electron-is'
import { app, BrowserWindow, dialog, ipcMain, Menu, net, powerSaveBlocker, safeStorage, session, shell } from 'electron'
import { existsSync, mkdirSync, readFileSync, readdirSync, rmSync, statSync, writeFileSync } from 'fs'
import { execFile } from 'child_process'
import { ShowError } from './dialog'
import { getAsarPath, getStaticPath, getUserDataPath } from '../utils/mainfile'
import { createHash } from 'crypto'
import { getMotrixApplicationRpcPort, parseElectronProxyRules, syncMotrixApplicationProxy } from '../aria/runtime'
import { embeddedMpvBridge } from '../mpv/embeddedMpvBridge'
import { convert as convertOpenDataLoaderPdf, type ConvertOptions as OpenDataLoaderPdfOptions } from '@opendataloader/pdf'
import { checkForUpdatesNow } from './autoUpdate'

let psbId: any

const QUARK_DOWNLOAD_AGENT = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) quark-cloud-drive/2.5.56 Chrome/100.0.4896.160 Electron/18.3.5.12-a038f7b798 Safari/537.36 Channel/pckk_other_ch'

function pathToFileUrl(filePath: string): string {
  const normalized = path.resolve(filePath).replace(/\\/g, '/')
  return 'file://' + (normalized.startsWith('/') ? normalized : '/' + normalized)
}

function findSoffice(): string {
  const candidates = [
    process.env.LIBREOFFICE_PATH || '',
    process.env.SOFFICE_PATH || '',
    is.macOS() ? '/Applications/LibreOffice.app/Contents/MacOS/soffice' : '',
    is.windows() ? 'C:\\Program Files\\LibreOffice\\program\\soffice.exe' : '',
    is.windows() ? 'C:\\Program Files (x86)\\LibreOffice\\program\\soffice.exe' : '',
    '/usr/bin/libreoffice',
    '/usr/local/bin/libreoffice',
    '/opt/homebrew/bin/libreoffice',
    '/usr/bin/soffice',
    '/usr/local/bin/soffice',
    '/opt/homebrew/bin/soffice'
  ].filter(Boolean)
  for (const candidate of candidates) {
    if (existsSync(candidate)) return candidate
  }
  const names = is.windows() ? ['soffice.exe', 'libreoffice.exe'] : ['soffice', 'libreoffice']
  const pathDirs = (process.env.PATH || '').split(path.delimiter).filter(Boolean)
  for (const dir of pathDirs) {
    for (const name of names) {
      const candidate = path.join(dir, name)
      if (existsSync(candidate)) return candidate
    }
  }
  return ''
}

function convertOfficeFileToPdf(soffice: string, inputPath: string, outDir: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const userInstall = path.join(outDir, 'lo-profile').replace(/\\/g, '/')
    const args = ['--headless', '--nologo', '--nofirststartwizard', `-env:UserInstallation=file://${userInstall}`, '--convert-to', 'pdf', '--outdir', outDir, inputPath]
    execFile(soffice, args, { windowsHide: true }, (err, stdout, stderr) => {
      if (err) {
        reject(new Error((stderr || stdout || err.message || '').trim() || 'LibreOffice 转换失败'))
      } else {
        resolve()
      }
    })
  })
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
    this.handleWebToElectron()
    this.handleWebToElectronCB()
    this.handleShowContextMenu()
    this.handleWebShowOpenDialogSync()
    this.handleWebShowSaveDialogSync()
    this.handleWebShowItemInFolder()
    this.handleWebPlatformSync()
    this.handleWebSaveTheme()
    this.handleWebClearCookies()
    this.handleWebGetCookies()
    this.handleWebQuarkAccountInfo()
    this.handleWebQuarkDownloadUrl()
    this.handleWebSetCookies()
    this.handleWebClearCache()
    this.handleWebReload()
    this.handleWebRelaunch()
    this.handleWebRelaunchAria()
    this.handleWebSetProgressBar()
    this.handleWebShutDown()
    this.handleWebSetProxy()
    this.handleWebOpenExternal()
    this.handleAutoUpdate()
    this.handleSafeStorage()
    this.handleOfficePreviewConvertToPdf()
    this.handleOpenDataLoaderConvertPdf()
    this.handleWebOpenWindow()
    this.handlePowerSaveBlocker()
    this.handleMpvEmbedded()
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
      dialog.showOpenDialog(AppWindow.mainWindow!, config).then((result) => {
        event.returnValue = result.filePaths
      })
    })
  }

  private static handleWebShowSaveDialogSync() {
    ipcMain.on('WebShowSaveDialogSync', (event, config) => {
      dialog.showSaveDialog(AppWindow.mainWindow!, config).then((result) => {
        event.returnValue = result.filePath || ''
      })
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
        asarPath: asarPath,
        argv0: process.argv0
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

  private static handleWebQuarkAccountInfo() {
    ipcMain.handle('WebQuarkAccountInfo', async (_event, data: { serviceTicket?: string }) => {
      const serviceTicket = String(data?.serviceTicket || '')
      if (!serviceTicket) return { ok: false, status: 0, body: '', cookies: [], error: '夸克登录凭证为空' }
      const params = new URLSearchParams({ st: serviceTicket, lw: 'scan' })
      const url = `https://pan.quark.cn/account/info?${params.toString()}`
      const existingCookies = await session.defaultSession.cookies.get({ domain: 'quark.cn' })
      const cookieHeader = existingCookies
        .filter((cookie) => cookie.name && cookie.value)
        .map((cookie) => `${cookie.name}=${cookie.value}`)
        .join('; ')

      return await new Promise((resolve) => {
        const request = net.request({
          method: 'GET',
          url,
          useSessionCookies: true
        } as any)
        request.setHeader('Accept', 'application/json, text/plain, */*')
        request.setHeader('Accept-Language', 'zh-CN,zh;q=0.9')
        request.setHeader('Cache-Control', 'no-cache')
        request.setHeader('Pragma', 'no-cache')
        request.setHeader('Origin', 'https://pan.quark.cn')
        request.setHeader('Referer', 'https://pan.quark.cn/')
        request.setHeader('User-Agent', 'Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/94.0.4606.71 Safari/537.36 Core/1.94.225.400 QQBrowser/12.2.5544.400')
        if (cookieHeader) request.setHeader('Cookie', cookieHeader)
        request.on('response', (response) => {
          const chunks: Buffer[] = []
          response.on('data', (chunk) => chunks.push(Buffer.from(chunk)))
          response.on('end', async () => {
            const body = Buffer.concat(chunks).toString('utf8')
            const setCookie = response.headers['set-cookie']
            const setCookieList = Array.isArray(setCookie) ? setCookie : setCookie ? [String(setCookie)] : []
            for (const rawCookie of setCookieList) {
              const cookie = ipcEvent.parseSetCookie(rawCookie, 'https://pan.quark.cn')
              if (cookie) {
                try {
                  await session.defaultSession.cookies.set(cookie)
                } catch (err) {
                  console.error(err)
                }
              }
            }
            const cookies = await session.defaultSession.cookies.get({ domain: 'quark.cn' })
            resolve({ ok: response.statusCode >= 200 && response.statusCode < 300, status: response.statusCode, body, cookies })
          })
        })
        request.on('error', (error) => resolve({ ok: false, status: 0, body: '', cookies: [], error: error.message }))
        request.end()
      })
    })
  }

  private static handleWebQuarkDownloadUrl() {
    ipcMain.handle('WebQuarkDownloadUrl', async (_event, data: { fileId?: string; cookie?: string }) => {
      const fileId = String(data?.fileId || '')
      if (!fileId) return { ok: false, status: 0, body: '', cookies: [], error: '夸克文件 ID 为空' }
      const params = new URLSearchParams({
        pr: 'ucpro',
        fr: 'pc',
        sys: 'win32',
        ve: '2.5.56',
        ut: '',
        guid: ''
      })
      const url = `https://drive-pc.quark.cn/1/clouddrive/file/download?${params.toString()}`
      const existingCookies = await session.defaultSession.cookies.get({ domain: 'quark.cn' })
      const cookieHeader =
        String(data?.cookie || '') ||
        existingCookies
          .filter((cookie) => cookie.name && cookie.value)
          .map((cookie) => `${cookie.name}=${cookie.value}`)
          .join('; ')

      return await new Promise((resolve) => {
        const request = net.request({
          method: 'POST',
          url,
          useSessionCookies: true
        } as any)
        request.setHeader('Accept', 'application/json, text/plain, */*')
        request.setHeader('Accept-Language', 'zh-CN,zh;q=0.9')
        request.setHeader('Content-Type', 'application/json')
        request.setHeader('Origin', 'https://pan.quark.cn')
        request.setHeader('Referer', 'https://pan.quark.cn/')
        request.setHeader('User-Agent', QUARK_DOWNLOAD_AGENT)
        if (cookieHeader) request.setHeader('Cookie', cookieHeader)
        request.on('response', (response) => {
          const chunks: Buffer[] = []
          response.on('data', (chunk) => chunks.push(Buffer.from(chunk)))
          response.on('end', async () => {
            const body = Buffer.concat(chunks).toString('utf8')
            const setCookie = response.headers['set-cookie']
            const setCookieList = Array.isArray(setCookie) ? setCookie : setCookie ? [String(setCookie)] : []
            for (const rawCookie of setCookieList) {
              const cookie = ipcEvent.parseSetCookie(rawCookie, 'https://pan.quark.cn')
              if (cookie) {
                try {
                  await session.defaultSession.cookies.set(cookie)
                } catch (err) {
                  console.error(err)
                }
              }
            }
            const cookies = await session.defaultSession.cookies.get({ domain: 'quark.cn' })
            resolve({ ok: response.statusCode >= 200 && response.statusCode < 300, status: response.statusCode, body, cookies })
          })
        })
        request.on('error', (error) => resolve({ ok: false, status: 0, body: '', cookies: [], error: error.message }))
        request.write(JSON.stringify({ fids: [fileId] }))
        request.end()
      })
    })
  }

  private static parseSetCookie(rawCookie: string, defaultUrl: string) {
    const parts = rawCookie
      .split(';')
      .map((part) => part.trim())
      .filter(Boolean)
    const [nameValue, ...attrs] = parts
    if (!nameValue || !nameValue.includes('=')) return undefined
    const [name, ...valueParts] = nameValue.split('=')
    const cookie: any = {
      url: defaultUrl,
      name,
      value: valueParts.join('='),
      path: '/',
      secure: true
    }
    for (const attr of attrs) {
      const [key, ...attrValueParts] = attr.split('=')
      const lowerKey = key.toLowerCase()
      const attrValue = attrValueParts.join('=')
      if (lowerKey === 'domain' && attrValue) cookie.domain = attrValue
      else if (lowerKey === 'path' && attrValue) cookie.path = attrValue
      else if (lowerKey === 'expires' && attrValue) {
        const expires = Date.parse(attrValue)
        if (Number.isFinite(expires)) cookie.expirationDate = Math.floor(expires / 1000)
      } else if (lowerKey === 'max-age' && attrValue) {
        const maxAge = Number(attrValue)
        if (Number.isFinite(maxAge)) cookie.expirationDate = Math.floor(Date.now() / 1000) + maxAge
      } else if (lowerKey === 'secure') cookie.secure = true
      else if (lowerKey === 'httponly') cookie.httpOnly = true
    }
    return cookie
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
          cmdArguments.push('-s')
          cmdArguments.push('-f')
          cmdArguments.push('-t 0')
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

  private static handleOfficePreviewConvertToPdf() {
    ipcMain.handle('OfficePreview:convertToPdf', async (_event, data: { sourceUrl?: string; fileName?: string }) => {
      try {
        const sourceUrl = data?.sourceUrl || ''
        const fileName = path.basename(data?.fileName || 'document')
        if (!sourceUrl) return { ok: false, error: '文档预览地址为空' }
        const soffice = findSoffice()
        if (!soffice) {
          return {
            ok: false,
            error: '未找到 LibreOffice。请安装 LibreOffice 后重试，或继续使用网盘自带预览。'
          }
        }

        const hash = createHash('sha1')
          .update(sourceUrl + fileName)
          .digest('hex')
        const previewRoot = path.join(app.getPath('userData'), 'office-preview')
        const workDir = path.join(previewRoot, hash)
        mkdirSync(workDir, { recursive: true })
        const ext = path.extname(fileName) || '.office'
        const inputPath = path.join(workDir, `source${ext}`)
        const outputPath = path.join(workDir, 'source.pdf')
        if (!existsSync(outputPath)) {
          const response = await fetch(sourceUrl)
          if (!response.ok) return { ok: false, error: `文档下载失败：${response.status}` }
          const arrayBuffer = await response.arrayBuffer()
          writeFileSync(inputPath, Buffer.from(arrayBuffer))
          await convertOfficeFileToPdf(soffice, inputPath, workDir)
        }
        if (!existsSync(outputPath)) return { ok: false, error: 'LibreOffice 未生成 PDF 文件' }
        return { ok: true, pdfUrl: pathToFileUrl(outputPath) }
      } catch (err: any) {
        return { ok: false, error: err?.message || '文档转换 PDF 失败' }
      }
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
