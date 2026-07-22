import { IAliGetFileModel } from '../../aliapi/alimodels'
import AliFile from '../../aliapi/file'
import AliFileCmd from '../../aliapi/filecmd'

import DebugLog from '../../utils/debuglog'
import message from '../../utils/message'
import { modalCreatNewShareLink, modalDownload, modalPassword, modalSearchPan, modalSelectPanDir, modalUpload } from '../../utils/modal'
import { ArrayKeyList } from '../../utils/utils'
import PanDAL from '../pandal'
import usePanTreeStore from '../pantreestore'
import usePanFileStore from '../panfilestore'
import { useDowningStore, useSettingStore } from '../../store'
import { Sleep } from '../../utils/format'
import TreeStore from '../../store/treestore'
import { copyToClipboard } from '../../utils/electronhelper'
import DownDAL from '../../down/DownDAL'
import { isEmpty } from 'lodash'
import { GetDriveID } from '../../aliapi/utils'
import { getEncType } from '../../utils/proxyhelper'
import { Modal, Option, Select } from '@arco-design/web-vue'
import { h } from 'vue'
import { getWebDavConnection, getWebDavConnectionId, uploadWebDavLocalPaths } from '../../utils/webdavClient'
import { getS3Connection, getS3ConnectionId, uploadS3LocalPaths } from '../../utils/s3Client'
import UserDAL from '../../user/userdal'
import { getDriveProviderCapabilities, getDriveProviderLabel, isDriveProviderRootId, resolveDriveProvider } from '../../utils/driveProvider'
import { listProviderTrashItems } from '../../utils/providerDirList'

const topbtnLock = new Set()

const getCurrentUploadContext = (parentFileId = '') => {
  const pantreeStore = usePanTreeStore()
  const panfileStore = usePanFileStore()
  const currentDirId = parentFileId || panfileStore.DirID || pantreeStore.selectDir.file_id
  const token = UserDAL.GetUserToken(pantreeStore.user_id)
  const provider = resolveDriveProvider({ tokenfrom: token?.tokenfrom, userId: pantreeStore.user_id, driveId: pantreeStore.drive_id })
  return { pantreeStore, currentDirId, provider, capabilities: getDriveProviderCapabilities(provider) }
}

const isUploadTargetDirectory = (directoryId: string) => {
  if (!directoryId) return false
  if (directoryId.startsWith('color') || directoryId.startsWith('search')) return false
  return !['favorite', 'recover', 'trash'].includes(directoryId)
}

export async function uploadLocalPaths(files: string[] | undefined, parentFileId = '', encType = ''): Promise<void> {
  if (!files?.length) return
  const { pantreeStore, currentDirId, provider, capabilities } = getCurrentUploadContext(parentFileId)
  if (!pantreeStore.user_id || !pantreeStore.drive_id || !currentDirId) {
    message.error('上传操作失败 父文件夹错误')
    return
  }
  if (!isUploadTargetDirectory(currentDirId)) {
    message.error('当前视图不能上传，请先进入网盘文件夹')
    return
  }
  if (!capabilities.upload) {
    message.error(`${getDriveProviderLabel(provider)} 暂不支持本地上传`)
    return
  }
  if (encType && !capabilities.encryption) {
    message.error(`${getDriveProviderLabel(provider)} 暂不支持加密上传`)
    return
  }
  if (provider === 'webdav') {
    const connection = getWebDavConnection(getWebDavConnectionId(pantreeStore.drive_id))
    if (!connection) {
      message.error(`${getDriveProviderLabel(provider)} 连接不存在`)
      return
    }
    try {
      await uploadWebDavLocalPaths(connection, currentDirId, files)
      message.success('上传完成')
      await PanDAL.aReLoadOneDirToRefreshTree(pantreeStore.user_id, pantreeStore.drive_id, currentDirId)
    } catch (error: any) {
      message.error(error?.message || '上传失败')
    }
    return
  }
  if (provider === 's3') {
    const connection = getS3Connection(getS3ConnectionId(pantreeStore.drive_id))
    if (!connection) {
      message.error('S3 连接不存在')
      return
    }
    try {
      await uploadS3LocalPaths(connection, currentDirId, files)
      message.success('上传完成')
      await PanDAL.aReLoadOneDirToRefreshTree(pantreeStore.user_id, pantreeStore.drive_id, currentDirId)
    } catch (error: any) {
      message.error(error?.message || '上传失败')
    }
    return
  }
  modalUpload(currentDirId, files, false, encType)
}

