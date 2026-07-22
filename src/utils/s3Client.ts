import fs from 'node:fs'
import fsPromises from 'node:fs/promises'
import path from 'node:path'
import { CopyObjectCommand, DeleteObjectCommand, DeleteObjectsCommand, GetObjectCommand, HeadBucketCommand, HeadObjectCommand, ListObjectsV2Command, PutObjectCommand, S3Client, type _Object as S3Object } from '@aws-sdk/client-s3'
import { Upload } from '@aws-sdk/lib-storage'
import { getSignedUrl } from '@aws-sdk/s3-request-presigner'
import type { IAliGetFileModel } from '../aliapi/alimodels'
import getFileIcon from '../aliapi/fileicon'
import type { ITokenInfo } from '../user/userstore'
import { humanDateTimeDateStr, humanSize } from './format'

const STORAGE_KEY = 'Mnemo_S3Connections'
const S3_PREFIX = 's3:'
const VIDEO_EXTENSIONS = new Set(['.mp4', '.mkv', '.avi', '.mov', '.wmv', '.flv', '.webm', '.m4v', '.mpg', '.mpeg', '.3gp', '.rmvb', '.asf', '.divx', '.xvid', '.ts', '.m2ts', '.mts', '.vob', '.ogv', '.dv'])

export interface S3ConnectionConfig {
  id: string
  name: string
  endpoint: string
  region: string
  accessKeyId: string
  secretAccessKey: string
  sessionToken: string
  bucket: string
  rootPrefix: string
  forcePathStyle: boolean
  createdAt: string
}

type PersistedS3Connection = Omit<S3ConnectionConfig, 'accessKeyId' | 'secretAccessKey' | 'sessionToken'> & Partial<Pick<S3ConnectionConfig, 'accessKeyId' | 'secretAccessKey' | 'sessionToken'>> & {
  __mnemo_secret?: string
  __mnemo_secret_version?: number
}

export type S3ConnectionInput = Omit<S3ConnectionConfig, 'id' | 'createdAt'>

const normalizeEndpoint = (value: string) => value.trim().replace(/\/+$/, '')

export const normalizeS3RelativePath = (value?: string) => {
  const normalized = String(value || '/')
    .replace(/\\/g, '/')
    .replace(/\/{2,}/g, '/')
  if (!normalized || normalized === '/') return '/'
  const withLeadingSlash = normalized.startsWith('/') ? normalized : `/${normalized}`
  return withLeadingSlash.replace(/\/$/, '') || '/'
}

export const normalizeS3Prefix = (value?: string) => {
  const normalized = String(value || '')
    .trim()
    .replace(/\\/g, '/')
    .replace(/^\/+|\/+$/g, '')
    .replace(/\/{2,}/g, '/')
  return normalized ? `${normalized}/` : ''
}

const joinS3Key = (rootPrefix: string, relativePath: string, directory = false) => {
  const root = normalizeS3Prefix(rootPrefix)
  const relative = normalizeS3RelativePath(relativePath).replace(/^\//, '')
  const key = `${root}${relative}`.replace(/\/{2,}/g, '/')
  if (!key) return ''
  return directory && !key.endsWith('/') ? `${key}/` : key
}

const stripRootPrefix = (key: string, rootPrefix: string) => {
  const root = normalizeS3Prefix(rootPrefix)
  const stripped = root && key.startsWith(root) ? key.slice(root.length) : key
  return normalizeS3RelativePath(stripped.replace(/\/$/, ''))
}

const createS3Client = (config: S3ConnectionConfig) => {
  return new S3Client({
    endpoint: config.endpoint || undefined,
    region: config.region || 'us-east-1',
    forcePathStyle: config.forcePathStyle,
    credentials: {
      accessKeyId: config.accessKeyId,
      secretAccessKey: config.secretAccessKey,
      sessionToken: config.sessionToken || undefined
    }
  })
}

const createConnectionId = (input: S3ConnectionInput) => {
  const seed = `${Date.now()}|${Math.random()}|${input.name}|${input.endpoint}|${input.bucket}|${input.rootPrefix}`
  return btoa(unescape(encodeURIComponent(seed)))
    .replace(/[^a-zA-Z0-9]/g, '')
    .slice(0, 28)
}

const protectS3Connection = (connection: S3ConnectionConfig): PersistedS3Connection => {
  const { accessKeyId, secretAccessKey, sessionToken, ...stored } = connection
  return {
    ...stored,
    __mnemo_secret: (globalThis as unknown as Window).WebSafeStorageEncryptSync(JSON.stringify({ accessKeyId, secretAccessKey, sessionToken })),
    __mnemo_secret_version: 1
  }
}

const revealS3Connection = (stored: PersistedS3Connection): S3ConnectionConfig => {
  if (!stored.__mnemo_secret) return stored as S3ConnectionConfig
  const { __mnemo_secret, __mnemo_secret_version, ...connection } = stored
  const credentials = JSON.parse((globalThis as unknown as Window).WebSafeStorageDecryptSync(__mnemo_secret)) as Pick<S3ConnectionConfig, 'accessKeyId' | 'secretAccessKey' | 'sessionToken'>
  return { ...connection, ...credentials }
}

const saveS3Connections = (connections: S3ConnectionConfig[]) => {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(connections.map(protectS3Connection)))
}

