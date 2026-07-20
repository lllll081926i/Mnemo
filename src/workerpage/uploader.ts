import { Dirent, Stats } from 'fs'
import AliFileCmd from '../aliapi/filecmd'
import { useSettingStore } from '../store'
import UserDAL from '../user/userdal'
import { IStateUploadInfo, IStateUploadTaskFile, IUploadingUI } from '../utils/dbupload'
import DebugLog from '../utils/debuglog'
import { CheckWindowsBreakPath, FileSystemErrorMessage } from '../utils/filehelper'
import { humanSize } from '../utils/format'
import { RuningList } from './uiupload'
import path from 'path'
import fspromises from 'fs/promises'
import PikPakUploadDisk from '../pikpak/upload'
import { getDriveProviderCapabilities, getDriveProviderLabel, resolveDriveProvider } from '../utils/driveProvider'
import OneDriveUploadDisk from '../onedrive/upload'
import DropboxUploadDisk from '../dropbox/upload'
import GoogleDriveUploadDisk from '../gdrive/upload'
import GofileUploadDisk from '../gofile/upload'

type UploadDiskHandler = (fileui: IUploadingUI) => Promise<string>

const failUpload = (fileui: IUploadingUI, message: string, code = 505) => {
  fileui.Info.uploadState = 'error'
  fileui.Info.failedCode = code
  fileui.Info.failedMessage = message
}

const runUploadDisk = async (fileui: IUploadingUI, handler: UploadDiskHandler): Promise<void> => {
  await checkFileSize(fileui)
  if (fileui.Info.uploadState === 'error') return
  let uploadResult = ''
  try {
    uploadResult = await handler(fileui)
  } catch (error: any) {
    if (!fileui.IsRunning) fileui.Info.uploadState = '已暂停'
    else failUpload(fileui, error?.message || '上传请求失败')
    return
  }
  if (uploadResult === 'success') {
    fileui.Info.uploadState = 'success'
  } else if (!fileui.IsRunning || uploadResult === '已暂停') {
    fileui.Info.uploadState = '已暂停'
  } else if (fileui.Info.uploadState === 'running' || fileui.Info.uploadState === 'hashing') {
    failUpload(fileui, uploadResult)
  }
}

export async function StartUpload(fileui: IUploadingUI): Promise<void> {
  const token = await UserDAL.GetUserTokenFromDB(fileui.user_id)
  if (!token || token.user_id !== fileui.user_id) {
    failUpload(fileui, '找不到账号,无法继续', 402)
    return
  }
  const provider = resolveDriveProvider({ tokenfrom: token.tokenfrom, userId: fileui.user_id, driveId: fileui.drive_id })
  const capabilities = getDriveProviderCapabilities(provider)
  if (capabilities.uploadMode !== 'queue') {
    const providerLabel = getDriveProviderLabel(provider)
    const message = capabilities.uploadMode === 'direct' ? `${providerLabel} 请从网盘页面直接上传` : `${providerLabel} 暂不支持本地上传`
    failUpload(fileui, message)
    return
  }
  // 创建文件夹
  if (fileui.File.isDir) {
    return creatDirAndReadChildren(fileui)
  }

  if (provider === 'pikpak') return runUploadDisk(fileui, PikPakUploadDisk.UploadOneFile)
  if (provider === 'onedrive') return runUploadDisk(fileui, OneDriveUploadDisk.UploadOneFile)
  if (provider === 'dropbox') return runUploadDisk(fileui, DropboxUploadDisk.UploadOneFile)
  if (provider === 'gdrive') return runUploadDisk(fileui, GoogleDriveUploadDisk.UploadOneFile)
  if (provider === 'gofile') return runUploadDisk(fileui, GofileUploadDisk.UploadOneFile)
  failUpload(fileui, `${getDriveProviderLabel(provider)} 暂不支持队列上传`)
}

