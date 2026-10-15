import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import DeletionHistory from './DeletionHistory.vue'

vi.mock('../api', () => ({ formatBytes: value => `${value} B`, formatTime: value => String(value) }))
let wrapper
afterEach(() => { wrapper?.unmount(); vi.unstubAllGlobals() })
const account = { user_id: 'onedrive:user', drive_id: 'drive', token: { provider_drive_type: 'personal' } }
function setup(bridge, selectedAccount = account) {
  vi.stubGlobal('go', { app: { App: bridge } })
  wrapper = mount(DeletionHistory, { props: { account: selectedAccount }, global: { stubs: { Modal: { template: '<div><slot /></div>' } } } })
}
describe('本机删除记录', () => {
  it('清除仅处理当前作用域，取消不清除', async () => {
    const bridge = { ListDeletionHistory: vi.fn().mockResolvedValue([{ fileId: 'file', name: '原文件', status: 'deleted' }]), ClearDeletionHistory: vi.fn().mockResolvedValue() }
    setup(bridge)
    await flushPromises()
    expect(wrapper.text()).toContain('不是完整云端回收站')
    const clear = () => wrapper.findAll('button').find(button => button.text() === '清除本机记录')
    await clear().trigger('click')
    await wrapper.findComponent({ name: 'ConfirmModal' }).vm.$emit('cancel')
    expect(bridge.ClearDeletionHistory).not.toHaveBeenCalled()
    await clear().trigger('click')
    await wrapper.findComponent({ name: 'ConfirmModal' }).vm.$emit('ok')
    await flushPromises()
    expect(bridge.ClearDeletionHistory).toHaveBeenCalledWith('onedrive:user', 'drive')
  })
  it('恢复失败后重读检查点，不显示成功', async () => {
    const bridge = { ListDeletionHistory: vi.fn().mockResolvedValue([{ fileId: 'file', name: '原文件', providerType: 'personal', status: 'deleted' }]), RestoreDeletionHistory: vi.fn().mockRejectedValue('回执保存失败') }
    setup(bridge)
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '恢复').trigger('click')
    await flushPromises()
    expect(bridge.ListDeletionHistory).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('回执保存失败')
    expect(wrapper.emitted('toast')).toBeUndefined()
  })
  it('企业只读快照读取失败保留上次快照，不提供恢复操作', async () => {
    const bridge = { ListDeletionHistory: vi.fn().mockResolvedValue([]), PreviewEnterpriseTrash: vi.fn().mockResolvedValueOnce({ scope: '一级只读', items: [{ id: 'one', siteId: 'site', name: '快照文件' }] }).mockRejectedValueOnce('403 权限不足') }
    setup(bridge, { ...account, token: { provider_drive_type: 'business' } })
    await flushPromises()
    const preview = wrapper.findAll('button').find(button => button.text().includes('试验读取'))
    await preview.trigger('click'); await flushPromises()
    await preview.trigger('click'); await flushPromises()
    expect(wrapper.text()).toContain('快照文件')
    expect(wrapper.text()).toContain('当前快照未更新')
    expect(wrapper.findAll('button').some(button => button.text() === '恢复')).toBe(false)
  })
})
