import path from 'path'
import fsPromises from 'fs/promises'
import { createClient, type FileStat } from 'webdav'
import type { IAliGetFileModel } from '../aliapi/alimodels'
import getFileIcon from '../aliapi/fileicon'
import type { ITokenInfo } from '../user/userstore'

const STORAGE_KEY = 'mnemo.webdav.connections'
const LEGACY_STORAGE_KEY = 'MediaLibrary_WebDavConnections'

export interface WebDavConnectionConfig {
  id: string
  name: string
  url: string
  username: string
  password: string
  rootPath: string
  createdAt: string
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
  return withLeadingSlash.replace(/\/+/g, '/').replace(/\/$/, '') || '/'
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
  const ext = isDir ? '' : path.extname(name).replace('.', '').toLowerCase()
  const size = Number(stat.size || 0)
  const mimeType = stat.mime || ''
  const updatedAt = stat.lastmod
  const time = updatedAt ? new Date(updatedAt).getTime() : Date.now()
  const driveId = `webdav:${config.id}`
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
    sizeStr: '',
    time,
    timeStr: '',
    starred: false,
    isDir,
    thumbnail: '',
    description: '',
    media_duration: undefined,
    media_height: undefined
  }
}

export const isWebDavDrive = (driveId?: string, driveServerId?: string) => {
  return (driveId || '').startsWith('webdav:') || driveServerId === 'webdav'
}

export const getWebDavConnectionId = (driveId?: string) => {
  if (!driveId || !driveId.startsWith('webdav:')) return ''
  return driveId.slice('webdav:'.length)
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
    const raw = localStorage.getItem(STORAGE_KEY) || localStorage.getItem(LEGACY_STORAGE_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    const connections = (parsed as PersistedWebDavConnection[]).map(revealWebDavConnection)
    if (!localStorage.getItem(STORAGE_KEY) || parsed.some((item) => !item?.__mnemo_secret)) {
      saveWebDavConnections(connections)
      localStorage.removeItem(LEGACY_STORAGE_KEY)
    }
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
  const id = btoa(unescape(encodeURIComponent(idSeed)))
    .replace(/[^a-zA-Z0-9]/g, '')
    .slice(0, 24)
  return {
    id,
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
  user_id: `webdav:${connection.id}`,
  user_name: connection.username || connection.name,
  avatar: '',
  nick_name: connection.name,
  default_drive_id: `webdav:${connection.id}`,
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
  const encodedPath = requestPath
    .split('/')
    .filter(Boolean)
    .map((segment) => encodeURIComponent(segment))
    .join('/')
  baseUrl.pathname = `/${encodedPath}`
  if (config.username) baseUrl.username = config.username
  if (config.password) baseUrl.password = config.password
  const urlString = baseUrl.toString()
  return urlString
}

const getApiBaseUrl = (config: WebDavConnectionConfig): string => {
  const currentUrl = new URL(config.url.endsWith('/') ? config.url : `${config.url}/`)
  const pathSegments = currentUrl.pathname.split('/').filter(Boolean)
  if (pathSegments.length > 0) {
    currentUrl.pathname = '/' + pathSegments.slice(0, -1).join('/')
  } else {
    currentUrl.pathname = '/'
  }
  if (!currentUrl.pathname.endsWith('/')) currentUrl.pathname += '/'
  currentUrl.search = ''
  currentUrl.hash = ''
  return currentUrl.toString()
}

const fetchWebDavApiToken = async (config: WebDavConnectionConfig): Promise<string> => {
  const loginUrl = new URL('api/auth/login', getApiBaseUrl(config)).toString()
  const response = await fetch(loginUrl, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({
      username: config.username,
      password: config.password || ''
    })
  })
  if (!response.ok) throw new Error(`WebDAV 登录失败 (${response.status})`)
  const payload = (await response.json().catch(() => null)) as any
  const token = payload?.data?.token
  if (!token) throw new Error(payload?.message || '获取 WebDAV token 失败')
  return token
}

const getWebDavStoragePath = (config: WebDavConnectionConfig, relativePath: string) => {
  const requestPath = joinDavPath(config.rootPath, relativePath)
  return stripPathPrefix(requestPath, getDavBasePath(config))
}

export const getWebDavPlayUrl = async (config: WebDavConnectionConfig, relativePath: string): Promise<string> => {
  const token = await fetchWebDavApiToken(config)
  const apiUrl = new URL('api/fs/get', getApiBaseUrl(config)).toString()
  const requestPath = getWebDavStoragePath(config, relativePath)
  const response = await fetch(apiUrl, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: token
    },
    body: JSON.stringify({ path: relativePath })
  })
  if (!response.ok) throw new Error(`获取 WebDAV 播放地址失败 (${response.status})`)
  const payload = (await response.json().catch(() => null)) as any
  const rawUrl = payload?.data?.raw_url
  if (!rawUrl || typeof rawUrl !== 'string') {
    throw new Error(payload?.message || '获取 WebDAV 播放地址失败')
  }
  return rawUrl
}

export const getWebDavDownloadUrl = async (config: WebDavConnectionConfig, relativePath: string): Promise<string> => {
  try {
    return await getWebDavPlayUrl(config, relativePath)
  } catch (error) {
    console.warn('获取 WebDAV 播放地址失败，回退直链:', error)
    const client = createWebDavClient(config)
    return buildWebDavDownloadUrl(config, relativePath)
  }
}

export const listWebDavDirectory = async (config: WebDavConnectionConfig, relativePath = '/'): Promise<IAliGetFileModel[]> => {
  const normalizedRelativePath = normalizeWebDavPath(relativePath)
  const requestPath = joinDavPath(config.rootPath, normalizedRelativePath)
  const client = createWebDavClient(config)
  const stats = (await client.getDirectoryContents(requestPath)) as FileStat[]
  return stats.filter((stat) => normalizeWebDavPath(stat.filename) !== normalizeWebDavPath(requestPath)).map((stat) => toAliModel(config, stat))
}

export const statWebDavPath = async (config: WebDavConnectionConfig, relativePath: string) => {
  const client = createWebDavClient(config)
  const requestPath = joinDavPath(config.rootPath, relativePath)
  return (await client.stat(requestPath)) as FileStat
}

export const copyWebDavPath = async (config: WebDavConnectionConfig, sourcePath: string, targetPath: string) => {
  const client = createWebDavClient(config)
  await client.copyFile(joinDavPath(config.rootPath, sourcePath), joinDavPath(config.rootPath, targetPath))
}

export const moveWebDavPath = async (config: WebDavConnectionConfig, sourcePath: string, targetPath: string) => {
  const client = createWebDavClient(config)
  await client.moveFile(joinDavPath(config.rootPath, sourcePath), joinDavPath(config.rootPath, targetPath))
}

export const renameWebDavPath = async (config: WebDavConnectionConfig, sourcePath: string, newName: string) => {
  const normalizedSource = normalizeWebDavPath(sourcePath)
  const targetPath = normalizeWebDavPath(`${path.posix.dirname(normalizedSource)}/${newName}`)
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
