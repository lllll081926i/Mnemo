import crypto from 'crypto'
import { IAliGetDirModel, IAliGetFileModel } from '../aliapi/alimodels'
import AliFile from '../aliapi/file'
import { NewIAliFileResp } from '../aliapi/dirfilelist'
import { ITokenInfo, useFootStore, usePanFileStore } from '../store'
import TreeStore, { IDriverModel, TreeNodeData } from '../store/treestore'
import DB from '../utils/db'
import DebugLog from '../utils/debuglog'
import message from '../utils/message'
import usePanTreeStore from './pantreestore'
import { GetDriveID, GetDriveType, isPikPakUser } from '../aliapi/utils'
import { apiPikPakFileList, mapPikPakFileToAliModel } from '../pikpak/dirfilelist'
import { getWebDavConnection, getWebDavConnectionId, isWebDavDrive, listWebDavDirectory } from '../utils/webdavClient'
import { getS3Connection, getS3ConnectionId, isS3Drive, listS3Directory } from '../utils/s3Client'
import { OrderDir } from '../utils/filenameorder'
import { extFromFileName } from '../utils/filetype'
import getFileIcon from '../aliapi/fileicon'
import { isDriveProviderRootId, resolveDriveProvider } from '../utils/driveProvider'
import { apiOneDriveFileList, mapOneDriveItemToAliModel } from '../onedrive/dirfilelist'
import { apiDropboxFileList, mapDropboxFileToAliModel, resolveDropboxParentIdFromPath } from '../dropbox/dirfilelist'
import { apiGoogleDriveFileList, apiGoogleDriveSearch, apiGoogleDriveTrash, mapGoogleDriveItemToAliModel } from '../gdrive/dirfilelist'
import { apiGofileFileList, mapGofileItemToAliModel } from '../gofile/dirfilelist'
import { apiOneDriveSearch } from '../onedrive/search'
import { apiDropboxSearch } from '../dropbox/search'

export interface PanSelectedData {
  isError: boolean
  isErrorSelected: boolean
  user_id: string
  drive_id: string
  dirID: string
  albumId: string
  parentDirID: string
  fileDescription: string
  parentDirDescription: string
  selectedKeys: string[]
  selectedParentKeys: string[]
}

export interface QuickFileEntry {
  key: string
  drive_id: string
  drive_name: string
  title: string
  file_id?: string
  parent_file_id?: string
  kind?: 'folder' | 'file'
  icon?: string
  /** 文件扩展名（标签筛选视图里要能识别视频/音频直接播放） */
  ext?: string
  tag?: string
  tagColor?: string
  favorite?: boolean
}

/** 标签定义——独立于收藏/快捷方式，支持自定义名称与颜色 */
export interface FileTagDef {
  id: string
  name: string
  color: string
}

/** 文件-标签关联——记录哪个文件打了哪个标签，附带展示用元数据（标签筛选视图无需再请求云端） */
export interface FileTagLink {
  drive_id: string
  file_id: string
  kind: 'folder' | 'file'
  tagId: string
  title: string
  parent_file_id: string
  icon: string
  ext: string
}

const quickEntryKey = (kind: 'folder' | 'file', driveId: string, fileId: string) => `quick:${kind}:${encodeURIComponent(driveId)}:${encodeURIComponent(fileId)}`

const quickEntryIdentity = (entry: Pick<QuickFileEntry, 'drive_id' | 'file_id' | 'key' | 'kind'>) => `${entry.kind || 'folder'}|${entry.drive_id || ''}|${entry.file_id || entry.key || ''}`

const isLocalTagColorClass = (value: string) => /^c[0-9a-f]{3,8}$/i.test(value)

const removeLocalTagColorClasses = (description: string) => String(description || '')
  .split(/\s+/)
  .filter((token) => token && !isLocalTagColorClass(token))
  .join(' ')

/** 颜色统一成小写 hex（#df5659），供标签定义匹配使用 */
const normalizeTagColor = (color: string) => {
  const value = String(color || '').trim().toLowerCase()
  if (!value) return ''
  return value.startsWith('#') ? value : `#${value.replace(/^c/, '')}`
}

const tagLinkIdentity = (link: Pick<FileTagLink, 'kind' | 'drive_id' | 'file_id'>) => `${link.kind}|${link.drive_id}|${link.file_id}`

const readJsonArray = <T>(key: string): T[] => {
  if (typeof localStorage === 'undefined') return []
  try {
    const raw = localStorage.getItem(key)
    const parsed = raw ? JSON.parse(raw) : []
    return Array.isArray(parsed) ? parsed : []
  } catch {
    return []
  }
}

const writeJsonArray = (key: string, list: unknown[]) => {
  if (typeof localStorage === 'undefined') return
  localStorage.setItem(key, JSON.stringify(list))
}

// 标签数据的 localStorage schema 版本：将来字段结构变更时升级版本号即可整体迁移/作废旧数据，避免读到不兼容的旧结构
const TAG_STORAGE_VERSION = 'v1'
const getTagDefsKey = (userId: string) => `FileTags-${TAG_STORAGE_VERSION}-${userId}`
const getTagLinksKey = (userId: string) => `FileTagLinks-${TAG_STORAGE_VERSION}-${userId}`

/** 生成防碰撞的标签 ID（crypto.randomUUID 为密码学随机，杜绝 Date.now()+Math.random 在高并发打标签时的碰撞风险） */
const generateTagId = (): string => crypto.randomUUID()

