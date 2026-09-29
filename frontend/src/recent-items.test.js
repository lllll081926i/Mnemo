import { beforeEach, describe, expect, it, vi } from 'vitest'
import { clearRecentItems, recentEnabled, recentItems, recordRecentItem, setRecentEnabled } from './recent-items'

const values = new Map()
beforeEach(() => {
  values.clear()
  vi.stubGlobal('localStorage', {
    getItem: key => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, value),
    removeItem: key => values.delete(key),
  })
  clearRecentItems()
  setRecentEnabled(true)
})

describe('最近使用', () => {
  it('只保存导航字段，同一账号和空间内去重', () => {
    const file = { userId: 'user', driveId: 'drive', fileId: 'file', dirId: 'root', name: '文档', isDir: false, path: [], download_url: 'secret', token: 'secret' }
    recordRecentItem(file)
    recordRecentItem({ ...file, name: '新名称' })
    expect(recentItems.value).toHaveLength(1)
    expect(recentItems.value[0].name).toBe('新名称')
    expect(values.get('mnemo.recentItems.v1')).not.toContain('secret')
    recordRecentItem({ ...file, driveId: 'other' })
    expect(recentItems.value).toHaveLength(2)
  })

  it('暂停时不新增记录，清空会删除本地数据', () => {
    setRecentEnabled(false)
    recordRecentItem({ userId: 'user', driveId: 'drive', fileId: 'file', name: '文件' })
    expect(recentItems.value).toHaveLength(0)
    setRecentEnabled(true)
    recordRecentItem({ userId: 'user', driveId: 'drive', fileId: 'file', name: '文件' })
    clearRecentItems()
    expect(recentEnabled.value).toBe(true)
    expect(recentItems.value).toHaveLength(0)
    expect(values.has('mnemo.recentItems.v1')).toBe(false)
  })
})
