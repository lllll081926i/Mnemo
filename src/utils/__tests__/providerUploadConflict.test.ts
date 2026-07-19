import { describe, expect, it, vi } from 'vitest'
import type { IUploadingUI } from '../dbupload'
import { resolveProviderUploadConflict } from '../providerUploadConflict'

const fileui = (mode: string) => ({ check_name_mode: mode, File: { name: 'folder/demo.bin' } }) as IUploadingUI

describe('provider upload conflict policy', () => {
  it('refuses existing names without deleting them', async () => {
    const removeConflict = vi.fn(async () => true)
    const error = await resolveProviderUploadConflict(fileui('refuse'), {
      findConflict: async () => ({ id: '1', name: 'demo.bin' }),
      removeConflict
    })
    expect(error).toContain('已存在同名文件')
    expect(removeConflict).not.toHaveBeenCalled()
  })

  it('replaces conflicts only for declared modes', async () => {
    const removeConflict = vi.fn(async () => true)
    expect(
      await resolveProviderUploadConflict(fileui('ignore'), {
        replaceModes: ['ignore'],
        findConflict: async () => ({ id: '1', name: 'demo.bin' }),
        removeConflict
      })
    ).toBe('')
    expect(removeConflict).toHaveBeenCalledOnce()
  })
})
