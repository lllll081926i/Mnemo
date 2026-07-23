import { describe, expect, it } from 'vitest'
import { buildUpstreamProxyHeaders } from '../proxyHeaders'
import { shouldRefreshProxyUrl } from '../proxyCache'
import { GetExpiresTime } from '../utils'

describe('buildUpstreamProxyHeaders', () => {
  it('keeps range and media auth headers while dropping conditional and hop-by-hop headers', () => {
    const headers = buildUpstreamProxyHeaders({
      host: '127.0.0.1:4961',
      connection: 'keep-alive',
      range: 'bytes=32768-33051',
      'if-none-match': '"b968913fc5e95732a0646ac5c32db3db"',
      'accept-encoding': 'gzip, deflate, br',
      referer: 'https://www.aliyundrive.com/',
      authorization: 'Bearer local-token',
      'user-agent': 'Mozilla/5.0'
    }, JSON.stringify({
      'X-Provider-Authorization': 'Bearer provider-token',
      'X-Provider-Token': 'provider-token'
    }))

    expect(headers.range).toBe('bytes=32768-33051')
    expect(headers['x-provider-authorization']).toBe('Bearer provider-token')
    expect(headers['x-provider-token']).toBe('provider-token')
    expect(headers['accept-encoding']).toBe('identity')
    expect(headers.host).toBeUndefined()
    expect(headers.connection).toBeUndefined()
    expect(headers['if-none-match']).toBeUndefined()
    expect(headers.referer).toBeUndefined()
    expect(headers.authorization).toBeUndefined()
  })
})

describe('shouldRefreshProxyUrl', () => {
  it('does not reuse a cached URL from another drive when file ids collide', () => {
    expect(shouldRefreshProxyUrl({
      driveId: 'gdrive:account-a',
      fileId: 'same-file-id',
      proxyUrl: 'https://cached.example/file',
      selectQuality: 'Origin',
      proxyInfo: {
        drive_id: 'onedrive:account-b',
        file_id: 'same-file-id',
        expires_time: Date.now() + 60_000,
        videoQuality: 'Origin'
      }
  })).toBe(true)
  })

  it('does not refresh an unknown-expiry URL on every range request', () => {
    expect(shouldRefreshProxyUrl({
      driveId: 'pikpak',
      fileId: 'long-video',
      proxyUrl: 'https://cdn.example/long-video',
      selectQuality: 'HD',
      proxyInfo: {
        drive_id: 'pikpak',
        file_id: 'long-video',
        expires_time: 0,
        videoQuality: 'HD'
      }
    })).toBe(false)
  })

  it('reads PikPak expire query parameters as milliseconds', () => {
    expect(GetExpiresTime('https://cdn.example/video.mp4?expire=1999999999')).toBe(1999999999000)
  })

  it('returns zero for malformed percent-encoded URLs instead of throwing', () => {
    expect(() => GetExpiresTime('https://cdn.example/video.mp4?token=%E0%A4%A')).not.toThrow()
    expect(GetExpiresTime('https://cdn.example/video.mp4?token=%E0%A4%A')).toBe(0)
  })
})