const RefreshLock = new Set<string>()
export default class PanDAL {
  static async aReLoadProviderDrive(token: ITokenInfo): Promise<void> {
    const provider = resolveDriveProvider(token)
    if (provider !== 'onedrive' && provider !== 'dropbox' && provider !== 'gdrive' && provider !== 'gofile') return
    const { user_id } = token
    const drive_id = token.default_drive_id
    if (!user_id || !drive_id) return
    const rootKey = provider === 'onedrive' ? 'onedrive_root' : provider === 'dropbox' ? 'dropbox_root' : provider === 'gdrive' ? 'gdrive_root' : 'gofile_root'
    const pantreeStore = usePanTreeStore()
    pantreeStore.mSaveUser(user_id, drive_id, '', '', '')
    pantreeStore.drive_id = drive_id
    useFootStore().mSaveLoading(`加载 ${GetDriveType(user_id, drive_id).title} 文件夹...`)
    try {
      const items = provider === 'onedrive'
        ? (await apiOneDriveFileList(user_id, rootKey)).map((item) => mapOneDriveItemToAliModel(item, drive_id, rootKey))
        : provider === 'dropbox'
          ? (await apiDropboxFileList(user_id, rootKey)).map((item) => mapDropboxFileToAliModel(item, drive_id, rootKey))
          : provider === 'gdrive'
            ? (await apiGoogleDriveFileList(user_id, rootKey)).map((item) => mapGoogleDriveItemToAliModel(item, drive_id, rootKey))
            : (await apiGofileFileList(user_id, rootKey)).map((item) => mapGofileItemToAliModel(item, drive_id, rootKey))
      const dirs = items.filter((item) => item.isDir).map((item) => ({ file_id: item.file_id, drive_id, parent_file_id: rootKey, name: item.name, description: item.description || '', time: item.time, size: 0 }))
      await TreeStore.ConvertToOneDriver(user_id, drive_id, dirs, false, true)
      PanDAL.RefreshPanTreeAllNode(drive_id)
    } finally {
      useFootStore().mSaveLoading('')
    }
  }

  static async aReLoadPikPakDrive(token: ITokenInfo): Promise<void> {
    const { user_id } = token
    const drive_id = token.default_drive_id || 'pikpak'
    const pantreeStore = usePanTreeStore()
    pantreeStore.mSaveUser(user_id, drive_id, '', '', '')
    pantreeStore.drive_id = drive_id
    if (!user_id) return
    useFootStore().mSaveLoading('加载 PikPak 文件夹...')
    const { items } = await apiPikPakFileList(user_id, 'pikpak_root', 100)
    const driveType = GetDriveType(user_id, drive_id)
    const dirs = items
      .filter((item) => (item.kind || '').includes('folder'))
      .map((item) => ({
        file_id: String(item.id),
        drive_id: drive_id,
        parent_file_id: driveType.key,
        name: item.name,
        description: '',
        time: new Date(item.modified_time || item.created_time || '').getTime() || 0,
        size: 0
      }))
    await TreeStore.ConvertToOneDriver(user_id, drive_id, dirs, false, true)
    PanDAL.RefreshPanTreeAllNode(drive_id)
    useFootStore().mSaveLoading('')
  }

  static async aReLoadWebDavDrive(token: ITokenInfo): Promise<void> {
    const { user_id } = token
    const drive_id = token.default_drive_id || user_id
    const connection = getWebDavConnection(getWebDavConnectionId(drive_id))
    const pantreeStore = usePanTreeStore()
    pantreeStore.mSaveUser(user_id, drive_id, '', '', '')
    pantreeStore.drive_id = drive_id
    if (!user_id || !connection) return

    useFootStore().mSaveLoading(`加载 ${connection.name} 文件夹...`)
    try {
      const list = await listWebDavDirectory(connection, '/')
      const driveType = GetDriveType(user_id, drive_id)
      const dirs = list
        .filter((item) => item.isDir)
        .map((item) => ({
          file_id: item.file_id,
          drive_id,
          parent_file_id: driveType.key,
          path: item.path,
          name: item.name,
          description: '',
          time: item.time,
          size: 0
        }))
      await TreeStore.ConvertToOneDriver(user_id, drive_id, dirs, false, true)
      PanDAL.RefreshPanTreeAllNode(drive_id)
    } finally {
      useFootStore().mSaveLoading('')
    }
  }

  static async aReLoadS3Drive(token: ITokenInfo): Promise<void> {
    const { user_id } = token
    const drive_id = token.default_drive_id || user_id
    const connection = getS3Connection(getS3ConnectionId(drive_id))
    const pantreeStore = usePanTreeStore()
    pantreeStore.mSaveUser(user_id, drive_id, '', '', '')
    pantreeStore.drive_id = drive_id
    if (!user_id || !connection) return

    useFootStore().mSaveLoading(`加载 ${connection.name} 文件夹...`)
    try {
      const list = await listS3Directory(connection, '/')
      const driveType = GetDriveType(user_id, drive_id)
      const dirs = list.filter((item) => item.isDir).map((item) => ({ file_id: item.file_id, drive_id, parent_file_id: driveType.key, path: item.path, name: item.name, description: '', time: item.time, size: 0 }))
      await TreeStore.ConvertToOneDriver(user_id, drive_id, dirs, false, true)
      PanDAL.RefreshPanTreeAllNode(drive_id)
    } finally {
      useFootStore().mSaveLoading('')
    }
  }

  static RefreshPanTreeAllNode(drive_id: string) {
    const OneDriver = TreeStore.GetDriver(drive_id)
    if (!OneDriver) return
    const pantreeStore = usePanTreeStore()
    const driveType = GetDriveType(usePanTreeStore().user_id, drive_id)
    const dir: TreeNodeData = {
      __v_skip: true,
      key: driveType.key,
      drive_id: drive_id,
      parent_file_id: '',
      title: driveType.title,
      namesearch: '',
      children: []
    }
    const expandedKeys = new Set(usePanTreeStore().treeExpandedKeys)
    const map = new Map<string, TreeNodeData>()
    TreeStore.GetTreeDataToShow(OneDriver, dir, expandedKeys, map, true)
    map.set(dir.key, dir)
    pantreeStore.mSaveTreeAllNode(OneDriver.drive_id, dir, map)
  }

  static GetPanTreeAllNode(user_id: string, drive_id: string, treeExpandedKeys: string[], getChildren: boolean = true, isLeafForce: boolean = false): TreeNodeData[] {
    const driveType = GetDriveType(user_id, drive_id)
    const dir: TreeNodeData = {
      __v_skip: true,
      title: driveType.title,
      drive_id: drive_id,
      parent_file_id: '',
      namesearch: '',
      key: driveType.key,
      children: []
    }
    const OneDriver = TreeStore.GetDriver(drive_id)
    if (!OneDriver) return [dir]
    const expandedKeys = new Set(treeExpandedKeys)
    const map = new Map<string, TreeNodeData>()
    TreeStore.GetTreeDataToShow(OneDriver, dir, expandedKeys, map, getChildren, '', isLeafForce)
    map.set(dir.key, dir)
    return [dir]
  }

