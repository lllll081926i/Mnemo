import { defineStore } from 'pinia'
import DebugLog from '../utils/debuglog'
import { getSystemDownloadsPath, getUserDataPath } from '../utils/electronhelper'
import { useAppStore } from '../store'
import PanDAL from '../pan/pandal'
import { copyFileSync, existsSync, readFileSync, writeFileSync } from 'fs'

type ProxyType = 'system' | 'none' | 'http' | 'https' | 'socks5' | 'socks5h'
type VideoQuality = 'Origin' | 'QHD' | 'FHD' | 'HD' | 'SD' | 'LD'
export interface SettingState {
  // 应用设置
  uiTheme: string
  uiDefaultTab: string
  uiImageMode: string
  uiExitOnClose: boolean
  uiLaunchStart: boolean
  uiLaunchStartShow: boolean

  // 安全设置
  securityEncType: string
  securityPassword: string
  securityPasswordConfirm: boolean
  securityEncFileName: boolean
  securityEncFileNameHideExt: boolean
  securityFileNameAutoDecrypt: boolean
  securityPreviewAutoDecrypt: boolean

  securityHideBackupDrive: boolean
  securityHideResourceDrive: boolean
  securityHidePicDrive: boolean

  // 在线预览
  uiVideoQuality: VideoQuality
  uiVideoQualityTips: boolean
  uiVideoQualityLastSelect: boolean
  uiVideoPlayer: string
  uiVideoEnablePlayerList: boolean
  uiVideoPlayerExit: boolean
  uiVideoPlayerHistory: boolean
  uiVideoPlayerParams: string
  uiVideoSubtitleMode: string
  uiVideoPlayerPath: string

  uiAutoColorVideo: boolean

  uiXBTNumber: number
  uiXBTWidth: number

  // 网盘设置
  uiShowPanPath: boolean
  uiFolderPreviewEnabled: boolean
  uiFolderPreviewAutoHide: number
  uiFileOrderDuli: string
  uiTimeFolderFormate: string
  uiTimeFolderIndex: number
  uiShareDays: string
  uiSharePassword: string
  uiShareFormate: string
  uiFileListOrder: string
  uiFileListMode: string
  uiFileColorArray: { key: string; title: string }[]

  // 下载文件
  downSavePath: string
  downSavePathDefault: boolean
  downSavePathFull: boolean
  downFileMax: number
  downThreadMax: number
  downGlobalSpeed: number
  downGlobalSpeedM: string
  ariaMaxConnectionPerServer: number
  ariaUserAgent: string

  // 任务通知
  // 上传文件
  uploadFileMax: number
  uploadGlobalSpeed: number
  uploadGlobalSpeedM: string

  // 上传下载综合设置
  downAutoShutDown: number
  downSaveShowPro: boolean
  downSmallFileFirst: boolean
  downUploadWhatExist: string
  downIngoredList: string[]
  downFinishAudio: boolean
  downAutoStart: boolean

  // 高级选项
  debugDirSize: string
  debugCacheSize: string
  debugFileListMax: number
  debugDownedListMax: number
  debugProxyHost: string
  debugProxyPort: string
  debugLogEnabled: boolean
  debugLogLevel: 'debug' | 'info' | 'warn' | 'error'
  debugLogMaxSizeMB: number
  // 网络代理
  proxyType: ProxyType
  proxyHost: string
  proxyPort: number
  proxyUserName: string
  proxyPassword: string

  // 远程Aria
  ariaSavePath: string
  ariaUrl: string
  ariaPwd: string
  ariaHttps: boolean
  ariaState: string

}

