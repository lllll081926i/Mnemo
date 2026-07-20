import { readFileSync } from 'fs'
import path from 'path'
import { describe, expect, it } from 'vitest'

const read = (file: string) => readFileSync(path.resolve(process.cwd(), file), 'utf8').replace(/\r\n/g, '\n')

describe('provider upload routing', () => {
  it('routes every queue-enabled aggregate provider to a concrete handler', () => {
    const uploader = read('src/workerpage/uploader.ts')
    for (const provider of ['guangya', 'pikpak', 'quark', '139', '189', 'onedrive', 'dropbox', 'gdrive', 'gofile']) {
      expect(uploader).toContain(`if (provider === '${provider}') return runUploadDisk`)
    }
    for (const handler of ['GuangyaUploadDisk', 'PikPakUploadDisk', 'QuarkUploadDisk', 'Cloud139UploadDisk', 'Cloud189UploadDisk', 'OneDriveUploadDisk', 'DropboxUploadDisk', 'GoogleDriveUploadDisk', 'GofileUploadDisk']) {
      expect(uploader).toContain(handler)
    }
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
    expect(filecmd).toContain("isDir: t.type === 'folder'")
    expect(filecmd).not.toContain("isDir: t.type !== 'folder'")
    expect(filecmd).toContain("isDir: info.type === 'directory'")
    expect(filecmd).toContain("if (provider !== 'aliyun')")
    expect(filecmd).toContain('AliFile.ApiGetFile(user_id, drive_id, file_id)')
    expect(share).toContain('apiGofileCreateDirectLink')
    expect(share).toContain("if (provider !== 'aliyun') return `${getDriveProviderLabel(provider)} 暂不支持创建分享链接`")
  })

  it('only exposes share settings and combined sharing where the provider supports them', () => {
    const modal = read('src/share/share/CreatNewShareLinkModal.vue')
    expect(modal).toContain('getDriveProviderCapabilities(provider)')
    expect(modal).toContain('supportsShareSettings')
    expect(modal).toContain('supportsCombinedShare')
    expect(modal).toContain('shareCapabilities.manageCreatedShares')
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
