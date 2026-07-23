import path from 'path'
import { createReadStream } from 'fs'
import fsPromises from 'fs/promises'
import { createClient, type FileStat } from 'webdav'
import type { IAliGetFileModel } from '../aliapi/alimodels'
import getFileIcon from '../aliapi/fileicon'
import type { ITokenInfo } from '../user/userstore'
import { humanDateTimeDateStr, humanSize } from './format'
import { resolveFileExt } from './filetype'

const STORAGE_KEY = 'mnemo.webdav.connections'

export interface WebDavConnectionConfig {
  id: string
  provider?: 'webdav'
  name: string
  url: string
  username: string
  password: string
  rootPath: string
  createdAt: string
}

export interface WebDavQuotaInfo {
  used: number
  available: number | 'unknown' | 'unlimited'
  total: number
}

type PersistedWebDavConnection = Omit<WebDavConnectionConfig, 'username' | 'password'> & Partial<Pick<WebDavConnectionConfig, 'username' | 'password'>> & {
  __mnemo_secret?: string
  __mnemo_secret_version?: number
}

const VIDEO_EXTENSIONS = new Set(['.mp4', '.mkv', '.avi', '.mov', '.wmv', '.flv', '.webm', '.m4v', '.mpg', '.mpeg', '.3gp', '.rmvb', '.asf', '.divx', '.xvid', '.ts', '.m2ts', '.mts', '.vob', '.ogv', '.dv'])

const normalizeUrl = (url: string) => url.trim().replace(/\/+$/, '')

export const normalizeWebDavPath = (value: string) => {
  const trimmed = (value || '/').trim()
  if (!trimmed || trimmed === '/') return '/'
  const withLeadingSlash = trimmed.startsWith('/') ? trimmed : `/${trimmed}`
  const normalized = path.posix.normalize(withLeadingSlash.replace(/\/+/g, '/'))
  return normalized.replace(/\/$/, '') || '/'
}

const joinDavPath = (basePath: string, nextPath: string) => {
  const normalizedBase = normalizeWebDavPath(basePath)
  const normalizedNext = normalizeWebDavPath(nextPath)
  if (normalizedBase === '/') return normalizedNext
  if (normalizedNext === '/') return normalizedBase
  return normalizeWebDavPath(`${normalizedBase}/${normalizedNext.slice(1)}`)
}

const createWebDavClient = (config: WebDavConnectionConfig) => {
  return createClient(config.url, {
    username: config.username,
    password: config.password
  })
}

const getWebDavDriveId = (config: WebDavConnectionConfig) => `webdav:${config.id}`

const getDavBasePath = (config: WebDavConnectionConfig) => {
  const currentUrl = new URL(config.url.endsWith('/') ? config.url : `${config.url}/`)
  return normalizeWebDavPath(currentUrl.pathname || '/')
}

const stripPathPrefix = (value: string, prefix: string) => {
  const normalizedValue = normalizeWebDavPath(value)
  const normalizedPrefix = normalizeWebDavPath(prefix)
  if (normalizedPrefix !== '/' && normalizedValue.startsWith(normalizedPrefix)) {
    return normalizeWebDavPath(normalizedValue.slice(normalizedPrefix.length) || '/')
  }
  return normalizedValue
}

const getRelativeDavPath = (filename: string, config: WebDavConnectionConfig) => {
  let normalizedFilename = normalizeWebDavPath(filename)
  normalizedFilename = stripPathPrefix(normalizedFilename, getDavBasePath(config))
  normalizedFilename = stripPathPrefix(normalizedFilename, config.rootPath)
  return normalizedFilename
}

