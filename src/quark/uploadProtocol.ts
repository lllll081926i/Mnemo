export interface QuarkUploadPreData {
  task_id?: string
  finish?: boolean
  upload_id?: string
  obj_key?: string
  upload_url?: string
  fid?: string
  bucket?: string
  callback?: Record<string, any>
  auth_info?: string
}

export const buildQuarkUploadPreBody = (parentId: string, name: string, size: number, formatType: string, now = Date.now()) => ({
  dir_name: '',
  file_name: name,
  format_type: formatType,
  l_created_at: now,
  l_updated_at: now,
  pdir_fid: parentId === 'quark_root' || parentId.includes('root') ? '0' : parentId,
  size
})

export const buildQuarkPartAuthMeta = (pre: QuarkUploadPreData, contentType: string, date: string, partNumber: number) =>
  `PUT\n\n${contentType}\n${date}\nx-oss-date:${date}\nx-oss-user-agent:aliyun-sdk-js/6.6.1 Chrome 98.0.4758.80 on Windows 10 64-bit\n/${pre.bucket}/${pre.obj_key}?partNumber=${partNumber}&uploadId=${pre.upload_id}`

export const buildQuarkCompleteXml = (eTags: string[]) =>
  `<?xml version="1.0" encoding="UTF-8"?>\n<CompleteMultipartUpload>\n${eTags.map((eTag, index) => `<Part>\n<PartNumber>${index + 1}</PartNumber>\n<ETag>${eTag}</ETag>\n</Part>`).join('\n')}\n</CompleteMultipartUpload>`

export const buildQuarkCompleteAuthMeta = (pre: QuarkUploadPreData, contentMd5: string, callbackBase64: string, date: string) =>
  `POST\n${contentMd5}\napplication/xml\n${date}\nx-oss-callback:${callbackBase64}\nx-oss-date:${date}\nx-oss-user-agent:aliyun-sdk-js/6.6.1 Chrome 98.0.4758.80 on Windows 10 64-bit\n/${pre.bucket}/${pre.obj_key}?uploadId=${pre.upload_id}`
