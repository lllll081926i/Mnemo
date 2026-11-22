import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

const api = vi.hoisted(() => ({
  listDir: vi.fn(),
  mkdir: vi.fn(),
  providerMetaOf: vi.fn(),
  providerIconUrl: vi.fn(() => ''),
  accountName: vi.fn(account => account.user_id),
}))

vi.mock('../api', () => api)

import SelectDirModal from './SelectDirModal.vue'

const wrapperOptions = {
  props: {
    account: { user_id: 'webdav:test', drive_id: 'webdav' },
    providers: [],
  },
  global: {
    stubs: {
      Modal: { template: '<section><slot /><slot name="actions" /></section>' },
      UiIcon: true,
    },
  },
}

let wrapper

afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  vi.clearAllMocks()
})

describe('选择目录弹窗', () => {
  it('万条目录仅渲染可见窗口，键盘能跳到末尾、进入并返回', async () => {
    api.providerMetaOf.mockReturnValue({ rootKey: 'root' })
    const dirs = Array.from({ length: 10000 }, (_, index) => ({ file_id: `folder-${index}`, name: `目录-${String(index).padStart(5, '0')}`, isDir: true }))
    api.listDir.mockImplementation(async (_user, _drive, id) => id === 'root' ? dirs : [])
    wrapper = mount(SelectDirModal, wrapperOptions)
    await flushPromises()
    const list = wrapper.get('.directory-list')
    Object.defineProperty(list.element, 'clientHeight', { value: 298 })
    expect(list.findAll('[role="option"]')).toHaveLength(18)
    await list.trigger('keydown', { key: 'End' })
    const active = list.get(`[id="${list.attributes('aria-activedescendant')}"]`)
    expect(active.attributes('aria-posinset')).toBe('10000')
    expect(active.text()).toContain('目录-09999')
    await list.trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(api.listDir).toHaveBeenLastCalledWith('webdav:test', 'webdav', 'folder-9999')
    expect(list.attributes('aria-activedescendant')).toBeUndefined()
    await list.trigger('keydown', { key: 'ArrowLeft' })
    await flushPromises()
    expect(list.element.scrollTop).toBeGreaterThan(0)
    expect(list.get(`[id="${list.attributes('aria-activedescendant')}"]`).attributes('aria-posinset')).toBe('10000')
  })

  it('切换账号时重置根目录，丢弃旧账号的目录响应', async () => {
    api.providerMetaOf.mockImplementation(account => ({ rootKey: account.drive_id, rootTitle: account.user_id }))
    let finishOld
    api.listDir.mockImplementation((user) => user === 'webdav:test'
      ? new Promise(resolve => { finishOld = resolve })
      : Promise.resolve([{ file_id: 'new', name: '新账号目录', isDir: true }]))
    wrapper = mount(SelectDirModal, wrapperOptions)
    await wrapper.setProps({ account: { user_id: 'webdav:other', drive_id: 'other-root' } })
    await flushPromises()
    expect(api.listDir).toHaveBeenLastCalledWith('webdav:other', 'other-root', 'other-root')
    finishOld([{ file_id: 'old', name: '旧账号目录', isDir: true }])
    await flushPromises()
    expect(wrapper.text()).toContain('新账号目录')
    expect(wrapper.text()).not.toContain('旧账号目录')
    await wrapper.get('button.primary').trigger('click')
    expect(wrapper.emitted('select')).toEqual([[{ id: 'other-root', name: 'webdav:other' }]])
  })

  it('旧账号新建目录完成后不改动新账号的输入与目录', async () => {
    api.providerMetaOf.mockReturnValue({ rootKey: 'root', rootTitle: '根目录' })
    api.listDir.mockResolvedValue([])
    let finishCreate
    api.mkdir.mockImplementation(() => new Promise(resolve => { finishCreate = resolve }))
    wrapper = mount(SelectDirModal, wrapperOptions)
    await flushPromises()
    await wrapper.get('input').setValue('旧目录')
    await wrapper.get('.new-folder-row button').trigger('click')
    await wrapper.setProps({ account: { user_id: 'webdav:other', drive_id: 'other' } })
    await flushPromises()
    await wrapper.get('input').setValue('新账号输入')
    const calls = api.listDir.mock.calls.length
    finishCreate({ file_id: 'created' })
    await flushPromises()
    expect(wrapper.get('input').element.value).toBe('新账号输入')
    expect(api.listDir).toHaveBeenCalledTimes(calls)
    expect(wrapper.emitted('toast')).toBeUndefined()
  })

  it('创建后刷新目录期间切换账号不发送旧账号成功提示', async () => {
    api.providerMetaOf.mockReturnValue({ rootKey: 'root', rootTitle: '根目录' })
    api.listDir.mockResolvedValue([])
    api.mkdir.mockResolvedValue({ file_id: 'created' })
    wrapper = mount(SelectDirModal, wrapperOptions)
    await flushPromises()
    let finishRefresh
    api.listDir.mockImplementationOnce(() => new Promise(resolve => { finishRefresh = resolve }))
    await wrapper.get('input').setValue('旧目录')
    await wrapper.get('.new-folder-row button').trigger('click')
    await flushPromises()
    await wrapper.setProps({ account: { user_id: 'webdav:other', drive_id: 'other' } })
    await flushPromises()
    finishRefresh([])
    await flushPromises()
    expect(wrapper.emitted('toast')).toBeUndefined()
  })

  it('同一账号元数据刷新不重置当前目录', async () => {
    api.providerMetaOf.mockReturnValue({ rootKey: 'root', rootTitle: '根目录' })
    api.listDir.mockImplementation((_user, _drive, id) => Promise.resolve(id === 'root' ? [{ file_id: 'child', name: '子目录', isDir: true }] : []))
    wrapper = mount(SelectDirModal, wrapperOptions)
    await flushPromises()
    await wrapper.get('.tree-node').trigger('click')
    await flushPromises()
    const calls = api.listDir.mock.calls.length
    await wrapper.setProps({ account: { ...wrapperOptions.props.account, custom_name: '新的昵称' } })
    await flushPromises()
    expect(api.listDir).toHaveBeenCalledTimes(calls)
    await wrapper.get('button.primary').trigger('click')
    expect(wrapper.emitted('select')).toEqual([[{ id: 'child', name: '子目录' }]])
  })

  it('根目录加载失败后可直接重试', async () => {
    api.providerMetaOf.mockReturnValue({ rootKey: 'root', rootTitle: '根目录' })
    api.listDir.mockRejectedValueOnce(new Error('网络暂不可用')).mockResolvedValue([{ file_id: 'child', name: '子目录', isDir: true }])
    wrapper = mount(SelectDirModal, wrapperOptions)
    await flushPromises()

    expect(wrapper.text()).toContain('网络暂不可用')
    await wrapper.get('button.btn.sm').trigger('click')
    await flushPromises()
    expect(api.listDir).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('子目录')
  })

  it('新建目录返回业务错误时保留输入并展示失败，不伪报成功', async () => {
    api.providerMetaOf.mockReturnValue({ rootKey: 'root', rootTitle: '根目录' })
    api.listDir.mockResolvedValue([])
    api.mkdir.mockResolvedValue({ error: 'context canceled' })
    wrapper = mount(SelectDirModal, wrapperOptions)
    await nextTick()
    await Promise.resolve()

    await wrapper.get('input[placeholder="新建文件夹..."]').setValue('临时目录')
    await wrapper.get('.new-folder-row .btn').trigger('click')
    await Promise.resolve()

    expect(api.mkdir).toHaveBeenCalledWith('webdav:test', 'webdav', 'root', '临时目录')
    expect(wrapper.emitted('toast')).toEqual([['Error: context canceled', 'error']])
    expect(wrapper.emitted('toast')).not.toContainEqual(['文件夹已创建', 'success'])
    expect(wrapper.get('input[placeholder="新建文件夹..."]').element.value).toBe('临时目录')
  })
})
