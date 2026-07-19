import { describe, expect, it } from 'vitest'
import { buildUpstreamProxyHeaders } from '../proxyHeaders'

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
