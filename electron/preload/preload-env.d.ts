/* eslint-disable no-unused-vars */
declare interface Window {
  Electron: any
  platform: any
  WinMsg: any
  WebToElectron: any
  WebToWindow: any
  WebToElectronCB: any
  WebShowOpenDialogSync: any
  WebShowSaveDialogSync: any
  WebShowItemInFolder: any
  WebPlatformSync: any
  WebClearCookies: any
  WebClearCache: any
  WebUserToken: any
  WebSaveTheme: any
  WebReload: any
  WebRelaunch: any
  WebRelaunchAria: () => Promise<number>
  WebSetProgressBar: any
  WebGetCookies: any
  WebQuarkAccountInfo: any
  WebQuarkDownloadUrl: any
  WebSetCookies: any
  WebOpenWindow: any
  WebOpenExternal: (url: string) => Promise<{ ok: boolean; error?: string }>
  WebCheckUpdate: () => Promise<{ ok: boolean; version?: string; error?: string }>
  WebSafeStorageEncrypt: (value: string) => Promise<string>
  WebSafeStorageDecrypt: (value: string) => Promise<string>
  WebSafeStorageEncryptSync: (value: string) => string
  WebSafeStorageDecryptSync: (value: string) => string
  WebShutDown: any
  WebSetProxy: any
  WebMpvEmbeddedCapability: () => Promise<any>
  WebMpvEmbeddedLoad: (data: any) => Promise<any>
  WebMpvEmbeddedControl: (data: any) => Promise<any>
  WebMpvEmbeddedStatus: () => Promise<any>
  WebMpvSharedTextureCapability: () => { available: boolean; platform: string; reason?: string }
  WebMpvSharedTexture: {
    isAvailable: () => boolean
    onFrame: (callback: (videoFrame: VideoFrame, index: number) => void) => void
    removeFrameListener: () => void
    onClear: (callback: () => void) => void
    removeClearListener: () => void
  }
  TvBoxInvoke: (channel: string, data?: unknown) => Promise<unknown>
  IsMainPage: boolean
}
