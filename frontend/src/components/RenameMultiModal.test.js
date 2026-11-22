import { DOMWrapper, flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import RenameMultiModal from './RenameMultiModal.vue'
import { RenameBatch } from '../api'

vi.mock('../api', () => ({ RenameBatch: vi.fn() }))

let wrapper
function open(files) {
  wrapper = mount(RenameMultiModal, { props: { account: { user_id: 'account', drive_id: 'drive' }, files } })
  return new DOMWrapper(document.body)
}
afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  vi.clearAllMocks()
})

describe('批量重命名预览', () => {
  it('万条预览按视口渲染，但提交包含全部新名称', async () => {
    const files = Array.from({ length: 10000 }, (_, index) => ({ file_id: `item-${index}`, name: '报告.txt', isDir: false }))
    RenameBatch.mockImplementation(async (_user, _drive, refs) => refs.map(ref => ({ file_id: ref.id })))
    const body = open(files)
    Object.assign(wrapper.vm.rules.index, { enable: true, format: '_#', minlen: 5, beginindex: 1 })
    await wrapper.vm.$nextTick()
    const viewport = body.get('.rr-preview')
    expect(viewport.findAll('.rr-row.changed')).toHaveLength(16)
    viewport.element.scrollTop = 32 * 9994
    await viewport.trigger('scroll')
    expect(viewport.findAll('.rr-row.changed').length).toBeLessThanOrEqual(16)
    expect(viewport.text()).toContain('报告_10000.txt')
    await body.get('.modal-actions .primary').trigger('click')
    await flushPromises()
    const [user, drive, refs, names] = RenameBatch.mock.calls[0]
    expect([user, drive, refs.length, names.length]).toEqual(['account', 'drive', 10000, 10000])
    expect(refs.at(-1)).toEqual({ id: 'item-9999', isDir: false })
    expect(names.at(-1)).toBe('报告_10000.txt')
  })

  it('无效正则阻止提交，修正后只提交实际变化的项目', async () => {
    const body = open([{ file_id: 'first', name: 'AbA.txt' }, { file_id: 'second', name: '原名.txt' }])
    Object.assign(wrapper.vm.rules.replace, { chkReg: true, search: '[', newword: '甲' })
    await wrapper.vm.$nextTick()
    expect(body.get('[role="alert"]').text()).toContain('不是有效的正则表达式')
    await wrapper.vm.submit()
    expect(RenameBatch).not.toHaveBeenCalled()
    wrapper.vm.rules.replace.search = 'a'
    RenameBatch.mockResolvedValue([{ file_id: 'first' }])
    await wrapper.vm.submit()
    expect(RenameBatch).toHaveBeenCalledExactlyOnceWith('account', 'drive', [{ id: 'first', isDir: undefined }], ['甲b甲.txt'])
  })
})