  static aTreeScrollToDir(dirID: string) {
    usePanTreeStore().mSaveTreeScrollTo(dirID)
    usePanFileStore().mSaveFileScrollTo(dirID)
  }

  static async aReLoadOneDirToShow(drive_id: string, file_id: string, selfExpand: boolean, album_id: string = ''): Promise<boolean> {
    const panTreeStore = usePanTreeStore()
    const user_id = panTreeStore.user_id
    const isBack = file_id == 'back'
    if (!drive_id) {
      drive_id = GetDriveID(user_id, file_id) || panTreeStore.drive_id
    }
    const driveType = GetDriveType(user_id, drive_id)
    panTreeStore.drive_id = drive_id
    if (file_id == 'refresh') {
      file_id = panTreeStore.selectDir.file_id
    }
    if (isBack) {
      if (panTreeStore.History.length > 0) {
        panTreeStore.History.shift()
        if (panTreeStore.History.length > 0) {
          drive_id = panTreeStore.History[0].drive_id
          file_id = panTreeStore.History[0].file_id
        }
      }
      if (file_id == 'back') {
        file_id = driveType.key
        panTreeStore.History = []
      }
      if (file_id.includes('pic')) {
        panTreeStore.selectDir.album_type = file_id
      } else {
        panTreeStore.selectDir.album_type = 'pic_root'
        panTreeStore.selectDir.album_id = ''
      }
    }
    let dir = TreeStore.GetDir(drive_id, file_id)
    let dirPath = TreeStore.GetDirPath(drive_id, file_id)
    const isRoot = isDriveProviderRootId({ userId: user_id, driveId: drive_id }, file_id) || file_id === driveType.key
    if (!dir || (dirPath.length == 0 && !isRoot)) {
      if (isRoot) {
        const driveType = GetDriveType(user_id, drive_id)
        dir = {
          __v_skip: true,
          file_id: file_id,
          drive_id: drive_id,
          parent_file_id: '',
          name: driveType.title || '根目录',
          namesearch: '',
          description: '',
          time: 0,
          size: 0
        } as IAliGetDirModel
        dirPath = [dir]
      } else {
        let findPath: IAliGetDirModel[] = []
        if (!album_id) {
          findPath = await AliFile.ApiFileGetPath(panTreeStore.user_id, drive_id, file_id)
        }
        if (findPath.length > 0) {
          dirPath = findPath
          dir = { ...dirPath[dirPath.length - 1] }
        }
      }
    }
    if (!dir || (dirPath.length == 0 && !isRoot)) {
      message.error('出错，找不到指定的文件夹 ' + file_id)
      return false
    }
    // 记录跳转历史
    if (!isBack && panTreeStore.selectDir.file_id != dir.file_id) {
      const history: IAliGetDirModel[] = [dir]
      for (let i = 0, maxi = panTreeStore.History.length; i < maxi; i++) {
        history.push(panTreeStore.History[i])
        if (history.length >= 50) break
      }
      panTreeStore.History = history
    }
    // 展开列表节点
    const treeExpandedKeys = new Set(panTreeStore.treeExpandedKeys)
    for (let i = 0, maxi = dirPath.length - 1; i < maxi; i++) {
      treeExpandedKeys.add(dirPath[i].file_id)
    }
    if (selfExpand) {
      treeExpandedKeys.add(dir.file_id)
    }
    panTreeStore.mShowDir(dir, dirPath, [dir.file_id], Array.from(treeExpandedKeys))
    // console.warn('selectDir', panTreeStore.selectDir)
    PanDAL.RefreshPanTreeAllNode(drive_id)
    const panfileStore = usePanFileStore()
    if (panfileStore.ListLoading && panfileStore.DriveID == drive_id && panfileStore.DirID == dir.file_id) {
      return false
    }
    panfileStore.mSaveDirFileLoading(drive_id, dir.file_id, dir.name, dir.album_id)
    return PanDAL.GetDirFileList(panTreeStore.user_id, dir.drive_id, dir.file_id, dir.name, dir.album_id)
  }