interface ReadConfig {
  TaskID: number
  user_id: string
  drive_id: string
  parent_file_id: string
  localFilePath: string
  filetime: number
  encType: string
  ingoredList: string[]
}

async function creatDirAndReadChildren(fileui: IUploadingUI): Promise<void> {
  fileui.Info.uploadState = '读取中'

  let uploaded_file_id = ''
  if (fileui.File.IsRoot) {
    const data = await AliFileCmd.ApiCreatNewForder(fileui.user_id, fileui.drive_id, fileui.parent_file_id, fileui.File.name, fileui.encType)
    if (data.error) {
      fileui.Info.uploadState = 'error'
      fileui.Info.failedCode = 503
      fileui.Info.failedMessage = data.error
      return
    }
    uploaded_file_id = data.file_id
  }

  let childList: IStateUploadTaskFile[] = []
  const settingStore = useSettingStore()
  const timeStr = Date.now().toString().substring(5, 13) + '00000'
  const fileTime = parseInt(timeStr)
  const readConfig: ReadConfig = {
    TaskID: fileui.TaskID,
    user_id: fileui.user_id,
    drive_id: fileui.drive_id,
    parent_file_id: fileui.parent_file_id,
    localFilePath: fileui.localFilePath,
    filetime: fileTime,
    encType: fileui.encType,
    ingoredList: [...settingStore.downIngoredList]
  }
  const read = await readChildren(fileui.File.partPath, fileui.File.name, readConfig, fileui.Info)
  if (read.error) {
    fileui.Info.uploadState = 'error'
    fileui.Info.failedCode = 703
    fileui.Info.failedMessage = read.error
    return
  }

  childList = read.fileList
  if (read.dirList.length > 0) childList.push(...read.dirList)

  window.WinMsgToMain({
    cmd: 'MainUploadAppendFiles',
    TaskID: fileui.TaskID,
    UploadID: fileui.UploadID,
    AppendList: childList,
    CreatedDirID: uploaded_file_id
  })

  RuningList.delete(fileui.UploadID)
}

async function readChildren(parentDirPartPath: string, parentDirName: string, readConfig: ReadConfig, Info: IStateUploadInfo) {
  const localDirPath = path.join(readConfig.localFilePath, parentDirPartPath, path.sep)
  const files = await readDir(localDirPath, readConfig.ingoredList)
  if (files.error)
    return {
      error: files.error,
      fileList: [] as IStateUploadTaskFile[],
      dirList: [] as IStateUploadTaskFile[]
    }

  const addFileList: IStateUploadTaskFile[] = []
  const addDirList: IStateUploadTaskFile[] = []

  await AddFiles(addFileList, files.fileList, parentDirPartPath, parentDirName, readConfig)
  Info.failedMessage = '读取中 ' + addFileList.length + '个'

  await AddDirs(addFileList, addDirList, files.dirList, parentDirPartPath, parentDirName, readConfig, Info)

  return { error: '', fileList: addFileList, dirList: addDirList }
}

async function AddFiles(addFileList: IStateUploadTaskFile[], fileList: string[], parentDirPartPath: string, parentDirName: string, readConfig: ReadConfig): Promise<void> {
  const localDirPath = path.join(readConfig.localFilePath, parentDirPartPath, path.sep)
  let plist: Promise<void>[] = []
  for (let i = 0, maxi = fileList.length; i < maxi; i++) {
    const fileName = fileList[i]
    const filePath = localDirPath + fileName

    plist.push(
      fspromises
        .lstat(filePath)
        .then((stat: Stats) => {
          return stat
        })
        .catch((err: any) => {
          err = FileSystemErrorMessage(err.code, err.message)
          DebugLog.mSaveDanger('上传文件出错 ' + err + ' ' + filePath)
          return undefined
        })
        .then((stat: Stats | undefined) => {
          readConfig.filetime += 1
          const fileItem: IStateUploadTaskFile = {
            TaskID: readConfig.TaskID,
            UploadID: readConfig.filetime,
            partPath: path.join(parentDirPartPath, fileName),
            name: parentDirName + '/' + fileName,
            size: stat ? stat.size : 1,
            sizeStr: humanSize(stat ? stat.size : 1),
            mtime: stat ? stat.mtime.getTime() : 0,
            isDir: false,
            IsRoot: false,
            uploaded_is_rapid: false,
            uploaded_file_id: ''
          }
          addFileList.push(fileItem)
        })
    )

    if (plist.length >= 10) {
      await Promise.all(plist).catch()
      plist = []
    }
  }
  if (plist.length > 0) {
    await Promise.all(plist).catch()
  }
}

