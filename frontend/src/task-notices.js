import { dispatchDriveNotice, driveErrorNotice } from './workspace'

export function taskErrorText(reason, action, issue) {
  return issue?.message || (reason ? driveErrorNotice(reason, action).message : '')
}

const actions = {
  reauthorize: '重新登录原账号后，手动恢复任务',
  network_settings: '检查网络和代理设置后，手动恢复任务',
  check_disk: '清理本地磁盘空间后重试',
  check_local_path: '请检查本地文件路径、权限或是否被占用',
  refresh_directory: '请刷新目标目录',
  check_target: '请先核对目标目录，确认文件不存在后再重试，避免重复上传',
  check_access: '请检查文件访问权限、链接有效期或账号状态',
  inspect_capability: '当前网盘不支持此操作',
  manual_resume: '请核对断点后手动继续任务',
  replace_source: '请重新获取下载链接',
  check_source: '请重新下载并校验文件',
  wait: '请稍后重试',
}

export function taskFailureNotice(reason, action, issue, context = {}) {
  const categories = { NEEDS_AUTH: 'auth', REMOTE_ACCESS_REJECTED: 'permission', PROVIDER_COOLDOWN: 'limited', NETWORK_UNAVAILABLE: 'network', REMOTE_NOT_FOUND: 'notFound', BLOCKED_BY_CONFLICT: 'conflict', UNSUPPORTED: 'unsupported' }
  const notice = issue?.message
    ? { category: categories[issue.code] || 'unknown', type: 'warn', message: issue.message }
    : driveErrorNotice(reason, action)
  if (issue?.code === 'USER_CANCELED') return { ...notice, silent: true }
  if (issue?.code === 'UPLOAD_RESULT_UNKNOWN') notice.message = '上传结果尚未确认，请先核对目标目录；确认文件不存在后，再选择“核对后重试”，避免重复上传'
  else if (issue?.code !== 'UPLOAD_CREDENTIAL_SAVE_FAILED' && actions[issue?.action] && !notice.message.includes(actions[issue.action])) notice.message += `。${actions[issue.action]}`
  const retryTime = new Date(Number(issue?.retryAt) * 1000)
  if (issue?.retryAt > 0 && Number.isFinite(retryTime.getTime())) notice.message += `。参考恢复时间：${retryTime.toLocaleString()}（到时不代表已解除限制）`
  const repair = ['reauthorize', 'network_settings'].includes(issue?.action)
  return { ...notice, action: repair ? { label: issue.action === 'reauthorize' ? '重新登录账号' : '网络设置', repair: issue.action, userId: context.userId } : { label: '查看任务', tab: 'transfer' } }
}

export function createTaskFailureNotifier() {
  const notified = new Map()
  return {
    clearTaskFailure: key => notified.delete(key),
    notifyTaskFailure(key, reason, action, issue, context) {
      if (!reason && !issue?.code && !issue?.message) return
      const fingerprint = JSON.stringify([reason, issue])
      if (notified.get(key) === fingerprint) return
      notified.set(key, fingerprint)
      if (notified.size > 500) notified.delete(notified.keys().next().value)
      dispatchDriveNotice(taskFailureNotice(reason, action, issue, context))
    },
  }
}

const transferNotifier = createTaskFailureNotifier()
export function notifyTransferFailure(event) {
  const task = event?.task
  if (!task?.id || !['download', 'upload'].includes(event.kind)) return
  const key = `${event.kind}:${task.id}`
  if (task.status === 'failed' && !task.groupId) transferNotifier.notifyTaskFailure(key, task.error, event.kind === 'upload' ? '上传文件' : '下载文件', task.issue, { userId: task.user_id })
  else transferNotifier.clearTaskFailure(key)
}

const migrateNotifier = createTaskFailureNotifier()
export function notifyMigrateFailure(job) {
  if (!job?.id) return
  const key = `migrate:${job.id}`
  const issue = job.issue?.action === 'reauthorize' ? { ...job.issue, action: 'check_access' } : job.issue
  if (['failed', 'partial'].includes(job.status)) migrateNotifier.notifyTaskFailure(key, job.message, '迁移文件', issue)
  else migrateNotifier.clearTaskFailure(key)
}