  static GetDirFileList(user_id: string, drive_id: string, dirID: string, dirName: string, albumID: string = '', hasFiles: boolean = true): Promise<boolean> {
    return new Promise<boolean>((resolve) => {
      if (dirID == 'search') {
        if (hasFiles) {
          usePanFileStore().mSaveDirFileLoadingFinish(drive_id, dirID, [])
        }
        resolve(true)
        return
      }

      // 本地标签筛选：标记数据存在独立的标签模型里，所有网盘共用
      if (dirID.startsWith('color')) {
        this.migrateLegacyTags(user_id)
        const colorKey = dirID.slice('color'.length).trim().split(' ')[0].toLowerCase()
        const hexColor = colorKey.startsWith('c') ? `#${colorKey.slice(1)}` : colorKey
        const defs = this.readTagDefs(user_id)
        const matchTagIds = new Set(defs.filter((def) => normalizeTagColor(def.color) === normalizeTagColor(hexColor)).map((def) => def.id))
        const tagged = this.readTagLinks(user_id).filter((link) => matchTagIds.has(link.tagId))
        const items: IAliGetFileModel[] = tagged.map((link) => {
          const isFile = link.kind === 'file'
          // 标签视图里的文件要保留真实类型信息，否则视频/音频在筛选视图里无法直接播放
          const ext = isFile ? link.ext || extFromFileName(link.title) : ''
          const iconInfo = isFile ? getFileIcon('', ext, ext, '', 0) : ['folder', link.icon || 'iconfile-folder']
          return {
            __v_skip: true,
            drive_id: link.drive_id,
            file_id: link.file_id,
            parent_file_id: link.parent_file_id || '',
            name: link.title,
            namesearch: link.title,
            path: '',
            ext,
            mime_type: '',
            mime_extension: ext,
            category: isFile ? iconInfo[0] : 'folder',
            icon: link.icon || iconInfo[1] || (isFile ? 'iconwenjian' : 'iconfile-folder'),
            size: 0,
            sizeStr: '',
            time: 0,
            timeStr: '',
            starred: false,
            isDir: !isFile,
            thumbnail: '',
            description: hexColor.replace('#', 'c')
          }
        })
        const dir = NewIAliFileResp(user_id, drive_id, dirID, dirName)
        dir.items = hasFiles ? items : items.filter((item) => item.isDir)
        dir.itemsKey = new Set(dir.items.map((item) => item.file_id))
        dir.next_marker = ''
        dir.itemsTotal = dir.items.length
        const panfileStore = usePanFileStore()
        panfileStore.mSaveDirFileLoadingPart(0, dir, dir.itemsTotal)
        TreeStore.SaveOneDirFileList(dir, hasFiles).then(() => {
          if (hasFiles) panfileStore.mSaveDirFileLoadingFinish(drive_id, dirID, dir.items, dir.itemsTotal)
          resolve(true)
        })
        return
      }

      const provider = resolveDriveProvider({ userId: user_id, driveId: drive_id })
      if (provider === 'onedrive' || provider === 'dropbox' || provider === 'gdrive' || provider === 'gofile') {
        const rootKey = provider === 'onedrive' ? 'onedrive_root' : provider === 'dropbox' ? 'dropbox_root' : provider === 'gdrive' ? 'gdrive_root' : 'gofile_root'
        const isSearch = dirID.startsWith('search')
        const keyword = isSearch ? dirID.slice('search'.length).trim() : ''
        const isTrash = provider === 'gdrive' && dirID === 'trash'
        const loadItems = async () => {
          if (provider === 'onedrive') {
            const list = isSearch ? await apiOneDriveSearch(user_id, keyword) : await apiOneDriveFileList(user_id, dirID)
            return list.map((item) => mapOneDriveItemToAliModel(item, drive_id, isSearch ? item.parentReference?.id || rootKey : dirID))
          }
          if (provider === 'dropbox') {
            const list = isSearch ? await apiDropboxSearch(user_id, keyword) : await apiDropboxFileList(user_id, dirID)
            return list.map((item) => mapDropboxFileToAliModel(item, drive_id, isSearch ? resolveDropboxParentIdFromPath(item.path_lower || item.path_display) : dirID))
          }
          if (provider === 'gdrive') {
            const list = isSearch ? await apiGoogleDriveSearch(user_id, keyword) : isTrash ? await apiGoogleDriveTrash(user_id) : await apiGoogleDriveFileList(user_id, dirID)
            return list.map((item) => mapGoogleDriveItemToAliModel(item, drive_id, isSearch || isTrash ? item.parents?.[0] || rootKey : dirID))
          }
          return (await apiGofileFileList(user_id, dirID)).map((item) => mapGofileItemToAliModel(item, drive_id, dirID))
        }
        loadItems()
          .then(async (allItems) => {
            this.applyLocalQuickTags(user_id, allItems)
            // 正常目录（非搜索/回收站）拿到的是完整列表，可据此清掉该目录下已失效的孤儿标签
            if (!isSearch && !isTrash) this.pruneOrphanedTagLinks(drive_id, dirID, new Set(allItems.map((item) => `${item.drive_id}:${item.file_id}`)))
            const items = hasFiles ? allItems : allItems.filter((item) => item.isDir)
            const dir = NewIAliFileResp(user_id, drive_id, dirID, dirName || GetDriveType(user_id, drive_id).title)
            dir.items = items
            dir.itemsKey = new Set(items.map((item) => item.file_id))
            dir.next_marker = ''
            dir.itemsTotal = items.length
            const panfileStore = usePanFileStore()
            panfileStore.mSaveDirFileLoadingPart(0, dir, dir.itemsTotal)
            if (!TreeStore.GetDriver(drive_id)) await TreeStore.ConvertToOneDriver(user_id, drive_id, [], false, true)
            await TreeStore.SaveOneDirFileList(dir, hasFiles)
            if (hasFiles) panfileStore.mSaveDirFileLoadingFinish(drive_id, dirID, dir.items, dir.itemsTotal)
            PanDAL.RefreshPanTreeAllNode(drive_id)
            resolve(true)
          })
          .catch((err: any) => {
            if (hasFiles) usePanFileStore().mSaveDirFileLoadingFinish(drive_id, dirID, [])
            message.warning(`无法读取 ${GetDriveType(user_id, drive_id).title} 文件夹，请检查网络连接后重试`)
            resolve(false)
          })
        return
      }

      if (isWebDavDrive(drive_id)) {
        const connectionId = getWebDavConnectionId(drive_id)
        const connection = getWebDavConnection(connectionId)
        if (!connection) {
          if (hasFiles) usePanFileStore().mSaveDirFileLoadingFinish(drive_id, dirID, [])
          message.warning('WebDAV 连接不存在，请重新连接')
          resolve(false)
          return
        }
        const requestPath = dirID === '/' ? '/' : dirID
        listWebDavDirectory(connection, requestPath)
          .then(async (allItems) => {
            this.applyLocalQuickTags(user_id, allItems)
            this.pruneOrphanedTagLinks(drive_id, requestPath, new Set(allItems.map((item) => `${item.drive_id}:${item.file_id}`)))
            const items = hasFiles ? allItems : allItems.filter((item) => item.isDir)
            const dir = NewIAliFileResp(user_id, drive_id, dirID, dirName || (dirID === '/' ? connection.name : dirID.split('/').pop() || connection.name))
            dir.items = items
            dir.itemsKey = new Set(items.map((item) => item.file_id))
            dir.next_marker = ''
            dir.itemsTotal = items.length
            const panfileStore = usePanFileStore()
            panfileStore.mSaveDirFileLoadingPart(0, dir, dir.itemsTotal || 0)
            if (!TreeStore.GetDriver(drive_id)) {
              await TreeStore.ConvertToOneDriver(user_id, drive_id, [], false, true)
            }
            await TreeStore.SaveOneDirFileList(dir, hasFiles)
            if (hasFiles) {
              panfileStore.mSaveDirFileLoadingFinish(drive_id, dirID, dir.items, dir.itemsTotal || 0)
            }
            PanDAL.RefreshPanTreeAllNode(drive_id)
            resolve(true)
          })
          .catch((err: any) => {
            if (hasFiles) usePanFileStore().mSaveDirFileLoadingFinish(drive_id, dirID, [])
            message.warning('无法读取 WebDAV 文件夹，请检查服务器连接后重试')
            resolve(false)
          })
        return
      }

      if (isS3Drive(drive_id)) {
        const connection = getS3Connection(getS3ConnectionId(drive_id))
        if (!connection) {
          if (hasFiles) usePanFileStore().mSaveDirFileLoadingFinish(drive_id, dirID, [])
          message.warning('S3 连接不存在，请重新连接')
          resolve(false)
          return
        }
        const requestPath = dirID === '/' ? '/' : dirID
        listS3Directory(connection, requestPath)
          .then(async (allItems) => {
            this.applyLocalQuickTags(user_id, allItems)
            this.pruneOrphanedTagLinks(drive_id, requestPath, new Set(allItems.map((item) => `${item.drive_id}:${item.file_id}`)))
            const items = hasFiles ? allItems : allItems.filter((item) => item.isDir)
            const dir = NewIAliFileResp(user_id, drive_id, dirID, dirName || (dirID === '/' ? connection.name : dirID.split('/').pop() || connection.name))
            dir.items = items
            dir.itemsKey = new Set(items.map((item) => item.file_id))
            dir.next_marker = ''
            dir.itemsTotal = items.length
            const panfileStore = usePanFileStore()
            panfileStore.mSaveDirFileLoadingPart(0, dir, dir.itemsTotal || 0)
            if (!TreeStore.GetDriver(drive_id)) await TreeStore.ConvertToOneDriver(user_id, drive_id, [], false, true)
            await TreeStore.SaveOneDirFileList(dir, hasFiles)
            if (hasFiles) panfileStore.mSaveDirFileLoadingFinish(drive_id, dirID, dir.items, dir.itemsTotal || 0)
            PanDAL.RefreshPanTreeAllNode(drive_id)
            resolve(true)
          })
          .catch((err: any) => {
            if (hasFiles) usePanFileStore().mSaveDirFileLoadingFinish(drive_id, dirID, [])
            message.warning('无法读取 S3 文件夹，请检查存储连接后重试')
            resolve(false)
          })
        return
      }

      if (isPikPakUser(user_id)) {
        const isTrash = dirID === 'trash'
        const isSearch = dirID.startsWith('search')
        if (isSearch) {
          if (hasFiles) usePanFileStore().mSaveDirFileLoadingFinish(drive_id, dirID, [])
          message.warning('PikPak 暂不支持全盘搜索，请在当前文件夹使用快速筛选')
          resolve(true)
          return
        }
        const parentId = dirID === 'pikpak_root' || isTrash ? 'pikpak_root' : dirID
        apiPikPakFileList(user_id, parentId, 100, '', isTrash)
          .then(({ items: list }) => {
            const allItems = this.applyLocalQuickTags(user_id, list.map((item) => mapPikPakFileToAliModel(item, drive_id, parentId)))
            const items = hasFiles ? allItems : allItems.filter((item) => item.isDir)
            const order = TreeStore.GetDirOrder(drive_id, dirID).replace('ext ', 'updated_at ')
            const orders = order.split(' ')
            OrderDir(orders[0], orders[1], items)
            const dir = NewIAliFileResp(user_id, drive_id, dirID, dirName)
            dir.items = items
            dir.itemsKey = new Set(items.map((item) => item.file_id))
            dir.next_marker = ''
            dir.itemsTotal = items.length
            const panfileStore = usePanFileStore()
            panfileStore.mSaveDirFileLoadingPart(0, dir, dir.itemsTotal || 0)
            TreeStore.SaveOneDirFileList(dir, hasFiles).then(() => {
              if (hasFiles) {
                panfileStore.mSaveDirFileLoadingFinish(drive_id, dirID, dir.items, dir.itemsTotal || 0)
              }
              PanDAL.RefreshPanTreeAllNode(drive_id)
              resolve(true)
            })
          })
          .catch(() => {
            if (hasFiles) usePanFileStore().mSaveDirFileLoadingFinish(drive_id, dirID, [])
            resolve(false)
          })
        return
      }

      if (hasFiles) usePanFileStore().mSaveDirFileLoadingFinish(drive_id, dirID, [])
      message.warning('当前网盘无法打开这个目录')
      resolve(false)
    })
  }

