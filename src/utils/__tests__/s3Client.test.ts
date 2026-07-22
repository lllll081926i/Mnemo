import { beforeEach, describe, expect, it, vi } from 'vitest'
import { CopyObjectCommand, HeadObjectCommand, ListObjectsV2Command, PutObjectCommand, S3Client } from '@aws-sdk/client-s3'
import { copyS3Path, createS3Connection, createS3Directory, createS3UserToken, getS3Connections, isS3Drive, listS3Directory, moveS3Path, normalizeS3Prefix, normalizeS3RelativePath, renameS3Path, saveS3Connection, type S3ConnectionInput } from '../s3Client'
import { createClient } from 'webdav'
import { buildWebDavDownloadUrl, copyWebDavPath, createWebDavConnection, getWebDavConnections, getWebDavRequestHeaders, listWebDavDirectory, moveWebDavPath, normalizeWebDavPath, renameWebDavPath, saveWebDavConnection } from '../webdavClient'

vi.mock('webdav', () => ({ createClient: vi.fn() }))

const storage = new Map<string, string>()

const localStorageMock = {
  getItem: (key: string) => storage.get(key) ?? null,
  setItem: (key: string, value: string) => storage.set(key, value),
  removeItem: (key: string) => storage.delete(key),
  clear: () => storage.clear(),
  key: (index: number) => [...storage.keys()][index] ?? null,
  get length() {
    return storage.size
  }
}

Object.defineProperty(globalThis, 'localStorage', { value: localStorageMock, configurable: true })
Object.assign(globalThis, {
  WebSafeStorageEncryptSync: (value: string) => Buffer.from(value, 'utf8').toString('base64'),
  WebSafeStorageDecryptSync: (value: string) => Buffer.from(value, 'base64').toString('utf8')
})

const createInput = (name: string): S3ConnectionInput => ({
  name,
  endpoint: 'https://s3.example.com/',
  region: 'us-east-1',
  accessKeyId: 'access-key',
  secretAccessKey: 'secret-key',
  sessionToken: '',
  bucket: 'mnemo-test',
  rootPrefix: '/documents//',
  forcePathStyle: true
})

