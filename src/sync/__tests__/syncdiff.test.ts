import { describe, expect, it } from 'vitest'
import { computeSyncPlan } from '../syncdiff'
import { LocalFileEntry, RemoteFileEntry, SyncFileEntry } from '../syncmodel'

const local = (relpath: string, size = 100, mtimeMs = 1000): LocalFileEntry => ({ relpath, size, mtimeMs })
const remote = (relpath: string, size = 100, time = 1000): RemoteFileEntry => ({ relpath, file_id: `id:${relpath}`, name: relpath.split('/').pop() || relpath, size, time, isDir: false })
const snap = (size = 100, mtimeMs = 1000, remoteSize = size): SyncFileEntry => ({ size, mtimeMs, remote_file_id: 'id', remote_size: remoteSize, remote_time: 1000 })

const NOW = new Date(2026, 5, 1, 12, 0, 0)

const plan = (
  localMap: Map<string, LocalFileEntry>,
  remoteMap: Map<string, RemoteFileEntry>,
  snapshot: Map<string, SyncFileEntry>,
  direction: 'upload' | 'download' | 'both' = 'both',
  deletePropagation = false,
  deleteThreshold = 20
) => computeSyncPlan(localMap, remoteMap, snapshot, { direction, deletePropagation, deleteThreshold, now: NOW })