  static aReLoadOneDirToRefreshTree(user_id: string, drive_id: string, dirID: string, albumID?: string): Promise<boolean> {
    return new Promise<boolean>((resolve) => {
      if (dirID == 'favorite' || dirID.startsWith('color') || dirID.startsWith('search') || dirID.startsWith('video')) {
        resolve(true)
        return
      }
      if (RefreshLock.has(dirID)) {
        resolve(true)
        return
      }
      RefreshLock.add(dirID)
      PanDAL.GetDirFileList(user_id, drive_id, dirID, '', albumID, false)
        .then((success) => {
          if (!success) {
            RefreshLock.delete(dirID)
            resolve(false)
            return
          }
          PanDAL.RefreshPanTreeAllNode(drive_id)
          const pantreeStore = usePanTreeStore()
          if (pantreeStore.selectDir.drive_id == drive_id && pantreeStore.selectDir.file_id == dirID) {
            PanDAL.aReLoadOneDirToShow(drive_id, dirID, false, albumID).then(() => {
              RefreshLock.delete(dirID)
              resolve(true)
            })
          } else {
            RefreshLock.delete(dirID)
            resolve(true)
          }
        })
        .catch((err: any) => {
          DebugLog.mSaveWarning('列出文件夹失败file_id=' + dirID, err)
          RefreshLock.delete(dirID)
          resolve(false)
        })
    })
  }

  static GetPanSelectedData(istree: boolean): PanSelectedData {
    const panTreeStore = usePanTreeStore()
    const panFileStore = usePanFileStore()
    const data: PanSelectedData = {
      isError: false,
      isErrorSelected: false,
      user_id: panTreeStore.user_id,
      drive_id: panTreeStore.drive_id,
      dirID: panTreeStore.selectDir.file_id,
      albumId: panTreeStore.selectDir.album_id || '',
      parentDirID: panTreeStore.selectDir.parent_file_id,
      selectedKeys: istree ? [panTreeStore.selectDir.file_id] : panFileStore.GetSelectedID(),
      selectedParentKeys: istree ? [panTreeStore.selectDir.parent_file_id] : panFileStore.GetSelectedParentDirID(),
      fileDescription: panFileStore.GetSelectedFirst()?.description || '',
      parentDirDescription: panTreeStore.selectDir.description
    }

    data.isError = !data.user_id || !data.drive_id || !data.dirID
    data.isErrorSelected = data.selectedKeys.length == 0
    return data
  }

