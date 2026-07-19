import { describe, expect, it } from 'vitest'
import { getDriveProviderCapabilities, resolveDriveProvider } from '../driveProvider'

describe('drive provider capabilities', () => {
  it('resolves providers from tokens, account ids and drive ids', () => {
    expect(resolveDriveProvider('s3')).toBe('s3')
    expect(resolveDriveProvider({ userId: 'webdav:connection-id' })).toBe('webdav')
    expect(resolveDriveProvider({ userId: 'cloud123_user-id' })).toBe('cloud123')
    expect(resolveDriveProvider({ driveId: 'drive115' })).toBe('115')
    expect(resolveDriveProvider({ userId: 'aliyun_user-id', driveId: 'resource-drive-id' })).toBe('aliyun')
  })

  it('keeps Aliyun-only operations isolated', () => {
    const aliyun = getDriveProviderCapabilities('aliyun')
    expect(aliyun.encryption).toBe(true)
    expect(aliyun.favorite).toBe(true)
    expect(aliyun.colorTag).toBe(true)
    expect(aliyun.playbackHistory).toBe(true)
    expect(aliyun.copyTree).toBe(true)

    const cloud123 = getDriveProviderCapabilities('cloud123')
    expect(cloud123.encryption).toBe(false)
    expect(cloud123.favorite).toBe(false)
    expect(cloud123.colorTag).toBe(false)
    expect(cloud123.copyTree).toBe(false)
  })

  it('exposes sharing only for providers with implemented share creation', () => {
    for (const provider of ['aliyun', 'cloud123', 'pikpak', 'quark', 'guangya', 'dropbox', 'onedrive', 'box']) {
      expect(getDriveProviderCapabilities(provider).createShare, provider).toBe(true)
    }
    for (const provider of ['115', '139', '189', 'baidu', 'webdav', 's3']) {
      expect(getDriveProviderCapabilities(provider).createShare, provider).toBe(false)
    }
  })

  it('keeps mounted storage destructive operations explicit', () => {
    for (const provider of ['webdav', 's3']) {
      const capabilities = getDriveProviderCapabilities(provider)
      expect(capabilities.mountedStorage).toBe(true)
      expect(capabilities.recycleBin).toBe(false)
      expect(capabilities.permanentDelete).toBe(true)
      expect(capabilities.createTextFile).toBe(false)
      expect(capabilities.encryption).toBe(false)
      expect(capabilities.createShare).toBe(false)
      expect(capabilities.trashView).toBe(false)
    }
  })

  it('distinguishes recycle actions from an in-app trash view', () => {
    const aliyun = getDriveProviderCapabilities('aliyun')
    expect(aliyun.trashView).toBe(true)
    expect(aliyun.trashRestore).toBe(true)
    expect(aliyun.trashPurge).toBe(true)
    expect(aliyun.trashClear).toBe(true)

    const cloud123 = getDriveProviderCapabilities('cloud123')
    expect(cloud123.trashView).toBe(true)
    expect(cloud123.trashRestore).toBe(true)
    expect(cloud123.trashPurge).toBe(false)
    expect(cloud123.trashClear).toBe(false)

    for (const provider of ['115', 'pikpak']) {
      const capabilities = getDriveProviderCapabilities(provider)
      expect(capabilities.trashView).toBe(true)
      expect(capabilities.trashRestore).toBe(true)
      expect(capabilities.trashPurge).toBe(true)
      expect(capabilities.trashClear).toBe(false)
    }

    for (const provider of ['quark', '139', '189', 'guangya', 'baidu', 'dropbox', 'onedrive', 'box']) {
      expect(getDriveProviderCapabilities(provider).trashView, provider).toBe(false)
    }
  })

  it('hides copy for providers whose command layer rejects it', () => {
    expect(getDriveProviderCapabilities('quark').move).toBe(true)
    expect(getDriveProviderCapabilities('quark').copy).toBe(false)
  })

  it('does not expose text-file creation through providers without a buffer upload implementation', () => {
    expect(getDriveProviderCapabilities('dropbox').createTextFile).toBe(true)
    expect(getDriveProviderCapabilities('onedrive').createTextFile).toBe(true)
    expect(getDriveProviderCapabilities('box').createTextFile).toBe(true)
    expect(getDriveProviderCapabilities('guangya').createTextFile).toBe(true)
    expect(getDriveProviderCapabilities('cloud123').createTextFile).toBe(false)
    expect(getDriveProviderCapabilities('pikpak').createTextFile).toBe(false)
    expect(getDriveProviderCapabilities('quark').createTextFile).toBe(false)
  })

  it('defaults unknown providers to no capabilities', () => {
    const capabilities = getDriveProviderCapabilities({ userId: 'unrecognized-account' })
    expect(capabilities.provider).toBe('unknown')
    expect(capabilities.download).toBe(false)
    expect(capabilities.rename).toBe(false)
    expect(capabilities.createShare).toBe(false)
  })
})
