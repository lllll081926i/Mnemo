<script setup lang='ts'>
import { IAliFileItem, IAliGetForderSizeModel } from '../../aliapi/alimodels'
import AliFile from '../../aliapi/file'
import { useFootStore, usePanFileStore, usePanTreeStore } from '../../store'
import { copyToClipboard } from '../../utils/electronhelper'
import message from '../../utils/message'
import { modalCloseAll } from '../../utils/modal'
import { humanDateTimeDateStr, humanSize, humanTime } from '../../utils/format'
import { Braces, Copy, Download, FileText, Folder, Link2, RefreshCw } from 'lucide-vue-next'
import { computed, ref, watch } from 'vue'
import DebugLog from '../../utils/debuglog'
import { GetDriveID, isPikPakUser } from '../../aliapi/utils'
import { getEncType, getProxyUrl, getRawUrl, isLocalProxyUrl } from '../../utils/proxyhelper'
import { folderStatsVersion, getCachedFolderStats, requestFolderStats } from '../../utils/folderstats'
import TreeStore from '../../store/treestore'
import { apiPikPakFileDetail, mapPikPakFileToAliModel } from '../../pikpak/dirfilelist'

const props = defineProps({
  visible: {
    type: Boolean,
    required: true
  },
  istree: {
    type: Boolean,
    required: true
  },
  ispic: {
    type: Boolean,
    required: true
  }
})

const loading = ref(false)
const formateSize = ref(true)
const fileInfo = ref<IAliFileItem>()
const dirInfo = ref<IAliGetForderSizeModel>()
const dirPath = ref('')

const fileTypeLabel = computed(() => fileInfo.value?.type === 'folder' ? '文件夹' : '文件')
const displaySize = computed(() => {
  const size = fileInfo.value?.size || dirInfo.value?.size || 0
  return formateSize.value ? humanSize(size) : `${size} 字节`
})

const folderStatsText = computed(() => {
  if (!dirInfo.value) return '正在统计子文件…'
  return `子文件大小：${humanSize(dirInfo.value.size)}，子文件：${dirInfo.value.file_count} 个，子文件夹：${dirInfo.value.folder_count} 个${dirInfo.value.reach_limit ? '（已达统计上限）' : ''}`
})

const propertyText = (value: unknown) => {
  if (value === undefined || value === null || value === '') return '—'
  return String(value)
}

