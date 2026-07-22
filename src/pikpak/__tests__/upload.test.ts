import { describe, expect, it } from 'vitest'
import { createPikPakDeviceId, getPikPakAccountId } from '../auth'
import { buildPikPakUploadBody, normalizePikPakOssEndpoint, toPikPakOssCredentials } from '../uploadProtocol'
import { waitForPikPakTask } from '../filecmd'

describe('PikPak upload protocol', () => {
  it('builds resumable upload requests without a root parent id', () => {
    expect(buildPikPakUploadBody('pikpak_root', 'demo.bin', 12, 'ABC')).toEqual({
      kind: 'drive#file',
      name: 'demo.bin',
      size: 12,
      hash: 'ABC',
      upload_type: 'UPLOAD_TYPE_RESUMABLE',
      resumable: { provider: 'PROVIDER_ALIYUN' },
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
    expect(normalizePikPakOssEndpoint('https://vip-lixian-07.mypikpak.net/')).toBe('mypikpak.com')
    expect(normalizePikPakOssEndpoint('oss.example.com')).toBe('oss.example.com')
  })

  it('creates valid random device identities', () => {
    const first = createPikPakDeviceId()
    const second = createPikPakDeviceId()
    expect(first).toMatch(/^[a-f0-9]{32}$/)
    expect(second).not.toBe(first)
  })

  it('uses the PikPak token subject so multiple accounts do not overwrite each other', () => {
    const payload = btoa(JSON.stringify({ sub: 'remote-account-id' })).replace(/=/g, '')
    expect(getPikPakAccountId(`header.${payload}.signature`, 'fallback@example.com')).toBe('remote-account-id')
    expect(getPikPakAccountId('not-a-jwt', 'Fallback@Example.com')).toBe('fallback@example.com')
  })

  it('waits for asynchronous file commands to complete', async () => {
    const phases = ['PHASE_TYPE_PENDING', 'PHASE_TYPE_RUNNING', 'PHASE_TYPE_COMPLETE']
    await expect(waitForPikPakTask(async () => ({ phase: phases.shift() }), 3, 0)).resolves.toEqual({ success: true, error: '' })
  })

  it('does not report failed or timed-out file commands as successful', async () => {
    await expect(waitForPikPakTask(async () => ({ phase: 'PHASE_TYPE_ERROR', message: 'denied' }), 1, 0)).resolves.toEqual({ success: false, error: 'denied' })
    await expect(waitForPikPakTask(async () => ({ phase: 'PHASE_TYPE_RUNNING' }), 2, 0)).resolves.toEqual({ success: false, error: 'PikPak 操作等待超时' })
  })
})
