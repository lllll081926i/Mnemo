import { describe, expect, it } from 'vitest'
import { encodeCloud189FileName, encodeCloud189UploadParams, encryptCloud189UploadParams, getCloud189UploadPartSize, parseCloud189UploadHeaders } from '../uploadProtocol'

describe('189 upload protocol', () => {
  it('sorts parameters before AES encryption', () => {
    expect(encodeCloud189UploadParams({ z: '2', a: '1' })).toBe('a=1&z=2')
    const encrypted = encryptCloud189UploadParams({ z: '2', a: '1' }, '1234567890abcdef-secret')
    expect(encrypted).toMatch(/^[0-9A-F]+$/)
    expect(encrypted).toBe(encryptCloud189UploadParams({ a: '1', z: '2' }, '1234567890abcdef-secret'))
  })

  it('encodes file names, upload headers and large-file part sizes', () => {
    expect(encodeCloud189FileName('a b.txt')).toBe('a+b.txt')
    expect(parseCloud189UploadHeaders('Content-Type%3Dapplication%2Foctet-stream%26x-test%3Da%3Db')).toEqual({
      'Content-Type': 'application/octet-stream',
      'x-test': 'a=b'
    })
    expect(getCloud189UploadPartSize(10 * 1024 * 1024)).toBe(10 * 1024 * 1024)
  })
})
