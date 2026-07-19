import { describe, expect, it } from 'vitest'
import type { IUploadingUI } from '../../utils/dbupload'
import { buildCloud139PartInfos, buildCloud139UploadCreateBody } from '../uploadProtocol'

const fileui = {
  parent_file_id: 'cloud139_root',
  check_name_mode: 'overwrite',
  File: { name: 'folder/demo.bin', size: 250 * 1024 * 1024 }
} as IUploadingUI

describe('139 upload protocol', () => {
  it('creates stable sequential part metadata', () => {
    const parts = buildCloud139PartInfos(fileui.File.size, 100 * 1024 * 1024)
    expect(parts).toHaveLength(3)
    expect(parts[2]).toEqual({
      partNumber: 3,
      partSize: 50 * 1024 * 1024,
      parallelHashCtx: { partOffset: 200 * 1024 * 1024 }
    })
  })

  it('maps the root and conflict mode into the create request', () => {
    const body = buildCloud139UploadCreateBody(fileui, 'sha256', buildCloud139PartInfos(fileui.File.size))
    expect(body).toMatchObject({
      parentFileId: '/',
      name: 'demo.bin',
      contentHashAlgorithm: 'SHA256',
      fileRenameMode: 'auto_rename'
    })
  })
})