export const getS3Connections = (): S3ConnectionConfig[] => {
  try {
    const parsed = JSON.parse(localStorage.getItem(STORAGE_KEY) || '[]')
    if (!Array.isArray(parsed)) return []
    const connections = (parsed as PersistedS3Connection[]).map(revealS3Connection)
    if (parsed.some((item) => !item?.__mnemo_secret)) saveS3Connections(connections)
    return connections
  } catch (error) {
    console.error('读取 S3 连接配置失败:', error)
    return []
  }
}

export const saveS3Connection = (connection: S3ConnectionConfig) => {
  const connections = getS3Connections()
  const name = connection.name.trim()
  if (!name) throw new Error('请填写 S3 连接名称')
  const duplicate = connections.find((item) => item.id !== connection.id && item.name.trim().toLocaleLowerCase() === name.toLocaleLowerCase())
  if (duplicate) throw new Error(`S3 连接名称“${name}”已存在`)
  const normalizedConnection = { ...connection, name }
  const index = connections.findIndex((item) => item.id === connection.id)
  if (index >= 0) connections[index] = normalizedConnection
  else connections.unshift(normalizedConnection)
  saveS3Connections(connections)
}

export const removeS3Connection = (id: string) => saveS3Connections(getS3Connections().filter((item) => item.id !== id))

export const getS3Connection = (id: string) => getS3Connections().find((item) => item.id === id)

export const getS3ConnectionId = (driveId?: string) => (driveId?.startsWith(S3_PREFIX) ? driveId.slice(S3_PREFIX.length) : '')

export const isS3Drive = (driveId?: string, driveServerId?: string) => !!driveId?.startsWith(S3_PREFIX) || driveServerId === 's3'

export const createS3Connection = (input: S3ConnectionInput): S3ConnectionConfig => {
  const name = input.name.trim()
  const bucket = input.bucket.trim()
  if (!name) throw new Error('请填写 S3 连接名称')
  if (!bucket) throw new Error('请填写 S3 Bucket')
  if (!input.accessKeyId.trim() || !input.secretAccessKey.trim()) throw new Error('请填写 S3 Access Key 和 Secret Key')
  return {
    ...input,
    id: createConnectionId(input),
    name,
    endpoint: normalizeEndpoint(input.endpoint),
    region: input.region.trim() || 'us-east-1',
    accessKeyId: input.accessKeyId.trim(),
    secretAccessKey: input.secretAccessKey.trim(),
    sessionToken: input.sessionToken.trim(),
    bucket,
    rootPrefix: normalizeS3Prefix(input.rootPrefix),
    forcePathStyle: !!input.forcePathStyle,
    createdAt: new Date().toISOString()
  }
}

