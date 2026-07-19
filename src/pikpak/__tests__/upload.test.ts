import { describe, expect, it } from 'vitest'
import { buildPikPakUploadBody, normalizePikPakOssEndpoint, toPikPakOssCredentials } from '../uploadProtocol'

describe('PikPak upload protocol', () => {
  it('builds resumable upload requests without a root parent id', () => {
    expect(buildPikPakUploadBody('pikpak_root', 'demo.bin', 12, 'ABC')).toEqual({
      kind: 'drive#file',
      name: 'demo.bin',
      size: 12,
      hash: 'ABC',
      upload_type: 'UPLOAD_TYPE_RESUMABLE',
      objProvider: { provider: 'UPLOAD_TYPE_UNKNOWN' },
      parent_id: undefined,
      folder_type: 'NORMAL'
    })
  })

  it('maps PikPak temporary credentials to the shared OSS uploader', () => {
    expect(
      toPikPakOssCredentials({
        endpoint: 'oss.example.com',
        bucket: 'bucket',
        key: 'folder/file.bin',
        access_key_id: 'id',
        access_key_secret: 'secret',
        security_token: 'token'
      })
    ).toEqual({
      endpoint: 'oss.example.com',
      bucket: 'bucket',
      objectPath: 'folder/file.bin',
      accessKeyId: 'id',
      accessKeySecret: 'secret',
      securityToken: 'token'
    })
  })

  it('normalizes Android upload endpoints to the PikPak OSS root domain', () => {
    expect(normalizePikPakOssEndpoint('https://vip-lixian-07.mypikpak.net/')).toBe('mypikpak.net')
    expect(normalizePikPakOssEndpoint('oss.example.com')).toBe('oss.example.com')
  })
})
