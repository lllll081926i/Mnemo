import { describe, expect, it } from 'vitest'
import AliShareList from '../sharelist'

describe('AliShareList provider routing', () => {
  it('returns empty share lists without contacting legacy Aliyun APIs', async () => {
    const resp = await AliShareList.ApiShareListAll('unsupported_user')
    expect(resp.items).toEqual([])
    expect(resp.next_marker).toBe('')
  })

  it('never claims a managed share id for non-aliyun accounts', async () => {
    await expect(AliShareList.ApiShareListUntilShareID('unsupported_user', 'share-id')).resolves.toBe(false)
  })
})
