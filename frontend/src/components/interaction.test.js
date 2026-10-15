import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import Modal from './Modal.vue'
import UiSelect from './UiSelect.vue'
import ContextMenu from './ContextMenu.vue'
import TreeNode from './TreeNode.vue'
import AccountRail from './AccountRail.vue'
import PreviewModal from './PreviewModal.vue'
import QuickOpen from './QuickOpen.vue'
import PlayerPanel from './PlayerPanel.vue'
import ShareView from '../views/ShareView.vue'
import SyncView from '../views/SyncView.vue'
import SelectDirModal from './SelectDirModal.vue'
import SettingsView from '../views/SettingsView.vue'
import UpdateModal from './UpdateModal.vue'
import * as appearance from '../appearance'
import App from '../App.vue'
import WorkspaceView from '../views/WorkspaceView.vue'
import PanView from '../views/PanView.vue'
import TransferView from '../views/TransferView.vue'

const api = vi.hoisted(() => ({
  listDir: vi.fn().mockResolvedValue([]),
  listTrash: vi.fn().mockResolvedValue([]),
  search: vi.fn().mockResolvedValue([]),
  listDirSilently: vi.fn((...args) => api.listDir(...args)),
  GetDirectoryCache: vi.fn().mockResolvedValue(null),
  SaveDirectoryCache: vi.fn().mockResolvedValue(undefined),
  ListFavorites: vi.fn().mockResolvedValue([]),
  AddFavorite: vi.fn().mockResolvedValue(undefined),
  RemoveFavorite: vi.fn().mockResolvedValue(undefined),
  RestoreFavorite: vi.fn().mockResolvedValue(undefined),
  favorite: vi.fn().mockResolvedValue([]),
  onFileDrop: vi.fn(() => () => {}),
  formatTimeParts: vi.fn(() => ({ date: '', clock: '' })),
  CheckUpdate: vi.fn(),
  GetUpdateStatus: vi.fn(),
  DownloadUpdate: vi.fn(),
  CancelUpdate: vi.fn(),
  ApplyUpdate: vi.fn(),
  listAccounts: vi.fn(),
  listProviders: vi.fn(),
  prewarmRootDirectories: vi.fn().mockResolvedValue({ cached: 0, warmed: 0, failed: 0 }),
  setAccountCustomMeta: vi.fn(),
  login: vi.fn(),
  ListShareHistory: vi.fn(),
  ListSyncConfigs: vi.fn(),
  ListSyncRunStates: vi.fn().mockResolvedValue({}),
  RunDiagnostics: vi.fn(),
  ExportDiagnostics: vi.fn(),
  GetSettings: vi.fn(),
  GetDownloadDirectory: vi.fn().mockResolvedValue('D:/系统下载'),
  OpenDownloadDirectory: vi.fn().mockResolvedValue(undefined),
  SaveSettings: vi.fn(),
  RespondQuit: vi.fn().mockResolvedValue(undefined),
  GetLogPath: vi.fn(),
  ClearCache: vi.fn(),
  RevealInFolder: vi.fn(),
  ClearLogs: vi.fn(),
  ExportLogs: vi.fn(),
  ListRunningSyncIDs: vi.fn(),
  SaveSyncConfig: vi.fn(),
  DeleteSyncConfig: vi.fn(),
  RunSync: vi.fn(),
  CancelSync: vi.fn(),
  PickDirectory: vi.fn(),
  capsOf: vi.fn(),
  importShare: vi.fn(),
  saveImportedShare: vi.fn(),
  listShareImportReceipts: vi.fn().mockResolvedValue([]),
  refreshShareImportReceipt: vi.fn(),
  cancelShare: vi.fn(),
  saveMounted: vi.fn(),
  validateMountedWrite: vi.fn(),
  SendGuangyaSms: vi.fn(),
  SendPan139SMS: vi.fn(),
  SendPan189SMS: vi.fn(),
  SendThunderSMS: vi.fn(),
  providerIconUrl: vi.fn(() => ''),
  OpenBrowser: vi.fn(),
  OpenFile: vi.fn(),
  onEvent: vi.fn(() => () => {}),
  ClosePikPakCaptcha: vi.fn(),
  ShowPikPakCaptcha: vi.fn(),
  ShowThunderVerification: vi.fn().mockResolvedValue(true),
  CancelThunderLogin: vi.fn().mockResolvedValue(undefined),
  refreshAccountSilently: vi.fn(),
  refreshAccountNow: vi.fn(),
  accountName: vi.fn((account) => account?.user_id || ''),
  providerMetaOf: vi.fn(() => ({ key: 'webdav', label: 'WebDAV' })),
  providerOf: vi.fn((id) => String(id).split(':')[0]),
  formatBytes: vi.fn((value) => `${value} B`),
  formatSpeed: vi.fn((value) => `${value} B/s`),
  PreviewURL: vi.fn(),
  cachedPreviewImageURL: vi.fn().mockRejectedValue(new Error('缓存不可用')),
  PinFileSnapshot: vi.fn().mockResolvedValue(undefined),
  openKindOf: vi.fn(file => file.name.endsWith('.wav') ? 'audio' : 'image'),
  formatTime: vi.fn(() => ''),
  saveCloudText: vi.fn(),
  copyText: vi.fn(),
  iconOf: vi.fn(() => 'image'),
  getPlayCursor: vi.fn().mockResolvedValue(0),
  savePlayCursor: vi.fn().mockResolvedValue(undefined),
  playVideo: vi.fn(),
  playVideoQuality: vi.fn(),
  pinFileSnapshot: vi.fn(),
  getSettings: vi.fn(),
  previewUrl: vi.fn(),
  download: vi.fn(),
  PickFiles: vi.fn(),
  validateUploadFiles: vi.fn(),
  uploadFiles: vi.fn(),
  ListDownloads: vi.fn().mockResolvedValue([]),
  ListUploads: vi.fn().mockResolvedValue([]),
  PauseDownload: vi.fn().mockResolvedValue(undefined),
  ResumeDownload: vi.fn().mockResolvedValue(undefined),
  CancelDownload: vi.fn().mockResolvedValue(undefined),
  ListMigrateJobs: vi.fn().mockResolvedValue([]),
  ListOfflineTasks: vi.fn().mockResolvedValue([]),
  refreshOfflineTasks: vi.fn().mockResolvedValue([]),
  DownloadURL: vi.fn().mockResolvedValue(undefined),
  OfflineDownload: vi.fn().mockResolvedValue(undefined),
  DeleteOfflineTask: vi.fn().mockResolvedValue(undefined),
  VerifyMigration: vi.fn().mockResolvedValue([]),
  EventsOn: vi.fn(() => () => {}),
  trash: vi.fn(),
  remove: vi.fn(),
  restore: vi.fn(),
  migrateFiles: vi.fn().mockResolvedValue({ id: 'migration-test', status: 'pending' }),
  move: vi.fn().mockResolvedValue(['folder', 'file']),
  copy: vi.fn().mockResolvedValue(['folder', 'file']),
  DeleteDirectoryCache: vi.fn().mockResolvedValue(undefined),
}))

const tsMock = vi.hoisted(() => ({ createPlayer: vi.fn(), isSupported: vi.fn(() => true), Events: { ERROR: 'error' }, ErrorTypes: { NETWORK_ERROR: 'network' } }))
vi.mock('mpegts.js', () => ({ default: tsMock }))

vi.mock('../api', () => api)
vi.mock('../logger', () => ({
  debug: vi.fn(),
  info: vi.fn(),
  warn: vi.fn(),
  error: vi.fn(),
  errorText: vi.fn((value) => String(value)),
  configKeys: vi.fn(() => []),
  installGlobalErrorLogging: vi.fn(() => () => {}),
}))

import LoginModal from './LoginModal.vue'
import AccountAvatar from './AccountAvatar.vue'

const storage = new Map()
Object.defineProperty(globalThis, 'localStorage', {
  configurable: true,
  value: {
    getItem: (key) => storage.has(key) ? storage.get(key) : null,
    setItem: (key, value) => storage.set(String(key), String(value)),
    removeItem: (key) => storage.delete(String(key)),
    clear: () => storage.clear(),
  },
})

const wrappers = []
beforeEach(() => {
  api.OpenBrowser.mockReset().mockResolvedValue(undefined)
  api.OpenFile.mockReset().mockResolvedValue(undefined)
  api.RevealInFolder.mockReset().mockResolvedValue(undefined)
  api.copyText.mockReset().mockResolvedValue(true)
  api.download.mockReset().mockResolvedValue(undefined)
  api.PickFiles.mockReset().mockResolvedValue([])
  api.validateUploadFiles.mockReset().mockResolvedValue(undefined)
  api.uploadFiles.mockReset().mockResolvedValue(undefined)
  api.ListDownloads.mockReset().mockResolvedValue([])
  api.ListUploads.mockReset().mockResolvedValue([])
  api.PauseDownload.mockReset().mockResolvedValue(undefined)
  api.ResumeDownload.mockReset().mockResolvedValue(undefined)
  api.CancelDownload.mockReset().mockResolvedValue(undefined)
  api.ListMigrateJobs.mockReset().mockResolvedValue([])
  api.ListOfflineTasks.mockReset().mockResolvedValue([])
  api.refreshOfflineTasks.mockReset().mockResolvedValue([])
  api.DownloadURL.mockReset().mockResolvedValue(undefined)
  api.OfflineDownload.mockReset().mockResolvedValue(undefined)
  api.DeleteOfflineTask.mockReset().mockResolvedValue(undefined)
  api.VerifyMigration.mockReset().mockResolvedValue([])
  api.trash.mockReset().mockResolvedValue(undefined)
  api.remove.mockReset().mockResolvedValue(undefined)
  api.restore.mockReset().mockResolvedValue(undefined)
  api.ListFavorites.mockReset().mockResolvedValue([])
  api.AddFavorite.mockReset().mockResolvedValue(undefined)
  api.RemoveFavorite.mockReset().mockResolvedValue(undefined)
  api.openKindOf.mockImplementation(file => file.name.endsWith('.wav') ? 'audio' : 'image')
  vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => {})
  vi.spyOn(HTMLMediaElement.prototype, 'load').mockImplementation(() => {})
})

describe('网盘工具栏状态', () => {
  const account = { user_id: 'pikpak_toolbar', drive_id: 'drive' }
  const file = { file_id: 'file', name: 'photo.jpg', isDir: false, size: 42 }

  it('未选中时仅显示目录操作，选中后切换为文件操作', async () => {
    api.capsOf.mockReturnValue({ search: true, createFolder: true, upload: true, download: true, createShare: true, move: true, copy: true, recycleBin: true })
    api.listDir.mockResolvedValue([file])
    const wrapper = mountAttached(PanView, { props: { account } })
    await flushPromises()
    const toolbar = wrapper.get('.pan-file-toolbar')
    expect(toolbar.text()).toContain('新建')
    expect(toolbar.text()).toContain('上传')
    expect(toolbar.text()).not.toContain('快速筛选')
    await wrapper.get('.fileitem').trigger('click')
    expect(toolbar.text()).toContain('已选 1 项')
    expect(toolbar.text()).toContain('下载')
    expect(toolbar.text()).toContain('更多')
    expect(toolbar.text()).not.toContain('新建')
    expect(toolbar.text()).not.toContain('上传')
    expect(toolbar.text()).not.toContain('名称')
    await toolbar.find('.sel-mgmt .dropbtn').trigger('click')
    expect([...document.querySelectorAll('[role="menuitem"]')].map(item => item.textContent)).toEqual(expect.arrayContaining(['全选', '反向选择', '取消选择']))
    ;[...document.querySelectorAll('[role="menuitem"]')].find(item => item.textContent.includes('取消选择')).click()
    await nextTick()
    expect(toolbar.text()).toContain('新建')
  })

  it('回收站选中时不显示普通文件操作', async () => {
    api.capsOf.mockReturnValue({ trashView: true, trashRestore: true, permanentDelete: true, download: true, createShare: true, recycleBin: true })
    api.listDir.mockResolvedValue([])
    api.listTrash.mockResolvedValue([file])
    const wrapper = mountAttached(PanView, { props: { account } })
    await flushPromises()
    await wrapper.findAll('.tree-node').find(node => node.text().includes('回收站')).trigger('click')
    await flushPromises()
    await wrapper.get('.fileitem').trigger('click')
    const toolbar = wrapper.get('.pan-file-toolbar')
    expect(toolbar.text()).toContain('还原')
    expect(toolbar.text()).toContain('彻底删除')
    expect(toolbar.text()).not.toContain('下载')
    expect(toolbar.text()).not.toContain('分享')
    expect(toolbar.text()).not.toContain('更多')
  })

  it('根目录不保留无效后退键，搜索页只保留一个搜索入口', async () => {
    api.capsOf.mockReturnValue({ search: true })
    api.listDir.mockResolvedValue([file])
    const wrapper = mountAttached(PanView, { props: { account } })
    await flushPromises()
    const toolbar = wrapper.get('.pan-file-toolbar')
    expect(toolbar.find('[title="后退 (Backspace)"]').exists()).toBe(false)
    await toolbar.get('[title="全盘搜索 (Ctrl/⌘+F)"]').trigger('click')
    expect(toolbar.find('[title="全盘搜索 (Ctrl/⌘+F)"]').exists()).toBe(false)
    expect(toolbar.findAll('.search-quick')).toHaveLength(1)
    await toolbar.get('.search-quick').setValue('photo')
    await toolbar.get('.search-quick').trigger('keydown.enter')
    await flushPromises()
    expect(api.search).toHaveBeenCalledWith(account.user_id, account.drive_id, 'photo')
  })
})

describe('网盘文件选择与目录切换', () => {
  const account = { user_id: 'webdav:selection', drive_id: 'selection' }
  const files = [
    { file_id: 'a', name: 'A.txt', isDir: false },
    { file_id: 'b', name: 'B.txt', isDir: false },
    { file_id: 'c', name: 'C.txt', isDir: false },
  ]

  it('新目录尚未返回时隐藏旧文件，保留加载占位', async () => {
    let finishDirectory
    api.capsOf.mockReturnValue({})
    api.listDir.mockImplementation((_user, _drive, dir) => dir === 'root'
      ? Promise.resolve(files)
      : new Promise(resolve => { finishDirectory = resolve }))
    const wrapper = mountAttached(PanView, { props: { account } })
    await flushPromises()
    expect(wrapper.findAll('.fileitem')).toHaveLength(3)
    const navigation = wrapper.vm.navigate({ dirId: 'new-directory', name: '新目录' })
    await nextTick()
    expect(wrapper.findAll('.fileitem')).toHaveLength(0)
    expect(wrapper.find('.skeleton-list').exists()).toBe(true)
    finishDirectory([{ file_id: 'new-file', name: '新文件.txt', isDir: false }])
    await navigation
    await flushPromises()
    expect(wrapper.text()).toContain('新文件.txt')
    expect(wrapper.text()).not.toContain('A.txt')
  })

  it('单击保持选中，Shift 继续沿原锚点扩展，空白和 Escape 可取消', async () => {
    api.capsOf.mockReturnValue({})
    api.listDir.mockResolvedValue(files)
    const wrapper = mountAttached(PanView, { props: { account } })
    await flushPromises()
    const rows = wrapper.findAll('.fileitem')
    await rows[0].trigger('click')
    await rows[0].trigger('click')
    expect(wrapper.findAll('.fileitem.selected')).toHaveLength(1)
    await rows[2].trigger('click', { shiftKey: true })
    expect(wrapper.findAll('.fileitem.selected')).toHaveLength(3)
    await rows[1].trigger('click', { shiftKey: true })
    expect(wrapper.findAll('.fileitem.selected')).toHaveLength(2)
    await wrapper.get('.file-list').trigger('click')
    expect(wrapper.findAll('.fileitem.selected')).toHaveLength(0)
    await rows[0].trigger('click')
    window.dispatchEvent(new KeyboardEvent('keydown', { code: 'Escape', bubbles: true }))
    await nextTick()
    expect(wrapper.findAll('.fileitem.selected')).toHaveLength(0)
  })

  it('同目录刷新保留选中并更新文件资料，点击列表留白可清除选择', async () => {
    api.capsOf.mockReturnValue({})
    api.listDir.mockResolvedValue([files[0]])
    const wrapper = mountAttached(PanView, { props: { account } })
    await flushPromises()
    await wrapper.get('.fileitem').trigger('click')
    api.listDir.mockResolvedValue([{ ...files[0], name: '更新后的文件.txt', size: 99 }])
    await wrapper.vm.refresh()
    await flushPromises()
    expect(wrapper.vm.snapshot().files).toMatchObject([{ name: '更新后的文件.txt', size: 99 }])
    expect(wrapper.findAll('.fileitem.selected')).toHaveLength(1)
    await wrapper.get('.file-list').trigger('click')
    expect(wrapper.vm.snapshot().files).toHaveLength(0)
  })

  it('右键多项文件下载整个选择集，文件夹下载加入目录任务', async () => {
    api.capsOf.mockReturnValue({ download: true, rename: true })
    api.listDir.mockResolvedValue(files)
    api.download.mockResolvedValue(undefined)
    const wrapper = mountAttached(PanView, { props: { account } })
    await flushPromises()
    const rows = wrapper.findAll('.fileitem')
    await rows[0].trigger('click')
    await rows[1].trigger('click', { ctrlKey: true })
    await rows[0].trigger('contextmenu')
    const menuItems = [...document.querySelectorAll('.ctx-menu [role="menuitem"]')]
    expect(menuItems.some(item => item.textContent.trim() === '重命名')).toBe(false)
    menuItems.find(item => item.textContent.includes('下载')).click()
    await flushPromises()
    expect(api.download).toHaveBeenCalledTimes(2)
    expect(api.download.mock.calls.map(call => call[2].file_id)).toEqual(['a', 'b'])
    api.listDir.mockResolvedValue([{ file_id: 'folder', name: '文件夹', isDir: true }])
    await wrapper.vm.refresh()
    await flushPromises()
    expect(wrapper.find('.pan-file-toolbar').text()).not.toContain('下载')
    await wrapper.get('.fileitem').trigger('contextmenu')
    const folderDownload = [...document.querySelectorAll('.ctx-menu [role="menuitem"]')].find(item => item.textContent.trim() === '下载')
    expect(folderDownload).toBeDefined()
    folderDownload.click()
    await flushPromises()
    expect(api.download).toHaveBeenCalledTimes(3)
    expect(api.download).toHaveBeenLastCalledWith(account.user_id, account.drive_id, expect.objectContaining({ file_id: 'folder', isDir: true }))
  })

  it('批量下载单项失败后继续入队其余文件，并报告实际成功数', async () => {
    api.capsOf.mockReturnValue({ download: true })
    api.listDir.mockResolvedValue(files)
    api.download.mockImplementation((_user, _drive, file) => file.file_id === 'a' ? Promise.reject(new Error('网络故障')) : Promise.resolve())
    const wrapper = mountAttached(PanView, { props: { account } })
    await flushPromises()
    await wrapper.findAll('.fileitem')[0].trigger('click')
    await wrapper.findAll('.fileitem')[1].trigger('click', { ctrlKey: true })
    await wrapper.find('.pan-file-toolbar').findAll('button').find(button => button.text() === '下载').trigger('click')
    await flushPromises()
    expect(api.download).toHaveBeenCalledTimes(2)
    expect(wrapper.emitted('toast')).toContainEqual(['已加入下载队列 1/2 项；失败：Error: 网络故障', 'error'])
  })

  it('彻底删除确认保留原选择，切换账号时关闭待确认操作', async () => {
    const other = { user_id: 'webdav:other-selection', drive_id: 'other-selection' }
    api.capsOf.mockReturnValue({ trashView: true, permanentDelete: true })
    api.listDir.mockImplementation(user => Promise.resolve(user === account.user_id ? files : []))
    api.listTrash.mockResolvedValue(files)
    api.remove.mockResolvedValue(undefined)
    const wrapper = mountAttached(PanView, { props: { account } })
    await flushPromises()
    await wrapper.findAll('.tree-node').find(node => node.text().includes('回收站')).trigger('click')
    await flushPromises()
    await wrapper.findAll('.fileitem')[0].trigger('click')
    await wrapper.find('.pan-file-toolbar .danger').trigger('click')
    expect(document.querySelector('.modal')).not.toBeNull()
    await wrapper.findAll('.fileitem')[1].trigger('click')
    expect(document.querySelector('.modal')).not.toBeNull()
    document.querySelector('.modal-actions .btn.primary').click()
    await flushPromises()
    expect(api.remove).toHaveBeenCalledWith(account.user_id, account.drive_id, ['a'])
    await wrapper.findAll('.fileitem')[0].trigger('click')
    await wrapper.find('.pan-file-toolbar .danger').trigger('click')
    await wrapper.setProps({ account: other })
    await flushPromises()
    expect(document.querySelector('.modal')).toBeNull()
  })

  it('旧账号操作完成后不重新加载当前账号目录', async () => {
    const other = { user_id: 'webdav:other-running', drive_id: 'other-running' }
    let finishTrash
    api.capsOf.mockReturnValue({ recycleBin: true })
    api.listDir.mockImplementation(user => Promise.resolve(user === account.user_id ? files : []))
    api.trash.mockImplementation(() => new Promise(resolve => { finishTrash = resolve }))
    const wrapper = mountAttached(PanView, { props: { account } })
    await flushPromises()
    await wrapper.get('.fileitem').trigger('click')
    await wrapper.find('.pan-file-toolbar .danger').trigger('click')
    await wrapper.setProps({ account: other })
    await flushPromises()
    const currentLoads = api.listDir.mock.calls.filter(call => call[0] === other.user_id).length
    finishTrash()
    await flushPromises()
    expect(api.listDir.mock.calls.filter(call => call[0] === other.user_id)).toHaveLength(currentLoads)
  })

  it('目录读取未完成时切到收藏或搜索，不保留旧页的加载遮罩', async () => {
    let finishDirectory
    api.capsOf.mockReturnValue({ search: true })
    api.listDir.mockImplementation(() => new Promise(resolve => { finishDirectory = resolve }))
    const wrapper = mountAttached(PanView, { props: { account } })
    await flushPromises()
    expect(wrapper.find('.skeleton-list').exists()).toBe(true)
    await wrapper.findAll('.tree-node').find(node => node.text().includes('收藏')).trigger('click')
    await flushPromises()
    expect(wrapper.find('.skeleton-list').exists()).toBe(false)
    expect(wrapper.text()).toContain('暂无收藏')
    await wrapper.get('[title="全盘搜索 (Ctrl/⌘+F)"]').trigger('click')
    await nextTick()
    expect(wrapper.find('.skeleton-list').exists()).toBe(false)
    finishDirectory(files)
    await flushPromises()
    expect(wrapper.text()).not.toContain('A.txt')
  })

  it('收藏读取失败时在内容区显示错误和重试，不误报为空', async () => {
    api.capsOf.mockReturnValue({})
    api.listDir.mockResolvedValue([])
    api.ListFavorites.mockRejectedValueOnce(new Error('请求失败')).mockRejectedValueOnce(new Error('请求失败')).mockResolvedValue([])
    const wrapper = mountAttached(PanView, { props: { account } })
    await flushPromises()
    await wrapper.findAll('.tree-node').find(node => node.text().includes('收藏')).trigger('click')
    await flushPromises()
    expect(wrapper.get('.pan-right .empty').text()).toContain('收藏加载失败')
    expect(wrapper.get('.pan-right .empty').text()).not.toContain('暂无收藏')
    await wrapper.get('.pan-right .empty button').trigger('click')
    await flushPromises()
    expect(wrapper.find('.pan-right .empty').exists()).toBe(false)
    expect(wrapper.text()).toContain('暂无收藏')
  })

  it('文件选择器返回前切换账号，不把文件上传到新账号', async () => {
    const other = { user_id: 'webdav:upload-other', drive_id: 'upload-other' }
    let finishPicker
    api.capsOf.mockReturnValue({ upload: true })
    api.listDir.mockResolvedValue([])
    api.PickFiles.mockImplementation(() => new Promise(resolve => { finishPicker = resolve }))
    const wrapper = mountAttached(PanView, { props: { account } })
    await flushPromises()
    await wrapper.get('[title="上传 (Ctrl+U)"]').trigger('click')
    document.querySelector('.uppick-card').click()
    await wrapper.setProps({ account: other })
    await flushPromises()
    finishPicker(['D:/待上传.txt'])
    await flushPromises()
    expect(api.validateUploadFiles).not.toHaveBeenCalled()
    expect(api.uploadFiles).not.toHaveBeenCalled()
    expect(wrapper.emitted('toast')).toContainEqual(['上传目标已切换，请重新选择文件', 'warn'])
  })

  it('上传冲突检查未完成时切换目录，取消旧目录的上传', async () => {
    let finishConflictCheck
    api.capsOf.mockReturnValue({ upload: true })
    api.listDir.mockResolvedValue([])
    api.PickFiles.mockResolvedValue(['D:/待上传.txt'])
    const wrapper = mountAttached(PanView, { props: { account } })
    await flushPromises()
    api.listDir.mockImplementation((_user, _drive, dir) => dir === 'root'
      ? new Promise(resolve => { finishConflictCheck = resolve })
      : Promise.resolve([]))
    await wrapper.get('[title="上传 (Ctrl+U)"]').trigger('click')
    document.querySelector('.uppick-card').click()
    await flushPromises()
    expect(finishConflictCheck).toBeTypeOf('function')
    await wrapper.vm.navigate({ dirId: 'another-directory', name: '其他目录' })
    finishConflictCheck([])
    await flushPromises()
    expect(api.uploadFiles).not.toHaveBeenCalled()
    expect(wrapper.emitted('toast')).toContainEqual(['上传目标已切换，请重新选择文件', 'warn'])
  })

  it('无法读取目标目录时取消上传，不默认覆盖同名文件', async () => {
    api.capsOf.mockReturnValue({ upload: true })
    api.listDir.mockResolvedValue([])
    api.PickFiles.mockResolvedValue(['D:/重要文件.txt'])
    const wrapper = mountAttached(PanView, { props: { account } })
    await flushPromises()
    api.listDir.mockRejectedValue(new Error('目录不可用'))
    await wrapper.get('[title="上传 (Ctrl+U)"]').trigger('click')
    document.querySelector('.uppick-card').click()
    await flushPromises()
    expect(api.uploadFiles).not.toHaveBeenCalled()
    expect(wrapper.emitted('toast')).toContainEqual(['无法检查同名文件，上传已取消：Error: 目录不可用', 'error'])
  })
})

