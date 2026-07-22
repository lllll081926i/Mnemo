import message from '../utils/message'
import { pikpakAuthHeaders } from './auth'

const API_URL = 'https://api-drive.mypikpak.com/drive/v1'

type PikPakCommandResp = {
  id?: string
  file?: { id?: string; name?: string; parent_id?: string; kind?: string }
  files?: Array<{ id?: string }>
  task?: unknown
  task_id?: string
  phase?: string
  error?: string
  error_description?: string
  message?: string
}

type PikPakTaskState = Pick<PikPakCommandResp, 'phase' | 'message' | 'error' | 'error_description'> | null

export const waitForPikPakTask = async (getTask: () => Promise<PikPakTaskState>, attempts = 20, delayMs = 500): Promise<{ success: boolean; error: string }> => {
  for (let attempt = 0; attempt < attempts; attempt++) {
    const task = await getTask()
    if (!task) return { success: false, error: '无法获取 PikPak 操作状态' }
    if (task.phase === 'PHASE_TYPE_COMPLETE') return { success: true, error: '' }
    if (task.phase === 'PHASE_TYPE_ERROR') return { success: false, error: task.error_description || task.message || task.error || 'PikPak 操作失败' }
    if (attempt < attempts - 1 && delayMs > 0) await new Promise((resolve) => setTimeout(resolve, delayMs))
  }
  return { success: false, error: 'PikPak 操作等待超时' }
}

const getToken = async (user_id: string) => {
  const { default: UserDAL } = await import('../user/userdal')
  let token = UserDAL.GetUserToken(user_id)
  if (!token?.access_token) {
    const dbToken = await UserDAL.GetUserTokenFromDB(user_id)
    if (dbToken) token = dbToken
  }
  return token
}

const requestPikPak = async <T = PikPakCommandResp>(
  user_id: string,
  url: string,
  init: RequestInit,
  errorTitle: string
): Promise<T | null> => {
  const token = await getToken(user_id)
  if (!token?.access_token) {
    message.error('未登录 PikPak')
    return null
  }
  try {
    const resp = await fetch(url, {
      ...init,
      headers: {
        ...pikpakAuthHeaders(token),
        ...(init.headers || {})
      }
    })
    const data = await resp.json().catch(() => undefined) as PikPakCommandResp | undefined
    if (!resp.ok || data?.error) {
      message.error(`${errorTitle}失败 ${data?.error_description || data?.message || data?.error || ''}`)
      return null
    }
    return data as T
  } catch (err: any) {
    message.error(`${errorTitle}失败 ${err?.message || ''}`)
    return null
  }
}

export const apiPikPakMkdir = async (user_id: string, parentId: string, name: string): Promise<{ file_id: string; error: string }> => {
  const data = await requestPikPak(user_id, `${API_URL}/files`, {
    method: 'POST',
    body: JSON.stringify({
      kind: 'drive#folder',
      name,
      parent_id: parentId === 'pikpak_root' ? undefined : parentId
    })
  }, '新建文件夹')
  const fileId = data?.file?.id || data?.id || ''
  return { file_id: fileId, error: fileId ? '' : '新建文件夹失败' }
}

export const apiPikPakRename = async (user_id: string, fileId: string, name: string): Promise<{ success: boolean; name: string; parent_file_id: string; isDir: boolean }> => {
  const data = await requestPikPak(user_id, `${API_URL}/files/${encodeURIComponent(fileId)}`, {
    method: 'PATCH',
    body: JSON.stringify({ name })
  }, '重命名')
  const file = data?.file || data as any
  return {
    success: !!file?.id || !!data,
    name: file?.name || name,
    parent_file_id: file?.parent_id || '',
    isDir: String(file?.kind || '').includes('folder')
  }
}

const batchCommand = async (user_id: string, url: string, ids: string[], title: string, bodyExtra: Record<string, unknown> = {}): Promise<string[]> => {
  if (!ids.length) return []
  const data = await requestPikPak(user_id, url, {
    method: 'POST',
    body: JSON.stringify({ ids, ...bodyExtra })
  }, title)
  if (!data) return []
  const taskId = data.task_id || (data.task as { id?: string } | undefined)?.id || ''
  if (!taskId) {
    message.error(`${title}失败 未返回任务 ID`)
    return []
  }
  const taskResult = await waitForPikPakTask(() => requestPikPak<PikPakCommandResp>(user_id, `${API_URL}/tasks/${encodeURIComponent(taskId)}`, { method: 'GET' }, `查询${title}状态`))
  if (!taskResult.success) {
    message.error(`${title}失败 ${taskResult.error}`)
    return []
  }
  return ids
}

export const apiPikPakTrashBatch = async (user_id: string, ids: string[]): Promise<string[]> => {
  return batchCommand(user_id, `${API_URL}/files:batchTrash`, ids, '放入回收站')
}

export const apiPikPakTrashRestore = async (user_id: string, ids: string[]): Promise<string[]> => {
  return batchCommand(user_id, `${API_URL}/files:batchUntrash`, ids, '从回收站还原')
}

export const apiPikPakTrashDelete = async (user_id: string, ids: string[]): Promise<string[]> => {
  return batchCommand(user_id, `${API_URL}/files:batchDelete`, ids, '彻底删除')
}

export const apiPikPakMoveBatch = async (user_id: string, ids: string[], toParentId: string): Promise<string[]> => {
  return batchCommand(user_id, `${API_URL}/files:batchMove`, ids, '移动', {
    to: toParentId === 'pikpak_root' ? {} : { parent_id: toParentId }
  })
}

export const apiPikPakCopyBatch = async (user_id: string, ids: string[], toParentId: string): Promise<string[]> => {
  return batchCommand(user_id, `${API_URL}/files:batchCopy`, ids, '复制', {
    to: toParentId === 'pikpak_root' ? {} : { parent_id: toParentId }
  })
}