export function handleUpload(uploadType: string, encType: string = '') {
  const { pantreeStore, currentDirId, provider, capabilities } = getCurrentUploadContext()
  if (!pantreeStore.user_id || !pantreeStore.drive_id || !currentDirId) {
    message.error('上传操作失败 父文件夹错误')
    return
  }
  if (!isUploadTargetDirectory(currentDirId)) {
    message.error('当前视图不能上传，请先进入网盘文件夹')
    return
  }
  if (!capabilities.upload) {
    message.error(`${getDriveProviderLabel(provider)} 暂不支持本地上传`)
    return
  }
  if (encType && !capabilities.encryption) {
    message.error(`${getDriveProviderLabel(provider)} 暂不支持加密上传`)
    return
  }
  if (encType == 'mnemoEncrypt1') {
    if (!useSettingStore().securityPassword) {
      modalPassword('new', (success) => {
        success && handleUpload(uploadType, encType)
      })
      return
    }
  }
  if (uploadType == 'file') {
    window.WebShowOpenDialogSync(
      {
        title: '选择多个文件上传到网盘',
        buttonLabel: `${encType == 'mnemoEncrypt1' ? '加密' : encType == 'mnemoEncrypt2' ? '私密' : ''}上传选中的文件`,
        properties: ['openFile', 'multiSelections', 'showHiddenFiles', 'noResolveAliases', 'treatPackageAsDirectory', 'dontAddToRecent']
      },
      (files: string[] | undefined) => {
        void uploadLocalPaths(files, currentDirId, encType)
      }
    )
  } else if (uploadType == 'folder') {
    window.WebShowOpenDialogSync(
      {
        title: '选择多个文件夹上传到网盘',
        buttonLabel: `${encType == 'mnemoEncrypt1' ? '加密' : encType == 'mnemoEncrypt2' ? '私密' : ''}上传文件夹`,
        properties: ['openDirectory', 'multiSelections', 'showHiddenFiles', 'noResolveAliases', 'treatPackageAsDirectory', 'dontAddToRecent']
      },
      (files: string[] | undefined) => {
        void uploadLocalPaths(files, currentDirId, encType)
      }
    )
  }
}

export function menuDownload(istree: boolean, tips: boolean = true) {
  const selectedData = PanDAL.GetPanSelectedData(istree)
  if (selectedData.isError) {
    message.error('下载操作失败 父文件夹错误')
    return
  }
  const capabilities = getDriveProviderCapabilities({ userId: selectedData.user_id, driveId: selectedData.drive_id })
  if (!capabilities.download) {
    message.error(`${getDriveProviderLabel(capabilities.provider)} 不支持下载`)
    return
  }
  if (selectedData.isErrorSelected) {
    message.error('没有可以下载的文件')
    return
  }
  const settingStore = useSettingStore()
  const panTreeStore = usePanTreeStore()
  const savePath = settingStore.AriaIsLocal ? settingStore.downSavePath : settingStore.ariaSavePath
  const savePathFull = settingStore.downSavePathFull
  const downSavePathDefault = settingStore.downSavePathDefault
  if (isEmpty(savePath)) {
    message.error('未设置保存路径')
    modalDownload(istree)
    return
  }
  if (topbtnLock.has('menuDownload')) return
  topbtnLock.add('menuDownload')
  let files: IAliGetFileModel[] = []
  if (istree) {
    files = [
      {
        ...panTreeStore.selectDir,
        isDir: true,
        ext: '',
        mime_extension: '',
        mime_type: '',
        category: '',
        icon: '',
        sizeStr: '',
        timeStr: '',
        starred: false,
        thumbnail: ''
      }
    ]
  } else {
    files = usePanFileStore().GetSelected()
  }
  try {
    if (downSavePathDefault || !tips) {
      DownDAL.aAddDownload(files, savePath, savePathFull)
      if (useDowningStore().ListDataRaw.length > 0) {
        message.success(`成功创建下载任务`)
      }
    } else {
      modalDownload(istree)
    }
  } catch (err: any) {
    message.error(err.message)
    DebugLog.mSaveDanger('menuDownload', err)
  }
  topbtnLock.delete('menuDownload')
}

