import { IAliGetDirModel } from '../aliapi/alimodels'
import AliFile from '../aliapi/file'
import AliDirFileList, { NewIAliFileResp } from '../aliapi/dirfilelist'
import { ITokenInfo, useFootStore, usePanFileStore } from '../store'
import TreeStore, { IDriverModel, TreeNodeData } from '../store/treestore'
import DB from '../utils/db'
import DebugLog from '../utils/debuglog'
import message from '../utils/message'
import usePanTreeStore from './pantreestore'
import { GetDriveID, GetDriveType, isCloud139User, isCloud189User, isGuangyaUser, isPikPakUser, isQuarkUser } from '../aliapi/utils'
import AliAlbum from '../aliapi/album'
import { apiPikPakFileList, mapPikPakFileToAliModel } from '../pikpak/dirfilelist'
import { apiQuarkFileList, apiQuarkSearch, mapQuarkFileToAliModel } from '../quark/dirfilelist'
import { apiCloud139FileList, mapCloud139FileToAliModel } from '../cloud139/dirfilelist'
import { apiCloud189FileList, mapCloud189FileToAliModel } from '../cloud189/dirfilelist'
import { apiGuangyaFileList, mapGuangyaFileToAliModel } from '../guangya/dirfilelist'
import { getWebDavConnection, getWebDavConnectionId, isWebDavDrive, listWebDavDirectory } from '../utils/webdavClient'
import { getS3Connection, getS3ConnectionId, isS3Drive, listS3Directory } from '../utils/s3Client'
import { OrderDir } from '../utils/filenameorder'

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

const RefreshLock = new Set<string>()
export default class PanDAL {
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

  static async aReLoadQuarkDrive(token: ITokenInfo): Promise<void> {
    const { user_id } = token
    const drive_id = token.default_drive_id || 'quark'
    const pantreeStore = usePanTreeStore()
    pantreeStore.mSaveUser(user_id, drive_id, '', '', '')
    pantreeStore.drive_id = drive_id
    if (!user_id) return
    useFootStore().mSaveLoading('加载夸克网盘文件夹...')
    const { items: list } = await apiQuarkFileList(user_id, '0', 100)
    const driveType = GetDriveType(user_id, drive_id)
    const dirs = list
      .filter((item) => Number(item.file_type || 0) === 0)
      .map((item) => ({
        file_id: String(item.fid),
        drive_id: drive_id,
        parent_file_id: driveType.key,
        name: item.file_name,
        description: item.fid ? `quark_fid:${item.fid};quark_pdir:${item.pdir_fid || '0'}` : '',
        time: new Date(item.updated_at || item.created_at || '').getTime() || 0,
        size: 0
      }))
    await TreeStore.ConvertToOneDriver(user_id, drive_id, dirs, false, true)
    PanDAL.RefreshPanTreeAllNode(drive_id)
    useFootStore().mSaveLoading('')
  }

  static async aReLoadCloud139Drive(token: ITokenInfo): Promise<void> {
    const { user_id } = token
    const drive_id = token.default_drive_id || 'cloud139'
    const pantreeStore = usePanTreeStore()
    pantreeStore.mSaveUser(user_id, drive_id, '', '', '')
    pantreeStore.drive_id = drive_id
    if (!user_id) return
    useFootStore().mSaveLoading('加载 139 云盘文件夹...')
    const list = await apiCloud139FileList(user_id, '/', 100)
    const driveType = GetDriveType(user_id, drive_id)
    const dirs = list
      .filter((item) => item.type === 'folder')
      .map((item) => {
        const mapped = mapCloud139FileToAliModel(item, drive_id, driveType.key)
        return { file_id: mapped.file_id, drive_id, parent_file_id: driveType.key, name: mapped.name, description: mapped.description || '', time: mapped.time, size: 0 }
      })
    await TreeStore.ConvertToOneDriver(user_id, drive_id, dirs, false, true)
    PanDAL.RefreshPanTreeAllNode(drive_id)
    useFootStore().mSaveLoading('')
  }

