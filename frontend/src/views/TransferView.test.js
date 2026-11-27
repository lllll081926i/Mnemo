import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const api = vi.hoisted(() => ({
  ListDownloads: vi.fn().mockResolvedValue([]),
  DownloadURL: vi.fn(),
  ListUploads: vi.fn().mockResolvedValue([]),
  ResumeUpload: vi.fn().mockResolvedValue(undefined),
  ResumeUploadReviewed: vi.fn().mockResolvedValue(undefined),
  ListMigrateJobs: vi.fn().mockResolvedValue([]),
  VerifyMigration: vi.fn().mockResolvedValue([]),
  ListOfflineTasks: vi.fn().mockResolvedValue([]),
  OfflineDownload: vi.fn(),
  refreshOfflineTasks: vi.fn().mockResolvedValue([]),
  EventsOn: vi.fn(() => () => {}),
  capsOf: vi.fn(() => ({})),
  verifyDownload: vi.fn(),
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
  api.OfflineDownload.mockReset()
  api.refreshOfflineTasks.mockReset().mockResolvedValue([])
  api.ListUploads.mockReset().mockResolvedValue([])
  vi.clearAllMocks()
})

describe('结构化任务原因', () => {
  it('批量添加下载时保留对话框，仅重试失败链接', async () => {
    let finish
    const first = 'https://example.test/one.zip'
    const second = 'https://example.test/two.zip'
    api.DownloadURL.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
      .mockRejectedValueOnce(new Error('添加失败')).mockResolvedValueOnce(undefined)
    wrapper = mount(TransferView, { attachTo: document.body, global: { stubs: { Teleport: true } } })
    await flushPromises()
    wrapper.vm.openDlModal()
    await wrapper.vm.$nextTick()
    const dialog = wrapper.get('[aria-label="新建下载"]')
    await dialog.get('textarea').setValue(`${first}\n${second}`)
    const submit = dialog.get('.btn.primary')
    submit.element.focus()
    await submit.trigger('click')
    expect(wrapper.vm.dlModal).toBe(true)
    expect(submit.attributes('aria-busy')).toBe('true')
    expect(document.activeElement).toBe(submit.element)
    await submit.trigger('click')
    expect(api.DownloadURL).toHaveBeenCalledTimes(1)
    finish()
    await flushPromises()
    expect(dialog.get('textarea').element.value).toBe(second)
    expect(submit.attributes('aria-busy')).toBe('false')
    await submit.trigger('click')
    await flushPromises()
    expect(api.DownloadURL.mock.calls.map(([, url]) => url)).toEqual([first, second, second])
    expect(wrapper.vm.dlModal).toBe(false)
  })

  it('后台添加下载失败时不自动弹窗，手动打开后保留失败链接', async () => {
    let fail
    const link = 'https://example.test/retry.zip'
    api.DownloadURL.mockImplementationOnce(() => new Promise((resolve, reject) => { fail = reject }))
    wrapper = mount(TransferView, { global: { stubs: { Teleport: true } } })
    await flushPromises()
    wrapper.vm.openDlModal()
    await wrapper.vm.$nextTick()
    const dialog = wrapper.get('[aria-label="新建下载"]')
    await dialog.get('textarea').setValue(link)
    await dialog.get('.btn.primary').trigger('click')
    await dialog.findAll('button').find(button => button.text() === '转到后台').trigger('click')
    fail(new Error('添加失败'))
    await flushPromises()
    expect(wrapper.vm.dlModal).toBe(false)
    expect(wrapper.emitted('toast').at(-1)[0]).toContain('可从「新建下载」重试失败链接')
    wrapper.vm.openDlModal()
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[aria-label="新建下载"] textarea').element.value).toBe(link)
  })

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
      expect(listener.mock.calls[0][0].detail).toMatchObject({ category: 'permission', message: expect.stringContaining('链接访问被拒绝，请核对有效期') })
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
    let notice
    window.addEventListener('mnemo:drive-notice', event => { notice = event.detail }, { once: true })
    await recommendation.get('button').trigger('click')
    expect(notice.action).toMatchObject({ repair: 'network_settings', userId: 'original-user' })
    expect(wrapper.emitted('repair')).toBeUndefined()
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
  it('后台轮询只替换有变化的上传任务，快照相同时复用列表和索引', async () => {
    const jobs = Array.from({ length: 40 }, (_, index) => ({
      UploadID: `snapshot-${index}`,
      Info: { name: `上传 ${index}`, size: 100 },
      Upload: { DownState: 'uploading', IsDowning: true, DownProcess: 10 },
    }))
    api.ListUploads.mockImplementation(async () => structuredClone(jobs))
    wrapper = mount(TransferView, { global: { stubs: { UiIcon: true } } })
    await flushPromises()
    await wrapper.findAll('[role="tab"]').find(tab => tab.text().includes('正在上传')).trigger('click')
    const originalList = wrapper.vm.activeUploads
    const originalIndex = wrapper.vm.uploadByID
    await wrapper.vm.refresh()
    expect(wrapper.vm.activeUploads).toBe(originalList)
    expect(wrapper.vm.uploadByID).toBe(originalIndex)
    jobs[0].Upload.DownProcess = 60
    await wrapper.vm.refresh()
    expect(wrapper.vm.activeUploads[0]).not.toBe(originalList[0])
    expect(wrapper.vm.activeUploads[1]).toBe(originalList[1])
    expect(wrapper.findAll('[role="progressbar"]')[0].attributes('aria-valuenow')).toBe('60')
  })

  it('账号和名称筛选隐藏活动下载时仍刷新任务', async () => {
    vi.useFakeTimers()
    const hidden = vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    const task = { id: 'active', user_id: 'original', name: '文件', status: 'downloading' }
    api.ListDownloads.mockResolvedValueOnce([task]).mockResolvedValueOnce([{ ...task, status: 'completed' }])
    try {
      wrapper = mount(TransferView, { global: { stubs: { UiIcon: true } } })
      await flushPromises()
      wrapper.vm.filterUser = 'another'
      wrapper.vm.taskFilterRaw = '不存在的名称'
      await vi.advanceTimersByTimeAsync(5000)
      expect(wrapper.vm.activeDownloads).toHaveLength(0)
      expect(api.ListDownloads).toHaveBeenCalledTimes(2)
      expect(wrapper.vm.downloadByID.get(task.id).status).toBe('completed')
    } finally {
      hidden.mockRestore()
    }
  })

  it('暂停与失败的任务不触发轮询，迁移页不连带读取下载和上传', async () => {
    vi.useFakeTimers()
    const hidden = vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    api.ListDownloads.mockResolvedValueOnce([{ id: 'paused', status: 'paused' }])
    api.ListUploads.mockResolvedValueOnce([{ UploadID: 'failed', Upload: { IsFailed: true, DownState: 'failed' } }])
    try {
      wrapper = mount(TransferView, { global: { stubs: { UiIcon: true } } })
      await flushPromises()
      await vi.advanceTimersByTimeAsync(5000)
      expect(api.ListDownloads).toHaveBeenCalledTimes(1)
      expect(api.ListUploads).toHaveBeenCalledTimes(1)
      let finishMigration
      api.ListMigrateJobs.mockImplementationOnce(() => new Promise(resolve => { finishMigration = resolve }))
      await wrapper.findAll('[role="tab"]').find(tab => tab.text().includes('迁移')).trigger('click')
      await vi.advanceTimersByTimeAsync(15000)
      expect(api.ListMigrateJobs).toHaveBeenCalledTimes(2)
      expect(wrapper.vm.migrateLoading).toBe(false)
      expect(api.ListDownloads).toHaveBeenCalledTimes(1)
      expect(api.ListUploads).toHaveBeenCalledTimes(1)
      finishMigration([])
      await flushPromises()
    } finally {
      hidden.mockRestore()
    }
  })

  it('隐藏再打开传输页后，旧轮询响应不能建立第二组定时器', async () => {
    vi.useFakeTimers()
    const hidden = vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    const timers = vi.spyOn(globalThis, 'setTimeout')
    let finish
    api.ListDownloads.mockResolvedValueOnce([{ id: 'active', status: 'downloading' }])
      .mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    try {
      wrapper = mount(TransferView, { global: { stubs: { UiIcon: true } } })
      await flushPromises()
      await vi.advanceTimersByTimeAsync(5000)
      expect(finish).toBeTypeOf('function')
      wrapper.vm.deactivateView()
      wrapper.vm.activateView()
      const count = () => timers.mock.calls.filter(([callback]) => callback.name === 'pollTransfers').length
      expect(count()).toBe(2)
      finish([])
      await flushPromises()
      await vi.advanceTimersByTimeAsync(0)
      expect(count()).toBe(2)
    } finally {
      timers.mockRestore()
      hidden.mockRestore()
    }
  })

  it('返回传输页立即刷新，旧请求结束不干扰新一轮刷新', async () => {
    vi.useFakeTimers()
    const previous = { id: 'previous', name: '旧任务', status: 'downloading' }
    const latest = { id: 'latest', name: '新任务', status: 'downloading', progress: 5 }
    let finishOld
    let finishNew
    api.ListDownloads.mockResolvedValueOnce([previous])
      .mockImplementationOnce(() => new Promise(resolve => { finishOld = resolve }))
      .mockImplementationOnce(() => new Promise(resolve => { finishNew = resolve }))
      .mockResolvedValue([latest])
    wrapper = mount(TransferView, { global: { stubs: { UiIcon: true } } })
    await flushPromises()
    const oldRefresh = wrapper.vm.refresh()
    wrapper.vm.deactivateView()
    wrapper.vm.activateView()
    expect(finishNew).toBeTypeOf('function')
    finishOld([{ ...previous, name: '过期响应' }])
    await oldRefresh
    expect(wrapper.vm.downloadByID.get(previous.id).name).toBe('旧任务')
    await wrapper.vm.refresh()
    expect(api.ListDownloads).toHaveBeenCalledTimes(3)
    const notify = api.EventsOn.mock.calls.filter(([name]) => name === 'transfer:event').at(-1)[1]
    notify({ kind: 'download', task: { ...latest, progress: 40 } })
    finishNew([latest])
    await flushPromises()
    expect(wrapper.vm.downloadByID.get(latest.id).progress).toBe(40)
    await vi.advanceTimersByTimeAsync(0)
    expect(api.ListDownloads).toHaveBeenCalledTimes(4)
  })

  it('连续进度更新复用任务索引，新增与移除后同步索引', async () => {
    vi.useFakeTimers({ toFake: ['requestAnimationFrame', 'cancelAnimationFrame'] })
    api.ListDownloads.mockResolvedValueOnce([{ id: 'indexed', status: 'downloading', name: '下载文件' }])
    api.ListUploads.mockResolvedValueOnce([{ UploadID: 'upload-indexed', Info: { name: '上传文件' }, Upload: {} }])
    wrapper = mount(TransferView, { global: { stubs: { UiIcon: true } } })
    await flushPromises()
    const downloadsIndex = wrapper.vm.downloadByID
    const uploadsIndex = wrapper.vm.uploadByID
    const notify = api.EventsOn.mock.calls.find(([name]) => name === 'transfer:event')[1]
    const formatCount = api.formatSpeed.mock.calls.length
    for (let progress = 1; progress <= 50; progress++) {
      notify({ kind: 'download', task: { id: 'indexed', status: 'downloading', progress } })
      notify({ kind: 'upload', task: { id: 'upload-indexed', status: 'uploading', progress, speed: progress } })
    }
    expect(downloadsIndex.get('indexed').progress).toBeUndefined()
    expect(uploadsIndex.get('upload-indexed').Upload.DownProcess).toBe(1)
    vi.advanceTimersToNextFrame()
    expect(wrapper.vm.downloadByID).toBe(downloadsIndex)
    expect(wrapper.vm.uploadByID).toBe(uploadsIndex)
    expect(downloadsIndex.get('indexed').progress).toBe(50)
    expect(uploadsIndex.get('upload-indexed').Upload.DownProcess).toBe(50)
    expect(uploadsIndex.get('upload-indexed').Info.name).toBe('上传文件')
    expect(api.formatSpeed.mock.calls.length - formatCount).toBe(2)
    notify({ kind: 'download', task: { id: 'new-task', status: 'queued' } })
    expect(wrapper.vm.downloadByID.has('new-task')).toBe(true)
    notify({ kind: 'download', task: { id: 'indexed', status: 'removed' } })
    expect(wrapper.vm.downloadByID.has('indexed')).toBe(false)
  })

  it('上传进度只更新目标行，不重新计算队列或其他行的图标', async () => {
    vi.useFakeTimers({ toFake: ['requestAnimationFrame', 'cancelAnimationFrame'] })
    api.ListUploads.mockResolvedValue(Array.from({ length: 40 }, (_, index) => ({
      UploadID: `upload-${index}`,
      Info: { name: `测试上传 ${index}`, size: 100 },
      Upload: { DownState: 'uploading', stage: 'uploading', IsDowning: true, IsCompleted: false, issue: null, DownProcess: 10 },
    })))
    wrapper = mount(TransferView, { global: { stubs: { UiIcon: true } } })
    await flushPromises()
    await wrapper.findAll('[role="tab"]').find(tab => tab.text().includes('正在上传')).trigger('click')
    const list = wrapper.vm.activeUploads
    const job = list[0].Upload
    const notify = api.EventsOn.mock.calls.find(([name]) => name === 'transfer:event')[1]
    api.iconOf.mockClear()
    api.formatSpeed.mockClear()
    for (let progress = 11; progress <= 60; progress++) {
      notify({ kind: 'upload', task: { id: 'upload-0', status: 'uploading', stage: 'uploading', progress, speed: 20 } })
    }
    vi.advanceTimersToNextFrame()
    await wrapper.vm.$nextTick()
    const bars = wrapper.findAll('[role="progressbar"]')
    expect(bars).toHaveLength(40)
    expect(bars[0].attributes('aria-valuenow')).toBe('60')
    expect(bars[1].attributes('aria-valuenow')).toBe('10')
    expect(wrapper.vm.activeUploads).toBe(list)
    expect(wrapper.vm.activeUploads[0].Upload).toBe(job)
    expect(api.iconOf).not.toHaveBeenCalled()
    expect(api.formatSpeed).toHaveBeenCalledOnce()
    const oldBadge = bars[0].element.previousElementSibling.firstElementChild
    notify({ kind: 'upload', task: { id: 'upload-0', status: 'uploading', stage: 'checking' } })
    vi.advanceTimersToNextFrame()
    await wrapper.vm.$nextTick()
    expect(bars[0].attributes('aria-valuenow')).toBeUndefined()
    expect(bars[0].attributes('aria-label')).toContain('检查秒传')
    expect(bars[0].element.previousElementSibling.firstElementChild).not.toBe(oldBadge)
  })

  it('下载进度只渲染目标行，校验百分比更新不会重播阶段动效', async () => {
    vi.useFakeTimers({ toFake: ['requestAnimationFrame', 'cancelAnimationFrame'] })
    api.ListDownloads.mockResolvedValueOnce(Array.from({ length: 40 }, (_, index) => ({
      id: `download-${index}`, name: `下载 ${index}`, size: 100, status: 'downloading', progress: 10,
    })))
    wrapper = mount(TransferView, { global: { stubs: { UiIcon: true } } })
    await flushPromises()
    const notify = api.EventsOn.mock.calls.find(([name]) => name === 'transfer:event')[1]
    const bars = wrapper.findAll('[role="progressbar"]')
    expect(bars).toHaveLength(40)
    api.formatBytes.mockClear()
    for (let progress = 11; progress <= 60; progress++) {
      notify({ kind: 'download', task: { id: 'download-0', status: 'downloading', progress, speed: 20 } })
    }
    vi.advanceTimersToNextFrame()
    await wrapper.vm.$nextTick()
    expect(api.formatBytes).toHaveBeenCalledOnce()
    expect(bars[0].attributes('aria-valuenow')).toBe('60')
    expect(bars[1].attributes('aria-valuenow')).toBe('10')
    const selection = wrapper.get('[aria-label="选择 下载 0"]')
    await selection.trigger('click')
    expect(selection.attributes('aria-pressed')).toBe('true')
    const badge = () => bars[0].element.previousElementSibling.firstElementChild
    const oldBadge = badge()
    let verifyBadge
    for (const stageProgress of [72, 89]) {
      notify({ kind: 'download', task: { id: 'download-0', status: 'downloading', stage: 'verifying', stageProgress } })
      vi.advanceTimersToNextFrame()
      await wrapper.vm.$nextTick()
      expect(badge().textContent).toBe(`校验内容 ${stageProgress}%`)
      if (stageProgress === 72) expect(badge()).not.toBe(oldBadge)
      else expect(badge()).toBe(verifyBadge)
      verifyBadge = badge()
    }
    expect(selection.attributes('aria-pressed')).toBe('true')
  })

  it('后台刷新保留帧内最新进度，完成后不回退，离开页面取消待更新帧', async () => {
    vi.useFakeTimers({ toFake: ['requestAnimationFrame', 'cancelAnimationFrame'] })
    const task = { id: 'pending', name: '文件', status: 'downloading', progress: 5 }
    let finishRefresh
    api.ListDownloads.mockResolvedValueOnce([task]).mockImplementationOnce(() => new Promise(resolve => { finishRefresh = resolve }))
    wrapper = mount(TransferView, { global: { stubs: { UiIcon: true } } })
    await flushPromises()
    const notify = api.EventsOn.mock.calls.find(([name]) => name === 'transfer:event')[1]
    const refreshed = wrapper.vm.refresh()
    notify({ kind: 'download', task: { id: task.id, status: 'downloading', progress: 40 } })
    finishRefresh([{ ...task, progress: 15 }])
    await refreshed
    expect(wrapper.vm.downloadByID.get(task.id).progress).toBe(40)
    notify({ kind: 'download', task: { id: task.id, status: 'downloading', progress: 99 } })
    notify({ kind: 'download', task: { id: task.id, groupId: 'folder', status: 'completed', progress: 100 } })
    expect(wrapper.vm.downloadByID.get(task.id).status).toBe('completed')
    vi.advanceTimersToNextFrame()
    expect(wrapper.vm.downloadByID.get(task.id).progress).toBe(100)
    notify({ kind: 'download', task: { id: task.id, status: 'downloading', progress: 5 } })
    notify({ kind: 'download', task: { id: task.id, status: 'downloading', progress: 60 } })
    wrapper.vm.deactivateView()
    vi.advanceTimersToNextFrame()
    expect(wrapper.vm.downloadByID.get(task.id).progress).toBe(5)
  })

  it('迁移合并帧内进度，保留完成状态并丢弃离开页面后的响应', async () => {
    vi.useFakeTimers({ toFake: ['requestAnimationFrame', 'cancelAnimationFrame'] })
    const jobs = Array.from({ length: 30 }, (_, index) => ({ id: `migration-${index}`, srcUser: 'source', dstUser: 'target', status: 'running', stage: 'uploading', total: 100, processed: 5 }))
    api.ListMigrateJobs.mockResolvedValueOnce(jobs)
    wrapper = mount(TransferView, { global: { stubs: { UiIcon: true } } })
    await flushPromises()
    await wrapper.findAll('[role="tab"]').find(tab => tab.text().includes('迁移')).trigger('click')
    const notify = api.EventsOn.mock.calls.find(([name]) => name === 'migrate:progress')[1]
    const current = wrapper.vm.migrateByID.get('migration-0')
    const first = wrapper.findAll('[role="progressbar"]')[0]
    api.providerMetaOf.mockClear()
    for (let processed = 6; processed <= 50; processed++) notify({ ...jobs[0], processed })
    expect(current.processed).toBe(5)
    vi.advanceTimersToNextFrame()
    await wrapper.vm.$nextTick()
    expect(first.attributes('aria-valuenow')).toBe('50')
    expect(wrapper.vm.migrateByID.get('migration-0')).toBe(current)
    expect(api.providerMetaOf).not.toHaveBeenCalled()
    let resolveSnapshot
    api.ListMigrateJobs.mockImplementationOnce(() => new Promise(resolve => { resolveSnapshot = resolve }))
    const pending = wrapper.vm.refreshMigrateJobs()
    notify({ ...jobs[0], processed: 75 })
    const { stage, ...completed } = jobs[0]
    notify({ ...completed, status: 'completed', processed: 100 })
    expect(current.status).toBe('completed')
    expect(current.stage).toBeUndefined()
    resolveSnapshot([{ ...jobs[0], status: 'running', stage, processed: 30 }])
    await pending
    expect(wrapper.vm.migrateByID.get('migration-0').processed).toBe(100)
    api.ListMigrateJobs.mockImplementationOnce(() => new Promise(resolve => { resolveSnapshot = resolve }))
    const hidden = wrapper.vm.refreshMigrateJobs()
    wrapper.vm.deactivateView()
    resolveSnapshot([])
    await hidden
    expect(wrapper.vm.migrateByID.get('migration-0').status).toBe('completed')
  })

  it('区分传输字节核对与远端目标校验', async () => {
    const stages = ['transferred-size', 'remote-size', 'remote-hash']
    api.ListMigrateJobs.mockResolvedValueOnce([{ id: 'migration', status: 'completed', items: Object.fromEntries(stages.map(id => [id, { id, name: id, verification: id, status: 'completed' }])) }])
    wrapper = mount(TransferView, { global: { stubs: { UiIcon: true, teleport: true } } })
    await flushPromises()
    wrapper.vm.detailID = 'migration'
    await wrapper.vm.$nextTick()
    expect(wrapper.text()).toContain('传输大小已核对')
    expect(wrapper.text()).toContain('目标大小已核对')
    expect(wrapper.text()).toContain('目标哈希已核对')
  })

  it('迁移详情限制分页节点数，关闭后不应用晚到的校验结果', async () => {
    let finishVerification
    api.VerifyMigration.mockImplementationOnce(() => new Promise(resolve => { finishVerification = resolve }))
    const items = Object.fromEntries(Array.from({ length: 110 }, (_, index) => [`file-${index}`, { id: `file-${index}`, name: `资料-${index}.txt`, status: 'completed' }]))
    api.ListMigrateJobs.mockResolvedValueOnce([{ id: 'migration', status: 'completed', items }])
    wrapper = mount(TransferView, { global: { stubs: { UiIcon: true, teleport: true } } })
    await flushPromises()
    wrapper.vm.detailID = 'migration'
    await wrapper.vm.$nextTick()
    expect(wrapper.findAll('[role="listitem"]')).toHaveLength(50)
    const findButton = text => wrapper.findAll('button').find(button => button.text() === text)
    await findButton('下一页').trigger('click')
    expect(wrapper.findAll('[role="listitem"]')[0].text()).toContain('资料-50.txt')
    await wrapper.get('[aria-label="按文件名筛选"]').setValue('资料-109.txt')
    expect(wrapper.findAll('[role="listitem"]')).toHaveLength(1)
    const verify = findButton('校验文件完整性')
    await verify.trigger('click')
    await verify.trigger('click')
    expect(api.VerifyMigration).toHaveBeenCalledOnce()
    wrapper.vm.detailID = ''
    await wrapper.vm.$nextTick()
    finishVerification([{ id: 'file-109', status: 'mismatch', detail: '哈希不一致' }])
    wrapper.vm.detailID = 'migration'
    await flushPromises()
    expect(wrapper.text()).not.toContain('哈希不一致')
    expect(wrapper.findAll('[role="listitem"]')).toHaveLength(50)
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

  it('云离线部分失败留在原窗口，只重试失败链接', async () => {
    await openGuangyaOffline()
    let finish
    const first = 'https://example.test/one.zip'
    const second = 'https://example.test/two.zip'
    api.OfflineDownload.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
      .mockRejectedValueOnce(new Error('提交失败')).mockResolvedValueOnce(undefined)
    await wrapper.findAll('button').find(button => button.text().includes('新建离线任务')).trigger('click')
    wrapper.vm.offLinks = `${first}\n${second}`
    const pending = wrapper.vm.submitOffline()
    await wrapper.vm.$nextTick()
    expect(wrapper.vm.offModal).toBe(true)
    await wrapper.vm.submitOffline()
    expect(api.OfflineDownload).toHaveBeenCalledTimes(1)
    finish()
    await pending
    expect(wrapper.vm.offModal).toBe(true)
    expect(wrapper.vm.offLinks).toBe(second)
    await wrapper.vm.submitOffline()
    expect(api.OfflineDownload.mock.calls.map(([, , url]) => url)).toEqual([first, second, second])
    expect(wrapper.vm.offModal).toBe(false)
  })

  it('云离线后台失败不弹窗，只在原账号恢复失败链接', async () => {
    const first = await openGuangyaOffline()
    const second = { user_id: 'guangya_other', drive_id: 'other-drive' }
    const link = 'https://example.test/retry.zip'
    let fail
    api.OfflineDownload.mockImplementationOnce(() => new Promise((resolve, reject) => { fail = reject }))
    const open = () => wrapper.findAll('button').find(button => button.text().includes('新建离线任务')).trigger('click')
    await open()
    wrapper.vm.offLinks = link
    const pending = wrapper.vm.submitOffline()
    await wrapper.setProps({ accounts: [second] })
    fail(new Error('提交失败'))
    await pending
    expect(wrapper.vm.offModal).toBe(false)
    await open()
    expect(wrapper.vm.offLinks).toBe('')
    await wrapper.setProps({ accounts: [first] })
    expect(wrapper.vm.offModal).toBe(false)
    await open()
    expect(wrapper.vm.offLinks).toBe(link)
  })

  it('光鸭云离线刷新实际查询远端进度，部分完成不显示全部完成', async () => {
    api.ListOfflineTasks.mockResolvedValue([{ task_id: 'task', drive_id: 'remote-drive', status: 'running', progress: 0 }])
    api.refreshOfflineTasks.mockResolvedValue([{ task_id: 'task', file_name: '远端资源', status: 'partial', progress: 70, message: '部分文件未完成', file_size: 1024 }])
    const account = await openGuangyaOffline()
    expect(api.refreshOfflineTasks).toHaveBeenCalledWith(account.user_id, account.drive_id)
    expect(wrapper.text()).toContain('远端资源')
    expect(wrapper.text()).toContain('部分完成')
    expect(wrapper.text()).toContain('70%')
    expect(wrapper.text()).toContain('查看通知')
    expect(wrapper.find('.text-error').exists()).toBe(false)
    expect(wrapper.find('.progress-current').element.style.getPropertyValue('--progress')).toBe('0.7')
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

  it('重复刷新只发一个远端请求，等待期间保持最新进度', async () => {
    const task = { task_id: 'task', file_name: '离线资源', drive_id: 'remote-drive', status: 'running' }
    api.ListOfflineTasks.mockResolvedValue([{ ...task, progress: 10 }])
    api.refreshOfflineTasks.mockResolvedValueOnce([{ ...task, progress: 60 }])
    await openGuangyaOffline()
    let finish
    api.refreshOfflineTasks.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const pending = wrapper.vm.refreshOffline()
    expect(wrapper.vm.refreshOffline()).toBe(pending)
    await flushPromises()
    expect(api.ListOfflineTasks).toHaveBeenCalledOnce()
    expect(api.refreshOfflineTasks).toHaveBeenCalledTimes(2)
    expect(wrapper.get('[role="progressbar"]').attributes('aria-valuenow')).toBe('60')
    const busy = wrapper.get('.offline-refresh')
    expect(busy.attributes('aria-busy')).toBe('true')
    expect(busy.text()).toBe('刷新')
    finish([{ ...task, progress: 80 }])
    await pending
    expect(wrapper.get('[role="progressbar"]').attributes('aria-valuenow')).toBe('80')
    expect(busy.attributes('aria-busy')).toBe('false')
  })

  it('同账号切换网盘立即清理旧列表，忽略旧盘响应', async () => {
    api.refreshOfflineTasks.mockResolvedValueOnce([{ task_id: 'old', file_name: '旧盘资源', progress: 45 }])
    const account = await openGuangyaOffline()
    let finishOld, finishNew
    api.refreshOfflineTasks.mockImplementationOnce(() => new Promise(resolve => { finishOld = resolve }))
      .mockImplementationOnce(() => new Promise(resolve => { finishNew = resolve }))
    const old = wrapper.vm.refreshOffline()
    await flushPromises()
    await wrapper.setProps({ accounts: [{ ...account, drive_id: 'next-drive' }] })
    await flushPromises()
    expect(wrapper.text()).not.toContain('旧盘资源')
    finishOld([{ task_id: 'old', file_name: '旧盘资源', progress: 90 }])
    await old
    expect(wrapper.text()).not.toContain('旧盘资源')
    expect(wrapper.get('.offline-refresh').attributes('aria-busy')).toBe('true')
    finishNew([{ task_id: 'new', file_name: '新盘资源', progress: 20 }])
    await flushPromises()
    expect(wrapper.text()).toContain('新盘资源')
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
    [2971135900, '100%', '1'],
    [-5, '0%', '0'],
    [Number.NaN, '0%', '0'],
    [Number.POSITIVE_INFINITY, '0%', '0'],
  ])('下载记录百分比 %s 不会越界显示', async (progress, text, scale) => {
    api.ListDownloads.mockResolvedValueOnce([
      { id: 'download', name: '测试文件.esd', status: 'paused', size: 1, progress },
    ])
    wrapper = mount(TransferView, { props: { accounts: [], providers: [] }, global: { stubs: { UiIcon: true } } })
    await flushPromises()
    expect(wrapper.find('.text-state').text()).toContain(text)
    expect(wrapper.find('.progress-current').element.style.getPropertyValue('--progress')).toBe(scale)
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
    const details = wrapper.get('.folder-task-details')
    expect(details.findAll('.folder-task-child')).toHaveLength(0)
    details.element.open = true
    await details.trigger('toggle')
    expect(details.get('.folder-task-child').text()).toContain('file.txt')
    details.element.open = false
    await details.trigger('toggle')
    expect(details.findAll('.folder-task-child')).toHaveLength(0)
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
