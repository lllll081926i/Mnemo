import { computed, ref, watch } from 'vue'
import { getPlayCursor, openKindOf, PinFileSnapshot, PreviewURL } from '../api'

export function usePreviewLoader({ props, loadImage, onAudioPrepared, onTextLoaded }) {
  const activeFile = ref(props.file)
  const kind = computed(() => openKindOf(activeFile.value))
  const isImmersive = computed(() => kind.value === 'image' || kind.value === 'audio')
  const url = ref('')
  const loading = ref(true)
  const error = ref('')
  let loadSeq = 0

  async function loadPreview() {
    if (kind.value === 'image') return loadImage()
    const seq = ++loadSeq
    // 切曲/换图时保留已渲染舞台，避免整屏闪烁；仅首次或空态才展示全屏 loading
    if (!url.value) loading.value = true
    error.value = ''
    url.value = ''
    try {
      if (!['image', 'text', 'audio'].includes(kind.value)) {
        throw new Error(kind.value === 'pdf' ? 'PDF 暂不支持在线预览，请下载后查看' : '此文件格式不支持在线预览，请下载后查看')
      }
      await PinFileSnapshot(
        props.account.user_id,
        props.account.drive_id,
        activeFile.value
      )
      const previewUrl = await PreviewURL(
        props.account.user_id,
        props.account.drive_id,
        activeFile.value.file_id
      )
      if (seq !== loadSeq) return
      if (kind.value === 'audio') {
        const cursor = await getPlayCursor(
          props.account.user_id,
          props.account.drive_id,
          activeFile.value.file_id
        ).catch(() => 0)
        if (seq !== loadSeq) return
        onAudioPrepared(cursor)
      }
      url.value = previewUrl
      if (kind.value === 'text') {
        const resp = await fetch(previewUrl)
        if (!resp.ok) throw new Error(`HTTP ${resp.status} 加载失败`)
        const buf = await resp.arrayBuffer()
        if (buf.byteLength > 4 * 1024 * 1024) throw new Error('文本文件超过 4MB，不支持在线预览，请下载后查看')
        onTextLoaded(buf)
      }
    } catch (e) {
      if (seq !== loadSeq) return
      error.value = String(e && e.message ? e.message : e)
    } finally {
      if (seq === loadSeq) loading.value = false
    }
  }

  watch(() => props.file, (file) => {
    if (file) activeFile.value = file
  })
  watch(() => activeFile.value.file_id, loadPreview)

  return {
    activeFile,
    kind,
    isImmersive,
    url,
    loading,
    error,
    loadPreview,
  }
}