const toAliModel = (config: WebDavConnectionConfig, stat: FileStat): IAliGetFileModel => {
  const relativePath = getRelativeDavPath(stat.filename, config)
  const name = stat.basename || path.posix.basename(relativePath) || config.name
  const isDir = stat.type === 'directory'
  const ext = isDir ? '' : resolveFileExt(name, '', stat.mime || '')
  const size = Number(stat.size || 0)
  const mimeType = stat.mime || ''
  const updatedAt = stat.lastmod
  const parsedTime = updatedAt ? new Date(updatedAt).getTime() : 0
  const time = Number.isFinite(parsedTime) ? parsedTime : 0
  const driveId = getWebDavDriveId(config)
  const iconInfo = isDir ? ['folder', 'iconfile-folder'] : getFileIcon(isDir ? 'folder' : VIDEO_EXTENSIONS.has(`.${ext}`) ? 'video' : 'others', ext, ext, mimeType, size)

  return {
    __v_skip: true,
    drive_id: driveId,
    file_id: relativePath,
    parent_file_id: normalizeWebDavPath(path.posix.dirname(relativePath)),
    name,
    namesearch: name.toLowerCase(),
    path: relativePath,
    ext,
    mime_type: mimeType,
    mime_extension: ext,
    category: iconInfo[0],
    icon: iconInfo[1],
    size,
    sizeStr: isDir ? '' : humanSize(size),
    time,
    timeStr: time ? humanDateTimeDateStr(new Date(time).toISOString()) : '',
    starred: false,
    isDir,
    thumbnail: '',
    description: '',
    media_duration: undefined,
    media_height: undefined
  }
}

export const isWebDavDrive = (driveId?: string, driveServerId?: string) => /^webdav:/.test(driveId || '') || driveServerId === 'webdav'

export const getWebDavConnectionId = (driveId?: string) => {
  const match = /^webdav:(.*)$/.exec(driveId || '')
  return match?.[1] || ''
}

const protectWebDavConnection = (connection: WebDavConnectionConfig): PersistedWebDavConnection => {
  const { username, password, ...stored } = connection
  return {
    ...stored,
    __mnemo_secret: (globalThis as unknown as Window).WebSafeStorageEncryptSync(JSON.stringify({ username, password })),
    __mnemo_secret_version: 1
  }
}

const revealWebDavConnection = (stored: PersistedWebDavConnection): WebDavConnectionConfig => {
  if (!stored.__mnemo_secret) return stored as WebDavConnectionConfig
  const { __mnemo_secret, __mnemo_secret_version, ...connection } = stored
  const credentials = JSON.parse((globalThis as unknown as Window).WebSafeStorageDecryptSync(__mnemo_secret)) as Pick<WebDavConnectionConfig, 'username' | 'password'>
  return { ...connection, ...credentials }
}

const saveWebDavConnections = (connections: WebDavConnectionConfig[]) => {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(connections.map(protectWebDavConnection)))
}

export const getWebDavConnections = (): WebDavConnectionConfig[] => {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    const connections = (parsed as PersistedWebDavConnection[]).map(revealWebDavConnection)
    if (parsed.some((item) => !item?.__mnemo_secret)) saveWebDavConnections(connections)
    return connections
  } catch (error) {
    console.error('读取 WebDAV 连接配置失败:', error)
    return []
  }
}

export const saveWebDavConnection = (config: WebDavConnectionConfig) => {
  const list = getWebDavConnections()
  const name = config.name.trim()
  if (!name) throw new Error('请填写 WebDAV 连接名称')
  const duplicate = list.find((item) => item.id !== config.id && item.name.trim().toLocaleLowerCase() === name.toLocaleLowerCase())
  if (duplicate) throw new Error(`WebDAV 连接名称“${name}”已存在`)
  const normalizedConfig = { ...config, name }
  const index = list.findIndex((item) => item.id === config.id)
  if (index >= 0) {
    list[index] = normalizedConfig
  } else {
    list.unshift(normalizedConfig)
  }
  saveWebDavConnections(list)
}

export const removeWebDavConnection = (id: string) => {
  const list = getWebDavConnections().filter((item) => item.id !== id)
  saveWebDavConnections(list)
}

export const getWebDavConnection = (id: string) => {
  return getWebDavConnections().find((item) => item.id === id)
}

