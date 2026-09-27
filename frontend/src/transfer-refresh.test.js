import { describe, expect, it } from 'vitest'
import { mergeLiveTasks } from './transfer-refresh'

describe('传输任务刷新', () => {
  it('保留请求期间收到的最新进度与阶段', () => {
    const snapshot = [{ id: 'file', progress: 10, stage: 'hashing' }, { id: 'other', progress: 20 }]
    const events = new Map([['file', { id: 'file', progress: 80, stage: 'uploading' }]])
    expect(mergeLiveTasks(snapshot, events, (task) => task.id)).toEqual([
      { id: 'file', progress: 80, stage: 'uploading' },
      { id: 'other', progress: 20 },
    ])
  })

  it('不会用旧列表复活已移除任务，并保留新增任务', () => {
    const snapshot = [{ id: 'removed' }, { id: 'remaining' }]
    const events = new Map([['removed', null], ['new', { id: 'new', progress: 1 }]])
    expect(mergeLiveTasks(snapshot, events, (task) => task.id)).toEqual([
      { id: 'new', progress: 1 },
      { id: 'remaining' },
    ])
  })
})
