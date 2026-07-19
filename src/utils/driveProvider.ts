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
  encryption: boolean
  playbackHistory: boolean
  copyTree: boolean
  photoAlbum: boolean
}

const driveProviderMap: Record<DriveProvider, DriveProviderMeta> = {
  aliyun: {
    key: 'aliyun',
    label: '阿里云盘',
    icon: 'images/drive-icons/aliyun.svg'
  },
  cloud123: {
    key: 'cloud123',
    label: '123网盘',
    icon: 'images/drive-icons/cloud123.svg'
  },
  '115': {
    key: '115',
    label: '115网盘',
    icon: 'images/drive-icons/drive115.svg'
  },
  '139': {
    key: '139',
    label: '139云盘',
    icon: 'images/drive-icons/cloud139.svg'
  },
  '189': {
    key: '189',
    label: '天翼云盘',
    icon: 'images/drive-icons/cloud189.svg'
  },
  guangya: {
    key: 'guangya',
    label: '光鸭云盘',
    icon: 'images/drive-icons/guangya.svg'
  },
  baidu: {
    key: 'baidu',
    label: '百度网盘',
    icon: 'images/drive-icons/baidu.svg'
  },
  pikpak: {
    key: 'pikpak',
    label: 'PikPak',
    icon: 'images/drive-icons/pikpak.png'
  },
  quark: {
    key: 'quark',
    label: '夸克网盘',
    icon: 'images/drive-icons/quark.svg'
  },
  dropbox: {
    key: 'dropbox',
    label: 'Dropbox',
    icon: 'images/drive-icons/dropbox.svg'
  },
  onedrive: {
    key: 'onedrive',
    label: 'OneDrive',
    icon: 'images/drive-icons/onedrive.svg'
  },
  box: {
    key: 'box',
    label: 'Box',
    icon: 'images/drive-icons/box.svg'
  },
  webdav: {
    key: 'webdav',
    label: 'WebDAV',
    icon: ''
  },
  s3: {
    key: 's3',
    label: 'S3',
    icon: ''
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
  recycleBin: true
}

const createCapabilities = (provider: DriveProvider, overrides: Partial<DriveProviderCapabilities> = {}): DriveProviderCapabilities => ({ ...noCapabilities, ...standardFileCapabilities, ...overrides, provider })

const driveProviderCapabilities: Record<DriveProvider, DriveProviderCapabilities> = {
  aliyun: createCapabilities('aliyun', {
    search: true,
    createTextFile: true,
    permanentDelete: true,
    trashView: true,
    trashRestore: true,
    trashPurge: true,
    trashClear: true,
    createShare: true,
    importShare: true,
    manageCreatedShares: true,
    editCreatedShares: true,
    cancelCreatedShares: true,
    manageImportedShares: true,
    shareHistory: true,
    quickTransfer: true,
    favorite: true,
    colorTag: true,
    encryption: true,
    playbackHistory: true,
    copyTree: true,
    photoAlbum: true
  }),
  cloud123: createCapabilities('cloud123', { search: true, createShare: true, importShare: true, manageCreatedShares: true, editCreatedShares: true, trashView: true, trashRestore: true }),
  '115': createCapabilities('115', { search: true, trashView: true, trashRestore: true, trashPurge: true }),
  '139': createCapabilities('139'),
  '189': createCapabilities('189'),
  guangya: createCapabilities('guangya', { search: true, createTextFile: true, createShare: true, importShare: true, manageCreatedShares: true, editCreatedShares: true, cancelCreatedShares: true }),
  baidu: createCapabilities('baidu', { search: true }),
  pikpak: createCapabilities('pikpak', { createShare: true, trashView: true, trashRestore: true, trashPurge: true }),
  quark: createCapabilities('quark', { search: true, createShare: true, importShare: true, manageCreatedShares: true, editCreatedShares: true, cancelCreatedShares: true, manageImportedShares: true, copy: false }),
  dropbox: createCapabilities('dropbox', { search: true, createTextFile: true, createShare: true, manageCreatedShares: true }),
  onedrive: createCapabilities('onedrive', { search: true, createTextFile: true, createShare: true }),
  box: createCapabilities('box', { search: true, createTextFile: true, createShare: true }),
  webdav: createCapabilities('webdav', { uploadMode: 'direct', mountedStorage: true, recycleBin: false, permanentDelete: true }),
  s3: createCapabilities('s3', { uploadMode: 'direct', mountedStorage: true, recycleBin: false, permanentDelete: true }),
  unknown: { ...noCapabilities, provider: 'unknown' }
}

const providerAliases: Array<[DriveProvider, string[], string[]]> = [
  ['webdav', ['webdav:'], []],
  ['s3', ['s3:'], []],
  ['cloud123', ['cloud123_'], ['cloud123']],
  ['115', ['115_'], ['drive115', '115']],
  ['139', ['cloud139_'], ['cloud139', '139']],
  ['189', ['cloud189_'], ['cloud189', '189']],
  ['guangya', ['guangya_'], ['guangya']],
  ['baidu', ['baidu_'], ['baidu']],
  ['pikpak', ['pikpak_'], ['pikpak']],
  ['quark', ['quark_'], ['quark']],
  ['dropbox', ['dropbox_'], ['dropbox']],
  ['onedrive', ['onedrive_'], ['onedrive']],
  ['box', ['box_'], ['box']],
  ['aliyun', ['aliyun_'], []]
]

const driveProviderUserIdPrefixes: Record<DriveProvider, string> = {
  aliyun: '',
  cloud123: 'cloud123_',
  '115': '115_',
  '139': 'cloud139_',
  '189': 'cloud189_',
  guangya: 'guangya_',
  baidu: 'baidu_',
  pikpak: 'pikpak_',
  quark: 'quark_',
  dropbox: 'dropbox_',
  onedrive: 'onedrive_',
  box: 'box_',
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
  for (const [provider, , driveIds] of providerAliases) if (driveIds.includes(driveId)) return provider
  return 'unknown'
}

export const canUseAliyunPreviewApi = (context: DriveProviderContext | string = {}) => resolveDriveProvider(context) === 'aliyun'

export const buildDriveProviderUserId = (provider: DriveProvider, accountId: string | number): string => {
  const value = String(accountId ?? '').trim()
  if (!value) return ''
  const prefix = driveProviderUserIdPrefixes[provider]
  if (!prefix || value.startsWith(prefix)) return value
  return `${prefix}${value}`
}

export const getDriveProviderAccountId = (userId: string, provider?: DriveProvider): string => {
  const value = String(userId || '')
  if (!value) return ''
  const resolvedProvider = provider || resolveDriveProvider({ userId: value })
  const prefix = driveProviderUserIdPrefixes[resolvedProvider]
  return prefix && value.startsWith(prefix) ? value.slice(prefix.length) : value
}

export const getDriveProviderCapabilities = (context: DriveProviderContext | string = {}): DriveProviderCapabilities => driveProviderCapabilities[resolveDriveProvider(context)]

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
  if (key === 'trash') return 'icondelete'
  if (key === 'recover') return 'iconrecover'
  if (key === 'pic_root') return 'iconjietu'
  if (key === 'safe_root') return 'iconsafebox'
  return 'iconfile-folder'
}