// 后台统计完成时刷新当前文件夹的结果
watch(folderStatsVersion, () => {
  const pantreeStore = usePanTreeStore()
  const fileId = fileInfo.value?.file_id
  if (!fileId) return
  const cached = getCachedFolderStats(pantreeStore.user_id, fileInfo.value?.drive_id || pantreeStore.drive_id, fileId)
  if (cached) dirInfo.value = cached
})
const handleOpen = async () => {
  loading.value = true
  fileInfo.value = undefined
  dirInfo.value = undefined
  dirPath.value = ''
  const pantreeStore = usePanTreeStore()
  let file_id = ''
  let drive_id = ''
  if (props.istree) {
    file_id = pantreeStore.selectDir.file_id
    drive_id = pantreeStore.selectDir.drive_id
  } else {
    const panfileStore = usePanFileStore()
    let fileList = panfileStore.GetSelected()
    if (fileList.length == 0) {
      const focus = panfileStore.mGetFocus()
      panfileStore.mKeyboardSelect(focus, false, false)
      fileList = panfileStore.GetSelected()
    }
    if (fileList.length > 0) {
      file_id = fileList[0].file_id
      drive_id = fileList[0].drive_id
    }
  }
  if (props.ispic) {
    drive_id = GetDriveID(pantreeStore.user_id, 'pic')
  }
  if (!file_id) {
    message.error('没有选中任何文件')
    loading.value = false
    return
  }
  try {
    const isPikPak = isPikPakUser(pantreeStore.user_id) || drive_id === 'pikpak'
    if (isPikPak) {
      const pathList = TreeStore.GetDirPath(drive_id, file_id)
      const pathNames = pathList.map((item) => item.name).filter((name) => name)
      dirPath.value = '/' + pathNames.join('/')
      const detail = await apiPikPakFileDetail(pantreeStore.user_id, file_id)
      if (detail) {
        const mapped: any = mapPikPakFileToAliModel(detail, drive_id, detail.parent_id || 'pikpak_root')
        mapped.type = mapped.isDir ? 'folder' : 'file'
        mapped.created_at = detail.created_time || ''
        mapped.updated_at = detail.modified_time || detail.created_time || ''
        fileInfo.value = mapped
      }
    } else {
      let path_file_id = props.ispic ? 'pic_root' : file_id
      let fileName = pantreeStore.selectDir.name
      AliFile.ApiFileGetPathString(pantreeStore.user_id, drive_id, path_file_id, '/').then((data) => {
        dirPath.value = '/' + data + (props.ispic ? '/' + fileName : '')
      })
      fileInfo.value = await AliFile.ApiFileInfo(pantreeStore.user_id, drive_id, file_id, props.ispic)
    }
    if (fileInfo.value && ['audio', 'video'].includes(fileInfo.value.category)) {
      const encType = getEncType(fileInfo.value)
      const category = fileInfo.value.category
      const rawUrl = await getRawUrl(
        pantreeStore.user_id, drive_id, file_id,
        encType, '',
        category === 'audio', category
      )
      if (typeof rawUrl == 'string') {
        message.error(rawUrl)
      } else if (rawUrl && rawUrl.url) {
        fileInfo.value.thumbnail = isLocalProxyUrl(rawUrl.url) ? rawUrl.url : getProxyUrl({
          user_id: pantreeStore.user_id,
          drive_id,
          file_id,
          file_size: rawUrl.size || fileInfo.value.size,
          proxy_url: rawUrl.url,
          proxy_headers: rawUrl.headers ? JSON.stringify(rawUrl.headers) : undefined,
          proxy_kind: 'audio'
        })
      }
    }
    if (fileInfo.value?.type == 'folder') {
      // 先展示缓存，后台静默缓慢统计（签名一致时直接复用缓存）
      dirInfo.value = getCachedFolderStats(pantreeStore.user_id, drive_id, file_id)
      void requestFolderStats(pantreeStore.user_id, drive_id, file_id)
    }
  } finally {
    loading.value = false
  }
}

const handleClose = () => {

  if (loading.value) loading.value = false
  dirInfo.value = { size: 0, folder_count: 0, file_count: 0, reach_limit: undefined }
  fileInfo.value = undefined
  dirPath.value = ''
}

const makeFenBianLv = (width: number | undefined, height: number | undefined) => {
  if (!width) width = 0
  if (!height) height = 0
  if (width == 0 || height == 0) return ''
  return width + ' x ' + height
}

const makeImageSheBei = (exif: string | undefined) => {
  if (!exif) return ''
  try {
    let msg = ''
    const exobj = JSON.parse(exif)
    if (exobj.Make && exobj.Make.value) msg += exobj.Make.value + ' '
    if (exobj.Model && exobj.Model.value) msg += exobj.Model.value + ' '
    return msg
  } catch (err: any) {
    DebugLog.mSaveWarning(exif, err)
  }
  return ''
}

const makeImageShiJian = (exif: string | undefined) => {
  if (!exif) return ''
  try {
    const exobj = JSON.parse(exif)
    if (exobj.DateTimeOriginal && exobj.DateTimeOriginal.value) return exobj.DateTimeOriginal.value
    if (exobj.DateTimeDigitized && exobj.DateTimeDigitized.value) return exobj.DateTimeDigitized.value
    if (exobj.DateTime && exobj.DateTime.value) return exobj.DateTime.value
  } catch (err: any) {
    DebugLog.mSaveWarning(exif, err)
  }
  return ''
}

const handleAudioPlay = () => {
  useFootStore().mSaveAudioUrl('')
}

const handleSize = () => {
  formateSize.value = !formateSize.value
}

const handleHide = () => {
  modalCloseAll()
}

