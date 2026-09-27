import { capsOf, copy, move, migrateFiles, pinFileSnapshot } from './api'

export function sameSpace(source, target) {
  return source?.account?.user_id === target?.account?.user_id && source?.account?.drive_id === target?.account?.drive_id
}

export function transferProblem(source, target, providers, moving = false) {
  if (!source?.account || !target?.account || !source.files?.length || target.mode !== 'list') return '请选择文件，并打开目标目录'
  if (sameSpace(source, target)) {
    if (source.dirId === target.dirId || source.files.some(file => file.parent_file_id === target.dirId)) return '来源与目标目录相同'
    if (source.files.some(file => file.isDir && (file.file_id === target.dirId || target.path?.some(part => part.id === file.file_id) || (file.file_id.startsWith('/') && target.dirId.startsWith(file.file_id.replace(/\/$/, '') + '/'))))) return '不能移动或复制到自身及其子目录'
    if (!capsOf(source.account, providers)[moving ? 'move' : 'copy']) return moving ? '该网盘不支持盘内移动' : '该网盘不支持盘内复制'
  } else {
    const sourceCaps = capsOf(source.account, providers), targetCaps = capsOf(target.account, providers)
    if (!sourceCaps.download) return '源网盘不支持下载，无法迁移'
    if (!targetCaps.upload && !targetCaps.rapidUploadHashes?.length) return '目标网盘不支持上传，无法迁移'
    if (moving && !sourceCaps.recycleBin && !sourceCaps.permanentDelete) return '源网盘不支持移除文件，请改用复制'
  }
  return ''
}

export async function transferFiles(source, target, providers, moving = false) {
  const problem = transferProblem(source, target, providers, moving)
  if (problem) throw new Error(problem)
  for (const file of source.files) {
    await pinFileSnapshot(source.account.user_id, source.account.drive_id, { ...file, parent_file_id: file.parent_file_id || source.dirId })
  }
  const ids = source.files.map(file => file.file_id)
  if (sameSpace(source, target)) {
    const completed = []
    for (const id of ids) {
      try {
        const result = await (moving ? move : copy)(source.account.user_id, source.account.drive_id, [id], target.dirId)
        if (!Array.isArray(result) || result.length !== 1) throw new Error('网盘未确认操作完成')
        completed.push(id)
      } catch (error) {
        const failure = error instanceof Error ? error : new Error(String(error))
        failure.completedIDs = completed
        throw failure
      }
    }
    return { completed, queued: false }
  }
  const job = await migrateFiles(source.account.user_id, source.account.drive_id, target.account.user_id, target.account.drive_id, target.dirId, ids, moving)
  return { queued: true, job, completed: [] }
}

export function uniqueFileSelection(files) {
  const unique = [...new Map(files.filter(file => file?.file_id).map(file => [file.file_id, file])).values()]
  const byID = new Map(unique.map(file => [file.file_id, file]))
  return unique.filter(file => {
    const seen = new Set([file.file_id])
    let parent = file.parent_file_id
    while (parent && !seen.has(parent)) {
      seen.add(parent)
      const selectedParent = byID.get(parent)
      if (!selectedParent) break
      if (selectedParent.isDir) return false
      parent = selectedParent.parent_file_id
    }
    return !unique.some(folder => folder.isDir && folder !== file && folder.path && file.path && file.path.startsWith(folder.path.replace(/\/$/, '') + '/'))
  })
}
