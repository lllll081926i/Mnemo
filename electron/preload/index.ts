import Electron, { ipcRenderer } from 'electron'

window.Electron = Electron
process.noAsar = true
window.platform = process.platform

window.WebToElectron = function(data: any) {
  try {
    ipcRenderer.send('WebToElectron', data)
  } catch {
  }
}

window.WebToWindow = function(data: any, callback: any) {
  try {
    const backData = ipcRenderer.sendSync('WebToWindow', data)
    callback && callback(backData)
  } catch {
  }
}

window.WebToElectronCB = function(data: any, callback: any) {
  try {
    const backData = ipcRenderer.sendSync('WebToElectronCB', data)
    callback(backData)
  } catch {
  }
}

ipcRenderer.on('MainSendToken', function(event, arg) {
  try {
    window.postMessage(arg)
  } catch {
  }
})

window.WebShowOpenDialogSync = function(config: any, callback: any) {
  try {
    const backData = ipcRenderer.sendSync('WebShowOpenDialogSync', config)
    callback(backData)
  } catch {
  }
}

window.WebShowSaveDialogSync = function(config: any, callback: any) {
  try {
    const backData = ipcRenderer.sendSync('WebShowSaveDialogSync', config)
    callback(backData)
  } catch {
  }
}
window.WebShowItemInFolder = function(fullPath: string) {
  try {
    ipcRenderer.send('WebShowItemInFolder', fullPath)
  } catch {
  }
}

window.WebPlatformSync = function(callback: any) {
  try {
    const backData = ipcRenderer.sendSync('WebPlatformSync')
    callback(backData)
  } catch {
  }
}
window.WebGetAppPaths = async function() {
  try {
    return await ipcRenderer.invoke('AppPaths:get')
  } catch {
    return undefined
  }
}
window.WebSetUserDataPath = async function(value: string) {
  try {
    return await ipcRenderer.invoke('AppPaths:setUserData', value)
  } catch (error: any) {
    return { ok: false, error: error?.message || '保存用户数据目录失败' }
  }
}

window.WebClearCookies = function(data: any) {
  try {
    ipcRenderer.send('WebClearCookies', data)
  } catch {
  }
}
window.WebClearCache = function(data: any) {
  try {
    ipcRenderer.send('WebClearCache', data)
  } catch {
  }
}
window.WebUserToken = function(data: any) {
  try {
    ipcRenderer.send('WebUserToken', data)
  } catch {
  }
}
window.WebSaveTheme = function(data: any) {
  try {
    ipcRenderer.send('WebSaveTheme', data)
  } catch {
  }
}

window.WebReload = function(data: any) {
  try {
    ipcRenderer.send('WebReload', data)
  } catch {
  }
}
window.WebRelaunch = function(data: any) {
  try {
    ipcRenderer.send('WebRelaunch', data)
  } catch {
  }
}
window.WebRelaunchAria = async function() {
  try {
    return await ipcRenderer.invoke('WebRelaunchAria')
  } catch {
    return 0
  }
}
window.WebSetProgressBar = function(data: any) {
  try {
    ipcRenderer.send('WebSetProgressBar', data)
  } catch {
  }
}
window.WebGetCookies = async function(data: any) {
  try {
    return await ipcRenderer.invoke('WebGetCookies', data)
  } catch {
  }
}
window.WebSetCookies = function(cookies: any) {
  try {
    ipcRenderer.send('WebSetCookies', cookies)
  } catch {
  }
}

