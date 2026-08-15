<script setup>
// 文件预览弹窗：图片 / 文本 / PDF / 音频（走本地 Range 代理）；视频由 mpv 内嵌播放，前端只提示。
import { ref, onMounted } from 'vue'
import { PreviewURL, openKindOf, formatBytes } from '../api'
import Modal from './Modal.vue'
import UiIcon from './UiIcon.vue'

const props = defineProps({
  account: { type: Object, required: true },
  file: { type: Object, required: true },
})
const emit = defineEmits(['close'])

const kind = openKindOf(props.file)
const url = ref('')
const text = ref('')
const loading = ref(true)
const error = ref('')

onMounted(async () => {
  try {
    url.value = await PreviewURL(props.account.user_id, props.account.drive_id, props.file.file_id)
    if (kind === 'text') {
      const resp = await fetch(url.value)
      const buf = await resp.arrayBuffer()
      if (buf.byteLength > 2 * 1024 * 1024) throw new Error('文件过大，无法预览')
      text.value = new TextDecoder('utf-8').decode(buf)
    }
  } catch (e) {
    error.value = String(e)
  }
  loading.value = false
})
</script>

<template>
  <Modal :title="file.name" width="" @close="emit('close')" body-class="preview-body" class="preview-modal-host">
    <div v-if="loading" class="empty"><span class="spin"></span><span>加载预览…</span></div>
    <div v-else-if="error" class="empty"><span class="empty-icon"><UiIcon name="warning" :size="30" /></span><span>{{ error }}</span></div>
    <template v-else>
      <img v-if="kind === 'image'" :src="url" :alt="file.name" />
      <iframe v-else-if="kind === 'pdf'" :src="url" title="pdf"></iframe>
      <audio v-else-if="kind === 'audio'" :src="url" controls autoplay style="height:auto;background:none"></audio>
      <pre v-else>{{ text }}</pre>
    </template>
  </Modal>
</template>

<style scoped>
:deep(.modal) { width: 860px; max-width: 94vw; height: 82vh; }
</style>
