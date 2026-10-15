import { expect, it } from 'vitest'
import { summarizeSync } from './sync-risk'

it('统计上传下载冲突与两侧删除，不把冲突当成已决定的删除', () => {
  expect(summarizeSync([{ action: 'upload' }, { action: 'download' }, { action: 'conflict' }, { action: 'delete-local' }, { action: 'delete-remote' }])).toEqual({ upload: 1, download: 1, conflict: 1, 'delete-local': 1, 'delete-remote': 1, deletes: 2 })
  expect(summarizeSync().deletes).toBe(0)
})
