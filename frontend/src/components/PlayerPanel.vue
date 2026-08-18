<script setup>
// 视频播放面板：使用 HTML5 <video> 直接播放本地代理流（支持 Range/CORS）。
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { playVideo, playVideoQuality, pinFileSnapshot, getPlayCursor, savePlayCursor, getSettings } from '../api'
import { getPrefs } from '../appearance'
import UiIcon from './UiIcon.vue'
import UiSelect from './UiSelect.vue'

const props = defineProps({
  account: { type: Object, required: true },
  file: { type: Object, required: true },
})
const emit = defineEmits(['close', 'toast'])

const videoEl = ref(null)
const loading = ref(true)
const error = ref('')
const playing = ref(false)
const position = ref(0)
const duration = ref(0)
const buffered = ref(0)
const volume = ref(getPrefs().defaultVolume ?? 50)
const speed = ref(getPrefs().defaultSpeed || 1)
const seekStep = getPrefs().seekStep || 10
const qualities = ref([])
const currentQuality = ref('')
const src = ref('')

let unmounted = false
let playbackSeq = 0
let progressTimer = null

onMounted(() => startPlayback())
onBeforeUnmount(() => {
  unmounted = true
  playbackSeq++
  saveCursor()
  if (progressTimer) clearInterval(progressTimer)
})

async function startPlayback() {
  const seq = ++playbackSeq
  loading.value = true
  error.value = ''
  src.value = ''
  try {
    await pinFileSnapshot(props.account.user_id, props.account.drive_id, props.file)
    if (unmounted || seq !== playbackSeq) return
    const st = await getSettings().catch(() => null)
    let resumeAt = 0
    if (!st || st.playbackResume !== false) {
      resumeAt = await getPlayCursor(props.account.user_id, props.account.drive_id, props.file.file_id).catch(() => 0)
    }
    if (unmounted || seq !== playbackSeq) return
    const preview = await playVideo(props.account.user_id, props.account.drive_id, props.file.file_id)
    if (unmounted || seq !== playbackSeq) return
    setQualityOptions(preview)
    if (preview && Number.isFinite(preview.duration) && preview.duration > 0) {
      duration.value = preview.duration
    }
    src.value = preview.url || ''
    if (resumeAt > 0) {
      const v = videoEl.value
      if (v) v.currentTime = resumeAt
    }
  } catch (e) {
    if (!unmounted && seq === playbackSeq) error.value = String(e)
  } finally {
    if (!unmounted && seq === playbackSeq) loading.value = false
  }
}

function setQualityOptions(preview) {
  if (!preview || !preview.qualities) { qualities.value = []; return }
  qualities.value = preview.qualities.map((q) => ({
    value: q.value || q.quality,
    label: q.label || q.quality,
  }))
  currentQuality.value = preview.currentQuality || (qualities.value[0] && qualities.value[0].value) || ''
}

// ---- video element events ----
function onLoaded() {
  const v = videoEl.value
  if (!v) return
  duration.value = v.duration || 0
  v.volume = volume.value / 100
  v.playbackRate = speed.value
  v.play().catch(() => {})
}
function onTimeUpdate() {
  const v = videoEl.value
  if (!v) return
  position.value = v.currentTime
  if (v.buffered.length > 0) buffered.value = v.buffered.end(v.buffered.length - 1)
}
function onPlay() { playing.value = true }
function onPause() { playing.value = false }
function onEnded() { playing.value = false; saveCursor() }
function onError() { error.value = '视频加载失败，可能是不支持的格式或网络错误' }

function togglePlay() {
  const v = videoEl.value
  if (!v) return
  if (v.paused) v.play().catch(() => {})
  else v.pause()
}

function seek(delta) {
  const v = videoEl.value
  if (!v) return
  v.currentTime = Math.max(0, Math.min(v.duration || 0, v.currentTime + delta))
}

function onSeekInput(e) {
  const v = videoEl.value
  if (!v) return
  const t = Number(e.target.value)
  v.currentTime = t
  position.value = t
}

function onVolume(e) {
  volume.value = Number(e.target.value)
  const v = videoEl.value
  if (v) v.volume = volume.value / 100
}

function onSpeed(e) {
  speed.value = Number(e.target.value)
  const v = videoEl.value
  if (v) v.playbackRate = speed.value
}

function saveCursor() {
  const v = videoEl.value
  if (!v || !v.currentTime || v.currentTime < 1) return
  savePlayCursor(props.account.user_id, props.account.drive_id, props.file.file_id, v.currentTime).catch(() => {})
}

async function switchQuality(q) {
  if (q === currentQuality.value) return
  const v = videoEl.value
  const wasPlaying = v && !v.paused
  const cur = v ? v.currentTime : 0
  const seq = playbackSeq
  try {
    const preview = await playVideoQuality(props.account.user_id, props.account.drive_id, props.file.file_id, q)
    if (unmounted || seq !== playbackSeq) return
    currentQuality.value = q
    src.value = preview.url || src.value
    // restore position after src change
    const restore = () => {
      const nv = videoEl.value
      if (!nv) return
      nv.removeEventListener('loadedmetadata', restore)
      nv.currentTime = cur
      if (wasPlaying) nv.play().catch(() => {})
    }
    if (v) v.addEventListener('loadedmetadata', restore)
  } catch (e) {
    emit('toast', String(e), 'error')
  }
}

