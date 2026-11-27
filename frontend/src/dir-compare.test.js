import { describe, expect, it } from 'vitest'
import { compareDirectoryLists } from './dir-compare'

describe('双栏目录对比', () => {
  it('按名称 O(N) 归类仅左侧、仅右侧、差异、哈希一致与元数据一致', () => {
    const left = [
      { file_id: '1', name: 'only-left.txt', size: 10 },
      { file_id: '2', name: 'diff-size.bin', size: 100 },
      { file_id: '3', name: 'same-hash.zip', size: 500, content_hash_name: 'sha1', content_hash: 'aaa' },
      { file_id: '4', name: 'diff-hash.zip', size: 500, content_hash_name: 'sha1', content_hash: 'bbb' },
      { file_id: '5', name: 'same-size.jpg', size: 200 },
    ]
    const right = [
      { file_id: 'r2', name: 'diff-size.bin', size: 101 },
      { file_id: 'r3', name: 'same-hash.zip', size: 500, content_hash_name: 'SHA1', content_hash: 'AAA' },
      { file_id: 'r4', name: 'diff-hash.zip', size: 500, content_hash_name: 'sha1', content_hash: 'ccc' },
      { file_id: 'r5', name: 'same-size.jpg', size: 200 },
      { file_id: 'r6', name: 'only-right.txt', size: 20 },
    ]
    const result = compareDirectoryLists(left, right)
    expect(result.counts).toEqual({
      leftOnly: 1,
      rightOnly: 1,
      different: 2,
      hashMatch: 1,
      sameMeta: 1,
    })
    expect(result.rows.find(r => r.name === 'diff-hash.zip')?.status).toBe('different')
    expect(result.rows.find(r => r.name === 'same-hash.zip')?.status).toBe('hashMatch')
  })
})
