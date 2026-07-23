import type { ITokenInfo } from '../user/userstore'

export type DriveProvider = ITokenInfo['tokenfrom']

export interface DriveProviderMeta {
  key: DriveProvider
  label: string
  icon: string
}

export interface DriveProviderContext {
  tokenfrom?: string
  userId?: string
  driveId?: string
}

export interface DriveProviderCapabilities {
  provider: DriveProvider
  mountedStorage: boolean
  download: boolean
  offlineDownload: boolean
  search: boolean
  upload: boolean
  uploadMode: 'queue' | 'direct' | 'none'
  createFolder: boolean
  createDateFolder: boolean
  createTextFile: boolean
  rename: boolean
  move: boolean
  copy: boolean
  recycleBin: boolean
  permanentDelete: boolean
  trashView: boolean
  trashRestore: boolean
  trashPurge: boolean
  trashClear: boolean
  createShare: boolean
  importShare: boolean
  manageCreatedShares: boolean
  editCreatedShares: boolean
  cancelCreatedShares: boolean
  manageImportedShares: boolean
  shareHistory: boolean
  quickTransfer: boolean
  favorite: boolean
  colorTag: boolean
  /** 本地标签/快捷方式（存在本地 localStorage，不依赖网盘服务端能力，所有文件网盘都可用） */
  localTag: boolean
  encryption: boolean
  playbackHistory: boolean
  copyTree: boolean
  photoAlbum: boolean
}

const driveProviderMap: Partial<Record<DriveProvider, DriveProviderMeta>> = {
  pikpak: {
    key: 'pikpak',
    label: 'PikPak',
    icon: 'images/drive-icons/pikpak.png'
  },
  onedrive: {
    key: 'onedrive',
    label: 'OneDrive',
    icon: 'images/drive-icons/onedrive.svg'
  },
  dropbox: {
    key: 'dropbox',
    label: 'Dropbox',
    icon: 'images/drive-icons/dropbox.svg'
  },
  gdrive: {
    key: 'gdrive',
    label: 'Google Drive',
    icon: 'images/drive-icons/gdrive.svg'
  },
  gofile: {
    key: 'gofile',
    label: 'GoFile',
    icon: 'images/drive-icons/gofile.svg'
  },
  webdav: {
    key: 'webdav',
    label: 'WebDAV',
    icon: 'images/drive-icons/webdav.svg'
  },
  s3: {
    key: 's3',
    label: 'S3',
    icon: 'images/drive-icons/s3.svg'
  },
  unknown: {
    key: 'unknown',
    label: '未知网盘',
    icon: ''
  }
}

const noCapabilities: Omit<DriveProviderCapabilities, 'provider'> = {
  mountedStorage: false,
  download: false,
  offlineDownload: false,
  search: false,
  upload: false,
  uploadMode: 'none',
  createFolder: false,
  createDateFolder: false,
  createTextFile: false,
  rename: false,
  move: false,
  copy: false,
  recycleBin: false,
  permanentDelete: false,
  trashView: false,
  trashRestore: false,
  trashPurge: false,
  trashClear: false,
  createShare: false,
  importShare: false,
  manageCreatedShares: false,
  editCreatedShares: false,
  cancelCreatedShares: false,
  manageImportedShares: false,
  shareHistory: false,
  quickTransfer: false,
  favorite: false,
  colorTag: false,
  localTag: false,
  encryption: false,
  playbackHistory: false,
  copyTree: false,
  photoAlbum: false
}

const standardFileCapabilities: Partial<DriveProviderCapabilities> = {
  download: true,
  upload: true,
  uploadMode: 'queue',
  createFolder: true,
  createDateFolder: true,
  rename: true,
  move: true,
  copy: true,
  recycleBin: true,
  localTag: true
}

const createCapabilities = (provider: DriveProvider, overrides: Partial<DriveProviderCapabilities> = {}): DriveProviderCapabilities => ({ ...noCapabilities, ...standardFileCapabilities, ...overrides, provider })

