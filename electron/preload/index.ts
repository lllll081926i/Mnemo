import { clipboard, contextBridge, ipcRenderer } from 'electron'

// ---------------------------------------------------------------------------
// macOS sharedTexture bridge (must live in the preload renderer process)
// ---------------------------------------------------------------------------
let mpvSharedTextureFrameCallback: ((videoFrame: VideoFrame, index: number) => void) | null = null
let mpvSharedTextureClearCallback: (() => void) | null = null
let mpvSharedTextureReceiverReady = false

ipcRenderer.on('MpvEmbedded:clearTexture', () => {
  mpvSharedTextureClearCallback?.()
})

const registerMpvSharedTextureReceiver = (): boolean => {
  try {
    const sharedTexture = (require('electron') as any).sharedTexture
    if (process.platform !== 'darwin' || !sharedTexture?.setSharedTextureReceiver) return false
    sharedTexture.setSharedTextureReceiver(async (data: { importedSharedTexture?: { getVideoFrame: () => VideoFrame; release: () => void } }, ...args: unknown[]) => {
      const imported = data?.importedSharedTexture
      try {
        if (imported && mpvSharedTextureFrameCallback) {
          const frameIndex = typeof args[0] === 'number' ? args[0] : 0
          const videoFrame = imported.getVideoFrame()
          mpvSharedTextureFrameCallback(videoFrame, frameIndex)
        }
      } catch (error) {
        console.error('[mpv] sharedTexture receiver error:', error)
      } finally {
        try { imported?.release() } catch {}
      }
    })
    mpvSharedTextureReceiverReady = true
    console.log('[mpv] sharedTexture receiver registered')
    return true
  } catch (error) {
    mpvSharedTextureReceiverReady = false
    console.warn('[mpv] sharedTexture receiver is not available:', error)
    return false
  }
}

registerMpvSharedTextureReceiver()

// ---------------------------------------------------------------------------
// Upload worker port (main window side)
// ---------------------------------------------------------------------------
let uploadPort: MessagePort | null = null
const pendingUploadMessages: any[] = []
let uploadMessageHandler: ((data: any) => void) | null = null

ipcRenderer.on('setUploadPort', (_event) => {
  const [port] = _event.ports
  uploadPort = port
  port.onmessage = (event: MessageEvent) => {
    Promise.resolve().then(() => {
      try { uploadMessageHandler?.(event.data) } catch {}
    })
  }
  const pending = pendingUploadMessages.splice(0, pendingUploadMessages.length)
  for (const msg of pending) port.postMessage(msg)
})

ipcRenderer.on('clearUploadPort', () => {
  uploadPort = null
})

// ---------------------------------------------------------------------------
// Worker port (upload worker window side)
// ---------------------------------------------------------------------------
let workerPort: MessagePort | null = null
let workerMessageHandler: ((data: any) => void) | null = null

ipcRenderer.on('setPort', (_event) => {
  const [port] = _event.ports
  workerPort = port
  port.onmessage = (event: MessageEvent) => {
    Promise.resolve().then(() => {
      try { workerMessageHandler?.(event.data) } catch {}
    })
  }
})

// ---------------------------------------------------------------------------
// MainSendToken → window.postMessage
// ---------------------------------------------------------------------------
ipcRenderer.on('MainSendToken', function (_event, arg) {
  try {
    window.postMessage(arg)
  } catch {}
})

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------
function normalizeMpvEmbeddedLoadData(data: any) {
  const headers: Record<string, string> = {}
  for (const [key, value] of Object.entries(data?.headers || {})) {
    if (!key || value == null) continue
    headers[String(key)] = String(value)
  }
  return {
    url: String(data?.url || ''),
    headers,
    title: String(data?.title || ''),
    startPosition: typeof data?.startPosition === 'number' ? data.startPosition : Number(data?.startPosition || 0)
  }
}

function isEleEditable(e: any): boolean {
  if (!e) return false
  if (e.tagName === 'TEXTAREA' || (e.tagName === 'INPUT' && e.type !== 'checkbox') || e.contentEditable == 'true') {
    return true
  } else {
    // eslint-disable-next-line @typescript-eslint/no-unsafe-return
    return isEleEditable(e.parentNode)
  }
}

