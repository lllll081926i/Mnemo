import { ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useTransferTaskModel } from './useTransferTaskModel'

describe('useTransferTaskModel', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  it('按账号、状态和防抖关键词拆分任务', async () => {
    const downloads = ref([
      { id: 'd1', user_id: 'u1', name: 'movie.mkv', status: 'downloading' },
      { id: 'd2', user_id: 'u1', name: 'notes.txt', status: 'completed' },
      { id: 'd3', user_id: 'u2', name: 'movie-2.mkv', status: 'downloading' },
    ])
    const uploads = ref([
      { UploadID: 'u1', UserID: 'u1', Info: { name: 'movie-upload.mkv' }, Upload: { IsCompleted: false } },
      { UploadID: 'u2', UserID: 'u1', Info: { name: 'done.bin' }, Upload: { IsCompleted: true } },
    ])
    const model = useTransferTaskModel({ downloads, uploads, filterUser: ref('u1') })

    expect(model.activeDownloads.value.map((task) => task.id)).toEqual(['d1'])
    expect(model.doneDownloads.value.map((task) => task.id)).toEqual(['d2'])
    expect(model.activeUploads.value.map((task) => task.UploadID)).toEqual(['u1'])
    expect(model.doneUploads.value.map((task) => task.UploadID)).toEqual(['u2'])

    model.taskFilterRaw.value = 'notes'
    await vi.advanceTimersByTimeAsync(120)
    expect(model.activeDownloads.value).toEqual([])
    expect(model.doneDownloads.value.map((task) => task.id)).toEqual(['d2'])
    model.disposeTaskModel()
  })

  it('使用新 Set 维护单选、多选和全选', () => {
    const downloads = ref([
      { id: 'd1', name: 'one', status: 'downloading' },
      { id: 'd2', name: 'two', status: 'paused' },
    ])
    const model = useTransferTaskModel({ downloads, uploads: ref([]), filterUser: ref('') })

    model.onItemClick({ ctrlKey: false, metaKey: false }, 'd1')
    expect([...model.selectedIds.value]).toEqual(['d1'])
    model.onItemClick({ ctrlKey: true, metaKey: false }, 'd2')
    expect(model.allActiveSelected.value).toBe(true)
    expect(model.selectedTasks.value.map((task) => task.id)).toEqual(['d1', 'd2'])
    model.toggleSelectAllActive()
    expect(model.selectedIds.value.size).toBe(0)
    model.disposeTaskModel()
  })
})
