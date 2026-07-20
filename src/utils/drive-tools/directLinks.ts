import type { IAliGetFileModel } from '../../aliapi/alimodels'
import type { IDownloadUrl } from '../../aliapi/models'

export type DirectLinkFormat = 'url' | 'aria2'

export interface DirectLinkItem {
  name: string
  fileId: string
  driveId: string
  userId: string
  url: string
  size: number
  headers?: Record<string, string>
}

export interface DirectLinkExportResult {
  total: number
  success: number
  failed: number
  links: DirectLinkItem[]
  failures: { name: string; reason: string }[]
  text: string
}

export const normalizeDriveToolDriveId = (driveId: string): string => {
  const id = String(driveId || '')
  const aliases: Record<string, string> = { '139': 'cloud139', '189': 'cloud189' }
  return aliases[id] || id
}

export const normalizeDriveToolPlatform = (platform: string): string => {
  const value = String(platform || '')
  const aliases: Record<string, string> = {
    cloud139: '139',
    cloud189: '189'
  }
  return aliases[value] || value
}

export const driveToolPlatformMatches = (tokenfrom: string, requested?: string): boolean => {
  if (!requested) return true
  return normalizeDriveToolPlatform(tokenfrom || 'aliyun') === normalizeDriveToolPlatform(requested)
}

export const driveToolRootIdFor = (driveId: string): string => {
  const map: Record<string, string> = {
    pikpak: 'pikpak_root',
    quark: 'quark_root',
    cloud139: 'cloud139_root',
    cloud189: 'cloud189_root',
    guangya: 'guangya_root',
  }
  return map[normalizeDriveToolDriveId(driveId)] || 'root'
}

export const driveToolDriveIdForPlatform = (platform: string, defaultDriveId = ''): string => {
  if (platform === 'aliyun') return defaultDriveId
  return normalizeDriveToolDriveId(defaultDriveId || platform)
}

const providerRootParent = (driveId: string, fileId: string) => {
  const providerDriveId = normalizeDriveToolDriveId(driveId)
  if (providerDriveId === 'pikpak') return fileId === 'pikpak_root' ? 'pikpak_root' : fileId
  if (providerDriveId === 'quark') return fileId === 'quark_root' ? '0' : fileId
  if (providerDriveId === 'cloud139') return fileId === 'cloud139_root' ? '/' : fileId
  if (providerDriveId === 'cloud189') return fileId === 'cloud189_root' ? '-11' : fileId
  if (providerDriveId === 'guangya') return fileId === 'guangya_root' ? 'guangya_root' : fileId
  return fileId
}

export const listDriveToolChildren = async (userId: string, driveId: string, fileId: string): Promise<IAliGetFileModel[]> => {
  if (driveId.startsWith('webdav:')) {
    const { getWebDavConnection, getWebDavConnectionId, listWebDavDirectory } = await import('../webdavClient')
    const connection = getWebDavConnection(getWebDavConnectionId(driveId))
    return connection ? listWebDavDirectory(connection, fileId || '/') : []
  }
  const providerDriveId = normalizeDriveToolDriveId(driveId)
  const parentId = providerRootParent(providerDriveId, fileId)
  if (providerDriveId === 'pikpak') {
    const { apiPikPakFileList, mapPikPakFileToAliModel } = await import('../../pikpak/dirfilelist')
    const { items } = await apiPikPakFileList(userId, parentId, 100)
    return items.map(item => mapPikPakFileToAliModel(item, providerDriveId, parentId))
  }
  if (providerDriveId === 'quark') {
    const { apiQuarkFileList, mapQuarkFileToAliModel } = await import('../../quark/dirfilelist')
    const { items } = await apiQuarkFileList(userId, parentId, 200)
    return items.map(item => mapQuarkFileToAliModel(item, providerDriveId, fileId))
  }
  if (providerDriveId === 'cloud139') {
    const { apiCloud139FileList, mapCloud139FileToAliModel } = await import('../../cloud139/dirfilelist')
    return (await apiCloud139FileList(userId, parentId, 200)).map(item => mapCloud139FileToAliModel(item, providerDriveId, fileId))
  }
  if (providerDriveId === 'cloud189') {
    const { apiCloud189FileList, mapCloud189FileToAliModel } = await import('../../cloud189/dirfilelist')
    return (await apiCloud189FileList(userId, parentId, 200)).map(item => mapCloud189FileToAliModel(item, providerDriveId, fileId))
  }
  if (providerDriveId === 'guangya') {
    const { apiGuangyaFileList, mapGuangyaFileToAliModel } = await import('../../guangya/dirfilelist')
    return (await apiGuangyaFileList(userId, parentId, 200)).map(item => mapGuangyaFileToAliModel(item, providerDriveId, fileId))
  }
  const { default: AliDirFileList } = await import('../../aliapi/dirfilelist')
  const dir = await AliDirFileList.ApiDirFileList(userId, driveId, fileId, '', 'name asc')
  return dir.items
}

export const flattenDriveToolFiles = async (files: IAliGetFileModel[], userId: string, maxFiles = 300): Promise<IAliGetFileModel[]> => {
  const output: IAliGetFileModel[] = []
  const queue = [...files]
  while (queue.length && output.length < maxFiles) {
    const item = queue.shift()!
    if (!item.isDir) {
      output.push(item)
      continue
    }
    const children = await listDriveToolChildren(userId, item.drive_id, item.file_id).catch(() => [])
    queue.push(...children)
  }
  return output.slice(0, maxFiles)
}

export const formatDirectLinks = (links: DirectLinkItem[], format: DirectLinkFormat): string => {
  if (format === 'aria2') {
    return links.map(item => {
      const headers = Object.entries(item.headers || {}).map(([key, value]) => `  header=${key}: ${value}`)
      return [item.url, `  out=${item.name}`, ...headers].join('\n')
    }).join('\n\n')
  }
  return links.map(item => item.url).join('\n')
}

export const exportDirectLinks = async (files: IAliGetFileModel[], userId: string, format: DirectLinkFormat = 'url', maxFiles = 300): Promise<DirectLinkExportResult> => {
  const result: DirectLinkExportResult = { total: 0, success: 0, failed: 0, links: [], failures: [], text: '' }
  const { default: AliFile } = await import('../../aliapi/file')
  const groups = new Map<string, IAliGetFileModel[]>()
  for (const file of files) {
    const key = (file as any).user_id || (file as any).userId || userId
    groups.set(key, [...(groups.get(key) || []), file])
  }
  for (const [currentUserId, group] of groups) {
    const flatFiles = await flattenDriveToolFiles(group, currentUserId, maxFiles)
    result.total += flatFiles.length
    for (const file of flatFiles) {
      const data = await AliFile.ApiFileDownloadUrl(currentUserId, file.drive_id, file.file_id, 14400).catch((error: any) => error?.message || '获取下载地址失败')
      if (typeof data === 'string') {
        result.failed += 1
        result.failures.push({ name: file.name, reason: data })
        continue
      }
      const down = data as IDownloadUrl
      result.links.push({ name: file.name, fileId: file.file_id, driveId: file.drive_id, userId: currentUserId, url: down.url, size: down.size || file.size || 0, headers: down.headers })
      result.success += 1
    }
  }
  result.text = formatDirectLinks(result.links, format)
  return result
}
