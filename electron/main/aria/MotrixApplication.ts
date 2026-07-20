import { EventEmitter } from 'node:events'
import { app, ipcMain, BrowserWindow, dialog } from 'electron'
import { isEmpty } from 'lodash'
import ConfigManager from './ConfigManager'
import Engine from './Engine'
import EngineClient from './EngineClient'
import EnergyManager from './EnergyManager'
import ProtocolManager from './ProtocolManager'
import Context from './Context'
import logger from './Logger'

export default class MotrixApplication extends EventEmitter {
  configManager!: ConfigManager
  engine!: Engine
  engineClient!: EngineClient
  energyManager!: EnergyManager
  protocolManager!: ProtocolManager
  context!: Context
  private initialized = false
  private engineReady: Promise<void> | undefined

  async init (): Promise<void> {
    if (this.initialized) return
    this.initContext()
    this.initConfigManager()
    this.initEnergyManager()
    this.initProtocolManager()
    this.handleConfigChanges()
    this.handleIpc()
    ;(globalThis as any).motrixApplication = this
    this.initialized = true
    this.emit('application:initialized')
    setTimeout(() => this.afterInit(), 0)
  }

  async ensureEngineReady (): Promise<void> {
    if (this.engineClient) return
    if (this.engineReady) return this.engineReady
    this.engineReady = Promise.resolve().then(() => {
      this.startEngine()
      this.initEngineClient()
    })
    return this.engineReady
  }

  initContext (): void { this.context = new Context() }
  initConfigManager (): void { this.configManager = new ConfigManager() }

  startEngine (): void {
    try {
      this.engine = new Engine({
        systemConfig: this.configManager.getSystemConfig(),
        userConfig: this.configManager.getUserConfig()
      })
      this.engine.start()
    } catch (err: any) {
      logger.error('[motrix] startEngine failed: ' + err.message)
      try {
        dialog.showMessageBox({ type: 'error', title: 'Engine Error', message: err.message })
      } catch {}
    }
  }

  async stopEngine (): Promise<void> {
    try { await this.engineClient?.shutdown({ force: true }) }
    catch (err: any) { logger.warn('[motrix] shutdown engine failed: ' + err.message) }
    setImmediate(() => { try { this.engine?.stop() } catch {} })
  }

  initEngineClient (): void {
    const port = this.configManager.getSystemConfig('rpc-listen-port')
    const secret = this.configManager.getSystemConfig('rpc-secret')
    this.engineClient = new EngineClient({ port, secret })
    this.engineClient.init()
  }

  initEnergyManager (): void {
    this.energyManager = new EnergyManager()
    this.on('download-status-change', (downloading: boolean) => {
      if (downloading) this.energyManager.startPowerSaveBlocker()
      else this.energyManager.stopPowerSaveBlocker()
    })
  }

  initProtocolManager (): void {
    const protocols = this.configManager.getUserConfig('protocols') as Record<string, boolean>
    this.protocolManager = new ProtocolManager({ protocols })
    this.protocolManager.init()
  }

  handleConfigChanges (): void {
    this.configManager.userConfig.onDidChange('proxy', async (newValue: any) => {
      const { enable, server, bypass, scope = [] } = newValue || {}
      const system = enable && server && scope.includes('download')
        ? { 'all-proxy': server, 'no-proxy': bypass || '' } : {}
      this.configManager.setSystemConfig(system)
      try { await this.engineClient.call('changeGlobalOption', system) } catch {}
    })
  }

  handleIpc (): void {
    ipcMain.handle('motrix:get-app-config', async () => ({
      ...this.configManager.getSystemConfig(),
      ...this.configManager.getUserConfig(),
      ...this.context.get()
    }))
    ipcMain.handle('motrix:set-config', async (_event, config: { system?: any; user?: any }) => {
      await this.savePreference(config)
    })
    ipcMain.on('motrix:command', (_event, command: string, ...args: any[]) => {
      this.emit(command, ...args)
    })
    ipcMain.on('motrix:event', (_event, eventName: string, ...args: any[]) => {
      this.emit(eventName, ...args)
    })
    this.on('application:save-preference', (config: any) => this.savePreference(config))
  }

  async savePreference (config: any = {}): Promise<void> {
    const { system, user } = config || {}
    if (!isEmpty(system)) {
      this.configManager.setSystemConfig(system)
      try { await this.engineClient.changeGlobalOption(system) } catch {}
    }
    if (!isEmpty(user)) this.configManager.setUserConfig(user)
  }

  sendCommandToAll (command: string, ...args: any[]): void {
    BrowserWindow.getAllWindows().forEach((w) => {
      try { w.webContents.send('motrix:command', command, ...args) } catch {}
    })
  }

  afterInit (): void {
    if (this.configManager.getUserConfig('resume-all-when-app-launched')) {
      this.ensureEngineReady().then(() => this.autoResumeTask()).catch((err: any) => logger.warn('[motrix] resume engine failed: ' + err.message))
    }
  }

  autoResumeTask (): void {
    this.engineClient?.call('unpauseAll').catch(() => undefined)
  }

  async quit (): Promise<void> {
    await this.stopEngine()
    try { this.energyManager?.stopPowerSaveBlocker() } catch {}
  }
}
