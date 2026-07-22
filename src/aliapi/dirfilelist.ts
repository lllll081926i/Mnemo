import { humanDateTimeDateStr, humanSize, humanTime } from '../utils/format'
import { HanToPin } from '../utils/utils'
import { IAliFileItem, IAliGetFileModel } from './alimodels'
import getFileIcon from './fileicon'
import { DecodeEncName } from './utils'

export interface IAliFileResp {
  items: IAliGetFileModel[]
  itemsKey: Set<string>
  punished_file_count: number
  next_marker: string
  m_user_id: string
  m_drive_id: string
  dirID: string
  albumID?: string
  dirName: string
  itemsTotal?: number
}

export function NewIAliFileResp(user_id: string, drive_id: string, dirID: string, dirName: string): IAliFileResp {
  return {
    items: [],
    itemsKey: new Set<string>(),
    punished_file_count: 0,
    next_marker: '',
    m_user_id: user_id,
    m_drive_id: drive_id,
    dirID: dirID,
    dirName: dirName
  }
}

export default class AliDirFileList {
  static getFileInfo(user_id: string, item: IAliFileItem, downUrl: string): IAliGetFileModel {
    const size = item.size ? item.size : 0
    const file_count = item.file_count || item.image_count || item.video_count || 0
    const time = new Date(item.updated_at || item.created_at || item.gmt_deleted || item.last_played_at || '')
    const timeStr = humanDateTimeDateStr(item.updated_at || item.created_at || item.gmt_deleted || item.last_played_at || '')
    const isDir = item.type == 'folder'
    const { name, mine_type, ext } = DecodeEncName(user_id, item)
    const add: IAliGetFileModel = {
      __v_skip: true,
      drive_id: item.drive_id,
      file_id: item.file_id,
      parent_file_id: item.parent_file_id || '',
      name: name,
      namesearch: HanToPin(name),
      ext: ext || '',
      mime_type: mine_type || '',
      mime_extension: item.mime_extension,
      category: item.category || '',
      starred: item.starred || false,
      time: time.getTime(),
      file_count: file_count,
      size: size,
      sizeStr: humanSize(size),
      timeStr: timeStr,
      icon: 'iconfile-folder',
      isDir: isDir,
      thumbnail: '',
      from_share_id: item.from_share_id,
      punish_flag: item.punish_flag,
      description: item.description || '',
      user_meta: item.user_meta || ''
    }
    if (!isDir) {
      const icon = getFileIcon(add.category, add.ext, add.ext || item.mime_extension, add.mime_type, add.size)
      add.category = icon[0]
      add.icon = icon[1]
      if (downUrl == 'download_url') add.download_url = item.download_url || ''
      if (item.video_media_metadata && Object.keys(item.video_media_metadata).length > 0) {
        add.media_width = item.video_media_metadata.width || 0
        add.media_height = item.video_media_metadata.height || 0
        add.media_time = humanDateTimeDateStr(item.video_media_metadata.time)
        add.media_duration = humanTime(item.video_media_metadata.duration)
      } else if (item.video_preview_metadata && Object.keys(item.video_preview_metadata).length > 0) {
        add.media_width = item.video_preview_metadata.width || 0
        add.media_height = item.video_preview_metadata.height || 0
        add.media_duration = humanTime(item.video_preview_metadata.duration)
      } else if (item.image_media_metadata && Object.keys(item.image_media_metadata).length > 0) {
        add.media_width = item.image_media_metadata.width || 0
        add.media_height = item.image_media_metadata.height || 0
        add.media_time = humanDateTimeDateStr(item.image_media_metadata.time)
      }
      if (item.play_cursor) add.media_play_cursor = humanTime(item.play_cursor)
      else if (item.user_meta) {
        try {
          const meta = JSON.parse(item.user_meta)
          if (meta.play_cursor) add.media_play_cursor = humanTime(meta.play_cursor)
        } catch {}
      }
      if (!add.media_duration && item.duration) add.media_duration = humanTime(item.duration)
    }
    if (item.punish_flag == 2) add.icon = 'iconweifa'
    else if (item.punish_flag == 103) add.icon = 'iconpartweifa'
    else if (item.punish_flag > 0) add.icon = 'iconweixiang'
    return add
  }

  /** Active providers list via provider modules; legacy Aliyun directory APIs are removed. */
  static async ApiDirFileList(user_id: string, drive_id: string, dirID: string, dirName: string, _order: string, _type: string = '', albumID?: string, _refresh: boolean = true): Promise<IAliFileResp> {
    const dir = NewIAliFileResp(user_id, drive_id, dirID, dirName)
    dir.albumID = albumID
    dir.next_marker = 'unsupported'
    return dir
  }

  static async ApiDirFileSize(_user_id: string, _drive_id: string, _file_idList: string[]): Promise<Record<string, number> | undefined> {
    return {}
  }
}