  private static readQuickFileList(userId: string): QuickFileEntry[] {
    if (!userId || typeof localStorage === 'undefined') return []
    try {
      const jsonstr = localStorage.getItem('FileQuick-' + userId)
      const raw = jsonstr ? JSON.parse(jsonstr) : []
      if (!Array.isArray(raw)) return []
      return raw
        .filter((item) => item && item.drive_id && (item.file_id || item.key))
        .map((item): QuickFileEntry => {
          const kind: 'folder' | 'file' = item.kind === 'file' ? 'file' : 'folder'
          const fileId = String(item.file_id || item.key)
          return {
            key: String(item.key || quickEntryKey(kind, String(item.drive_id), fileId)),
            drive_id: String(item.drive_id),
            drive_name: String(item.drive_name || ''),
            title: String(item.title || item.name || fileId),
            file_id: fileId,
            parent_file_id: String(item.parent_file_id || ''),
            kind,
            icon: String(item.icon || (kind === 'file' ? 'iconwenjian' : 'iconfile-folder')),
            ext: String(item.ext || ''),
            tag: String(item.tag || ''),
            tagColor: String(item.tagColor || ''),
            favorite: item.favorite !== false
          }
        })
    } catch {
      return []
    }
  }

  private static saveQuickFileList(userId: string, list: QuickFileEntry[]) {
    if (!userId || typeof localStorage === 'undefined') return
    localStorage.setItem('FileQuick-' + userId, JSON.stringify(list))
    usePanTreeStore().mSaveQuick(list)
  }

  static updateQuickFile(list: QuickFileEntry[]) {
    if (list.length == 0) return
    const pantreeStore = usePanTreeStore()
    const arr = this.readQuickFileList(pantreeStore.user_id)
    const indexMap = new Map(arr.map((item, index) => [quickEntryIdentity(item), index]))
    for (const input of list) {
      const kind: 'folder' | 'file' = input.kind === 'file' ? 'file' : 'folder'
      const fileId = String(input.file_id || input.key || '')
      if (!fileId || !input.drive_id) continue
      const normalized: QuickFileEntry = {
        key: input.key || quickEntryKey(kind, input.drive_id, fileId),
        drive_id: input.drive_id,
        drive_name: input.drive_name || '',
        title: input.title || fileId,
        file_id: fileId,
        parent_file_id: input.parent_file_id || '',
        kind,
        icon: input.icon || (kind === 'file' ? 'iconwenjian' : 'iconfile-folder'),
        ext: input.ext || '',
        tag: input.tag || '',
        tagColor: input.tagColor || '',
        favorite: input.favorite !== false
      }
      const identity = quickEntryIdentity(normalized)
      const existingIndex = indexMap.get(identity)
      if (existingIndex === undefined) {
        indexMap.set(identity, arr.length)
        arr.push(normalized)
      } else {
        arr[existingIndex] = {
          ...arr[existingIndex],
          ...normalized,
          tag: normalized.tag || arr[existingIndex].tag,
          tagColor: normalized.tagColor || arr[existingIndex].tagColor,
          ext: normalized.ext || arr[existingIndex].ext || '',
          favorite: normalized.favorite || arr[existingIndex].favorite
        }
      }
    }
    this.saveQuickFileList(pantreeStore.user_id, arr)
  }

  static addLocalQuickFiles(files: IAliGetFileModel[], tag = '', tagColor = '') {
    const pantreeStore = usePanTreeStore()
    const driveName = (driveId: string) => GetDriveType(pantreeStore.user_id, driveId).title
    this.updateQuickFile(files.map((file): QuickFileEntry => ({
      key: quickEntryKey(file.isDir ? 'folder' : 'file', file.drive_id, file.file_id),
      drive_id: file.drive_id,
      drive_name: driveName(file.drive_id),
      title: file.name,
      file_id: file.file_id,
      parent_file_id: file.parent_file_id,
      kind: file.isDir ? 'folder' : 'file',
      icon: file.icon,
      ext: file.ext || '',
      tag,
      tagColor,
      favorite: true
    })))
  }

  static removeLocalQuickFiles(files: IAliGetFileModel[]) {
    const pantreeStore = usePanTreeStore()
    const list = this.readQuickFileList(pantreeStore.user_id)
    const identities = new Set(files.map((file) => `${file.isDir ? 'folder' : 'file'}|${file.drive_id}|${file.file_id}`))
    const next = list
      .map((item) => identities.has(quickEntryIdentity(item)) ? { ...item, favorite: false } : item)
      .filter((item) => item.favorite || item.tag)
    this.saveQuickFileList(pantreeStore.user_id, next)
  }

  // ===== 本地标签：独立的数据模型（标签定义 + 文件关联），与收藏/快捷方式解耦，所有网盘通用 =====

  // 标签数据的模块级缓存：以 localStorage 原始串为失效依据——原始串变化（切换账号、外部写入、存储被清空）时自动重读，
  // 因此无需在账号切换处手动清缓存，也能避免高频读取（列表渲染逐行取标签）反复 JSON.parse
  private static tagDefsCache = new Map<string, { raw: string; list: FileTagDef[] }>()
  private static tagLinksCache = new Map<string, { raw: string; list: FileTagLink[] }>()

  /** 首次使用版本化 key 时，把旧版无版本 key 的数据整体搬移过来，保证升级后标签不丢失 */
  private static migratedTagKeys = new Set<string>()
  private static migrateLegacyTagStorageKey(oldKey: string, newKey: string) {
    if (this.migratedTagKeys.has(newKey)) return
    this.migratedTagKeys.add(newKey)
    if (typeof localStorage === 'undefined') return
    if (localStorage.getItem(newKey) !== null) return
    const legacy = localStorage.getItem(oldKey)
    if (legacy === null) return
    localStorage.setItem(newKey, legacy)
    localStorage.removeItem(oldKey)
  }

  private static readTagDefs(userId: string): FileTagDef[] {
    if (!userId || typeof localStorage === 'undefined') return []
    const key = getTagDefsKey(userId)
    this.migrateLegacyTagStorageKey('FileTags-' + userId, key)
    const raw = localStorage.getItem(key) ?? ''
    const cached = this.tagDefsCache.get(userId)
    if (cached && cached.raw === raw) return cached.list
    const list = readJsonArray<FileTagDef>(key).filter((item) => item && item.id && item.color)
    this.tagDefsCache.set(userId, { raw, list })
    return list
  }