function createDefaultSetting(): SettingState {
  return {
    // 应用设置
    uiTheme: 'system',
    uiDefaultTab: 'pan',
    uiImageMode: 'fill',
    uiExitOnClose: false,
    uiLaunchStart: false,
    uiLaunchStartShow: false,

    // 安全设置
    securityEncType: 'aesctr',
    securityPassword: '',
    securityPasswordConfirm: false,
    securityEncFileName: true,
    securityEncFileNameHideExt: false,
    securityFileNameAutoDecrypt: true,
    securityPreviewAutoDecrypt: true,

    securityHideBackupDrive: false,
    securityHideResourceDrive: false,
    securityHidePicDrive: false,

    // 在线预览
    uiVideoQuality: 'Origin',
    uiVideoQualityTips: false,
    uiVideoQualityLastSelect: true,
    uiVideoPlayer: 'web',
    uiVideoEnablePlayerList: false,
    uiVideoPlayerExit: false,
    uiVideoPlayerHistory: false,
    uiVideoPlayerParams: '',
    uiVideoSubtitleMode: 'auto',
    uiVideoPlayerPath: '',

    uiAutoColorVideo: true,

    uiXBTNumber: 36,
    uiXBTWidth: 960,

    // 网盘设置
    uiShowPanPath: true,
    uiFolderPreviewEnabled: true,
    uiFolderPreviewAutoHide: 6,
    uiFileOrderDuli: 'null',
    uiTimeFolderFormate: 'yyyy-MM-dd HH-mm-ss',
    uiTimeFolderIndex: 1,
    uiShareDays: 'always',
    uiSharePassword: 'random',
    uiShareFormate: '「NAME」URL\n提取码: PWD',
    uiFileListOrder: 'name asc',
    uiFileListMode: 'list',
    uiFileColorArray: [
      { key: '#df5659', title: '鹅冠红' },
      { key: '#9c27b0', title: '兰花紫' },
      { key: '#42a5f5', title: '晴空蓝' },
      { key: '#00bc99', title: '竹叶青' },
      { key: '#4caf50', title: '宝石绿' },
      { key: '#ff9800', title: '金盏黄' }
    ],

    // 下载文件
    downSavePath: '',
    downSavePathDefault: true,
    downSavePathFull: true,
    downFileMax: 5,
    downThreadMax: 4,
    downGlobalSpeed: 0,
    downGlobalSpeedM: 'MB',
    ariaMaxConnectionPerServer: 16,
    ariaUserAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/89.0.4577.63 Safari/537.36',

    // 上传文件
    uploadFileMax: 5,
    uploadGlobalSpeed: 0,
    uploadGlobalSpeedM: 'MB',

    // 上传下载综合设置
    downAutoShutDown: 0,
    downSaveShowPro: true,
    downSmallFileFirst: false,
    downUploadWhatExist: 'refuse',
    downIngoredList: ['thumbs.db', 'desktop.ini', '.ds_store', '.td', '~', '.downloading'],
    downFinishAudio: true,
    downAutoStart: true,

    // 高级选项
    debugCacheSize: '',
    debugDirSize: '',
    debugFileListMax: 3000,
    debugDownedListMax: 5000,
    debugProxyHost: '127.0.0.1',
    debugProxyPort: '6666',
    debugLogEnabled: true,
    debugLogLevel: 'info',
    debugLogMaxSizeMB: 5,
    // 网络代理
    proxyType: 'system',
    proxyHost: '',
    proxyPort: 0,
    proxyUserName: '',
    proxyPassword: '',

    // 远程Aria
    ariaSavePath: '',
    ariaUrl: '',
    ariaPwd: '',
    ariaHttps: false,
    ariaState: 'local'
  }
}

function defaultValue(val: any, check: any[]) {
  if (val && check.includes(val)) return val
  return check[0]
}

function defaultString(val: any, check: string) {
  if (val && typeof val == 'string') return val
  return check
}

function defaultBool(val: any, check: boolean) {
  if (typeof val == 'boolean') return val
  return check
}

function defaultNumber(val: any, check: number) {
  if (typeof val == 'number') return val
  return check
}

function defaultNumberSub(val: any, check: number, min: number, max: number) {
  if (typeof val == 'number') {
    if (val < min) return min
    if (val > max) return max
    return val
  }
  return check
}