describe('传输页面任务提交', () => {
  it('打开、定位或复制下载结果失败时提供明确反馈', async () => {
    api.capsOf.mockReturnValue({})
    api.OpenFile.mockRejectedValue(new Error('文件已移动'))
    api.RevealInFolder.mockRejectedValue(new Error('目录不存在'))
    api.copyText.mockRejectedValue(new Error('剪贴板不可用'))
    const wrapper = mountAttached(TransferView)
    await flushPromises()
    await wrapper.vm.openFile({ localPath: 'D:/文件.txt' })
    await wrapper.vm.reveal({ localPath: 'D:/文件.txt' })
    await wrapper.vm.copyLink({ id: 'task', url: 'https://example.test/file' })
    expect(wrapper.emitted('toast')).toContainEqual(['打开文件失败：文件已移动', 'error'])
    expect(wrapper.emitted('toast')).toContainEqual(['定位文件失败：目录不存在', 'error'])
    expect(wrapper.emitted('toast')).toContainEqual(['复制失败：剪贴板不可用', 'error'])
  })

  it('批量操作只处理可执行状态，按钮显示实际目标数量', async () => {
    api.capsOf.mockReturnValue({})
    api.ListDownloads.mockImplementation(async () => [
      { id: 'paused', name: '暂停的文件', status: 'paused' },
      { id: 'active', name: '进行中的文件', status: 'downloading' },
      { id: 'canceled', name: '已取消的文件', status: 'canceled' },
    ])
    const wrapper = mountAttached(TransferView)
    await flushPromises()
    await wrapper.get('[title="全选"]').trigger('click')
    const toolbar = wrapper.get('.toppanbtns')
    expect(toolbar.text()).toContain('继续 (1)')
    expect(toolbar.text()).toContain('暂停 (1)')
    expect(toolbar.text()).toContain('取消 (2)')
    await toolbar.findAll('button').find(button => button.text().includes('继续 (1)')).trigger('click')
    await flushPromises()
    expect(api.ResumeDownload).toHaveBeenCalledOnce()
    expect(api.ResumeDownload).toHaveBeenCalledWith('paused')
    await toolbar.findAll('button').find(button => button.text().includes('暂停 (1)')).trigger('click')
    await flushPromises()
    expect(api.PauseDownload).toHaveBeenCalledOnce()
    expect(api.PauseDownload).toHaveBeenCalledWith('active')
    api.ResumeDownload.mockClear()
    await wrapper.findAll('.taskrow')[0].trigger('contextmenu')
    const menuItems = [...document.querySelectorAll('.ctx-menu [role="menuitem"]')]
    expect(menuItems.find(item => item.textContent.includes('继续')).textContent).not.toContain('(3)')
    menuItems.find(item => item.textContent.includes('继续')).click()
    await flushPromises()
    expect(api.ResumeDownload.mock.calls.map(call => call[0])).toEqual(['paused'])
  })

  it('轮询发现任务已完成时清理旧选择，恢复普通工具栏', async () => {
    api.capsOf.mockReturnValue({})
    api.ListDownloads.mockResolvedValue([{ id: 'task', name: '文件', status: 'downloading' }])
    const wrapper = mountAttached(TransferView)
    await flushPromises()
    await wrapper.get('[title="全选"]').trigger('click')
    expect(wrapper.text()).toContain('已选 1 项')
    api.ListDownloads.mockResolvedValue([{ id: 'task', name: '文件', status: 'completed' }])
    await wrapper.vm.refresh()
    await flushPromises()
    expect(wrapper.text()).not.toContain('已选 1 项')
    expect(wrapper.get('.toppanbtns').text()).toContain('开始全部')
  })

  it('下载链接含无效行时阻止提交，部分失败时只保留失败链接并报告真实数量', async () => {
    api.capsOf.mockReturnValue({})
    api.DownloadURL.mockImplementation((_name, url) => url.endsWith('/bad') ? Promise.reject(new Error('服务不可用')) : Promise.resolve())
    const wrapper = mountAttached(TransferView)
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text().includes('新建下载')).trigger('click')
    const input = document.querySelector('.modal textarea')
    input.value = 'https://example.com/good\nhttps://'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await nextTick()
    expect(document.querySelector('.modal [role="alert"]').textContent).toContain('1 行')
    expect(document.querySelector('.modal-actions .btn.primary').disabled).toBe(true)
    input.value = 'https://example.com/good\nhttps://example.com/bad'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await nextTick()
    document.querySelector('.modal-actions .btn.primary').click()
    await flushPromises()
    expect(api.DownloadURL).toHaveBeenCalledTimes(2)
    expect(wrapper.emitted('toast')).toContainEqual(['已添加 1/2 个下载任务；失败：Error: 服务不可用', 'error'])
    expect(document.querySelector('.modal textarea').value).toBe('https://example.com/bad')
  })

  it('离线任务提交期间切换账号，余下链接仍归属原账号', async () => {
    const first = { user_id: 'pikpak:first-offline', drive_id: 'first-drive' }
    const second = { user_id: 'pikpak:second-offline', drive_id: 'second-drive' }
    let finishFirst
    api.capsOf.mockReturnValue({ offlineDownload: true })
    api.OfflineDownload.mockImplementationOnce(() => new Promise(resolve => { finishFirst = resolve }))
    const wrapper = mountAttached(TransferView, { props: { accounts: [first, second] } })
    await flushPromises()
    await wrapper.findAll('[role="tab"]').find(tab => tab.text().includes('云离线')).trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text().includes('新建离线任务')).trigger('click')
    const input = document.querySelector('.modal textarea')
    input.value = 'https://example.com/one\nhttps://example.com/two'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await nextTick()
    document.querySelector('.modal-actions .btn.primary').click()
    await flushPromises()
    await wrapper.setProps({ accounts: [second] })
    await flushPromises()
    finishFirst()
    await flushPromises()
    expect(api.OfflineDownload.mock.calls).toHaveLength(2)
    expect(api.OfflineDownload.mock.calls.every(call => call[0] === first.user_id && call[1] === first.drive_id)).toBe(true)
  })

  it('离线任务确认删除前切换账号时不误删另一账号任务', async () => {
    const first = { user_id: 'pikpak:delete-first', drive_id: 'first-drive' }
    const second = { user_id: 'pikpak:delete-second', drive_id: 'second-drive' }
    api.capsOf.mockReturnValue({ offlineDownload: true })
    api.refreshOfflineTasks.mockResolvedValue([{ task_id: 'task-1', file_name: '测试任务', drive_id: first.drive_id }])
    const wrapper = mountAttached(TransferView, { props: { accounts: [first, second] } })
    await flushPromises()
    await wrapper.findAll('[role="tab"]').find(tab => tab.text().includes('云离线')).trigger('click')
    await flushPromises()
    await wrapper.get('[title="删除离线任务"]').trigger('click')
    await wrapper.setProps({ accounts: [second] })
    await flushPromises()
    document.querySelector('.modal-actions .btn.primary').click()
    await flushPromises()
    expect(api.DeleteOfflineTask).not.toHaveBeenCalled()
    expect(wrapper.emitted('toast')).toContainEqual(['账号已切换，请重新选择任务', 'warn'])
  })
})