export async function menuFavSelectFile(istree: boolean, isFavor: boolean) {
  const selectedData = PanDAL.GetPanSelectedData(istree)
  if (selectedData.isError) {
    message.error('收藏操作失败 父文件夹错误')
    return
  }
  if (selectedData.isErrorSelected) {
    message.error('没有可以收藏的文件')
    return
  }

  if (topbtnLock.has('menuFavSelectFile')) return
  topbtnLock.add('menuFavSelectFile')
  try {
    const successList = await AliFileCmd.ApiFavorBatch(selectedData.user_id, selectedData.drive_id, isFavor, true, selectedData.selectedKeys)
    if (isFavor) {
      if (usePanTreeStore().selectDir.file_id == 'favorite') {
        PanDAL.aReLoadOneDirToShow('', 'refresh', false)
      } else {
        usePanFileStore().mFavorFiles(isFavor, successList)
      }
    } else {
      if (usePanTreeStore().selectDir.file_id == 'favorite') {
        usePanFileStore().mDeleteFiles('favorite', successList, false)
      } else {
        usePanFileStore().mFavorFiles(isFavor, successList)
      }
    }
  } catch (err: any) {
    message.error(err.message)
    DebugLog.mSaveDanger('menuFavSelectFile', err)
  }
  topbtnLock.delete('menuFavSelectFile')
}

export async function menuTrashSelectFile(istree: boolean, isDelete: boolean, ispic: boolean = false) {
  const selectedData = PanDAL.GetPanSelectedData(istree)
  if (selectedData.isError) {
    message.error('删除操作失败 父文件夹错误')
    return
  }
  if (selectedData.isErrorSelected) {
    message.error('没有可以删除的文件')
    return
  }
  if (selectedData.dirID.startsWith('video')) {
    message.error('请不要在放映室里删除文件')
    return
  }
  const capabilities = getDriveProviderCapabilities({ userId: selectedData.user_id, driveId: selectedData.drive_id })
  const canPermanentlyDelete = capabilities.permanentDelete || (selectedData.dirID === 'trash' && capabilities.trashPurge)
  if ((isDelete && !canPermanentlyDelete) || (!isDelete && !capabilities.recycleBin)) {
    message.error(`${getDriveProviderLabel(capabilities.provider)} 不支持${isDelete ? '彻底删除' : '放入回收站'}`)
    return
  }
  if (topbtnLock.has('menuTrashSelectFile')) return
  topbtnLock.add('menuTrashSelectFile')
  try {
    let successList: string[] = []
    if (isDelete) {
      successList = await AliFileCmd.ApiDeleteBatch(selectedData.user_id, selectedData.drive_id, selectedData.selectedKeys)
    } else {
      successList = await AliFileCmd.ApiTrashBatch(selectedData.user_id, selectedData.drive_id, selectedData.selectedKeys)
    }

    if (istree) {
      await PanDAL.aReLoadOneDirToShow(selectedData.drive_id, selectedData.parentDirID, false)
    } else {
      usePanFileStore().mDeleteFiles(selectedData.dirID, successList, selectedData.dirID !== 'trash')
      if (selectedData.dirID !== 'trash') {
        await PanDAL.aReLoadOneDirToRefreshTree(selectedData.user_id, selectedData.drive_id, selectedData.dirID, selectedData.albumId)
        TreeStore.ClearDirSize(selectedData.drive_id, selectedData.selectedParentKeys)
      }
    }
  } catch (err: any) {
    message.error(err.message)
    DebugLog.mSaveDanger('menuTrashSelectFile', err)
  }
  topbtnLock.delete('menuTrashSelectFile')
}

