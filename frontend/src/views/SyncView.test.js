import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

const api = vi.hoisted(() => ({
  ListSyncConfigs: vi.fn(),
  SaveSyncConfig: vi.fn(),
  DeleteSyncConfig: vi.fn(),
  PreviewSync: vi.fn(),
  RunSync: vi.fn(),
  RunSyncPlan: vi.fn(),
  CancelSync: vi.fn(),
  ListRunningSyncIDs: vi.fn(),
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
})

describe('同步任务操作状态', () => {
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
