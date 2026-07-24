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

export function getStaticPath(fileName: string) {
  let basePath = path.resolve(app.getAppPath(), '..')
  if (is.dev()) basePath = path.resolve(app.getAppPath(), './static')
  if (fileName.startsWith('icon')) {
    if (fileName === 'icon_256x256.ico' && !is.windows()) {
      fileName = path.join('images', 'icon_30x30.png')
    } else {
      fileName = path.join('images', fileName)
    }
  }
  return path.join(basePath, fileName)
}

/** Resolve the first existing Mnemo app icon for BrowserWindow / Tray / About. */
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

  // Last resort: packaged resources images / dev static root
  return getStaticPath(is.windows() ? 'icon_256x256.ico' : 'icon_256x256.png')
}

export function getUserDataPath(fileName: string) {
  return path.join(app.getPath('userData'), fileName)
}
