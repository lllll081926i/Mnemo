import { describe, expect, it } from 'vitest'
import { resolveAriaProgressErrorState, resolveDownloadTaskbarProgress, resolveRestoredDownloadState, type RestoredDownloadState } from './downloadProgressState'

describe('resolveAriaProgressErrorState', () => {
  it('clears stale error state when aria reports a running task', () => {
    const state = resolveAriaProgressErrorState(
      {
        status: 'active',
        errorCode: '',
        errorMessage: ''
      },
      () => '创建 Aria 任务失败连接断开'
    )

    expect(state).toEqual({
      isFailed: false,
      failedCode: 0,
      failedMessage: ''
    })
  })

  it('formats aria errors when aria reports an error status', () => {
    const state = resolveAriaProgressErrorState(
      {
        status: 'error',
        errorCode: '19',
        errorMessage: 'Name resolution failed'
      },
      (code, message) => `${code}:${message}`
    )

    expect(state).toEqual({
      isFailed: true,
      failedCode: 19,
      failedMessage: '19:Name resolution failed'
    })
  })
})

describe('resolveDownloadTaskbarProgress', () => {
  it('resets the taskbar when no task is actively downloading', () => {
    expect(resolveDownloadTaskbarProgress([{
      Info: { size: 100 },
      Down: { DownSize: 50, IsDowning: false, IsCompleted: false }
    }])).toBe(-1)
  })

  it('calculates progress from active tasks and honors the setting switch', () => {
    const tasks = [{
      Info: { size: 100 },
      Down: { DownSize: 40, IsDowning: true, IsCompleted: false }
    }]
    expect(resolveDownloadTaskbarProgress(tasks)).toBe(0.4)
    expect(resolveDownloadTaskbarProgress(tasks, false)).toBe(-1)
  })
})

describe('resolveRestoredDownloadState', () => {
  const state = (overrides: Partial<RestoredDownloadState> = {}): RestoredDownloadState => ({
    DownState: '下载中',
    DownSpeed: 10,
    DownSpeedStr: '10B/s',
    IsStop: false,
    IsDowning: true,
    IsCompleted: false,
    IsFailed: false,
    FailedCode: 0,
    FailedMessage: '',
    AutoTry: 0,
    ...overrides
  })

  it('keeps a failed task stopped until the user retries it', () => {
    expect(resolveRestoredDownloadState(state({ IsFailed: true, DownState: '已出错', FailedCode: 9, FailedMessage: '磁盘空间不足' }))).toMatchObject({
      DownState: '已出错',
      IsDowning: false,
      IsFailed: true,
      ManualRetryRequired: true,
      FailedCode: 9
    })
  })

  it('queues an interrupted task but preserves a manually paused task', () => {
    expect(resolveRestoredDownloadState(state())).toMatchObject({ DownState: '队列中', IsDowning: false, ManualRetryRequired: false })
    const paused = state({ DownState: '已暂停', IsStop: true, IsDowning: false })
    expect(resolveRestoredDownloadState(paused)).toBe(paused)
  })
})
