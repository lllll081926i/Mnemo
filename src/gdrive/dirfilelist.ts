import type { IAliGetFileModel } from '../aliapi/alimodels'
import getFileIcon from '../aliapi/fileicon'
import { humanDateTimeDateStr, humanSize } from '../utils/format'
import { HanToPin } from '../utils/utils'
import { googleDriveRequest } from './client'

export const GOOGLE_FOLDER_MIME = 'application/vnd.google-apps.folder'

export interface GoogleDriveItem {
  id: string
  name: string
  mimeType: string
  size?: string
  createdTime?: string
  modifiedTime?: string
  md5Checksum?: string
  thumbnailLink?: string
  webContentLink?: string
  webViewLink?: string
  parents?: string[]
  trashed?: boolean
  imageMediaMetadata?: { width?: number; height?: number }
  videoMediaMetadata?: { width?: number; height?: number; durationMillis?: string }
}

const FILE_FIELDS = 'id,name,mimeType,size,createdTime,modifiedTime,md5Checksum,thumbnailLink,webContentLink,webViewLink,parents,trashed,imageMediaMetadata,videoMediaMetadata'
const escapeQuery = (value: string) => value.replace(/\\/g, '\\\\').replace(/'/g, "\\'")

export const mapGoogleDriveItemToAliModel = (item: GoogleDriveItem, driveId: string, parentId: string): IAliGetFileModel => {
  const isDir = item.mimeType === GOOGLE_FOLDER_MIME
  const name = item.name || ''
  const ext = isDir ? '' : (name.split('.').pop() || '').toLowerCase()
  const size = Number(item.size || 0)
  const time = new Date(item.modifiedTime || item.createdTime || '').getTime() || 0
  const iconInfo = isDir ? ['folder', 'iconfile-folder'] : getFileIcon('others', ext, ext, item.mimeType, size)
  return {
    __v_skip: true,
    drive_id: driveId,
    file_id: item.id,
    parent_file_id: parentId,
    name,
    namesearch: HanToPin(name),
    ext,
    mime_type: item.mimeType,
    mime_extension: ext,
    category: iconInfo[0],
    icon: iconInfo[1],
    file_count: 0,
    size,
    sizeStr: humanSize(size),
    time,
    timeStr: item.modifiedTime ? humanDateTimeDateStr(item.modifiedTime) : '',
    starred: false,
    isDir,
    thumbnail: item.thumbnailLink || '',
    description: item.webViewLink ? `gdrive_view:${encodeURIComponent(item.webViewLink)}` : '',
    content_hash: item.md5Checksum || '',
    created_at: item.createdTime || '',
    updated_at: item.modifiedTime || '',
    type: isDir ? 'folder' : 'file',
    image_media_metadata: item.imageMediaMetadata,
    video_media_metadata: item.videoMediaMetadata ? { width: item.videoMediaMetadata.width || 0, height: item.videoMediaMetadata.height || 0, duration: Math.floor(Number(item.videoMediaMetadata.durationMillis || 0) / 1000) } : undefined
  } as any
}

const listGoogleDriveQuery = async (userId: string, query: string) => {
  const items: GoogleDriveItem[] = []
  let pageToken = ''
  do {
    const params = new URLSearchParams({ q: query, fields: `nextPageToken,files(${FILE_FIELDS})`, pageSize: '1000', spaces: 'drive', supportsAllDrives: 'true', includeItemsFromAllDrives: 'true' })
    if (pageToken) params.set('pageToken', pageToken)
    const response = await googleDriveRequest<{ files?: GoogleDriveItem[]; nextPageToken?: string }>(userId, `/files?${params.toString()}`)
    items.push(...(response.files || []))
    pageToken = response.nextPageToken || ''
  } while (pageToken)
  return items
}

export const apiGoogleDriveFileList = (userId: string, parentId: string) => listGoogleDriveQuery(userId, `'${escapeQuery(parentId === 'gdrive_root' ? 'root' : parentId)}' in parents and trashed = false`)

export const apiGoogleDriveSearch = (userId: string, name: string) => listGoogleDriveQuery(userId, `name contains '${escapeQuery(name)}' and trashed = false`)

export const apiGoogleDriveTrash = (userId: string) => listGoogleDriveQuery(userId, 'trashed = true')

export const apiGoogleDriveFileDetail = (userId: string, fileId: string) => googleDriveRequest<GoogleDriveItem>(userId, `/files/${encodeURIComponent(fileId === 'gdrive_root' ? 'root' : fileId)}?fields=${encodeURIComponent(FILE_FIELDS)}&supportsAllDrives=true`)
