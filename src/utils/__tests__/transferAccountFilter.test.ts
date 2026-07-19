import { beforeEach, describe, expect, it } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import useUploadingStore, { type IUploadingModel } from '../../down/UploadingStore'
import useUploadedStore from '../../down/UploadedStore'
import type { IStateUploadTask } from '../dbupload'

const uploadingItem = (taskId: number, userId: string): IUploadingModel => ({
  UploadID: taskId,
  TaskID: taskId,
  user_id: userId,
  localFilePath: `C:/temp/${taskId}`,
  name: `task-${taskId}`,
  sizeStr: '1 MB',
  icon: 'iconwenjian',
  isDir: false,
  uploadState: '排队中',
  speedStr: '',
  Progress: 0,
  ProgressStr: '',
  errorMessage: ''
})

const uploadedItem = (taskId: number, userId: string) =>
  ({
    TaskID: taskId,
    user_id: userId,
    TaskName: `uploaded-${taskId}`,
    localFilePath: `C:/temp/${taskId}`,
    ChildTotalSize: 1024
  }) as IStateUploadTask

describe('transfer account filters', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('filters active uploads without discarding aggregate tasks', () => {
    const store = useUploadingStore()
    store.aLoadListData(0, '', [uploadingItem(1, 'account-a'), uploadingItem(2, 'account-b')], 2)

    store.mSetAccountFilter('account-b')
    expect(store.ListDataShow.map((item) => item.TaskID)).toEqual([2])
    expect(store.ListDataRaw).toHaveLength(2)
  })

  it('filters upload history without discarding aggregate records', () => {
    const store = useUploadedStore()
    store.aLoadListData([uploadedItem(1, 'account-a'), uploadedItem(2, 'account-b')], 2)

    store.mSetAccountFilter('account-a')
    expect(store.ListDataShow.map((item) => item.TaskID)).toEqual([1])
    expect(store.ListDataRaw).toHaveLength(2)
  })
})
