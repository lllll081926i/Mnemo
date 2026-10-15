import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const api = vi.hoisted(() => ({
  ListDownloads: vi.fn().mockResolvedValue([]),
  ListUploads: vi.fn().mockResolvedValue([]),
  ResumeUpload: vi.fn().mockResolvedValue(undefined),
  ResumeUploadReviewed: vi.fn().mockResolvedValue(undefined),
  ListMigrateJobs: vi.fn().mockResolvedValue([]),
  ListOfflineTasks: vi.fn().mockResolvedValue([]),
  refreshOfflineTasks: vi.fn().mockResolvedValue([]),
  EventsOn: vi.fn(() => () => {}),
  capsOf: vi.fn(() => ({})),
  accountName: vi.fn(() => ''),
  providerIconUrl: vi.fn(() => ''),
  providerMetaOf: vi.fn(() => ({})),
  formatBytes: vi.fn(value => `${value} B`),
  formatSpeed: vi.fn(value => `${value} B/s`),
  formatTime: vi.fn(() => ''),
  iconOf: vi.fn(() => 'file'),
}))

vi.mock('../api', () => api)

import TransferView from './TransferView.vue'
import TaskIssue from '../components/TaskIssue.vue'
import { DOWNLOAD_SOUNDS } from '../download-sounds'
import { setPref } from '../appearance'

let wrapper
afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  vi.useRealTimers()
  api.capsOf.mockReset().mockReturnValue({})
  api.ListOfflineTasks.mockReset().mockResolvedValue([])
  api.refreshOfflineTasks.mockReset().mockResolvedValue([])
  api.ListUploads.mockReset().mockResolvedValue([])
  vi.clearAllMocks()
})

describe('结构化任务原因', () => {

  it('未知上传回执先复核，只有再次确认才允许重试，并保留目标作用域', async () => {
    const task = { UploadID: 'unknown-upload', user_id: 'original-account', Info: { name: '文件.txt', drive_id: 'original-drive', parent_file_id: 'original-parent' }, Upload: { IsFailed: true, attempt: { state: 'unknown', parentId: 'resolved-parent' }, issue: { code: 'UPLOAD_RESULT_UNKNOWN', action: 'check_target' } } }
    api.ListUploads.mockResolvedValue([task])
    wrapper = mount(TransferView, { global: { stubs: { UiIcon: true } } })
    await flushPromises()
    await wrapper.findAll('[role="tab"]').find(tab => tab.text().includes('正在上传')).trigger('click')
    await wrapper.get('[title="复核上传结果，不重新提交"]').trigger('click')
    expect(api.ResumeUpload).toHaveBeenCalledWith(task.UploadID)
    expect(api.ResumeUploadReviewed).not.toHaveBeenCalled()
    await wrapper.get('[title="打开上传目标目录"]').trigger('click')
    expect(wrapper.emitted('navigate')[0][0]).toEqual({ userId: 'original-account', driveId: 'original-drive', dirId: 'resolved-parent', name: '上传目标' })
    await wrapper.findAll('button').find(button => button.text() === '核对后重试').trigger('click')
    expect(api.ResumeUploadReviewed).not.toHaveBeenCalled()
    const dialog = document.querySelector('[role="dialog"]')
    expect(dialog.textContent).toContain('可能产生重复文件')
    ;[...dialog.querySelectorAll('button')].find(button => button.textContent === '取消').click()
    await flushPromises()
    expect(api.ResumeUploadReviewed).not.toHaveBeenCalled()
    await wrapper.findAll('button').find(button => button.text() === '核对后重试').trigger('click')
    ;[...document.querySelector('[role="dialog"]').querySelectorAll('button')].find(button => button.textContent === '确认仍需上传').click()
    await flushPromises()
    expect(api.ResumeUploadReviewed).toHaveBeenCalledExactlyOnceWith(task.UploadID)
  })
  it('结构化访问拒绝不会被原始 401 字符串误报为账号失效', async () => {
    api.ListDownloads.mockResolvedValueOnce([])
    api.ListUploads.mockResolvedValueOnce([])
    wrapper = mount(TransferView, { global: { stubs: { UiIcon: true } } })
    await flushPromises()
    const listener = vi.fn()
    window.addEventListener('mnemo:drive-notice', listener)
    try {
      const notify = api.EventsOn.mock.calls.find(([name]) => name === 'transfer:event')[1]
      notify({ kind: 'download', task: { id: 'rejected', status: 'failed', error: 'HTTP 401', issue: { code: 'REMOTE_ACCESS_REJECTED', message: '链接访问被拒绝，请核对有效期', action: 'check_access' } } })
      expect(listener.mock.calls[0][0].detail).toMatchObject({ category: 'permission', message: '链接访问被拒绝，请核对有效期' })
    } finally {
      window.removeEventListener('mnemo:drive-notice', listener)
    }
  })
  it('原因随下载事件清除，网络入口保留原账号身份', async () => {
    const issue = { code: 'NETWORK_UNAVAILABLE', message: '网络请求中断', action: 'network_settings' }
    api.ListDownloads.mockResolvedValueOnce([{ id: 'issue-download', user_id: 'original-user', name: '文件', status: 'failed', error: 'raw', issue }])
    api.ListUploads.mockResolvedValueOnce([])
    wrapper = mount(TransferView, { global: { stubs: { UiIcon: true } } })
    await flushPromises()
    const recommendation = wrapper.getComponent(TaskIssue)
    await recommendation.get('button').trigger('click')
    expect(wrapper.emitted('repair')).toEqual([[{ action: 'network_settings', userId: 'original-user' }]])
    const notify = api.EventsOn.mock.calls.find(([name]) => name === 'transfer:event')[1]
    notify({ kind: 'download', task: { id: 'issue-download', status: 'downloading' } })
    await flushPromises()
    expect(wrapper.vm.downloads[0].issue).toBeNull()
  })

  it('恢复上传事件清除旧失败原因与诊断，不把缺省字段当成保留旧错误', async () => {
    api.ListDownloads.mockResolvedValueOnce([])
    api.ListUploads.mockResolvedValueOnce([{ UploadID: 'issue-upload', Info: {}, Upload: { IsFailed: true, failedMessage: 'old-error', issue: { code: 'NEEDS_AUTH', action: 'reauthorize' } } }])
    wrapper = mount(TransferView, { global: { stubs: { UiIcon: true } } })
    await flushPromises()
    const notify = api.EventsOn.mock.calls.find(([name]) => name === 'transfer:event')[1]
    notify({ kind: 'upload', task: { id: 'issue-upload', status: 'uploading' } })
    await flushPromises()
    expect(wrapper.vm.uploads[0].Upload.issue).toBeNull()
    expect(wrapper.vm.uploads[0].Upload.failedMessage).toBe('')
  })
})

