import type { IAliGetFileModel } from '../aliapi/alimodels'
import { apiDropboxFileList, mapDropboxFileToAliModel } from '../dropbox/dirfilelist'
import { apiGoogleDriveFileList, apiGoogleDriveTrash, mapGoogleDriveItemToAliModel } from '../gdrive/dirfilelist'
import { apiGofileFileList, mapGofileItemToAliModel } from '../gofile/dirfilelist'
import { apiOneDriveFileList, mapOneDriveItemToAliModel } from '../onedrive/dirfilelist'
import { apiPikPakFileList, mapPikPakFileToAliModel } from '../pikpak/dirfilelist'
import { isDriveProviderRootId, resolveDriveProvider } from './driveProvider'
import { getS3Connection, getS3ConnectionId, isS3Drive, listS3Directory } from './s3Client'
import { getWebDavConnection, getWebDavConnectionId, isWebDavDrive, listWebDavDirectory } from './webdavClient'

const listPikPakPages = async (user_id: string, parentId: string, trashed: boolean, drive_id: string) => {
  const items: IAliGetFileModel[] = []
  let pageToken = ''
  for (let page = 0; page < 100; page++) {
    const result = await apiPikPakFileList(user_id, parentId, 100, pageToken, trashed)
    items.push(...result.items.map((item) => mapPikPakFileToAliModel(item, drive_id, parentId)))
    if (!result.nextPageToken) break
    pageToken = result.nextPageToken
  }
  return items
}

export const listProviderDirChildren = async (user_id: string, drive_id: string, dir_id: string): Promise<IAliGetFileModel[]> => {
  if (!user_id || !drive_id || !dir_id) return []
  const provider = resolveDriveProvider({ userId: user_id, driveId: drive_id })
  if (provider === 'pikpak') return listPikPakPages(user_id, dir_id, false, drive_id)
  if (provider === 'onedrive') {
    const list = await apiOneDriveFileList(user_id, dir_id)
    return list.map((item) => mapOneDriveItemToAliModel(item, drive_id, dir_id))
  }
  if (provider === 'dropbox') {
    const list = await apiDropboxFileList(user_id, dir_id)
    return list.map((item) => mapDropboxFileToAliModel(item, drive_id, dir_id))
  }
  if (provider === 'gdrive') {
    const list = await apiGoogleDriveFileList(user_id, dir_id)
    return list.map((item) => mapGoogleDriveItemToAliModel(item, drive_id, dir_id))
  }
  if (provider === 'gofile') {
    const list = await apiGofileFileList(user_id, dir_id)
    return list.map((item) => mapGofileItemToAliModel(item, drive_id, dir_id))
  }
  if (provider === 'webdav' || isWebDavDrive(drive_id)) {
    const connection = getWebDavConnection(getWebDavConnectionId(drive_id))
    if (!connection) return []
    return listWebDavDirectory(connection, isDriveProviderRootId(provider, dir_id) ? '/' : dir_id)
  }
  if (provider === 's3' || isS3Drive(drive_id)) {
    const connection = getS3Connection(getS3ConnectionId(drive_id))
    if (!connection) return []
    return listS3Directory(connection, isDriveProviderRootId(provider, dir_id) ? '/' : dir_id)
  }
  return []
}

export const listProviderTrashItems = async (user_id: string, drive_id: string): Promise<IAliGetFileModel[]> => {
  if (!user_id || !drive_id) return []
  const provider = resolveDriveProvider({ userId: user_id, driveId: drive_id })
  if (provider === 'pikpak') return listPikPakPages(user_id, 'pikpak_root', true, drive_id)
  if (provider === 'gdrive') {
    const list = await apiGoogleDriveTrash(user_id)
    return list.map((item) => mapGoogleDriveItemToAliModel(item, drive_id, 'trash'))
  }
  return []
}