// ---------------------------------------------------------------------------
// Context menu
// ---------------------------------------------------------------------------
function createRightMenu() {
  window.addEventListener('contextmenu', (e) => {
    if (e) e.preventDefault()
    const target = e.target as HTMLElement
    const selectText = !!window.getSelection()?.toString()
    if (selectText || isEleEditable(target)) {
      const showPaste = !!navigator.clipboard.readText()
      const isReadOnly = target.hasAttribute('readonly')
      ipcRenderer.send('show-context-menu', {
        showPaste: !isReadOnly && showPaste,
        showCopy: selectText,
        showCut: !isReadOnly && selectText
      })
    }
  })
}

createRightMenu()

// ---------------------------------------------------------------------------
// webview legacy events
// ---------------------------------------------------------------------------
ipcRenderer.on('webview-new-window', (e, webContentsId, details) => {
  const webview = document.getElementById('webview') as any
  const evt = new Event('new-window', { bubbles: true, cancelable: false })
  webview.dispatchEvent(Object.assign(evt, details))
})

ipcRenderer.on('webview-redirect', (e, webContentsId, url) => {
  const webview = document.getElementById('webview') as any
  const evt = new Event('will-redirect', { bubbles: true, cancelable: false })
  webview.dispatchEvent(Object.assign(evt, { url }))
})

// ---------------------------------------------------------------------------
// Expose APIs via contextBridge
// ---------------------------------------------------------------------------
contextBridge.exposeInMainWorld('platform', process.platform)

contextBridge.exposeInMainWorld('WebToElectron', function (data: any) {
  try {
    ipcRenderer.send('WebToElectron', data)
  } catch {}
})

contextBridge.exposeInMainWorld('WebToWindow', function (data: any, callback: any) {
  try {
    const backData = ipcRenderer.sendSync('WebToWindow', data)
    callback && callback(backData)
  } catch {}
})

contextBridge.exposeInMainWorld('WebToElectronCB', function (data: any, callback: any) {
  try {
    const backData = ipcRenderer.sendSync('WebToElectronCB', data)
    callback(backData)
  } catch {}
})

contextBridge.exposeInMainWorld('WebShowOpenDialogSync', function (config: any, callback: any) {
  try {
    const backData = ipcRenderer.sendSync('WebShowOpenDialogSync', config)
    callback(backData)
  } catch {}
})

contextBridge.exposeInMainWorld('WebShowSaveDialogSync', function (config: any, callback: any) {
  try {
    const backData = ipcRenderer.sendSync('WebShowSaveDialogSync', config)
    callback(backData)
  } catch {}
})

contextBridge.exposeInMainWorld('WebShowItemInFolder', function (fullPath: string) {
  try {
    ipcRenderer.send('WebShowItemInFolder', fullPath)
  } catch {}
})

contextBridge.exposeInMainWorld('WebPlatformSync', function (callback: any) {
  try {
    const backData = ipcRenderer.sendSync('WebPlatformSync')
    callback(backData)
  } catch {}
})

contextBridge.exposeInMainWorld('WebGetAppPaths', async function () {
  try {
    return await ipcRenderer.invoke('AppPaths:get')
  } catch {
    return undefined
  }
})

contextBridge.exposeInMainWorld('WebSetUserDataPath', async function (value: string) {
  try {
    return await ipcRenderer.invoke('AppPaths:setUserData', value)
  } catch (error: any) {
    return { ok: false, error: error?.message || '保存用户数据目录失败' }
  }
})

contextBridge.exposeInMainWorld('WebClearCookies', function (data: any) {
  try {
    ipcRenderer.send('WebClearCookies', data)
  } catch {}
})

contextBridge.exposeInMainWorld('WebClearCache', function (data: any) {
  try {
    ipcRenderer.send('WebClearCache', data)
  } catch {}
})

contextBridge.exposeInMainWorld('WebUserToken', function (data: any) {
  try {
    ipcRenderer.send('WebUserToken', data)
  } catch {}
})

contextBridge.exposeInMainWorld('WebSaveTheme', function (data: any) {
  try {
    ipcRenderer.send('WebSaveTheme', data)
  } catch {}
})

contextBridge.exposeInMainWorld('WebReload', function (data: any) {
  try {
    ipcRenderer.send('WebReload', data)
  } catch {}
})

contextBridge.exposeInMainWorld('WebRelaunch', function (data: any) {
  try {
    ipcRenderer.send('WebRelaunch', data)
  } catch {}
})

contextBridge.exposeInMainWorld('WebRelaunchAria', async function () {
  try {
    return await ipcRenderer.invoke('WebRelaunchAria')
  } catch {
    return 0
  }
})

