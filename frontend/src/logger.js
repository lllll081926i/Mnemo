const prefix = '[mnemo]'

function write(level, scope, message, fields) {
  const method = console[level] || console.log
  const label = `${prefix}[${scope}] ${message}`
  if (fields && Object.keys(fields).length) method.call(console, label, fields)
  else method.call(console, label)
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
