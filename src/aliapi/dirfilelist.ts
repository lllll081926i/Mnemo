import { IAliGetFileModel } from './alimodels'

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
