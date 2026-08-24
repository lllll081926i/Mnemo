import { describe, expect, it } from 'vitest'
import {
  downStatusBadge,
  downStatusText,
  migCompletedTopLevel,
  migProgress,
  migProgressText,
  migRemaining,
  offProgress,
  upName,
  upStatus,
} from './transferTaskUtils'

describe('transferTaskUtils', () => {
  it('保持下载和上传状态映射', () => {
    expect(downStatusText('paused')).toBe('已暂停')
    expect(downStatusBadge('failed')).toBe('error')
    expect(upStatus({ Upload: { IsCompleted: true, IsFailed: true } })).toBe('已完成')
    expect(upStatus({ Upload: { IsFailed: true } })).toBe('失败')
    expect(upName({ Info: { localFilePath: 'C:\\data\\movie.mkv' } })).toBe('movie.mkv')
  })

  it('钳制离线与迁移进度', () => {
    expect(offProgress({ progress: 101.2 })).toBe(100)
    expect(offProgress({ progress: -3 })).toBe(0)
    expect(migProgress({ processedBytes: 75, totalBytes: 100 })).toBe(75)
    expect(migProgress({ processed: 3, total: 4 })).toBe(75)
  })

  it('保持迁移顶层完成统计和展示', () => {
    const job = { fileIDs: ['a', 'b', 'c'], completedFileIDs: ['b'], processed: 1, total: 3 }
    expect(migCompletedTopLevel(job)).toBe(1)
    expect(migRemaining(job)).toBe(2)
    expect(migProgressText(job)).toBe('1 / 3 个文件')
  })
})
