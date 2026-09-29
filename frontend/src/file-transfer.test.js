import { describe, expect, it, vi } from 'vitest'

vi.mock('./api', () => ({
  capsOf: () => ({ download: true, upload: true, move: true, copy: true, recycleBin: true }),
  pinFileSnapshot: vi.fn(async () => {}),
  copy: vi.fn(async (_user, _drive, ids) => ids),
  move: vi.fn(async (_user, _drive, ids) => ids),
  migrateFiles: vi.fn(async () => ({ id: 'migration' })),
}))

import { copy, migrateFiles } from './api'
import { transferFiles, transferProblem, uniqueFileSelection } from './file-transfer'
import { fileClipboard, setFileClipboard, retainClipboardFiles, clipboardPasteSource, rememberClipboardCopy, clipboardMigrationEvent } from './file-clipboard'

const account = { user_id: 'a', drive_id: 'disk' }
const files = [{ file_id: 'folder', isDir: true }, { file_id: 'child', parent_file_id: 'folder', name: 'child' }]

describe('文件操作', () => {
  it('下载选择中排除已选文件夹的子项', () => {
    expect(uniqueFileSelection(files).map(file => file.file_id)).toEqual(['folder'])
  })

  it('阻止将文件夹复制到自身及子目录', () => {
    const source = { account, dirId: 'root', files: [files[0]] }
    const target = { account, dirId: 'nested', path: [{ id: 'folder' }, { id: 'nested' }], mode: 'list' }
    expect(transferProblem(source, target, [])).toContain('自身及其子目录')
  })

  it('盘内操作逐项确认，跨账号只提交一次迁移', async () => {
    copy.mockClear()
    migrateFiles.mockClear()
    const source = { account, dirId: 'root', files: [{ file_id: 'one' }, { file_id: 'two' }] }
    const sameTarget = { account, dirId: 'elsewhere', path: [], mode: 'list' }
    expect((await transferFiles(source, sameTarget, [])).completed).toEqual(['one', 'two'])
    expect(copy).toHaveBeenCalledTimes(2)
    expect((await transferFiles(source, { ...sameTarget, account: { user_id: 'b', drive_id: 'disk' } }, [], true)).job.id).toBe('migration')
    expect(migrateFiles).toHaveBeenCalledTimes(1)
  })

  it('剪切仅消耗已确认的项目，迁移完成后清空', () => {
    fileClipboard.value = null
    expect(setFileClipboard({ account, dirId: 'root', path: [], files: [{ file_id: 'one' }, { file_id: 'two' }], mode: 'list' }, true)).toBe(true)
    const source = fileClipboard.value
    retainClipboardFiles(source, ['one'])
    expect(fileClipboard.value.files.map(file => file.file_id)).toEqual(['two'])
    fileClipboard.value = { ...fileClipboard.value, jobId: 'job', moving: true }
    clipboardMigrationEvent({ id: 'job', status: 'completed' })
    expect(fileClipboard.value).toBeNull()
  })

  it('复制部分成功后只对同一目标重试剩余项，其他目标仍可完整粘贴', () => {
    fileClipboard.value = null
    setFileClipboard({ account, dirId: 'root', files: [{ file_id: 'one' }, { file_id: 'two' }], mode: 'list' }, false)
    const target = { account, dirId: 'target' }
    rememberClipboardCopy(fileClipboard.value, target, ['one'])
    expect(clipboardPasteSource(fileClipboard.value, target).files.map(file => file.file_id)).toEqual(['two'])
    const other = { account, dirId: 'other' }
    expect(clipboardPasteSource(fileClipboard.value, other).files).toHaveLength(2)
    rememberClipboardCopy(fileClipboard.value, other, ['one', 'two'], true)
    expect(clipboardPasteSource(fileClipboard.value, target).files.map(file => file.file_id)).toEqual(['two'])
    rememberClipboardCopy(fileClipboard.value, target, ['two'], true)
    expect(clipboardPasteSource(fileClipboard.value, target).files).toHaveLength(2)
  })

  it('盘内复制失败后再次调用只执行未确认的项目', async () => {
    copy.mockReset()
    copy.mockResolvedValueOnce(['one']).mockRejectedValueOnce(new Error('暂时失败')).mockResolvedValueOnce(['two'])
    fileClipboard.value = null
    setFileClipboard({ account, dirId: 'root', files: [{ file_id: 'one' }, { file_id: 'two' }], mode: 'list' }, false)
    const target = { account, dirId: 'target', path: [], mode: 'list' }
    try {
      await transferFiles(fileClipboard.value, target, [])
      throw new Error('应当失败')
    } catch (error) {
      expect(error.completedIDs).toEqual(['one'])
      rememberClipboardCopy(fileClipboard.value, target, error.completedIDs)
    }
    await transferFiles(clipboardPasteSource(fileClipboard.value, target), target, [])
    expect(copy.mock.calls.map(call => call[2])).toEqual([['one'], ['two'], ['two']])
  })
})
