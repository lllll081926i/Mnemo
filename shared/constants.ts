export const EMPTY_STRING = ''
export const PORTABLE_EXECUTABLE_DIR = process.env.PORTABLE_EXECUTABLE_DIR || ''
export const IS_PORTABLE = !!PORTABLE_EXECUTABLE_DIR

export const APP_THEME = { AUTO: 'auto', LIGHT: 'light', DARK: 'dark' } as const
export const APP_RUN_MODE = { STANDARD: 1, TRAY: 2, HIDE_TRAY: 3 } as const
export const ADD_TASK_TYPE = { URI: 'uri' } as const
export const TASK_STATUS = {
  ACTIVE: 'active', WAITING: 'waiting', PAUSED: 'paused',
  ERROR: 'error', COMPLETE: 'complete', REMOVED: 'removed'
} as const

export const LOG_LEVELS = ['error', 'warn', 'info', 'verbose', 'debug', 'silly']
export const MAX_NUM_OF_DIRECTORIES = 5

export const ENGINE_RPC_HOST = '127.0.0.1'
export const ENGINE_RPC_PORT = 16800
export const ENGINE_MAX_CONCURRENT_DOWNLOADS = 10
export const ENGINE_MAX_CONNECTION_PER_SERVER = 64

export const GRAPHIC = '░▒▓█'

export const ONE_SECOND = 1000
export const ONE_MINUTE = 60 * ONE_SECOND
export const ONE_HOUR = 60 * ONE_MINUTE
export const ONE_DAY = 24 * ONE_HOUR
export const AUTO_CHECK_UPDATE_INTERVAL = ONE_DAY * 7

export const PROXY_SCOPES = {
  DOWNLOAD: 'download',
  UPDATE_APP: 'update-app'
} as const
export type ProxyScopeType = typeof PROXY_SCOPES[keyof typeof PROXY_SCOPES]
export const PROXY_SCOPE_OPTIONS = [PROXY_SCOPES.DOWNLOAD, PROXY_SCOPES.UPDATE_APP]
export const LOGIN_SETTING_OPTIONS = { args: ['--opened-at-login=1'] }

export const TRAY_CANVAS_CONFIG = {
  WIDTH: 66, HEIGHT: 16, ICON_WIDTH: 16, ICON_HEIGHT: 16,
  TEXT_WIDTH: 46, TEXT_FONT_SIZE: 8
}

export const RESOURCE_TAGS = ['http://', 'https://']

export const SUPPORT_RTL_LOCALES = ['ar', 'fa', 'he', 'ku', 'pa', 'ps', 'sd', 'ur', 'yi']

export const IMAGE_SUFFIXES = ['.ai', '.bmp', '.eps', '.fig', '.gif', '.heic', '.icn', '.ico', '.jpeg', '.jpg', '.png', '.psd', '.raw', '.sketch', '.svg', '.tif', '.webp', '.xd']
export const AUDIO_SUFFIXES = ['.aac', '.ape', '.flac', '.flav', '.m4a', '.mp3', '.ogg', '.wav', '.wma']
export const VIDEO_SUFFIXES = ['.avi', '.m4v', '.mkv', '.mov', '.mp4', '.mpg', '.rmvb', '.vob', '.wmv']
export const SUB_SUFFIXES = ['.ass', '.idx', '.smi', '.srt', '.ssa', '.sst', '.sub']
export const DOCUMENT_SUFFIXES = ['.azw3', '.csv', '.doc', '.docx', '.epub', '.key', '.mobi', '.numbers', '.pages', '.pdf', '.ppt', '.pptx', '.txt', '.xsl', '.xslx']
