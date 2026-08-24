import { mount } from '@vue/test-utils'
import { computed, defineComponent, effectScope, nextTick, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

const api = vi.hoisted(() => ({
  DeleteDirectoryCache: vi.fn(),
  ListDirPage: vi.fn(),
  SaveDirectoryCache: vi.fn(),
  listDir: vi.fn(),
  providerOf: vi.fn((userID) => String(userID || '').split(/[_:]/, 1)[0]),
}))

vi.mock('../api', () => api)

import { useDirectoryCache } from './useDirectoryCache'
import { useFileSelection } from './useFileSelection'
import { useVirtualFileList } from './useVirtualFileList'

afterEach(() => {
  vi.useRealTimers()
  vi.clearAllMocks()
})

describe('PanView composables', () => {
  it('按分页游标合并目录，并向界面逐页提交快照', async () => {
    vi.useFakeTimers()
    api.ListDirPage
      .mockResolvedValueOnce({ items: [{ file_id: 'a' }], nextMarker: 'page-2' })
      .mockResolvedValueOnce({ items: [{ file_id: 'b' }], nextMarker: '' })
    const snapshots = []
    const cache = useDirectoryCache()
    const resultPromise = cache.listProgressively('pikpak_user', 'drive', 'root', () => true, (files) => {
      snapshots.push(files.map((file) => file.file_id))
    })

    await vi.advanceTimersByTimeAsync(180)
    await expect(resultPromise).resolves.toEqual([{ file_id: 'a' }, { file_id: 'b' }])
    expect(api.ListDirPage.mock.calls.map((call) => call[3])).toEqual(['', 'page-2'])
    expect(snapshots).toEqual([['a'], ['a', 'b']])
  })

  it('目录变更会立即遮蔽旧快照，并串行删除持久化缓存', async () => {
    api.DeleteDirectoryCache.mockResolvedValue(undefined)
    const cache = useDirectoryCache()
    const key = cache.directoryKey('pan189_user', 'drive', 'list', 'folder', '')
    cache.put(key, [{ file_id: 'stale' }])
    expect(cache.get(key)?.files).toEqual([{ file_id: 'stale' }])

    cache.invalidateDirectory('pan189_user', 'drive', 'list', 'folder')
    expect(cache.get(key)).toBeNull()
    await vi.waitFor(() => expect(api.DeleteDirectoryCache).toHaveBeenCalledWith(key))
  })

  it('保留桌面式区间、反选和列表变更后的选中项裁剪', async () => {
    const files = [
      { file_id: 'a', name: 'A' },
      { file_id: 'b', name: 'B' },
      { file_id: 'c', name: 'C' },
    ]
    const visibleFiles = ref(files)
    const scope = effectScope()
    const selection = scope.run(() => useFileSelection(visibleFiles))

    selection.toggle(files[0])
    selection.toggle(files[2], { shiftKey: true })
    expect(selection.selected.value.map((file) => file.file_id)).toEqual(['a', 'b', 'c'])

    selection.invert()
    expect(selection.selected.value).toEqual([])
    selection.toggleRangeSelecting()
    selection.toggle(files[1])
    selection.toggle(files[2])
    expect(selection.selected.value.map((file) => file.file_id)).toEqual(['b', 'c'])
    expect(selection.rangeSelecting.value).toBe(false)

    visibleFiles.value = [files[2]]
    await nextTick()
    expect(selection.selected.value).toEqual([files[2]])
    scope.stop()
  })

  it('大目录只渲染视口附近行，选择和业务数据仍保留完整列表', () => {
    const files = ref(Array.from({ length: 250 }, (_, index) => ({ file_id: String(index) })))
    const rows = computed(() => files.value.map((file) => ({ f: file })))
    let virtualList
    const wrapper = mount(defineComponent({
      setup() {
        virtualList = useVirtualFileList({
          files,
          getLoadSequence: () => 1,
          loading: ref(false),
          rowsShown: rows,
          viewKey: () => 'account|drive|list|root|',
          viewMode: ref('list'),
          visibleFiles: files,
        })
        return () => null
      },
    }))

    expect(virtualList.listVirtualized.value).toBe(true)
    expect(files.value).toHaveLength(250)
    expect(virtualList.listRenderRows.value).toHaveLength(18)
    expect(virtualList.listVirtualTop.value).toBe(0)
    expect(virtualList.listVirtualBottom.value).toBe((250 - 18) * 48)
    wrapper.unmount()
  })
})
