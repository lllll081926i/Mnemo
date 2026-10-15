import { afterEach, describe, expect, it, vi } from 'vitest'
import { createPreviewLease, createPreviewLeasePool, releasePreviewURL } from './preview-lease'

const address = name => `http://127.0.0.1:45678/stream/${name}?t=local-token`
afterEach(() => vi.unstubAllGlobals())

describe('播放会话所有权', () => {
  it('图片缓存淘汰释放无引用会话，关闭后迟到地址立即释放', () => {
    const release = vi.fn()
    const pool = createPreviewLeasePool(release)
    const old = pool.acquire('old')
    old.add(address('old'))
    pool.acquire('visible').add(address('visible'))
    pool.retain(['visible'])
    expect(release.mock.calls.flat()).toEqual([address('old')])
    pool.dispose()
    old.add(address('late-old'))
    pool.acquire('late').add(address('late'))
    expect(release.mock.calls.flat()).toEqual([address('old'), address('visible'), address('late-old'), address('late')])
  })
  it('切换时释放旧主源与字幕，保留仍被使用的会话', () => {
    const release = vi.fn()
    const lease = createPreviewLease(release)
    lease.replace({ url: address('first'), subtitles: [{ url: address('shared') }, { url: address('old-subtitle') }] })
    lease.replace({ url: address('second'), subtitles: [{ url: address('shared') }] })
    expect(release.mock.calls.flat()).toEqual([address('first'), address('old-subtitle')])
    lease.dispose()
    expect(release.mock.calls.flat()).toEqual([address('first'), address('old-subtitle'), address('second'), address('shared')])
    lease.dispose()
    expect(release).toHaveBeenCalledTimes(4)
  })

  it('丢弃乱序结果不撤销当前会话，关闭后的迟到结果立即释放', () => {
    const release = vi.fn()
    const lease = createPreviewLease(release)
    lease.replace({ url: address('current') })
    lease.discard({ url: address('current'), subtitles: [{ url: address('stale') }] })
    expect(release.mock.calls.flat()).toEqual([address('stale')])
    lease.dispose()
    lease.replace({ url: address('late') })
    lease.add(address('late-subtitle'))
    expect(release.mock.calls.flat()).toEqual([address('stale'), address('current'), address('late'), address('late-subtitle')])
  })

  it('同名字幕去重，并将源 URL 限制为带令牌的本机根会话', () => {
    const release = vi.fn()
    const lease = createPreviewLease(release)
    lease.add(address('subtitle'))
    lease.add(address('subtitle') + '&offset=100#fragment')
    for (const value of ['https://cloud.example/video?token=secret', 'http://localhost:45678/stream/file?t=token', 'http://127.0.0.1:45678/local/file?t=token', 'http://127.0.0.1:45678/stream/file/resource?t=token', 'http://127.0.0.1:45678/stream/file', 'http://user:pass@127.0.0.1:45678/stream/file?t=token']) lease.add(value)
    lease.clear()
    expect(release.mock.calls.flat()).toEqual([address('subtitle')])
  })

  it('释放失败不阻塞关闭，也不产生未处理拒绝', async () => {
    const fetch = vi.fn().mockRejectedValue(new Error('服务已关闭'))
    vi.stubGlobal('fetch', fetch)
    await expect(releasePreviewURL(address('video') + '&seek=10')).resolves.toBeUndefined()
    expect(fetch).toHaveBeenCalledWith(address('video'), { method: 'DELETE', keepalive: true, credentials: 'omit', cache: 'no-store' })
    await releasePreviewURL('https://example.com/stream/source?t=cloud-secret')
    expect(fetch).toHaveBeenCalledTimes(1)
    const lease = createPreviewLease(() => { throw new Error('释放失败') })
    lease.add(address('video'))
    expect(() => lease.dispose()).not.toThrow()
  })
})
