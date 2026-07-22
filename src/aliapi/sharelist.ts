import type { IAliShareItem, IAliShareRecentItem } from './alimodels'

export interface IAliShareResp {
  items: IAliShareItem[]
  itemsKey: Set<string>
  next_marker: string
  m_time: number
  m_user_id: string
}

export interface IAliShareRecentResp {
  items: IAliShareRecentItem[]
  itemsKey: Set<string>
  next_marker: string
  m_time: number
  m_user_id: string
}

const emptyShare = (user_id: string): IAliShareResp => ({
  items: [],
  itemsKey: new Set(),
  next_marker: '',
  m_time: 0,
  m_user_id: user_id
})

const emptyRecent = (user_id: string): IAliShareRecentResp => ({
  items: [],
  itemsKey: new Set(),
  next_marker: '',
  m_time: 0,
  m_user_id: user_id
})

/** Share listing was Aliyun-only; retained providers create shares but do not list managed share history yet. */
export default class AliShareList {
  static EmptyShareResp(user_id: string) {
    return emptyShare(user_id)
  }

  static async ApiShareListAll(user_id: string): Promise<IAliShareResp> {
    return emptyShare(user_id)
  }

  static async ApiShareRecentListAll(user_id: string): Promise<IAliShareRecentResp> {
    return emptyRecent(user_id)
  }

  static async ApiShareListUntilShareID(_user_id: string, _share_id: string): Promise<boolean> {
    return false
  }
}