describe('迅雷登录安全验证', () => {
  const providers = [{ ID: 'thunder', Meta: { label: '迅雷网盘' }, Login: { fields: [
    { key: 'username', type: 'text', label: '手机号', required: true },
    { key: 'password', type: 'password', label: '密码', required: true },
  ] } }]

  beforeEach(() => {
    api.login.mockReset().mockRejectedValueOnce(new Error('thunder_verification_required\nurl=https://captcha.xunlei.com/verify\nattempt=attempt-1\nsession=session-1')).mockResolvedValue(undefined)
    api.ShowThunderVerification.mockReset().mockResolvedValue(true)
    api.CancelThunderLogin.mockReset().mockResolvedValue(undefined)
  })

  async function prepare() {
    const wrapper = mountAttached(LoginModal, { props: { initialProvider: 'thunder', providers }, global: { stubs: { UiIcon: true } } })
    await nextTick()
    const fields = [...document.body.querySelectorAll('.login-field input')]
    await setDomInput(fields[0], '13800138000')
    await setDomInput(fields[1], 'login-password')
    document.body.querySelector('form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await flushPromises()
    return wrapper
  }

  it('打开官方验证窗口，回调后携带内部会话自动续办', async () => {
    const wrapper = await prepare()
    expect(api.ShowThunderVerification).toHaveBeenCalledExactlyOnceWith('session-1', 'https://captcha.xunlei.com/verify')
    expect(document.body.textContent).toContain('请完成迅雷官方安全验证')
    expect([...document.body.querySelectorAll('.login-field label')].map(label => label.textContent)).toEqual(['手机号', '密码'])
    const notify = api.onEvent.mock.calls.find(([name]) => name === 'thunder:verification:completed')[1]
    notify({ session_id: 'stale', captcha_token: 'wrong-token' })
    await flushPromises()
    expect(api.login).toHaveBeenCalledOnce()
    notify({ session_id: 'session-1', captcha_token: 'verified-token' })
    await flushPromises()
    expect(api.login).toHaveBeenLastCalledWith('thunder', expect.objectContaining({ username: '13800138000', password: 'login-password', login_attempt: 'attempt-1', captcha_verified: 'true', captcha_token: 'verified-token' }))
    expect(wrapper.emitted('close')).toHaveLength(1)
    notify({ session_id: 'session-1', captcha_token: 'duplicate-token' })
    await flushPromises()
    expect(api.login).toHaveBeenCalledTimes(2)
  })

  it('不支持原生窗口时显示验证页，并可在完成后继续', async () => {
    api.ShowThunderVerification.mockResolvedValue(false)
    await prepare()
    expect(document.querySelector('iframe[title="迅雷官方安全验证"]').getAttribute('src')).toBe('https://captcha.xunlei.com/verify')
    ;[...document.querySelectorAll('button')].find(button => button.textContent.includes('已完成验证，继续登录')).click()
    await flushPromises()
    expect(api.login).toHaveBeenLastCalledWith('thunder', expect.objectContaining({ login_attempt: 'attempt-1', captcha_verified: 'true' }))
    expect(api.CancelThunderLogin).toHaveBeenCalledWith('', 'session-1')
  })

  it('内嵌验证使用独立二级弹窗，Esc 只关闭验证，不关闭登录页', async () => {
    api.ShowThunderVerification.mockResolvedValue(false)
    const wrapper = await prepare()
    const verification = document.querySelector('.verification-mask .login-verification')
    expect(verification.getAttribute('aria-label')).toBe('迅雷官方安全验证')
    expect(verification.querySelector('.modal-window-close')).not.toBeNull()
    expect(document.querySelector('.login-form iframe')).toBeNull()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(document.querySelector('.login-verification')).toBeNull()
    expect(document.querySelector('.login-modal')).not.toBeNull()
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(api.CancelThunderLogin).toHaveBeenCalledWith('attempt-1', 'session-1')
  })

  it('短信验证回传信任参数，不混入滑块 Token', async () => {
    const wrapper = await prepare()
    const notify = api.onEvent.mock.calls.find(([name]) => name === 'thunder:verification:completed')[1]
    notify({ session_id: 'session-1', captcha_token: '', creditkey: 'verified-credit-key' })
    await flushPromises()
    expect(api.login).toHaveBeenLastCalledWith('thunder', expect.objectContaining({ login_attempt: 'attempt-1', captcha_verified: 'true', creditkey: 'verified-credit-key' }))
    expect(api.login.mock.calls.at(-1)[1].captcha_token).toBeUndefined()
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('取消验证清理临时会话，不再接受旧回调', async () => {
    const wrapper = await prepare()
    const notify = api.onEvent.mock.calls.find(([name]) => name === 'thunder:verification:completed')[1]
    ;[...document.querySelectorAll('button')].find(button => button.textContent.trim() === '取消验证').click()
    await flushPromises()
    expect(api.CancelThunderLogin).toHaveBeenCalledWith('attempt-1', 'session-1')
    expect(wrapper.vm.form.login_attempt).toBeUndefined()
    notify({ session_id: 'session-1', captcha_token: 'late-token' })
    await flushPromises()
    expect(api.login).toHaveBeenCalledOnce()
  })

  it('验证期间修改手机号会丢弃原账号的验证会话', async () => {
    const wrapper = await prepare()
    await setDomInput(document.querySelector('.login-field input'), '13900139000')
    await flushPromises()
    expect(api.CancelThunderLogin).toHaveBeenCalledWith('attempt-1', 'session-1')
    expect(wrapper.vm.form.login_attempt).toBeUndefined()
    expect(document.body.textContent).not.toContain('请完成迅雷官方安全验证')
  })

  it('关闭登录框后到达的验证挑战会被清理，不再打开窗口', async () => {
    let rejectLogin
    api.login.mockReset().mockImplementation(() => new Promise((_resolve, reject) => { rejectLogin = reject }))
    const wrapper = await prepare()
    expect(api.login).toHaveBeenCalledOnce()
    wrapper.unmount()
    rejectLogin(new Error('thunder_verification_required\nurl=https://captcha.xunlei.com/verify\nattempt=late-attempt\nsession=late-session'))
    await flushPromises()
    expect(api.CancelThunderLogin).toHaveBeenCalledWith('late-attempt', 'late-session')
    expect(api.ShowThunderVerification).not.toHaveBeenCalled()
  })
})

describe('密码和短信登录切换', () => {
  const modeField = { key: 'login_mode', type: 'select', label: '登录方式', required: true, options: [{ value: 'password', label: '账号密码' }, { value: 'sms', label: '短信验证码' }] }
  const dualProvider = id => ({ ID: id, Meta: { label: id }, Login: { fields: [
    modeField,
    { key: 'username', type: 'text', label: '手机号', required: true },
    { key: 'password', type: 'password', label: '密码', required: true },
    { key: 'sms_code', type: 'text', label: '短信验证码' },
  ] } })
  const field = label => [...document.querySelectorAll('.login-field')].find(item => item.querySelector('label')?.textContent === label)
  const methods = () => [...document.querySelectorAll('.login-method-switch button')]
  const smsButton = () => field('短信验证码').querySelector('button')

  beforeEach(() => {
    api.login.mockReset().mockResolvedValue(undefined)
    api.SendThunderSMS.mockReset().mockResolvedValue({ login_attempt: 'sms-attempt', retry_after: '60' })
    api.CancelThunderLogin.mockReset().mockResolvedValue(undefined)
    api.ShowThunderVerification.mockReset().mockResolvedValue(true)
  })

  async function prepare(id = 'thunder') {
    const wrapper = mountAttached(LoginModal, { props: { initialProvider: id, providers: [dualProvider(id)] }, global: { stubs: { UiIcon: true } } })
    await nextTick()
    await setDomInput(field('手机号').querySelector('input'), '13800138000')
    return wrapper
  }

  it.each(['thunder', 'pan139', 'pan189'])('%s 使用统一的双选项按钮，切换保留手机号并清理另一方式凭据', async id => {
    const wrapper = await prepare(id)
    expect(methods().map(button => button.textContent)).toEqual(['密码登录', '短信验证码登录'])
    expect(methods()[0].getAttribute('aria-pressed')).toBe('true')
    expect(field('短信验证码').classList.contains('login-sms-field')).toBe(true)
    expect(wrapper.findComponent(UiSelect).exists()).toBe(false)
    expect(field('登录方式')).toBeUndefined()
    await setDomInput(field('密码').querySelector('input'), 'old-password')
    methods()[1].click()
    await nextTick()
    expect(field('密码').style.display).toBe('none')
    expect(field('短信验证码').style.display).not.toBe('none')
    expect(wrapper.vm.form.password).toBeUndefined()
    expect(wrapper.vm.form.username).toBe('13800138000')
    await setDomInput(field('短信验证码').querySelector('input'), '123456')
    methods()[0].click()
    await nextTick()
    expect(wrapper.vm.form.sms_code).toBeUndefined()
    expect(field('密码').style.display).not.toBe('none')
    expect(methods()[0].getAttribute('aria-pressed')).toBe('true')
  })

  it('迅雷可从登录页发送短信并无密码登录', async () => {
    const wrapper = await prepare()
    methods()[1].click()
    await nextTick()
    smsButton().click()
    await flushPromises()
    expect(api.SendThunderSMS).toHaveBeenCalledWith(expect.objectContaining({ login_mode: 'sms', username: '13800138000' }))
    expect(api.SendThunderSMS.mock.calls[0][0].password).toBeUndefined()
    expect(smsButton().disabled).toBe(true)
    await setDomInput(field('短信验证码').querySelector('input'), '123456')
    document.querySelector('form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await flushPromises()
    expect(api.login).toHaveBeenCalledWith('thunder', expect.objectContaining({ login_mode: 'sms', login_attempt: 'sms-attempt', sms_code: '123456' }))
    expect(api.login.mock.calls[0][1].password).toBeUndefined()
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('发送前的安全验证完成后只续办短信发送，不错误提交登录', async () => {
    api.SendThunderSMS.mockRejectedValueOnce(new Error('thunder_verification_required\nurl=https://captcha.xunlei.com/verify\nattempt=challenge-attempt\nsession=challenge-session'))
    await prepare()
    methods()[1].click()
    await nextTick()
    smsButton().click()
    await flushPromises()
    expect(api.ShowThunderVerification).toHaveBeenCalledWith('challenge-session', 'https://captcha.xunlei.com/verify')
    const notify = api.onEvent.mock.calls.find(([name]) => name === 'thunder:verification:completed')[1]
    notify({ session_id: 'challenge-session', captcha_token: 'send-verified-token' })
    await flushPromises()
    expect(api.SendThunderSMS).toHaveBeenLastCalledWith(expect.objectContaining({ login_attempt: 'challenge-attempt', captcha_verified: 'true', captcha_token: 'send-verified-token' }))
    expect(api.SendThunderSMS).toHaveBeenCalledTimes(2)
    expect(api.login).not.toHaveBeenCalled()
    expect(smsButton().disabled).toBe(true)
  })

  it('切回密码登录会取消迅雷短信会话', async () => {
    const wrapper = await prepare()
    methods()[1].click()
    await nextTick()
    smsButton().click()
    await flushPromises()
    methods()[0].click()
    await nextTick()
    expect(api.CancelThunderLogin).toHaveBeenCalledWith('sms-attempt', '')
    expect(wrapper.vm.form.login_attempt).toBeUndefined()
    methods()[1].click()
    await nextTick()
    expect(smsButton().disabled).toBe(true)
    expect(smsButton().textContent).toContain('秒后重发')
  })

  it('短信请求期间禁止重复发送和提交，手机号改变后丢弃迟到结果', async () => {
    let complete
    api.SendThunderSMS.mockImplementation(() => new Promise(resolve => { complete = resolve }))
    const wrapper = await prepare()
    methods()[1].click()
    await nextTick()
    smsButton().click()
    await nextTick()
    expect(methods().every(button => button.disabled)).toBe(true)
    expect(document.querySelector('button[type="submit"]').disabled).toBe(true)
    await setDomInput(field('手机号').querySelector('input'), '13900139000')
    complete({ login_attempt: 'late-attempt' })
    await flushPromises()
    expect(api.CancelThunderLogin).toHaveBeenCalledWith('late-attempt', '')
    expect(wrapper.vm.form.login_attempt).toBeUndefined()
    expect(smsButton().disabled).toBe(false)
    expect(api.login).not.toHaveBeenCalled()
  })

  it.each([
    { ID: 'guangya', Meta: { label: '光鸭' }, Login: { fields: [{ key: 'phone', type: 'text', label: '手机号' }, { key: 'sms_code', type: 'text', label: '短信验证码' }] } },
    { ID: 'pikpak', Meta: { label: 'PikPak' }, Login: { fields: [{ key: 'username', type: 'text', label: '账号' }, { key: 'password', type: 'password', label: '密码' }] } },
    { ID: 'onedrive', Meta: { label: 'OneDrive' }, Login: { fields: [{ key: 'oauth', type: 'oauth', label: '授权' }] } },
  ])('单一登录方式的 $ID 不显示虚假的双选项入口', async provider => {
    mountAttached(LoginModal, { props: { initialProvider: provider.ID, providers: [provider] }, global: { stubs: { UiIcon: true } } })
    await nextTick()
    expect(document.querySelector('.login-method-switch')).toBeNull()
  })
})

describe('主窗口顶栏', () => {
  it('Ctrl+K 打开命令，Ctrl+Shift+F 打开跨盘搜索', async () => {
    vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
    api.listAccounts.mockResolvedValue([])
    api.listProviders.mockResolvedValue([])
    api.GetSettings.mockResolvedValue({ theme: 'light', autoUpdate: false })
    const wrapper = mountAttached(App, { global: { stubs: { PanView: true, AccountAvatar: true, GlobalSearch: { template: '<div data-test="global-search" />' } } } })
    await flushPromises()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', code: 'KeyK', ctrlKey: true, bubbles: true }))
    await nextTick()
    expect(wrapper.findComponent(QuickOpen).props('show')).toBe(true)
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'F', code: 'KeyF', ctrlKey: true, shiftKey: true, bubbles: true }))
    await flushPromises()
    expect(wrapper.find('[data-test="global-search"]').exists()).toBe(true)
    vi.unstubAllGlobals()
  })
  it('移除重复入口但保留搜索、设置与快捷命令', async () => {
    vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
    api.listAccounts.mockResolvedValue([])
    api.listProviders.mockResolvedValue([])
    api.GetSettings.mockResolvedValue({ theme: 'light', autoUpdate: false })
    try {
      const wrapper = mountAttached(App, { global: { stubs: { PanView: true, AccountAvatar: true } } })
      await flushPromises()
      const topbar = wrapper.get('.topbar')
      expect(topbar.find('[title="快捷命令面板 (Ctrl+P)"]').exists()).toBe(false)
      expect(topbar.find('[title="切换到深色"]').exists()).toBe(false)
      expect(topbar.find('[title="全盘搜索"]').exists()).toBe(true)
      expect(topbar.find('[title="设置 (Alt+5)"]').exists()).toBe(true)
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'p', code: 'KeyP', ctrlKey: true, bubbles: true }))
      await nextTick()
      expect(wrapper.findComponent(QuickOpen).props('show')).toBe(true)
    } finally { vi.unstubAllGlobals() }
  })
})

describe('声明式授权来源', () => {
  it('确认第三方接收方后才能提交，切换自有应用清除中转同意和隐藏凭据', async () => {
    const provider = { ID: 'aliopen', Meta: { label: '阿里云盘' }, Login: {
      fields: [{ key: 'refresh_token', type: 'text', label: 'Refresh Token', required: true }, { key: 'client_id', type: 'text', label: 'Client ID' }, { key: 'client_secret', type: 'password', label: 'Client Secret' }],
      methods: [{ id: 'relay', label: '导入已有令牌', fieldKeys: ['refresh_token'], recipient: 'https://api.alistgo.com/alist/ali_open/token', notice: '第三方中转', consent: '同意交换和刷新令牌' }, { id: 'own', label: '自有 OAuth 应用', fieldKeys: ['refresh_token', 'client_id', 'client_secret'], requiredKeys: ['client_id'], recipient: 'https://openapi.alipan.com/oauth/access_token', notice: '自有应用' }],
    } }
    const wrapper = mountAttached(LoginModal, { props: { initialProvider: 'aliopen', providers: [provider] } })
    wrapper.vm.form.refresh_token = 'secret'
    expect(wrapper.vm.validate()).not.toBe('')
    expect(wrapper.find('#login-field-client_id').exists()).toBe(false)
    const consent = document.querySelector('[aria-label="令牌使用说明"] input[type="checkbox"]')
    expect(consent).not.toBeNull()
    consent.checked = true
    consent.dispatchEvent(new Event('change', { bubbles: true }))
    await nextTick()
    expect(wrapper.vm.validate()).toBe('')
    document.querySelectorAll('[aria-label="授权来源"] button')[1].click()
    await nextTick()
    expect(wrapper.vm.form.relay_consent).toBeUndefined()
    expect(wrapper.vm.validate()).toContain('Client ID')
    wrapper.vm.form.client_id = 'own'
    wrapper.vm.form.client_secret = 'private'
    document.querySelectorAll('[aria-label="授权来源"] button')[0].click()
    await nextTick()
    expect(wrapper.vm.form.client_secret).toBeUndefined()
    expect(wrapper.vm.form.refresh_token).toBe('secret')
  })
})

describe('主窗口退出确认', () => {
  beforeEach(() => {
    vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
    vi.stubGlobal('runtime', { Quit: vi.fn() })
    api.listAccounts.mockResolvedValue([])
    api.listProviders.mockResolvedValue([])
    api.GetSettings.mockResolvedValue({ theme: 'light', autoUpdate: false })
    api.RespondQuit.mockReset().mockResolvedValue(undefined)
  })
  afterEach(() => vi.unstubAllGlobals())

  async function prepare() {
    const wrapper = mountAttached(App, { global: { stubs: { WorkspaceView: true, AccountAvatar: true } } })
    await flushPromises()
    const notify = api.onEvent.mock.calls.find(([name]) => name === 'app:quit-confirm')[1]
    notify(1)
    await nextTick()
    return { wrapper, notify }
  }

  it('使用应用内弹窗，确认后调用后端退出而非再次普通关闭', async () => {
    const { wrapper } = await prepare()
    const dialog = document.querySelector('[role="dialog"][aria-label="退出 Mnemo"]')
    expect(dialog.textContent).toContain('还有 1 个下载任务')
    expect(dialog.textContent).toContain('下次启动可继续')
    dialog.querySelector('.modal-actions .btn.primary').click()
    await flushPromises()
    expect(api.RespondQuit).toHaveBeenCalledExactlyOnceWith(true)
    expect(window.runtime.Quit).not.toHaveBeenCalled()
    expect(wrapper.findComponent({ name: 'ConfirmModal' }).exists()).toBe(false)
  })

  it.each(['继续下载', '关闭对话框', 'Escape', '遮罩'])('%s 会取消退出并通知后端', async (action) => {
    await prepare()
    if (action === 'Escape') window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    else if (action === '遮罩') document.querySelector('.modal-mask').click()
    else if (action === '关闭对话框') document.querySelector('[aria-label="关闭对话框"]').click()
    else document.querySelector('.modal-actions .btn:not(.primary)').click()
    await flushPromises()
    expect(api.RespondQuit).toHaveBeenCalledExactlyOnceWith(false)
    expect(document.querySelector('[aria-label="退出 Mnemo"]')).toBeNull()
  })

  it('重复退出请求只保留一个弹窗，提交时禁用按钮', async () => {
    const { notify } = await prepare()
    notify(2)
    await nextTick()
    expect(document.querySelectorAll('[aria-label="退出 Mnemo"]')).toHaveLength(1)
    let finish
    api.RespondQuit.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    document.querySelector('.modal-actions .btn.primary').click()
    await nextTick()
    expect([...document.querySelectorAll('.modal-actions button')].every(button => button.disabled)).toBe(true)
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    document.querySelector('.modal-actions .btn.primary').click()
    expect(api.RespondQuit).toHaveBeenCalledTimes(1)
    finish()
    await flushPromises()
  })

  it('退出请求失败时保留弹窗并允许重试', async () => {
    api.RespondQuit.mockRejectedValueOnce(new Error('连接中断'))
    const { wrapper } = await prepare()
    document.querySelector('.modal-actions .btn.primary').click()
    await flushPromises()
    expect(wrapper.text()).toContain('退出操作失败')
    expect(document.querySelector('.modal-actions .btn.primary').disabled).toBe(false)
    document.querySelector('.modal-actions .btn.primary').click()
    await flushPromises()
    expect(api.RespondQuit).toHaveBeenCalledTimes(2)
    expect(document.querySelector('[aria-label="退出 Mnemo"]')).toBeNull()
  })

  it('设置尚未加载时就开始监听退出确认，卸载时注销', async () => {
    let finishSettings
    const stop = vi.fn()
    api.onEvent.mockImplementationOnce(() => stop)
    api.GetSettings.mockImplementationOnce(() => new Promise(resolve => { finishSettings = resolve }))
    const wrapper = mountAttached(App, { global: { stubs: { WorkspaceView: true, AccountAvatar: true } } })
    expect(api.onEvent).toHaveBeenCalledWith('app:quit-confirm', expect.any(Function))
    api.onEvent.mock.calls.find(([name]) => name === 'app:quit-confirm')[1](1)
    await nextTick()
    expect(document.querySelector('[aria-label="退出 Mnemo"]')).not.toBeNull()
    finishSettings({ theme: 'light', autoUpdate: false })
    await flushPromises()
    wrapper.unmount()
    expect(stop).toHaveBeenCalledOnce()
  })
})

describe('网盘收藏', () => {
  const account = { user_id: 'pikpak_favorites', drive_id: 'drive' }
  const file = { file_id: 'file', name: 'photo.jpg', isDir: false, size: 42, parent_file_id: 'parent', category: 'image' }
  async function prepare() {
    api.capsOf.mockReturnValue({ favorite: true })
    api.listDir.mockResolvedValue([file])
    const wrapper = mountAttached(PanView, { props: { account } })
    await flushPromises()
    await wrapper.get('.fileitem').trigger('contextmenu', { clientX: 10, clientY: 10 })
    await nextTick()
    return wrapper
  }
  it('添加收藏只调用统一入口，并保存完整文件信息', async () => {
    const wrapper = await prepare()
    Array.from(document.querySelectorAll('[role="menuitem"]')).find(item => item.textContent.includes('加入收藏')).click()
    await flushPromises()
    expect(api.AddFavorite).toHaveBeenCalledExactlyOnceWith(account.user_id, account.drive_id, expect.objectContaining({ file_id: file.file_id, file: expect.objectContaining({ size: 42, parent_file_id: 'parent' }) }))
    expect(api.favorite).not.toHaveBeenCalled()
  })
  it('收藏失败显示错误并重新获取已经部分成功的列表', async () => {
    api.AddFavorite.mockRejectedValue(new Error('云端收藏失败'))
    const wrapper = await prepare()
    const before = api.ListFavorites.mock.calls.length
    Array.from(document.querySelectorAll('[role="menuitem"]')).find(item => item.textContent.includes('加入收藏')).click()
    await flushPromises()
    expect(wrapper.emitted('toast')).toContainEqual([expect.stringContaining('云端收藏失败'), 'error'])
    expect(wrapper.emitted('toast').some(event => event[1] === 'success')).toBe(false)
    expect(api.ListFavorites.mock.calls.length).toBeGreaterThan(before)
  })
  it('收藏列表保留大小和预览信息', async () => {
    api.ListFavorites.mockResolvedValue([{ ...file, file, source: 'cloud', added: 10 }])
    const wrapper = await prepare()
    await wrapper.findAll('.tree-node').find(node => node.text().startsWith('收藏')).trigger('click')
    await flushPromises()
    expect(wrapper.get('.fileitem').text()).toContain('42 B')
  })
  it('取消收藏只调用统一入口', async () => {
    api.ListFavorites.mockResolvedValue([{ ...file, file, source: 'cloud' }])
    const wrapper = await prepare()
    Array.from(document.querySelectorAll('[role="menuitem"]')).find(item => item.textContent.includes('移出收藏')).click()
    await flushPromises()
    expect(api.RemoveFavorite).toHaveBeenCalledExactlyOnceWith(account.user_id, account.drive_id, 'file')
    expect(api.favorite).not.toHaveBeenCalled()
    expect(wrapper.emitted('toast')).toContainEqual(['已移出收藏', 'success'])
  })
  it('快速刷新不会让较早返回的旧收藏覆盖新收藏', async () => {
    let first, second
    api.ListFavorites.mockImplementationOnce(() => new Promise(resolve => { first = resolve }))
      .mockImplementationOnce(() => new Promise(resolve => { second = resolve }))
    const wrapper = await prepare()
    await wrapper.findAll('.tree-node').find(node => node.text().startsWith('收藏')).trigger('click')
    second([{ file_id: 'new', name: '新收藏.jpg', isDir: false, source: 'cloud' }])
    await flushPromises()
    first([{ file_id: 'old', name: '旧收藏.jpg', isDir: false, source: 'cloud' }])
    await flushPromises()
    expect(wrapper.text()).toContain('新收藏.jpg')
    expect(wrapper.text()).not.toContain('旧收藏.jpg')
  })
  it('批量收藏期间切换账号仍只操作原账号，部分失败显示进度', async () => {
    let finishFirst
    api.AddFavorite.mockImplementationOnce(() => new Promise(resolve => { finishFirst = resolve }))
      .mockRejectedValueOnce(new Error('第二项失败'))
    api.capsOf.mockReturnValue({})
    api.listDir.mockResolvedValue([file, { ...file, file_id: 'second', name: 'second.jpg' }])
    const wrapper = mountAttached(PanView, { props: { account } })
    await flushPromises()
    const rows = wrapper.findAll('.fileitem')
    await rows[0].trigger('click')
    await rows[1].trigger('click', { ctrlKey: true })
    await rows[0].trigger('contextmenu', { clientX: 10, clientY: 10 })
    Array.from(document.querySelectorAll('[role="menuitem"]')).find(item => item.textContent.includes('加入收藏')).click()
    await flushPromises()
    await wrapper.setProps({ account: { user_id: 'webdav:other', drive_id: 'other' } })
    finishFirst()
    await flushPromises()
    expect(api.AddFavorite).toHaveBeenCalledTimes(2)
    expect(api.AddFavorite.mock.calls.every(([user, drive]) => user === account.user_id && drive === account.drive_id)).toBe(true)
    expect(wrapper.emitted('toast')).toContainEqual([expect.stringContaining('已完成 1/2 项'), 'error'])
  })
})

function mountAttached(component, options = {}) {
  const wrapper = mount(component, { attachTo: document.body, ...options })
  wrappers.push(wrapper)
  return wrapper
}

async function setDomInput(input, value) {
  input.value = value
  input.dispatchEvent(new Event('input', { bubbles: true }))
  await nextTick()
}

