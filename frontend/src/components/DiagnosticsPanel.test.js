import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

const api = vi.hoisted(() => ({
  listAccounts: vi.fn().mockResolvedValue([{ user_id: 'account', drive_id: 'drive' }]),
  accountName: vi.fn(() => '测试账号'),
  RunDiagnostics: vi.fn().mockResolvedValue({ checks: [{ name: '下载目录', status: 'passed', message: '可写入' }] }),
  ExportDiagnostics: vi.fn().mockResolvedValue('diagnostic.json'),
}))
vi.mock('../api', () => api)
import DiagnosticsPanel from './DiagnosticsPanel.vue'

let wrapper
afterEach(() => { wrapper?.unmount(); vi.clearAllMocks() })

describe('按需诊断', () => {
  it('默认不加载账号或联网，展开后运行诊断并允许导出', async () => {
    wrapper = mount(DiagnosticsPanel)
    expect(wrapper.find('.diagnostics-actions').exists()).toBe(false)
    expect(api.listAccounts).not.toHaveBeenCalled()
    wrapper.element.open = true
    await wrapper.trigger('toggle')
    await flushPromises()
    expect(api.listAccounts).toHaveBeenCalledOnce()
    expect(api.RunDiagnostics).not.toHaveBeenCalled()
    await wrapper.get('.primary').trigger('click')
    await flushPromises()
    expect(api.RunDiagnostics).toHaveBeenCalledExactlyOnceWith('', '', false)
    expect(wrapper.text()).toContain('下载目录 · 通过')
    await wrapper.findAll('button').find(button => button.text() === '导出诊断').trigger('click')
    await flushPromises()
    expect(api.ExportDiagnostics).toHaveBeenCalledOnce()
    await wrapper.get('input').setValue(true)
    expect(wrapper.find('.diagnostic-checks').exists()).toBe(false)
  })
})