const handleCopyFileName = () => {
  if (fileInfo.value?.name) {
    copyToClipboard(fileInfo.value?.name)
    message.success('文件名已复制到剪切板')
  }
}
const handleCopyPath = () => {
  if (dirPath.value) {
    copyToClipboard(dirPath.value)
    message.success('路径已复制到剪切板')
  }
}
const handleCopyJson = () => {
  if (fileInfo.value) {
    copyToClipboard(JSON.stringify(fileInfo.value))
    message.success('文件信息已复制到剪切板')
  }
}
const handleCopyDownload = () => {
  const pantreeStore = usePanTreeStore()
  if (fileInfo.value) {
    const selectedDriveId = fileInfo.value.drive_id || pantreeStore.drive_id
    getRawUrl(pantreeStore.user_id, selectedDriveId, fileInfo.value.file_id || '', getEncType(fileInfo.value)).then(data => {
      if (data && typeof data !== 'string' && data.url) {
        const copyUrl = data.headers && Object.keys(data.headers).length > 0
          ? getProxyUrl({
              user_id: pantreeStore.user_id,
              drive_id: selectedDriveId,
              file_id: fileInfo.value?.file_id || '',
              file_size: data.size || fileInfo.value?.size,
              proxy_url: data.url,
              proxy_headers: data.headers ? JSON.stringify(data.headers) : undefined
            })
          : data.url
        copyToClipboard(copyUrl)
        message.success('下载链接已复制到剪切板，4小时内有效')
      } else {
        message.error('下载链接获取失败，请稍后重试')
      }
    })
  } else {
    message.error('下载链接获取失败，请稍后重试')
  }
}
const handleCopyThumbnail = () => {
  if (fileInfo.value?.thumbnail) {
    copyToClipboard(fileInfo.value?.thumbnail)
    message.success('预览链接已复制到剪切板')
  }
}

</script>

