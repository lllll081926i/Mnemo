import type { IUploadingUI } from './dbupload'

export interface ProviderUploadConflictItem {
  id: string
  name: string
}

export interface ProviderUploadConflictOptions {
  findConflict: (name: string) => Promise<ProviderUploadConflictItem | undefined>
  removeConflict: (item: ProviderUploadConflictItem) => Promise<boolean>
  replaceModes?: string[]
}

export const resolveProviderUploadConflict = async (fileui: IUploadingUI, options: ProviderUploadConflictOptions) => {
  const mode = fileui.check_name_mode || 'refuse'
  const replaceModes = options.replaceModes || ['ignore', 'overwrite']
  if (mode === 'auto_rename' || (mode !== 'refuse' && !replaceModes.includes(mode))) return ''
  const conflict = await options.findConflict(fileui.File.name.split(/[\\/]/).pop() || fileui.File.name)
  if (!conflict) return ''
  if (mode === 'refuse') return `目标目录已存在同名文件：${conflict.name}`
  return (await options.removeConflict(conflict)) ? '' : `处理同名文件失败：${conflict.name}`
}
