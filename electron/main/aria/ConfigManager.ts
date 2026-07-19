import { app } from 'electron'
import is from 'electron-is'
import Store from 'electron-store'
import { APP_THEME, APP_RUN_MODE, ENGINE_RPC_PORT, ENGINE_MAX_CONCURRENT_DOWNLOADS } from '@shared/constants'
import { CHROME_UA } from '@shared/ua'
import { userKeys, systemKeys } from '@shared/configKeys'
import { separateConfig } from '@shared/utils'
import { getConfigBasePath, getMaxConnectionPerServer, getUserDownloadsPath } from './utils'
import logger from './Logger'

export default class ConfigManager {
  systemConfig!: InstanceType<typeof Store>
  userConfig!: InstanceType<typeof Store>

  constructor () { this.init() }

  init () {
    this.initUserConfig()
    this.initSystemConfig()
  }

  initSystemConfig () {
    this.systemConfig = new Store({
      name: 'system',
      cwd: getConfigBasePath(),
      defaults: {
        'all-proxy': '',
        'allow-overwrite': false,
        'auto-file-renaming': true,
        'continue': true,
        'dir': getUserDownloadsPath(),
        'follow-metalink': true,
        'max-concurrent-downloads': ENGINE_MAX_CONCURRENT_DOWNLOADS,
        'max-connection-per-server': getMaxConnectionPerServer(),
        'max-download-limit': 0,
        'max-overall-download-limit': 0,
        'max-overall-upload-limit': 0,
        'no-proxy': '',
        'pause': true,
        'rpc-listen-port': ENGINE_RPC_PORT,
        'rpc-secret': 'S4znWTaZYQi3cpRNb',
        'split': getMaxConnectionPerServer(),
        'user-agent': CHROME_UA
      }
    } as any)
    this.fixSystemConfig()
  }

  initUserConfig () {
    this.userConfig = new Store({
      name: 'user',
      cwd: getConfigBasePath(),
      defaults: {
        'auto-check-update': true,
        'auto-hide-window': false,
        'cookie': '',
        'engine-bin-path': '',
        'engine-max-connection-per-server': getMaxConnectionPerServer(),
        'favorite-directories': [],
        'hide-app-menu': false,
        'history-directories': [],
        'keep-window-state': true,
        'last-check-update-time': 0,
        'locale': app.getLocale(),
        'log-level': 'info',
        'open-at-login': false,
        'protocols': { mo: true, motrix: true },
        'proxy': { enable: false, server: '', bypass: '', scope: ['download'] },
        'resume-all-when-app-launched': false,
        'run-mode': APP_RUN_MODE.STANDARD,
        'show-progress-bar': true,
        'task-notification': true,
        'theme': APP_THEME.AUTO,
        'tray-speedometer': true
      }
    } as any)
    this.fixUserConfig()
  }

  fixSystemConfig () {
    if (!this.systemConfig) return
    const all = this.systemConfig.store as Record<string, any>
    for (const k of Object.keys(all)) {
      if (!systemKeys.includes(k)) (this.systemConfig as any).delete(k)
    }
  }

  fixUserConfig () {
    if (!this.userConfig) return
    const all = this.userConfig.store as Record<string, any>
    for (const key of Object.keys(all)) {
      if (!userKeys.includes(key)) (this.userConfig as any).delete(key)
    }
    const protocols = this.userConfig.get('protocols', {}) as Record<string, boolean>
    this.userConfig.set('protocols', {
      mo: protocols.mo !== false,
      motrix: protocols.motrix !== false
    })
    const proxy = this.userConfig.get('proxy', {}) as Record<string, any>
    if (Array.isArray(proxy.scope)) {
      this.userConfig.set('proxy', {
        ...proxy,
        scope: proxy.scope.filter((scope: string) => scope === 'download' || scope === 'update-app')
      })
    }
    try {
      const settings = app.getLoginItemSettings()
      this.userConfig.set('open-at-login', !!settings.openAtLogin)
    } catch {}
  }

  getSystemConfig (key?: string, def?: any): any {
    if (!key) return this.systemConfig.store
    return this.systemConfig.get(key, def)
  }

  getUserConfig (key?: string, def?: any): any {
    if (!key) return this.userConfig.store
    return this.userConfig.get(key, def)
  }

  getLocale (): string { return (this.userConfig.get('locale') as string) || app.getLocale() }

  setSystemConfig (...args: any[]): void {
    if (args.length === 1 && typeof args[0] === 'object') this.systemConfig.set(args[0])
    else (this.systemConfig.set as any)(args[0], args[1])
  }

  setUserConfig (...args: any[]): void {
    if (args.length === 1 && typeof args[0] === 'object') this.userConfig.set(args[0])
    else (this.userConfig.set as any)(args[0], args[1])
  }

  reset () {
    this.systemConfig.clear()
    this.userConfig.clear()
  }
}
