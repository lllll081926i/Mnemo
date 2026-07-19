import { describe, expect, it } from 'vitest'
import { buildQuarkCompleteAuthMeta, buildQuarkCompleteXml, buildQuarkPartAuthMeta, buildQuarkUploadPreBody } from '../uploadProtocol'

describe('Quark upload protocol', () => {
  const pre = {
    task_id: 'task',
    upload_id: 'upload',
    obj_key: 'folder/demo.bin',
    upload_url: 'https://oss.example.com',
    fid: 'fid',
    bucket: 'bucket',
    callback: {},
    auth_info: 'auth'
  }

  it('maps the virtual root to Quark pdir 0', () => {
    expect(buildQuarkUploadPreBody('quark_root', 'demo.bin', 20, 'application/octet-stream', 123)).toMatchObject({
      pdir_fid: '0',
      file_name: 'demo.bin',
      size: 20,
      l_created_at: 123,
      l_updated_at: 123
    })
  })

  it('builds the exact OSS authorization paths for parts and completion', () => {
    expect(buildQuarkPartAuthMeta(pre, 'application/octet-stream', 'date', 2)).toContain('/bucket/folder/demo.bin?partNumber=2&uploadId=upload')
    expect(buildQuarkCompleteAuthMeta(pre, 'md5', 'callback', 'date')).toContain('/bucket/folder/demo.bin?uploadId=upload')
    expect(buildQuarkCompleteXml(['"etag-1"', '"etag-2"'])).toContain('<PartNumber>2</PartNumber>\n<ETag>"etag-2"</ETag>')
  })
})
