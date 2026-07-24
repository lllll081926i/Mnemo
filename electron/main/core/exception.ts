import is from 'electron-is'
import { ShowError } from './dialog'
import { app } from 'electron'

export default class exception {
  private constructor() {}

  static handler() {
    if (is.dev()) {
      return
    }
    process.on('unhandledRejection', (reason, p) => {
      console.error('[mnemo] Unhandled Rejection at:', p, 'reason:', reason)
    })
    process.on('uncaughtException', (err) => {
      const message = err?.message || String(err)
      const stack = err?.stack || ''
      console.error('[mnemo] uncaughtException:', message, stack)
      // 不要在任意未知异常上强制退出 / 重启，否则表现为「报错后闪退」
      if (app.isReady()) {
        try {
          ShowError('发生未处理的异常', message + (stack ? '\n' + stack : ''))
        } catch (showErr) {
          console.error('[mnemo] failed to show exception dialog', showErr)
        }
      }
    })
  }
}