export async function topRestoreSelectedFile() {
  const selectedData = PanDAL.GetPanSelectedData(false)
  if (selectedData.isError) {
    message.error('还原文件操作失败 父文件夹错误')
    return
  }
  if (selectedData.isErrorSelected) {
    message.error('没有可以还原的文件')
    return
  }

  const panfileStore = usePanFileStore()
  const diridList: string[] = []
  panfileStore
    .GetSelected()
    .filter((t) => t.isDir)
    .map((t) => diridList.push(t.file_id))

  if (topbtnLock.has('topRestoreSelectedFile')) return
  topbtnLock.add('topRestoreSelectedFile')
  try {
    await AliFileCmd.ApiTrashRestoreBatch(selectedData.user_id, selectedData.drive_id, true, selectedData.selectedKeys)
    if (usePanTreeStore().selectDir.file_id == 'trash') {
      usePanFileStore().mDeleteFiles('trash', selectedData.selectedKeys, false)
    } else {
      PanDAL.aReLoadOneDirToShow('', 'refresh', false)
    }
    await Sleep(2000)
    const dirList = await AliFileCmd.ApiGetFileBatch(selectedData.user_id, selectedData.drive_id, diridList)

    const pset = new Set<string>()
    for (let i = 0, maxi = dirList.length; i < maxi; i++) {
      let parent_file_id = dirList[i].parent_file_id
      if (isDriveProviderRootId({ userId: selectedData.user_id, driveId: selectedData.drive_id }, parent_file_id)) parent_file_id = 'root'
      if (pset.has(parent_file_id)) continue
      pset.add(parent_file_id)
      await PanDAL.aReLoadOneDirToRefreshTree(selectedData.user_id, selectedData.drive_id, parent_file_id)
    }
    TreeStore.ClearDirSize(selectedData.drive_id, Array.from(pset))
  } catch (err: any) {
    message.error(err.message)
    DebugLog.mSaveDanger('topRestoreSelectedFile', err)
  }
  topbtnLock.delete('topRestoreSelectedFile')
}

export function menuCopySelectedFile(istree: boolean, copyby: string) {
  const selectedData = PanDAL.GetPanSelectedData(istree)
  if (selectedData.isError) {
    message.error('复制移动操作失败 父文件夹错误')
    return
  }
  if (selectedData.dirID.startsWith('video')) {
    message.error('请不要在放映室里移动文件文件')
    return
  }
  const capabilities = getDriveProviderCapabilities({ userId: selectedData.user_id, driveId: selectedData.drive_id })
  const isCopy = copyby === 'copy'
  if ((isCopy && !capabilities.copy) || (!isCopy && !capabilities.move)) {
    message.error(`${getDriveProviderLabel(capabilities.provider)} 不支持${isCopy ? '复制' : '移动'}`)
    return
  }

  let files: IAliGetFileModel[] = []
  if (istree) {
    files = [
      {
        ...usePanTreeStore().selectDir,
        isDir: true,
        ext: '',
        category: '',
        icon: '',
        sizeStr: '',
        timeStr: '',
        starred: false,
        thumbnail: ''
      } as IAliGetFileModel
    ]
  } else {
    files = usePanFileStore().GetSelected()
  }
  if (files.length == 0) {
    message.error('没有选择要复制移动的文件！')
    return
  }
  const parent_file_id = files[0].parent_file_id

  const file_idList: string[] = []
  const diridList: string[] = []
  for (let i = 0, maxi = files.length; i < maxi; i++) {
    if (files[i].isDir && !diridList.includes(files[i].parent_file_id)) diridList.push(files[i].parent_file_id)
    file_idList.push(files[i].file_id)
  }

  if (file_idList.length == 0) {
    message.error('没有可以复制移动的文件')
    return
  }
  modalSelectPanDir(copyby, parent_file_id, async function (user_id: string, drive_id: string, selectFile: any) {
    if (!drive_id || !selectFile.drive_id || !selectFile.file_id) return
    if (parent_file_id == selectFile.file_id) {
      message.error('不能移动复制到原位置！')
      return
    }
    let successList: string[]
    if (copyby == 'copy') {
      successList = await AliFileCmd.ApiCopyBatch(user_id, drive_id, file_idList, selectFile.drive_id, selectFile.path || selectFile.file_id, selectFile.description || '')
      await PanDAL.aReLoadOneDirToRefreshTree(selectedData.user_id, selectFile.drive_id, selectFile.file_id)
      TreeStore.ClearDirSize(selectedData.drive_id, [selectFile.file_id])
    } else {
      successList = await AliFileCmd.ApiMoveBatch(user_id, drive_id, file_idList, selectFile.drive_id, selectFile.path || selectFile.file_id, selectFile.description || '')
      if (istree) {
        await PanDAL.aReLoadOneDirToShow(selectedData.drive_id, selectedData.parentDirID, false)
      } else {
        usePanFileStore().mDeleteFiles(selectedData.dirID, successList, true)
      }
      await PanDAL.aReLoadOneDirToRefreshTree(selectedData.user_id, selectFile.drive_id, selectFile.file_id)
      TreeStore.ClearDirSize(drive_id, [selectFile.file_id, ...selectedData.selectedParentKeys])
    }
  })
}