  private static writeTagDefs(userId: string, defs: FileTagDef[]) {
    if (!userId || typeof localStorage === 'undefined') return
    const key = getTagDefsKey(userId)
    const raw = JSON.stringify(defs)
    localStorage.setItem(key, raw)
    this.tagDefsCache.set(userId, { raw, list: defs })
  }

  private static readTagLinks(userId: string): FileTagLink[] {
    if (!userId || typeof localStorage === 'undefined') return []
    const key = getTagLinksKey(userId)
    this.migrateLegacyTagStorageKey('FileTagLinks-' + userId, key)
    const raw = localStorage.getItem(key) ?? ''
    const cached = this.tagLinksCache.get(userId)
    if (cached && cached.raw === raw) return cached.list
    const list = readJsonArray<FileTagLink>(key).filter((item) => item && item.drive_id && item.file_id && item.tagId)
    this.tagLinksCache.set(userId, { raw, list })
    return list
  }

  private static writeTagLinks(userId: string, links: FileTagLink[]) {
    if (!userId || typeof localStorage === 'undefined') return
    const key = getTagLinksKey(userId)
    const raw = JSON.stringify(links)
    localStorage.setItem(key, raw)
    this.tagLinksCache.set(userId, { raw, list: links })
  }

  static getTagDefs(): FileTagDef[] {
    return this.readTagDefs(usePanTreeStore().user_id)
  }

  static saveTagDefs(defs: FileTagDef[]) {
    this.writeTagDefs(usePanTreeStore().user_id, defs)
  }

  static getTagLinks(): FileTagLink[] {
    return this.readTagLinks(usePanTreeStore().user_id)
  }

  static saveTagLinks(links: FileTagLink[]) {
    this.writeTagLinks(usePanTreeStore().user_id, links)
  }

  static createTag(name: string, color: string): FileTagDef {
    const userId = usePanTreeStore().user_id
    const defs = this.readTagDefs(userId)
    const def: FileTagDef = { id: generateTagId(), name: name || color, color: normalizeTagColor(color) }
    defs.push(def)
    this.writeTagDefs(userId, defs)
    return def
  }

  /** 删除标签定义，同时清掉所有引用它的文件关联 */
  static deleteTag(tagId: string) {
    const userId = usePanTreeStore().user_id
    this.writeTagDefs(userId, this.readTagDefs(userId).filter((def) => def.id !== tagId))
    this.writeTagLinks(userId, this.readTagLinks(userId).filter((link) => link.tagId !== tagId))
  }

  static renameTag(tagId: string, newName: string) {
    const userId = usePanTreeStore().user_id
    const defs = this.readTagDefs(userId).map((def) => (def.id === tagId ? { ...def, name: newName } : def))
    this.writeTagDefs(userId, defs)
  }

  /** 按颜色找到已有标签定义，没有就新建一个（保证同一颜色复用同一个标签） */
  private static ensureTagDefForColor(userId: string, name: string, color: string): FileTagDef {
    const normalized = normalizeTagColor(color)
    const defs = this.readTagDefs(userId)
    const existing = defs.find((def) => normalizeTagColor(def.color) === normalized)
    if (existing) return existing
    const def: FileTagDef = { id: generateTagId(), name: name || color, color: normalized }
    defs.push(def)
    this.writeTagDefs(userId, defs)
    return def
  }

  static getFileTags(driveId: string, fileId: string, kind: 'folder' | 'file'): FileTagDef[] {
    const userId = usePanTreeStore().user_id
    const defs = new Map(this.readTagDefs(userId).map((def) => [def.id, def]))
    return this.readTagLinks(userId)
      .filter((link) => link.drive_id === driveId && link.file_id === fileId && link.kind === kind)
      .map((link) => defs.get(link.tagId))
      .filter((def): def is FileTagDef => !!def)
  }

  /** 给一批文件打上某个标签（已存在的关联不重复添加） */
  static setFileTag(files: IAliGetFileModel[], tagId: string) {
    const userId = usePanTreeStore().user_id
    const links = this.readTagLinks(userId)
    const existing = new Set(links.filter((link) => link.tagId === tagId).map((link) => tagLinkIdentity(link)))
    for (const file of files) {
      const kind: 'folder' | 'file' = file.isDir ? 'folder' : 'file'
      const identity = `${kind}|${file.drive_id}|${file.file_id}`
      if (existing.has(identity)) continue
      existing.add(identity)
      links.push({ drive_id: file.drive_id, file_id: file.file_id, kind, tagId, title: file.name, parent_file_id: file.parent_file_id, icon: file.icon || '', ext: file.ext || '' })
    }
    this.writeTagLinks(userId, links)
  }

  /** 移除一批文件上的某个标签 */
  static removeFileTag(files: IAliGetFileModel[], tagId: string) {
    const userId = usePanTreeStore().user_id
    const identities = new Set(files.map((file) => `${file.isDir ? 'folder' : 'file'}|${file.drive_id}|${file.file_id}`))
    const links = this.readTagLinks(userId).filter((link) => !(link.tagId === tagId && identities.has(tagLinkIdentity(link))))
    this.writeTagLinks(userId, links)
  }

  /** 清除一批文件上的全部标签 */
  static clearFileTags(files: IAliGetFileModel[]) {
    const userId = usePanTreeStore().user_id
    const identities = new Set(files.map((file) => `${file.isDir ? 'folder' : 'file'}|${file.drive_id}|${file.file_id}`))
    const links = this.readTagLinks(userId).filter((link) => !identities.has(tagLinkIdentity(link)))
    this.writeTagLinks(userId, links)
  }

  /**
   * 目录刷新后清理该目录下的孤儿标签关联（文件已在云端被删除/移走，但标签记录还留着，导致标签筛选视图出现点不开的死项）。
   * 只处理 parent_file_id 落在本次刷新目录内的关联，其余目录的关联一律保留——
   * 因为这里的 validFileIds 只是单个目录的完整列表，而非全盘列表，全局过滤会误删其它目录里仍然有效的标签。
   */
  static pruneOrphanedTagLinks(drive_id: string, parent_file_id: string, validFileIds: Set<string>) {
    const userId = usePanTreeStore().user_id
    if (!userId || !drive_id || !parent_file_id) return
    const links = this.readTagLinks(userId)
    const pruned = links.filter((link) => {
      const inRefreshedDir = link.drive_id === drive_id && link.parent_file_id === parent_file_id
      if (!inRefreshedDir) return true
      return validFileIds.has(`${link.drive_id}:${link.file_id}`)
    })
    if (pruned.length < links.length) this.writeTagLinks(userId, pruned)
  }

