import { describe, expect, it } from 'vitest'
import { existsSync } from 'node:fs'
import { resolve } from 'node:path'
import { buildDriveProviderDriveId, buildDriveProviderUserId, getDriveProviderAccountId, getDriveProviderCapabilities, getDriveProviderDriveAccountId, getDriveProviderMeta, getDriveProviderSidebarEntries, getDriveSidebarIcon, isDriveProviderRootId, isDriveProviderSessionUsable, isDriveProviderSidebarEntryAvailable, resolveDriveProvider } from '../driveProvider'

const retainedProviders = ['pikpak', 'onedrive', 'dropbox', 'gdrive', 'gofile', 'webdav', 's3'] as const

describe('drive provider capabilities', () => {
  it('resolves every retained provider', () => {
    expect(resolveDriveProvider('s3')).toBe('s3')
    expect(resolveDriveProvider({ userId: 'webdav:connection-id' })).toBe('webdav')
    expect(resolveDriveProvider({ userId: 'pikpak_account-id' })).toBe('pikpak')
    expect(resolveDriveProvider({ userId: 'onedrive_account-id' })).toBe('onedrive')
    expect(resolveDriveProvider({ userId: 'dropbox_account-id' })).toBe('dropbox')
    expect(resolveDriveProvider({ userId: 'gdrive_account-id' })).toBe('gdrive')
    expect(resolveDriveProvider({ userId: 'unsupported:connection-id' })).toBe('unknown')
    expect(resolveDriveProvider({ userId: 'gofile_account-id' })).toBe('gofile')
    expect(resolveDriveProvider({ driveId: 'onedrive:drive-id' })).toBe('onedrive')
    for (const userId of ['aliyun_user-id', 'quark_account-id', 'cloud139_account-id', 'cloud189_account-id', 'guangya_account-id']) {
      expect(resolveDriveProvider({ userId }), userId).toBe('unknown')
    }
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
    expect(buildDriveProviderUserId('pikpak', 'remote-id')).toBe('pikpak_remote-id')
    expect(getDriveProviderAccountId('webdav:connection-a', 'webdav')).toBe('connection-a')
  })

  it('recognizes only exact provider root ids', () => {
    expect(isDriveProviderRootId({ driveId: 'webdav:connection-a' }, '/')).toBe(true)
    expect(isDriveProviderRootId({ driveId: 's3:connection-a' }, 'root')).toBe(true)
    expect(isDriveProviderRootId('pikpak', 'pikpak_root')).toBe(true)
    expect(isDriveProviderRootId('gdrive', 'gdrive_root')).toBe(true)
    expect(isDriveProviderRootId({ driveId: 'webdav:connection-a' }, '/folder/rooted')).toBe(false)
    expect(isDriveProviderRootId('pikpak', 'folder_rooted')).toBe(false)
  })

  it('accepts mounted-storage sessions without OAuth access tokens', () => {
    expect(isDriveProviderSessionUsable({ tokenfrom: 'webdav', access_token: '' }, { driveId: 'webdav:connection-a' })).toBe(true)
    expect(isDriveProviderSessionUsable({ tokenfrom: 's3', access_token: '' }, { driveId: 's3:connection-a' })).toBe(true)
    expect(isDriveProviderSessionUsable({ tokenfrom: 'gdrive', access_token: '' }, { driveId: 'gdrive:account-a' })).toBe(false)
    expect(isDriveProviderSessionUsable({ tokenfrom: 'gdrive', access_token: 'token' }, { driveId: 'gdrive:account-a' })).toBe(true)
  })

  it('does not expose removed-provider operations', () => {
    for (const provider of retainedProviders) {
      expect(getDriveProviderCapabilities(provider).encryption, provider).toBe(false)
    }
    for (const provider of ['aliyun', 'quark', '139', '189', 'guangya'] as const) {
      const capabilities = getDriveProviderCapabilities(provider)
      expect(capabilities.provider, provider).toBe('unknown')
      expect(capabilities.favorite, provider).toBe(false)
      expect(capabilities.encryption, provider).toBe(false)
    }
  })

  it('exposes only implemented search and share paths', () => {
    for (const provider of ['onedrive', 'dropbox', 'gdrive'] as const) expect(getDriveProviderCapabilities(provider).search, provider).toBe(true)
    for (const provider of ['pikpak', 'gofile', 'webdav', 's3'] as const) expect(getDriveProviderCapabilities(provider).search, provider).toBe(false)
    for (const provider of ['pikpak', 'onedrive', 'dropbox', 'gdrive', 'gofile'] as const) expect(getDriveProviderCapabilities(provider).createShare, provider).toBe(true)
    for (const provider of ['webdav', 's3'] as const) expect(getDriveProviderCapabilities(provider).createShare, provider).toBe(false)
    expect(getDriveProviderCapabilities('pikpak').offlineDownload).toBe(true)
    for (const provider of ['onedrive', 'dropbox', 'gdrive', 'gofile', 'webdav', 's3'] as const) expect(getDriveProviderCapabilities(provider).offlineDownload, provider).toBe(false)
  })

  it('enables local tags for every file provider so the tag filter tree is visible', () => {
    // 本地标签存在本地 localStorage，不依赖网盘服务端；全部文件网盘都应显示“标记”筛选树
    for (const provider of retainedProviders) {
      expect(getDriveProviderCapabilities(provider).localTag, provider).toBe(true)
      expect(getDriveProviderCapabilities(provider).colorTag, provider).toBe(false)
    }
    expect(getDriveProviderCapabilities('aliyun').localTag).toBe(false)
  })

  it('builds only retained provider sidebars', () => {
    expect(getDriveProviderSidebarEntries('pikpak').map((entry) => entry.key)).toEqual(['trash'])
    expect(getDriveProviderSidebarEntries('onedrive').map((entry) => entry.key)).toEqual(['search'])
    expect(getDriveProviderSidebarEntries('gdrive').map((entry) => entry.key)).toEqual(['trash', 'search'])
    expect(getDriveProviderSidebarEntries('aliyun')).toEqual([])
    expect(isDriveProviderSidebarEntryAvailable('pikpak', 'search')).toBe(false)
    expect(isDriveProviderSidebarEntryAvailable('pikpak', 'trash')).toBe(true)
    expect(getDriveSidebarIcon('trash')).toBe('icontrash')
  })

  it('keeps upload and destructive operations provider-specific', () => {
    for (const provider of ['pikpak', 'onedrive', 'dropbox', 'gdrive', 'gofile'] as const) {
      expect(getDriveProviderCapabilities(provider).uploadMode, provider).toBe('queue')
    }
    for (const provider of ['webdav', 's3'] as const) {
      const capabilities = getDriveProviderCapabilities(provider)
      expect(capabilities.uploadMode).toBe('direct')
      expect(capabilities.mountedStorage).toBe(true)
      expect(capabilities.recycleBin).toBe(false)
      expect(capabilities.permanentDelete).toBe(true)
    }
    expect(getDriveProviderCapabilities('gdrive').trashRestore).toBe(true)
    expect(getDriveProviderCapabilities('gdrive').trashPurge).toBe(true)
    expect(getDriveProviderCapabilities('gdrive').trashClear).toBe(true)
  })

  it('defaults removed and unknown providers to no capabilities', () => {
    for (const id of ['aliyun_user', 'quark_user', 'cloud139_user', 'cloud189_user', 'guangya_user', 'cloud123', '115', 'baidu', 'box', 'unrecognized-account']) {
      const capabilities = getDriveProviderCapabilities({ userId: id })
      expect(capabilities.provider, id).toBe('unknown')
      expect(capabilities.download, id).toBe(false)
      expect(capabilities.rename, id).toBe(false)
    }
  })
})