export const getDriveProviderSidebarEntries = (context: DriveProviderContext | string, token?: Partial<ITokenInfo>, options: DriveSidebarOptions = {}): DriveSidebarEntry[] => {
  const provider = resolveDriveProvider(context)
  const capabilities = getDriveProviderCapabilities(provider)
  const entries: DriveSidebarEntry[] = []

  if (provider === 'aliyun' && token) {
    const spaces: DriveSidebarEntry[] = []
    const usedDriveIds = new Set<string>()
    const addSpace = (key: string, title: string, driveId: string | undefined) => {
      if (!driveId || usedDriveIds.has(driveId)) return
      usedDriveIds.add(driveId)
      spaces.push({ key, title, icon: getDriveSidebarIcon(key), kind: 'space', driveId })
    }

    const mergedDrive = token.resource_drive_id && token.resource_drive_id === token.backup_drive_id
    if (mergedDrive) {
      if (!options.hideResourceDrive || !options.hideBackupDrive) addSpace('resource_root', '网盘文件', token.resource_drive_id)
    } else {
      if (!options.hideResourceDrive) addSpace('resource_root', '网盘文件', token.resource_drive_id)
      if (!options.hideBackupDrive) addSpace('backup_root', spaces.length ? '备份空间' : '网盘文件', token.backup_drive_id)
    }
    if (!token.resource_drive_id && !token.backup_drive_id) addSpace('backup_root', '网盘文件', token.default_drive_id)
    addSpace('safe_root', '安全盘', token.default_sbox_drive_id || token.sbox_drive_id)
    entries.push(...spaces)

    if (capabilities.photoAlbum && token.pic_drive_id && !options.hideAlbum) {
      entries.push({ key: 'pic_root', title: '相册', icon: getDriveSidebarIcon('pic_root'), kind: 'space', driveId: token.pic_drive_id })
    }
  }

  if (capabilities.favorite) entries.push({ key: 'favorite', title: '收藏夹', icon: getDriveSidebarIcon('favorite'), kind: 'feature' })
  if (capabilities.search) entries.push({ key: 'search', title: '全盘搜索', icon: getDriveSidebarIcon('search'), kind: 'feature' })
  if (capabilities.trashView) entries.push({ key: 'trash', title: '回收站', icon: getDriveSidebarIcon('trash'), kind: 'feature' })
  if (provider === 'aliyun') entries.push({ key: 'recover', title: '文件恢复', icon: getDriveSidebarIcon('recover'), kind: 'feature' })
  return entries
}

export const isDriveProviderSidebarEntryAvailable = (context: DriveProviderContext | string, key: string, token?: Partial<ITokenInfo>, options: DriveSidebarOptions = {}) => {
  if (!isDriveSidebarKey(key)) return true
  return getDriveProviderSidebarEntries(context, token, options).some((entry) => entry.key === key)
}

export const getDriveProviderMeta = (tokenfrom?: string): DriveProviderMeta => {
  return driveProviderMap[(tokenfrom || 'unknown') as DriveProvider] || driveProviderMap.unknown
}

export const getDriveProviderLabel = (tokenfrom?: string): string => getDriveProviderMeta(tokenfrom).label

export const getDriveProviderIcon = (tokenfrom?: string): string => getDriveProviderMeta(tokenfrom).icon