// 把磁盘上的配置合并进 state，逐项校验、兜底并做历史值迁移
function applyStoredConfig(setting: SettingState, val: any) {
  // 应用设置
  setting.uiTheme = defaultValue(val.uiTheme, ['system', 'light', 'dark'])
  setting.uiDefaultTab = defaultValue(val.uiDefaultTab, ['pan', 'down', 'sync', 'share', 'setting'])
  setting.uiImageMode = defaultValue(val.uiImageMode, ['fill', 'width', 'web'])
  setting.uiExitOnClose = defaultBool(val.uiExitOnClose, false)
  setting.uiLaunchStart = defaultBool(val.uiLaunchStart, false)
  setting.uiLaunchStartShow = defaultBool(val.uiLaunchStartShow, false)

  // 安全设置
  setting.securityEncType = defaultValue(val.securityEncType, ['aesctr', 'rc4md5'])
  setting.securityPassword = defaultString(val.securityPassword, '')
  setting.securityPasswordConfirm = defaultBool(val.securityPasswordConfirm, false)
  setting.securityEncFileName = defaultBool(val.securityEncFileName, true)
  setting.securityEncFileNameHideExt = defaultBool(val.securityEncFileNameHideExt, false)
  setting.securityFileNameAutoDecrypt = defaultBool(val.securityFileNameAutoDecrypt, true)
  setting.securityPreviewAutoDecrypt = defaultBool(val.securityPreviewAutoDecrypt, true)
  setting.securityHideBackupDrive = defaultBool(val.securityHideBackupDrive, false)
  setting.securityHideResourceDrive = defaultBool(val.securityHideResourceDrive, false)
  setting.securityHidePicDrive = defaultBool(val.securityHidePicDrive, false)

  // 在线预览
  setting.uiVideoQuality = defaultValue(val.uiVideoQuality, ['Origin', 'QHD', 'FHD', 'HD', 'SD', 'LD'])
  setting.uiVideoQualityTips = defaultBool(val.uiVideoQualityTips, false)
  setting.uiVideoQualityLastSelect = defaultBool(val.uiVideoQualityLastSelect, true)
  setting.uiVideoPlayer = defaultValue(val.uiVideoPlayer, ['web', 'mpv', 'other'])
  setting.uiVideoEnablePlayerList = defaultBool(val.uiVideoEnablePlayerList, false)
  setting.uiVideoPlayerExit = defaultBool(val.uiVideoPlayerExit, false)
  setting.uiVideoPlayerHistory = defaultBool(val.uiVideoPlayerHistory, false)
  setting.uiVideoPlayerParams = defaultString(val.uiVideoPlayerParams, '')
  setting.uiVideoSubtitleMode = defaultValue(val.uiVideoSubtitleMode, ['close', 'auto', 'select'])
  setting.uiVideoPlayerPath = defaultString(val.uiVideoPlayerPath, '')
  setting.uiAutoColorVideo = defaultBool(val.uiAutoColorVideo, true)

  setting.uiXBTNumber = defaultValue(val.uiXBTNumber, [36, 24, 48, 60, 72])
  setting.uiXBTWidth = defaultValue(val.uiXBTWidth, [960, 720, 1080, 1280])

  // 网盘设置
  setting.uiShowPanPath = defaultBool(val.uiShowPanPath, true)
  setting.uiFolderPreviewEnabled = defaultBool(val.uiFolderPreviewEnabled, true)
  setting.uiFolderPreviewAutoHide = defaultNumber(val.uiFolderPreviewAutoHide, 6)
  setting.uiFileOrderDuli = defaultString(val.uiFileOrderDuli, 'null')
  setting.uiTimeFolderFormate = defaultString(val.uiTimeFolderFormate, 'yyyy-MM-dd HH-mm-ss').replace('mm-dd', 'MM-dd').replace('HH-MM', 'HH-mm')
  setting.uiTimeFolderIndex = defaultNumber(val.uiTimeFolderIndex, 1)
  setting.uiShareDays = defaultValue(val.uiShareDays, ['always', 'week', 'month'])
  setting.uiSharePassword = defaultValue(val.uiSharePassword, ['random', 'last', 'nopassword'])
  setting.uiShareFormate = defaultString(val.uiShareFormate, '「NAME」URL\n提取码: PWD')
  setting.uiFileListOrder = defaultValue(val.uiFileListOrder, ['updated_at desc', 'name asc', 'name desc', 'updated_at asc', 'updated_at desc', 'size asc', 'size desc'])
  setting.uiFileListMode = defaultValue(val.uiFileListMode, ['list', 'image', 'bigimage'])
  if (val.uiFileColorArray && val.uiFileColorArray.length >= 6) setting.uiFileColorArray = val.uiFileColorArray

  // 下载文件
  setting.downSavePath = defaultString(val.downSavePath, getSystemDownloadsPath())
  setting.downSavePathDefault = defaultBool(val.downSavePathDefault, true)
  setting.downSavePathFull = defaultBool(val.downSavePathFull, true)
  setting.downFileMax = defaultValue(val.downFileMax, [5, 1, 2, 3, 4, 5])
  setting.downThreadMax = defaultValue(val.downThreadMax, [4, 1, 2, 4, 8, 16, 24, 32])
  setting.downGlobalSpeed = defaultNumberSub(val.downGlobalSpeed, 0, 0, 999)
  setting.downGlobalSpeedM = defaultValue(val.downGlobalSpeedM, ['MB', 'KB'])

  // 上传文件
  setting.uploadFileMax = defaultValue(val.uploadFileMax, [5, 1, 3, 5, 10, 20, 30, 50])
  setting.uploadGlobalSpeed = defaultNumberSub(val.uploadGlobalSpeed, 0, 0, 999)
  setting.uploadGlobalSpeedM = defaultValue(val.uploadGlobalSpeedM, ['MB', 'KB'])

  // 上传下载综合设置
  setting.downAutoShutDown = 0
  setting.downSaveShowPro = defaultBool(val.downSaveShowPro, true)
  setting.downSmallFileFirst = defaultBool(val.downSmallFileFirst, false)
  setting.downUploadWhatExist = defaultValue(val.downUploadWhatExist, ['ignore', 'overwrite', 'auto_rename', 'refuse'])
  setting.downIngoredList = val.downIngoredList && val.downIngoredList.length > 0 ? val.downIngoredList : ['thumbs.db', 'desktop.ini', '.ds_store', '.td', '~', '.downloading']
  setting.downFinishAudio = defaultBool(val.downFinishAudio, true)
  setting.downAutoStart = defaultBool(val.downAutoStart, true)

  // 高级选项
  setting.debugDirSize = defaultString(val.debugDirSize, '')
  setting.debugCacheSize = defaultString(val.debugCacheSize, '')
  setting.debugFileListMax = defaultNumberSub(val.debugFileListMax, 3000, 3000, 10000)
  setting.debugDownedListMax = defaultNumberSub(val.debugDownedListMax, 5000, 1000, 50000)
  setting.debugProxyHost = defaultString(val.debugProxyHost, '127.0.0.1')
  // Chromium blocks a number of ports, including 6666 (ERR_UNSAFE_PORT).
  // Migrate the historical default so media playback can reach the local proxy.
  const debugProxyPort = defaultString(val.debugProxyPort, '18888')
  setting.debugProxyPort = debugProxyPort === '6666' ? '18888' : debugProxyPort
  setting.debugLogEnabled = defaultBool(val.debugLogEnabled, true)
  setting.debugLogLevel = defaultValue(val.debugLogLevel, ['info', 'debug', 'warn', 'error'])
  setting.debugLogMaxSizeMB = defaultNumberSub(val.debugLogMaxSizeMB, 5, 1, 100)
  // 网络代理
  const storedProxyType = defaultValue(val.proxyType, ['system', 'none', 'http', 'https', 'socks5', 'socks5h']) as ProxyType
  if (Object.hasOwn(val, 'proxyUseProxy')) {
    setting.proxyType = val.proxyUseProxy === false && storedProxyType !== 'none' ? 'none' : storedProxyType === 'none' ? 'system' : storedProxyType
  } else {
    setting.proxyType = storedProxyType
  }
  setting.proxyHost = defaultString(val.proxyHost, '')
  setting.proxyPort = defaultNumber(val.proxyPort, 0)
  setting.proxyUserName = defaultString(val.proxyUserName, '')
  setting.proxyPassword = defaultString(val.proxyPassword, '')

  // 远程Aria
  setting.ariaSavePath = defaultString(val.ariaSavePath, '')
  if (setting.ariaSavePath.indexOf('/') < 0 && setting.ariaSavePath.indexOf('\\') < 0) setting.ariaSavePath = ''
  setting.ariaUrl = defaultString(val.ariaUrl, '')
  if (setting.ariaUrl.indexOf(':') < 0) setting.ariaUrl = ''
  setting.ariaPwd = defaultString(val.ariaPwd, '')
  setting.ariaHttps = defaultBool(val.ariaHttps, false)
  setting.ariaState = defaultValue(val.ariaState, ['local', 'remote'])
}

