import { beforeEach, describe, expect, it, vi } from 'vitest'
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

describe('local tag model', () => {
  beforeEach(() => {
    ;((globalThis as any).__quickTagStorage as Map<string, string>).clear()
    setActivePinia(createPinia())
    usePanTreeStore().mSaveUser('user-1', 'pikpak', '', '', '')
    usePanTreeStore().$patch({ drive_id: 'pikpak' })
  })

  it('tags a file into the dedicated tag model (defs + links), not the quick list', () => {
    PanDAL.updateLocalQuickTag([makeFile('file-1', '视频A.mp4')], '鹅冠红', '#df5659')

    const defs = PanDAL.getTagDefs()
    expect(defs).toHaveLength(1)
    expect(defs[0]).toMatchObject({ name: '鹅冠红', color: '#df5659' })

    const links = PanDAL.getTagLinks()
    expect(links).toHaveLength(1)
    expect(links[0]).toMatchObject({ drive_id: 'pikpak', file_id: 'file-1', kind: 'file', tagId: defs[0].id })

    // 标签与收藏/快捷方式解耦：快捷列表里没有这个纯标签文件
    expect(PanDAL.getQuickFileList()).toHaveLength(0)

    expect(PanDAL.getFileTags('pikpak', 'file-1', 'file')).toMatchObject([{ name: '鹅冠红', color: '#df5659' }])
  })

  it('clears the tag and removes the link when tagging with empty color', () => {
    const file = makeFile('file-1', '视频A.mp4')
    const panfileStore = usePanFileStore()
    panfileStore.$patch({ DriveID: 'pikpak', DirID: 'pikpak_root' })
    panfileStore.mSaveDirFileLoadingPart(0, { m_drive_id: 'pikpak', dirID: 'pikpak_root', items: [file] } as any, 1)
    panfileStore.mSaveDirFileLoadingFinish('pikpak', 'pikpak_root', [file], 1)

    PanDAL.updateLocalQuickTag([file], '鹅冠红', '#df5659')
    expect(PanDAL.getTagLinks()).toHaveLength(1)
    expect(file.description).toContain('cdf5659')

    PanDAL.updateLocalQuickTag([file], '', '')
    expect(PanDAL.getTagLinks()).toHaveLength(0)
    expect(PanDAL.getFileTags('pikpak', 'file-1', 'file')).toHaveLength(0)
    expect(file.description).not.toContain('cdf5659')
  })

  it('keeps a shortcut and its tag independent of each other', () => {
    const file = makeFile('file-2', '视频B.mp4')
    PanDAL.addLocalQuickFiles([file])
    expect(PanDAL.getQuickFileList()[0]).toMatchObject({ favorite: true })
    expect(PanDAL.getTagLinks()).toHaveLength(0)

    PanDAL.updateLocalQuickTag([file], '靛青', '#5966df')

    // 快捷方式仍在，标签独立存在
    expect(PanDAL.getQuickFileList()[0]).toMatchObject({ favorite: true })
    expect(PanDAL.getTagLinks()).toHaveLength(1)
    expect(PanDAL.getFileTags('pikpak', 'file-2', 'file')).toMatchObject([{ name: '靛青', color: '#5966df' }])
  })

  it('reuses the same tag definition for the same color across files', () => {
    PanDAL.updateLocalQuickTag([makeFile('file-a', 'A.mp4')], '鹅冠红', '#df5659')
    PanDAL.updateLocalQuickTag([makeFile('file-b', 'B.mp4')], '鹅冠红', '#df5659')

    expect(PanDAL.getTagDefs()).toHaveLength(1)
    expect(PanDAL.getTagLinks()).toHaveLength(2)
    expect(PanDAL.getTagLinks()[0].tagId).toBe(PanDAL.getTagLinks()[1].tagId)
  })

  it('tags the selected file through the real menu flow', async () => {
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

    const links = PanDAL.getTagLinks()
    expect(links.map((item) => item.file_id)).toContain('file-9')
    const def = PanDAL.getTagDefs().find((item) => item.id === links.find((link) => link.file_id === 'file-9')!.tagId)
    expect(def).toMatchObject({ name: '鹅冠红', color: '#df5659' })
  })

  it('loads tagged files in the color filter view', async () => {
    PanDAL.updateLocalQuickTag([makeFile('file-c1', '标记视频.mp4')], '鹅冠红', '#df5659')
    const panfileStore = usePanFileStore()
    panfileStore.$patch({ DriveID: 'pikpak', DirID: 'colorcdf5659 鹅冠红' })

    const ok = await PanDAL.GetDirFileList('user-1', 'pikpak', 'colorcdf5659 鹅冠红', '标记 · 鹅冠红', '', true)

    expect(ok).toBe(true)
    expect(panfileStore.ListDataShow.map((item) => item.file_id)).toContain('file-c1')
  })

  it('keeps the real file type in the color filter view so tagged videos can play', async () => {
    PanDAL.updateLocalQuickTag([makeFile('file-v1', '标签里的长视频.mp4')], '鹅冠红', '#df5659')
    const panfileStore = usePanFileStore()
    panfileStore.$patch({ DriveID: 'pikpak', DirID: 'colorcdf5659 鹅冠红' })

    await PanDAL.GetDirFileList('user-1', 'pikpak', 'colorcdf5659 鹅冠红', '标记 · 鹅冠红', '', true)

    const item = panfileStore.ListDataShow.find((entry) => entry.file_id === 'file-v1')
    expect(item).toBeTruthy()
    expect(item!.ext).toBe('mp4')
    expect(item!.category).toMatch(/^video/)
    expect(item!.icon).toBe('iconfile-video')
  })
})