async function AddDirs(addFileList: IStateUploadTaskFile[], addDirList: IStateUploadTaskFile[], dirList: string[], parentDirPartPath: string, parentDirName: string, readConfig: ReadConfig, Info: IStateUploadInfo): Promise<void> {
  const plist: Promise<{ file_id: string; error: string }>[] = []
  for (let i = 0, maxi = dirList.length; i < maxi; i++) {
    const dirName = dirList[i]
    readConfig.filetime += 1
    const dirItem: IStateUploadTaskFile = {
      TaskID: readConfig.TaskID,
      UploadID: readConfig.filetime,
      partPath: path.join(parentDirPartPath, dirName),
      name: parentDirName + '/' + dirName,
      size: 0,
      sizeStr: humanSize(0),
      mtime: 0,
      isDir: true,
      IsRoot: false,
      uploaded_is_rapid: false,
      uploaded_file_id: ''
    }
    await readChildrenDiGui(addFileList, addDirList, dirItem, readConfig, Info, plist)
    Info.failedMessage = '读取中 ' + addFileList.length + '个'
    if (plist.length >= 10) {
      await Promise.all(plist).catch()
      plist.splice(0, plist.length)
    }
  }
  if (plist.length > 0) await Promise.all(plist).catch(() => {})
}

const MAXFILE = 20
const MAXDIR = 20

async function readChildrenDiGui(
  addFileList: IStateUploadTaskFile[],
  addDirList: IStateUploadTaskFile[],
  diritem: IStateUploadTaskFile,
  readConfig: ReadConfig,
  Info: IStateUploadInfo,
  plist: Promise<{
    file_id: string
    error: string
  }>[]
): Promise<void> {
  const localDirPath = path.join(readConfig.localFilePath, diritem.partPath, path.sep)
  const dirFiles = await readDir(localDirPath, readConfig.ingoredList)
  if (dirFiles.error) {
    plist.push(AliFileCmd.ApiCreatNewForder(readConfig.user_id, readConfig.drive_id, readConfig.parent_file_id, diritem.name, readConfig.encType))
    addDirList.push(diritem)
    return
  }
  if (dirFiles.fileList.length == 0) {
    if (dirFiles.dirList.length == 0) {
      plist.push(AliFileCmd.ApiCreatNewForder(readConfig.user_id, readConfig.drive_id, readConfig.parent_file_id, diritem.name, readConfig.encType))
      return
    } else if (dirFiles.dirList.length <= MAXDIR) {
      await AddDirs(addFileList, addDirList, dirFiles.dirList, diritem.partPath, diritem.name, readConfig, Info)
      return
    } else {
      addDirList.push(diritem)
      return
    }
  }

  if (dirFiles.fileList.length == 1) {
    if (dirFiles.dirList.length == 0) {
      await AddFiles(addFileList, dirFiles.fileList, diritem.partPath, diritem.name, readConfig)
      return
    } else if (dirFiles.dirList.length <= MAXDIR) {
      await AddFiles(addFileList, dirFiles.fileList, diritem.partPath, diritem.name, readConfig)
      await AddDirs(addFileList, addDirList, dirFiles.dirList, diritem.partPath, diritem.name, readConfig, Info)
      return
    } else {
      addDirList.push(diritem)
      return
    }
  }

  if (dirFiles.fileList.length <= MAXFILE) {
    if (dirFiles.dirList.length == 0) {
      plist.push(AliFileCmd.ApiCreatNewForder(readConfig.user_id, readConfig.drive_id, readConfig.parent_file_id, diritem.name, readConfig.encType))
      await AddFiles(addFileList, dirFiles.fileList, diritem.partPath, diritem.name, readConfig)
    } else if (dirFiles.dirList.length <= MAXDIR) {
      plist.push(AliFileCmd.ApiCreatNewForder(readConfig.user_id, readConfig.drive_id, readConfig.parent_file_id, diritem.name, readConfig.encType))
      await AddFiles(addFileList, dirFiles.fileList, diritem.partPath, diritem.name, readConfig)
      await AddDirs(addFileList, addDirList, dirFiles.dirList, diritem.partPath, diritem.name, readConfig, Info)
    } else {
      plist.push(AliFileCmd.ApiCreatNewForder(readConfig.user_id, readConfig.drive_id, readConfig.parent_file_id, diritem.name, readConfig.encType))
      addDirList.push(diritem)
    }
  } else {
    if (dirFiles.dirList.length == 0) {
      plist.push(AliFileCmd.ApiCreatNewForder(readConfig.user_id, readConfig.drive_id, readConfig.parent_file_id, diritem.name, readConfig.encType))
      addDirList.push(diritem)
    } else if (dirFiles.dirList.length <= MAXDIR) {
      addDirList.push(diritem)
    } else {
      addDirList.push(diritem)
    }
  }
}

