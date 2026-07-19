import { afterEach, describe, expect, it, vi } from 'vitest'

const { getUserToken } = vi.hoisted(() => ({ getUserToken: vi.fn() }))

vi.mock('../../user/userdal', () => ({
  default: { GetUserToken: getUserToken }
}))

vi.mock('../debuglog', () => ({
  default: { mSaveWarning: vi.fn() }
}))

import { apiDrive115OfflineCreate, apiDrive115OfflineProcess } from '../../cloud115/offline'
import { apiDrive115FileList } from '../../cloud115/dirfilelist'
import { isDrive115ApiSuccess, unwrapDrive115List } from '../drive115'

describe('115 offline download API', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    getUserToken.mockReset()
  })

  it('submits link tasks as form data', async () => {
    getUserToken.mockReturnValue({ access_token: 'token-115' })
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({
      state: true,
      code: 0,
      data: [{ state: true, code: 0, info_hash: 'hash-1', url: 'magnet:?xt=urn:btih:hash-1' }]
    }), { status: 200 }))

    const result = await apiDrive115OfflineCreate('115_user', 'magnet:?xt=urn:btih:hash-1', '12345')

    expect(result).toEqual({ taskIds: ['hash-1'], error: '' })
    expect(fetchMock).toHaveBeenCalledWith('https://proapi.115.com/open/offline/add_task_urls', expect.objectContaining({
      method: 'POST',
      headers: expect.objectContaining({ Authorization: 'Bearer token-115', 'Content-Type': 'application/x-www-form-urlencoded' })
    }))
    const request = fetchMock.mock.calls[0][1] as RequestInit
    expect(new URLSearchParams(request.body as string).toString()).toContain('urls=magnet%3A%3Fxt%3Durn%3Abtih%3Ahash-1')
    expect(new URLSearchParams(request.body as string).get('wp_path_id')).toBe('12345')
  })

  it('passes wp_path_id=0 when saving to the 115 root folder', async () => {
    getUserToken.mockReturnValue({ access_token: 'token-115' })
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({
      state: true,
      code: 0,
      data: [{ state: true, code: 0, info_hash: 'hash-root' }]
    }), { status: 200 }))

    await apiDrive115OfflineCreate('115_user', 'https://example.com/file.zip', 'drive115_root')

    const request = fetchMock.mock.calls[0][1] as RequestInit
    expect(new URLSearchParams(request.body as string).get('wp_path_id')).toBe('0')
  })

  it('maps task list status and progress', async () => {
    getUserToken.mockReturnValue({ access_token: 'token-115' })
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({
      state: true,
      code: 0,
      data: { tasks: [{ info_hash: 'hash-1', percentDone: 72, size: 2048, name: 'demo.mkv', status: 1 }] }
    }), { status: 200 }))

    await expect(apiDrive115OfflineProcess('115_user', 'hash-1')).resolves.toEqual({
      process: 72,
      status: 1,
      name: 'demo.mkv',
      size: 2048,
      error: ''
    })
  })

  it('accepts string success codes and unwraps supported file-list response shapes', () => {
    expect(isDrive115ApiSuccess({ status: true, code: '0' })).toBe(true)
    expect(isDrive115ApiSuccess({ state: true, code: 0 })).toBe(true)
    expect(isDrive115ApiSuccess({ status: false, code: '401' })).toBe(false)
    expect(unwrapDrive115List({ status: true, code: '0', data: [{ fid: '1' }] })).toEqual([{ fid: '1' }])
    expect(unwrapDrive115List({ state: true, code: 0, data: { data: [{ fid: '2' }] } })).toEqual([{ fid: '2' }])
    expect(unwrapDrive115List({ state: true, code: 0, data: { list: [{ fid: '3' }] } })).toEqual([{ fid: '3' }])
  })

  it('loads nested file lists with the required 115 directory query', async () => {
    getUserToken.mockReturnValue({ access_token: 'token-115' })
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({
      status: true,
      code: '0',
      data: { data: [{ fid: 'folder-1', pid: '0', fc: '0', fn: '目录' }] }
    }), { status: 200 }))

    await expect(apiDrive115FileList('115_user', 0, 200)).resolves.toEqual([{ fid: 'folder-1', pid: '0', fc: '0', fn: '目录' }])
    const requestUrl = new URL(fetchMock.mock.calls[0][0] as string)
    expect(requestUrl.searchParams.get('aid')).toBe('1')
    expect(requestUrl.searchParams.get('count_folders')).toBe('1')
    expect(requestUrl.searchParams.get('record_open_time')).toBe('1')
    expect(requestUrl.searchParams.get('show_dir')).toBe('1')
  })
})