  static async aReLoadCloud189Drive(token: ITokenInfo): Promise<void> {
    const { user_id } = token
    const drive_id = token.default_drive_id || 'cloud189'
    const pantreeStore = usePanTreeStore()
    pantreeStore.mSaveUser(user_id, drive_id, '', '', '')
    pantreeStore.drive_id = drive_id
    if (!user_id) return
    useFootStore().mSaveLoading('加载天翼云盘文件夹...')
    const list = await apiCloud189FileList(user_id, '-11', 100)
    const driveType = GetDriveType(user_id, drive_id)
    const dirs = list
      .filter((item) => item.isFolder)
      .map((item) => {
        const mapped = mapCloud189FileToAliModel(item, drive_id, driveType.key)
        return { file_id: mapped.file_id, drive_id, parent_file_id: driveType.key, name: mapped.name, description: mapped.description || '', time: mapped.time, size: 0 }
      })
    await TreeStore.ConvertToOneDriver(user_id, drive_id, dirs, false, true)
    PanDAL.RefreshPanTreeAllNode(drive_id)
    useFootStore().mSaveLoading('')
  }

  static async aReLoadGuangyaDrive(token: ITokenInfo): Promise<void> {
    const { user_id } = token
    const drive_id = token.default_drive_id || 'guangya'
    const pantreeStore = usePanTreeStore()
    pantreeStore.mSaveUser(user_id, drive_id, '', '', '')
    pantreeStore.drive_id = drive_id
    if (!user_id) return
    useFootStore().mSaveLoading('加载光鸭云盘文件夹...')
    const list = await apiGuangyaFileList(user_id, 'guangya_root', 100)
    const driveType = GetDriveType(user_id, drive_id)
    const dirs = list
      .map((item) => mapGuangyaFileToAliModel(item, drive_id, driveType.key))
      .filter((item) => item.isDir)
      .map((item) => ({ file_id: item.file_id, drive_id, parent_file_id: driveType.key, name: item.name, description: item.description || '', time: item.time, size: 0 }))
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

  static async aReLoadBackupDrive(token: ITokenInfo): Promise<void> {
    const { user_id, backup_drive_id: drive_id } = token
    if (!user_id || !drive_id) return
    const backupCache = await DB.getValueObject('AllDir_' + drive_id)
    if (backupCache) {
      console.log('aReLoadDrive backupCache')
      await TreeStore.ConvertToOneDriver(user_id, drive_id, backupCache as IAliGetDirModel[], false, true)
    }
    if (backupCache) {
      const dt = await DB.getValueNumber('AllDir_' + drive_id)
      if (Date.now() - dt < 1000 * 60 * 60) {
        return
      }
    }
    useFootStore().mSaveLoading('加载全部文件夹...')
    window.WinMsgToUpload({ cmd: 'AllDirList', user_id, drive_id: drive_id, drive_root: 'backup_root' })
  }

  static async aReLoadResourceDrive(token: ITokenInfo): Promise<void> {
    const { user_id, resource_drive_id } = token
    if (!user_id || !resource_drive_id) return
    const resourceCache = await DB.getValueObject('AllDir_' + resource_drive_id)
    if (resourceCache) {
      console.log('aReLoadDrive resourceCache')
      await TreeStore.ConvertToOneDriver(user_id, resource_drive_id, resourceCache as IAliGetDirModel[], false, true)
    }
    if (resourceCache) {
      const dt = await DB.getValueNumber('AllDir_' + resource_drive_id)
      if (Date.now() - dt < 1000 * 60 * 60) {
        return
      }
    }
    useFootStore().mSaveLoading('加载全部文件夹...')
    window.WinMsgToUpload({ cmd: 'AllDirList', user_id, drive_id: resource_drive_id, drive_root: 'resource_root' })
  }

  static async aReLoadDriveSave(OneDriver: IDriverModel, error: string): Promise<void> {
    if (error == 'time') return
    if (!error) {
      await TreeStore.SaveOneDriver(OneDriver)
      PanDAL.RefreshPanTreeAllNode(OneDriver.drive_id)
    } else {
      message.error('列出全盘文件夹失败' + error)
    }
    useFootStore().mSaveLoading('')
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
    const driveType = GetDriveType(user_id, drive_id)
    const isBack = file_id == 'back'
    if (!drive_id) {
      drive_id = GetDriveID(user_id, file_id) || panTreeStore.drive_id
    }
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
    if (!dir || (dirPath.length == 0 && !file_id.includes('root'))) {
      if (file_id.includes('root')) {
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
        let findPath = []
        if (!album_id) {
          findPath = await AliFile.ApiFileGetPath(panTreeStore.user_id, drive_id, file_id)
        } else {
          findPath = await AliAlbum.ApiAlbumGetPath(panTreeStore.user_id, drive_id, album_id)
        }
        if (findPath.length > 0) {
          dirPath = findPath
          dir = { ...dirPath[dirPath.length - 1] }
        }
      }
    }
    if (!dir || (dirPath.length == 0 && !file_id.includes('root'))) {
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
            message.warning('列出 WebDAV 文件夹失败 ' + (err?.message || ''))
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
            message.warning('列出 S3 文件夹失败 ' + (err?.message || ''))
            resolve(false)
          })
        return
      }

