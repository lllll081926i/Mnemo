import { readFileSync } from 'fs'
import path from 'path'
import { describe, expect, it } from 'vitest'

const read = (file: string) => readFileSync(path.resolve(process.cwd(), file), 'utf8').replace(/\r\n/g, '\n')

describe('provider dir list residual cleanup', () => {
  it('routes trash clear and folder download through multi-provider listing', () => {
    const helper = read('src/utils/providerDirList.ts')
    const topbtn = read('src/pan/topbtns/topbtn.ts')
    const aria = read('src/utils/aria2c.ts')
    const downDal = read('src/down/DownDAL.ts')
    expect(helper).toContain('listProviderDirChildren')
    expect(helper).toContain('listProviderTrashItems')
    expect(helper).toContain("provider === 'pikpak'")
    expect(helper).toContain("provider === 'gdrive'")
    expect(topbtn).toContain('listProviderTrashItems')
    expect(topbtn).not.toContain('AliTrash')
    expect(topbtn).not.toContain('topFavorDeleteAll')
    expect(aria).toContain('listProviderDirChildren')
    expect(aria).toContain('DownDAL.aAddDownload(children, dirFull, false, info.user_id)')
    expect(aria).toContain('isDriveProviderSessionUsable(token')
    expect(aria).not.toContain('AliTrash')
    expect(aria).not.toContain("message.error('连接失败")
    expect(aria).not.toContain("message.error('Aria2")
    expect(aria).not.toContain("return 'Aria2未连接")
    expect(downDal).toContain('sourceUserId || useUserStore().user_id')
    expect(downDal).toContain('`${userID}|${file.drive_id}|${file.file_id}`')
  })

  it('removes aliyun-only archive and promotion residuals', () => {
    expect(() => readFileSync(path.resolve(process.cwd(), 'src/aliapi/trash.ts'))).toThrow()
    expect(() => readFileSync(path.resolve(process.cwd(), 'src/aliapi/archive.ts'))).toThrow()
    expect(() => readFileSync(path.resolve(process.cwd(), 'src/pan/topbtns/ArchiveModal.vue'))).toThrow()
    const openFile = read('src/utils/openfile.ts')
    expect(openFile).toContain('isSupportedPreviewExtension')
    expect(openFile).toContain('暂不支持打开此格式，请下载后使用其他应用查看')
    expect(openFile).toContain('IMAGE_PREVIEW_EXTS.has(normalizedExt)')
    expect(openFile).toContain('VIDEO_PREVIEW_EXTS.has(normalizedExt)')
    expect(read('src/utils/driveProvider.ts')).not.toContain('canUseAliyunPreviewApi')
    expect(read('src/aliapi/dirfilelist.ts')).toContain('NewIAliFileResp')
    expect(read('src/aliapi/dirfilelist.ts')).not.toContain('adrive/v1.0/openFile/list')
    expect(read('src/aliapi/alihttp.ts')).not.toContain('AliUser')
    expect(read('src/onedrive/upload.ts')).toContain('const SMALL_UPLOAD_LIMIT = 4 * 1024 * 1024')
  })

  it('preserves real parent ids for provider search and trash results', () => {
    const panDal = read('src/pan/pandal.ts')
    const fileApi = read('src/aliapi/file.ts')
    expect(panDal).toContain('isSearch ? item.parentReference?.id || rootKey : dirID')
    expect(panDal).toContain('isSearch ? resolveDropboxParentIdFromPath(item.path_lower || item.path_display) : dirID')
    expect(panDal).toContain('isSearch || isTrash ? item.parents?.[0] || rootKey : dirID')
    expect(panDal).not.toContain('isSearch ? rootKey : dirID')
    expect(fileApi).toContain('const cached = TreeStore.GetDirPath(drive_id, file_id)')
    expect(fileApi).toContain('const visited = new Set<string>()')
    expect(fileApi).toContain('await AliFile.ApiFileInfo(user_id, drive_id, currentId)')
    expect(fileApi).toContain('isDriveProviderRootId({ userId: user_id, driveId: drive_id }, detail.file_id)')
  })
})