let settingstr = ''
let saveTimer: ReturnType<typeof setTimeout> | undefined
// Pinia 的响应式代理直接写回这个原始对象，beforeunload 时据此强制落盘
let activeState: SettingState | null = null

function persistSetting(state: SettingState, immediate = false) {
  if (saveTimer) {
    clearTimeout(saveTimer)
    saveTimer = undefined
  }
  if (!immediate) {
    // 输入类设置每次击键都会触发保存，合并到一次写盘，避免同步 IO 卡住渲染
    saveTimer = setTimeout(() => persistSetting(state, true), 400)
    return
  }
  try {
    const saveStr = JSON.stringify(state)
    if (saveStr != settingstr) {
      const settingConfig = getUserDataPath('setting.config')
      writeFileSync(settingConfig, saveStr, 'utf-8')
      settingstr = saveStr
    }
  } catch (err: any) {
    DebugLog.mSaveDanger('SaveSettingToJson', err)
  }
}

if (typeof window !== 'undefined') window.addEventListener('beforeunload', () => {
  if (activeState) persistSetting(activeState, true)
})

export function LoadSetting() {
  const state = createDefaultSetting()
  try {
    const settingConfig = getUserDataPath('setting.config')
    if (settingConfig && existsSync(settingConfig)) {
      settingstr = readFileSync(settingConfig, 'utf-8')
      applyStoredConfig(state, JSON.parse(settingstr))
      useAppStore().toggleTheme(state.uiTheme)
    }
  } catch (err) {
    // 配置文件损坏时先备份再用默认值重建，避免直接覆盖用户数据
    try {
      const settingConfig = getUserDataPath('setting.config')
      if (settingConfig && existsSync(settingConfig)) copyFileSync(settingConfig, `${settingConfig}.bak`)
    } catch {}
    DebugLog.mSaveDanger('LoadSetting', err)
  }
  DebugLog.configure({ enabled: state.debugLogEnabled, level: state.debugLogLevel, maxSizeMB: state.debugLogMaxSizeMB })
  activeState = state
  // 首次运行或配置发生过迁移时，把规范化后的结果回写磁盘
  persistSetting(state, true)
  return state
}

