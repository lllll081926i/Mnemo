import { describe, expect, it, vi } from 'vitest'
import * as GoogleClient from '../client'
import { GOOGLE_FOLDER_MIME, apiGoogleDriveFileList, apiGoogleDriveSearch, mapGoogleDriveItemToAliModel } from '../dirfilelist'
import { apiGoogleDriveCopyBatch, apiGoogleDriveDeleteBatch, apiGoogleDriveRestoreBatch } from '../filecmd'

describe('Google Drive file mapping', () => {
  it('maps folders and media without losing the account-scoped drive id', () => {
    const folder = mapGoogleDriveItemToAliModel({ id: 'folder', name: 'Photos', mimeType: GOOGLE_FOLDER_MIME }, 'gdrive:account-a', 'gdrive_root')
    const file = mapGoogleDriveItemToAliModel({ id: 'file', name: 'photo.jpg', mimeType: 'image/jpeg', size: '12', modifiedTime: '2026-01-01T00:00:00.000Z', md5Checksum: 'hash' }, 'gdrive:account-a', 'folder')

    expect(folder.isDir).toBe(true)
    expect(folder.drive_id).toBe('gdrive:account-a')
    expect(file.category).toBe('image')
    expect((file as any).content_hash).toBe('hash')
  })

  it('restores and permanently deletes selected trash items through Google Drive APIs', async () => {
    const request = vi.spyOn(GoogleClient, 'googleDriveRequest').mockResolvedValue({} as any)

    await expect(apiGoogleDriveRestoreBatch('gdrive_account-a', ['file one'])).resolves.toEqual(['file one'])
    await expect(apiGoogleDriveDeleteBatch('gdrive_account-a', ['file two'])).resolves.toEqual(['file two'])

    expect(request).toHaveBeenNthCalledWith(1, 'gdrive_account-a', '/files/file%20one?supportsAllDrives=true', { method: 'PATCH', body: JSON.stringify({ trashed: false }) })
    expect(request).toHaveBeenNthCalledWith(2, 'gdrive_account-a', '/files/file%20two?supportsAllDrives=true', { method: 'DELETE' })
    request.mockRestore()
  })

  it('copies folders recursively instead of sending a folder to the file-only copy endpoint', async () => {
    const request = vi.spyOn(GoogleClient, 'googleDriveRequest')
      .mockResolvedValueOnce({ id: 'folder', name: 'Folder', mimeType: GOOGLE_FOLDER_MIME })
      .mockResolvedValueOnce({ id: 'new-folder' })
      .mockResolvedValueOnce({ files: [{ id: 'child', name: 'child.txt', mimeType: 'text/plain' }] })
      .mockResolvedValueOnce({ id: 'new-child' })

    await expect(apiGoogleDriveCopyBatch('gdrive_account-a', ['folder'], 'gdrive_root')).resolves.toEqual(['folder'])

    expect(request).toHaveBeenNthCalledWith(2, 'gdrive_account-a', '/files?supportsAllDrives=true', { method: 'POST', body: JSON.stringify({ name: 'Folder', mimeType: GOOGLE_FOLDER_MIME, parents: ['root'] }) })
    expect(request).toHaveBeenNthCalledWith(4, 'gdrive_account-a', '/files/child/copy?supportsAllDrives=true', { method: 'POST', body: JSON.stringify({ parents: ['new-folder'] }) })
    request.mockRestore()
  })

  it('paginates file listings and safely escapes search query apostrophes', async () => {
    const request = vi.spyOn(GoogleClient, 'googleDriveRequest')
      .mockResolvedValueOnce({ files: [{ id: 'first', name: 'first.txt', mimeType: 'text/plain' }], nextPageToken: 'page-2' } as any)
      .mockResolvedValueOnce({ files: [{ id: 'second', name: 'second.txt', mimeType: 'text/plain' }] } as any)

    await expect(apiGoogleDriveFileList('gdrive_account-a', 'folder-a')).resolves.toHaveLength(2)
    const firstPath = request.mock.calls[0][1] as string
    const secondPath = request.mock.calls[1][1] as string
    expect(new URLSearchParams(firstPath.split('?')[1]).get('q')).toBe("'folder-a' in parents and trashed = false")
    expect(new URLSearchParams(secondPath.split('?')[1]).get('pageToken')).toBe('page-2')

    request.mockReset().mockResolvedValue({ files: [] } as any)
    await apiGoogleDriveSearch('gdrive_account-a', "owner's file")
    const searchPath = request.mock.calls[0][1] as string
    expect(new URLSearchParams(searchPath.split('?')[1]).get('q')).toBe("name contains 'owner\\'s file' and trashed = false")
    request.mockRestore()
  })
})