afterEach(async () => {
  for (const wrapper of wrappers.splice(0)) wrapper.unmount()
  await nextTick()
  document.body.replaceChildren()
  localStorage.clear()
  vi.useRealTimers()
  vi.clearAllMocks()
  vi.restoreAllMocks()
})

describe('关键交互组件', () => {
  it('无标题的通用二级弹窗也提供右上角关闭按钮', async () => {
    const wrapper = mountAttached(Modal, { global: { stubs: { UiIcon: true } } })
    await nextTick()
    document.querySelector('.modal-head .modal-window-close').click()
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('快捷命令面板提供独立关闭按钮', async () => {
    const wrapper = mountAttached(QuickOpen, { props: { show: true }, global: { stubs: { UiIcon: true } } })
    await nextTick()
    document.querySelector('.qo-close').click()
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('添加网盘弹窗只保留紧凑标题，关闭键在标题栏末端', async () => {
    localStorage.setItem('login_provider', 'pikpak')
    const wrapper = mountAttached(LoginModal, {
      props: { providers: [{ ID: 'pikpak', Meta: { label: 'PikPak' }, Login: { fields: [] } }] },
      global: { stubs: { UiIcon: true } },
    })
    await nextTick()
    const head = document.querySelector('.login-modal .modal-head')
    expect(head.textContent.trim()).toBe('添加网盘')
    expect(head.querySelector('.modal-window-controls').lastElementChild.classList.contains('modal-window-close')).toBe(true)
    expect(head.querySelector('.modal-window-close').getAttribute('aria-label')).toBe('关闭添加网盘')
    expect([...document.querySelectorAll('.login-actions .btn')]).toHaveLength(2)
    head.querySelector('.modal-window-close').click()
    expect(wrapper.emitted('close')).toHaveLength(1)
  })
  it('切换网盘时选中高亮沿侧栏移动', async () => {
    localStorage.setItem('login_provider', 'pikpak')
    mountAttached(LoginModal, {
      props: { providers: [
        { ID: 'pikpak', Meta: { label: 'PikPak' }, Login: { fields: [] } },
        { ID: 'dropbox', Meta: { label: 'Dropbox' }, Login: { fields: [] } },
      ] },
      global: { stubs: { UiIcon: true } },
    })
    const list = document.querySelector('.login-provider-list')
    const buttons = [...list.querySelectorAll('.lp-item')]
    list.getBoundingClientRect = () => ({ left: 10, top: 20 })
    Object.defineProperty(list, 'clientHeight', { configurable: true, value: 39 })
    list.scrollTo = vi.fn()
    buttons[0].getBoundingClientRect = () => ({ left: 10, top: 20, width: 152, height: 39 })
    buttons[1].getBoundingClientRect = () => ({ left: 10, top: 61, width: 152, height: 39 })
    await nextTick()
    buttons[1].click()
    await flushPromises()
    const selection = list.querySelector('.lp-selection')
    expect(selection).not.toBeNull()
    expect(selection.style.transform).toBe('translate3d(0px, 41px, 0)')
    expect(selection.style.height).toBe('39px')
    expect(buttons[1].getAttribute('aria-selected')).toBe('true')
    expect(list.scrollTo).toHaveBeenCalledWith(expect.objectContaining({ top: 41, behavior: 'smooth' }))
  })
  it('不限量账号在侧栏显示容量说明而不是零容量进度条', () => {
    const wrapper = mountAttached(AccountRail, { props: { accounts: [{ user_id: 'lanzou:quota', usage: { type: 'unlimited', size: 0, status: 'available' } }] } })
    expect(wrapper.text()).toContain('总空间不限量')
    expect(wrapper.find('.rail-quota').exists()).toBe(false)
  })
  it('账号栏收起时仍为当前账号保留明确选中标记', () => {
    const current = { user_id: 'pikpak:current' }
    const wrapper = mountAttached(AccountRail, {
      props: { accounts: [current, { user_id: 'pikpak:other' }], current },
    })
    const items = wrapper.findAll('.rail-item')
    expect(wrapper.get('.account-rail').classes()).not.toContain('expanded')
    expect(items[0].classes()).toContain('active')
    expect(items[0].attributes('data-selected')).toBe('true')
    expect(items[1].attributes('data-selected')).toBe('false')
  })
  it('账号上下切换时仅为新选中账号标记进入方向', async () => {
    const accounts = [{ user_id: 'pikpak:top' }, { user_id: 'pikpak:bottom' }]
    const wrapper = mountAttached(AccountRail, { props: { accounts, current: accounts[0] } })
    await wrapper.setProps({ current: accounts[1] })
    const items = wrapper.findAll('.rail-item')
    expect(items[1].classes()).toContain('active-enter-down')
    expect(items[1].attributes('data-switch-direction')).toBe('down')
    await wrapper.setProps({ current: accounts[0] })
    expect(items[0].classes()).toContain('active-enter-up')
    expect(items[0].attributes('data-switch-direction')).toBe('up')
  })
  it('展开账号栏后仍可正常切换账号', async () => {
    vi.useFakeTimers()
    const accounts = [{ user_id: 'pikpak:one' }, { user_id: 'pikpak:two' }]
    const wrapper = mountAttached(AccountRail, { props: { accounts, current: accounts[0] } })
    await wrapper.get('.account-rail').trigger('mouseenter')
    await vi.advanceTimersByTimeAsync(250)
    await wrapper.findAll('.rail-item')[1].trigger('click')
    expect(wrapper.emitted('select')?.[0]).toEqual([accounts[1]])
  })
  it('系统默认下载目录显示实际路径，可直接打开并用于文件夹选择器', async () => {
    api.GetSettings.mockResolvedValue({ downloadDir: '' })
    api.GetLogPath.mockResolvedValue('')
    api.GetDownloadDirectory.mockResolvedValue('D:/系统下载')
    api.PickDirectory.mockResolvedValue('')
    const wrapper = mountAttached(SettingsView)
    await flushPromises()
    expect(wrapper.get('#sg-transfer input.input').attributes('placeholder')).toBe('D:/系统下载')
    const open = wrapper.get('#sg-transfer').findAll('button').find(button => button.text() === '打开')
    expect(open).toBeTruthy()
    await open.trigger('click')
    await flushPromises()
    expect(api.OpenDownloadDirectory).toHaveBeenCalledOnce()
    await wrapper.get('#sg-transfer').findAll('button').find(button => button.text() === '选择').trigger('click')
    expect(api.PickDirectory).toHaveBeenCalledWith('选择下载文件夹', 'D:/系统下载')
  })
  it('设置页目录选择器的旧结果不覆盖手动修改', async () => {
    api.GetSettings.mockResolvedValue({ downloadDir: 'D:/原目录' })
    api.GetLogPath.mockResolvedValue('')
    api.GetDownloadDirectory.mockResolvedValue('D:/原目录')
    let finishPick
    api.PickDirectory.mockImplementation(() => new Promise(resolve => { finishPick = resolve }))
    const wrapper = mountAttached(SettingsView)
    await flushPromises()
    const picking = wrapper.vm.pickDownloadDir()
    wrapper.vm.settings.downloadDir = 'D:/新目录'
    finishPick('D:/过期目录')
    await picking
    expect(wrapper.vm.settings.downloadDir).toBe('D:/新目录')
    expect(api.SaveSettings).not.toHaveBeenCalled()
  })
  it('恢复默认目录失败不撤销等待期间的手动修改', async () => {
    api.GetSettings.mockResolvedValue({ downloadDir: 'D:/原目录' })
    api.GetLogPath.mockResolvedValue('')
    api.GetDownloadDirectory.mockResolvedValue('D:/原目录')
    let failSave
    api.SaveSettings.mockImplementation(() => new Promise((_resolve, reject) => { failSave = reject }))
    const wrapper = mountAttached(SettingsView)
    await flushPromises()
    const resetting = wrapper.vm.resetDownloadDir()
    wrapper.vm.settings.downloadDir = 'D:/新目录'
    failSave(new Error('保存失败'))
    await resetting
    expect(wrapper.vm.settings.downloadDir).toBe('D:/新目录')
  })

  it('设置页合并相关分类，并使用可访问的开关按钮', async () => {
    api.GetSettings.mockResolvedValue({})
    api.GetLogPath.mockResolvedValue('')
    api.GetDownloadDirectory.mockResolvedValue('D:/系统下载')
    const wrapper = mountAttached(SettingsView)
    await flushPromises()
    expect(wrapper.findAll('.settings-nav .sn-item')).toHaveLength(6)
    expect(wrapper.get('#sg-maintenance').text()).toContain('日志等级')
    expect(wrapper.find('#sg-network').exists()).toBe(false)
    const switches = wrapper.findAll('button.switch')
    expect(switches.length).toBeGreaterThan(0)
    expect(switches.every(item => item.attributes('role') === 'switch')).toBe(true)
  })

  it('恢复默认下载位置保存空配置，并更新当前生效路径', async () => {
    api.GetSettings.mockResolvedValue({ downloadDir: 'E:/自定义下载' })
    api.GetLogPath.mockResolvedValue('')
    api.GetDownloadDirectory.mockResolvedValueOnce('E:/自定义下载').mockResolvedValue('D:/系统下载')
    api.SaveSettings.mockResolvedValue(undefined)
    const wrapper = mountAttached(SettingsView)
    await flushPromises()
    const reset = wrapper.get('#sg-transfer').findAll('button').find(button => button.text() === '恢复默认')
    expect(reset).toBeTruthy()
    await reset.trigger('click')
    await flushPromises()
    expect(api.SaveSettings).toHaveBeenLastCalledWith(expect.objectContaining({ downloadDir: '' }))
    expect(wrapper.get('#sg-transfer input.input').element.value).toBe('')
    expect(wrapper.get('#sg-transfer input.input').attributes('placeholder')).toBe('D:/系统下载')
  })
  it('下一级目录预缓存限制数量和并发，展开树使用缓存且不递归扫描', async () => {
    vi.useFakeTimers()
    const account = { user_id: 'pan189:prefetch', drive_id: 'prefetch' }
    const folders = Array.from({ length: 10 }, (_, i) => ({ file_id: `folder-${i}`, name: `Folder ${i}`, isDir: true }))
    const pending = []
    api.listDir.mockImplementation(async (_user, _drive, dir) => {
      if (dir === 'root') return folders
      return new Promise(resolve => pending.push({ dir, resolve }))
    })
    api.capsOf.mockReturnValue({})
    const wrapper = mountAttached(PanView, { props: { account, accounts: [account], providers: [] } })
    await flushPromises()
    expect(api.listDir).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(181)
    expect(pending).toHaveLength(2)
    for (let batch = 0; batch < 3; batch++) {
      for (const request of pending.splice(0)) request.resolve([{ file_id: `${request.dir}-child`, name: 'Child', isDir: true }])
      await flushPromises()
      expect(pending.length).toBeLessThanOrEqual(2)
    }
    expect(api.listDir).toHaveBeenCalledTimes(7)
    await wrapper.findAll('.tree-node').find(node => node.text() === 'Folder 0').get('.tn-arrow').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('.tree-node').map(node => node.text())).toContain('Child')
    await vi.advanceTimersByTimeAsync(1000)
    expect(api.listDir).toHaveBeenCalledTimes(7)
  })

  it('快速切换账号丢弃旧响应，列表网格切换和空目录更新不会破坏 DOM', async () => {
    const accounts = [{ user_id: 'pan189:slow', drive_id: 'slow' }, { user_id: 'webdav:fast', drive_id: 'fast' }]
    let slow
    api.listDir.mockImplementation(user => user === accounts[0].user_id ? new Promise(resolve => { slow = resolve }) : Promise.resolve([{ file_id: 'fast', name: '新账号.txt', isDir: false }]))
    api.capsOf.mockReturnValue({})
    const wrapper = mountAttached(WorkspaceView, { props: { account: accounts[0], accounts, providers: [], dual: false } })
    await flushPromises()
    await wrapper.setProps({ account: accounts[1] })
    await flushPromises()
    slow([{ file_id: 'slow', name: '旧账号.txt', isDir: false }])
    await flushPromises()
    expect(wrapper.text()).toContain('新账号.txt')
    expect(wrapper.text()).not.toContain('旧账号.txt')
    for (let i = 0; i < 3; i++) {
      await wrapper.get('[title="网格视图"]').trigger('click')
      expect(wrapper.findAll('.griditem')).toHaveLength(1)
      await wrapper.get('[title="列表视图"]').trigger('click')
      expect(wrapper.findAll('.fileitem')).toHaveLength(1)
    }
    api.listDir.mockResolvedValue([])
    await wrapper.vm.refresh()
    await flushPromises()
    expect(wrapper.findAll('.fileitem')).toHaveLength(0)
    expect(wrapper.text()).toContain('空目录')
  })

  it('大目录只格式化当前视口的文件行', async () => {
    const account = { user_id: 'webdav:large', drive_id: 'large' }
    const files = Array.from({ length: 1000 }, (_, index) => ({ file_id: `file-${index}`, name: `文件 ${index}`, isDir: false, size: index }))
    api.capsOf.mockReturnValue({})
    api.listDir.mockResolvedValue(files)
    api.formatTimeParts.mockClear()
    const wrapper = mountAttached(PanView, { props: { account } })
    await flushPromises()
    expect(wrapper.findAll('.fileitem').length).toBeGreaterThan(0)
    expect(wrapper.findAll('.fileitem').length).toBeLessThan(100)
    expect(api.formatTimeParts.mock.calls.length).toBeLessThan(100)
    await wrapper.get('[title="网格视图"]').trigger('click')
    expect(wrapper.findAll('.griditem').length).toBeLessThan(100)
    expect(api.formatTimeParts.mock.calls.length).toBeLessThan(200)
  })

  it('目录缓存先展示再后台更新，写缓存失败不影响目录显示', async () => {
    const account = { user_id: 'webdav:cache', drive_id: 'cache' }
    let finish
    api.GetDirectoryCache.mockResolvedValue([{ file_id: 'cached', name: '缓存.txt', isDir: false }])
    api.SaveDirectoryCache.mockRejectedValue(new Error('cache unavailable'))
    api.listDir.mockImplementation(() => new Promise(resolve => { finish = resolve }))
    api.capsOf.mockReturnValue({})
    try {
      const wrapper = mountAttached(PanView, { props: { account } })
      await flushPromises()
      expect(wrapper.text()).toContain('缓存.txt')
      finish([{ file_id: 'fresh', name: '最新.txt', isDir: false }])
      await flushPromises()
      expect(wrapper.text()).toContain('最新.txt')
      expect(wrapper.text()).not.toContain('缓存.txt')
    } finally {
      api.GetDirectoryCache.mockResolvedValue(null)
      api.SaveDirectoryCache.mockResolvedValue(undefined)
    }
  })

  it('账号切走再切回后，旧目录树响应不能覆盖新一轮展开结果', async () => {
    vi.useFakeTimers()
    const accounts = [{ user_id: 'pan189:tree-race', drive_id: 'tree-race' }, { user_id: 'webdav:tree-other', drive_id: 'other' }]
    let finishOld
    let childCalls = 0
    api.capsOf.mockReturnValue({})
    api.listDir.mockImplementation(async (user, _drive, dir) => {
      if (user !== accounts[0].user_id) return []
      if (dir === 'root') return [{ file_id: 'folder', name: 'Folder', isDir: true }]
      if (++childCalls === 1) return new Promise(resolve => { finishOld = resolve })
      return [{ file_id: 'fresh-child', name: 'Fresh child', isDir: true }]
    })
    const wrapper = mountAttached(PanView, { props: { account: accounts[0] } })
    await flushPromises()
    const expandFolder = () => wrapper.findAll('.tree-node').find(node => node.text() === 'Folder').get('.tn-arrow').trigger('click')
    await expandFolder()
    await flushPromises()
    await wrapper.setProps({ account: accounts[1] })
    await flushPromises()
    await wrapper.setProps({ account: accounts[0] })
    await flushPromises()
    await expandFolder()
    await flushPromises()
    expect(wrapper.text()).toContain('Fresh child')
    finishOld([{ file_id: 'old-child', name: 'Old child', isDir: true }])
    await flushPromises()
    expect(wrapper.text()).toContain('Fresh child')
    expect(wrapper.text()).not.toContain('Old child')
  })

  it.each(['webdav:drop', 'pan189:drop'])('双栏拖动选中的文件和文件夹到 %s 账号目录会提交移动迁移', async targetUser => {
    const accounts = [{ user_id: 'pan189:drag', drive_id: 'source' }, { user_id: targetUser, drive_id: 'target' }]
    api.listDir.mockImplementation(async user => user === accounts[0].user_id
      ? [{ file_id: 'folder', name: '文件夹', isDir: true }, { file_id: 'file', name: '文档.txt', isDir: false }]
      : [{ file_id: 'destination', name: '目标文件夹', isDir: true }])
    api.capsOf.mockReturnValue({ download: true, upload: true, move: true, copy: true, recycleBin: true })
    const wrapper = mountAttached(WorkspaceView, { props: { account: accounts[0], accounts, providers: [], dual: true } })
    await flushPromises()
    const panes = wrapper.findAll('.workspace-pane')
    await panes[0].findAll('.fileitem')[0].trigger('click')
    await panes[0].findAll('.fileitem')[1].trigger('click', { ctrlKey: true })
    const dataTransfer = { setData: vi.fn(), types: [], effectAllowed: '', dropEffect: '', files: [] }
    await panes[0].findAll('.fileitem')[0].trigger('dragstart', { dataTransfer })
    await panes[1].get('.fileitem').trigger('dragover', { dataTransfer })
    expect(dataTransfer.dropEffect).toBe('move')
    await panes[1].get('.fileitem').trigger('drop', { dataTransfer })
    await flushPromises()
    expect(api.migrateFiles).not.toHaveBeenCalled()
    await wrapper.vm.confirmTransfer()
    await flushPromises()
    expect(api.migrateFiles).toHaveBeenCalledWith('pan189:drag', 'source', targetUser, 'target', 'destination', ['folder', 'file'], true)
    expect(api.move).not.toHaveBeenCalled()
    expect(wrapper.text()).not.toContain('松开鼠标移动')
  })

  it('同账号跨栏拖动走盘内移动，目标是自身目录时阻止提交', async () => {
    const account = { user_id: 'pan189:local-drag', drive_id: 'local-drag' }
    api.listDir.mockImplementation(async (_user, _drive, dir) => dir === 'root' ? [{ file_id: 'folder', name: '来源目录', isDir: true }] : [])
    api.capsOf.mockReturnValue({ move: true, copy: true })
    api.move.mockResolvedValueOnce(['folder'])
    const wrapper = mountAttached(WorkspaceView, { props: { account, accounts: [account], providers: [], dual: true } })
    await flushPromises()
    const views = wrapper.findAllComponents(PanView)
    const dataTransfer = { setData: vi.fn(), types: [], files: [] }
    await views[1].vm.navigate({ dirId: 'folder', name: '来源目录' })
    await flushPromises()
    await views[0].get('.fileitem').trigger('dragstart', { dataTransfer })
    await views[1].get('.pan-right').trigger('drop', { dataTransfer })
    await flushPromises()
    expect(api.move).not.toHaveBeenCalled()
    expect(wrapper.emitted('toast').at(-1)[0]).toContain('自身')
    await views[1].vm.navigate({ dirId: 'destination', name: '目标目录' })
    await flushPromises()
    await views[0].get('.fileitem').trigger('dragstart', { dataTransfer })
    await views[1].get('.pan-right').trigger('drop', { dataTransfer })
    await flushPromises()
    await wrapper.vm.confirmTransfer()
    await flushPromises()
    expect(api.move).toHaveBeenCalledWith(account.user_id, account.drive_id, ['folder'], 'destination')
    expect(api.migrateFiles).not.toHaveBeenCalled()
    expect(api.DeleteDirectoryCache.mock.calls.some(([key]) => key.includes('destination'))).toBe(true)
  })

  it('迁移结束会刷新两栏，拖动期间切换账号会取消原拖动', async () => {
    const handlers = new Map()
    api.onEvent.mockImplementation((name, handler) => { handlers.set(name, handler); return () => handlers.delete(name) })
    const accounts = [{ user_id: 'pan189:events', drive_id: 'source' }, { user_id: 'webdav:events', drive_id: 'target' }]
    api.listDir.mockResolvedValue([{ file_id: 'file', name: '原文件.txt', isDir: false }])
    api.capsOf.mockReturnValue({ download: true, upload: true })
    try {
      const wrapper = mountAttached(WorkspaceView, { props: { account: accounts[0], accounts, providers: [], dual: true } })
      await flushPromises()
      const dataTransfer = { setData: vi.fn(), types: [], files: [] }
      await wrapper.findAll('.workspace-pane')[0].get('.fileitem').trigger('dragstart', { dataTransfer })
      await wrapper.setProps({ account: accounts[1] })
      await flushPromises()
      await wrapper.findAll('.workspace-pane')[1].get('.pan-right').trigger('drop', { dataTransfer })
      await flushPromises()
      expect(api.migrateFiles).not.toHaveBeenCalled()
      await wrapper.setProps({ account: accounts[0] })
      await flushPromises()
      api.listDir.mockResolvedValue([{ file_id: 'updated', name: '迁移后.txt', isDir: false }])
      handlers.get('migrate:progress')({ id: 'done', srcUser: accounts[0].user_id, srcDrive: 'source', dstUser: accounts[1].user_id, dstDrive: 'target', dstParent: 'root', status: 'completed' })
      await flushPromises()
      expect(wrapper.findAll('.fileitem .filename').map(row => row.text())).toEqual(['迁移后.txt', '迁移后.txt'])
    } finally { api.onEvent.mockImplementation(() => () => {}) }
  })

  it('忽略空白或重复 ID 的旧目录缓存，数字 ID 字符串的文件夹独立选中', async () => {
    const account = { user_id: 'pan189:ids', drive_id: 'ids' }
    let finish
    api.listDir.mockImplementation(() => new Promise(resolve => { finish = resolve }))
    api.GetDirectoryCache.mockResolvedValue([{ file_id: '', name: '旧目录A', isDir: true }, { file_id: '', name: '旧目录B', isDir: true }])
    api.capsOf.mockReturnValue({})
    try {
      const wrapper = mountAttached(WorkspaceView, { props: { account, accounts: [account], providers: [], dual: false } })
      await flushPromises()
      expect(wrapper.findAll('.fileitem')).toHaveLength(0)
      finish([{ file_id: '9007199254740992', name: 'A', isDir: true }, { file_id: '9007199254740993', name: 'B', isDir: true }])
      await flushPromises()
      await wrapper.findAll('.fileitem')[0].trigger('click')
      expect(wrapper.findAll('.fileitem.selected')).toHaveLength(1)
      await wrapper.findAll('.fileitem')[1].trigger('click', { ctrlKey: true })
      expect(wrapper.findAll('.fileitem.selected')).toHaveLength(2)
      await wrapper.findAll('.fileitem')[0].trigger('click', { ctrlKey: true })
      expect(wrapper.findAll('.fileitem.selected')).toHaveLength(1)
      expect(wrapper.get('.fileitem.selected').text()).toContain('B')
    } finally { api.GetDirectoryCache.mockResolvedValue(null) }
  })

  it('工作区真实列表在单栏、双栏和账号切换后显示返回文件', async () => {
    const accounts = [{ user_id: 'webdav:one', drive_id: 'one' }, { user_id: 'webdav:two', drive_id: 'two' }]
    api.listDir.mockImplementation(async user => [{ file_id: user + '/file', name: user + '.txt', size: 1, isDir: false }])
    api.capsOf.mockReturnValue({})
    const wrapper = mountAttached(WorkspaceView, { props: { account: accounts[0], accounts, providers: [], dual: false } })
    await flushPromises()
    expect(wrapper.findAll('.fileitem')).toHaveLength(1)
    expect(wrapper.text()).toContain('webdav:one.txt')
    await wrapper.get('.fileitem').trigger('click')
    expect(wrapper.get('.pan-file-toolbar').text()).toContain('已选 1 项')
    await wrapper.setProps({ dual: true })
    await flushPromises()
    expect(wrapper.findAll('.fileitem')).toHaveLength(2)
    expect(wrapper.findAll('.pan-file-toolbar')[1].text()).not.toContain('已选 1 项')
    await wrapper.setProps({ dual: false, account: accounts[1] })
    await flushPromises()
    expect(wrapper.findAll('.fileitem')).toHaveLength(1)
    expect(wrapper.text()).toContain('webdav:two.txt')
    expect(wrapper.get('.pan-file-toolbar').text()).not.toContain('已选 1 项')
  })

  it('重新点击当前账号会主动刷新左侧真实文件列表', async () => {
    vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
    const account = { user_id: 'webdav:reload', drive_id: 'drive' }
    const other = { user_id: 'webdav:other', drive_id: 'other' }
    api.listAccounts.mockResolvedValue([account, other])
    api.listProviders.mockResolvedValue([])
    api.GetSettings.mockResolvedValue({ theme: 'light', autoUpdate: false })
    api.capsOf.mockReturnValue({})
    api.listDir.mockResolvedValue([])
    const wrapper = mountAttached(App)
    await flushPromises()
    api.listDir.mockResolvedValue([{ file_id: 'new', name: '重新加载的文件.txt', size: 1, isDir: false }])
    const calls = api.listDir.mock.calls.length
    await wrapper.get('.rail-item').trigger('click')
    await flushPromises()
    expect(api.listDir.mock.calls.length).toBeGreaterThan(calls)
    expect(wrapper.text()).toContain('重新加载的文件.txt')
    api.listDir.mockResolvedValue([{ file_id: 'other-file', name: '另一个账号的文件.txt', size: 1, isDir: false }])
    await wrapper.findAll('.rail-item')[1].trigger('click')
    await flushPromises()
    expect(api.listDir).toHaveBeenLastCalledWith(other.user_id, other.drive_id, 'root')
    expect(wrapper.text()).toContain('另一个账号的文件.txt')
    expect(wrapper.text()).not.toContain('重新加载的文件.txt')
    vi.unstubAllGlobals()
  })
  it('PR1复核：重新授权后清除登录失效标记', async () => {
    vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
    const { accountHealth } = await import('../workspace')
    const handlers = new Map()
    const account = { user_id: 'dropbox_review_reauth', drive_id: 'dropbox:review', auth_state: 'reauth_required' }
    api.onEvent.mockImplementation((name, fn) => { handlers.set(name, fn); return () => handlers.delete(name) })
    api.listAccounts.mockResolvedValue([account])
    api.listProviders.mockResolvedValue([])
    api.GetSettings.mockResolvedValue({ theme: 'light', autoUpdate: false })
    api.listDir.mockResolvedValue([])
    mountAttached(App, { global: { stubs: { PanView: true, AccountAvatar: true } } })
    await flushPromises()
    handlers.get('account:expired')({ userId: account.user_id, provider: 'dropbox' })
    await flushPromises()
    expect(accountHealth[account.user_id]?.status).toBe('auth')
    api.listAccounts.mockResolvedValue([{ ...account, auth_state: '' }])
    handlers.get('account:changed')({ ...account, auth_state: '' })
    await flushPromises()
    expect(accountHealth[account.user_id]?.status).not.toBe('auth')
  })
  it('PR1复核：重启后显示持久化的重新授权状态', async () => {
    vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
    const { accountHealth } = await import('../workspace')
    const account = { user_id: 'dropbox_review_restart', drive_id: 'dropbox:review', auth_state: 'reauth_required' }
    api.listAccounts.mockResolvedValue([account])
    api.listProviders.mockResolvedValue([])
    api.GetSettings.mockResolvedValue({ theme: 'light', autoUpdate: false })
    api.listDir.mockResolvedValue([])
    mountAttached(App, { global: { stubs: { PanView: true, AccountAvatar: true } } })
    await flushPromises()
    expect(accountHealth[account.user_id]?.status).toBe('auth')
  })
  it('账号登录失效后保留账号和任务，不自动打开重新登录窗口', async () => {
    vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
    const handlers = new Map()
    api.onEvent.mockImplementation((name, fn) => { handlers.set(name, fn); return () => handlers.delete(name) })
    api.listAccounts.mockResolvedValue([{ user_id: 'dropbox_expired', drive_id: 'dropbox:expired' }])
    api.listProviders.mockResolvedValue([
      { ID: 'pikpak', Meta: { label: 'PikPak' }, Login: { fields: [] } },
      { ID: 'dropbox', Meta: { label: 'Dropbox' }, Login: { type: 'oauth', fields: [] } },
    ])
    api.GetSettings.mockResolvedValue({ theme: 'light', autoUpdate: false })
    api.listDir.mockResolvedValue([])
    const wrapper = mountAttached(App, { global: { stubs: { PanView: true, AccountAvatar: true } } })
    await flushPromises()

    api.listAccounts.mockResolvedValue([{ user_id: 'dropbox_expired', drive_id: 'dropbox:expired', auth_state: 'reauth_required' }])
    handlers.get('account:expired')({ userId: 'dropbox_expired', provider: 'dropbox', accountName: '工作盘' })
    await flushPromises()

    expect(document.body.textContent).toContain('Dropbox')
    expect(document.body.textContent).toContain('账号及任务已保留')
    expect(document.body.textContent).toContain('请重新登录授权')
    expect(wrapper.findComponent({ name: 'AccountRail' }).props('current')?.user_id).toBe('dropbox_expired')
    expect(wrapper.findComponent(LoginModal).exists()).toBe(false)
    vi.unstubAllGlobals()
  })
  it('手动检查无更新时明确显示当前版本，不自动关闭', async () => {
    api.GetUpdateStatus.mockResolvedValue({ revision: 0, phase: 'idle', info: { currentVersion: '0.3.0' } })
    api.CheckUpdate.mockResolvedValue({ available: false, currentVersion: '0.3.0' })
    api.getSettings.mockResolvedValue({})
    const wrapper = mountAttached(UpdateModal)
    await flushPromises()
    expect(document.body.textContent).toContain('已是最新正式版')
    expect(document.body.textContent).toContain('0.3.0')
    expect(wrapper.emitted('close')).toBeUndefined()
  })

  it('重新打开更新窗口恢复下载状态，忽略过期事件，并能取消下载', async () => {
    const handlers = new Map()
    api.onEvent.mockImplementation((name, fn) => { handlers.set(name, fn); return () => handlers.delete(name) })
    const state = { revision: 4, phase: 'downloading', downloaded: 50, total: 100, info: { available: true, version: 'v0.3.0', canInstall: true } }
    api.GetUpdateStatus.mockResolvedValue(state)
    api.getSettings.mockResolvedValue({})
    api.CancelUpdate.mockResolvedValue(true)
    try {
      mountAttached(UpdateModal)
      await flushPromises()
      expect(document.body.textContent).toContain('50%')
      expect(api.CheckUpdate).not.toHaveBeenCalled()
      handlers.get('update:state')({ ...state, revision: 3, phase: 'done' })
      await nextTick()
      expect(document.body.textContent).toContain('正在下载更新')
      api.GetUpdateStatus.mockResolvedValue({ ...state, revision: 5, phase: 'canceled' })
      ;[...document.querySelectorAll('.modal button')].find(b => b.textContent === '取消下载').click()
      await flushPromises()
      expect(api.CancelUpdate).toHaveBeenCalledOnce()
      expect(document.body.textContent).toContain('下载已取消')
      expect(api.ApplyUpdate).not.toHaveBeenCalled()
    } finally { api.onEvent.mockImplementation(() => () => {}) }
  })

  it('安装启动失败后可重试已验证安装包，无需重新下载', async () => {
    const status = { revision: 2, phase: 'done', path: 'C:/updates/setup.exe', info: { available: true, canInstall: true } }
    api.GetUpdateStatus.mockResolvedValue(status)
    api.getSettings.mockResolvedValue({ confirmUpdate: false })
    api.ApplyUpdate.mockRejectedValueOnce(new Error('用户取消了系统授权'))
    mountAttached(UpdateModal)
    await flushPromises()
    ;[...document.querySelectorAll('.modal button')].find(b => b.textContent === '安装并重启').click()
    await flushPromises()
    const retry = [...document.querySelectorAll('.modal button')].find(b => b.textContent === '重试安装')
    expect(retry).toBeTruthy()
    api.ApplyUpdate.mockResolvedValueOnce(undefined)
    retry.click()
    await flushPromises()
    expect(api.ApplyUpdate).toHaveBeenLastCalledWith(status.path)
    expect(api.DownloadUpdate).not.toHaveBeenCalled()
  })

  it('非 Windows 更新显示手动安装入口，发布说明不执行 HTML', async () => {
    api.GetUpdateStatus.mockResolvedValue({ revision: 2, phase: 'done', path: '/tmp/update.tar.gz', info: { currentVersion: '0.3.0', canInstall: false, notes: '<img src=x onerror=alert(1)>' } })
    api.getSettings.mockResolvedValue({})
    api.RevealInFolder.mockResolvedValue(undefined)
    mountAttached(UpdateModal)
    await flushPromises()
    expect(document.body.textContent).toContain('手动安装')
    expect(document.body.textContent).not.toContain('安装并重启')
    expect(document.querySelector('.upd-notes img')).toBeNull()
    ;[...document.querySelectorAll('.modal button')].find(b => b.textContent === '打开目录').click()
    await flushPromises()
    expect(api.RevealInFolder).toHaveBeenCalledWith('/tmp/update.tar.gz')
  })

  it('Markdown 代码块保留原始代码并转义 HTML', async () => {
    api.openKindOf.mockReturnValue('text')
    api.PinFileSnapshot.mockResolvedValue(undefined)
    api.PreviewURL.mockResolvedValue('http://127.0.0.1/document')
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, arrayBuffer: async () => new TextEncoder().encode('```html\n<img src=x onerror=alert(1)>\n```').buffer }))
    try {
      mountAttached(PreviewModal, { props: { file: { file_id: 'md', name: 'readme.md' }, account: { user_id: 'webdav:one', drive_id: 'drive' } } })
      await flushPromises()
      expect(document.querySelector('.md-code-block pre code')).not.toBeNull()
      expect(document.querySelector('.md-code-block pre code').textContent).toBe('<img src=x onerror=alert(1)>')
      expect(document.querySelector('.pv-markdown-view img')).toBeNull()
    } finally { vi.unstubAllGlobals(); api.openKindOf.mockImplementation(file => file.name.endsWith('.wav') ? 'audio' : 'image') }
  })
  it('文档预览复用独立窗口标题栏的最小化、最大化和关闭三键', async () => {
    api.openKindOf.mockReturnValue('pdf')
    api.PinFileSnapshot.mockResolvedValue(undefined)
    api.PreviewURL.mockResolvedValue('http://127.0.0.1/document.pdf')
    mountAttached(PreviewModal, { props: { file: { file_id: 'pdf', name: 'manual.pdf', size: 1024 }, account: { user_id: 'webdav:one', drive_id: 'drive' } } })
    await nextTick()
    expect([...document.querySelectorAll('.preview-modal .modal-window-controls button')].map(button => button.getAttribute('aria-label'))).toEqual(['最小化窗口', '最大化窗口', '关闭对话框'])
    expect(document.querySelector('.preview-modal .pv-window-controls')).toBeNull()
  })
  it('账号右键可打开图标名称编辑页，保存后更新所有页面使用的账号', async () => {
    vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
    api.listAccounts.mockResolvedValue([{ user_id: 'webdav:one', drive_id: 'drive' }])
    api.listProviders.mockResolvedValue([{ ID: 'webdav', Meta: { label: 'WebDAV' } }])
    api.GetSettings.mockResolvedValue({ theme: 'light', autoUpdate: false })
    api.setAccountCustomMeta.mockResolvedValue(undefined)
    const wrapper = mountAttached(App, { global: { stubs: { PanView: true, AccountAvatar: true } } })
    await flushPromises()
    expect(wrapper.find('.workspace-controls').exists()).toBe(false)
    expect(wrapper.findAll('header button').every(button => button.text().trim() === '')).toBe(true)
    await wrapper.get('button[aria-label="切换双栏"]').trigger('click')
    expect(wrapper.get('button[aria-label="切换单栏"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.findAll('.workspace-pane')).toHaveLength(2)
    await wrapper.get('button[aria-label="切换单栏"]').trigger('click')
    expect(wrapper.findAll('.workspace-pane')).toHaveLength(1)
    expect(wrapper.find('.workspace-controls').exists()).toBe(false)
    await wrapper.get('.rail-item').trigger('contextmenu', { clientX: 50, clientY: 60 })
    await flushPromises()
    const action = [...document.querySelectorAll('.ctx-item')].find(button => button.textContent.includes('自定义'))
    expect(action).toBeTruthy()
    action.click()
    await flushPromises()
    expect(document.body.textContent).toContain('自定义')
    expect(document.querySelector('.preset-icons-grid')).not.toBeNull()
    await setDomInput(document.querySelector('input[placeholder="留空则使用默认账号名称"]'), '我的资料库')
    document.querySelector('.preset-icon-chip[title="坚果云"]').click()
    await nextTick()
    const save = [...document.querySelectorAll('.modal button')].find(button => button.textContent.trim() === '保存')
    save.click()
    await flushPromises()
    expect(api.setAccountCustomMeta).toHaveBeenCalledWith('webdav:one', '我的资料库', 'jianguoyun.svg')
    expect(wrapper.findComponent({ name: 'PanView' }).props('accounts')[0]).toMatchObject({ custom_name: '我的资料库', custom_icon: 'jianguoyun.svg' })
    expect(document.querySelector('.custom-acc-form')).toBeNull()
    vi.unstubAllGlobals()
  })

  it('账号排序偏好改变后侧栏即时更新，新账号保留在末尾', async () => {
    const accounts = [{ user_id: 'webdav:one' }, { user_id: 'webdav:two' }, { user_id: 'webdav:new' }]
    const wrapper = mountAttached(AccountRail, { props: { accounts } })
    appearance.setPref('accountOrder', ['webdav:two', 'deleted', 'webdav:two', 'webdav:one'])
    await nextTick()
    const items = wrapper.findAll('.rail-item')
    expect(items.map(item => item.attributes('aria-label'))).toEqual(['webdav · webdav:two', 'webdav · webdav:one', 'webdav · webdav:new'])
  })

  it('焦点在其他输入框时方向键不会展开下拉框', async () => {
    mountAttached(UiSelect, { props: { options: [{ value: 'a', label: 'A' }] } })
    const input = document.createElement('input')
    document.body.appendChild(input)
    input.focus()
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }))
    await nextTick()
    expect(document.querySelector('.uiselect-drop')).toBeNull()
  })
  it('鼠标切换账号后移出侧栏，即使按钮仍有焦点也能收起', async () => {
    vi.useFakeTimers()
    const wrapper = mountAttached(AccountRail, { props: { accounts: [{ user_id: 'pikpak:one' }] } })
    await wrapper.get('.account-rail').trigger('mouseenter')
    await vi.advanceTimersByTimeAsync(250)
    const item = wrapper.get('.rail-item')
    await item.trigger('pointerdown', { button: 0 })
    item.element.focus()
    window.dispatchEvent(new MouseEvent('pointerup'))
    await item.trigger('click')
    await wrapper.get('.account-rail').trigger('mouseleave')
    await vi.advanceTimersByTimeAsync(250)
    expect(wrapper.get('.account-rail').classes()).not.toContain('expanded')
    expect(wrapper.emitted('select')).toHaveLength(1)
  })

  it('没有兼容账号时分享页仍展示导入入口和说明', async () => {
    api.ListShareHistory.mockResolvedValue([])
    const wrapper = mountAttached(ShareView)
    await flushPromises()
    expect(wrapper.text()).toContain('还没有分享记录')
    const entry = wrapper.findAll('button').find(button => button.text().includes('导入分享'))
    expect(entry).toBeTruthy()
    await entry.trigger('click')
    await flushPromises()
    expect(document.body.textContent).toContain('请先添加支持分享导入的网盘账号')
  })
  it('OLED 背景默认关闭，可即时切换并在重新打开设置后保留', async () => {
    api.GetSettings.mockResolvedValue({ theme: 'dark' })
    api.GetLogPath.mockResolvedValue('')
    appearance.applyAppearance('dark')
    const wrapper = mountAttached(SettingsView)
    await flushPromises()
    const toggle = wrapper.get('[role="switch"][aria-labelledby="oled-background-label"]')
    expect(toggle.attributes('aria-checked')).toBe('false')
    await toggle.trigger('click')
    expect(appearance.getPrefs().oledBackground).toBe(true)
    expect(document.documentElement.matches('html.dark.oled')).toBe(true)
    appearance.applyAppearance('light')
    expect(document.documentElement.matches('html.dark.oled')).toBe(false)
    appearance.applyAppearance('dark')
    expect(document.documentElement.matches('html.dark.oled')).toBe(true)
    const reopened = mountAttached(SettingsView)
    await flushPromises()
    const restored = reopened.get('[role="switch"][aria-labelledby="oled-background-label"]')
    expect(restored.attributes('aria-checked')).toBe('true')
    await restored.trigger('click')
    expect(appearance.getPrefs().oledBackground).toBe(false)
    expect(document.documentElement.classList.contains('oled')).toBe(false)
    document.documentElement.classList.remove('dark')
  })
  it('设置读取失败时禁止保存默认值，重试成功后恢复保存', async () => {
    api.GetSettings.mockRejectedValueOnce(new Error('读取失败')).mockResolvedValue({ proxy: 'http://localhost:7890' })
    api.GetLogPath.mockResolvedValue('')
    api.SaveSettings.mockResolvedValue(undefined)
    const wrapper = mountAttached(SettingsView)
    await flushPromises()
    await wrapper.vm.save(true)
    expect(api.SaveSettings).not.toHaveBeenCalled()
    expect(document.body.textContent).toContain('重试')
    await wrapper.vm.loadSettings()
    await wrapper.vm.save(true)
    expect(api.SaveSettings).toHaveBeenCalledWith(expect.objectContaining({ proxy: 'http://localhost:7890' }))
  })
  it('日志路径读取失败不重置已加载的设置', async () => {
    api.GetSettings.mockResolvedValue({ proxy: 'http://localhost:7890', maxConcurrentDownloads: 7, maxDownloadSpeed: 2048 })
    api.GetLogPath.mockRejectedValue(new Error('日志路径不可用'))
    const wrapper = mountAttached(SettingsView)
    await flushPromises()
    expect(wrapper.vm.settings.proxy).toBe('http://localhost:7890')
    expect(wrapper.vm.settings.maxConcurrentDownloads).toBe(7)
    expect(wrapper.vm.settings.maxDownloadSpeed).toBe(2)
  })
  it('旧同步任务保存完成后不关闭新打开的编辑表单', async () => {
    api.ListSyncConfigs.mockResolvedValue([])
    api.ListRunningSyncIDs.mockResolvedValue([])
    let finishSave
    api.SaveSyncConfig.mockImplementation(() => new Promise(resolve => { finishSave = resolve }))
    const wrapper = mountAttached(SyncView, { props: { accounts: [{ user_id: 'user', drive_id: 'drive' }] } })
    wrapper.vm.openEdit({ id: 'first', name: '旧任务', user_id: 'user', drive_id: 'drive', local_dir: 'D:/first' })
    const saving = wrapper.vm.save()
    wrapper.vm.showEdit = false
    wrapper.vm.openEdit({ id: 'second', name: '新任务', user_id: 'user', drive_id: 'drive', local_dir: 'D:/second' })
    finishSave()
    await saving
    expect(wrapper.vm.showEdit).toBe(true)
    expect(wrapper.vm.form.name).toBe('新任务')
  })
  it('本地目录选择结果不会写入重新打开的同步表单', async () => {
    api.ListSyncConfigs.mockResolvedValue([])
    api.ListRunningSyncIDs.mockResolvedValue([])
    let finishPick
    api.PickDirectory.mockImplementation(() => new Promise(resolve => { finishPick = resolve }))
    const wrapper = mountAttached(SyncView, { props: { accounts: [{ user_id: 'user', drive_id: 'drive' }] } })
    wrapper.vm.openCreate()
    const picking = wrapper.vm.pickLocalDir()
    wrapper.vm.closeEdit()
    wrapper.vm.openCreate()
    finishPick('D:/old-directory')
    await picking
    expect(wrapper.vm.form.local_dir).toBe('')
    expect(wrapper.vm.showEdit).toBe(true)
  })
  it('切换同步账号或编辑任务时关闭旧网盘目录选择器', async () => {
    api.ListSyncConfigs.mockResolvedValue([])
    api.ListRunningSyncIDs.mockResolvedValue([])
    const wrapper = mountAttached(SyncView, { props: { accounts: [{ user_id: 'user', drive_id: 'drive' }] } })
    wrapper.vm.openCreate()
    wrapper.vm.showDirPick = true
    wrapper.vm.onFormAccount()
    expect(wrapper.vm.showDirPick).toBe(false)
    wrapper.vm.showDirPick = true
    wrapper.vm.openEdit({ id: 'another', user_id: 'user', drive_id: 'drive' })
    expect(wrapper.vm.showDirPick).toBe(false)
  })
  it('目录选择器切换目录时隐藏旧文件夹且等待新目录加载', async () => {
    let finishChild
    api.listDir.mockImplementation((_user, _drive, dir) => dir === 'root'
      ? Promise.resolve([{ file_id: 'child', name: '旧目录', isDir: true }])
      : new Promise(resolve => { finishChild = resolve }))
    const wrapper = mountAttached(SelectDirModal, { props: { account: { user_id: 'user', drive_id: 'drive' } } })
    await flushPromises()
    wrapper.vm.enter({ file_id: 'child', name: '子目录' })
    await nextTick()
    expect(wrapper.vm.dirs).toEqual([])
    expect(document.querySelector('.modal-mask .btn.primary')?.disabled).toBe(true)
    finishChild([])
    await flushPromises()
    expect(wrapper.vm.loading).toBe(false)
  })
  it('同步初始化的旧运行列表不能清除刚启动的任务', async () => {
    api.ListSyncConfigs.mockResolvedValue([])
    let resolveInitial, resolveRun
    api.ListRunningSyncIDs.mockImplementation(() => new Promise(resolve => { resolveInitial = resolve }))
    api.RunSync.mockImplementation(() => new Promise(resolve => { resolveRun = resolve }))
    const wrapper = mountAttached(SyncView)
    const task = wrapper.vm.run({ id: 'sync-new', name: '任务' })
    resolveInitial([])
    await flushPromises()
    expect(wrapper.vm.running.has('sync-new')).toBe(true)
    resolveRun()
    await task
    expect(wrapper.vm.running.has('sync-new')).toBe(false)
  })
  it('同步任务刷新乱序时保留最新列表', async () => {
    const pending = []
    api.ListSyncConfigs.mockImplementation(() => new Promise(resolve => pending.push(resolve)))
    api.ListRunningSyncIDs.mockResolvedValue([])
    const wrapper = mountAttached(SyncView)
    wrapper.vm.refresh()
    pending[1]([{ id: 'new', name: '新任务' }])
    await flushPromises()
    pending[0]([{ id: 'old', name: '旧任务' }])
    await flushPromises()
    expect(wrapper.vm.jobs.map(job => job.id)).toEqual(['new'])
  })
  it('分享解析期间目标账号被移除时丢弃旧会话', async () => {
    api.ListShareHistory.mockResolvedValue([])
    api.capsOf.mockReturnValue({ importShare: true })
    let resolveImport
    api.importShare.mockImplementation(() => new Promise(resolve => { resolveImport = resolve }))
    const first = { user_id: 'first', drive_id: 'drive' }
    const second = { user_id: 'second', drive_id: 'drive' }
    const wrapper = mountAttached(ShareView, { props: { accounts: [first, second] } })
    wrapper.vm.openImport()
    wrapper.vm.importForm.url = 'https://example.test/share'
    const parsing = wrapper.vm.parseImport()
    await wrapper.setProps({ accounts: [second] })
    resolveImport({ files: [{ fileId: 'old-file' }] })
    await parsing
    expect(wrapper.vm.importSession).toBeNull()
    expect(wrapper.vm.importStep).toBe('form')
    expect(wrapper.vm.importBusy).toBe(false)
  })
  it('粘贴新的分享链接时清除上一条链接的提取码', async () => {
    api.ListShareHistory.mockResolvedValue([])
    api.capsOf.mockReturnValue({ importShare: true })
    const wrapper = mountAttached(ShareView, { props: { accounts: [{ user_id: 'pikpak:share', drive_id: 'drive' }] } })
    wrapper.vm.importForm.url = 'https://mypikpak.com/s/first 提取码: abcd'
    await nextTick()
    expect(wrapper.vm.importForm.password).toBe('abcd')
    wrapper.vm.importForm.url = 'https://mypikpak.com/s/second'
    await nextTick()
    expect(wrapper.vm.importForm.password).toBe('')
  })
  it('分享链接打开或复制失败时显示错误，不留下未处理异常', async () => {
    api.ListShareHistory.mockResolvedValue([])
    api.OpenBrowser.mockRejectedValue(new Error('浏览器不可用'))
    api.copyText.mockRejectedValue(new Error('剪贴板不可用'))
    const wrapper = mountAttached(ShareView)
    await flushPromises()
    await wrapper.vm.openLink({ share_url: 'https://example.test/share' })
    await wrapper.vm.copy('链接', '已复制', 'link')
    expect(wrapper.emitted('toast')).toContainEqual(['打开分享链接失败：浏览器不可用', 'error'])
    expect(wrapper.emitted('toast')).toContainEqual(['复制失败：剪贴板不可用', 'error'])
  })
  it('偏好恢复中途失败后保留确认数据，允许原样重试', async () => {
    api.GetSettings.mockResolvedValue({ theme: 'light' })
    api.GetLogPath.mockResolvedValue('')
    api.listAccounts.mockResolvedValue([])
    api.SaveSettings.mockRejectedValueOnce(new Error('写入失败')).mockResolvedValue(undefined)
    const wrapper = mountAttached(SettingsView)
    await flushPromises()
    wrapper.vm.pendingBackup = { preferences: { accountAliases: {}, accountIcons: {} }, favorites: [], theme: 'dark' }
    await wrapper.vm.restorePrefs()
    expect(wrapper.vm.pendingBackup).not.toBeNull()
    await wrapper.vm.restorePrefs()
    expect(wrapper.vm.pendingBackup).toBeNull()
    expect(api.SaveSettings).toHaveBeenCalledTimes(2)
  })
  it('音频播放结束时清除旧的续播位置', async () => {
    api.PinFileSnapshot.mockResolvedValue(undefined)
    api.getPlayCursor.mockResolvedValue(45)
    api.savePlayCursor.mockResolvedValue(undefined)
    api.PreviewURL.mockResolvedValue('https://example.test/song.wav')
    const wrapper = mountAttached(PreviewModal, { props: { account: { user_id: 'test', drive_id: 'drive' }, file: { file_id: 'song', name: 'song.wav' } } })
    await flushPromises()
    const audio = document.querySelector('audio')
    audio.currentTime = 120
    wrapper.vm.audioDur = 120
    audio.dispatchEvent(new Event('ended'))
    await flushPromises()
    expect(api.savePlayCursor).toHaveBeenCalledWith('test', 'drive', 'song', 0)
  })
  it('视频切换账号时保存旧账号进度并重新获取播放地址', async () => {
    vi.spyOn(HTMLMediaElement.prototype, 'load').mockImplementation(() => {})
    vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => {})
    api.pinFileSnapshot.mockResolvedValue(undefined)
    api.getSettings.mockResolvedValue({ playbackResume: false })
    api.savePlayCursor.mockResolvedValue(undefined)
    api.playVideo.mockImplementation(async user => ({ url: `https://example.test/${user}.mp4`, stream_type: 'mp4', qualities: [] }))
    const wrapper = mountAttached(PlayerPanel, { props: { account: { user_id: 'first', drive_id: 'drive' }, file: { file_id: 'video', name: 'video.mp4' } } })
    await flushPromises()
    document.querySelector('video').currentTime = 42
    await wrapper.setProps({ account: { user_id: 'second', drive_id: 'drive' } })
    await flushPromises()
    expect(api.savePlayCursor).toHaveBeenCalledWith('first', 'drive', 'video', 42)
    expect(api.playVideo).toHaveBeenLastCalledWith('second', 'drive', 'video')
    expect(wrapper.vm.src).toBe('https://example.test/second.mp4')
  })
  it('连续切换视频清晰度时只采用最后一次选择', async () => {
    vi.spyOn(HTMLMediaElement.prototype, 'load').mockImplementation(() => {})
    vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => {})
    api.pinFileSnapshot.mockResolvedValue(undefined)
    api.getSettings.mockResolvedValue({ playbackResume: false })
    api.playVideo.mockResolvedValue({ url: 'https://example.test/original.mp4', stream_type: 'mp4', qualities: [] })
    const pending = []
    api.playVideoQuality.mockImplementation(() => new Promise(resolve => pending.push(resolve)))
    const wrapper = mountAttached(PlayerPanel, { props: { account: { user_id: 'test', drive_id: 'test' }, file: { file_id: 'video', name: 'video.mp4' } } })
    await flushPromises()
    const first = wrapper.vm.switchQuality('720p')
    const last = wrapper.vm.switchQuality('1080p')
    pending[1]({ url: 'https://example.test/1080.mp4', stream_type: 'mp4', qualities: [] })
    await last
    pending[0]({ url: 'https://example.test/720.mp4', stream_type: 'mp4', qualities: [] })
    await first
    expect(wrapper.vm.src).toBe('https://example.test/1080.mp4')
  })
  it('不同账号使用相同文件 ID 时重新加载预览', async () => {
    api.PinFileSnapshot.mockResolvedValue(undefined)
    api.getPlayCursor.mockResolvedValue(0)
    api.savePlayCursor.mockResolvedValue(undefined)
    api.PreviewURL.mockImplementation(async (user) => `https://example.test/${user}.wav`)
    const wrapper = mountAttached(PreviewModal, { props: { account: { user_id: 'first', drive_id: 'drive' }, file: { file_id: '/song.wav', name: 'song.wav' } } })
    await flushPromises()
    expect(wrapper.vm.url).toBe('https://example.test/first.wav')
    await wrapper.setProps({ account: { user_id: 'second', drive_id: 'drive' } })
    await flushPromises()
    expect(api.PreviewURL).toHaveBeenLastCalledWith('second', 'drive', '/song.wav')
    expect(wrapper.vm.url).toBe('https://example.test/second.wav')
  })
  it('切换文本文件后旧响应不得覆盖新内容，关闭时取消读取', async () => {
    vi.spyOn(api, 'openKindOf').mockReturnValue('text')
    api.PreviewURL.mockImplementation(async (_user, _drive, id) => `https://example.test/${id}`)
    const pending = []
    vi.spyOn(globalThis, 'fetch').mockImplementation((url, options) => Promise.resolve({
      ok: true,
      arrayBuffer: () => new Promise(resolve => pending.push({ url, options, resolve })),
    }))
    const wrapper = mountAttached(PreviewModal, { props: { account: { user_id: 'test', drive_id: 'test' }, file: { file_id: 'old', name: 'old.txt' } } })
    await flushPromises()
    await wrapper.setProps({ file: { file_id: 'new', name: 'new.txt' } })
    await flushPromises()
    pending[1].resolve(new TextEncoder().encode('new content').buffer)
    await flushPromises()
    pending[0].resolve(new TextEncoder().encode('old content').buffer)
    await flushPromises()
    expect(wrapper.vm.text).toBe('new content')
    expect(pending[0].options.signal.aborted).toBe(true)
    wrapper.unmount()
    expect(pending[1].options.signal.aborted).toBe(true)
  })
  it('文本工具栏实际显示，修改后请求关闭先确认', async () => {
    vi.spyOn(api, 'openKindOf').mockReturnValue('text')
    api.PinFileSnapshot.mockResolvedValue(undefined)
    api.PreviewURL.mockResolvedValue('http://127.0.0.1/document')
    vi.spyOn(globalThis, 'fetch').mockResolvedValue({ ok: true, arrayBuffer: async () => new TextEncoder().encode('hello').buffer })
    const wrapper = mountAttached(PreviewModal, { props: { account: { user_id: 'test', drive_id: 'test' }, file: { file_id: 'text', name: 'test.txt' } } })
    await flushPromises()
    const toolbar = document.querySelector('.pv-toolbar')
    expect(toolbar.querySelector('template')).toBeNull()
    expect(toolbar.textContent).toContain('在线编辑')
    ;[...toolbar.querySelectorAll('button')].find(b => b.textContent.includes('在线编辑')).click()
    await nextTick()
    await setDomInput(document.querySelector('textarea'), 'changed')
    wrapper.vm.requestClose()
    await nextTick()
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(document.body.textContent).toContain('未保存')
  })
  it('窗口播放和暂停时闲置都会隐藏上下控制区，移动鼠标恢复', async () => {
    vi.spyOn(HTMLMediaElement.prototype, 'load').mockImplementation(() => {})
    vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => {})
    api.pinFileSnapshot.mockResolvedValue(undefined)
    api.getSettings.mockResolvedValue({ playbackResume: false })
    api.playVideo.mockResolvedValue({ url: 'http://127.0.0.1/video.mp4', stream_type: 'mp4' })
    mountAttached(PlayerPanel, { props: { account: { user_id: 'test', drive_id: 'test' }, file: { file_id: 'video', name: 'video.mp4' } } })
    await flushPromises()
    vi.useFakeTimers()
    try {
      const panel = document.querySelector('.player-panel')
      const video = document.querySelector('video')
      for (const event of ['play', 'pause']) {
        video.dispatchEvent(new Event(event))
        await vi.advanceTimersByTimeAsync(2700)
        expect(document.querySelector('.pp-topbar').classList.contains('hidden')).toBe(true)
        expect(document.querySelector('.pp-bottom').classList.contains('hidden')).toBe(true)
        panel.dispatchEvent(new MouseEvent('mousemove', { bubbles: true }))
        await nextTick()
        expect(document.querySelector('.pp-topbar').classList.contains('hidden')).toBe(false)
        expect(document.querySelector('.pp-bottom').classList.contains('hidden')).toBe(false)
      }
    } finally { vi.useRealTimers() }
  })
  it('TS 转码流跳转会重新定位、取消旧请求并在关闭时释放播放器', async () => {
    vi.spyOn(HTMLMediaElement.prototype, 'load').mockImplementation(() => {})
    vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => {})
    vi.spyOn(HTMLMediaElement.prototype, 'play').mockResolvedValue(undefined)
    api.pinFileSnapshot.mockResolvedValue(undefined)
    api.getSettings.mockResolvedValue({ playbackResume: false })
    api.playVideo.mockResolvedValue({ url: 'http://127.0.0.1/stream/test', stream_type: 'ts', duration: 120, qualities: [] })
    const player = { on: vi.fn(), attachMediaElement: vi.fn(), load: vi.fn(), destroy: vi.fn() }
    tsMock.createPlayer.mockReturnValue(player)
    const pending = []
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockImplementation((url, options) => new Promise(resolve => pending.push({ url, options, resolve })))
    const wrapper = mountAttached(PlayerPanel, { props: { account: { user_id: 'test', drive_id: 'test' }, file: { file_id: 'video', name: 'video.mkv' } } })
    await flushPromises()
    const video = document.querySelector('video')
    expect(video).not.toBeNull()
    expect(player.attachMediaElement).toHaveBeenCalledWith(video)
    expect(player.load).toHaveBeenCalledOnce()
    const progress = document.querySelector('.pp-progress input')
    expect(progress).not.toBeNull()
    await setDomInput(progress, 90)
    expect(video.currentTime).toBe(0)
    expect(fetchSpy).toHaveBeenCalledOnce()
    await setDomInput(progress, 45)
    expect(pending[0].options.signal.aborted).toBe(true)
    pending[0].resolve({ ok: true, json: async () => ({ url: '/stream/test?offset=900', start: 88 }) })
    pending[1].resolve({ ok: true, json: async () => ({ url: '/stream/test?offset=450', start: 43 }) })
    await flushPromises()
    expect(tsMock.createPlayer).toHaveBeenCalledTimes(2)
    expect(tsMock.createPlayer.mock.calls[1][0].url).toContain('offset=450')
    wrapper.unmount()
    expect(player.destroy).toHaveBeenCalledTimes(2)
  })
  it.each(['image', 'audio'])('%s 预览加载中和失败后始终保留窗口关闭按钮', async (kind) => {
    let rejectPreview
    api.PreviewURL.mockImplementation(() => new Promise((resolve, reject) => { rejectPreview = reject }))
    const wrapper = mountAttached(PreviewModal, { props: { account: { user_id: 'test', drive_id: 'test' }, file: { file_id: 'test', name: kind === 'audio' ? 'test.wav' : 'test.png' } } })
    await flushPromises()
    expect(document.querySelectorAll('.pv-window-btn')).toHaveLength(3)
    rejectPreview(new Error('资源加载失败'))
    await flushPromises()
    expect(document.body.textContent).toContain('资源加载失败')
    document.querySelector('.pv-window-close').click()
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('图片来回切换复用预载缓存，初次焦点不落在窗口按钮上', async () => {
    const originalImage = globalThis.Image
    vi.stubGlobal('Image', class {
      naturalWidth = 640
      naturalHeight = 480
      set src(value) { this.url = value; queueMicrotask(() => this.onload?.()) }
    })
    const files = [
      { file_id: 'first', name: 'first.png', size: 100, isDir: false },
      { file_id: 'second', name: 'second.png', size: 100, isDir: false },
    ]
    api.PinFileSnapshot.mockResolvedValue(undefined)
    api.cachedPreviewImageURL.mockImplementation(async (_user, _drive, file) => `http://127.0.0.1/cache/${file.file_id}`)
    try {
      mountAttached(PreviewModal, { props: { account: { user_id: 'test', drive_id: 'test' }, file: files[0], fileList: files } })
      await flushPromises()
      expect(document.activeElement).toBe(document.querySelector('.preview-modal'))
      expect(api.cachedPreviewImageURL).toHaveBeenCalledTimes(2)
      document.querySelector('.pv-edge.right').click()
      await flushPromises()
      document.querySelector('.pv-edge.left').click()
      await flushPromises()
      expect(api.cachedPreviewImageURL).toHaveBeenCalledTimes(2)
      expect(api.PinFileSnapshot).toHaveBeenCalledTimes(2)
      expect(document.querySelector('.pv-topbar-name').textContent).toBe('first.png')
    } finally {
      vi.stubGlobal('Image', originalImage)
    }
  })

  it('图片磁盘缓存损坏时回退到原始预览地址', async () => {
    const originalImage = globalThis.Image
    vi.stubGlobal('Image', class {
      naturalWidth = 800
      naturalHeight = 600
      set src(value) { queueMicrotask(() => value.includes('/local/') ? this.onerror?.() : this.onload?.()) }
    })
    api.PinFileSnapshot.mockResolvedValue(undefined)
    api.cachedPreviewImageURL.mockResolvedValue('http://127.0.0.1/local/broken')
    api.PreviewURL.mockResolvedValue('http://127.0.0.1/stream/original')
    try {
      mountAttached(PreviewModal, { props: { account: { user_id: 'test', drive_id: 'test' }, file: { file_id: 'image', name: 'image.png', size: 100 } } })
      await flushPromises()
      expect(document.querySelector('.pv-img-layer img')?.getAttribute('src')).toBe('http://127.0.0.1/stream/original')
      expect(api.PreviewURL).toHaveBeenCalledOnce()
    } finally {
      vi.stubGlobal('Image', originalImage)
    }
  })

  it('图片请求期间切换账号时，旧图片后续请求仍使用原账号', async () => {
    const originalImage = globalThis.Image
    vi.stubGlobal('Image', class {
      naturalWidth = 640
      naturalHeight = 480
      set src(value) { queueMicrotask(() => this.onload?.()) }
    })
    let finishOldSnapshot
    api.PinFileSnapshot.mockImplementationOnce(() => new Promise(resolve => { finishOldSnapshot = resolve })).mockResolvedValue(undefined)
    api.cachedPreviewImageURL.mockImplementation(async (user) => `http://127.0.0.1/cache/${user}`)
    try {
      const file = { file_id: 'shared-id', name: 'photo.png', size: 100 }
      const wrapper = mountAttached(PreviewModal, { props: { account: { user_id: 'first', drive_id: 'first-drive' }, file } })
      await flushPromises()
      await wrapper.setProps({ account: { user_id: 'second', drive_id: 'second-drive' } })
      await flushPromises()
      finishOldSnapshot()
      await flushPromises()
      expect(api.cachedPreviewImageURL.mock.calls.map(call => [call[0], call[1]])).toEqual(expect.arrayContaining([['first', 'first-drive'], ['second', 'second-drive']]))
    } finally {
      vi.stubGlobal('Image', originalImage)
    }
  })

  it('音频滑杆保留原生方向键行为，不触发全局音量快捷键', async () => {
    api.PinFileSnapshot.mockResolvedValue(undefined)
    api.getPlayCursor.mockResolvedValue(0)
    api.savePlayCursor.mockResolvedValue(undefined)
    api.PreviewURL.mockResolvedValue('https://example.test/audio.wav')
    const wrapper = mountAttached(PreviewModal, { props: { account: { user_id: 'test', drive_id: 'test' }, file: { file_id: 'test', name: 'test.wav' } } })
    await flushPromises()
    const before = wrapper.vm.audioVolume
    expect(document.querySelector('.pv-audio-vol-range'), document.body.textContent).not.toBeNull()
    document.querySelector('.pv-audio-vol-range').dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowUp', bubbles: true, cancelable: true }))
    expect(wrapper.vm.audioVolume).toBe(before)
  })

  it('账号侧栏键盘焦点到达添加按钮后仍可返回账号', async () => {
    const wrapper = mountAttached(AccountRail, { props: { accounts: [{ user_id: 'pikpak:one' }] } })
    const account = wrapper.get('.rail-item')
    account.element.focus()
    await account.trigger('keydown', { key: 'ArrowDown' })
    expect(document.activeElement).toBe(wrapper.get('.rail-add').element)
    await wrapper.get('.rail-add').trigger('keydown', { key: 'ArrowUp' })
    expect(document.activeElement).toBe(account.element)
  })

  it('鼠标仍在账号侧栏内时焦点移出不会收起侧栏', async () => {
    vi.useFakeTimers()
    const wrapper = mountAttached(AccountRail, { props: { accounts: [{ user_id: 'pikpak:one' }] } })
    const outside = document.createElement('button')
    document.body.appendChild(outside)
    await wrapper.get('.account-rail').trigger('mouseenter')
    await vi.advanceTimersByTimeAsync(250)
    wrapper.get('.rail-item').element.focus()
    outside.focus()
    await vi.advanceTimersByTimeAsync(250)
    expect(wrapper.get('.account-rail').classes()).toContain('expanded')
    await wrapper.get('.account-rail').trigger('mouseleave')
    await vi.advanceTimersByTimeAsync(250)
    expect(wrapper.get('.account-rail').classes()).not.toContain('expanded')
  })

  it('账号侧栏右键菜单打开时保持宽度，不因鼠标移入菜单而收起', async () => {
    vi.useFakeTimers()
    const account = { user_id: 'pikpak:one' }
    const wrapper = mountAttached(AccountRail, { props: { accounts: [account] } })
    await wrapper.get('.account-rail').trigger('mouseenter')
    await vi.advanceTimersByTimeAsync(250)
    expect(wrapper.get('.account-rail').classes()).toContain('expanded')
    await wrapper.get('.rail-item').trigger('contextmenu', { clientX: 80, clientY: 100 })
    await wrapper.get('.account-rail').trigger('mouseleave')
    await vi.advanceTimersByTimeAsync(250)
    expect(wrapper.get('.account-rail').classes()).toContain('expanded')
    expect(wrapper.emitted('select')).toBeUndefined()
  })

  it('账号拖拽取消或组件卸载时不保存临时顺序并清理拖动状态', async () => {
    const save = vi.spyOn(appearance, 'setPref')
    const accounts = [{ user_id: 'pikpak:one' }, { user_id: 'pikpak:two' }]
    const wrapper = mountAttached(AccountRail, { props: { accounts } })
    await wrapper.findAll('.rail-item')[0].trigger('pointerdown', { button: 0, clientX: 10, clientY: 10 })
    window.dispatchEvent(new MouseEvent('pointermove', { clientX: 10, clientY: 30 }))
    expect(document.body.classList.contains('rail-drag-active')).toBe(true)
    window.dispatchEvent(new MouseEvent('pointercancel'))
    expect(save).not.toHaveBeenCalled()
    expect(document.body.classList.contains('rail-drag-active')).toBe(false)
    await wrapper.findAll('.rail-item')[0].trigger('pointerdown', { button: 0, clientX: 10, clientY: 10 })
    window.dispatchEvent(new MouseEvent('pointermove', { clientX: 10, clientY: 30 }))
    wrapper.unmount()
    wrappers.splice(wrappers.indexOf(wrapper), 1)
    expect(document.body.classList.contains('rail-drag-active')).toBe(false)
    window.dispatchEvent(new MouseEvent('pointerup'))
    expect(save).not.toHaveBeenCalled()
  })

  it('账号拖拽换位时显示幽灵与碰撞动画，并保存新顺序', async () => {
    const frames = new Map()
    let frameID = 0
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => {
      frames.set(++frameID, callback)
      return frameID
    })
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(id => frames.delete(id))
    const flushFrame = async () => {
      const pending = [...frames.values()]
      frames.clear()
      pending.forEach(callback => callback(0))
      await nextTick()
    }
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function () {
      const items = [...document.querySelectorAll('.rail-list .rail-item')]
      const index = items.indexOf(this)
      const top = index < 0 ? 0 : index * 50
      return { top, bottom: top + 44, left: 0, right: 44, width: 44, height: 44 }
    })
    const save = vi.spyOn(appearance, 'setPref')
    const accounts = [{ user_id: 'pikpak:one' }, { user_id: 'pikpak:two' }]
    const wrapper = mountAttached(AccountRail, { props: { accounts } })
    await wrapper.findAll('.rail-item')[0].trigger('pointerdown', { button: 0, clientX: 10, clientY: 10 })
    window.dispatchEvent(new MouseEvent('pointermove', { clientX: 10, clientY: 80 }))
    await flushFrame()
    expect(wrapper.find('.rail-ghost').exists()).toBe(true)
    expect(wrapper.find('.rail-item.dragging').exists()).toBe(true)
    for (let step = 0; step < 8; step++) await flushFrame()
    expect(wrapper.find('.rail-item.bump-up').exists()).toBe(true)
    expect(wrapper.findAll('.rail-list .rail-item').map(item => item.attributes('aria-label'))).toEqual(['pikpak · pikpak:two', 'pikpak · pikpak:one'])
    window.dispatchEvent(new MouseEvent('pointerup', { clientX: 10, clientY: 80 }))
    expect(save).toHaveBeenCalledWith('accountOrder', ['pikpak:two', 'pikpak:one'])
    expect(wrapper.find('.rail-ghost.dropping').exists()).toBe(true)
  })

  it.each(['首帧之前松手', '动画尚未收敛', '松手位置更新'])('账号拖拽%s时立即保存最终位置，重新挂载后保留顺序', async (mode) => {
    vi.useFakeTimers()
    const frames = new Map()
    let frameID = 0
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => {
      frames.set(++frameID, callback)
      return frameID
    })
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(id => frames.delete(id))
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function () {
      const index = [...document.querySelectorAll('.rail-list .rail-item')].indexOf(this)
      const top = Math.max(index, 0) * 50
      return { top, bottom: top + 44, left: 0, right: 44, width: 44, height: 44 }
    })
    const accounts = [{ user_id: 'pikpak:one' }, { user_id: 'pikpak:two' }, { user_id: 'pikpak:three' }]
    const wrapper = mountAttached(AccountRail, { props: { accounts } })
    await wrapper.findAll('.rail-item')[0].trigger('pointerdown', { button: 0, clientX: 10, clientY: 10 })
    window.dispatchEvent(new MouseEvent('pointermove', { clientX: 10, clientY: mode === '松手位置更新' ? 20 : 120 }))
    if (mode !== '首帧之前松手') {
      for (let step = 0; step < 2; step++) {
        const pending = [...frames.values()]
        frames.clear()
        pending.forEach(callback => callback(0))
        await nextTick()
      }
    }
    window.dispatchEvent(new MouseEvent('pointerup', { clientX: 10, clientY: 120 }))
    expect(JSON.parse(localStorage.getItem('mnemo.prefs')).accountOrder).toEqual(['pikpak:two', 'pikpak:three', 'pikpak:one'])
    await wrapper.setProps({ accounts: accounts.map(account => ({ ...account })) })
    await vi.advanceTimersByTimeAsync(300)
    expect(wrapper.find('.rail-ghost').exists()).toBe(false)
    wrapper.unmount()
    wrappers.splice(wrappers.indexOf(wrapper), 1)
    const restored = mountAttached(AccountRail, { props: { accounts } })
    expect(restored.findAll('.rail-list .rail-item').map(item => item.attributes('aria-label'))).toEqual([
      'pikpak · pikpak:two', 'pikpak · pikpak:three', 'pikpak · pikpak:one',
    ])
  })

  it('固定右栏目标后导航不会改变确认的复制位置', async () => {
    const account = { user_id: 'webdav:pinned', drive_id: 'drive' }
    api.listDir.mockResolvedValue([{ file_id: 'file', name: '文件.txt', isDir: false }])
    api.capsOf.mockReturnValue({ copy: true })
    api.copy.mockResolvedValue(['file'])
    const wrapper = mountAttached(WorkspaceView, { props: { account, accounts: [account], providers: [], dual: true } })
    await flushPromises()
    const views = wrapper.findAllComponents(PanView)
    await views[1].vm.navigate({ dirId: 'target', name: '固定位置' })
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '固定右栏目标').trigger('click')
    await views[1].vm.navigate({ dirId: 'other', name: '另一个位置' })
    await flushPromises()
    await views[0].get('.fileitem').trigger('click')
    await wrapper.findAll('button').find(button => button.text() === '复制到右栏').trigger('click')
    await flushPromises()
    expect(api.copy).not.toHaveBeenCalled()
    expect(document.body.querySelector('.transfer-summary')?.textContent).toContain('固定位置')
    await wrapper.vm.confirmTransfer()
    expect(api.copy).toHaveBeenCalledWith(account.user_id, account.drive_id, ['file'], 'target')
  })

  it.each(['audio', 'text'])('%s 预览关闭后释放迟到的本机预览会话', async (kind) => {
    vi.spyOn(api, 'openKindOf').mockReturnValue(kind)
    api.PinFileSnapshot.mockResolvedValue(undefined)
    let resolvePreview
    api.PreviewURL.mockImplementationOnce(() => new Promise(resolve => { resolvePreview = resolve }))
    const fetch = vi.spyOn(globalThis, 'fetch').mockResolvedValue({ ok: true })
    const wrapper = mountAttached(PreviewModal, { props: { account: { user_id: 'test', drive_id: 'test' }, file: { file_id: 'late', name: kind === 'audio' ? 'late.wav' : 'late.txt' } } })
    await flushPromises()
    wrapper.unmount()
    wrappers.splice(wrappers.indexOf(wrapper), 1)
    const address = 'http://127.0.0.1:45678/stream/late-preview?t=local-token'
    resolvePreview(address)
    await flushPromises()
    expect(fetch).toHaveBeenCalledWith(address, expect.objectContaining({ method: 'DELETE', keepalive: true }))
    expect(fetch).toHaveBeenCalledTimes(1)
  })

  it('账号排序存储失败时提示失败并清理拖动状态', async () => {
    vi.spyOn(window, 'requestAnimationFrame').mockReturnValue(1)
    const wrapper = mountAttached(AccountRail, { props: { accounts: [{ user_id: 'pikpak:one' }, { user_id: 'pikpak:two' }] } })
    vi.spyOn(localStorage, 'setItem').mockImplementationOnce(() => { throw new Error('storage unavailable') })
    await wrapper.findAll('.rail-item')[0].trigger('pointerdown', { button: 0, clientX: 10, clientY: 10 })
    window.dispatchEvent(new MouseEvent('pointermove', { clientX: 10, clientY: 80 }))
    window.dispatchEvent(new MouseEvent('pointerup', { clientX: 10, clientY: 80 }))
    await nextTick()
    expect(wrapper.emitted('toast')?.[0]?.[0]).toContain('保存失败')
    expect(wrapper.find('.rail-ghost').exists()).toBe(false)
    expect(wrapper.get('.account-rail').classes()).not.toContain('rail-frozen')
    expect(document.body.classList.contains('rail-drag-active')).toBe(false)
    expect(wrapper.find('.rail-item.dragging').exists()).toBe(false)
    expect(localStorage.getItem('mnemo.prefs')).toBeNull()
  })

  it('目录树点击已展开目录只导航，箭头才折叠', async () => {
    const node = { file_id: 'folder', name: '文件夹' }
    const wrapper = mountAttached(TreeNode, { props: { node, tree: {}, expanded: { folder: true } } })
    await wrapper.get('.tn-label').trigger('click')
    expect(wrapper.emitted('select')).toEqual([[node]])
    expect(wrapper.emitted('toggle')).toBeUndefined()
    await wrapper.get('.tn-arrow').trigger('click')
    expect(wrapper.emitted('toggle')).toEqual([[node]])
    expect(wrapper.emitted('select')).toHaveLength(1)
  })

  it('右键菜单按实际尺寸避让窗口边缘', async () => {
    const bounds = vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function () {
      return this.classList.contains('ctx-menu') ? { width: 340, height: 120 } : { width: 0, height: 0 }
    })
    try {
      mountAttached(ContextMenu, { props: { x: window.innerWidth - 10, y: window.innerHeight - 10, items: [{ label: '较长的操作名称', action: 'open' }] } })
      await flushPromises()
      const menu = document.querySelector('.ctx-menu')
      expect(parseFloat(menu.style.left) + 340).toBeLessThanOrEqual(window.innerWidth - 8)
      expect(parseFloat(menu.style.top) + 120).toBeLessThanOrEqual(window.innerHeight - 8)
    } finally {
      bounds.mockRestore()
    }
  })

  it('菜单鼠标焦点与键盘确认一致，更新项目后重置选项', async () => {
    const wrapper = mountAttached(ContextMenu, { props: { x: 10, y: 10, items: [
      { label: '打开', action: 'open' }, { label: '删除', action: 'delete' },
    ] } })
    await flushPromises()
    const buttons = document.querySelectorAll('.ctx-item')
    buttons[1].focus()
    buttons[1].dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true }))
    expect(wrapper.emitted('select')).toEqual([['delete']])
    await wrapper.setProps({ items: [{ label: '刷新', action: 'refresh' }] })
    await flushPromises()
    expect(document.activeElement.textContent).toContain('刷新')
  })

  it('菜单外部滚动关闭，内部滚动不关闭，Escape 恢复触发位置焦点', async () => {
    const opener = document.createElement('button')
    document.body.appendChild(opener)
    opener.focus()
    const wrapper = mountAttached(ContextMenu, { props: { x: 10, y: 10, items: [{ label: '打开', action: 'open' }] } })
    await flushPromises()
    const menu = document.querySelector('.ctx-menu')
    menu.dispatchEvent(new Event('scroll'))
    expect(wrapper.emitted('close')).toBeUndefined()
    document.activeElement.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
    wrappers.splice(wrappers.indexOf(wrapper), 1)
    expect(document.activeElement).toBe(opener)
    const second = mountAttached(ContextMenu, { props: { x: 10, y: 10, items: [{ label: '刷新' }] } })
    await flushPromises()
    window.dispatchEvent(new Event('scroll'))
    expect(second.emitted('close')).toHaveLength(1)
  })

  it('PikPak 独立窗口使用原会话，提前到达的最终 token 只自动登录一次', async () => {
    localStorage.setItem('login_provider', 'pikpak')
    let complete
    api.onEvent.mockImplementation((name, callback) => {
      if (name === 'pikpak:captcha:completed') complete = callback
      return () => {}
    })
    api.login.mockReset()
    api.login.mockRejectedValueOnce(new Error('pikpak: captcha_required\nurl=https://user.mypikpak.com/challenge\ntoken=initial-token\nsession=current-session'))
      .mockResolvedValueOnce(undefined)
    let opened
    api.ShowPikPakCaptcha.mockImplementation(() => new Promise(resolve => { opened = resolve }))
    const wrapper = mountAttached(LoginModal, {
      props: { providers: [{ ID: 'pikpak', Meta: { label: 'PikPak' }, Login: { fields: [
        { key: 'username', type: 'text', label: '账号', required: true },
        { key: 'password', type: 'password', label: '密码', required: true },
      ] } }] },
      global: { stubs: { UiIcon: true } },
    })
    await nextTick()
    const inputs = [...document.body.querySelectorAll('input')]
    await setDomInput(inputs.find(input => input.type === 'text'), 'test@example.test')
    await setDomInput(inputs.find(input => input.type === 'password'), 'password')
    document.body.querySelector('form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await flushPromises()
    expect(api.ShowPikPakCaptcha).toHaveBeenCalledWith('current-session', 'https://user.mypikpak.com/challenge')
    expect(document.body.querySelector('iframe')).toBeNull()
    complete({ session_id: 'stale-session', captcha_token: 'wrong' })
    complete({ session_id: 'current-session', captcha_token: 'final-token' })
    expect(api.login).toHaveBeenCalledTimes(1)
    opened(true)
    await flushPromises()
    expect(api.login).toHaveBeenCalledTimes(2)
    expect(api.login.mock.calls[1][1]).toMatchObject({ captcha_token: 'final-token', captcha_verified: 'true' })
    complete({ session_id: 'current-session', captcha_token: 'final-token' })
    await flushPromises()
    expect(api.login).toHaveBeenCalledTimes(2)
    expect(wrapper.emitted('close')).toHaveLength(1)
    api.onEvent.mockImplementation(() => () => {})
  })

  it('弹窗声明对话框语义，并由 Escape 请求关闭和恢复焦点', async () => {
    const opener = document.createElement('button')
    document.body.appendChild(opener)
    opener.focus()

    const wrapper = mountAttached(Modal, {
      props: { title: '删除文件' },
      slots: { default: '<button type="button">确认</button>' },
    })
    await nextTick()

    const dialog = document.querySelector('[role="dialog"]')
    expect(dialog).not.toBeNull()
    expect(dialog.getAttribute('aria-modal')).toBe('true')
    expect(dialog.getAttribute('aria-label')).toBe('删除文件')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    expect(wrapper.emitted('close')).toHaveLength(1)

    wrapper.unmount()
    wrappers.splice(wrappers.indexOf(wrapper), 1)
    await nextTick()
    expect(document.activeElement).toBe(opener)
  })

  it('普通弹窗统一提供经典三键，沉浸式弹窗不渲染实体标题栏', async () => {
    const standard = mountAttached(Modal, { props: { title: '窗口控制' } })
    await nextTick()

    const controls = [...document.querySelectorAll('.modal-window-btn')]
    expect(controls).toHaveLength(3)
    expect(controls.map((button) => button.getAttribute('aria-label'))).toEqual(['最小化窗口', '最大化窗口', '关闭对话框'])

    standard.unmount()
    wrappers.splice(wrappers.indexOf(standard), 1)
    await nextTick()

    mountAttached(Modal, { props: { title: '图片预览', hideHead: true } })
    await nextTick()
    expect(document.querySelector('.modal-head')).toBeNull()
    expect(document.querySelectorAll('.modal-window-btn')).toHaveLength(0)
  })

  it('下拉框以键盘跳过禁用项并提交当前可选项', async () => {
    const wrapper = mountAttached(UiSelect, {
      props: {
        modelValue: 'alpha',
        placeholder: '选择供应商',
        options: [
          { value: 'alpha', label: 'Alpha' },
          { value: 'blocked', label: '不可用', disabled: true },
          { value: 'beta', label: 'Beta' },
        ],
      },
    })
    const trigger = wrapper.get('[role="combobox"]')

    await trigger.trigger('keydown', { key: 'ArrowDown' })
    await nextTick()
    expect(trigger.attributes('aria-expanded')).toBe('true')
    expect(document.querySelector('[role="listbox"]')).not.toBeNull()

    await trigger.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')).toEqual([['beta']])
    expect(wrapper.emitted('change')).toEqual([['beta']])
    expect(trigger.attributes('aria-expanded')).toBe('false')
  })

  it('迅雷只显示手机号和密码，设备 ID 自动生成并在重新打开后复用', async () => {
    localStorage.setItem('login_provider', 'thunder')
    api.login.mockResolvedValueOnce(undefined)
    mountAttached(LoginModal, {
      props: { providers: [{
        ID: 'thunder', Meta: { label: '迅雷网盘' }, Login: { fields: [
          { key: 'username', type: 'text', label: '手机号', required: true },
          { key: 'password', type: 'password', label: '密码', required: true },
        ] },
      }] },
      global: { stubs: { UiIcon: true } },
    })
    await nextTick()
    const fields = [...document.body.querySelectorAll('.login-field')]
    expect(fields.map(field => field.querySelector('label')?.textContent)).toEqual(['手机号', '密码'])
    const phoneInput = fields[0].querySelector('input')
    const passwordInput = fields[1].querySelector('input')
    expect(passwordInput.type).toBe('password')
    expect(phoneInput.getAttribute('inputmode')).toBe('numeric')
    await setDomInput(phoneInput, '13800138000')
    await setDomInput(passwordInput, 'login-password')
    const deviceID = localStorage.getItem('mnemo.thunder.deviceId')
    expect(deviceID).toMatch(/^[a-f0-9]{32}$/)
    document.body.querySelector('form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await flushPromises()
    expect(api.login).toHaveBeenCalledWith('thunder', { username: '13800138000', password: 'login-password', device_id: deviceID, login_mode: 'password' })
    expect(JSON.stringify([...storage.values()])).not.toContain('login-password')
    const next = mountAttached(LoginModal, { props: { initialProvider: 'thunder', providers: [{ ID: 'thunder', Meta: { label: '迅雷网盘' }, Login: { fields: [] } }] }, global: { stubs: { UiIcon: true } } })
    expect(next.vm.form.device_id).toBe(deviceID)
  })

  it('WebDAV 预设会同时填写连接名称和地址，且密码只有一个自定义切换按钮', async () => {
    localStorage.setItem('login_provider', 'webdav')
    const wrapper = mountAttached(LoginModal, {
      props: {
        providers: [
          { ID: 'webdav', Meta: { label: 'WebDAV' }, Login: { fields: [] } },
        ],
      },
      global: { stubs: { UiIcon: true } },
    })
    await nextTick()
    const preset = wrapper.findAllComponents(UiSelect)[0]
    await preset.vm.$emit('update:modelValue', 'jianguoyun')
    await preset.vm.$emit('change', 'jianguoyun')
    await nextTick()

    const inputs = [...document.body.querySelectorAll('input')]
    expect(inputs.find((input) => input.getAttribute('placeholder') === '我的 WebDAV / S3').value).toBe('坚果云')
    expect(inputs.find((input) => input.getAttribute('placeholder') === 'https://dav.example.com').value).toBe('https://dav.jianguoyun.com/dav/')
    expect(document.body.querySelectorAll('.password-toggle')).toHaveLength(1)
    expect(document.body.querySelector('[role="switch"]').getAttribute('aria-checked')).toBe('false')
  })

  it('普通蓝奏使用内置选择项，天翼云不再要求选择云空间', async () => {
    localStorage.setItem('login_provider', 'lanzou')
    const lanzou = mountAttached(LoginModal, {
      props: {
        providers: [{
          ID: 'lanzou', Meta: { label: '蓝奏云' }, Login: { fields: [
            { key: 'username', type: 'text', label: '账号', required: true },
            { key: 'password', type: 'password', label: '密码', required: true },
            { key: 'upload_tier', type: 'select', label: '会员等级', options: [
              { value: 'v0', label: 'V0（100 MB）' },
              { value: 'v3', label: 'V3（550 MB）' },
            ] },
          ] },
        }],
      },
      global: { stubs: { UiIcon: true } },
    })
    await nextTick()
    const tier = lanzou.findComponent(UiSelect)
    expect(tier.props('modelValue')).toBe('v0')
    expect(tier.props('options')).toContainEqual({ value: 'v3', label: 'V3（550 MB）' })

    lanzou.unmount()
    wrappers.splice(wrappers.indexOf(lanzou), 1)
    localStorage.setItem('login_provider', 'pan189')
    const pan189 = mountAttached(LoginModal, {
      props: {
        providers: [{
          ID: 'pan189', Meta: { label: '天翼云盘' }, Login: { fields: [
            { key: 'username', type: 'text', label: '账号', required: true },
            { key: 'password', type: 'password', label: '密码', required: true },
          ] },
        }],
      },
      global: { stubs: { UiIcon: true } },
    })
    await nextTick()
    expect(pan189.findAllComponents(UiSelect)).toHaveLength(0)
  })

  it.each(['pan139', 'pan189'])('%s 可主动选择短信登录且不要求密码', async (id) => {
    localStorage.setItem('login_provider', id)
    api.login.mockResolvedValue(undefined)
    api.SendPan139SMS.mockResolvedValue(undefined)
    api.SendPan189SMS.mockResolvedValue('')
    if (id === 'pan189') api.SendPan189SMS.mockResolvedValueOnce('data:image/png;base64,dGVzdA==')
    const wrapper = mountAttached(LoginModal, {
      props: { providers: [{ ID: id, Meta: { label: id }, Login: { fields: [
        { key: 'login_mode', type: 'select', label: '登录方式', required: true, options: [{ value: 'password', label: '账号密码' }, { value: 'sms', label: '短信验证码' }] },
        { key: 'username', type: 'text', label: '手机号', required: true },
        { key: 'password', type: 'password', label: '密码', required: true },
        { key: 'sms_code', type: 'text', label: '短信验证码' },
        { key: 'validate_code', type: 'text', label: '图形验证码' },
      ] } }] }, global: { stubs: { UiIcon: true } },
    })
    const mode = document.querySelector('.login-method-switch')
    expect(mode.querySelectorAll('button')).toHaveLength(2)
    expect(wrapper.findComponent(UiSelect).exists()).toBe(false)
    mode.querySelectorAll('button')[1].click()
    await nextTick()
    const field = (label) => [...document.body.querySelectorAll('.login-field')].find((f) => f.querySelector('label')?.textContent === label)
    expect(field('密码').style.display).toBe('none')
    expect(field('短信验证码').style.display).not.toBe('none')
    await setDomInput(field('手机号').querySelector('input'), '13800138000')
    await field('短信验证码').querySelector('button').click()
    await flushPromises()
    const send = id === 'pan139' ? api.SendPan139SMS : api.SendPan189SMS
    expect(send.mock.calls[0][0]).toBe('13800138000')
    if (id === 'pan189') {
      expect(document.body.querySelector('.captcha-image-row img')?.getAttribute('src')).toBe('data:image/png;base64,dGVzdA==')
      expect(field('图形验证码').style.display).not.toBe('none')
      expect(field('短信验证码').querySelector('button').disabled).toBe(false)
      await setDomInput(field('图形验证码').querySelector('input'), 'ABCD')
      await field('短信验证码').querySelector('button').click()
      await flushPromises()
      expect(api.SendPan189SMS).toHaveBeenLastCalledWith('13800138000', 'ABCD')
    }
    await setDomInput(field('短信验证码').querySelector('input'), '123456')
    document.body.querySelector('form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await flushPromises()
    expect(api.login).toHaveBeenCalledWith(id, expect.objectContaining({ login_mode: 'sms', username: '13800138000', sms_code: '123456' }))
    expect(api.login.mock.calls[0][1].password).toBeUndefined()
  })

  it.each(['139 登录需要短信安全校验', '139 登录失败：S305 尚未设置移动认证账号密码'])('139 账密失败后显示原因并切换短信：%s', async (reason) => {
    localStorage.setItem('login_provider', 'pan139')
    api.login
      .mockRejectedValueOnce(new Error(`pan139_sms_required\n${reason}`))
      .mockResolvedValueOnce(undefined)
    api.SendPan139SMS.mockResolvedValue(undefined)
    const wrapper = mountAttached(LoginModal, {
      props: {
        providers: [{
          ID: 'pan139', Meta: { label: '移动云盘' }, Login: { fields: [
            { key: 'login_mode', type: 'select', label: '登录方式', required: true, options: [] },
            { key: 'username', type: 'text', label: '手机号/账号', required: true },
            { key: 'password', type: 'password', label: '密码', required: false },
            { key: 'sms_code', type: 'text', label: '短信验证码', required: false },
          ] },
        }],
      },
      global: { stubs: { UiIcon: true } },
    })
    await nextTick()

    const inputForLabel = (label) => [...document.body.querySelectorAll('.login-field')]
      .find((field) => field.querySelector('label')?.textContent === label)
      ?.querySelector('input')
    await setDomInput(inputForLabel('手机号/账号'), '13800138000')
    await setDomInput(inputForLabel('密码'), 'password')
    document.body.querySelector('form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await Promise.resolve()
    await nextTick()

    expect(api.login).toHaveBeenCalledTimes(1)
    expect(document.body.querySelector('.form-error')?.textContent).toContain('请获取并填写短信验证码')
    expect(document.body.querySelector('.form-error')?.textContent).toContain(reason)
    const smsField = [...document.body.querySelectorAll('.login-field')]
      .find((field) => field.querySelector('label')?.textContent === '短信验证码')
    expect(smsField.style.display).not.toBe('none')
    expect([...document.body.querySelectorAll('.login-field')]
      .find((field) => field.querySelector('label')?.textContent === '密码').style.display).toBe('none')

    const sendButton = [...smsField.querySelectorAll('button')].find((button) => button.textContent.includes('获取验证码'))
    await sendButton.click()
    await Promise.resolve()
    expect(api.SendPan139SMS).toHaveBeenCalledWith('13800138000')

    await setDomInput(inputForLabel('短信验证码'), '123456')
    document.body.querySelector('form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await Promise.resolve()
    await nextTick()
    expect(api.login).toHaveBeenCalledTimes(2)
    expect(api.login.mock.calls[1][0]).toBe('pan139')
    expect(api.login.mock.calls[1][1]).toMatchObject({ login_mode: 'sms', username: '13800138000', sms_code: '123456' })
  })

  it('S3 默认不做写入验证和内网预览授权，保存时只提交一次连接配置', async () => {
    api.saveMounted.mockResolvedValue(undefined)
    localStorage.setItem('login_provider', 's3')
    const wrapper = mountAttached(LoginModal, {
      props: {
        providers: [
          { ID: 'webdav', Meta: { label: 'WebDAV' }, Login: { fields: [] } },
          { ID: 's3', Meta: { label: 'S3' }, Login: { fields: [] } },
        ],
      },
      global: { stubs: { UiIcon: true } },
    })
    await nextTick()

    const values = { endpoint: 'https://s3.example.test', username: 'access-key', password: 'secret-key', bucket: 'mnemo' }
    const inputs = [...document.body.querySelectorAll('input')]
    await setDomInput(inputs.find((input) => input.placeholder === 's3.us-east-1.amazonaws.com (可选，默认 AWS)'), values.endpoint)
    const inputForLabel = (label) => [...document.body.querySelectorAll('.login-field')]
      .find((field) => field.querySelector('label')?.textContent.startsWith(label))
      ?.querySelector('input')
    await setDomInput(inputForLabel('Access Key ID'), values.username)
    await setDomInput(inputForLabel('Secret Access Key'), values.password)
    await setDomInput(inputForLabel('Bucket'), values.bucket)

    const switches = [...document.body.querySelectorAll('[role="switch"]')]
    expect(switches).toHaveLength(3)
    expect(switches[0].getAttribute('aria-checked')).toBe('false')
    expect(switches[1].getAttribute('aria-checked')).toBe('true')
    expect(switches[2].getAttribute('aria-checked')).toBe('false')
    document.body.querySelector('form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await nextTick()

    expect(api.validateMountedWrite).not.toHaveBeenCalled()
    expect(api.saveMounted).toHaveBeenCalledTimes(1)
    expect(api.saveMounted.mock.calls[0][0]).toBe('s3')
    expect(api.saveMounted.mock.calls[0][1]).toMatchObject({ name: 'S3', endpoint: values.endpoint, username: values.username, password: values.password, bucket: values.bucket, allowPrivateNetwork: false })
    expect(api.saveMounted.mock.calls[0][1]).not.toHaveProperty('verifyWrite')
  })

  it('登录请求期间锁定服务商，避免异步结果串到另一个网盘', async () => {
    localStorage.setItem('login_provider', 'dropbox')
    let resolveLogin
    api.login.mockImplementation(() => new Promise((resolve) => { resolveLogin = resolve }))
    const wrapper = mountAttached(LoginModal, {
      props: {
        providers: [
          { ID: 'dropbox', Meta: { label: 'Dropbox' }, Login: { fields: [{ key: 'oauth', type: 'oauth', label: '浏览器授权' }] } },
          { ID: 'pikpak', Meta: { label: 'PikPak' }, Login: { fields: [{ key: 'username', type: 'text', label: '账号', required: true }] } },
        ],
      },
      global: { stubs: { UiIcon: true } },
    })
    await nextTick()
    document.body.querySelector('form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await nextTick()

    expect(api.login).toHaveBeenCalledWith('dropbox', {})
    const providerButtons = [...document.body.querySelectorAll('.lp-item')]
    const pikpakButton = providerButtons.find((button) => button.textContent.includes('PikPak'))
    expect(pikpakButton.disabled).toBe(true)
    expect(api.login).toHaveBeenCalledTimes(1)
    expect(providerButtons.find((button) => button.classList.contains('active')).textContent).toContain('Dropbox')

    resolveLogin()
    await Promise.resolve()
    await nextTick()
  })

  it('账号容量在启动同步，并支持右上角手动同步', async () => {
    vi.useFakeTimers()
    api.refreshAccountSilently.mockResolvedValue({ user_id: 'quota-dedupe', token: {}, usage: { size: 100, used: 20 } })
    api.refreshAccountNow.mockResolvedValue({ user_id: 'quota-dedupe', token: {}, usage: { size: 100, used: 20 } })
    const account = { user_id: 'quota-dedupe', token: {}, usage: null }
    const wrapper = mountAttached(AccountAvatar, { props: { account, providers: [] }, global: { stubs: { UiIcon: true } } })

    await vi.runAllTicks()
    expect(api.refreshAccountSilently).toHaveBeenCalledTimes(1)
    expect(api.refreshAccountNow).not.toHaveBeenCalled()
    await wrapper.get('.acc-ava').trigger('mouseenter')
    await vi.advanceTimersByTimeAsync(120)
    const refreshButton = document.querySelector('.ap-refresh')
    expect(refreshButton).not.toBeNull()
    refreshButton.click()
    await vi.runAllTicks()
    expect(api.refreshAccountNow).toHaveBeenCalledTimes(1)
  })

  it('家庭云容量只显示在右上角弹窗，侧边账号栏保持个人云容量', async () => {
	vi.useFakeTimers()
    api.refreshAccountSilently.mockResolvedValue({ user_id: 'pan189:family-quota', token: {}, usage: { size: 100, used: 10 }, family_usage: { size: 200, used: 20 } })
    const account = { user_id: 'pan189:family-quota', token: {}, usage: { size: 100, used: 10, sizeStr: '100 B', usedStr: '10 B' }, family_usage: { size: 200, used: 20, sizeStr: '200 B', usedStr: '20 B' } }
    const avatar = mountAttached(AccountAvatar, { props: { account, providers: [] }, global: { stubs: { UiIcon: true } } })
    await avatar.get('.acc-ava').trigger('mouseenter')
    await vi.advanceTimersByTimeAsync(120)
    expect(document.querySelector('.ap-quota')?.textContent).toContain('个人云')
    expect(document.querySelector('.ap-quota')?.textContent).toContain('家庭云')
    const rail = mountAttached(AccountRail, { props: { accounts: [account], current: account, providers: [] } })
    expect(rail.text()).not.toContain('家庭云')
    expect(rail.text()).toContain('10 B / 100 B')
    expect(rail.text()).not.toContain('20 B / 200 B')
    expect(rail.find('.rail-quota').exists()).toBe(true)
  })
})
