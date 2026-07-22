import { IAliGetFileModel } from '../aliapi/alimodels'
import { useModalStore } from '../store'
import { IRawUrl } from './proxyhelper'

export function modalCloseAll() {
  useModalStore().showModal('', {})
}

export function modalCreatNewDir(dirtype: string, encType: string = '', parentdirid: string = '', callback: any = undefined) {
  useModalStore().showModal('creatdir', { dirtype, encType, parentdirid, callback })
}

export function modalCreatNewShareLink(sharetype: string, driveType: string, filelist: IAliGetFileModel[]) {
  useModalStore().showModal('creatshare', { sharetype, driveType, filelist })
}

export function modalRename(istree: boolean, ismulti: boolean, ispic: boolean) {
  useModalStore().showModal(ismulti ? 'renamemulti' : 'rename', { istree, ispic })
}

export function modalSelectPanDir(selecttype: string, selectid: string,
                                  callback: (user_id: string, drive_id: string, selectFile: any) => void,
                                  category?: string,
                                  extFilter?: RegExp) {
  useModalStore().showModal('selectpandir', { selecttype, selectid, category, extFilter, callback })
}

export function modalSelectVideoQuality(fileInfo: IAliGetFileModel, qualityData: IRawUrl, callback: (quality: string) => void) {
  useModalStore().showModal('selectvideoquality', { fileInfo, qualityData, callback })
}

export function modalShuXing(istree: boolean, ispic: boolean = false) {
  useModalStore().showModal('shuxing', { istree, ispic })
}

export function modalSearchPan(inputsearchType: string[]) {
  useModalStore().showModal('searchpan', { inputsearchType })
}

export function modalM3U8Download() {
  useModalStore().showModal('m3u8download', {})
}

export function modalUpload(file_id: string, filelist: string[], ispic: boolean = false, encType: string = '') {
  useModalStore().showModal('upload', { file_id, filelist, ispic, encType })
}

export function modalDownload(istree: boolean) {
  useModalStore().showModal('download', { istree })
}

export function modalCloudOfflineDownload(offlineForm?: { dirId?: string; dirName?: string }) {
  useModalStore().showModal('cloudoffline', offlineForm ? { offlineForm } : {})
}

export function modalShowPost(msg: string, msgid: string) {
  useModalStore().showModal('showpostmodal', { msg, msgid })
}

export function modalPassword(optType: string, callback?: (success: boolean, inputpassword: string) => void) {
  useModalStore().showModal('showpassword', { optType, callback })
}
