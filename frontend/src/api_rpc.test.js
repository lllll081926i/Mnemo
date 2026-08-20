import { describe, expect, it, vi } from 'vitest'

const bridge = vi.hoisted(() => ({
  GetDirectoryCache: vi.fn(),
  SaveDirectoryCache: vi.fn(),
  DeleteDirectoryCache: vi.fn(),
  ClearCache: vi.fn(),
}))

vi.mock('../wailsjs/go/app/App', () => bridge)

import { ClearCache, GetDirectoryCache, SaveDirectoryCache } from './api'

describe('Wails 缓存 RPC 队列', () => {
  it('前一个调用失败后仍按提交顺序继续执行', async () => {
    const calls = []
    bridge.GetDirectoryCache.mockImplementation(async () => {
      calls.push('get')
      throw new Error('读取失败')
    })
    bridge.SaveDirectoryCache.mockImplementation(async () => {
      calls.push('save')
    })
    bridge.ClearCache.mockImplementation(async () => {
      calls.push('clear')
    })

    const first = GetDirectoryCache('account/root')
    const second = SaveDirectoryCache('account/root', [{ name: 'a.txt' }])
    const third = ClearCache()

    await expect(first).rejects.toThrow('读取失败')
    await second
    await third
    expect(calls).toEqual(['get', 'save', 'clear'])
    expect(bridge.SaveDirectoryCache).toHaveBeenCalledWith('account/root', [{ name: 'a.txt' }])
  })
})