      if (isPikPakUser(user_id)) {
        const isTrash = dirID === 'trash'
        const isSearch = dirID.startsWith('search')
        if (isSearch) {
          if (hasFiles) usePanFileStore().mSaveDirFileLoadingFinish(drive_id, dirID, [])
          message.warning('PikPak 暂不支持搜索')
          resolve(true)
          return
        }
        const parentId = dirID === 'pikpak_root' || isTrash ? 'pikpak_root' : dirID
        apiPikPakFileList(user_id, parentId, 100, '', isTrash)
          .then(({ items: list }) => {
            const allItems = list.map((item) => mapPikPakFileToAliModel(item, drive_id, parentId))
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

      if (isQuarkUser(user_id)) {
        const isSearch = dirID.startsWith('search')
        const parentId = dirID === 'quark_root' ? '0' : dirID
        const request = isSearch ? apiQuarkSearch(user_id, dirID.substring('search'.length).trim(), 200).then((items) => ({ items, total: items.length })) : apiQuarkFileList(user_id, parentId, 200)
        request
          .then(({ items: list, total }) => {
            const allItems = list.map((item) => mapQuarkFileToAliModel(item, drive_id, isSearch ? 'quark_root' : dirID))
            const items = hasFiles ? allItems : allItems.filter((item) => item.isDir)
            const order = TreeStore.GetDirOrder(drive_id, dirID).replace('ext ', 'updated_at ')
            const orders = order.split(' ')
            OrderDir(orders[0], orders[1], items)
            const dir = NewIAliFileResp(user_id, drive_id, dirID, dirName)
            dir.items = items
            dir.itemsKey = new Set(items.map((item) => item.file_id))
            dir.next_marker = ''
            dir.itemsTotal = total || items.length
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

      if (isCloud139User(user_id) || isCloud189User(user_id)) {
        const is139 = isCloud139User(user_id)
        const rootId = is139 ? 'cloud139_root' : 'cloud189_root'
        const parentId = dirID === rootId ? (is139 ? '/' : '-11') : dirID
        const request = is139 ? apiCloud139FileList(user_id, parentId, 200).then((items) => ({ items, total: items.length })) : apiCloud189FileList(user_id, parentId, 200).then((items) => ({ items, total: items.length }))
        request
          .then(({ items: list, total }) => {
            const allItems = is139 ? (list as any[]).map((item) => mapCloud139FileToAliModel(item, drive_id, dirID)) : (list as any[]).map((item) => mapCloud189FileToAliModel(item, drive_id, dirID))
            const items = hasFiles ? allItems : allItems.filter((item) => item.isDir)
            const order = TreeStore.GetDirOrder(drive_id, dirID).replace('ext ', 'updated_at ')
            const orders = order.split(' ')
            OrderDir(orders[0], orders[1], items)
            const dir = NewIAliFileResp(user_id, drive_id, dirID, dirName)
            dir.items = items
            dir.itemsKey = new Set(items.map((item) => item.file_id))
            dir.next_marker = ''
            dir.itemsTotal = total || items.length
            const panfileStore = usePanFileStore()
            panfileStore.mSaveDirFileLoadingPart(0, dir, dir.itemsTotal || 0)
            TreeStore.SaveOneDirFileList(dir, hasFiles).then(() => {
              if (hasFiles) panfileStore.mSaveDirFileLoadingFinish(drive_id, dirID, dir.items, dir.itemsTotal || 0)
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

      if (isGuangyaUser(user_id)) {
        const parentId = dirID === 'guangya_root' ? 'guangya_root' : dirID
        apiGuangyaFileList(user_id, parentId, 200)
          .then((list) => {
            const allItems = list.map((item) => mapGuangyaFileToAliModel(item, drive_id, dirID))
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
              if (hasFiles) panfileStore.mSaveDirFileLoadingFinish(drive_id, dirID, dir.items, dir.itemsTotal || 0)
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

      const order = TreeStore.GetDirOrder(drive_id, dirID).replace('ext ', 'updated_at ')
      AliDirFileList.ApiDirFileList(user_id, drive_id, dirID, dirName, order, hasFiles ? '' : 'folder', albumID)
        .then((dir) => {
          if (!dir.next_marker) {
            dir.dirID = dirID // 修复root
            TreeStore.SaveOneDirFileList(dir, hasFiles).then(() => {
              if (hasFiles) {
                usePanFileStore().mSaveDirFileLoadingFinish(drive_id, dirID, dir.items, dir.itemsTotal || 0)
              }
              PanDAL.RefreshPanTreeAllNode(drive_id)
              resolve(true)
            })
          } else if (dir.next_marker == 'cancel') {
            resolve(false)
          } else {
            message.warning('列出文件夹失败 ' + dir.next_marker)
            if (hasFiles) usePanFileStore().mSaveDirFileLoadingFinish(drive_id, dirID, [])
            resolve(false)
          }
        })
        .catch((err: any) => {
          if (hasFiles) usePanFileStore().mSaveDirFileLoadingFinish(drive_id, dirID, [])
          message.warning('列出文件夹失败 ' + (err.message || ''))
          DebugLog.mSaveWarning('列出文件夹失败file_id=' + dirID, err)
          resolve(false)
        })
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

  static updateQuickFile(list: { key: string; drive_id: string; drive_name: string; title: string }[]) {
    if (list.length == 0) return
    const pantreeStore = usePanTreeStore()
    const jsonstr = localStorage.getItem('FileQuick-' + pantreeStore.user_id)
    const arr = jsonstr ? JSON.parse(jsonstr) : []
    list.map((t) => {
      let find = false
      for (let i = 0; i < arr.length; i++) {
        if (arr[i].key == t.key) {
          arr[i].title = t.title
          arr[i].drive_id = t.drive_id
          arr[i].drive_name = t.drive_name
          find = true
        }
      }
      if (!find) arr.push({ key: t.key, drive_id: t.drive_id, drive_name: t.drive_name, title: t.title })
      return true
    })
    localStorage.setItem('FileQuick-' + pantreeStore.user_id, JSON.stringify(arr))
    pantreeStore.mSaveQuick(arr)
  }

  static deleteQuickFile(key: string) {
    if (!key) return
    const pantreeStore = usePanTreeStore()
    const jsonstr = localStorage.getItem('FileQuick-' + pantreeStore.user_id)
    const arr = jsonstr ? JSON.parse(jsonstr) : []
    const newArray: { key: string; drive_id: string; drive_name: string; title: string }[] = []
    for (let i = 0; i < arr.length; i++) {
      if (arr[i].key != key) newArray.push(arr[i])
    }
    localStorage.setItem('FileQuick-' + pantreeStore.user_id, JSON.stringify(newArray))
    pantreeStore.mSaveQuick(newArray)
  }

  static getQuickFileList() {
    const pantreeStore = usePanTreeStore()
    const jsonstr = localStorage.getItem('FileQuick-' + pantreeStore.user_id)
    return jsonstr ? JSON.parse(jsonstr) : []
  }

  static aReLoadQuickFile(user_id: string) {
    const jsonstr = localStorage.getItem('FileQuick-' + user_id)
    const arr = jsonstr ? JSON.parse(jsonstr) : []
    usePanTreeStore().mSaveQuick(arr)
  }

  static async aUpdateDirFileSize(drive_id: string): Promise<void> {
    const pantreeStore = usePanTreeStore()
    const user_id = pantreeStore.user_id

    const diridList = TreeStore.GetDirSizeNeedRefresh(drive_id, 604800)
    const partList: string[] = []
    for (let i = 0, maxi = diridList.length; i < maxi; i++) {
      partList.push(diridList[i])
      if (partList.length >= 30) {
        const partResult = await AliDirFileList.ApiDirFileSize(user_id, drive_id, partList)
        if (!partResult) return
        if (partResult) TreeStore.SaveDirSizeNeedRefresh(drive_id, partResult)
        partList.length = 0
      }
    }
    if (partList.length > 0) {
      const partResult = await AliDirFileList.ApiDirFileSize(user_id, drive_id, partList)
      if (partResult) TreeStore.SaveDirSizeNeedRefresh(drive_id, partResult)
      partList.length = 0
    }
  }
}
