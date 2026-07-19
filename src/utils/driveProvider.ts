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
  upload: boolean
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
  upload: false,
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
    createTextFile: true,
    permanentDelete: true,
    trashView: true,
    trashRestore: true,
    trashPurge: true,
    trashClear: true,
    createShare: true,
    importShare: true,
    quickTransfer: true,
    favorite: true,
    colorTag: true,
    encryption: true,
    playbackHistory: true,
    copyTree: true,
    photoAlbum: true
  }),
  cloud123: createCapabilities('cloud123', { createShare: true, importShare: true, trashView: true, trashRestore: true }),
  '115': createCapabilities('115', { trashView: true, trashRestore: true, trashPurge: true }),
  '139': createCapabilities('139'),
  '189': createCapabilities('189'),
  guangya: createCapabilities('guangya', { createTextFile: true, createShare: true, importShare: true }),
  baidu: createCapabilities('baidu'),
  pikpak: createCapabilities('pikpak', { createShare: true, trashView: true, trashRestore: true, trashPurge: true }),
  quark: createCapabilities('quark', { createShare: true, importShare: true, copy: false }),
  dropbox: createCapabilities('dropbox', { createTextFile: true, createShare: true }),
  onedrive: createCapabilities('onedrive', { createTextFile: true, createShare: true }),
  box: createCapabilities('box', { createTextFile: true, createShare: true }),
  webdav: createCapabilities('webdav', { mountedStorage: true, recycleBin: false, permanentDelete: true }),
  s3: createCapabilities('s3', { mountedStorage: true, recycleBin: false, permanentDelete: true }),
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

export const resolveDriveProvider = (context: DriveProviderContext | string = {}): DriveProvider => {
  const resolvedContext = typeof context === 'string' ? { tokenfrom: context } : context
  const tokenfrom = resolvedContext.tokenfrom as DriveProvider | undefined
  if (tokenfrom && tokenfrom !== 'unknown' && Object.prototype.hasOwnProperty.call(driveProviderMap, tokenfrom)) return tokenfrom
  const userId = resolvedContext.userId || ''
  const driveId = resolvedContext.driveId || ''
  for (const [provider, userPrefixes, driveIds] of providerAliases) {
    if (userPrefixes.some((prefix) => userId.startsWith(prefix)) || driveIds.includes(driveId)) return provider
  }
  return 'unknown'
}

export const getDriveProviderCapabilities = (context: DriveProviderContext | string = {}): DriveProviderCapabilities => driveProviderCapabilities[resolveDriveProvider(context)]

export const getDriveProviderMeta = (tokenfrom?: string): DriveProviderMeta => {
  return driveProviderMap[(tokenfrom || 'unknown') as DriveProvider] || driveProviderMap.unknown
}

export const getDriveProviderLabel = (tokenfrom?: string): string => getDriveProviderMeta(tokenfrom).label

export const getDriveProviderIcon = (tokenfrom?: string): string => getDriveProviderMeta(tokenfrom).icon