export const createWebDavConnection = (input: { name: string; url: string; username: string; password: string; rootPath?: string }): WebDavConnectionConfig => {
  if (!input.name.trim()) throw new Error('请填写 WebDAV 连接名称')
  const normalizedUrl = normalizeUrl(input.url)
  const normalizedRoot = normalizeWebDavPath(input.rootPath || '/')
  const timestamp = Date.now().toString()
  const idSeed = `${normalizedUrl}|${input.username}|${normalizedRoot}|${timestamp}`
  const randomId = globalThis.crypto?.randomUUID?.().replace(/-/g, '')
  const id = randomId || btoa(unescape(encodeURIComponent(idSeed))).replace(/[^a-zA-Z0-9]/g, '').slice(-24)
  return {
    id,
    provider: 'webdav',
    name: input.name.trim(),
    url: normalizedUrl,
    username: input.username.trim(),
    password: input.password,
    rootPath: normalizedRoot,
    createdAt: new Date().toISOString()
  }
}

export const createWebDavUserToken = (connection: WebDavConnectionConfig): ITokenInfo => ({
  tokenfrom: 'webdav',
  access_token: '',
  refresh_token: '',
  session_expires_in: 0,
  open_api_token_type: '',
  open_api_access_token: '',
  open_api_refresh_token: '',
  open_api_expires_in: 0,
  signature: '',
  device_id: '',
  expires_in: 0,
  token_type: '',
  user_id: getWebDavDriveId(connection),
  user_name: connection.username || connection.name,
  avatar: '',
  nick_name: connection.name,
  default_drive_id: getWebDavDriveId(connection),
  default_sbox_drive_id: '',
  resource_drive_id: '',
  backup_drive_id: '',
  sbox_drive_id: '',
  role: '',
  status: '',
  expire_time: '',
  state: '',
  pin_setup: false,
  is_first_login: false,
  need_rp_verify: false,
  name: connection.name,
  spu_id: '',
  is_expires: false,
  used_size: 0,
  total_size: 0,
  free_size: 0,
  space_expire: false,
  spaceinfo: '',
  vipname: '',
  vipIcon: '',
  vipexpire: '',
  pic_drive_id: '',
  signInfo: { signMon: -1, signDay: -1 }
})

export const buildWebDavDownloadUrl = (config: WebDavConnectionConfig, relativePath: string): string => {
  const requestPath = joinDavPath(config.rootPath, relativePath)
  const baseUrl = new URL(config.url.endsWith('/') ? config.url : `${config.url}/`)
  const baseSegments = baseUrl.pathname
    .split('/')
    .filter(Boolean)
  const requestSegments = requestPath.split('/').filter(Boolean)
  const encodeSegment = (segment: string) => {
    try {
      return encodeURIComponent(decodeURIComponent(segment))
    } catch {
      return encodeURIComponent(segment)
    }
  }
  baseUrl.pathname = `/${[...baseSegments, ...requestSegments].map(encodeSegment).join('/')}`
  baseUrl.username = ''
  baseUrl.password = ''
  return baseUrl.toString()
}

export const getWebDavDownloadUrl = async (config: WebDavConnectionConfig, relativePath: string): Promise<string> => {
  return buildWebDavDownloadUrl(config, relativePath)
}

export const getWebDavRequestHeaders = (config: WebDavConnectionConfig): Record<string, string> => {
  if (!config.username) return {}
  const bytes = new TextEncoder().encode(`${config.username}:${config.password || ''}`)
  let binary = ''
  bytes.forEach((byte) => {
    binary += String.fromCharCode(byte)
  })
  return { Authorization: `Basic ${btoa(binary)}` }
}

