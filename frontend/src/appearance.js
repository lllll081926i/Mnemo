// 外观应用：主题（明/暗）。强调色固定为品牌紫，不再提供色包切换。
import { WindowSetDarkTheme, WindowSetLightTheme, WindowSetSystemDefaultTheme } from '../wailsjs/runtime/runtime'

export function isDarkMode(theme) {
  return theme === 'dark' || (theme !== 'light' && window.matchMedia('(prefers-color-scheme: dark)').matches)
}

// 一次性应用主题；theme 为后端设置值（system/light/dark）
export function applyAppearance(theme) {
  const dark = isDarkMode(theme)
  document.documentElement.classList.toggle('dark', dark)
  // 清理旧版色包残留
  localStorage.removeItem('mnemo.themePackLight')
  localStorage.removeItem('mnemo.themePackDark')
  // 系统标题栏跟随主题（Wails Windows 主题 API）
  try {
    if (theme === 'dark') WindowSetDarkTheme()
    else if (theme === 'light') WindowSetLightTheme()
    else WindowSetSystemDefaultTheme()
  } catch { /* 非 Windows 或旧版 runtime 时忽略 */ }
  return dark
}

// ---------- 纯前端偏好（后端 Settings 无对应字段，存 localStorage） ----------
const PREFS_KEY = 'mnemo.prefs'
const LAST_DRIVE_KEY = 'mnemo.lastDrive'
const PREFS_DEFAULTS = {
  viewMode: 'list',       // 网盘默认视图 list | grid
  hoverPreview: true,     // 目录树悬停预览
  downloadSound: true,    // 传输完成提示音
  defaultVolume: 100,     // 播放器默认音量 0-200
  defaultSpeed: 1,        // 播放器默认倍速
  seekStep: 10,           // 快进/快退步长（秒）
  autoCloseOnEnd: false,  // 播放到结尾自动收起控制条
  hardwareDecode: 'auto', // 硬件解码: auto | d3d11va | no
  autoLoadSubtitles: true,// 自动加载同名字幕
  defaultSortKey: 'name', // 默认排序键: name | time | size
  defaultSortAsc: true,   // 默认排序方向: true=升序 | false=降序
  sideWidth: 220,         // 网盘侧边栏宽度（px）
}

export function getPrefs() {
  try {
    return { ...PREFS_DEFAULTS, ...(JSON.parse(localStorage.getItem(PREFS_KEY) || '{}') || {}) }
  } catch { return { ...PREFS_DEFAULTS } }
}

export function setPref(key, value) {
  const p = getPrefs()
  p[key] = value
  localStorage.setItem(PREFS_KEY, JSON.stringify(p))
  // localStorage is not reactive. Let mounted views update immediately when a
  // preference is changed from the settings page.
  if (typeof window !== 'undefined') {
    window.dispatchEvent(new CustomEvent('mnemo:prefs-changed', { detail: { key, value } }))
  }
}

// ---------- 当前网盘（跨应用重启恢复，不包含任何凭据） ----------
export function getLastDriveSelection() {
  try {
    const value = JSON.parse(localStorage.getItem(LAST_DRIVE_KEY) || 'null')
    if (!value || typeof value !== 'object') return null
    const userId = String(value.userId || '').trim()
    const driveId = String(value.driveId || '').trim()
    return userId ? { userId, driveId } : null
  } catch {
    return null
  }
}

export function setLastDriveSelection(userId, driveId) {
  const normalizedUserId = String(userId || '').trim()
  if (!normalizedUserId) return
  try {
    localStorage.setItem(LAST_DRIVE_KEY, JSON.stringify({
      userId: normalizedUserId,
      driveId: String(driveId || '').trim(),
    }))
  } catch { /* 存储不可用时不影响网盘操作 */ }
}

export function clearLastDriveSelection() {
  try { localStorage.removeItem(LAST_DRIVE_KEY) } catch { /* noop */ }
}
