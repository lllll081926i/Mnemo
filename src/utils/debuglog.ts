import { appendFile, stat, unlink } from 'node:fs/promises'
import { getUserDataPath } from './electronhelper'

export type DebugLogLevel = 'debug' | 'info' | 'warn' | 'error'

interface DebugLogOptions {
  enabled: boolean
  level: DebugLogLevel
  maxSizeMB: number
}

const levelWeight: Record<DebugLogLevel, number> = {
  debug: 10,
  info: 20,
  warn: 30,
  error: 40
}

const levelAliases: Record<string, DebugLogLevel> = {
  debug: 'debug',
  info: 'info',
  success: 'info',
  log: 'info',
  warning: 'warn',
  warn: 'warn',
  danger: 'error',
  error: 'error'
}

const normalizeError = (err: any): string => {
  if (!err) return ''
  if (typeof err === 'string') return err
  if (err instanceof Error) return `${err.message}${err.stack ? `\n${err.stack}` : ''}`
  try {
    return JSON.stringify(err)
  } catch {
    return 'stringify failed'
  }
}

class DebugLogC {
  private options: DebugLogOptions = { enabled: true, level: 'info', maxSizeMB: 5 }
  private writeQueue: Promise<void> = Promise.resolve()

  get logPath(): string {
    return getUserDataPath('mnemo.log')
  }

  configure(options: Partial<DebugLogOptions>) {
    // 始终写入日志：enabled 强制 true
    this.options.enabled = true
    if (options.level && levelAliases[options.level] === options.level) this.options.level = options.level
    if (typeof options.maxSizeMB === 'number' && Number.isFinite(options.maxSizeMB)) this.options.maxSizeMB = Math.min(100, Math.max(1, Math.round(options.maxSizeMB)))
  }

  mSaveDebug(logmessage: string, err: any = undefined) {
    this.mSaveLog('debug', logmessage, err)
  }

  mSaveInfo(logmessage: string, err: any = undefined) {
    this.mSaveLog('info', logmessage, err)
  }

  mSaveDanger(logmessage: string, err: any = undefined) {
    this.mSaveLog('danger', logmessage, err)
  }

  mSaveWarning(logmessage: string, err: any = undefined) {
    this.mSaveLog('warning', logmessage, err)
  }

  mSaveSuccess(logmessage: string, err: any = undefined) {
    this.mSaveLog('success', logmessage, err)
  }

  mSaveLog(logtype: string, logmessage: string, err: any = undefined) {
    const level = levelAliases[String(logtype || '').toLowerCase()] || 'info'
    if (!this.options.enabled || levelWeight[level] < levelWeight[this.options.level]) return
    const message = [String(logmessage || '').trim(), normalizeError(err)].filter(Boolean).join('\n')
    if (!message) return
    const line = `${new Date().toISOString()} [${level.toUpperCase()}] ${message}\n`
    this.writeQueue = this.writeQueue.then(() => this.append(line)).catch((writeError) => console.warn('Write log failed', writeError))
  }

  private async append(line: string) {
    if (!this.logPath) return
    const maxBytes = this.options.maxSizeMB * 1024 * 1024
    const incomingBytes = Buffer.byteLength(line, 'utf8')
    try {
      const current = await stat(this.logPath)
      if (current.size + incomingBytes >= maxBytes) await unlink(this.logPath)
    } catch (err: any) {
      if (err?.code !== 'ENOENT') throw err
    }
    await appendFile(this.logPath, line, 'utf8')
  }
}

const DebugLog = new DebugLogC()
export default DebugLog
