import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

const api = vi.hoisted(() => ({
  CheckUpdate: vi.fn(),
  GetUpdateStatus: vi.fn(),
  DownloadUpdate: vi.fn(),
  CancelUpdate: vi.fn(),
  ApplyUpdate: vi.fn(),
  RevealInFolder: vi.fn(),
  OpenBrowser: vi.fn(),
  getSettings: vi.fn().mockResolvedValue({}),
  onEvent: vi.fn(() => () => {}),
}))

vi.mock('../api', () => api)

import UpdateModal from './UpdateModal.vue'

let wrapper
afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  document.body.replaceChildren()
  vi.clearAllMocks()
})

describe('更新弹窗', () => {
  it('已有新版本信息时立即展示更新入口，不等待状态请求', async () => {
    let finishStatus
    api.GetUpdateStatus.mockImplementation(() => new Promise(resolve => { finishStatus = resolve }))
    wrapper = mount(UpdateModal, { attachTo: document.body, props: { initialInfo: { available: true, version: '0.4.3' } }, global: { stubs: { UiIcon: true } } })
    await flushPromises()
    expect(document.body.textContent).toContain('发现新版本')
    expect(document.body.textContent).toContain('不代表已验证发布者签名')
    expect(document.body.textContent).not.toContain('正在检查更新')
    finishStatus({ revision: 1, phase: 'idle' })
    await flushPromises()
    expect(document.body.textContent).toContain('发现新版本')
  })

  it('安装包路径缺失时禁止安装与打开目录，并提供重新下载', async () => {
    api.GetUpdateStatus.mockResolvedValue({ revision: 1, phase: 'done', path: '', info: { available: true, canInstall: true } })
    wrapper = mount(UpdateModal, { attachTo: document.body, global: { stubs: { UiIcon: true } } })
    await flushPromises()
    const buttons = [...document.querySelectorAll('.upd-actions button')]
    expect(buttons.find(button => button.textContent === '安装并重启')?.disabled).toBe(true)
    expect(buttons.find(button => button.textContent === '打开目录')?.disabled).toBe(true)
    expect(buttons.some(button => button.textContent === '重新下载')).toBe(true)
  })
})