<template>
  <a-modal :visible='visible' modal-class='modalclass shuxingmodal property-modal' :footer='false' :unmount-on-close='true'
           :mask-closable='false' @cancel='handleHide' @before-open='handleOpen' @close='handleClose'>
    <template #title>
      <span class='modaltitle'>查看属性</span>
    </template>

    <div class='modalbody property-dialog'>
      <div class='property-scroll'>
        <section class='property-hero'>
          <div class='property-kind-icon'>
            <Folder v-if="fileInfo?.type === 'folder'" :size='22' />
            <FileText v-else :size='22' />
          </div>
          <div class='property-hero-copy'>
            <span class='property-kicker'>{{ fileTypeLabel }}</span>
            <h2 class='property-name'>{{ propertyText(fileInfo?.name) }}</h2>
          </div>
          <a-spin v-if='loading' :size='20' />
        </section>

        <section class='property-section'>
          <div class='property-section-heading'>
            <span>路径</span>
            <a-button type='text' size='mini' title='复制路径' :disabled='!dirPath' @click='handleCopyPath'>
              <Copy :size='14' />
            </a-button>
          </div>
          <div class='property-path'>{{ propertyText(dirPath) }}</div>
        </section>

        <section class='property-section'>
          <div class='property-section-heading'>
            <span>文件身份</span>
            <a-button type='text' size='mini' title='复制文件名' :disabled='!fileInfo?.name' @click='handleCopyFileName'>
              <Copy :size='14' />
            </a-button>
          </div>
          <div class='property-name-row'>
            <span class='property-name-value'>{{ propertyText(fileInfo?.name) }}</span>
            <span class='property-type-badge'>{{ fileTypeLabel }}</span>
          </div>
        </section>

        <section class='property-section'>
          <div class='property-section-heading'><span>基本信息</span></div>
          <div class='property-grid'>
            <div class='property-field'>
              <span class='property-label'>文件大小</span>
              <span class='property-value property-value-with-action'>
                {{ displaySize }}
                <button class='property-icon-button' type='button' title='切换大小格式' @click='handleSize'>
                  <RefreshCw :size='14' />
                </button>
              </span>
            </div>
            <div class='property-field'>
              <span class='property-label'>创建日期</span>
              <span class='property-value'>{{ propertyText(humanDateTimeDateStr(fileInfo?.created_at)) }}</span>
            </div>
            <div class='property-field'>
              <span class='property-label'>更新日期</span>
              <span class='property-value'>{{ propertyText(humanDateTimeDateStr(fileInfo?.updated_at)) }}</span>
            </div>
            <div v-if="fileInfo?.type === 'file'" class='property-field'>
              <span class='property-label'>分类</span>
              <span class='property-value'>{{ propertyText(fileInfo?.category) }}</span>
            </div>
            <div v-if="fileInfo?.type === 'file'" class='property-field'>
              <span class='property-label'>媒体类型</span>
              <span class='property-value'>{{ propertyText(fileInfo?.mime_type) }}</span>
            </div>
            <div v-if="fileInfo?.type === 'file'" class='property-field property-field-wide'>
              <span class='property-label'>描述</span>
              <span class='property-value property-value-multiline'>{{ propertyText(fileInfo?.description) }}</span>
            </div>
          </div>
        </section>

        <section v-if="fileInfo?.type === 'folder'" class='property-section'>
          <div class='property-section-heading'><span>文件夹统计</span></div>
          <div class='property-stat'>{{ folderStatsText }}</div>
        </section>

        <section v-if="fileInfo?.type === 'file'" class='property-section'>
          <div class='property-section-heading'><span>文件校验</span></div>
          <div class='property-field property-field-wide'>
            <span class='property-label'>SHA1</span>
            <span class='property-value property-value-multiline'>{{ propertyText(fileInfo?.content_hash) }}</span>
          </div>
        </section>

        <section v-if="fileInfo?.category === 'video'" class='property-section'>
          <div class='property-section-heading'><span>视频元数据</span></div>
          <div class='property-grid'>
            <div class='property-field'>
              <span class='property-label'>分辨率</span>
              <span class='property-value'>{{ propertyText(makeFenBianLv(fileInfo?.video_media_metadata?.width, fileInfo?.video_media_metadata?.height)) }}</span>
            </div>
            <div class='property-field'>
              <span class='property-label'>视频时长</span>
              <span class='property-value'>{{ propertyText(humanTime(fileInfo?.video_media_metadata?.duration)) }}</span>
            </div>
            <div class='property-field'>
              <span class='property-label'>制作日期</span>
              <span class='property-value'>{{ propertyText(humanDateTimeDateStr(fileInfo?.video_media_metadata?.time)) }}</span>
            </div>
          </div>
        </section>

        <section v-if="fileInfo?.category === 'image'" class='property-section'>
          <div class='property-section-heading'><span>图片元数据</span></div>
          <div class='property-grid'>
            <div class='property-field'>
              <span class='property-label'>分辨率</span>
              <span class='property-value'>{{ propertyText(makeFenBianLv(fileInfo?.image_media_metadata?.width, fileInfo?.image_media_metadata?.height)) }}</span>
            </div>
            <div class='property-field'>
              <span class='property-label'>拍摄设备</span>
              <span class='property-value property-value-multiline'>{{ propertyText(makeImageSheBei(fileInfo?.image_media_metadata?.exif)) }}</span>
            </div>
            <div class='property-field'>
              <span class='property-label'>拍摄日期</span>
              <span class='property-value'>{{ propertyText(makeImageShiJian(fileInfo?.image_media_metadata?.exif)) }}</span>
            </div>
          </div>
        </section>

        <section v-if="fileInfo?.category === 'audio'" class='property-section property-audio'>
          <div class='property-section-heading'><span>音频预览</span></div>
          <audio controls :src='fileInfo?.thumbnail' @play='handleAudioPlay'>
            您的浏览器不支持 audio 元素
          </audio>
        </section>
      </div>

      <footer class='property-actions'>
        <a-button type='outline' size='small' @click='handleCopyJson'><Braces :size='14' />复制JSON</a-button>
        <span class='property-actions-spacer'></span>
        <template v-if="fileInfo?.description && !fileInfo?.description.includes('mnemoEncrypt')">
          <a-button v-if="fileInfo?.category === 'video' || fileInfo?.category === 'audio'" type='outline' size='small' @click='handleCopyThumbnail'>
            <Link2 :size='14' />复制预览链接
          </a-button>
        </template>
        <a-button v-if="fileInfo?.type !== 'folder'" type='outline' size='small' @click='handleCopyDownload'>
          <Download :size='14' />复制下载链接
        </a-button>
      </footer>
    </div>
  </a-modal>
