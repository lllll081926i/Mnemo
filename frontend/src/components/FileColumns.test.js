import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

vi.mock('../api', () => ({ formatBytes: size => `${size} B`, iconOf: file => file.isDir ? 'folder' : 'file' }))
import FileColumns from './FileColumns.vue'

let wrapper
afterEach(() => wrapper?.unmount())

describe('分栏浏览', () => {
  it('大目录只渲染可见文件，滚动后显示对应文件', async () => {
    const files = Array.from({ length: 10000 }, (_, id) => ({ file_id: String(id), name: `文件${id}`, size: id }))
    wrapper = mount(FileColumns, { props: { columns: [{ id: 'root', name: '根目录', files }], selectedIds: new Set(), focusId: '' } })
    expect(wrapper.findAll('.column-file').length).toBeLessThan(100)
    const list = wrapper.get('.column-list')
    list.element.scrollTop = 38000
    await list.trigger('scroll')
    expect(wrapper.text()).toContain('文件1000')
    expect(wrapper.text()).not.toContain('文件0 ')
    await wrapper.setProps({ focusId: '9999' })
    await flushPromises()
    expect(wrapper.text()).toContain('文件9999')
    expect(wrapper.findAll('.column-file').length).toBeLessThan(100)
  })
  it('祖先目录保留路径选中态，右键和重试带上所属列', async () => {
    const folder = { file_id: 'child', name: '项目', isDir: true }
    wrapper = mount(FileColumns, { props: { columns: [{ id: 'root', name: '根目录', files: [folder], activeId: 'child' }, { id: 'child', name: '项目', files: [], error: '加载失败' }], selectedIds: new Set(), focusId: '' } })
    expect(wrapper.get('.column-file').classes()).toContain('selected')
    await wrapper.get('.column-file').trigger('contextmenu')
    expect(wrapper.emitted('context')[0][0]).toBe(0)
    expect(wrapper.emitted('context')[0][2]).toEqual(folder)
    await wrapper.get('.column-state button').trigger('click')
    expect(wrapper.emitted('retry')[0]).toEqual(['child'])
  })
})