function fmtTime(s) {
  if (!s || !isFinite(s)) return '00:00'
  const m = Math.floor(s / 60)
  const sec = Math.floor(s % 60)
  return String(m).padStart(2, '0') + ':' + String(sec).padStart(2, '0')
}

const pct = computed(() => duration.value > 0 ? Math.min(100, (position.value / duration.value) * 100) : 0)
const bufPct = computed(() => duration.value > 0 ? Math.min(100, (buffered.value / duration.value) * 100) : 0)
</script>

<template>
  <div class="player-panel">
    <div class="pp-head">
      <div class="pp-title" :title="file.name">{{ file.name }}</div>
      <button class="icon-btn" title="关闭" @click="emit('close')"><UiIcon name="close" :size="14" /></button>
    </div>

    <div class="pp-stage">
      <div v-if="loading" class="pp-state"><span class="spin"></span>正在获取播放地址…</div>
      <div v-else-if="error" class="pp-state pp-error">
        <UiIcon name="warn" :size="28" />
        <span>{{ error }}</span>
        <button class="btn sm" @click="startPlayback">重试</button>
      </div>
      <video
        v-else
        ref="videoEl"
        :src="src"
        class="pp-video"
        @loadedmetadata="onLoaded"
        @timeupdate="onTimeUpdate"
        @play="onPlay"
        @pause="onPause"
        @ended="onEnded"
        @error="onError"
        @click="togglePlay"
        preload="auto"
      ></video>
    </div>

    <!-- 控制条 -->
    <div v-if="!loading && !error" class="pp-controls">
      <button class="pp-btn" @click="togglePlay" :title="playing ? '暂停' : '播放'">
        <UiIcon :name="playing ? 'pause' : 'play'" :size="16" />
      </button>
      <button class="pp-btn" @click="seek(-seekStep)" title="快退">
        <UiIcon name="back" :size="14" />
      </button>
      <button class="pp-btn" @click="seek(seekStep)" title="快进">
        <UiIcon name="forward" :size="14" />
      </button>
      <span class="pp-time">{{ fmtTime(position) }} / {{ fmtTime(duration) }}</span>
      <div class="pp-progress" @pointerdown="(e) => onSeekInput(e)">
        <div class="pp-progress-buffer" :style="{ width: bufPct + '%' }"></div>
        <div class="pp-progress-fill" :style="{ width: pct + '%' }"></div>
        <input type="range" class="pp-progress-input" min="0" :max="duration || 0" step="0.1" :value="position" @input="onSeekInput" />
      </div>
      <UiSelect
        v-if="qualities.length > 1"
        :modelValue="currentQuality"
        :options="qualities"
        style="width:90px"
        @update:modelValue="switchQuality"
      />
      <UiSelect
        :modelValue="speed"
        :options="[{value:0.5,label:'0.5x'},{value:0.75,label:'0.75x'},{value:1,label:'1x'},{value:1.25,label:'1.25x'},{value:1.5,label:'1.5x'},{value:2,label:'2x'}]"
        style="width:80px"
        @update:modelValue="onSpeed"
      />
      <div class="pp-volume">
        <UiIcon name="audio" :size="14" />
        <input type="range" class="pp-vol-input" min="0" max="100" :value="volume" @input="onVolume" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.player-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--bg-surface);
  border-radius: var(--radius-xl);
  overflow: hidden;
}
.pp-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 16px;
  border-bottom: 1px solid var(--border-light);
  flex-shrink: 0;
}
.pp-title {
  font-size: var(--fs-body);
  font-weight: 600;
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.pp-stage {
  flex: 1;
  min-height: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #000;
  position: relative;
}
.pp-video {
  width: 100%;
  height: 100%;
  object-fit: contain;
  cursor: pointer;
}
.pp-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  color: var(--text-tertiary);
  font-size: var(--fs-aux);
}
.pp-error { color: var(--color-danger); }
.pp-controls {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 16px;
  background: var(--bg-surface);
  border-top: 1px solid var(--border-light);
  flex-shrink: 0;
}
.pp-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-secondary);
  cursor: pointer;
  transition: background var(--motion-fast) var(--motion-ease);
}
.pp-btn:hover { background: var(--bg-hover); color: var(--text-primary); }
.pp-time {
  font-size: var(--fs-aux);
  color: var(--text-tertiary);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}
.pp-progress {
  flex: 1;
  height: 6px;
  position: relative;
  background: var(--bg-subtle);
  border-radius: var(--radius-full);
  cursor: pointer;
  overflow: hidden;
}
.pp-progress-buffer {
  position: absolute;
  height: 100%;
  background: color-mix(in srgb, var(--text-tertiary) 30%, transparent);
  border-radius: var(--radius-full);
}
.pp-progress-fill {
  position: absolute;
  height: 100%;
  background: var(--color-primary);
  border-radius: var(--radius-full);
}
.pp-progress-input {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  opacity: 0;
  cursor: pointer;
  margin: 0;
}
.pp-volume {
  display: flex;
  align-items: center;
  gap: 4px;
  color: var(--text-tertiary);
}
.pp-vol-input {
  width: 60px;
  cursor: pointer;
  accent-color: var(--color-primary);
}
</style>