const useSettingStore = defineStore('setting', {
  state: (): SettingState => LoadSetting(),
  getters: {
    AriaIsLocal(state: SettingState): boolean {
      return state.ariaState == 'local'
    }
  },
  actions: {
    async updateStore(partial: Partial<SettingState>) {
      if (partial.uiTimeFolderFormate) {
        partial.uiTimeFolderFormate = partial.uiTimeFolderFormate
          .replace('mm-dd', 'MM-dd').replace('HH-MM', 'HH-mm')
      }
      this.$patch(partial)
      if (Object.hasOwn(partial, 'debugLogEnabled') || Object.hasOwn(partial, 'debugLogLevel') || Object.hasOwn(partial, 'debugLogMaxSizeMB')) {
        DebugLog.configure({ enabled: this.debugLogEnabled, level: this.debugLogLevel, maxSizeMB: this.debugLogMaxSizeMB })
      }
      if (Object.hasOwn(partial, 'uiLaunchStart')) {
        window.WebToElectron({ cmd: { launchStart: this.uiLaunchStart, launchStartShow: this.uiLaunchStartShow } })
      }
      if (Object.hasOwn(partial, 'uiFileOrderDuli')) {
        await PanDAL.aReLoadOneDirToShow('', 'refresh', false)
      }
      if (Object.hasOwn(partial, 'proxyType')) {
        await this.WebSetProxy()
      }
      if (Object.hasOwn(partial, 'downSaveShowPro') && !this.downSaveShowPro) {
        window.WebToElectron?.({ cmd: 'downloadProgress', progress: -1, activeCount: 0, totalCount: 0 })
      }
      persistSetting(this.$state)
      useAppStore().toggleTheme(this.uiTheme)
      window.MainProxyHost = this.debugProxyHost
      window.MainProxyPort = this.debugProxyPort
      window.WinMsgToUpload({ cmd: 'SettingRefresh' })
    },
    updateFileColor(key: string, title: string) {
      if (!key) return
      const arr = this.uiFileColorArray.map((item) => (item.key == key ? { ...item, title } : item))
      this.$patch({ uiFileColorArray: arr })
      persistSetting(this.$state)
    },
    async WebSetProxy(): Promise<boolean> {
      if (this.proxyType === 'system') return await window.WebSetProxy({ mode: 'system' })
      if (this.proxyType === 'none') return await window.WebSetProxy({ mode: 'direct' })
      const host = this.proxyHost.trim()
      if (!host || !this.proxyPort) return false
      const username = this.proxyUserName.trim()
      const auth = username ? `${encodeURIComponent(username)}${this.proxyPassword ? `:${encodeURIComponent(this.proxyPassword)}` : ''}@` : ''
      const proxyUrl = `${this.proxyType}://${auth}${host}:${this.proxyPort}`
      return await window.WebSetProxy({ mode: 'fixed_servers', proxyUrl })
    }
  }
})

export default useSettingStore
