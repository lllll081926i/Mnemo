import { readFileSync } from 'fs'
import path from 'path'
import { describe, expect, it } from 'vitest'

const read = (file: string) => readFileSync(path.resolve(process.cwd(), file), 'utf8').replace(/\r\n/g, '\n')

describe('provider upload routing', () => {
  it('routes every retained queue-enabled provider to a concrete handler', () => {
    const uploader = read('src/workerpage/uploader.ts')
    for (const provider of ['pikpak', 'onedrive', 'dropbox', 'gdrive', 'gofile']) {
      expect(uploader).toContain(`if (provider === '${provider}') return runUploadDisk`)
    }
    for (const handler of ['PikPakUploadDisk', 'OneDriveUploadDisk', 'DropboxUploadDisk', 'GoogleDriveUploadDisk', 'GofileUploadDisk']) {
      expect(uploader).toContain(handler)
    }
    for (const handler of ['GuangyaUploadDisk', 'QuarkUploadDisk', 'Cloud139UploadDisk', 'Cloud189UploadDisk', 'AliUploadDisk', 'AliUploadHashPool']) expect(uploader).not.toContain(handler)
  })

  it('keeps provider uploads cancellable and chunks Google Drive resumable uploads', () => {
    const helper = read('src/utils/providerUpload.ts')
    const google = read('src/gdrive/upload.ts')
    const gofile = read('src/gofile/upload.ts')
    const oneDrive = read('src/onedrive/upload.ts')
    const dropbox = read('src/dropbox/upload.ts')
    expect(helper).toContain('fetchCancellableProviderUpload')
    expect(helper).toContain("if (!fileui.IsRunning) controller.abort('已暂停')")
    expect(google).toContain('const UPLOAD_CHUNK_SIZE = 8 * 1024 * 1024')
    expect(google).toContain('response.status !== 308')
    expect(gofile).toContain('fetchCancellableProviderUpload(fileui')
    expect(oneDrive).toContain('fetchCancellableProviderUpload(fileui')
    expect(dropbox).toContain("req.destroy(new Error('已暂停'))")
    expect(dropbox).toContain("if (fileui && !fileui.IsRunning) return { error: '已暂停' }")
  })

  it('routes added providers through their own file and share APIs', () => {
    const file = read('src/aliapi/file.ts')
    const filecmd = read('src/aliapi/filecmd.ts')
    const share = read('src/aliapi/share.ts')
    for (const provider of ['onedrive', 'dropbox', 'gdrive', 'gofile']) {
      expect(file).toContain(`provider === '${provider}'`)
      expect(filecmd).toContain(`provider === '${provider}'`)
      expect(share).toContain(`provider === '${provider}'`)
    }
    expect(file).toContain('apiGoogleDriveDownloadInfo')
    expect(filecmd).toContain('apiGoogleDriveTrashBatch')
    expect(filecmd).toContain('apiGoogleDriveRestoreBatch')
    expect(filecmd).toContain("resolveDriveProvider({ userId: user_id, driveId: drive_id }) === 'gdrive'")
    expect(filecmd).toContain("isDir: info.type === 'directory'")
    expect(filecmd).toContain('isDir: !!detail.isDir')
    expect(filecmd).not.toContain('ApiBatchMaker')
    expect(filecmd).toContain('AliFile.ApiGetFile(user_id, drive_id, file_id)')
    expect(share).toContain('apiGofileCreateDirectLink')
    expect(share).toContain('暂不支持创建分享链接')
  })

  it('only exposes share settings and combined sharing where the provider supports them', () => {
    const modal = read('src/share/share/CreatNewShareLinkModal.vue')
    expect(modal).toContain("import { getDriveProviderCapabilities, resolveDriveProvider, type DriveProvider } from '../../utils/driveProvider'")
    expect(modal).toContain('supportsShareSettings')
    expect(modal).toContain('supportsCombinedShare')
    expect(modal).not.toContain('manageCreatedShares')
    expect(modal).not.toContain('ShareDAL')
  })

  it('hides and rejects upload actions outside real drive folders', () => {
    const toolbar = read('src/pan/menus/PanTopbtn.vue')
    const actions = read('src/pan/topbtns/topbtn.ts')
    expect(toolbar).toContain('v-if="isShowBtn && !dirtype.includes(\'pic\') && capabilities.upload"')
    expect(actions).toContain("directoryId.startsWith('color') || directoryId.startsWith('search')")
    expect(actions).toContain("!['favorite', 'recover', 'trash'].includes(directoryId)")
  })

  it('uses one provider registry for sidebar visibility and activation', () => {
    const sidebar = read('src/pan/PanLeft.vue')
    const treeStore = read('src/pan/pantreestore.ts')
    expect(sidebar).toContain('getDriveProviderSidebarEntries')
    expect(treeStore).toContain('isDriveProviderSidebarEntryAvailable')
    expect(treeStore).toContain('hideResourceDrive: settingStore.securityHideResourceDrive')
    expect(treeStore).not.toContain("title: '放映室'")
    expect(treeStore).not.toContain('PikPak 不支持此功能')
    expect(treeStore).not.toContain('夸克网盘不支持此功能')
  })
})
