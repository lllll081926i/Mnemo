import { describe, expect, it } from 'vitest'
import { GOOGLE_FOLDER_MIME, mapGoogleDriveItemToAliModel } from '../dirfilelist'

describe('Google Drive file mapping', () => {
  it('maps folders and media without losing the account-scoped drive id', () => {
    const folder = mapGoogleDriveItemToAliModel({ id: 'folder', name: 'Photos', mimeType: GOOGLE_FOLDER_MIME }, 'gdrive:account-a', 'gdrive_root')
    const file = mapGoogleDriveItemToAliModel({ id: 'file', name: 'photo.jpg', mimeType: 'image/jpeg', size: '12', modifiedTime: '2026-01-01T00:00:00.000Z', md5Checksum: 'hash' }, 'gdrive:account-a', 'folder')

    expect(folder.isDir).toBe(true)
    expect(folder.drive_id).toBe('gdrive:account-a')
    expect(file.category).toBe('image')
    expect((file as any).content_hash).toBe('hash')
  })
})
