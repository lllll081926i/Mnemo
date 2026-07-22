import type { FileHandle } from 'fs/promises'
import { describe, expect, it, vi } from 'vitest'
import { buildPikPakS3ClientConfig, type OssUploadCredentials } from '../ossUpload'
import { readProviderUploadSlice } from '../providerUpload'

const credentials: OssUploadCredentials = {
  endpoint: 'oss.example.com',
  bucket: 'b',
  objectPath: 'folder/a.txt',
  accessKeyId: 'id',
  accessKeySecret: 'secret',
  securityToken: 'token'
}

describe('PikPak S3 upload protocol', () => {
  it('uses the SigV4 endpoint and region from the bundled rclone backend', () => {
    expect(buildPikPakS3ClientConfig(credentials)).toEqual({
      endpoint: 'https://mypikpak.com',
      region: 'pikpak',
      credentials: { accessKeyId: 'id', secretAccessKey: 'secret', sessionToken: 'token' }
    })
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