contextBridge.exposeInMainWorld('WebSetProgressBar', function (data: any) {
  try {
    ipcRenderer.send('WebSetProgressBar', data)
  } catch {}
})

contextBridge.exposeInMainWorld('WebGetCookies', async function (data: any) {
  try {
    return await ipcRenderer.invoke('WebGetCookies', data)
  } catch {}
})

contextBridge.exposeInMainWorld('WebSetCookies', function (cookies: any) {
  try {
    ipcRenderer.send('WebSetCookies', cookies)
  } catch {}
})

contextBridge.exposeInMainWorld('WebOpenWindow', function (data: any) {
  try {
    ipcRenderer.send('WebOpenWindow', data)
  } catch {}
})

contextBridge.exposeInMainWorld('WebOpenExternal', async function (url: string) {
  try {
    return await ipcRenderer.invoke('WebOpenExternal', url)
  } catch (error: any) {
    return { ok: false, error: error?.message || '无法打开系统浏览器' }
  }
})

contextBridge.exposeInMainWorld('WebPikPakCaptchaOpen', async function (url: string) {
  try {
    return await ipcRenderer.invoke('PikPakCaptcha:open', url)
  } catch (error: any) {
    return { ok: false, error: error?.message || '无法打开 PikPak 验证窗口' }
  }
})

contextBridge.exposeInMainWorld('WebPikPakCaptchaClose', async function () {
  try {
    return await ipcRenderer.invoke('PikPakCaptcha:close')
  } catch {
    return { ok: false }
  }
})

contextBridge.exposeInMainWorld('WebPikPakCaptchaOnCompleted', function (callback: (payload: { captchaToken?: string }) => void) {
  const listener = (_event: Electron.IpcRendererEvent, payload: { captchaToken?: string }) => callback(payload || {})
  ipcRenderer.on('PikPakCaptcha:completed', listener)
  return () => ipcRenderer.removeListener('PikPakCaptcha:completed', listener)
})

contextBridge.exposeInMainWorld('WebPikPakCaptchaReset', async function () {
  try {
    return await ipcRenderer.invoke('PikPakCaptcha:reset')
  } catch {
    return { ok: false }
  }
})

contextBridge.exposeInMainWorld('WebOAuthBegin', async function (provider: string) {
  return ipcRenderer.invoke('OAuth:begin', { provider })
})

contextBridge.exposeInMainWorld('WebOAuthOpen', async function (state: string, url: string) {
  return ipcRenderer.invoke('OAuth:open', { state, url })
})

contextBridge.exposeInMainWorld('WebOAuthCancel', async function (state: string) {
  return ipcRenderer.invoke('OAuth:cancel', { state })
})

contextBridge.exposeInMainWorld('WebOAuthOnCallback', function (callback: (payload: any) => void) {
  const listener = (_event: Electron.IpcRendererEvent, payload: any) => callback(payload)
  ipcRenderer.on('OAuth:callback', listener)
  return () => ipcRenderer.removeListener('OAuth:callback', listener)
})

contextBridge.exposeInMainWorld('WebShutDown', function (data: any) {
  try {
    ipcRenderer.send('WebShutDown', data)
  } catch {}
})

contextBridge.exposeInMainWorld('WebSetProxy', async function (data: { mode: 'system' | 'direct' | 'fixed_servers'; proxyUrl?: string }) {
  try {
    return await ipcRenderer.invoke('WebSetProxy', data)
  } catch {
    return false
  }
})

contextBridge.exposeInMainWorld('WebCheckUpdate', async function () {
  return ipcRenderer.invoke('AutoUpdate:check')
})

contextBridge.exposeInMainWorld('WebSafeStorageEncrypt', async function (value: string) {
  return ipcRenderer.invoke('SafeStorage:encrypt', value)
})

contextBridge.exposeInMainWorld('WebSafeStorageDecrypt', async function (value: string) {
  return ipcRenderer.invoke('SafeStorage:decrypt', value)
})

contextBridge.exposeInMainWorld('WebSafeStorageEncryptSync', function (value: string) {
  const result = ipcRenderer.sendSync('SafeStorage:encryptSync', value)
  if (!result?.ok) throw new Error(result?.error || '加密失败')
  return result.value
})

contextBridge.exposeInMainWorld('WebSafeStorageDecryptSync', function (value: string) {
  const result = ipcRenderer.sendSync('SafeStorage:decryptSync', value)
  if (!result?.ok) throw new Error(result?.error || '解密失败')
  return result.value
})

