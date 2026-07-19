import { describe, expect, it } from 'vitest'
import { parseExternalDownloadPayload } from './protocolPayload'

describe('parseExternalDownloadPayload', () => {
  it('rejects magnet links', () => {
    expect(parseExternalDownloadPayload('magnet:?xt=urn:btih:abc')).toBeNull()
  })

  it('rejects torrent URLs', () => {
    expect(parseExternalDownloadPayload('https://example.com/a.torrent')).toBeNull()
  })

  it('accepts http URL', () => {
    expect(parseExternalDownloadPayload('https://example.com/big.zip'))
      .toEqual({ source: 'https://example.com/big.zip', sourceType: 'url' })
  })

  it('rejects local torrent file path', () => {
    expect(parseExternalDownloadPayload('/tmp/a.torrent')).toBeNull()
  })

  it('rejects unsupported schemes', () => {
    expect(parseExternalDownloadPayload('ftp://example.com/a.zip')).toBeNull()
  })

  it('returns null for empty string', () => {
    expect(parseExternalDownloadPayload('')).toBeNull()
  })
})
