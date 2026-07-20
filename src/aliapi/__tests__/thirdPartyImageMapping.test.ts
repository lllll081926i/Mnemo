import { describe, expect, it, vi } from 'vitest'

(globalThis as any).pinyinlite = (input: string) => input.split('').map((char) => [char])

vi.mock('../../user/userdal', () => ({
  default: {
    GetUserToken: () => ({}),
    GetUserTokenFromDB: async () => undefined
  }
}))

describe('retained provider image file mapping', () => {
  it('maps common image extensions to the image preview category', async () => {
    const { mapPikPakFileToAliModel } = await import('../../pikpak/dirfilelist')
    const { mapGuangyaFileToAliModel } = await import('../../guangya/dirfilelist')

    expect(mapPikPakFileToAliModel({
      id: 'file-id',
      parent_id: 'folder-id',
      kind: 'drive#file',
      name: 'photo.bmp',
      size: 1
    }, 'pikpak', 'folder-id').category).toBe('image')

    expect(mapGuangyaFileToAliModel({
      fileId: 'file-id',
      parentId: 'folder-id',
      fileName: 'photo.png',
      fileSize: 1,
      fileType: 1
    }, 'guangya', 'folder-id').category).toBe('image')
  })
})