describe('S3 connection model', () => {
  beforeEach(() => {
    storage.clear()
    vi.restoreAllMocks()
  })

  it('normalizes relative paths and root prefixes', () => {
    expect(normalizeS3RelativePath('///folder\\child/')).toBe('/folder/child')
    expect(normalizeS3RelativePath('')).toBe('/')
    expect(normalizeS3Prefix('/folder//child/')).toBe('folder/child/')
  })

  it('requires an explicit connection name', () => {
    expect(() => createS3Connection(createInput('  '))).toThrow('请填写 S3 连接名称')
  })

  it('rejects duplicate names case-insensitively', () => {
    saveS3Connection(createS3Connection(createInput('主存储')))
    expect(() => saveS3Connection(createS3Connection(createInput('主存储')))).toThrow('已存在')
    saveS3Connection(createS3Connection(createInput('Archive')))
    expect(() => saveS3Connection(createS3Connection(createInput('archive')))).toThrow('已存在')
    expect(getS3Connections()).toHaveLength(2)
  })

  it('enforces the name invariant in the persistence layer', () => {
    const connection = createS3Connection(createInput('  归档存储  '))
    saveS3Connection(connection)
    expect(getS3Connections()[0].name).toBe('归档存储')
    expect(() => saveS3Connection({ ...connection, id: 'blank-name', name: '   ' })).toThrow('请填写 S3 连接名称')
    expect(() => saveS3Connection({ ...connection, id: 'duplicate-name', name: '归档存储' })).toThrow('已存在')
  })

  it('encrypts mounted-storage credentials at rest', () => {
    const s3 = createS3Connection(createInput('安全存储'))
    saveS3Connection(s3)
    const s3Raw = storage.get('Mnemo_S3Connections') || ''
    expect(s3Raw).not.toContain('access-key')
    expect(s3Raw).not.toContain('secret-key')
    expect(getS3Connections()[0]).toMatchObject({ accessKeyId: 'access-key', secretAccessKey: 'secret-key' })

    const webdav = createWebDavConnection({ name: 'WebDAV', url: 'https://dav.example.com', username: 'dav-user', password: 'dav-password' })
    saveWebDavConnection(webdav)
    const webdavRaw = storage.get('mnemo.webdav.connections') || ''
    expect(webdavRaw).not.toContain('dav-user')
    expect(webdavRaw).not.toContain('dav-password')
    expect(getWebDavConnections()[0]).toMatchObject({ username: 'dav-user', password: 'dav-password' })
  })

  it('does not load removed media-library WebDAV connection storage', () => {
    storage.set('MediaLibrary_WebDavConnections', JSON.stringify([{ id: 'legacy', name: 'Legacy', url: 'https://dav.example.com', username: 'legacy', password: 'secret', rootPath: '/', createdAt: '2026-01-01T00:00:00.000Z' }]))

    expect(getWebDavConnections()).toEqual([])
  })

  it('maps WebDAV paths beneath the configured endpoint and sends basic credentials', async () => {
    const connection = createWebDavConnection({ name: 'WebDAV', url: 'https://dav.example.com/dav/', username: 'dav-user', password: 'secret', rootPath: '/home' })
    const getDirectoryContents = vi.fn().mockResolvedValue([
      { filename: '/dav/home/projects', basename: 'projects', type: 'directory' },
      { filename: '/dav/home/projects/movie.mp4', basename: 'movie.mp4', type: 'file', size: 12, mime: 'video/mp4', lastmod: '2026-01-01T00:00:00.000Z' }
    ])
    vi.mocked(createClient).mockReturnValue({ getDirectoryContents } as any)

    const items = await listWebDavDirectory(connection, '/projects')

    expect(normalizeWebDavPath('/projects//archive/')).toBe('/projects/archive')
    expect(getDirectoryContents).toHaveBeenCalledWith('/home/projects')
    expect(items).toMatchObject([{ file_id: '/projects/movie.mp4', parent_file_id: '/projects', category: 'video' }])
    expect(getWebDavRequestHeaders(connection)).toEqual({ Authorization: 'Basic ZGF2LXVzZXI6c2VjcmV0' })
    expect(buildWebDavDownloadUrl(connection, '/projects/movie.mp4')).toBe('https://dav.example.com/dav/home/projects/movie.mp4')
  })

  it('creates isolated account and drive ids for every connection', () => {
    const first = createS3Connection(createInput('S3 A'))
    const second = createS3Connection(createInput('S3 B'))
    const firstToken = createS3UserToken(first)
    const secondToken = createS3UserToken(second)
    expect(firstToken.user_id).toBe(`s3:${first.id}`)
    expect(secondToken.user_id).toBe(`s3:${second.id}`)
    expect(firstToken.user_id).not.toBe(secondToken.user_id)
    expect(isS3Drive(firstToken.default_drive_id)).toBe(true)
  })

  it('maps S3 prefixes and objects into the unified file model', async () => {
    const connection = createS3Connection(createInput('列表测试'))
    vi.spyOn(S3Client.prototype, 'send').mockImplementation(async (command: any) => {
      expect(command).toBeInstanceOf(ListObjectsV2Command)
      expect(command.input.Prefix).toBe('documents/')
      return {
        CommonPrefixes: [{ Prefix: 'documents/folder/' }],
        Contents: [
          { Key: 'documents/', Size: 0 },
          { Key: 'documents/file.txt', Size: 12, LastModified: new Date('2026-01-01T00:00:00Z'), ETag: 'etag' }
        ],
        IsTruncated: false
      }
    })
    const items = await listS3Directory(connection, '/')
    expect(items.map((item) => [item.file_id, item.isDir])).toEqual([
      ['/folder', true],
      ['/file.txt', false]
    ])
    expect(items[1].size).toBe(12)
  })

  it('uses native S3 commands for directory creation and file copy', async () => {
    const connection = createS3Connection(createInput('操作测试'))
    const commands: any[] = []
    vi.spyOn(S3Client.prototype, 'send').mockImplementation(async (command: any) => {
      commands.push(command)
      if (command instanceof HeadObjectCommand) return { ContentLength: 10 }
      return {}
    })
    await createS3Directory(connection, '/new-folder')
    await copyS3Path(connection, '/source.txt', '/target.txt')
    expect(commands[0]).toBeInstanceOf(PutObjectCommand)
    expect(commands[0].input.Key).toBe('documents/new-folder/')
    expect(commands[2]).toBeInstanceOf(CopyObjectCommand)
    expect(commands[2].input.Key).toBe('documents/target.txt')
  })

  it('rejects destructive self moves and invalid rename paths before sending commands', async () => {
    const connection = createS3Connection(createInput('边界测试'))
    const send = vi.spyOn(S3Client.prototype, 'send')
    await expect(moveS3Path(connection, '/folder', '/folder')).rejects.toThrow('源路径与目标路径不能相同')
    await expect(copyS3Path(connection, '/folder', '/folder/child/folder')).rejects.toThrow('自身的子目录')
    await expect(renameS3Path(connection, '/folder/file.txt', '../other.txt')).rejects.toThrow('路径分隔符')
    expect(send).not.toHaveBeenCalled()
  })

  it('rejects destructive WebDAV self moves and path traversal before sending requests', async () => {
    const connection = createWebDavConnection({ name: 'WebDAV 边界', url: 'https://dav.example.com', username: '', password: '', rootPath: '/home' })
    const client = { copyFile: vi.fn(), moveFile: vi.fn() }
    vi.mocked(createClient).mockReturnValue(client as any)
    await expect(moveWebDavPath(connection, '/folder', '/folder')).rejects.toThrow('源路径与目标路径不能相同')
    await expect(copyWebDavPath(connection, '/folder', '/folder/child/folder')).rejects.toThrow('自身的子目录')
    await expect(renameWebDavPath(connection, '/folder/file.txt', '../other.txt')).rejects.toThrow('路径分隔符')
    expect(client.copyFile).not.toHaveBeenCalled()
    expect(client.moveFile).not.toHaveBeenCalled()
  })
})
