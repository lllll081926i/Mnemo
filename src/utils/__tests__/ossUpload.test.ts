import type { FileHandle } from 'fs/promises'
import { describe, expect, it, vi } from 'vitest'
import { buildOssAuthorization, buildOssCanonicalResource, buildOssObjectUrl, type OssUploadCredentials } from '../ossUpload'
import { readProviderUploadSlice } from '../providerUpload'

const credentials: OssUploadCredentials = {
  endpoint: 'oss.example.com',
  bucket: 'b',
  objectPath: 'folder/a.txt',
  accessKeyId: 'id',
  accessKeySecret: 'secret',
  securityToken: 'token'
}

describe('shared OSS upload protocol', () => {
  it('builds bucket-hosted URLs and canonical resources', () => {
    expect(buildOssCanonicalResource(credentials, 'uploads')).toBe('/b/folder/a.txt?uploads')
    expect(buildOssObjectUrl(credentials, 'uploads')).toBe('https://b.oss.example.com/folder/a.txt?uploads')
  })

  it('produces a stable OSS v1 signature', () => {
    const simple = { ...credentials, objectPath: 'a.txt' }
    expect(buildOssAuthorization('PUT', simple, 'Mon, 01 Jan 2024 00:00:00 GMT', 'application/octet-stream')).toBe('OSS id:Ncmb4iP+/emsuhju/1tNpLjUF1k=')
  })

  it('fills a requested upload slice across partial filesystem reads', async () => {
    const source = Buffer.from('abcdef')
    const read = vi.fn(async (buffer: Buffer, offset: number, length: number, position: number) => {
      const bytesRead = Math.min(2, length, source.length - position)
      if (bytesRead > 0) source.copy(buffer, offset, position, position + bytesRead)
      return { bytesRead, buffer }
    })
    const handle = { read } as unknown as FileHandle
    expect((await readProviderUploadSlice(handle, 1, 4)).toString()).toBe('bcde')
    expect(read).toHaveBeenCalledTimes(2)
  })
})
