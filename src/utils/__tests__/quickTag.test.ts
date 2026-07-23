import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

// node 环境下补 localStorage / self（需在被测模块导入前就位）
vi.hoisted(() => {
  const storage = new Map<string, string>()
  ;(globalThis as any).__quickTagStorage = storage
  ;(globalThis as any).localStorage = {
    getItem: (key: string) => (storage.has(key) ? storage.get(key)! : null),
    setItem: (key: string, value: string) => void storage.set(key, String(value)),
    removeItem: (key: string) => void storage.delete(key),
    clear: () => storage.clear(),
    get length() {
      return storage.size
    },
    key: (index: number) => Array.from(storage.keys())[index] ?? null
  }
  ;(globalThis as any).self = globalThis
})

import PanDAL from '../../pan/pandal'
import usePanTreeStore from '../../pan/pantreestore'
import { usePanFileStore } from '../../store'
import { menuFileColorChange } from '../../pan/topbtns/topbtn'

vi.mock('../message', () => ({ default: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() } }))

const makeFile = (fileId: string, name: string) =>
  ({
    drive_id: 'pikpak',
    file_id: fileId,
    parent_file_id: 'pikpak_root',
    name,
    namesearch: name,
    path: '/' + name,
    ext: 'mp4',
    mime_type: '',
    mime_extension: '',
    category: 'video',
    icon: 'iconfile-video',
    size: 1024,
    sizeStr: '1 KB',
    time: 0,
    timeStr: '',
    starred: false,
    isDir: false,
    thumbnail: '',
    description: ''
  }) as any

describe('quick tag entries', () => {
  beforeEach(() => {
    ;((globalThis as any).__quickTagStorage as Map<string, string>).clear()
    setActivePinia(createPinia())
    usePanTreeStore().mSaveUser('user-1', 'pikpak', '', '', '')
    usePanTreeStore().$patch({ drive_id: 'pikpak' })
  })

  it('adds a tagged file into the quick list and quick tree', () => {
    PanDAL.updateLocalQuickTag([makeFile('file-1', '视频A.mp4')], '鹅冠红', '#df5659')

    const list = PanDAL.getQuickFileList()
    expect(list).toHaveLength(1)
    expect(list[0]).toMatchObject({ file_id: 'file-1', tag: '鹅冠红', tagColor: '#df5659', favorite: false })

    const tree = usePanTreeStore().quickData
    expect(tree).toHaveLength(1)
    expect(tree[0]).toMatchObject({ tag: '鹅冠红', tagColor: '#df5659' })
  })

  it('clears the tag and removes the entry when tagging with empty color', () => {
    PanDAL.updateLocalQuickTag([makeFile('file-1', '视频A.mp4')], '鹅冠红', '#df5659')
    PanDAL.updateLocalQuickTag([makeFile('file-1', '视频A.mp4')], '', '')
    expect(PanDAL.getQuickFileList()).toHaveLength(0)
  })

  it('tags the selected file through the real menu flow so it shows in 快捷方式', async () => {
    const pantreeStore = usePanTreeStore()
    const panfileStore = usePanFileStore()
    const file = makeFile('file-9', '长视频.mp4')
    pantreeStore.$patch({
      selectDir: { __v_skip: true, drive_id: 'pikpak', file_id: 'pikpak_root', album_id: '', parent_file_id: '', name: '网盘文件', namesearch: '', size: 0, time: 0, description: '' }
    })
    panfileStore.$patch({ DriveID: 'pikpak', DirID: 'pikpak_root' })
    panfileStore.mSaveDirFileLoadingPart(0, { m_drive_id: 'pikpak', dirID: 'pikpak_root', items: [file] } as any, 1)
    panfileStore.mSaveDirFileLoadingFinish('pikpak', 'pikpak_root', [file], 1)
    panfileStore.mMouseSelect('file-9', false, false)

    await menuFileColorChange(false, '#df5659')

    const list = PanDAL.getQuickFileList()
    expect(list.map((item) => item.file_id)).toContain('file-9')
    expect(list.find((item) => item.file_id === 'file-9')).toMatchObject({ tag: '鹅冠红', tagColor: '#df5659' })

    // 标记筛选视图（标记 · 鹅冠红）能列出刚打的标签文件
    const pantreeStoreAfter = usePanTreeStore()
    expect(pantreeStoreAfter.quickData.some((item) => item.tag === '鹅冠红')).toBe(true)
  })

  it('loads tagged files in the color filter view', async () => {
    PanDAL.updateLocalQuickTag([makeFile('file-c1', '标记视频.mp4')], '鹅冠红', '#df5659')
    const panfileStore = usePanFileStore()
    panfileStore.$patch({ DriveID: 'pikpak', DirID: 'colorcdf5659 鹅冠红' })

    const ok = await PanDAL.GetDirFileList('user-1', 'pikpak', 'colorcdf5659 鹅冠红', '标记 · 鹅冠红', '', true)

    expect(ok).toBe(true)
    expect(panfileStore.ListDataShow.map((item) => item.file_id)).toContain('file-c1')
  })
})