window.WebOpenWindow = function(data: any) {
  try {
    ipcRenderer.send('WebOpenWindow', data)
  } catch {
  }
}
window.WebOpenExternal = async function(url: string) {
  try {
    return await ipcRenderer.invoke('WebOpenExternal', url)
  } catch (error: any) {
    return { ok: false, error: error?.message || '无法打开系统浏览器' }
  }
}
window.WebPikPakCaptchaOpen = async function(url: string) {
  try {
    return await ipcRenderer.invoke('PikPakCaptcha:open', url)
  } catch (error: any) {
    return { ok: false, error: error?.message || '无法打开 PikPak 验证窗口' }
  }
}
window.WebPikPakCaptchaClose = async function() {
  try {
    return await ipcRenderer.invoke('PikPakCaptcha:close')
  } catch {
    return { ok: false }
  }
}
window.WebPikPakCaptchaOnCompleted = function(callback: () => void) {
  const listener = () => callback()
  ipcRenderer.on('PikPakCaptcha:completed', listener)
  return () => ipcRenderer.removeListener('PikPakCaptcha:completed', listener)
}
window.WebOAuthBegin = async function(provider: string) {
  return ipcRenderer.invoke('OAuth:begin', { provider })
}
window.WebOAuthOpen = async function(state: string, url: string) {
  return ipcRenderer.invoke('OAuth:open', { state, url })
}
window.WebOAuthCancel = async function(state: string) {
  return ipcRenderer.invoke('OAuth:cancel', { state })
}
window.WebOAuthOnCallback = function(callback: (payload: any) => void) {
  const listener = (_event: Electron.IpcRendererEvent, payload: any) => callback(payload)
  ipcRenderer.on('OAuth:callback', listener)
  return () => ipcRenderer.removeListener('OAuth:callback', listener)
}
window.WebShutDown = function(data: any) {
  try {
    ipcRenderer.send('WebShutDown', data)
  } catch {
  }
}
window.WebSetProxy = async function(data: { mode: 'system' | 'direct' | 'fixed_servers'; proxyUrl?: string }) {
  try {
    return await ipcRenderer.invoke('WebSetProxy', data)
  } catch {
    return false
  }
}
window.WebCheckUpdate = async function() {
  return ipcRenderer.invoke('AutoUpdate:check')
}
window.WebSafeStorageEncrypt = async function(value: string) {
  return ipcRenderer.invoke('SafeStorage:encrypt', value)
}
window.WebSafeStorageDecrypt = async function(value: string) {
  return ipcRenderer.invoke('SafeStorage:decrypt', value)
}
window.WebSafeStorageEncryptSync = function(value: string) {
  const result = ipcRenderer.sendSync('SafeStorage:encryptSync', value)
  if (!result?.ok) throw new Error(result?.error || '加密失败')
  return result.value
}
window.WebSafeStorageDecryptSync = function(value: string) {
  const result = ipcRenderer.sendSync('SafeStorage:decryptSync', value)
  if (!result?.ok) throw new Error(result?.error || '解密失败')
  return result.value
}

window.WebMpvEmbeddedCapability = async function() {
  try {
    return await ipcRenderer.invoke('MpvEmbedded:getCapability')
  } catch (error: any) {
    return { enabled: false, status: 'disabled', reason: error?.message || 'mpv embedded capability ipc failed' }
  }
}

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

window.WebMpvEmbeddedLoad = async function(data: any) {
  try {
    return await ipcRenderer.invoke('MpvEmbedded:load', normalizeMpvEmbeddedLoadData(data))
  } catch (error: any) {
    return { ok: false, error: error?.message || 'mpv embedded load ipc failed' }
  }
}

window.WebMpvEmbeddedControl = async function(data: any) {
  try {
    return await ipcRenderer.invoke('MpvEmbedded:control', data)
  } catch (error: any) {
    return { ok: false, error: error?.message || 'mpv embedded control ipc failed' }
  }
}

window.WebMpvEmbeddedStatus = async function() {
  try {
    return await ipcRenderer.invoke('MpvEmbedded:getStatus')
  } catch (error: any) {
    return { ok: false, error: error?.message || 'mpv embedded status ipc failed' }
  }
}

window.WebMpvSharedTextureCapability = function() {
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
}

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

// Match sbtlTV's lifecycle: Electron's receiver belongs to the preload
// renderer process, while the UI component only owns the frame callback.
registerMpvSharedTextureReceiver()

window.WebMpvSharedTexture = {
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
}

window.TvBoxInvoke = async function(channel: string, data: unknown) {
  try {
    return await ipcRenderer.invoke(channel, data)
  } catch (e: unknown) {
    throw e
  }
}

function createRightMenu() {
  window.addEventListener('contextmenu', (e) => {
      if (e) e.preventDefault()
      const target = e.target as HTMLElement
      // 检查页面是否是有选择的文本 这里显示复制和剪切选项是否可见
      const selectText = !!window.getSelection().toString()
      if (selectText || isEleEditable(target)) {
        // 读取剪切板是否有文本 这里传递粘贴选项是否可见
        const showPaste = !!navigator.clipboard.readText()
        // 判断ReadOnly
        const isReadOnly = target.hasAttribute('readonly')
        // 发送给主进程让它显示菜单
        ipcRenderer.send('show-context-menu', {
          showPaste: !isReadOnly && showPaste,
          showCopy: selectText,
          showCut: !isReadOnly && selectText
        })
      }
    }
  )
}

function isEleEditable(e: any): boolean {
  if (!e) return false
  if (e.tagName === 'TEXTAREA'
    || (e.tagName === 'INPUT' && e.type !== 'checkbox')
    || e.contentEditable == 'true') {
    return true
  } else {
    // eslint-disable-next-line @typescript-eslint/no-unsafe-return
    return isEleEditable(e.parentNode)
  }
}

createRightMenu()

// fix: new-windows event
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