export const createS3UserToken = (connection: S3ConnectionConfig): ITokenInfo => ({
  tokenfrom: 's3',
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
  user_id: `${S3_PREFIX}${connection.id}`,
  user_name: connection.accessKeyId,
  avatar: '',
  nick_name: connection.name,
  default_drive_id: `${S3_PREFIX}${connection.id}`,
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

const toAliModel = (config: S3ConnectionConfig, key: string, object?: S3Object, directory = false): IAliGetFileModel => {
  const relativePath = stripRootPrefix(key, config.rootPrefix)
  const name = path.posix.basename(relativePath) || config.name
  const ext = directory ? '' : path.posix.extname(name).slice(1).toLowerCase()
  const size = Number(object?.Size || 0)
  const category = directory ? 'folder' : VIDEO_EXTENSIONS.has(`.${ext}`) ? 'video' : 'others'
  const iconInfo = directory ? ['folder', 'iconfile-folder'] : getFileIcon(category, ext, ext, '', size)
  return {
    __v_skip: true,
    drive_id: `${S3_PREFIX}${config.id}`,
    file_id: relativePath,
    parent_file_id: normalizeS3RelativePath(path.posix.dirname(relativePath)),
    name,
    namesearch: name.toLowerCase(),
    path: relativePath,
    ext,
    mime_type: '',
    mime_extension: ext,
    category: iconInfo[0],
    icon: iconInfo[1],
    size,
    sizeStr: directory ? '' : humanSize(size),
    time: object?.LastModified?.getTime() || 0,
    timeStr: object?.LastModified ? humanDateTimeDateStr(object.LastModified.toISOString()) : '',
    starred: false,
    isDir: directory,
    thumbnail: '',
    description: object?.ETag || '',
    media_duration: undefined,
    media_height: undefined
  }
}

export const testS3Connection = async (config: S3ConnectionConfig) => {
  await createS3Client(config).send(new HeadBucketCommand({ Bucket: config.bucket }))
}

export const listS3Directory = async (config: S3ConnectionConfig, relativePath = '/'): Promise<IAliGetFileModel[]> => {
  const client = createS3Client(config)
  const prefix = joinS3Key(config.rootPrefix, relativePath, relativePath !== '/')
  const directories = new Map<string, IAliGetFileModel>()
  const files = new Map<string, IAliGetFileModel>()
  let continuationToken: string | undefined
  do {
    const response = await client.send(new ListObjectsV2Command({ Bucket: config.bucket, Prefix: prefix, Delimiter: '/', ContinuationToken: continuationToken }))
    for (const item of response.CommonPrefixes || []) {
      if (!item.Prefix || item.Prefix === prefix) continue
      const model = toAliModel(config, item.Prefix, undefined, true)
      directories.set(model.file_id, model)
    }
    for (const item of response.Contents || []) {
      if (!item.Key || item.Key === prefix || item.Key.endsWith('/')) continue
      const model = toAliModel(config, item.Key, item, false)
      files.set(model.file_id, model)
    }
    continuationToken = response.IsTruncated ? response.NextContinuationToken : undefined
  } while (continuationToken)
  return [...directories.values(), ...files.values()]
}

export const getS3ObjectInfo = async (config: S3ConnectionConfig, relativePath: string) => {
  const key = joinS3Key(config.rootPrefix, relativePath)
  const client = createS3Client(config)
  try {
    const result = await client.send(new HeadObjectCommand({ Bucket: config.bucket, Key: key }))
    return { isDir: false, size: Number(result.ContentLength || 0), key }
  } catch {
    const directoryKey = joinS3Key(config.rootPrefix, relativePath, true)
    const result = await client.send(new ListObjectsV2Command({ Bucket: config.bucket, Prefix: directoryKey, MaxKeys: 1 }))
    if (result.Contents?.length) return { isDir: true, size: 0, key: directoryKey }
    throw new Error('S3 对象不存在')
  }
}

export const getS3DownloadUrl = async (config: S3ConnectionConfig, relativePath: string, expiresIn = 4 * 60 * 60) => {
  const client = createS3Client(config)
  return await getSignedUrl(client, new GetObjectCommand({ Bucket: config.bucket, Key: joinS3Key(config.rootPrefix, relativePath) }), { expiresIn })
}

export const createS3Directory = async (config: S3ConnectionConfig, relativePath: string) => {
  await createS3Client(config).send(new PutObjectCommand({ Bucket: config.bucket, Key: joinS3Key(config.rootPrefix, relativePath, true), Body: new Uint8Array() }))
}

const listAllS3Objects = async (config: S3ConnectionConfig, prefix: string) => {
  const client = createS3Client(config)
  const objects: S3Object[] = []
  let continuationToken: string | undefined
  do {
    const response = await client.send(new ListObjectsV2Command({ Bucket: config.bucket, Prefix: prefix, ContinuationToken: continuationToken }))
    objects.push(...(response.Contents || []))
    continuationToken = response.IsTruncated ? response.NextContinuationToken : undefined
  } while (continuationToken)
  return objects
}

const encodeCopySource = (bucket: string, key: string) => `${encodeURIComponent(bucket)}/${key.split('/').map(encodeURIComponent).join('/')}`

const normalizeS3TransferPaths = (sourcePath: string, targetPath: string) => {
  const source = normalizeS3RelativePath(sourcePath)
  const target = normalizeS3RelativePath(targetPath)
  if (source === '/' || target === '/') throw new Error('S3 根目录不能作为文件操作对象')
  if (source === target) throw new Error('S3 源路径与目标路径不能相同')
  if (target.startsWith(`${source}/`)) throw new Error('S3 目录不能复制或移动到自身的子目录')
  return { source, target }
}

export const copyS3Path = async (config: S3ConnectionConfig, sourcePath: string, targetPath: string) => {
  const { source, target } = normalizeS3TransferPaths(sourcePath, targetPath)
  const client = createS3Client(config)
  const info = await getS3ObjectInfo(config, source)
  if (!info.isDir) {
    const targetKey = joinS3Key(config.rootPrefix, target)
    await client.send(new CopyObjectCommand({ Bucket: config.bucket, CopySource: encodeCopySource(config.bucket, info.key), Key: targetKey }))
    return
  }
  const sourcePrefix = joinS3Key(config.rootPrefix, source, true)
  const targetPrefix = joinS3Key(config.rootPrefix, target, true)
  for (const object of await listAllS3Objects(config, sourcePrefix)) {
    if (!object.Key) continue
    const targetKey = `${targetPrefix}${object.Key.slice(sourcePrefix.length)}`
    await client.send(new CopyObjectCommand({ Bucket: config.bucket, CopySource: encodeCopySource(config.bucket, object.Key), Key: targetKey }))
  }
}

export const deleteS3Path = async (config: S3ConnectionConfig, relativePath: string) => {
  const client = createS3Client(config)
  const info = await getS3ObjectInfo(config, relativePath)
  if (!info.isDir) {
    await client.send(new DeleteObjectCommand({ Bucket: config.bucket, Key: info.key }))
    return
  }
  const objects = await listAllS3Objects(config, info.key)
  for (let index = 0; index < objects.length; index += 1000) {
    const batch = objects.slice(index, index + 1000).flatMap((item) => (item.Key ? [{ Key: item.Key }] : []))
    if (batch.length) await client.send(new DeleteObjectsCommand({ Bucket: config.bucket, Delete: { Objects: batch, Quiet: true } }))
  }
}

export const moveS3Path = async (config: S3ConnectionConfig, sourcePath: string, targetPath: string) => {
  await copyS3Path(config, sourcePath, targetPath)
  await deleteS3Path(config, sourcePath)
}

export const renameS3Path = async (config: S3ConnectionConfig, sourcePath: string, newName: string) => {
  const normalizedName = newName.trim()
  if (!normalizedName || normalizedName === '.' || normalizedName === '..' || /[\\/]/.test(normalizedName)) throw new Error('S3 名称不能为空或包含路径分隔符')
  const normalizedSource = normalizeS3RelativePath(sourcePath)
  const targetPath = normalizeS3RelativePath(`${path.posix.dirname(normalizedSource)}/${normalizedName}`)
  await moveS3Path(config, normalizedSource, targetPath)
  return targetPath
}

const uploadS3Entry = async (config: S3ConnectionConfig, localPath: string, remotePath: string) => {
  const stat = await fsPromises.stat(localPath)
  if (stat.isDirectory()) {
    await createS3Directory(config, remotePath)
    for (const child of await fsPromises.readdir(localPath)) await uploadS3Entry(config, path.join(localPath, child), normalizeS3RelativePath(`${remotePath}/${child}`))
    return
  }
  const client = createS3Client(config)
  const upload = new Upload({ client, params: { Bucket: config.bucket, Key: joinS3Key(config.rootPrefix, remotePath), Body: fs.createReadStream(localPath), ContentLength: stat.size } })
  await upload.done()
}

export const uploadS3LocalPaths = async (config: S3ConnectionConfig, parentRelativePath: string, localPaths: string[]) => {
  for (const localPath of localPaths) await uploadS3Entry(config, localPath, normalizeS3RelativePath(`${parentRelativePath}/${path.basename(localPath)}`))
}