describe('传输页刷新调度', () => {
  it('区分传输字节核对与远端目标校验', async () => {
    wrapper = mount(TransferView, { props: { accounts: [], providers: [] }, global: { stubs: { UiIcon: true } } })
    await flushPromises()
    expect(wrapper.vm.verificationLabels['transferred-size']).toBe('传输大小已核对')
    expect(wrapper.vm.verificationLabels['remote-size']).toBe('目标大小已核对')
    expect(wrapper.vm.verificationLabels['remote-hash']).toBe('目标哈希已核对')
  })

  async function openGuangyaOffline() {
    const account = { user_id: 'guangya_remote', drive_id: 'remote-drive' }
    api.capsOf.mockReturnValue({ offlineDownload: true })
    wrapper = mount(TransferView, { props: { accounts: [account], providers: [] }, global: { stubs: { UiIcon: true } } })
    await flushPromises()
    await wrapper.findAll('[role="tab"]').find(tab => tab.text().includes('云离线')).trigger('click')
    await flushPromises()
    return account
  }

  it('光鸭云离线刷新实际查询远端进度，部分完成不显示全部完成', async () => {
    api.ListOfflineTasks.mockResolvedValue([{ task_id: 'task', drive_id: 'remote-drive', status: 'running', progress: 0 }])
    api.refreshOfflineTasks.mockResolvedValue([{ task_id: 'task', file_name: '远端资源', status: 'partial', progress: 70, message: '部分文件未完成', file_size: 1024 }])
    const account = await openGuangyaOffline()
    expect(api.refreshOfflineTasks).toHaveBeenCalledWith(account.user_id, account.drive_id)
    expect(wrapper.text()).toContain('远端资源')
    expect(wrapper.text()).toContain('部分完成')
    expect(wrapper.text()).toContain('70%')
    expect(wrapper.text()).toContain('部分文件未完成')
    expect(wrapper.find('.progress-current').element.style.width).toBe('70%')
  })

  it('远端查询失败保留同一网盘缓存并显示错误', async () => {
    api.ListOfflineTasks.mockResolvedValue([
      { task_id: 'cached', file_name: '缓存资源', drive_id: 'remote-drive', progress: 20 },
      { task_id: 'other', file_name: '其他网盘资源', drive_id: 'other-drive' },
    ])
    api.refreshOfflineTasks.mockRejectedValue(new Error('查询限流'))
    await openGuangyaOffline()
    expect(wrapper.text()).toContain('缓存资源')
    expect(wrapper.text()).not.toContain('其他网盘资源')
    expect(wrapper.text()).toContain('查询限流')
  })

  it('部分完成即使上游进度为 100 也不能显示成功样式', async () => {
    api.refreshOfflineTasks.mockResolvedValue([{ task_id: 'partial', file_name: '部分资源', status: 'partial', progress: 100 }])
    await openGuangyaOffline()
    expect(wrapper.text()).toContain('部分完成')
    expect(wrapper.find('.progress-current').classes()).toContain('warning')
    expect(wrapper.find('.progress-current').classes()).not.toContain('succeed')
  })

  it('切换账号后旧云离线响应不能覆盖新账号', async () => {
    let finishOld
    api.refreshOfflineTasks.mockImplementation(userID => userID === 'guangya_remote'
      ? new Promise(resolve => { finishOld = resolve })
      : Promise.resolve([{ task_id: 'new', file_name: '新账号资源', status: 'completed', progress: 100 }]))
    await openGuangyaOffline()
    await wrapper.setProps({ accounts: [{ user_id: 'guangya_new', drive_id: 'new-drive' }] })
    await flushPromises()
    finishOld([{ task_id: 'old', file_name: '旧账号资源', progress: 40 }])
    await flushPromises()
    expect(wrapper.text()).toContain('新账号资源')
    expect(wrapper.text()).not.toContain('旧账号资源')
  })

  it.each(['pikpak', 'thunder', 'guangya'])('%s 已开启的云离线会显示入口并允许选择账号', async (provider) => {
    const actualApi = await vi.importActual('../api')
    api.capsOf.mockImplementation(actualApi.capsOf)
    const supported = { user_id: `${provider}_enabled`, drive_id: 'enabled' }
    const unsupported = { user_id: 'webdav_disabled', drive_id: 'disabled' }
    wrapper = mount(TransferView, {
      props: {
        accounts: [supported, unsupported],
        providers: [
          { ID: provider, Capabilities: { offlineDownload: true } },
          { ID: 'webdav', Capabilities: { offlineDownload: false } },
        ],
      },
      global: { stubs: { UiIcon: true } },
    })
    await flushPromises()
    expect(wrapper.text()).toContain('云离线')
    expect(wrapper.vm.offlineAccounts.map(account => account.user_id)).toEqual([supported.user_id])
  })

  it.each([
    [2971135900, '100%', '100%'],
    [-5, '0%', '0%'],
    [Number.NaN, '0%', '0%'],
    [Number.POSITIVE_INFINITY, '0%', '0%'],
  ])('下载记录百分比 %s 不会越界显示', async (progress, text, width) => {
    api.ListDownloads.mockResolvedValueOnce([
      { id: 'download', name: '测试文件.esd', status: 'paused', size: 1, progress },
    ])
    wrapper = mount(TransferView, { props: { accounts: [], providers: [] }, global: { stubs: { UiIcon: true } } })
    await flushPromises()
    expect(wrapper.find('.text-state').text()).toContain(text)
    expect(wrapper.find('.progress-current').element.style.width).toBe(width)
  })

  it('高频事件不会把已经安排的首次刷新不断往后推', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'Date'] })
    wrapper = mount(TransferView, { props: { accounts: [], providers: [] }, global: { stubs: { UiIcon: true } } })
    await flushPromises()
    api.ListDownloads.mockClear()

    wrapper.vm.scheduleRefresh(150)
    await vi.advanceTimersByTimeAsync(100)
    wrapper.vm.scheduleRefresh(150)
    await vi.advanceTimersByTimeAsync(50)
    await flushPromises()

    expect(api.ListDownloads).toHaveBeenCalledOnce()
  })

  it('下载详情保留具体 HTTP 错误而不是只显示通用提示', async () => {
    const task = { id: 'failed', name: '文件.sup', status: 'failed', error: 'dlengine: chunk 2 rejected: range http 503' }
    api.ListDownloads.mockResolvedValueOnce([task])
    wrapper = mount(TransferView, { props: { accounts: [], providers: [] }, global: { stubs: { UiIcon: true, teleport: true } } })
    await flushPromises()
    await wrapper.find('.taskrow').trigger('contextmenu', { clientX: 10, clientY: 10 })
    wrapper.vm.onCtxSelect('detail')
    await flushPromises()
    expect(wrapper.text()).toContain('错误详情')
    expect(wrapper.text()).toContain('range http 503')
  })

  it('文件夹只占用一条下载行，子任务在行内展开', async () => {
    api.ListDownloads.mockResolvedValueOnce([
      { id: 'folder', isDir: true, name: '文件夹', status: 'downloading', stage: 'scanning', totalFiles: 1, completedFiles: 0, localPath: 'folder' },
      { id: 'file', groupId: 'folder', name: 'file.txt', status: 'queued', localPath: 'folder/file.txt' },
    ])
    wrapper = mount(TransferView, { props: { accounts: [], providers: [] }, global: { stubs: { UiIcon: true } } })
    await flushPromises()
    expect(wrapper.findAll('.taskrow')).toHaveLength(1)
    expect(wrapper.text()).toContain('扫描目录')
    expect(wrapper.text()).toContain('查看文件（1/1）')
  })
})

