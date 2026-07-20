import { apiGoogleDriveFileDetail, apiGoogleDriveFileList, GOOGLE_FOLDER_MIME, type GoogleDriveItem } from './dirfilelist'
import { googleDriveRequest } from './client'

export const apiGoogleDriveMkdir = async (userId: string, parentId: string, name: string) => {
  const item = await googleDriveRequest<{ id?: string }>(userId, '/files?supportsAllDrives=true', { method: 'POST', body: JSON.stringify({ name, mimeType: GOOGLE_FOLDER_MIME, parents: [parentId === 'gdrive_root' ? 'root' : parentId] }) })
  return { file_id: item.id || '', error: item.id ? '' : 'Google Drive 新建文件夹失败' }
}

export const apiGoogleDriveRename = async (userId: string, fileId: string, name: string) => {
  await googleDriveRequest(userId, `/files/${encodeURIComponent(fileId)}?supportsAllDrives=true`, { method: 'PATCH', body: JSON.stringify({ name }) })
  return true
}

export const apiGoogleDriveTrashBatch = async (userId: string, fileIds: string[]) => {
  const success: string[] = []
  for (const fileId of fileIds) {
    await googleDriveRequest(userId, `/files/${encodeURIComponent(fileId)}?supportsAllDrives=true`, { method: 'PATCH', body: JSON.stringify({ trashed: true }) })
    success.push(fileId)
  }
  return success
}

export const apiGoogleDriveRestoreBatch = async (userId: string, fileIds: string[]) => {
  const success: string[] = []
  for (const fileId of fileIds) {
    await googleDriveRequest(userId, `/files/${encodeURIComponent(fileId)}?supportsAllDrives=true`, { method: 'PATCH', body: JSON.stringify({ trashed: false }) })
    success.push(fileId)
  }
  return success
}

export const apiGoogleDriveDeleteBatch = async (userId: string, fileIds: string[]) => {
  const success: string[] = []
  for (const fileId of fileIds) {
    await googleDriveRequest(userId, `/files/${encodeURIComponent(fileId)}?supportsAllDrives=true`, { method: 'DELETE' })
    success.push(fileId)
  }
  return success
}

export const apiGoogleDriveMoveBatch = async (userId: string, fileIds: string[], targetParentId: string) => {
  const success: string[] = []
  for (const fileId of fileIds) {
    const item = await apiGoogleDriveFileDetail(userId, fileId)
    const params = new URLSearchParams({ addParents: targetParentId === 'gdrive_root' ? 'root' : targetParentId, supportsAllDrives: 'true' })
    if (item.parents?.length) params.set('removeParents', item.parents.join(','))
    await googleDriveRequest(userId, `/files/${encodeURIComponent(fileId)}?${params.toString()}`, { method: 'PATCH' })
    success.push(fileId)
  }
  return success
}

const copyGoogleDriveItem = async (userId: string, item: GoogleDriveItem, targetParentId: string): Promise<boolean> => {
  const parentId = targetParentId === 'gdrive_root' ? 'root' : targetParentId
  if (item.mimeType !== GOOGLE_FOLDER_MIME) {
    const copy = await googleDriveRequest<{ id?: string }>(userId, `/files/${encodeURIComponent(item.id)}/copy?supportsAllDrives=true`, { method: 'POST', body: JSON.stringify({ parents: [parentId] }) })
    return Boolean(copy.id)
  }

  const folder = await googleDriveRequest<{ id?: string }>(userId, '/files?supportsAllDrives=true', { method: 'POST', body: JSON.stringify({ name: item.name, mimeType: GOOGLE_FOLDER_MIME, parents: [parentId] }) })
  if (!folder.id) return false
  const children = await apiGoogleDriveFileList(userId, item.id)
  for (const child of children) {
    if (!await copyGoogleDriveItem(userId, child, folder.id)) return false
  }
  return true
}

export const apiGoogleDriveCopyBatch = async (userId: string, fileIds: string[], targetParentId: string) => {
  const success: string[] = []
  for (const fileId of fileIds) {
    const item = await apiGoogleDriveFileDetail(userId, fileId)
    if (await copyGoogleDriveItem(userId, item, targetParentId)) success.push(fileId)
  }
  return success
}
