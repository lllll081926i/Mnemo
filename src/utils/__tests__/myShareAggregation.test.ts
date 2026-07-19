import { beforeEach, describe, expect, it } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import useMyShareStore, { type IManagedShareItem } from '../../share/share/MyShareStore'

const shareItem = (accountId: string, accountName: string, shareId: string): IManagedShareItem => ({
  account_id: accountId,
  account_name: accountName,
  account_provider: 'aliyun',
  share_key: `${accountId}:${shareId}`,
  created_at: '2026-01-01 00:00:00',
  creator: '',
  description: '',
  display_name: '',
  display_label: '',
  download_count: 0,
  drive_id: 'resource',
  expiration: '',
  expired: false,
  file_id: '',
  file_id_list: [],
  icon: 'iconlink2',
  preview_count: 0,
  save_count: 0,
  share_id: shareId,
  share_msg: '永久有效',
  full_share_msg: '',
  share_name: `${accountName} share`,
  share_policy: '',
  share_pwd: '',
  share_url: `https://example.test/${shareId}`,
  status: '',
  updated_at: '',
  is_share_saved: false,
  share_saved: ''
})

describe('multi-account share aggregation', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    ;(globalThis as any).pinyinlite = (input: string) => Array.from(input).map((character) => [character])
  })

  it('keeps identical provider share ids isolated by account', () => {
    const store = useMyShareStore()
    store.aLoadListData([shareItem('account-a', 'Account A', 'same-id'), shareItem('account-b', 'Account B', 'same-id')], ['account-a', 'account-b'])

    store.mMouseSelect('account-a:same-id', false, false)
    expect(store.GetSelectedFirst()?.account_id).toBe('account-a')

    store.mDeleteFiles(['account-a:same-id'])
    expect(store.ListDataRaw.map((item) => item.share_key)).toEqual(['account-b:same-id'])
  })

  it('filters account previews without discarding overview data', () => {
    const store = useMyShareStore()
    store.aLoadListData([shareItem('account-a', 'Account A', 'share-a'), shareItem('account-b', 'Account B', 'share-b')], ['account-a', 'account-b'])

    store.mSetAccountFilter('account-b')
    expect(store.ListDataShow.map((item) => item.account_id)).toEqual(['account-b'])
    expect(store.ListDataRaw).toHaveLength(2)

    store.mSetAccountFilter('')
    expect(store.ListDataShow).toHaveLength(2)
  })
})
