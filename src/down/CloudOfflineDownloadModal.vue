<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { modalCloseAll, modalSelectPanDir } from '../utils/modal'
import { useModalStore, useUserStore } from '../store'
import { isPikPakUser } from '../aliapi/utils'
import message from '../utils/message'
import DownDAL from './DownDAL'
import { isDriveProviderRootId } from '../utils/driveProvider'

const props = defineProps({
  visible: {
    type: Boolean,
    required: true
  }
})

const formRef = ref()
const okLoading = ref(false)
const modalStore = useModalStore()
const userStore = useUserStore()
const provider = computed(() => {
  const user = userStore.user_id || ''
  if (isPikPakUser(user)) return 'pikpak'
  return ''
})
const urlPlaceholder = 'http/https、magnet 或 ed2k 链接'
const providerLabel = computed(() => provider.value === 'pikpak' ? 'PikPak' : '当前网盘')
const form = reactive({
  url: '',
  fileName: '',
  dirId: '',
  dirName: '默认（来自:离线下载）'
})

const handleOpen = () => {
  const preset = modalStore.modalData?.offlineForm
  if (preset) {
    form.url = preset.url || ''
    form.fileName = preset.fileName || ''
    form.dirId = preset.dirId || ''
    form.dirName = preset.dirName || '默认（来自:离线下载）'
  } else {
    form.url = ''
    form.fileName = ''
    form.dirId = ''
    form.dirName = '默认（来自:离线下载）'
  }
}

const handleClose = () => {
  if (okLoading.value) okLoading.value = false
}

const handleHide = () => {
  modalCloseAll()
}

const handleSelectDir = () => {
  const snapshot = {
    url: form.url,
    fileName: form.fileName,
    dirId: form.dirId,
    dirName: form.dirName
  }
  modalSelectPanDir('offline', form.dirId, (user_id: string, drive_id: string, selectFile: any) => {
    if (!selectFile || selectFile.isDir !== true) return
    if (selectFile.file_id && isDriveProviderRootId('pikpak', String(selectFile.file_id))) {
      snapshot.dirId = ''
      snapshot.dirName = '默认（来自:离线下载）'
    } else {
      snapshot.dirId = String(selectFile.file_id || '')
      snapshot.dirName = selectFile.name || '已选择'
    }
    modalStore.showModal('cloudoffline', { offlineForm: snapshot })
  })
}

const handleCreate = async () => {
  if (!provider.value) {
    message.error('当前账号不支持离线下载')
    return
  }
  const urls = Array.from(new Set(form.url.split(/\r?\n/).map(item => item.trim()).filter(Boolean)))
  if (!urls.length) {
    message.error('请输入离线下载地址')
    return
  }
  const unsupported = urls.find(url => !/^(https?:\/\/|magnet:\?|ed2k:\/\/)/i.test(url))
  if (unsupported) {
    message.error('仅支持 http/https、magnet 或 ed2k 链接')
    return
  }
  okLoading.value = true
  let success = 0
  const failures: string[] = []
  for (const url of urls) {
    const result = provider.value === 'pikpak'
      ? await DownDAL.aAddPikPakOfflineDownload(url, urls.length === 1 ? form.fileName.trim() : '', form.dirId)
      : { success: false, message: '当前账号不支持离线下载' }
    if (result.success) success++
    else failures.push(`${url.slice(0, 80)}：${result.message || '创建失败'}`)
  }
  okLoading.value = false
  if (!success) {
    message.error(failures[0]?.split('：')[1] || '创建离线下载失败')
    return
  }
  if (failures.length) message.warning(`已创建 ${success}/${urls.length} 个离线任务`)
  else message.success(`已创建 ${success} 个 ${providerLabel.value} 离线任务`)
  modalCloseAll()
}
</script>

<template>
  <a-modal
    :visible="props.visible"
    modal-class="modalclass"
    :footer="false"
    :unmount-on-close="true"
    :mask-closable="false"
    @cancel="handleHide"
    @before-open="handleOpen"
    @close="handleClose"
  >
    <template #title>
      <span class="modaltitle">创建离线下载任务</span>
    </template>
    <div class="offline-modal">
      <div class="offline-provider">
        <span class="offline-provider-badge">{{ providerLabel }}</span>
        <span class="offline-provider-tip">将链接下载到网盘，不经过本机</span>
      </div>
      <a-form ref="formRef" :model="form" layout="vertical" class="offline-form">
        <a-form-item field="url" label="下载链接" class="offline-item">
          <a-textarea v-model="form.url" :placeholder="`${urlPlaceholder}，每行一个`" :auto-size="{ minRows: 4, maxRows: 10 }" />
        </a-form-item>
        <a-form-item field="fileName" label="自定义文件名（可选，仅单个链接时生效）" class="offline-item">
          <a-input v-model.trim="form.fileName" placeholder="例如：视频.mp4" allow-clear />
        </a-form-item>
        <a-form-item field="dirId" label="保存到" class="offline-item">
          <div class="offline-dir">
            <div class="offline-dir-name" :title="form.dirName">
              <IconFont name="iconfile-folder" />
              <span>{{ form.dirName }}</span>
            </div>
            <a-button type="outline" size="small" @click="handleSelectDir">选择文件夹</a-button>
          </div>
        </a-form-item>
        <div class="offline-footer mn-footer">
          <span class="mn-footer-spacer"></span>
          <a-button type="outline" size="small" @click="handleHide">取消</a-button>
          <a-button type="primary" size="small" :loading="okLoading" @click="handleCreate">创建任务</a-button>
        </div>
      </a-form>
    </div>
  </a-modal>
</template>

<style scoped>
.offline-modal {
  display: flex;
  flex-direction: column;
  gap: 14px;
  width: 560px;
  max-width: calc(100vw - 64px);
}

.offline-provider {
  display: flex;
  align-items: center;
  gap: 10px;
}

.offline-provider-badge {
  padding: 2px 10px;
  font-size: 12px;
  font-weight: 600;
  color: var(--color-primary);
  background: color-mix(in srgb, var(--color-primary) 10%, transparent);
  border-radius: 10px;
}

.offline-provider-tip {
  font-size: 12px;
  color: var(--color-text-3);
}

.offline-form :deep(.arco-form-item-label) {
  font-size: 13px;
  font-weight: 600;
  color: var(--color-text-2);
}

.offline-item {
  margin-bottom: 14px;
}

.offline-dir {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
}

.offline-dir-name {
  display: inline-flex;
  flex: 1 1 auto;
  gap: 6px;
  align-items: center;
  min-width: 0;
  height: 30px;
  padding: 0 10px;
  overflow: hidden;
  font-size: 13px;
  color: var(--color-text-1);
  background: var(--color-fill-2);
  border-radius: 6px;
}

.offline-dir-name span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.offline-dir-name .iconfont,
.offline-dir-name .iconfont-svg {
  flex: 0 0 auto;
  color: #ffb74d;
}

.offline-footer {
  display: flex;
  align-items: center;
  gap: 8px;
}
</style>