export const getWebDavQuota = async (config: WebDavConnectionConfig): Promise<WebDavQuotaInfo | null> => {
  const client = createWebDavClient(config)
  if (typeof client.getQuota !== 'function') return null
  try {
    type QuotaPayload = { used?: number | string; available?: number | string | 'unknown' | 'unlimited' }
    type QuotaResult = QuotaPayload & { body?: QuotaPayload; data?: QuotaPayload }
    const result = await client.getQuota({ path: normalizeWebDavPath(config.rootPath) }) as QuotaResult | null
    const quota = result?.data && typeof result.data === 'object'
      ? result.data
      : result?.body && typeof result.body === 'object'
        ? result.body
        : result
    const used = Number(quota?.used)
    const rawAvailable = quota?.available
    const available = (() => {
      if (typeof rawAvailable === 'string') {
        const marker = rawAvailable.trim().toLowerCase()
        if (marker === 'unlimited') return 'unlimited' as const
        if (marker === 'unknown') return 'unknown' as const
      }
      if (rawAvailable === undefined || rawAvailable === null || rawAvailable === '') return null
      const numeric = Number(rawAvailable)
      if (!Number.isFinite(numeric)) return null
      if (numeric === -3) return 'unlimited' as const
      if (numeric === -2 || numeric === -1) return 'unknown' as const
      return numeric >= 0 ? numeric : null
    })()
    if (!Number.isFinite(used) || used < 0 || (typeof available !== 'number' && available !== 'unknown' && available !== 'unlimited')) return null
    if (typeof available === 'number') {
      if (!Number.isFinite(available) || available < 0) return null
      const total = used + available
      if (!Number.isFinite(total)) return null
      return { used, available, total }
    }
    return { used, available, total: 0 }
  } catch {
    return null
  }
}

export const applyWebDavQuota = async (token: ITokenInfo, config: WebDavConnectionConfig): Promise<boolean> => {
  const quota = await getWebDavQuota(config)
  if (!quota) {
    token.used_size = 0
    token.free_size = 0
    token.total_size = 0
    token.spaceinfo = '服务器未提供容量信息'
    return false
  }
  token.used_size = quota.used
  token.free_size = typeof quota.available === 'number' ? quota.available : 0
  token.total_size = quota.total
  token.spaceinfo = quota.total > 0
    ? `${humanSize(quota.used)} / ${humanSize(quota.total)}`
    : quota.available === 'unlimited'
      ? `已用 ${humanSize(quota.used)}，容量无限制`
      : `已用 ${humanSize(quota.used)}，服务器未提供总容量`
  return true
}

export const listWebDavDirectory = async (config: WebDavConnectionConfig, relativePath = '/'): Promise<IAliGetFileModel[]> => {
  const normalizedRelativePath = normalizeWebDavPath(relativePath)
  const requestPath = joinDavPath(config.rootPath, normalizedRelativePath)
  const client = createWebDavClient(config)
  const stats = (await client.getDirectoryContents(requestPath)) as FileStat[]
  return stats.filter((stat) => getRelativeDavPath(stat.filename, config) !== normalizedRelativePath).map((stat) => toAliModel(config, stat))
}

export const statWebDavPath = async (config: WebDavConnectionConfig, relativePath: string) => {
  const client = createWebDavClient(config)
  const requestPath = joinDavPath(config.rootPath, relativePath)
  return (await client.stat(requestPath)) as FileStat
}

const normalizeWebDavTransferPaths = (sourcePath: string, targetPath: string) => {
  const source = normalizeWebDavPath(sourcePath)
  const target = normalizeWebDavPath(targetPath)
  if (source === '/' || target === '/') throw new Error('WebDAV 根目录不能作为文件操作对象')
  if (source === target) throw new Error('WebDAV 源路径与目标路径不能相同')
  if (target.startsWith(`${source}/`)) throw new Error('WebDAV 目录不能复制或移动到自身的子目录')
  return { source, target }
}

export const copyWebDavPath = async (config: WebDavConnectionConfig, sourcePath: string, targetPath: string) => {
  const { source, target } = normalizeWebDavTransferPaths(sourcePath, targetPath)
  const client = createWebDavClient(config)
  await client.copyFile(joinDavPath(config.rootPath, source), joinDavPath(config.rootPath, target))
}

export const moveWebDavPath = async (config: WebDavConnectionConfig, sourcePath: string, targetPath: string) => {
  const { source, target } = normalizeWebDavTransferPaths(sourcePath, targetPath)
  const client = createWebDavClient(config)
  await client.moveFile(joinDavPath(config.rootPath, source), joinDavPath(config.rootPath, target))
}