const driveProviderCapabilities: Partial<Record<DriveProvider, DriveProviderCapabilities>> = {
  pikpak: createCapabilities('pikpak', { offlineDownload: true, createShare: true, trashView: true, trashRestore: true, trashPurge: true, trashClear: true }),
  onedrive: createCapabilities('onedrive', { search: true, createShare: true }),
  dropbox: createCapabilities('dropbox', { search: true, createShare: true }),
  gdrive: createCapabilities('gdrive', { search: true, createShare: true, trashView: true, trashRestore: true, trashPurge: true, trashClear: true }),
  gofile: createCapabilities('gofile', { createShare: true, recycleBin: false, permanentDelete: true }),
  webdav: createCapabilities('webdav', { uploadMode: 'direct', mountedStorage: true, recycleBin: false, permanentDelete: true }),
  s3: createCapabilities('s3', { uploadMode: 'direct', mountedStorage: true, recycleBin: false, permanentDelete: true }),
  unknown: { ...noCapabilities, provider: 'unknown' }
}

const providerAliases: Array<[DriveProvider, string[], string[]]> = [
  ['webdav', ['webdav:'], []],
  ['s3', ['s3:'], []],
  ['pikpak', ['pikpak_'], ['pikpak']],
  ['onedrive', ['onedrive_'], ['onedrive']],
  ['dropbox', ['dropbox_'], ['dropbox']],
  ['gdrive', ['gdrive_'], ['gdrive']],
  ['gofile', ['gofile_'], ['gofile']],
]

const driveProviderUserIdPrefixes: Partial<Record<DriveProvider, string>> = {
  pikpak: 'pikpak_',
  onedrive: 'onedrive_',
  dropbox: 'dropbox_',
  gdrive: 'gdrive_',
  gofile: 'gofile_',
  webdav: 'webdav:',
  s3: 's3:',
  unknown: ''
}

export const resolveDriveProvider = (context: DriveProviderContext | string = {}): DriveProvider => {
  const resolvedContext = typeof context === 'string' ? { tokenfrom: context } : context
  const tokenfrom = resolvedContext.tokenfrom as DriveProvider | undefined
  if (tokenfrom && tokenfrom !== 'unknown' && Object.prototype.hasOwnProperty.call(driveProviderMap, tokenfrom)) return tokenfrom
  const userId = resolvedContext.userId || ''
  const driveId = resolvedContext.driveId || ''
  for (const [provider, userPrefixes] of providerAliases) if (userPrefixes.some((prefix) => userId.startsWith(prefix))) return provider
  for (const [provider, , driveIds] of providerAliases) if (driveIds.some((value) => driveId === value || driveId.startsWith(`${value}:`))) return provider
  return 'unknown'
}

export const isDriveProviderRootId = (context: DriveProviderContext | string, fileId: string): boolean => {
  const value = String(fileId || '')
  if (value === '/' || value === 'root') return true
  const provider = resolveDriveProvider(context)
  return provider !== 'unknown' && value === `${provider}_root`
}

export const isDriveProviderSessionUsable = (token: Partial<ITokenInfo> | null | undefined, context: DriveProviderContext | string = {}): token is ITokenInfo => {
  if (!token) return false
  const resolvedContext = typeof context === 'string' ? { tokenfrom: context } : context
  const provider = resolveDriveProvider({ tokenfrom: token.tokenfrom || resolvedContext.tokenfrom, userId: token.user_id || resolvedContext.userId, driveId: resolvedContext.driveId })
  return getDriveProviderCapabilities(provider).mountedStorage || Boolean(token.access_token)
}

export const buildDriveProviderUserId = (provider: DriveProvider, accountId: string | number): string => {
  const value = String(accountId ?? '').trim()
  if (!value) return ''
  const prefix = driveProviderUserIdPrefixes[provider] || ''
  if (!prefix || value.startsWith(prefix)) return value
  return `${prefix}${value}`
}

