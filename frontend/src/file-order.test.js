import { describe, expect, it } from 'vitest'
import { compareFileNames, sortedDirectories } from './file-order'

describe('目录自然排序', () => {
  it('按数字顺序排列目录，保留原始列表', () => {
    const source = [{ name: '目录10', isDir: true }, { name: '文件1.txt' }, { name: '目录2', isDir: true }, { name: '目录1', isDir: true }]
    expect(sortedDirectories(source).map(file => file.name)).toEqual(['目录1', '目录2', '目录10'])
    expect(source[0].name).toBe('目录10')
    expect(source).toHaveLength(4)
  })
  it('同名目录使用 ID 保持确定顺序', () => {
    const files = [{ name: '资料', file_id: 'b' }, { name: '资料', file_id: 'a' }]
    expect(files.sort(compareFileNames).map(file => file.file_id)).toEqual(['a', 'b'])
  })
})
