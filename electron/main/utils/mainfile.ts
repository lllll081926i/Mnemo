import { app } from 'electron'
import path from 'path'
import { copyFileSync, existsSync, rmSync } from 'fs'
import is from 'electron-is'

let NewCopyed = false
let NewSaved = false

export function getAsarPath(fileName: string) {
  if (is.dev()) {
    const basePath = path.resolve(app.getAppPath())
    return path.join(basePath, fileName)
  } else {
    const basePath = path.resolve(app.getAppPath())
    const baseNew = path.join(basePath, '..', 'app.new')
    const baseSave = path.join(basePath, '..', 'app.asar')
    if (!NewCopyed) {
      // 热更新asar
      if (existsSync(baseNew)) {
        try {
          console.log('copyFileSync', baseNew, '-->', baseSave)
          copyFileSync(baseNew, baseSave)
          rmSync(baseNew, { force: true })
          NewCopyed = true
        } catch (err: any) {
          console.log(err)
        }
      }
    }
    if (!NewSaved) NewSaved = existsSync(baseSave)
    if (NewSaved) return path.join(baseSave, fileName)
    return path.join(basePath, fileName)
  }
}

export function getPreloadPath() {
  if (is.dev()) return path.join(path.resolve(app.getAppPath()), 'dev-electron', 'preload', 'index.js')
  return getAsarPath('dist/electron/preload/index.js')
}

export function getResourcesPath(fileName: string) {
  let basePath = path.resolve(app.getAppPath(), '..')
  if (is.dev()) basePath = path.resolve(app.getAppPath(), '.')
  return path.join(basePath, fileName)
}

function resolveStaticBasePath(): string {
  // Prefer Electron's own packaged flag — more reliable than electron-is under vite-plugin-electron.
  if (!app.isPackaged) {
    const fromAppPath = path.resolve(app.getAppPath(), 'static')
    if (existsSync(fromAppPath)) return fromAppPath
    const fromCwd = path.resolve(process.cwd(), 'static')
    if (existsSync(fromCwd)) return fromCwd
    return fromAppPath
  }
  // Packaged: resources/ next to app.asar
  return process.resourcesPath || path.resolve(app.getAppPath(), '..')
}

export function getStaticPath(fileName: string) {
  const basePath = resolveStaticBasePath()
  if (fileName.startsWith('icon')) {
    if (fileName === 'icon_256x256.ico' && !is.windows()) {
      fileName = path.join('images', 'icon_30x30.png')
    } else {
      fileName = path.join('images', fileName)
    }
  }
  return path.join(basePath, fileName)
}

/** Resolve the first existing Mnemo app icon for BrowserWindow / About. */
export function getAppIconPath(): string {
  const candidates = is.windows()
    ? ['icon_256x256.ico', 'icon_256x256.png', 'icon_64x64.png']
    : is.macOS()
      ? ['icon.icns', 'icon_256x256.png', 'icon_64x64.png', 'icon_30x30.png']
      : ['icon_256x256.png', 'icon_64x64.png', 'icon_30x30.png']

  for (const name of candidates) {
    const full = getStaticPath(name)
    if (existsSync(full)) return full
  }

  return getStaticPath(is.windows() ? 'icon_256x256.ico' : 'icon_256x256.png')
}

/**
 * Windows 系统托盘必须用 .ico（Electron 官方建议）。
 * 开发 / 打包统一锁死 icon_256x256.ico。
 */
export function getTrayIconPath(): string {
  const candidates: string[] = []
  candidates.push(getStaticPath('icon_256x256.ico'))
  if (!app.isPackaged) {
    candidates.push(path.resolve(process.cwd(), 'static', 'images', 'icon_256x256.ico'))
    candidates.push(path.resolve(app.getAppPath(), 'static', 'images', 'icon_256x256.ico'))
  } else if (process.resourcesPath) {
    candidates.push(path.join(process.resourcesPath, 'images', 'icon_256x256.ico'))
  }
  for (const p of candidates) {
    if (p && existsSync(p)) return p
  }
  return candidates[0]
}

export function getUserDataPath(fileName: string) {
  return path.join(app.getPath('userData'), fileName)
}
