import { useSettingStore } from '../store'
import DebugLog from './debuglog'
import { getUserData } from './electronhelper'
import { FileSystemErrorMessage } from './filehelper'
import { humanSize, Sleep } from './format'
import message from './message'

import path from 'path'
import fsPromises from 'fs/promises'

export default class AppCache {

  static async LoadDirSize(dir: string): Promise<number> {
    try {
      const childFiles = await fsPromises.readdir(dir, { withFileTypes: true }).catch((err: any) => {
        err = FileSystemErrorMessage(err.code, err.message)
        DebugLog.mSaveDanger('LoadDirSize失败：' + dir, err)
        message.error(err + ' ' + dir)
        return []
      })
      let total = 0
      for (let i = 0, maxi = childFiles.length; i < maxi; i++) {
        if (childFiles[i].isFile()) {
          const stat = await fsPromises.lstat(path.join(dir, childFiles[i].name)).catch(() => {
            return { size: 0 }
          })
          total += stat.size
        } else if (childFiles[i].isDirectory()) {
          total += await AppCache.LoadDirSize(path.join(dir, childFiles[i].name))
        }
      }
      return total
    } catch {
      return 0
    }
  }

  static async LoadCacheDirSize(dir: string): Promise<number> {
    try {
      const childFiles = await fsPromises.readdir(dir, { withFileTypes: true }).catch((err: any) => {
        err = FileSystemErrorMessage(err.code, err.message)
        DebugLog.mSaveDanger('LoadCacheDirSize失败：' + dir, err)
        message.error(err + ' ' + dir)
        return []
      })
      let total = 0
      for (let i = 0, maxi = childFiles.length; i < maxi; i++) {
        if (childFiles[i].isFile()) {
          const stat = await fsPromises.lstat(path.join(dir, childFiles[i].name)).catch(() => {
            return { size: 0 }
          })
          total += stat.size
        } else if (childFiles[i].isDirectory()) {
          total += await AppCache.LoadDirSize(path.join(dir, childFiles[i].name))
        }
      }
      return total
    } catch {
      return 0
    }
  }

  static DeleteDir(dir: string): Promise<void> {
    return fsPromises
      .rm(dir, { force: true, recursive: true })
      .then(() => {})
      .catch(() => {})
  }


  static async aLoadDirSize(): Promise<void> {
    const userData = getUserData()
    if (!userData) return
    const dirSize = await AppCache.LoadDirSize(userData)
    if (dirSize > 800 * 1024 * 1024) message.warning('本地数据已超过 800MB，可在设置的“高级”页面清理')
    useSettingStore().debugDirSize = humanSize(dirSize)
  }

  static async aLoadCacheSize(): Promise<void> {
    const userData = getUserData()
    if (!userData) return
    const cacheSize = await AppCache.LoadCacheDirSize(path.join(userData, 'Cache'))
    if (cacheSize > 800 * 1024 * 1024) message.warning('缓存已超过 800MB，可在设置的“高级”页面清理')
    useSettingStore().debugCacheSize = humanSize(cacheSize)
  }
  
  static async aClearDir(delby: string): Promise<void> {
    const dir = getUserData()
    if (delby == 'all') {
      // window.WebClearCache({ cache: true })
      if (window.WebClearCache)
        window.WebClearCache({
          storages: ['appcache', 'cookies', 'filesystem', 'shadercache', 'serviceworkers', 'cachestorage', 'indexdb', 'localstorage', 'websql'],
          quotas: ['temporary', 'persistent', 'syncable']
        })
    } else {
      // window.WebClearCache({ cache: true })
      if (window.WebClearCache)
        window.WebClearCache({
          storages: ['appcache', 'cookies', 'filesystem', 'shadercache', 'serviceworkers', 'cachestorage'],
          quotas: ['temporary', 'persistent', 'syncable']
        })
    }
    if (delby == 'all') {
      await AppCache.DeleteDir(path.join(dir, 'databases')).catch(() => {})
      await AppCache.DeleteDir(path.join(dir, 'IndexedDB')).catch(() => {})
      await AppCache.DeleteDir(path.join(dir, 'Local Storage')).catch(() => {})
      await AppCache.DeleteDir(path.join(dir, 'Session Storage')).catch(() => {})
    } else if (delby == 'db') {
      await AppCache.DeleteDir(path.join(dir, 'databases')).catch(() => {})
    }
    await AppCache.DeleteDir(path.join(dir, 'Code Cache', 'js')).catch(() => {})
    await AppCache.DeleteDir(path.join(dir, 'Code Cache', 'wasm')).catch(() => {})

    await Sleep(4000)


    if (delby == 'all') {
      message.success('全部本地数据已删除，应用将自动重启')
      Sleep(3000).then(() => {
        window.WebRelaunch()
      })
    } else if (delby == 'db') {
      message.success('本地数据库已删除，应用将自动重启')
      Sleep(3000).then(() => {
        window.WebRelaunch()
      })
    } else {
      message.success('缓存已清理，应用将自动重启')
      Sleep(3000).then(() => {
        // window.WebReload()
        window.WebRelaunch()
      })
    }
  }

  static async aClearCache(): Promise<void> {
    const dir = getUserData()
    await AppCache.DeleteDir(path.join(dir, 'Cache')).catch(() => {})
    message.success('全部缓存已删除，应用将自动重启')
    Sleep(1500).then(() => {
      window.WebRelaunch()
    })
  }
}
