import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const batchResumeTasks = vi.fn()
const syncTaskbarProgress = vi.fn()

vi.mock('./aria2TaskApi', () => ({
  batchPauseTasks: vi.fn(),
  batchResumeTasks,
  batchRemoveTasks: vi.fn()
}))

vi.mock('../../utils/message', () => ({
  default: {
    info: vi.fn(),
    error: vi.fn(),
    warning: vi.fn(),
    success: vi.fn()
  }
}))

vi.mock('../../utils/dbdown', () => ({
  default: {
    saveDownings: vi.fn(),
    deleteDownings: vi.fn(),
    deleteDowningAll: vi.fn()
  }
}))

vi.mock('../DownDAL', () => ({
  default: {
    stopDowning: vi.fn(),
    deleteDowning: vi.fn(),
    deleteDowned: vi.fn(),
    syncTaskbarProgress
  }
}))

describe('DowningStore start state', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    batchResumeTasks.mockReset()
    syncTaskbarProgress.mockReset()
  })

  it('resumes a paused aria task when selected task is started', async () => {
    const { default: useDowningStore } = await import('../DowningStore')
    const store = useDowningStore()
    store.ListDataRaw = [{
      DownID: 'task-1',
      Info: {
        GID: 'gid-1',
        user_id: 'external',
        DownSavePath: '/tmp',
        ariaRemote: false,
        file_id: 'file-1',
        drive_id: 'external',
        name: 'movie.mkv',
        size: 0,
        sizestr: '',
        icon: 'iconcloud-download',
        isDir: false,
        encType: '',
        sha1: '',
        crc64: '',
        sourceType: 'url'
      },
      Down: {
        DownState: '已暂停',
        DownTime: 1,
        DownSize: 0,
        DownSpeed: 0,
        DownSpeedStr: '',
        DownProcess: 0,
        IsStop: true,
        IsDowning: false,
        IsCompleted: false,
        IsFailed: false,
        FailedCode: 0,
        FailedMessage: '',
        AutoTry: 0,
        DownUrl: ''
      }
    }]
    store.ListSelected = new Set(['task-1'])

    await store.mStartDowning()

    expect(store.ListDataRaw[0].Down.DownState).toBe('队列中')
    expect(batchResumeTasks).toHaveBeenCalledWith(['gid-1'])
  })

  it('only retries a restored failed task after the user starts it', async () => {
    const { default: useDowningStore } = await import('../DowningStore')
    const store = useDowningStore()
    store.ListDataRaw = [{
      DownID: 'task-failed',
      Info: {
        GID: 'gid-failed',
        user_id: 'external',
        DownSavePath: '/tmp',
        ariaRemote: false,
        file_id: 'file-failed',
        drive_id: 'external',
        name: 'failed.bin',
        size: 100,
        sizestr: '100.00B',
        icon: 'iconcloud-download',
        isDir: false,
        encType: '',
        sha1: '',
        crc64: '',
        sourceType: 'url'
      },
      Down: {
        DownState: '已出错',
        DownTime: 1,
        DownSize: 50,
        DownSpeed: 0,
        DownSpeedStr: '',
        DownProcess: 50,
        IsStop: false,
        IsDowning: false,
        IsCompleted: false,
        IsFailed: true,
        FailedCode: 1,
        FailedMessage: '下载失败',
        AutoTry: 0,
        ManualRetryRequired: true,
        DownUrl: ''
      }
    }]
    store.ListSelected = new Set(['task-failed'])

    await store.mStartDowning()

    expect(store.ListDataRaw[0].Down).toMatchObject({
      DownState: '队列中',
      IsFailed: false,
      ManualRetryRequired: false
    })
    expect(batchResumeTasks).not.toHaveBeenCalled()
  })

  it('clears taskbar progress as soon as a selected task is paused', async () => {
    const { default: useDowningStore } = await import('../DowningStore')
    const store = useDowningStore()
    store.ListDataRaw = [{
      DownID: 'task-running',
      Info: {
        GID: 'gid-running',
        user_id: 'external',
        DownSavePath: '/tmp',
        ariaRemote: false,
        file_id: 'file-running',
        drive_id: 'external',
        name: 'running.bin',
        size: 100,
        sizestr: '100.00B',
        icon: 'iconcloud-download',
        isDir: false,
        encType: '',
        sha1: '',
        crc64: '',
        sourceType: 'url'
      },
      Down: {
        DownState: '下载中',
        DownTime: 1,
        DownSize: 50,
        DownSpeed: 10,
        DownSpeedStr: '10B/s',
        DownProcess: 50,
        IsStop: false,
        IsDowning: true,
        IsCompleted: false,
        IsFailed: false,
        FailedCode: 0,
        FailedMessage: '',
        AutoTry: 0,
        DownUrl: ''
      }
    }]
    store.ListSelected = new Set(['task-running'])

    await store.batchPauseSelected()

    expect(store.ListDataRaw[0].Down.IsDowning).toBe(false)
    expect(syncTaskbarProgress).toHaveBeenCalledTimes(1)
  })
})