export function dropMoveSelectedFile(drive_id: string, movetodirid: string, istree: boolean) {
  const selectedData = PanDAL.GetPanSelectedData(istree)
  if (selectedData.isErrorSelected) return
  if (selectedData.isError) {
    message.error('复制移动操作失败 父文件夹错误！')
    return
  }
  const capabilities = getDriveProviderCapabilities({ userId: selectedData.user_id, driveId: selectedData.drive_id })
  if (!capabilities.move) {
    message.error(`${getDriveProviderLabel(capabilities.provider)} 不支持移动`)
    return
  }

  if (selectedData.dirID == 'trash') {
    message.error('回收站内文件不支持移动！')
    return
  }
  if (!movetodirid) {
    message.error('没有选择要移动到的位置！')
    return
  }
  if (movetodirid == selectedData.dirID) {
    message.error('不能移动到原位置！')
    return
  }

  const file_idList: string[] = []
  const filenameList: string[] = []
  const selectedFile = usePanFileStore().GetSelected()
  if (selectedFile.length == 0) {
    message.error('没有选择要拖放移动的文件！')
    return
  }
  for (let i = 0, maxi = selectedFile.length; i < maxi; i++) {
    file_idList.push(selectedFile[i].file_id)
    filenameList.push(selectedFile[i].name)
  }

  if (file_idList.includes(movetodirid)) {
    if (file_idList.length == 1) message.info('取消移动')
    else message.error('不能移动到原位置！')
    return
  }

  let to_drive_id = drive_id || selectedData.drive_id
  // 获取父节点
  if (isDriveProviderRootId({ userId: selectedData.user_id, driveId: drive_id }, movetodirid)) {
    to_drive_id = GetDriveID(selectedData.user_id, movetodirid)
    movetodirid = 'root'
  }
  AliFileCmd.ApiMoveBatch(selectedData.user_id, selectedData.drive_id, file_idList, to_drive_id, movetodirid).then(async (success: string[]) => {
    usePanFileStore().mDeleteFiles(selectedData.dirID, success, true)
    await PanDAL.aReLoadOneDirToRefreshTree(selectedData.user_id, selectedData.drive_id, selectedData.dirID)
    if (selectedData.drive_id != to_drive_id) {
      await PanDAL.aReLoadOneDirToRefreshTree(selectedData.user_id, to_drive_id, movetodirid)
    }
    await PanDAL.aReLoadOneDirToShow(to_drive_id, movetodirid, false)
    TreeStore.ClearDirSize(selectedData.drive_id, [movetodirid, ...selectedData.selectedParentKeys])
  })
}

export async function menuFileEncTypeChange(istree: boolean) {
  const selectedData = PanDAL.GetPanSelectedData(istree)
  const description = selectedData.fileDescription || selectedData.parentDirDescription || ''
  if (selectedData.isError) {
    message.error('标记加密文件操作失败 父文件夹错误')
    return
  }
  if (selectedData.isErrorSelected) {
    message.error('没有可以标记加密的文件')
    return
  }
  if (topbtnLock.has('menuFileEncTypeChange')) return
  topbtnLock.add('menuFileEncTypeChange')
  let encType = 'mnemoEncrypt1'
  Modal.open({
    title: '标记加密',
    okText: '标记',
    bodyStyle: { minWidth: '340px' },
    content: () =>
      h(
        Select,
        {
          tabindex: '-1',
          defaultValue: 'mnemoEncrypt1',
          onChange: (value: any) => (encType = value)
        },
        () => [h(Option, { tabindex: '-1', value: 'notEncrypt', label: '未加密' }), h(Option, { tabindex: '-1', value: 'mnemoEncrypt1', label: '加密文件' }), h(Option, { tabindex: '-1', value: 'mnemoEncrypt2', label: '私密文件' })]
      ),
    onOk: async () => {
      try {
        await AliFileCmd.ApiFileColorBatch(selectedData.user_id, selectedData.drive_id, description, encType, selectedData.selectedKeys)
      } catch (err: any) {
        message.error(err.message)
        DebugLog.mSaveDanger('menuFileEncTypeChange', err)
      }
    },
    onCancel: () => {
      topbtnLock.delete('menuFileEncTypeChange')
    }
  })
  topbtnLock.delete('menuFileEncTypeChange')
}

