<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { Folder, X } from 'lucide-vue-next'
import DownDAL from './DownDAL'
import { useSettingStore } from '../store'
import message from '../utils/message'
import { parseExternalDownloadPayload } from './integration/protocolPayload'

const props = defineProps({
  visible: {
    type: Boolean,
    required: true
  },
  initialUrl: {
    type: String,
    default: ''
  }
})

const emit = defineEmits(['update:visible'])

const settingStore = useSettingStore()
const okLoading = ref(false)
const showAdvanced = ref(false)
const form = reactive({
  sources: '',
  fileName: '',
  savePath: '',
  split: 64,
  userAgent: '',
  authorization: '',
  referer: '',
  cookie: '',
  allProxy: ''
})

const maxSplit = computed(() => Math.max(1, settingStore.downThreadMax || 64, 64))

const defaultSavePath = () => {
  const ariaRemote = settingStore.ariaState === 'remote'
  return ariaRemote ? settingStore.ariaSavePath : settingStore.downSavePath
}

const resetForm = () => {
  form.sources = props.initialUrl || ''
  form.fileName = ''
  form.savePath = defaultSavePath()
  form.split = Math.max(1, settingStore.downThreadMax || 64)
  form.userAgent = ''
  form.authorization = ''
  form.referer = ''
  form.cookie = ''
  form.allProxy = ''
  showAdvanced.value = false
}

const parseSourceLines = () => form.sources
  .split(/\r?\n/)
  .map(line => line.trim())
  .filter(Boolean)

const normalizeSplit = () => {
  const value = Number(form.split) || 1
  form.split = Math.min(Math.max(1, value), maxSplit.value)
}

const handleHide = () => emit('update:visible', false)

const handleClose = () => {
  okLoading.value = false
}

const handleSelectSavePath = () => {
  if (!window.WebShowOpenDialogSync) return
  window.WebShowOpenDialogSync({
    title: '选择下载保存目录',
    defaultPath: form.savePath || defaultSavePath(),
    properties: ['openDirectory', 'createDirectory', 'showHiddenFiles', 'noResolveAliases', 'dontAddToRecent']
  }, (result: string[] | undefined) => {
    if (result?.[0]) form.savePath = result[0]
  })
}

const validateSources = () => {
  const sources = parseSourceLines()
  if (!sources.length) return '请输入下载链接'
  if (sources.some(source => !parseExternalDownloadPayload(source))) return '仅支持 HTTP/HTTPS 下载链接，不支持 magnet 或种子文件'
  if (!form.savePath.trim()) return '请选择保存目录'
  return ''
}

const handleCreate = async () => {
  const error = validateSources()
  if (error) {
    message.error(error)
    return
  }

  normalizeSplit()
  okLoading.value = true
  const sources = parseSourceLines()
  let successCount = 0

  for (const source of sources) {
    const result = DownDAL.aAddExternalDownload({
      source,
      savePath: form.savePath.trim(),
      fileName: sources.length === 1 ? form.fileName.trim() : '',
      split: form.split,
      userAgent: form.userAgent.trim(),
      authorization: form.authorization.trim(),
      referer: form.referer.trim(),
      cookie: form.cookie.trim(),
      allProxy: form.allProxy.trim()
    })
    if (result.success) successCount++
    else message.error(result.message || '创建下载任务失败')
  }

  okLoading.value = false
  if (successCount > 0) {
    message.success(`已创建 ${successCount} 个下载任务`)
    handleHide()
  }
}

</script>