describe('下载完成提示音选择', () => {
  let rawPrefs
  beforeEach(() => {
    rawPrefs = '{}'
    vi.stubGlobal('localStorage', {
      getItem: vi.fn(() => rawPrefs),
      setItem: vi.fn((key, value) => { rawPrefs = value }),
    })
    vi.stubGlobal('Audio', vi.fn(function (url) {
      this.src = url
      this.currentTime = 0
      this.play = vi.fn().mockResolvedValue(undefined)
      this.pause = vi.fn()
    }))
  })
  afterEach(() => vi.unstubAllGlobals())

  async function prepare(tasks) {
    api.ListDownloads.mockResolvedValueOnce(tasks)
    wrapper = mount(TransferView, { props: { accounts: [], providers: [] }, global: { stubs: { UiIcon: true } } })
    await flushPromises()
    return api.EventsOn.mock.calls.find(([name]) => name === 'transfer:event')[1]
  }

  it('完成时读取最新偏好，不需要重新打开传输页', async () => {
    const notify = await prepare([
      { id: 'first', name: 'first.txt', status: 'downloading' },
      { id: 'second', name: 'second.txt', status: 'downloading' },
    ])
    notify({ kind: 'download', task: { id: 'first', status: 'completed' } })
    await flushPromises()
    expect(Audio).toHaveBeenLastCalledWith(DOWNLOAD_SOUNDS[0].url)
    const first = Audio.mock.instances[0]
    setPref('downloadSoundType', 'sound-2')
    notify({ kind: 'download', task: { id: 'second', status: 'completed' } })
    await flushPromises()
    expect(Audio).toHaveBeenLastCalledWith(DOWNLOAD_SOUNDS[1].url)
    expect(first.pause).toHaveBeenCalledOnce()
    const second = Audio.mock.instances[1]
    notify({ kind: 'download', task: { id: 'second', status: 'completed' } })
    await flushPromises()
    expect(second.play).toHaveBeenCalledOnce()
  })

  it('关闭总开关后下载完成不播放音效', async () => {
    setPref('downloadSound', false)
    const notify = await prepare([{ id: 'muted', name: 'muted.txt', status: 'downloading' }])
    notify({ kind: 'download', task: { id: 'muted', status: 'completed' } })
    await flushPromises()
    expect(Audio).not.toHaveBeenCalled()
  })

  it('文件夹子任务不单独响铃，整体完成只响一次', async () => {
    setPref('downloadSoundType', 'sound-2')
    const notify = await prepare([
      { id: 'folder', isDir: true, name: 'folder', status: 'downloading' },
      { id: 'child', groupId: 'folder', name: 'child.txt', status: 'downloading' },
    ])
    notify({ kind: 'download', task: { id: 'child', groupId: 'folder', status: 'completed' } })
    await flushPromises()
    expect(Audio).not.toHaveBeenCalled()
    notify({ kind: 'download', task: { id: 'folder', isDir: true, status: 'completed' } })
    await flushPromises()
    expect(Audio).toHaveBeenCalledExactlyOnceWith(DOWNLOAD_SOUNDS[1].url)
  })
})