  /** 把旧版 FileQuick 里混存的 tag/tagColor 迁移到独立的标签模型，仅执行一次 */
  static migrateLegacyTags(userId: string) {
    if (!userId || typeof localStorage === 'undefined') return
    const flagKey = 'FileTagsMigrated-' + userId
    if (localStorage.getItem(flagKey)) return
    try {
      const quick = this.readQuickFileList(userId)
      const defs = this.readTagDefs(userId)
      const links = this.readTagLinks(userId)
      const linkKeys = new Set(links.map((link) => `${link.tagId}|${tagLinkIdentity(link)}`))
      const colorToDef = new Map<string, FileTagDef>()
      for (const def of defs) colorToDef.set(normalizeTagColor(def.color), def)
      for (const entry of quick) {
        if (!entry.tag || !entry.tagColor) continue
        const color = normalizeTagColor(entry.tagColor)
        let def = colorToDef.get(color)
        if (!def) {
          def = { id: generateTagId(), name: entry.tag, color }
          colorToDef.set(color, def)
          defs.push(def)
        }
        const kind: 'folder' | 'file' = entry.kind === 'file' ? 'file' : 'folder'
        const identity = `${kind}|${entry.drive_id}|${entry.file_id || entry.key}`
        const linkKey = `${def.id}|${identity}`
        if (linkKeys.has(linkKey)) continue
        linkKeys.add(linkKey)
        links.push({ drive_id: entry.drive_id, file_id: entry.file_id || entry.key, kind, tagId: def.id, title: entry.title, parent_file_id: entry.parent_file_id || '', icon: entry.icon || '', ext: entry.ext || '' })
      }
      this.writeTagDefs(userId, defs)
      this.writeTagLinks(userId, links)
      localStorage.setItem(flagKey, '1')
    } catch (err: any) {
      DebugLog.mSaveDanger('migrateLegacyTags', err)
    }
  }

  static updateLocalQuickTag(files: IAliGetFileModel[], tag: string, tagColor = '') {
    try {
      const pantreeStore = usePanTreeStore()
      const userId = pantreeStore.user_id
      this.migrateLegacyTags(userId)
      if (tag && tagColor) {
        // 找到或新建该颜色对应的标签定义，再给文件建立关联
        const def = this.ensureTagDefForColor(userId, tag, tagColor)
        this.setFileTag(files, def.id)
      } else {
        // 清除：移除这批文件上的全部标签关联
        this.clearFileTags(files)
      }
      // 正在浏览某个标签筛选视图时同步刷新
      const panfileStore = usePanFileStore()
      if (panfileStore.DirID.startsWith('color')) void PanDAL.aReLoadOneDirToShow(panfileStore.DriveID, panfileStore.DirID, false)
      // 当前列表行立即更新标签图标
      this.applyLocalQuickTags(userId, panfileStore.ListDataRaw)
      panfileStore.mRefreshListDataShow(false)
    } catch (err: any) {
      DebugLog.mSaveDanger('updateLocalQuickTag', err)
      message.error('本地标签保存失败，请重试')
    }
  }

  static hasQuickFile(file: IAliGetFileModel, favoriteOnly = false) {
    const list = this.readQuickFileList(usePanTreeStore().user_id)
    const identity = `${file.isDir ? 'folder' : 'file'}|${file.drive_id}|${file.file_id}`
    const item = list.find((entry) => quickEntryIdentity(entry) === identity)
    return !!item && (!favoriteOnly || item.favorite === true)
  }

  /** 把本地标签的颜色 class 叠加到文件行的 description 上，让行内显示标签图标 */
  private static applyLocalQuickTags(userId: string, items: IAliGetFileModel[]) {
    if (!items.length) return items
    this.migrateLegacyTags(userId)
    const defs = new Map(this.readTagDefs(userId).map((def) => [def.id, def]))
    const tagMap = new Map<string, string>()
    for (const link of this.readTagLinks(userId)) {
      const def = defs.get(link.tagId)
      if (!def) continue
      const identity = tagLinkIdentity(link)
      if (tagMap.has(identity)) continue
      tagMap.set(identity, normalizeTagColor(def.color).replace('#', 'c'))
    }
    for (const item of items) {
      const colorClass = tagMap.get(`${item.isDir ? 'folder' : 'file'}|${item.drive_id}|${item.file_id}`)
      const description = removeLocalTagColorClasses(item.description || '')
      const nextDescription = colorClass ? (description ? `${description} ${colorClass}` : colorClass) : description
      if (nextDescription === (item.description || '')) continue
      try {
        item.description = nextDescription
      } catch {
        // 个别列表项可能被冻结，跳过行内标签展示不影响标签本身
      }
    }
    return items
  }

  static deleteQuickFile(key: string) {
    if (!key) return
    const pantreeStore = usePanTreeStore()
    const arr = this.readQuickFileList(pantreeStore.user_id).filter((item) => item.key != key)
    this.saveQuickFileList(pantreeStore.user_id, arr)
  }

  static getQuickFileList(): QuickFileEntry[] {
    return this.readQuickFileList(usePanTreeStore().user_id)
  }

  /** 启动/切换账号后把本地快捷列表水合到左侧树（否则重启后快捷方式页签为空） */
  static refreshQuickTree() {
    const userId = usePanTreeStore().user_id
    if (!userId) return
    // 首次加载时把旧版混存的标签迁移到独立模型
    this.migrateLegacyTags(userId)
    this.saveQuickFileList(userId, this.readQuickFileList(userId))
  }

  static aReLoadQuickFile(user_id: string) {
    usePanTreeStore().mSaveQuick(this.readQuickFileList(user_id))
  }

  static async aUpdateDirFileSize(_drive_id: string): Promise<void> {
    // Folder size batch APIs were Aliyun-only; active providers skip size refresh.
  }
}
