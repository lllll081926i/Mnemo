// 前端日志也会写入后端统一日志。这里先做一次字段收敛和短时间去重：
// 同一个渲染错误、轮询失败或连续点击不会把日志文件和 Wails RPC 刷满；
// 不同动作仍保留完整的「开始/完成/失败」链路。
const recentLogs = new Map()
const recentWindowMs = { debug: 700, info: 900, warn: 2500, error: 5000 }
const sensitiveField = /(?:password|passwd|cookie|authorization|captcha|token|secret|credential|device[_-]?id)/i
const transientField = /(?:duration|elapsed|time|timestamp|count)$/i
const maxRecentLogs = 512

function compactText(value, max = 240) {
  let text
  if (value instanceof Error) text = value.message || value.name
  else if (typeof value === 'string') text = value
  else if (value === null || value === undefined) text = ''
  else {
    try { text = typeof value === 'object' ? JSON.stringify(value) : String(value) } catch { text = String(value) }
  }
  text = String(text || '').replace(/\s+/g, ' ').trim()
  return text.length > max ? `${text.slice(0, max)}…` : text
}

function compactFields(fields) {
  const out = {}
  const entries = Object.entries(fields || {})
  for (const [index, [rawKey, rawValue]] of entries.entries()) {
    if (index >= 12) {
      out.omitted_fields = String(entries.length - index)
      break
    }
    const key = compactText(rawKey, 64)
    if (!key) continue
    const isPresence = /^has[_-]/i.test(key) || /(?:_present|_configured)$/i.test(key)
    out[key] = sensitiveField.test(key) && !isPresence ? '[REDACTED]' : compactText(rawValue)
  }
  return out
}

function logFingerprint(level, scope, message, fields) {
  const stableFields = Object.entries(fields)
    .filter(([key]) => !transientField.test(key))
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([key, value]) => `${key}=${value}`)
    .join('&')
  return `${level}|${scope}|${message}|${stableFields}`
}

function isRepeated(level, scope, message, fields) {
  const now = Date.now()
  const key = logFingerprint(level, scope, message, fields)
  const previous = recentLogs.get(key)
  recentLogs.set(key, now)

  // 有上限地回收，避免长时间运行后保存无限个日志指纹。
  if (recentLogs.size > maxRecentLogs) {
    const expiry = now - Math.max(...Object.values(recentWindowMs)) * 3
    for (const [entry, at] of recentLogs) {
      if (at < expiry || recentLogs.size > maxRecentLogs) recentLogs.delete(entry)
    }
  }
  return previous !== undefined && now - previous < (recentWindowMs[level] || recentWindowMs.info)
}

function write(level, scope, message, fields) {
  const normalizedLevel = String(level || 'info').toLowerCase()
  const normalizedScope = compactText(scope || 'frontend', 64) || 'frontend'
  const normalizedMessage = compactText(message || '事件', 160) || '事件'
  const safeFields = compactFields(fields)
  if (isRepeated(normalizedLevel, normalizedScope, normalizedMessage, safeFields)) return false

  const method = console[normalizedLevel] || console.log
  const label = `[${normalizedLevel.toUpperCase()}] ${normalizedMessage} | scope=${normalizedScope}`
  if (Object.keys(safeFields).length) method.call(console, label, safeFields)
  else method.call(console, label)

  try {
    const bridge = typeof window !== 'undefined' ? window.go?.app?.App?.LogFrontend : null
    if (typeof bridge === 'function') {
      void Promise.resolve(bridge(normalizedLevel, normalizedScope, normalizedMessage, safeFields)).catch(() => {})
    }
  } catch { /* browser preview has no Wails bridge */ }
  return true
}
export function debug(scope, message, fields) { write('debug', scope, message, fields) }
export function info(scope, message, fields) { write('info', scope, message, fields) }
export function warn(scope, message, fields) { write('warn', scope, message, fields) }
export function error(scope, message, fields) { write('error', scope, message, fields) }

export function errorText(value) {
  if (value instanceof Error) return value.message || value.name
  return String(value ?? '')
}

export function configKeys(config) {
  return Object.keys(config || {}).sort()
}

export function installGlobalErrorLogging() {
  const onError = (event) => {
    error('runtime', 'uncaught frontend error', {
      message: event?.message || 'unknown error',
      source: event?.filename || '',
      line: event?.lineno || 0,
      column: event?.colno || 0,
    })
  }
  const onRejection = (event) => {
    error('runtime', 'unhandled promise rejection', { reason: errorText(event?.reason) })
  }
  window.addEventListener('error', onError)
  window.addEventListener('unhandledrejection', onRejection)
  return () => {
    window.removeEventListener('error', onError)
    window.removeEventListener('unhandledrejection', onRejection)
  }
}