</template>

<style>
.property-modal .arco-modal {
  width: min(760px, calc(100vw - 24px)) !important;
  max-width: calc(100vw - 24px);
}

.property-dialog {
  display: flex;
  width: 100%;
  max-height: min(720px, calc(82vh - 72px));
  flex-direction: column;
  min-width: 0;
}

.property-scroll {
  min-width: 0;
  overflow-y: auto;
  padding: 0 2px 4px;
  scrollbar-width: thin;
}

.property-hero {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 12px;
  padding: 4px 0 18px;
}

.property-kind-icon {
  display: inline-flex;
  width: 42px;
  height: 42px;
  flex: 0 0 42px;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--color-border-2);
  border-radius: 10px;
  background: var(--color-fill-2);
  color: rgb(var(--primary-6));
}

.property-hero-copy {
  min-width: 0;
  flex: 1;
}

.property-kicker,
.property-label {
  color: var(--color-text-3);
  font-size: 12px;
}

.property-name {
  overflow-wrap: anywhere;
  margin: 3px 0 0;
  color: var(--color-text-1);
  font-size: 17px;
  font-weight: 600;
  line-height: 1.35;
}

.property-section {
  min-width: 0;
  padding: 14px 0;
  border-top: 1px solid var(--color-border-2);
}

.property-section-heading {
  display: flex;
  min-height: 24px;
  align-items: center;
  justify-content: space-between;
  color: var(--color-text-1);
  font-size: 13px;
  font-weight: 600;
}

.property-section-heading .arco-btn {
  color: var(--color-text-3);
}

.property-path,
.property-name-row,
.property-stat {
  min-width: 0;
  margin-top: 8px;
  padding: 9px 11px;
  border: 1px solid var(--color-border-2);
  border-radius: 6px;
  background: var(--color-fill-1);
  color: var(--color-text-2);
  font-size: 13px;
  line-height: 1.5;
  overflow-wrap: anywhere;
  user-select: text;
}

.property-name-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.property-name-value {
  min-width: 0;
  overflow-wrap: anywhere;
  color: var(--color-text-1);
  font-weight: 500;
}

.property-type-badge {
  flex: 0 0 auto;
  padding: 2px 7px;
  border-radius: 4px;
  background: rgb(var(--primary-1));
  color: rgb(var(--primary-6));
  font-size: 11px;
}

.property-grid {
  display: grid;
  min-width: 0;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
  margin-top: 8px;
}

.property-field {
  display: flex;
  min-width: 0;
  min-height: 58px;
  flex-direction: column;
  justify-content: center;
  gap: 5px;
  padding: 9px 11px;
  border: 1px solid var(--color-border-2);
  border-radius: 6px;
  background: var(--color-fill-1);
}

.property-field-wide {
  grid-column: 1 / -1;
}

.property-value {
  min-width: 0;
  overflow-wrap: anywhere;
  color: var(--color-text-1);
  font-size: 13px;
  line-height: 1.45;
  user-select: text;
}

.property-value-multiline {
  white-space: pre-wrap;
}

.property-value-with-action {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
}

.property-icon-button {
  display: inline-flex;
  width: 24px;
  height: 24px;
  flex: 0 0 24px;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: rgb(var(--primary-6));
  cursor: pointer;
}

.property-icon-button:hover {
  background: var(--color-fill-3);
}

.property-audio audio {
  width: 100%;
  height: 36px;
  margin-top: 10px;
}

.property-actions {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
  padding-top: 14px;
  border-top: 1px solid var(--color-border-2);
}

.property-actions .arco-btn {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  white-space: nowrap;
}

.property-actions-spacer {
  min-width: 0;
  flex: 1;
}

@media (max-width: 560px) {
  .property-dialog {
    max-height: calc(84vh - 64px);
  }

  .property-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .property-field-wide {
    grid-column: auto;
  }

  .property-actions {
    flex-wrap: wrap;
  }

  .property-actions-spacer {
    display: none;
  }
}
</style>
