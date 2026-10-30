import { describe, expect, it } from 'vitest'
import { createTaskFailureNotifier, notifyTransferFailure, notifyMigrateFailure, taskFailureNotice } from './task-notices'

function collect(run) {
  const notices = []
  const listener = event => notices.push(event.detail)
  window.addEventListener('mnemo:drive-notice', listener)
  try { run() } finally { window.removeEventListener('mnemo:drive-notice', listener) }
  return notices
}

describe('统一任务通知', () => {
  it('同一失败只通知一次，原因变化和恢复后再次失败重新通知', () => {
    const notifier = createTaskFailureNotifier()
    const notices = collect(() => {
      notifier.notifyTaskFailure('one', '网络断开', '上传文件')
      notifier.notifyTaskFailure('one', '网络断开', '上传文件')
      notifier.notifyTaskFailure('one', '权限不足', '上传文件')
      notifier.clearTaskFailure('one')
      notifier.notifyTaskFailure('one', '权限不足', '上传文件')
    })
    expect(notices).toHaveLength(3)
  })

  it('未打开传输页时也能通知，两个消费者共享去重状态', () => {
    const event = { kind: 'upload', task: { id: 'background-test', status: 'failed', user_id: 'original', error: '401', issue: { code: 'NEEDS_AUTH', message: '登录已失效', action: 'reauthorize' } } }
    const notices = collect(() => {
      notifyTransferFailure(event)
      notifyTransferFailure(event)
      notifyTransferFailure({ ...event, task: { ...event.task, status: 'uploading' } })
      notifyTransferFailure(event)
    })
    expect(notices).toHaveLength(2)
    expect(notices[0].action).toMatchObject({ repair: 'reauthorize', userId: 'original' })
  })

  it('迁移不把来源不明的凭据错误直接绑定到目标账号', () => {
    const notices = collect(() => notifyMigrateFailure({ id: 'migration-notice', status: 'partial', issue: { code: 'NEEDS_AUTH', message: '部分文件失败', action: 'reauthorize' } }))
    expect(notices[0].action).toEqual({ label: '查看任务', tab: 'transfer' })
  })

  it('真实未知结果保留核对要求，隐藏错误码和无效截止时间', () => {
    const notice = taskFailureNotice('UPLOAD_RESULT_UNKNOWN', '上传', { code: 'UPLOAD_RESULT_UNKNOWN', action: 'check_target', retryAt: Infinity })
    expect(notice.message).toContain('避免重复上传')
    expect(notice.message).not.toMatch(/UPLOAD_RESULT_UNKNOWN|Invalid Date|参考恢复时间/)
  })

  it('上传完成后的凭据警告不会要求恢复或重新上传', () => {
    const notice = taskFailureNotice('', '上传', { code: 'UPLOAD_CREDENTIAL_SAVE_FAILED', action: 'reauthorize', message: '文件已上传成功，但账号凭据未能保存' }, { userId: 'account' })
    expect(notice.message).not.toContain('手动恢复任务')
    expect(notice.action).toMatchObject({ repair: 'reauthorize', userId: 'account' })
    expect(taskFailureNotice('', '上传', { code: 'USER_CANCELED' }).silent).toBe(true)
  })
})
