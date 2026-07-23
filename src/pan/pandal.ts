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
  tag?: string
  tagColor?: string
  favorite?: boolean
}

const quickEntryKey = (kind: 'folder' | 'file', driveId: string, fileId: string) => `quick:${kind}:${encodeURIComponent(driveId)}:${encodeURIComponent(fileId)}`

const quickEntryIdentity = (entry: Pick<QuickFileEntry, 'drive_id' | 'file_id' | 'key' | 'kind'>) => `${entry.kind || 'folder'}|${entry.drive_id || ''}|${entry.file_id || entry.key || ''}`

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

      // 本地标签筛选：标记数据存在本地快捷列表里，所有网盘共用
      if (dirID.startsWith('color')) {
        const colorKey = dirID.slice('color'.length).trim().split(' ')[0].toLowerCase()
        const hexColor = colorKey.startsWith('c') ? `#${colorKey.slice(1)}` : colorKey
        const tagged = this.readQuickFileList(user_id).filter((entry) => entry.tag && (entry.tagColor || '').toLowerCase() === hexColor)
        const items: IAliGetFileModel[] = tagged.map((entry) => ({
          __v_skip: true,
          drive_id: entry.drive_id,
          file_id: entry.file_id || entry.key,
          parent_file_id: entry.parent_file_id || '',
          name: entry.title,
          namesearch: entry.title,
          path: '',
          ext: '',
          mime_type: '',
          mime_extension: '',
          category: entry.kind === 'file' ? 'file' : 'folder',
          icon: entry.icon || (entry.kind === 'file' ? 'iconwenjian' : 'iconfile-folder'),
          size: 0,
          sizeStr: '',
          time: 0,
          timeStr: '',
          starred: false,
          isDir: entry.kind !== 'file',
          thumbnail: '',
          description: hexColor.replace('#', 'c')
        }))
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

  static updateLocalQuickTag(files: IAliGetFileModel[], tag: string, tagColor = '') {
    try {
      const pantreeStore = usePanTreeStore()
      const existing = this.readQuickFileList(pantreeStore.user_id)
      const byIdentity = new Map(existing.map((item, index) => [quickEntryIdentity(item), index]))
      const driveName = (driveId: string) => GetDriveType(pantreeStore.user_id, driveId).title
      for (const file of files) {
        const kind: 'folder' | 'file' = file.isDir ? 'folder' : 'file'
        const identity = `${kind}|${file.drive_id}|${file.file_id}`
        const index = byIdentity.get(identity)
        if (index === undefined) {
          if (!tag) continue
          byIdentity.set(identity, existing.length)
          existing.push({
            key: quickEntryKey(kind, file.drive_id, file.file_id),
            drive_id: file.drive_id,
            drive_name: driveName(file.drive_id),
            title: file.name,
            file_id: file.file_id,
            parent_file_id: file.parent_file_id,
            kind,
            icon: file.icon,
            tag,
            tagColor,
            favorite: false
          })
        } else {
          existing[index] = { ...existing[index], tag, tagColor, title: file.name, icon: file.icon || existing[index].icon }
        }
      }
      this.saveQuickFileList(pantreeStore.user_id, existing.filter((item) => item.favorite || item.tag))
      // 正在浏览某个标签筛选视图时同步刷新
      const panfileStore = usePanFileStore()
      if (panfileStore.DirID.startsWith('color')) void PanDAL.aReLoadOneDirToShow(panfileStore.DriveID, panfileStore.DirID, false)
      // 当前列表行立即更新标签图标
      this.applyLocalQuickTags(pantreeStore.user_id, panfileStore.ListDataRaw)
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
    const quick = this.readQuickFileList(userId)
    if (!quick.length) return items
    const tagMap = new Map<string, string>()
    for (const entry of quick) {
      if (!entry.tag || !entry.tagColor) continue
      tagMap.set(quickEntryIdentity(entry), entry.tagColor.replace('#', 'c'))
    }
    if (!tagMap.size) return items
    for (const item of items) {
      const colorClass = tagMap.get(`${item.isDir ? 'folder' : 'file'}|${item.drive_id}|${item.file_id}`)
      if (!colorClass) continue
      const description = item.description || ''
      if (description.includes(colorClass)) continue
      try {
        item.description = description ? `${description} ${colorClass}` : colorClass
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
    this.saveQuickFileList(userId, this.readQuickFileList(userId))
  }

  static aReLoadQuickFile(user_id: string) {
    usePanTreeStore().mSaveQuick(this.readQuickFileList(user_id))
  }

  static async aUpdateDirFileSize(_drive_id: string): Promise<void> {
    // Folder size batch APIs were Aliyun-only; active providers skip size refresh.
  }
}