async function readDir(
  fullDirPath: string,
  ingoredList: string[]
): Promise<{
  error: string
  fileList: string[]
  dirList: string[]
}> {
  let errorMessage = ''
  const fileList: string[] = []
  const dirList: string[] = []

  await fspromises
    .readdir(fullDirPath, { withFileTypes: true })
    .then((files: Dirent[]) => {
      for (let i = 0, maxi = files.length; i < maxi; i++) {
        const stat = files[i]
        if (stat.isSymbolicLink()) continue
        if (CheckWindowsBreakPath(stat.name)) continue
        if (stat.isDirectory()) dirList.push(stat.name)
        else if (stat.isFile()) {
          const filePathLower = stat.name.toLowerCase()
          let ingored = false
          for (let j = 0, maxj = ingoredList.length; j < maxj; j++) {
            if (filePathLower.endsWith(ingoredList[j])) {
              ingored = true
              break
            }
          }
          if (!ingored) fileList.push(stat.name)
        }
      }
    })
    .catch((err: any) => {
      err = FileSystemErrorMessage(err.code, err.message)
      DebugLog.mSaveDanger('UploadLocalDir失败：' + fullDirPath, err)
      errorMessage = err
    })

  return { error: errorMessage, fileList, dirList }
}

async function checkFileSize(fileui: IUploadingUI): Promise<void> {
  let errorMessage = ''
  const stat = await fspromises.lstat(path.join(fileui.localFilePath, fileui.File.partPath)).catch((err: any) => {
    err = FileSystemErrorMessage(err.code, err.message)
    DebugLog.mSaveDanger('StartUpload失败：' + path.join(fileui.localFilePath, fileui.File.partPath), err)
    errorMessage = err
    return undefined
  })
  if (!stat) {
    fileui.Info.uploadState = 'error'
    fileui.Info.failedCode = 102
    fileui.Info.failedMessage = errorMessage
    return
  }

  if (fileui.File.size != stat.size) {
    fileui.File.size = stat.size
    fileui.Info.up_upload_id = ''
    fileui.Info.up_file_id = ''
  }
  if (fileui.File.mtime != stat.mtime.getTime()) {
    fileui.File.mtime = stat.mtime.getTime()
    fileui.Info.up_upload_id = ''
    fileui.Info.up_file_id = ''
  }
}
