import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'

let AliHttp: typeof import('../alihttp').default
let AliShareList: typeof import('../sharelist').default

describe('AliShareList provider routing', () => {
  beforeAll(async () => {
    ;(globalThis as any).self = globalThis
    ;[{ default: AliHttp }, { default: AliShareList }] = await Promise.all([import('../alihttp'), import('../sharelist')])
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('does not call Aliyun share list api for unsupported third-party users', async () => {
    const postSpy = vi.spyOn(AliHttp, 'Post').mockRejectedValue(new Error('should not call Aliyun'))

    const resp = await AliShareList.ApiShareListAll('unsupported_user')

    expect(resp.items).toEqual([])
    expect(resp.next_marker).toBe('')
    expect(postSpy).not.toHaveBeenCalled()
  })

  it('does not call Aliyun share list api when checking unsupported third-party share ids', async () => {
    const postSpy = vi.spyOn(AliHttp, 'Post').mockRejectedValue(new Error('should not call Aliyun'))

    await expect(AliShareList.ApiShareListUntilShareID('unsupported_user', 'share-id')).resolves.toBe(false)
    expect(postSpy).not.toHaveBeenCalled()
  })
})
