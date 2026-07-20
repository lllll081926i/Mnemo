import { describe, expect, it } from 'vitest'
import { existsSync } from 'node:fs'
import { resolve } from 'node:path'
import { buildDriveProviderDriveId, buildDriveProviderUserId, getDriveProviderAccountId, getDriveProviderCapabilities, getDriveProviderDriveAccountId, getDriveProviderMeta, getDriveProviderSidebarEntries, isDriveProviderSidebarEntryAvailable, resolveDriveProvider } from '../driveProvider'

const retainedProviders = ['aliyun', 'pikpak', 'quark', '139', '189', 'guangya', 'onedrive', 'dropbox', 'gdrive', 'gofile', 'webdav', 's3'] as const

describe('drive provider capabilities', () => {
  it('resolves every retained provider', () => {
    expect(resolveDriveProvider('s3')).toBe('s3')
    expect(resolveDriveProvider({ userId: 'webdav:connection-id' })).toBe('webdav')
    expect(resolveDriveProvider({ userId: 'pikpak_account-id' })).toBe('pikpak')
    expect(resolveDriveProvider({ userId: 'quark_account-id' })).toBe('quark')
    expect(resolveDriveProvider({ userId: 'cloud139_account-id' })).toBe('139')
    expect(resolveDriveProvider({ userId: 'cloud189_account-id' })).toBe('189')
    expect(resolveDriveProvider({ userId: 'guangya_account-id' })).toBe('guangya')
    expect(resolveDriveProvider({ userId: 'onedrive_account-id' })).toBe('onedrive')
    expect(resolveDriveProvider({ userId: 'dropbox_account-id' })).toBe('dropbox')
    expect(resolveDriveProvider({ userId: 'gdrive_account-id' })).toBe('gdrive')
    expect(resolveDriveProvider({ userId: 'nextcloud:connection-id' })).toBe('unknown')
    expect(resolveDriveProvider({ userId: 'gofile_account-id' })).toBe('gofile')
    expect(resolveDriveProvider({ driveId: 'onedrive:drive-id' })).toBe('onedrive')
    expect(resolveDriveProvider({ userId: 'aliyun_user-id', driveId: 'resource-drive-id' })).toBe('aliyun')
  })

  it('ships a real login icon for every retained provider', () => {
    for (const provider of retainedProviders) {
      const icon = getDriveProviderMeta(provider).icon
      expect(icon, provider).not.toBe('')
      expect(existsSync(resolve('public', icon)), `${provider}: ${icon}`).toBe(true)
    }
  })

  it('builds account-scoped drive ids for concurrent provider accounts', () => {
    expect(buildDriveProviderDriveId('onedrive', 'account-a')).toBe('onedrive:account-a')
    expect(buildDriveProviderDriveId('onedrive', 'account-b')).toBe('onedrive:account-b')
    expect(buildDriveProviderDriveId('dropbox', 'account-a')).toBe('dropbox:account-a')
    expect(buildDriveProviderDriveId('webdav', 'connection-a')).toBe('webdav:connection-a')
    expect(getDriveProviderDriveAccountId('gdrive:account-a')).toBe('account-a')
    expect(resolveDriveProvider({ driveId: buildDriveProviderDriveId('gofile', 'account-a') })).toBe('gofile')
  })

  it('keeps stable mounted-storage account ids', () => {
    expect(buildDriveProviderUserId('webdav', 'connection-a')).toBe('webdav:connection-a')
    expect(buildDriveProviderUserId('s3', 'connection-b')).toBe('s3:connection-b')
    expect(buildDriveProviderUserId('aliyun', 'aliyun-remote-id')).toBe('aliyun-remote-id')
    expect(getDriveProviderAccountId('webdav:connection-a', 'webdav')).toBe('connection-a')
  })

  it('keeps Aliyun-only operations isolated', () => {
    const aliyun = getDriveProviderCapabilities('aliyun')
    expect(aliyun.encryption).toBe(true)
    expect(aliyun.favorite).toBe(true)
    expect(aliyun.colorTag).toBe(true)
    expect(aliyun.playbackHistory).toBe(true)
    for (const provider of retainedProviders.filter((item) => item !== 'aliyun')) {
      expect(getDriveProviderCapabilities(provider).encryption, provider).toBe(false)
    }
  })

  it('exposes only implemented search and share paths', () => {
    for (const provider of ['aliyun', 'quark', 'guangya', 'onedrive', 'dropbox', 'gdrive'] as const) expect(getDriveProviderCapabilities(provider).search, provider).toBe(true)
    for (const provider of ['139', '189', 'pikpak', 'nextcloud', 'gofile', 'webdav', 's3'] as const) expect(getDriveProviderCapabilities(provider).search, provider).toBe(false)
    for (const provider of ['aliyun', 'pikpak', 'quark', 'guangya', 'onedrive', 'dropbox', 'gdrive', 'gofile'] as const) expect(getDriveProviderCapabilities(provider).createShare, provider).toBe(true)
    for (const provider of ['139', '189', 'nextcloud', 'webdav', 's3'] as const) expect(getDriveProviderCapabilities(provider).createShare, provider).toBe(false)
  })

  it('builds flat Aliyun spaces and retained provider sidebars', () => {
    const merged = getDriveProviderSidebarEntries('aliyun', {
      resource_drive_id: 'merged-drive',
      backup_drive_id: 'merged-drive',
      default_sbox_drive_id: 'safe-drive',
      pic_drive_id: 'album-drive'
    })
    expect(merged.filter((entry) => entry.driveId === 'merged-drive')).toHaveLength(1)
    expect(merged.map((entry) => entry.key)).toEqual(['favorite', 'trash', 'recover', 'search', 'pic_root', 'resource_root', 'safe_root'])
    expect(getDriveProviderSidebarEntries('pikpak').map((entry) => entry.key)).toEqual(['trash'])
    expect(getDriveProviderSidebarEntries('quark').map((entry) => entry.key)).toEqual(['search'])
    expect(getDriveProviderSidebarEntries('onedrive').map((entry) => entry.key)).toEqual(['search'])
    expect(getDriveProviderSidebarEntries('gdrive').map((entry) => entry.key)).toEqual(['trash', 'search'])
    expect(getDriveProviderSidebarEntries('139')).toEqual([])
    expect(isDriveProviderSidebarEntryAvailable('pikpak', 'search')).toBe(false)
    expect(isDriveProviderSidebarEntryAvailable('pikpak', 'trash')).toBe(true)
  })

  it('keeps upload and destructive operations provider-specific', () => {
    for (const provider of ['aliyun', '139', '189', 'guangya', 'pikpak', 'quark'] as const) {
      expect(getDriveProviderCapabilities(provider).uploadMode, provider).toBe('queue')
    }
    for (const provider of ['webdav', 's3'] as const) {
      const capabilities = getDriveProviderCapabilities(provider)
      expect(capabilities.uploadMode).toBe('direct')
      expect(capabilities.mountedStorage).toBe(true)
      expect(capabilities.recycleBin).toBe(false)
      expect(capabilities.permanentDelete).toBe(true)
    }
    expect(getDriveProviderCapabilities('quark').copy).toBe(false)
    expect(getDriveProviderCapabilities('gdrive').trashRestore).toBe(true)
    expect(getDriveProviderCapabilities('gdrive').trashPurge).toBe(true)
    expect(getDriveProviderCapabilities('gdrive').trashClear).toBe(false)
    expect(getDriveProviderCapabilities('guangya').createTextFile).toBe(true)
  })

  it('defaults removed and unknown providers to no capabilities', () => {
    for (const id of ['cloud123', '115', 'baidu', 'box', 'nextcloud:connection-id', 'unrecognized-account']) {
      const capabilities = getDriveProviderCapabilities({ userId: id })
      expect(capabilities.provider, id).toBe('unknown')
      expect(capabilities.download, id).toBe(false)
      expect(capabilities.rename, id).toBe(false)
    }
  })
})