describe('computeSyncPlan', () => {
  it('first sync uploads local-only files and downloads remote-only files in both mode', () => {
    const { actions } = plan(new Map([['a.txt', local('a.txt')]]), new Map([['b.txt', remote('b.txt')]]), new Map())
    expect(actions).toEqual([
      { type: 'upload', relpath: 'a.txt', size: 100 },
      { type: 'download', relpath: 'b.txt', size: 100 }
    ])
  })

  it('adopts same-size files present on both sides at first sync without transfer', () => {
    const { actions } = plan(new Map([['a.txt', local('a.txt', 500)]]), new Map([['a.txt', remote('a.txt', 500)]]), new Map())
    expect(actions).toEqual([{ type: 'adopt', relpath: 'a.txt' }])
  })

  it('keeps both versions when a same-name file differs at first sync in both mode', () => {
    const { actions } = plan(new Map([['a.txt', local('a.txt', 100)]]), new Map([['a.txt', remote('a.txt', 200)]]), new Map())
    expect(actions).toHaveLength(1)
    expect(actions[0].type).toBe('conflict')
    expect((actions[0] as any).conflictName).toContain('冲突-云端')
    expect((actions[0] as any).conflictName).toMatch(/^a \(.+\)\.txt$/)
  })

  it('uploads when only local changed, downloads when only remote changed', () => {
    const snapshot = new Map([['a.txt', snap()], ['b.txt', snap()]])
    const { actions } = plan(
      new Map([['a.txt', local('a.txt', 300, 1000)], ['b.txt', local('b.txt')]]),
      new Map([['a.txt', remote('a.txt')], ['b.txt', remote('b.txt', 999)]]),
      snapshot
    )
    expect(actions).toEqual([
      { type: 'upload', relpath: 'a.txt', size: 300 },
      { type: 'download', relpath: 'b.txt', size: 999 }
    ])
  })

  it('does nothing when neither side changed', () => {
    const snapshot = new Map([['a.txt', snap()]])
    const { actions } = plan(new Map([['a.txt', local('a.txt')]]), new Map([['a.txt', remote('a.txt')]]), snapshot)
    expect(actions).toEqual([])
  })

  it('conflicts when both sides changed since the last sync', () => {
    const snapshot = new Map([['a.txt', snap()]])
    const { actions } = plan(new Map([['a.txt', local('a.txt', 300, 5000)]]), new Map([['a.txt', remote('a.txt', 999)]]), snapshot)
    expect(actions).toHaveLength(1)
    expect(actions[0].type).toBe('conflict')
  })

  it('local change wins in upload-only mode even if remote also changed', () => {
    const snapshot = new Map([['a.txt', snap()]])
    const { actions } = plan(new Map([['a.txt', local('a.txt', 300, 5000)]]), new Map([['a.txt', remote('a.txt', 999)]]), snapshot, 'upload')
    expect(actions).toEqual([{ type: 'upload', relpath: 'a.txt', size: 300 }])
  })

  it('never deletes when delete propagation is off, and tombstones instead', () => {
    const snapshot = new Map([['a.txt', snap()]])
    // 本地删除，云端还在 → 双向默认不删云端
    const localDeleted = plan(new Map(), new Map([['a.txt', remote('a.txt')]]), snapshot, 'both', false)
    expect(localDeleted.actions).toEqual([])
    // 云端删除，本地还在 → 双向默认不删本地
    const remoteDeleted = plan(new Map([['a.txt', local('a.txt')]]), new Map(), snapshot, 'both', false)
    expect(remoteDeleted.actions).toEqual([])
  })

  it('propagates deletions in both mode when enabled and within threshold', () => {
    const snapshot = new Map([['a.txt', snap()]])
    const { actions } = plan(new Map(), new Map([['a.txt', remote('a.txt')]]), snapshot, 'both', true)
    expect(actions).toEqual([{ type: 'delete_remote', relpath: 'a.txt' }])
    const { actions: actions2 } = plan(new Map([['a.txt', local('a.txt')]]), new Map(), snapshot, 'both', true)
    expect(actions2).toEqual([{ type: 'delete_local', relpath: 'a.txt' }])
  })

  it('cancels all deletions and warns when over the delete threshold', () => {
    const snapshot = new Map([['a.txt', snap()], ['b.txt', snap()], ['c.txt', snap()]])
    const remoteMap = new Map([['a.txt', remote('a.txt')], ['b.txt', remote('b.txt')], ['c.txt', remote('c.txt')]])
    const { actions, warnings } = plan(new Map(), remoteMap, snapshot, 'both', true, 2)
    expect(actions).toEqual([])
    expect(warnings).toHaveLength(1)
    expect(warnings[0]).toContain('超过阈值')
  })

  it('upload mode mirrors local: re-uploads when remote copy was deleted', () => {
    const snapshot = new Map([['a.txt', snap()]])
    const { actions } = plan(new Map([['a.txt', local('a.txt')]]), new Map(), snapshot, 'upload')
    expect(actions).toEqual([{ type: 'upload', relpath: 'a.txt', size: 100 }])
  })

  it('download mode mirrors remote: re-downloads when local copy was deleted', () => {
    const snapshot = new Map([['a.txt', snap()]])
    const { actions } = plan(new Map(), new Map([['a.txt', remote('a.txt')]]), snapshot, 'download')
    expect(actions).toEqual([{ type: 'download', relpath: 'a.txt', size: 100 }])
  })

  it('does not resurrect a tombstoned file while the other side stays unchanged', () => {
    const tombstoned: SyncFileEntry = { ...snap(), tombstone: 'local_deleted' }
    const { actions } = plan(new Map(), new Map([['a.txt', remote('a.txt')]]), new Map([['a.txt', tombstoned]]), 'both')
    expect(actions).toEqual([])
  })

  it('uploads again when a remote-tombstoned file changes locally', () => {
    const tombstoned: SyncFileEntry = { ...snap(), tombstone: 'remote_deleted' }
    const { actions } = plan(new Map([['a.txt', local('a.txt', 300, 9000)]]), new Map(), new Map([['a.txt', tombstoned]]), 'both')
    expect(actions).toEqual([{ type: 'upload', relpath: 'a.txt', size: 300 }])
  })

  it('ignores remote changes in upload-only mode and local changes in download-only mode', () => {
    const snapshot = new Map([['a.txt', snap()], ['b.txt', snap()]])
    const uploadPlan = plan(new Map([['a.txt', local('a.txt')], ['b.txt', local('b.txt', 300, 5000)]]), new Map([['a.txt', remote('a.txt', 999)], ['b.txt', remote('b.txt')]]), snapshot, 'upload')
    expect(uploadPlan.actions).toEqual([{ type: 'upload', relpath: 'b.txt', size: 300 }])
    const downloadPlan = plan(new Map([['a.txt', local('a.txt')], ['b.txt', local('b.txt', 300, 5000)]]), new Map([['a.txt', remote('a.txt', 999)], ['b.txt', remote('b.txt')]]), snapshot, 'download')
    expect(downloadPlan.actions).toEqual([{ type: 'download', relpath: 'a.txt', size: 999 }])
  })
})
