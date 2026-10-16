import { dispatchDriveNotice, driveErrorNotice } from './workspace'

export function taskErrorText(reason, action, issue) {
  return issue?.message || (reason ? driveErrorNotice(reason, action).message : '')
}

export function createTaskFailureNotifier() {
  const notified = new Set()
  return {
    clearTaskFailure: key => notified.delete(key),
    notifyTaskFailure(key, reason, action, issue) {
      if ((!reason && !issue?.message) || notified.has(key)) return
      notified.add(key)
      const categories = { NEEDS_AUTH: 'auth', REMOTE_ACCESS_REJECTED: 'permission', PROVIDER_COOLDOWN: 'limited', NETWORK_UNAVAILABLE: 'network', REMOTE_NOT_FOUND: 'notFound', BLOCKED_BY_CONFLICT: 'conflict', UNSUPPORTED: 'unsupported' }
      dispatchDriveNotice(issue?.message
        ? { category: categories[issue.code] || 'unknown', type: 'warn', message: issue.message }
        : driveErrorNotice(reason, action))
    },
  }
}
