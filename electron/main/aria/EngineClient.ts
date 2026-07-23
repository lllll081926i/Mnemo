import { ENGINE_RPC_HOST, ENGINE_RPC_PORT, EMPTY_STRING } from '@shared/constants'
import { formatOptionsForEngine } from '@shared/utils'
import logger from './Logger'

export interface EngineClientOptions {
  host?: string
  port?: number
  secret?: string
}

export interface EngineReadinessOptions {
  attempts?: number
  delayMs?: number
}

const delay = (milliseconds: number) => new Promise<void>((resolve) => setTimeout(resolve, milliseconds))

export default class EngineClient {
  options: Required<EngineClientOptions>
  client!: any
  private clientPromise?: Promise<any>

  constructor (options: EngineClientOptions = {}) {
    this.options = {
      host: options.host || ENGINE_RPC_HOST,
      port: options.port || ENGINE_RPC_PORT,
      secret: options.secret ?? EMPTY_STRING
    }
  }

  init (): void { this.connect() }

  connect (): void {
    const { host, port, secret } = this.options
    ;(globalThis as any).self ||= globalThis
    // @ts-ignore
    this.clientPromise = import('aria2-lib').then(({ default: Aria2 }) => {
      this.client = new Aria2({ host, port, secret } as any)
      return this.client
    })
  }

  async call (method: string, ...args: any[]): Promise<any> {
    if (!this.clientPromise) this.connect()
    const client = await this.clientPromise
    try {
      return await client.call(method, ...args)
    } catch (err: any) {
      logger.warn(`[motrix] aria2 RPC ${method} fail: ${err?.message}`)
      throw err
    }
  }

  async waitUntilReady (options: EngineReadinessOptions = {}): Promise<void> {
    if (!this.clientPromise) this.connect()
    const client = await this.clientPromise
    const attempts = Math.max(1, options.attempts ?? 30)
    const delayMs = Math.max(0, options.delayMs ?? 100)
    let lastError: any
    for (let attempt = 0; attempt < attempts; attempt++) {
      try {
        await client.call('getVersion')
        return
      } catch (error: any) {
        lastError = error
        if (attempt + 1 < attempts && delayMs > 0) await delay(delayMs)
      }
    }
    const error = lastError instanceof Error ? lastError : new Error(String(lastError || 'aria2 RPC unavailable'))
    logger.warn(`[motrix] aria2 RPC readiness failed after ${attempts} attempts: ${error.message}`)
    throw error
  }

  async changeGlobalOption (options: Record<string, any>, readinessOptions: EngineReadinessOptions = {}): Promise<any> {
    await this.waitUntilReady(readinessOptions)
    const args = formatOptionsForEngine(options)
    return this.call('changeGlobalOption', args)
  }

  async shutdown (options: { force?: boolean } = {}): Promise<any> {
    const method = options.force ? 'forceShutdown' : 'shutdown'
    return this.call(method)
  }
}