describe('tag definition CRUD', () => {
  beforeEach(() => {
    ;((globalThis as any).__quickTagStorage as Map<string, string>).clear()
    setActivePinia(createPinia())
    usePanTreeStore().mSaveUser('user-1', 'pikpak', '', '', '')
    usePanTreeStore().$patch({ drive_id: 'pikpak' })
  })

  it('creates, renames and deletes a tag definition', () => {
    const def = PanDAL.createTag('工作', '#42a5f5')
    expect(PanDAL.getTagDefs()).toMatchObject([{ id: def.id, name: '工作', color: '#42a5f5' }])

    PanDAL.renameTag(def.id, '重要工作')
    expect(PanDAL.getTagDefs().find((item) => item.id === def.id)).toMatchObject({ name: '重要工作' })

    PanDAL.deleteTag(def.id)
    expect(PanDAL.getTagDefs()).toHaveLength(0)
  })

  it('deleting a tag also removes all of its file links', () => {
    PanDAL.updateLocalQuickTag([makeFile('file-x', 'X.mp4'), makeFile('file-y', 'Y.mp4')], '鹅冠红', '#df5659')
    const def = PanDAL.getTagDefs()[0]
    expect(PanDAL.getTagLinks()).toHaveLength(2)

    PanDAL.deleteTag(def.id)
    expect(PanDAL.getTagDefs()).toHaveLength(0)
    expect(PanDAL.getTagLinks()).toHaveLength(0)
  })

  it('removeFileTag only removes the targeted tag from a file', () => {
    const files = [makeFile('file-z', 'Z.mp4')]
    PanDAL.updateLocalQuickTag(files, '鹅冠红', '#df5659')
    const redDef = PanDAL.getTagDefs()[0]
    const blueDef = PanDAL.createTag('晴空蓝', '#42a5f5')
    PanDAL.setFileTag(files, blueDef.id)
    expect(PanDAL.getFileTags('pikpak', 'file-z', 'file')).toHaveLength(2)

    PanDAL.removeFileTag(files, redDef.id)
    expect(PanDAL.getFileTags('pikpak', 'file-z', 'file')).toMatchObject([{ id: blueDef.id }])
  })
})

describe('legacy tag migration', () => {
  beforeEach(() => {
    ;((globalThis as any).__quickTagStorage as Map<string, string>).clear()
    setActivePinia(createPinia())
    usePanTreeStore().mSaveUser('user-1', 'pikpak', '', '', '')
    usePanTreeStore().$patch({ drive_id: 'pikpak' })
  })

  it('migrates old FileQuick tag/tagColor entries into the new tag model once', () => {
    const storage = (globalThis as any).__quickTagStorage as Map<string, string>
    storage.set(
      'FileQuick-user-1',
      JSON.stringify([
        { key: 'quick:file:pikpak:old-1', drive_id: 'pikpak', drive_name: 'PikPak', title: '旧标签视频.mp4', file_id: 'old-1', parent_file_id: 'pikpak_root', kind: 'file', icon: 'iconfile-video', ext: 'mp4', tag: '鹅冠红', tagColor: '#df5659', favorite: false },
        { key: 'quick:file:pikpak:fav-1', drive_id: 'pikpak', drive_name: 'PikPak', title: '收藏视频.mp4', file_id: 'fav-1', parent_file_id: 'pikpak_root', kind: 'file', icon: 'iconfile-video', ext: 'mp4', tag: '', tagColor: '', favorite: true }
      ])
    )

    PanDAL.migrateLegacyTags('user-1')

    const defs = PanDAL.getTagDefs()
    expect(defs).toHaveLength(1)
    expect(defs[0]).toMatchObject({ name: '鹅冠红', color: '#df5659' })

    const links = PanDAL.getTagLinks()
    expect(links).toHaveLength(1)
    expect(links[0]).toMatchObject({ file_id: 'old-1', tagId: defs[0].id })

    // 收藏的快捷方式不受影响
    expect(PanDAL.getQuickFileList().some((item) => item.file_id === 'fav-1' && item.favorite)).toBe(true)

    // 再次迁移不会重复产生数据
    PanDAL.migrateLegacyTags('user-1')
    expect(PanDAL.getTagDefs()).toHaveLength(1)
    expect(PanDAL.getTagLinks()).toHaveLength(1)
  })

  it('migrated tags show up in the color filter view', async () => {
    const storage = (globalThis as any).__quickTagStorage as Map<string, string>
    storage.set(
      'FileQuick-user-1',
      JSON.stringify([{ key: 'quick:file:pikpak:old-2', drive_id: 'pikpak', drive_name: 'PikPak', title: '迁移视频.mp4', file_id: 'old-2', parent_file_id: 'pikpak_root', kind: 'file', icon: 'iconfile-video', ext: 'mp4', tag: '鹅冠红', tagColor: '#df5659', favorite: false }])
    )

    const panfileStore = usePanFileStore()
    panfileStore.$patch({ DriveID: 'pikpak', DirID: 'colorcdf5659 鹅冠红' })
    const ok = await PanDAL.GetDirFileList('user-1', 'pikpak', 'colorcdf5659 鹅冠红', '标记 · 鹅冠红', '', true)

    expect(ok).toBe(true)
    expect(panfileStore.ListDataShow.map((item) => item.file_id)).toContain('old-2')
  })
})
