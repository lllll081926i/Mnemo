import { getGofileRootId, gofileRequest } from './client'

const resolveGofileFolderId = async (userId: string, folderId: string) => folderId === 'gofile_root' ? await getGofileRootId(userId) : folderId

export const apiGofileMkdir = async (userId: string, parentId: string, name: string) => {
  const parentFolderId = await resolveGofileFolderId(userId, parentId)
  const response = await gofileRequest<{ id?: string }>(userId, '/contents/createFolder', {
    method: 'POST',
    body: JSON.stringify({ parentFolderId, folderName: name, modTime: Math.floor(Date.now() / 1000) })
  })
  return { file_id: response.data?.id || '', error: response.data?.id ? '' : 'GoFile 新建文件夹失败' }
}

export const apiGofileRename = async (userId: string, fileId: string, name: string) => {
  await gofileRequest(userId, `/contents/${encodeURIComponent(fileId)}/update`, { method: 'PUT', body: JSON.stringify({ attribute: 'name', attributeValue: name }) })
  return true
}

export const apiGofileDeleteBatch = async (userId: string, fileIds: string[]) => {
  await gofileRequest(userId, '/contents', { method: 'DELETE', body: JSON.stringify({ contentsId: fileIds.join(',') }) })
  return fileIds
}

export const apiGofileMoveBatch = async (userId: string, fileIds: string[], targetFolderId: string) => {
  const folderId = await resolveGofileFolderId(userId, targetFolderId)
  await gofileRequest(userId, '/contents/move', { method: 'PUT', body: JSON.stringify({ folderId, contentsId: fileIds.join(',') }) })
  return fileIds
}

export const apiGofileCopyBatch = async (userId: string, fileIds: string[], targetFolderId: string) => {
  const folderId = await resolveGofileFolderId(userId, targetFolderId)
  await gofileRequest(userId, '/contents/copy', { method: 'POST', body: JSON.stringify({ folderId, contentsId: fileIds.join(',') }) })
  return fileIds
}
