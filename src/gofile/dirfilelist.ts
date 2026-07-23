import type { IAliGetFileModel } from '../aliapi/alimodels'
import getFileIcon from '../aliapi/fileicon'
import { humanDateTimeDateStr, humanSize } from '../utils/format'
import { resolveFileExt } from '../utils/filetype'
import { HanToPin } from '../utils/utils'
import { getGofileRootId, gofileRequest, type GofileItem } from './client'

export const mapGofileItemToAliModel = (item: GofileItem, driveId: string, parentId: string): IAliGetFileModel => {
  const isDir = item.type === 'folder'
  const name = item.name || ''
  const ext = isDir ? '' : resolveFileExt(name, '', item.mimetype)
  const size = Number(item.size || 0)
  const time = Number(item.modTime || item.createTime || 0) * 1000
  const iconInfo = isDir ? ['folder', 'iconfile-folder'] : getFileIcon('others', ext, ext, item.mimetype || '', size)
  return {
    __v_skip: true,
    drive_id: driveId,
    file_id: item.id,
    parent_file_id: parentId,
    name,
    namesearch: HanToPin(name),
    ext,
    mime_type: item.mimetype || '',
    mime_extension: ext,
    category: iconInfo[0],
    icon: iconInfo[1],
    file_count: Number(item.childrenCount || 0),
    size,
    sizeStr: humanSize(size),
    time,
    timeStr: time ? humanDateTimeDateStr(new Date(time).toISOString()) : '',
    starred: false,
    isDir,
    thumbnail: '',
    description: item.link ? `gofile_link:${encodeURIComponent(item.link)}` : '',
    content_hash: item.md5 || '',
    created_at: item.createTime ? new Date(item.createTime * 1000).toISOString() : '',
    updated_at: item.modTime ? new Date(item.modTime * 1000).toISOString() : '',
    type: isDir ? 'folder' : 'file'
  } as any
}

export const apiGofileFileDetail = async (userId: string, fileId: string) => {
  const id = fileId === 'gofile_root' ? await getGofileRootId(userId) : fileId
  const response = await gofileRequest<GofileItem>(userId, `/contents/${encodeURIComponent(id)}?page=1&pageSize=1`)
  return response.data || null
}

export const apiGofileFileList = async (userId: string, parentId: string): Promise<GofileItem[]> => {
  const id = !parentId || parentId === 'gofile_root' ? await getGofileRootId(userId) : parentId
  const items: GofileItem[] = []
  for (let page = 1; page <= 100; page++) {
    const response = await gofileRequest<GofileItem>(userId, `/contents/${encodeURIComponent(id)}?page=${page}&pageSize=1000`)
    items.push(...Object.values(response.data?.children || {}))
    if (!response.metadata?.hasNextPage) break
  }
  return items
}