<template>
  <a-modal
    :visible="props.visible"
    modal-class="download-task-modal"
    :footer="false"
    :unmount-on-close="true"
    :mask-closable="false"
    :closable="false"
    :width="620"
    @cancel="handleHide"
    @before-open="resetForm"
    @close="handleClose"
  >
    <div class="download-task">
      <header class="download-task-header">
        <span>新建下载</span>
        <button class="icon-button" type="button" title="关闭" aria-label="关闭" @click="handleHide">
          <X :size="18" />
        </button>
      </header>

      <div class="download-task-body">
        <a-textarea
          v-model="form.sources"
          class="source-input"
          :auto-size="{ minRows: 3, maxRows: 6 }"
          placeholder="每行一个 HTTP/HTTPS 下载链接"
          @keydown.stop
        />

        <div class="form-grid">
          <label for="download-name">重命名</label>
          <a-input id="download-name" v-model.trim="form.fileName" placeholder="选填" />
          <label for="download-split">分片数</label>
          <a-input-number id="download-split" v-model="form.split" :min="1" :max="maxSplit" mode="button" @blur="normalizeSplit" />
        </div>

        <div class="path-row">
          <label for="download-path">存储路径</label>
          <div class="path-input">
            <a-input id="download-path" v-model="form.savePath" readonly />
            <button class="path-button" type="button" title="选择目录" aria-label="选择目录" @click="handleSelectSavePath">
              <Folder :size="18" />
            </button>
          </div>
        </div>

        <div v-if="showAdvanced" class="advanced-settings">
          <div class="form-grid advanced-grid">
            <label for="download-agent">User-Agent</label>
            <a-input id="download-agent" v-model="form.userAgent" />
            <label for="download-referer">Referer</label>
            <a-input id="download-referer" v-model="form.referer" />
          </div>
          <div class="advanced-row">
            <label for="download-authorization">Authorization</label>
            <a-input id="download-authorization" v-model="form.authorization" />
          </div>
          <div class="advanced-row">
            <label for="download-cookie">Cookie</label>
            <a-textarea id="download-cookie" v-model="form.cookie" :auto-size="{ minRows: 2, maxRows: 3 }" />
          </div>
          <div class="advanced-row">
            <label for="download-proxy">代理</label>
            <a-input id="download-proxy" v-model="form.allProxy" placeholder="[http://][USER:PASSWORD@]HOST[:PORT]" />
          </div>
        </div>
      </div>

      <footer class="download-task-footer">
        <a-checkbox v-model="showAdvanced">高级选项</a-checkbox>
        <div class="footer-actions">
          <a-button size="small" @click="handleHide">取消</a-button>
          <a-button size="small" type="primary" :loading="okLoading" @click="handleCreate">提交</a-button>
        </div>
      </footer>
    </div>
  </a-modal>
</template>

<style scoped>
:global(.download-task-modal) {
  min-width: 420px;
  padding: 0;
  overflow: hidden;
  border-radius: 5px;
}

:global(.download-task-modal .arco-modal-header) {
  display: none;
}

:global(.download-task-modal .arco-modal-body) {
  padding: 0;
}

.download-task {
  color: var(--mn-text-primary, #171a1f);
  background: var(--mn-surface, #fff);
}

.download-task-header,
.download-task-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 46px;
  padding: 0 18px;
}

.download-task-header {
  color: var(--mn-text-primary, #171a1f);
  font-size: 15px;
  font-weight: 600;
  border-bottom: 1px solid var(--mn-border, #e5e7eb);
}

.download-task-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 16px 18px;
}

.source-input {
  width: 100%;
}

.form-grid {
  display: grid;
  grid-template-columns: 64px minmax(160px, 1fr) 52px 132px;
  gap: 8px 10px;
  align-items: center;
}

.form-grid label,
.path-row > label,
.advanced-row > label {
  color: var(--mn-text-secondary, #4b5563);
  font-size: 13px;
}

.path-row,
.advanced-row {
  display: grid;
  grid-template-columns: 64px minmax(0, 1fr);
  gap: 10px;
  align-items: center;
}

.path-input {
  display: flex;
  min-width: 0;
}

.path-input :deep(.arco-input-wrapper) {
  border-radius: 4px 0 0 4px;
}

.icon-button,
.path-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  color: var(--mn-text-secondary, #4b5563);
  background: transparent;
  border: 0;
  cursor: pointer;
}

.icon-button {
  width: 28px;
  height: 28px;
}

.path-button {
  width: 36px;
  flex: 0 0 36px;
  border: 1px solid var(--mn-border-strong, #cfd4dc);
  border-left: 0;
  border-radius: 0 4px 4px 0;
}

.icon-button:hover,
.path-button:hover {
  color: var(--mn-primary, #356ae6);
  background: var(--mn-primary-soft, #eef4ff);
}

.advanced-settings {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding-top: 12px;
  border-top: 1px solid var(--mn-border, #e5e7eb);
}

.advanced-grid {
  grid-template-columns: 78px minmax(120px, 1fr) 52px minmax(120px, 1fr);
}

.advanced-row {
  grid-template-columns: 78px minmax(0, 1fr);
}

.download-task-footer {
  border-top: 1px solid var(--mn-border, #e5e7eb);
}

.footer-actions {
  display: flex;
  gap: 8px;
}

@media (max-width: 640px) {
  :global(.download-task-modal) {
    width: calc(100vw - 24px) !important;
    min-width: 0;
  }

  .form-grid,
  .advanced-grid {
    grid-template-columns: 64px minmax(0, 1fr);
  }
}
</style>