export const renameWebDavPath = async (config: WebDavConnectionConfig, sourcePath: string, newName: string) => {
  const normalizedName = newName.trim()
  if (!normalizedName || normalizedName === '.' || normalizedName === '..' || /[\\/]/.test(normalizedName)) throw new Error('WebDAV 名称不能为空或包含路径分隔符')
  const normalizedSource = normalizeWebDavPath(sourcePath)
  const targetPath = normalizeWebDavPath(`${path.posix.dirname(normalizedSource)}/${normalizedName}`)
  await moveWebDavPath(config, normalizedSource, targetPath)
  return targetPath
}

export const deleteWebDavPath = async (config: WebDavConnectionConfig, relativePath: string) => {
  const client = createWebDavClient(config)
  await client.deleteFile(joinDavPath(config.rootPath, relativePath))
}

export const createWebDavDirectory = async (config: WebDavConnectionConfig, relativePath: string) => {
  const client = createWebDavClient(config)
  await client.createDirectory(joinDavPath(config.rootPath, relativePath), { recursive: true })
}

const uploadWebDavEntry = async (client: ReturnType<typeof createWebDavClient>, localPath: string, remotePath: string) => {
  const stat = await fsPromises.stat(localPath)
  if (stat.isDirectory()) {
    await client.createDirectory(remotePath, { recursive: true })
    const children = await fsPromises.readdir(localPath)
    for (const child of children) {
      const childLocalPath = path.join(localPath, child)
      const childRemotePath = normalizeWebDavPath(`${remotePath}/${child}`)
      await uploadWebDavEntry(client, childLocalPath, childRemotePath)
    }
    return
  }

  const content = await fsPromises.readFile(localPath)
  await client.putFileContents(remotePath, content, { overwrite: true })
}

export const uploadWebDavLocalPaths = async (config: WebDavConnectionConfig, parentRelativePath: string, localPaths: string[]) => {
  const client = createWebDavClient(config)
  const parentRemotePath = joinDavPath(config.rootPath, parentRelativePath)
  await client.createDirectory(parentRemotePath, { recursive: true })
  for (const localPath of localPaths) {
    const baseName = path.basename(localPath)
    const remotePath = normalizeWebDavPath(`${parentRemotePath}/${baseName}`)
    await uploadWebDavEntry(client, localPath, remotePath)
  }
}

export const testWebDavConnection = async (config: WebDavConnectionConfig) => {
  await listWebDavDirectory(config, '/')
}

/** 递归列出某目录下的全部文件（同步引擎用），file_id 为相对路径 */
export const listWebDavRecursive = async (config: WebDavConnectionConfig, relativePath = '/'): Promise<IAliGetFileModel[]> => {
  const result: IAliGetFileModel[] = []
  const walk = async (dir: string): Promise<void> => {
    const items = await listWebDavDirectory(config, dir)
    for (const item of items) {
      if (item.isDir) await walk(item.file_id)
      else result.push(item)
    }
  }
  await walk(relativePath)
  return result
}

/** 以流的方式上传单个文件（避免大文件整体读入内存） */
export const uploadWebDavFile = async (config: WebDavConnectionConfig, relativePath: string, localPath: string, onProgress?: (transferred: number) => void) => {
  const client = createWebDavClient(config)
  const remotePath = joinDavPath(config.rootPath, relativePath)
  const stat = await fsPromises.stat(localPath)
  const stream = createReadStream(localPath)
  if (onProgress) {
    let transferred = 0
    stream.on('data', (chunk: string | Buffer) => {
      transferred += typeof chunk === 'string' ? Buffer.byteLength(chunk) : chunk.length
      onProgress(transferred)
    })
  }
  await client.putFileContents(remotePath, stream, { overwrite: true, contentLength: stat.size })
  const remote = await statWebDavPath(config, relativePath).catch(() => null)
  return { size: stat.size, time: remote?.lastmod ? new Date(remote.lastmod).getTime() || 0 : Date.now() }
}