export async function menuFileClearHistory(istree: boolean) {
  const selectedData = PanDAL.GetPanSelectedData(istree)
  if (selectedData.isError) {
    message.error('清除历史操作失败 父文件夹错误')
    return
  }
  if (selectedData.isErrorSelected) {
    message.error('没有可以清除历史的文件')
    return
  }
  if (topbtnLock.has('menuFileClearHistory')) return
  topbtnLock.add('menuFileClearHistory')
  await AliFileCmd.ApiFileHistoryBatch(selectedData.user_id, selectedData.drive_id, selectedData.selectedKeys)
  await PanDAL.aReLoadOneDirToShow('', 'refresh', false)
  usePanFileStore().mCancelSelect()
  topbtnLock.delete('menuFileClearHistory')
}

export async function menuFileColorChange(istree: boolean, color: string) {
  const selectedData = PanDAL.GetPanSelectedData(istree)
  const description = selectedData.fileDescription || selectedData.parentDirDescription
  color = color.toLowerCase().replace('#', 'c')
  if (selectedData.isError) {
    message.error('标记文件操作失败 父文件夹错误')
    return
  }
  if (selectedData.isErrorSelected) {
    message.error('没有可以标记的文件')
    return
  }
  if (color && description.includes(color)) {
    message.error('不能标记相同的颜色')
    return
  }
  if (topbtnLock.has('menuFileColorChange')) return
  topbtnLock.add('menuFileColorChange')
  try {
    await AliFileCmd.ApiFileColorBatch(selectedData.user_id, selectedData.drive_id, description, color, selectedData.selectedKeys)
  } catch (err: any) {
    message.error(err.message)
    DebugLog.mSaveDanger('menuFileColorChange', err)
  }
  topbtnLock.delete('menuFileColorChange')
}

export function menuCreatShare(istree: boolean, shareby: string, driveType: string) {
  const selectedData = PanDAL.GetPanSelectedData(istree)
  if (selectedData.isError) {
    message.error('创建分享操作失败 父文件夹错误')
    return
  }
  const capabilities = getDriveProviderCapabilities({ userId: selectedData.user_id, driveId: selectedData.drive_id })
  if (!capabilities.createShare) {
    message.error(`${getDriveProviderLabel(capabilities.provider)} 不支持创建分享链接`)
    return
  }

  let list: IAliGetFileModel[] = []
  if (istree) {
    const dir = usePanTreeStore().selectDir
    list = [
      {
        __v_skip: true,
        drive_id: dir.drive_id,
        file_id: dir.file_id,
        parent_file_id: dir.parent_file_id,
        name: dir.name,
        namesearch: dir.namesearch,
        ext: '',
        mime_type: '',
        mime_extension: '',
        category: '',
        icon: 'iconfile-folder',
        size: 0,
        sizeStr: '',
        time: 0,
        timeStr: '',
        starred: false,
        isDir: true,
        thumbnail: '',
        description: dir.description
      }
    ]
  } else {
    list = usePanFileStore().GetSelected()
  }
  if (list.length == 0) {
    message.error('没有可以分享的文件！')
    return
  }
  let encFiles = list.filter((l) => getEncType(l) == 'mnemoEncrypt2')
  if (encFiles.length > 0) {
    Modal.open({
      title: '存在私密的文件，是否继续分享？',
      bodyStyle: {
        minWidth: '340px',
        minHeight: '100px'
      },
      closable: false,
      content: encFiles.map((v) => v.name).join(','),
      okText: '确认',
      cancelText: '取消',
      onOk(e) {
        modalCreatNewShareLink(shareby, driveType, list)
      }
    })
  } else {
    modalCreatNewShareLink(shareby, driveType, list)
  }
}

