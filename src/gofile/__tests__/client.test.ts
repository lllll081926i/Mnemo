import { describe, expect, it, vi } from 'vitest'
import { loginGofile } from '../auth'
import * as GoFileClient from '../client'
import { gofileRequestWithToken } from '../client'
import { apiGofileFileList, mapGofileItemToAliModel } from '../dirfilelist'
import { apiGofileCopyBatch, apiGofileMkdir, apiGofileMoveBatch } from '../filecmd'

describe('GoFile client', () => {
  it('authenticates API calls with a bearer token', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ status: 'ok', data: { id: 'account-id' } }) })
    vi.stubGlobal('fetch', fetchMock)

    await gofileRequestWithToken('account-token', '/accounts/getid')

    const [, init] = fetchMock.mock.calls[0]
    expect(init.headers.get('Authorization')).toBe('Bearer account-token')
  })

  it('creates an account-scoped token from account metadata', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({ status: 'ok', data: { id: 'account-a' } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ status: 'ok', data: { id: 'account-a', email: 'user@example.com', rootFolder: 'root-a', tier: 'premium', statsCurrent: { storage: 12 } } }) })
    vi.stubGlobal('fetch', fetchMock)

    const token = await loginGofile('account-token')

    expect(token.user_id).toBe('gofile_account-a')
    expect(token.default_drive_id).toBe('gofile:account-a')
    expect(token.provider_root_id).toBe('root-a')
    expect(token.access_token).toBe('account-token')
  })

  it('paginates folder listings and maps media files for preview', async () => {
    const request = vi.spyOn(GoFileClient, 'gofileRequest')
      .mockResolvedValueOnce({ data: { children: { first: { id: 'first', type: 'file', name: 'first.mp4', size: 12, mimetype: 'video/mp4' } } }, metadata: { hasNextPage: true } } as any)
      .mockResolvedValueOnce({ data: { children: { second: { id: 'second', type: 'folder', name: 'second' } } }, metadata: { hasNextPage: false } } as any)

    const items = await apiGofileFileList('gofile_account-a', 'folder-a')

    expect(items.map((item) => item.id)).toEqual(['first', 'second'])
    expect(request).toHaveBeenNthCalledWith(1, 'gofile_account-a', '/contents/folder-a?page=1&pageSize=1000')
    expect(request).toHaveBeenNthCalledWith(2, 'gofile_account-a', '/contents/folder-a?page=2&pageSize=1000')
    expect(mapGofileItemToAliModel(items[0], 'gofile:account-a', 'folder-a').category).toBe('video2')
    request.mockRestore()
  })

  it('sends folder operations to the GoFile content APIs', async () => {
    const request = vi.spyOn(GoFileClient, 'gofileRequest').mockResolvedValue({ data: { id: 'created-folder' } } as any)

    await expect(apiGofileMkdir('gofile_account-a', 'parent-a', 'New Folder')).resolves.toEqual({ file_id: 'created-folder', error: '' })
    await apiGofileMoveBatch('gofile_account-a', ['file-a', 'file-b'], 'target-a')
    await apiGofileCopyBatch('gofile_account-a', ['file-a'], 'target-a')

    expect(request).toHaveBeenNthCalledWith(1, 'gofile_account-a', '/contents/createFolder', expect.objectContaining({ method: 'POST' }))
    expect(JSON.parse((request.mock.calls[0][2] as RequestInit).body as string)).toMatchObject({ parentFolderId: 'parent-a', folderName: 'New Folder' })
    expect(request).toHaveBeenNthCalledWith(2, 'gofile_account-a', '/contents/move', { method: 'PUT', body: JSON.stringify({ folderId: 'target-a', contentsId: 'file-a,file-b' }) })
    expect(request).toHaveBeenNthCalledWith(3, 'gofile_account-a', '/contents/copy', { method: 'POST', body: JSON.stringify({ folderId: 'target-a', contentsId: 'file-a' }) })
    request.mockRestore()
  })
})
