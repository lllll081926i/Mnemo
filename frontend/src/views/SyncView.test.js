import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { KeepAlive, h, nextTick, ref } from 'vue'

const api = vi.hoisted(() => ({
  ListSyncConfigs: vi.fn(),
  SaveSyncConfig: vi.fn(),
  DeleteSyncConfig: vi.fn(),
  PreviewSync: vi.fn(),
  RunSync: vi.fn(),
  RunSyncPlan: vi.fn(),
  CancelSync: vi.fn(),
  ListRunningSyncIDs: vi.fn(),
  ListSyncRunStates: vi.fn().mockResolvedValue({}),
  onEvent: vi.fn(() => () => {}),
  accountName: vi.fn(() => '账号'),
  providerMetaOf: vi.fn(() => ({ label: '网盘' })),
  providerIconUrl: vi.fn(() => ''),
  PickDirectory: vi.fn(),
}))

vi.mock('../api', () => api)

import SyncView from './SyncView.vue'

let wrapper
afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  vi.clearAllMocks()
  vi.restoreAllMocks()
})

describe('同步任务操作状态', () => {
  it('本地目录选择未完成时拦截重复操作，取消后允许重试', async () => {
    api.ListSyncConfigs.mockResolvedValue([])
    api.ListRunningSyncIDs.mockResolvedValue([])
    let finishPicker
    api.PickDirectory.mockImplementationOnce(() => new Promise(resolve => { finishPicker = resolve }))
      .mockResolvedValueOnce('D:/Sync')
    wrapper = mount(SyncView, { attachTo: document.body, global: { stubs: { Teleport: true } } })
    await flushPromises()
    wrapper.vm.openCreate()
    await nextTick()
    const button = wrapper.findAll('button').find(control => control.text() === '选择目录')
    button.element.focus()
    await button.trigger('click')
    expect(button.attributes('aria-busy')).toBe('true')
    expect(button.text()).toBe('选择中…')
    await button.trigger('click')
    expect(api.PickDirectory).toHaveBeenCalledTimes(1)
    expect(document.activeElement).toBe(button.element)
    finishPicker('')
    await flushPromises()
    expect(wrapper.vm.form.local_dir).toBe('')
    expect(button.attributes('aria-busy')).toBe('false')
    await button.trigger('click')
    await flushPromises()
    expect(wrapper.vm.form.local_dir).toBe('D:/Sync')
  })

  it('任务配置和运行记录并行读取，慢记录不阻塞空状态或覆盖实时状态', async () => {
    const callbacks = new Map()
    api.onEvent.mockImplementation((name, callback) => { callbacks.set(name, callback); return () => callbacks.delete(name) })
    let finishConfigs
    let finishStates
    api.ListSyncConfigs.mockImplementationOnce(() => new Promise(resolve => { finishConfigs = resolve }))
    api.ListSyncRunStates.mockImplementationOnce(() => new Promise(resolve => { finishStates = resolve }))
    api.ListRunningSyncIDs.mockResolvedValue([])
    wrapper = mount(SyncView)
    expect(finishConfigs).toBeTypeOf('function')
    expect(finishStates).toBeTypeOf('function')
    await nextTick()
    expect(wrapper.text()).toContain('正在加载同步任务')
    callbacks.get('sync:state')({ id: 'task', running: false, status: 'completed' })
    finishConfigs([])
    await flushPromises()
    expect(wrapper.text()).toContain('还没有同步任务')
    expect(wrapper.text()).not.toContain('正在加载同步任务')
    finishStates({ task: { status: 'running' }, other: { status: 'failed' } })
    await flushPromises()
    expect(wrapper.vm.runStates.task.status).toBe('completed')
    expect(wrapper.vm.runStates.other.status).toBe('failed')

    let finishRefresh
    api.ListSyncConfigs.mockImplementationOnce(() => new Promise(resolve => { finishRefresh = resolve }))
    callbacks.get('account:changed')()
    await nextTick()
    expect(wrapper.text()).toContain('还没有同步任务')
    finishRefresh([])
    await flushPromises()
  })

  it('缓存页面退场后停订阅，返回时合并实时状态与后端快照', async () => {
    const visible = ref(true)
    const callbacks = new Map()
    api.onEvent.mockImplementation((name, callback) => {
      callbacks.set(name, callback)
      return () => callbacks.delete(name)
    })
    api.ListSyncConfigs.mockResolvedValue([])
    let finishInitial
    let finishReentry
    api.ListRunningSyncIDs
      .mockImplementationOnce(() => new Promise(resolve => { finishInitial = resolve }))
      .mockImplementationOnce(() => new Promise(resolve => { finishReentry = resolve }))
    wrapper = mount({ render: () => h(KeepAlive, () => visible.value ? h(SyncView) : null) })
    await flushPromises()
    expect(callbacks.size).toBe(3)
    expect(api.ListRunningSyncIDs).toHaveBeenCalledTimes(1)
    visible.value = false
    await nextTick()
    expect(callbacks.size).toBe(0)
    visible.value = true
    await nextTick()
    expect(callbacks.size).toBe(3)
    expect(api.ListSyncConfigs).toHaveBeenCalledTimes(2)
    const view = wrapper.getComponent(SyncView).vm
    callbacks.get('sync:state')({ id: 'ended', running: false })
    callbacks.get('sync:state')({ id: 'started', running: true })
    finishReentry(['ended', 'already-running'])
    finishInitial(['outdated'])
    await flushPromises()
    expect([...view.running]).toEqual(['already-running', 'started'])
  })

  it('进度突发更新在同一帧合并，结束的任务不回显旧进度', async () => {
    const callbacks = new Map()
    api.onEvent.mockImplementation((name, callback) => {
      callbacks.set(name, callback)
      return () => callbacks.delete(name)
    })
    api.ListSyncConfigs.mockResolvedValue([])
    api.ListRunningSyncIDs.mockResolvedValue([])
    const frames = new Map()
    let frameId = 0
    const requestFrame = vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => {
      frames.set(++frameId, callback)
      return frameId
    })
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(id => frames.delete(id))
    wrapper = mount(SyncView)
    await flushPromises()
    requestFrame.mockClear()
    const event = callbacks.get('sync:progress')
    for (let done = 1; done <= 500; done++) event({ id: 'task', done, total: 500 })
    event({ id: 'ended', done: 5, total: 5 })
    callbacks.get('sync:state')({ id: 'ended', running: false })
    expect(requestFrame).toHaveBeenCalledTimes(1)
    expect(wrapper.vm.progress).toEqual({})
    frames.get(frameId)()
    expect(wrapper.vm.progress).toEqual({ task: { done: 500, total: 500 } })
    event({ id: 'task', done: 1, total: 5 })
    wrapper.unmount()
    wrapper = null
    expect(frames.has(frameId)).toBe(false)
    expect(callbacks.size).toBe(0)
  })

  it('单条进度只更新对应进度条，不重新计算列表账号信息', async () => {
    vi.useFakeTimers({ toFake: ['requestAnimationFrame', 'cancelAnimationFrame'] })
    try {
      const callbacks = new Map()
      api.onEvent.mockImplementation((name, callback) => { callbacks.set(name, callback); return () => callbacks.delete(name) })
      const jobs = Array.from({ length: 40 }, (_, index) => ({ id: `sync-${index}`, name: `任务 ${index}`, user_id: 'account', enabled: true }))
      api.ListSyncConfigs.mockResolvedValue(jobs)
      api.ListRunningSyncIDs.mockResolvedValue(jobs.map(job => job.id))
      wrapper = mount(SyncView, { props: { accounts: [{ user_id: 'account' }] } })
      await flushPromises()
      const bars = wrapper.findAll('[role="progressbar"]')
      expect(bars).toHaveLength(40)
      api.providerMetaOf.mockClear()
      callbacks.get('sync:progress')({ id: 'sync-0', done: 5, total: 10 })
      vi.advanceTimersToNextFrame()
      await nextTick()
      expect(bars[0].attributes('aria-valuenow')).toBe('50')
      expect(bars[1].attributes('aria-valuenow')).toBeUndefined()
      expect(api.providerMetaOf).not.toHaveBeenCalled()
    } finally {
      vi.useRealTimers()
    }
  })

  it('删除计划必须再次确认，预览变化后不执行旧确认', async () => {
    api.ListSyncConfigs.mockResolvedValue([])
    api.ListRunningSyncIDs.mockResolvedValue([])
    api.RunSyncPlan.mockResolvedValue()
    wrapper = mount(SyncView, { global: { stubs: { UiIcon: true } } })
    await flushPromises()
    wrapper.vm.previewJob = { id: 'job', name: '同步', local_dir: 'D:/source', remote_dir: '/target' }
    wrapper.vm.preview = { token: 'one', changes: [{ path: '旧文件', action: 'delete-local' }] }
    await wrapper.vm.executePreview()
    expect(api.RunSyncPlan).not.toHaveBeenCalled()
    expect(wrapper.vm.confirmDialog.message).toContain('本地 1 项')
    const confirmOld = wrapper.vm.confirmDialog.onOk
    wrapper.vm.preview = { token: 'two', changes: [{ path: '其他文件', action: 'delete-local' }] }
    await confirmOld()
    expect(api.RunSyncPlan).not.toHaveBeenCalled()
    await wrapper.vm.executePreview()
    await wrapper.vm.confirmDialog.onOk()
    expect(api.RunSyncPlan).toHaveBeenCalledWith('job', 'two', {})
  })
  it('首次加载失败时显示重试而非空任务提示', async () => {
    api.ListSyncConfigs.mockRejectedValueOnce(new Error('网络异常')).mockResolvedValue([])
    api.ListRunningSyncIDs.mockResolvedValue([])
    wrapper = mount(SyncView, { global: { stubs: { UiIcon: true } } })
    await flushPromises()

    expect(wrapper.text()).toContain('同步任务加载失败')
    expect(wrapper.text()).not.toContain('还没有同步任务')
    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('还没有同步任务')
  })

  it('启停设置保存期间不伪装为正在同步', async () => {
    const job = { id: 'job', name: '任务', user_id: 'user', drive_id: 'drive', enabled: true, local_dir: 'D:/data', remote_dir: 'root' }
    api.ListSyncConfigs.mockResolvedValue([job])
    api.ListRunningSyncIDs.mockResolvedValue([])
    let finishSave
    api.SaveSyncConfig.mockImplementation(() => new Promise(resolve => { finishSave = resolve }))
    wrapper = mount(SyncView, { props: { accounts: [{ user_id: 'user', drive_id: 'drive' }] }, global: { stubs: { UiIcon: true } } })
    await flushPromises()

    const saving = wrapper.vm.toggle(job)
    await flushPromises()
    expect(wrapper.vm.running.has(job.id)).toBe(false)
    expect(wrapper.get('.sync-task-actions button[role="switch"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('.sync-task-actions button[title="预览同步"]').exists()).toBe(true)
    expect(api.CancelSync).not.toHaveBeenCalled()

    finishSave()
    await saving
    await flushPromises()
    expect(wrapper.vm.toggling.has(job.id)).toBe(false)
    expect(wrapper.get('.sync-task-actions button[role="switch"]').attributes('aria-checked')).toBe('false')
  })

  it('确认删除时再次检查任务是否已开始同步', async () => {
    const job = { id: 'job', name: '任务', user_id: 'user', drive_id: 'drive', enabled: true, local_dir: 'D:/data', remote_dir: 'root' }
    api.ListSyncConfigs.mockResolvedValue([job])
    api.ListRunningSyncIDs.mockResolvedValue([])
    wrapper = mount(SyncView, { props: { accounts: [{ user_id: 'user', drive_id: 'drive' }] }, global: { stubs: { UiIcon: true } } })
    await flushPromises()

    wrapper.vm.remove(job)
    wrapper.vm.running = new Set([job.id])
    await wrapper.vm.confirmDialog.onOk()
    expect(api.DeleteSyncConfig).not.toHaveBeenCalled()
    expect(wrapper.emitted('toast')).toContainEqual([expect.stringContaining('正在同步或更新中'), 'warn'])
  })
})