export async function topTrashDeleteAll() {
  const selectedData = PanDAL.GetPanSelectedData(false)
  if (selectedData.isError) {
    message.error('清空回收站操作失败 父文件夹错误')
    return
  }
  const capabilities = getDriveProviderCapabilities({ userId: selectedData.user_id, driveId: selectedData.drive_id })
  if (!capabilities.trashPurge) {
    message.error(`${getDriveProviderLabel(capabilities.provider)} 不支持清空回收站`)
    return
  }

  if (topbtnLock.has('topTrashDeleteAll')) return
  topbtnLock.add('topTrashDeleteAll')
  const loadingKey = 'cleartrash_' + Date.now().toString()
  try {
    message.loading('清空回收站执行中...', 60, loadingKey)
    let count = 0
    let failed = false
    while (true) {
      const items = await listProviderTrashItems(selectedData.user_id, selectedData.drive_id)
      if (items.length === 0) break
      const selectkeys = ArrayKeyList<string>('file_id', items)
      const successList = await AliFileCmd.ApiTrashCleanBatch(selectedData.user_id, selectedData.drive_id, false, selectkeys)
      if (successList.length === 0) {
        message.error('清空回收站失败，请稍后重试', 3, loadingKey)
        failed = true
        break
      }
      count += successList.length
      message.loading('清空回收站执行中...(' + count.toString() + ')', 0, loadingKey)
    }
    if (!failed) message.success(count > 0 ? '清空回收站 成功!' : '回收站已为空', 3, loadingKey)
    if (usePanTreeStore().selectDir.file_id == 'trash') PanDAL.aReLoadOneDirToShow('', 'refresh', false)
  } catch (err: any) {
    message.error(err.message, 3, loadingKey)
    DebugLog.mSaveDanger('topTrashDeleteAll', err)
  }
  topbtnLock.delete('topTrashDeleteAll')
}

export async function topSearchAll(word: string, inputsearchType: string[]) {
  if (!word) return
  if (word == 'topSearchAll高级搜索') {
    // Advanced search UI was Aliyun-only; retained providers use keyword search.
    message.info('当前网盘不支持高级搜索，请直接输入关键字搜索')
    return
  }
  const pantreeStore = usePanTreeStore()
  if (!pantreeStore.user_id || !pantreeStore.selectDir.file_id) {
    message.error('搜索失败 父文件夹错误')
    return
  }
  const capabilities = getDriveProviderCapabilities({ userId: pantreeStore.user_id, driveId: pantreeStore.drive_id })
  if (!capabilities.search) {
    message.error(`${getDriveProviderLabel(capabilities.provider)} 不支持全盘搜索`)
    return
  }
  const keyword = word.trim()
  if (!keyword) {
    message.error('搜索失败 搜索关键字不能为空')
    return
  }
  await PanDAL.aReLoadOneDirToShow('', 'search' + keyword, false)
}

export async function menuJumpToDir() {
  let panTreeStore = usePanTreeStore()
  let first = usePanFileStore().GetSelectedFirst()
  if (first && !first.parent_file_id) {
    first = await AliFile.ApiGetFile(panTreeStore.user_id, first.drive_id, first.file_id)
  }
  if (!first) {
    message.error('没有选中任何文件')
    return
  }
  PanDAL.aReLoadOneDirToShow(first.drive_id, first.parent_file_id, true).then(() => {
    usePanFileStore().mKeyboardSelect(first!.file_id, false, false)
    usePanFileStore().mSaveFileScrollTo(first!.file_id)
  })
}

export function menuCopyFileName() {
  const list: IAliGetFileModel[] = usePanFileStore().GetSelected()
  if (list.length == 0) {
    message.error('没有选择要复制文件名的文件！')
    return
  }

  if (topbtnLock.has('menuCopyFileName')) return
  topbtnLock.add('menuCopyFileName')
  try {
    const nameList: string[] = []
    for (let i = 0, maxi = list.length; i < maxi; i++) {
      nameList.push(list[i].name)
    }
    const fullStr = nameList.join('\r\n')
    copyToClipboard(fullStr)
    message.success('选中文件的文件名已复制到剪切板')
  } catch (err: any) {
    message.error(err.message)
    DebugLog.mSaveDanger('menuCopyFileName', err)
  }
  topbtnLock.delete('menuCopyFileName')
}