contextBridge.exposeInMainWorld('WebMpvEmbeddedCapability', async function () {
  try {
    return await ipcRenderer.invoke('MpvEmbedded:getCapability')
  } catch (error: any) {
    return { enabled: false, status: 'disabled', reason: error?.message || 'mpv embedded capability ipc failed' }
  }
})

contextBridge.exposeInMainWorld('WebMpvEmbeddedLoad', async function (data: any) {
  try {
    return await ipcRenderer.invoke('MpvEmbedded:load', normalizeMpvEmbeddedLoadData(data))
  } catch (error: any) {
    return { ok: false, error: error?.message || 'mpv embedded load ipc failed' }
  }
})

contextBridge.exposeInMainWorld('WebMpvEmbeddedControl', async function (data: any) {
  try {
    return await ipcRenderer.invoke('MpvEmbedded:control', data)
  } catch (error: any) {
    return { ok: false, error: error?.message || 'mpv embedded control ipc failed' }
  }
})

contextBridge.exposeInMainWorld('WebMpvEmbeddedStatus', async function () {
  try {
    return await ipcRenderer.invoke('MpvEmbedded:getStatus')
  } catch (error: any) {
    return { ok: false, error: error?.message || 'mpv embedded status ipc failed' }
  }
})

contextBridge.exposeInMainWorld('WebMpvSharedTextureCapability', function () {
  if (process.platform !== 'darwin') {
    return { available: false, platform: process.platform, reason: 'sharedTexture receiver is only planned for macOS embedded MPV' }
  }
  try {
    const sharedTexture = (require('electron') as any).sharedTexture
    return {
      available: Boolean(sharedTexture?.setSharedTextureReceiver),
      platform: process.platform,
      reason: sharedTexture ? undefined : 'macOS 内嵌 MPV 渲染桥接尚未启用'
    }
  } catch (error: any) {
    return { available: false, platform: process.platform, reason: error?.message || 'macOS 内嵌 MPV 渲染桥接检测失败' }
  }
})

contextBridge.exposeInMainWorld('WebMpvSharedTexture', {
  isAvailable: () => mpvSharedTextureReceiverReady,
  onFrame: (callback: (videoFrame: VideoFrame, index: number) => void) => {
    mpvSharedTextureFrameCallback = callback
  },
  removeFrameListener: () => {
    mpvSharedTextureFrameCallback = null
  },
  onClear: (callback: () => void) => {
    mpvSharedTextureClearCallback = callback
  },
  removeClearListener: () => {
    mpvSharedTextureClearCallback = null
  }
})

// ---------------------------------------------------------------------------
// Upload port API (main window)
// ---------------------------------------------------------------------------
contextBridge.exposeInMainWorld('WebUploadPort', {
  send: (data: any) => {
    if (uploadPort) {
      uploadPort.postMessage(data)
    } else {
      pendingUploadMessages.push(data)
      ipcRenderer.send('ensureUploadWorker')
    }
  },
  onMessage: (callback: (data: any) => void) => {
    uploadMessageHandler = callback
  },
  removeMessageHandler: () => {
    uploadMessageHandler = null
  }
})

// ---------------------------------------------------------------------------
// Worker port API (upload worker window)
// ---------------------------------------------------------------------------
contextBridge.exposeInMainWorld('WebWorkerPort', {
  send: (data: any) => {
    workerPort?.postMessage(data)
  },
  onMessage: (callback: (data: any) => void) => {
    workerMessageHandler = callback
  },
  notifyReady: () => {
    ipcRenderer.send('uploadWorkerReady')
  }
})

// ---------------------------------------------------------------------------
// Page / theme notifications
// ---------------------------------------------------------------------------
contextBridge.exposeInMainWorld('WebOnSetPage', function (callback: (args: any) => void) {
  ipcRenderer.on('setPage', (_event, args) => callback(args))
})

contextBridge.exposeInMainWorld('WebOnSetTheme', function (callback: (args: any) => void) {
  ipcRenderer.on('setTheme', (_event, args) => callback(args))
})

// ---------------------------------------------------------------------------
// Clipboard / shell (sandbox is false so require('electron') is available)
// ---------------------------------------------------------------------------
contextBridge.exposeInMainWorld('WebClipboardWriteText', function (text: string) {
  clipboard.writeText(text, 'clipboard')
})

contextBridge.exposeInMainWorld('WebShellOpenPath', async (fullPath: string) => {
  return ipcRenderer.invoke('Shell:openPath', fullPath)
})
