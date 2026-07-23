/* eslint-disable no-unused-vars */
declare interface Window {
  platform: any
  WinMsg: any
  WebToElectron: any
  WebToWindow: any
  WebToElectronCB: any
  WebShowOpenDialogSync: any
  WebShowSaveDialogSync: any
  WebShowItemInFolder: any
  WebPlatformSync: any
  WebGetAppPaths: () => Promise<{ userDataPath?: string; downloadsPath?: string; defaultUserDataPath?: string } | undefined>
  WebSetUserDataPath: (value: string) => Promise<{ ok: boolean; path?: string; requiresRestart?: boolean; error?: string }>
  WebClearCookies: any
  WebClearCache: any
  WebUserToken: any
  WebSaveTheme: any
  WebReload: any
  WebRelaunch: any
  WebRelaunchAria: () => Promise<number>
  WebSetProgressBar: any
  WebGetCookies: any
  WebSetCookies: any
  WebOpenWindow: any
  WebOpenExternal: (url: string) => Promise<{ ok: boolean; error?: string }>
  WebPikPakCaptchaOpen: (url: string) => Promise<{ ok: boolean; error?: string }>
  WebPikPakCaptchaClose: () => Promise<{ ok: boolean }>
  WebPikPakCaptchaReset: () => Promise<{ ok: boolean }>
  WebPikPakCaptchaOnCompleted: (callback: (payload: { captchaToken?: string }) => void) => () => void
  WebOAuthBegin: (provider: 'onedrive' | 'dropbox' | 'gdrive') => Promise<{ ok: boolean; state?: string; redirectUri?: string; error?: string }>
  WebOAuthOpen: (state: string, url: string) => Promise<{ ok: boolean; error?: string }>
  WebOAuthCancel: (state: string) => Promise<{ ok: boolean }>
  WebOAuthOnCallback: (callback: (payload: { provider: 'onedrive' | 'dropbox' | 'gdrive'; state: string; code: string; error: string; errorDescription: string }) => void) => () => void
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
  WebUploadPort: {
    send: (data: any) => void
    onMessage: (callback: (data: any) => void) => void
    removeMessageHandler: () => void
  }
  WebWorkerPort: {
    send: (data: any) => void
    onMessage: (callback: (data: any) => void) => void
    notifyReady: () => void
  }
  WebOnSetPage: (callback: (args: any) => void) => void
  WebOnSetTheme: (callback: (args: any) => void) => void
  WebClipboardWriteText: (text: string) => void
  WebShellOpenPath: (fullPath: string) => void
  IsMainPage: boolean
}