export const getDriveProviderAccountId = (userId: string, provider?: DriveProvider): string => {
  const value = String(userId || '')
  if (!value) return ''
  const resolvedProvider = provider || resolveDriveProvider({ userId: value })
  const prefix = driveProviderUserIdPrefixes[resolvedProvider] || ''
  return prefix && value.startsWith(prefix) ? value.slice(prefix.length) : value
}

export const buildDriveProviderDriveId = (provider: DriveProvider, accountId: string | number): string => {
  const value = String(accountId ?? '').trim()
  if (!value) return ''
  if (provider === 'unknown') return value
  const prefix = `${provider}:`
  return value.startsWith(prefix) ? value : `${prefix}${value}`
}

export const getDriveProviderDriveAccountId = (driveId: string, provider?: DriveProvider): string => {
  const value = String(driveId || '').trim()
  if (!value) return ''
  const resolvedProvider = provider || resolveDriveProvider({ driveId: value })
  const prefix = `${resolvedProvider}:`
  return resolvedProvider !== 'unknown' && value.startsWith(prefix) ? value.slice(prefix.length) : value
}

export const getDriveProviderCapabilities = (context: DriveProviderContext | string = {}): DriveProviderCapabilities => driveProviderCapabilities[resolveDriveProvider(context)] || { ...noCapabilities, provider: 'unknown' }

export type DriveSidebarEntryKind = 'space' | 'feature'

export interface DriveSidebarEntry {
  key: string
  title: string
  icon: string
  kind: DriveSidebarEntryKind
  driveId?: string
}

export interface DriveSidebarOptions {
  hideResourceDrive?: boolean
  hideBackupDrive?: boolean
  hideAlbum?: boolean
}

const driveSidebarKeys = new Set(['favorite', 'search', 'trash', 'recover', 'video', 'pic_root', 'backup_root', 'resource_root', 'safe_root'])

export const isDriveSidebarKey = (key: string) => driveSidebarKeys.has(key)

export const getDriveSidebarIcon = (key: string) => {
  if (key === 'favorite') return 'iconcrown'
  if (key === 'search') return 'iconsearch'
  if (key === 'trash') return 'icontrash'
  if (key === 'recover') return 'iconrecover'
  if (key === 'pic_root') return 'iconjietu'
  if (key === 'safe_root') return 'iconsafebox'
  return 'iconfile-folder'
}

export const getDriveProviderSidebarEntries = (context: DriveProviderContext | string, token?: Partial<ITokenInfo>, options: DriveSidebarOptions = {}): DriveSidebarEntry[] => {
  const provider = resolveDriveProvider(context)
  const capabilities = getDriveProviderCapabilities(provider)
  const entries: DriveSidebarEntry[] = []

  if (capabilities.favorite) entries.push({ key: 'favorite', title: '收藏夹', icon: getDriveSidebarIcon('favorite'), kind: 'feature' })
  if (capabilities.trashView) entries.push({ key: 'trash', title: '回收站', icon: getDriveSidebarIcon('trash'), kind: 'feature' })
  if (capabilities.search) entries.push({ key: 'search', title: '全盘搜索', icon: getDriveSidebarIcon('search'), kind: 'feature' })
  return entries
}

export const isDriveProviderSidebarEntryAvailable = (context: DriveProviderContext | string, key: string, token?: Partial<ITokenInfo>, options: DriveSidebarOptions = {}) => {
  if (!isDriveSidebarKey(key)) return true
  return getDriveProviderSidebarEntries(context, token, options).some((entry) => entry.key === key)
}

export const getDriveProviderMeta = (tokenfrom?: string): DriveProviderMeta => {
  return driveProviderMap[(tokenfrom || 'unknown') as DriveProvider] || driveProviderMap.unknown!
}

export const getDriveProviderLabel = (tokenfrom?: string): string => getDriveProviderMeta(tokenfrom).label

export const getDriveProviderIcon = (tokenfrom?: string): string => getDriveProviderMeta(tokenfrom).icon
