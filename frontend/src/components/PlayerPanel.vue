<script setup>
// 视频播放面板：HTML5 <video> 直接播放本地代理流（支持 Range/CORS）。
// 功能：播放/暂停、进度条、音量、倍速、画质切换、断点续播、全屏、画中画、
// 循环、静音、快捷键、双击全屏、截图、字幕加载。
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
const containerEl = ref(null)
const loading = ref(true)
const error = ref('')
const playing = ref(false)
const position = ref(0)
const duration = ref(0)
const buffered = ref(0)
const volume = ref(getPrefs().defaultVolume ?? 50)
const muted = ref(false)
const speed = ref(getPrefs().defaultSpeed || 1)
const seekStep = getPrefs().seekStep || 10
const qualities = ref([])
const currentQuality = ref('')
const src = ref('')
const looping = ref(false)
const isFullscreen = ref(false)
const pipActive = ref(false)
const showControls = ref(true)
const subtitleTracks = ref([])
const currentSubtitle = ref('')
const subtitleEnabled = ref(false)

let unmounted = false
let playbackSeq = 0
let controlsTimer = null
let saveTimer = null

const SPEEDS = [0.5, 0.75, 1, 1.25, 1.5, 2]
const speedOptions = SPEEDS.map((s) => ({ value: s, label: s + 'x' }))

onMounted(() => startPlayback())
onBeforeUnmount(() => {
  unmounted = true
  playbackSeq++
  saveCursor()
  if (saveTimer) clearInterval(saveTimer)
  if (controlsTimer) clearTimeout(controlsTimer)
  document.removeEventListener('keydown', onKeyDown)
  document.removeEventListener('fullscreenchange', onFullscreenChange)
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
    pendingResume = resumeAt
    document.addEventListener('keydown', onKeyDown)
    document.addEventListener('fullscreenchange', onFullscreenChange)
  } catch (e) {
    if (!unmounted && seq === playbackSeq) error.value = String(e)
  } finally {
    if (!unmounted && seq === playbackSeq) loading.value = false
  }
}

let pendingResume = 0
function onLoaded() {
  const v = videoEl.value
  if (!v) return
  duration.value = v.duration || 0
  v.volume = volume.value / 100
  v.playbackRate = speed.value
  if (pendingResume > 0 && pendingResume < v.duration) {
    v.currentTime = pendingResume
  }
  pendingResume = 0
  v.play().catch(() => {})
  // start periodic save
  if (saveTimer) clearInterval(saveTimer)
  saveTimer = setInterval(saveCursor, 5000)
}

function onTimeUpdate() {
  const v = videoEl.value
  if (!v) return
  position.value = v.currentTime
  if (v.buffered.length > 0) buffered.value = v.buffered.end(v.buffered.length - 1)
}
function onProgress() {
  const v = videoEl.value
  if (!v) return
  if (v.buffered.length > 0) buffered.value = v.buffered.end(v.buffered.length - 1)
}
function onPlay() { playing.value = true; scheduleHideControls() }
function onPause() { playing.value = false; showControls.value = true }
function onEnded() { playing.value = false; saveCursor(); if (!looping.value) showControls.value = true }
function onError() { error.value = '视频加载失败，可能是不支持的格式或网络错误' }
function onVolumeChange() {
  const v = videoEl.value
  if (!v) return
  volume.value = Math.round(v.volume * 100)
  muted.value = v.muted
}
function onPipEnter() { pipActive.value = true }
function onPipLeave() { pipActive.value = false }

function setQualityOptions(preview) {
  if (!preview || !preview.qualities) { qualities.value = []; return }
  qualities.value = preview.qualities.map((q) => ({
    value: q.value || q.quality,
    label: q.label || q.quality,
  }))
  currentQuality.value = preview.current_quality || (qualities.value[0] && qualities.value[0].value) || ''
}

// ---- controls ----
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
  const val = Number(e.target.value)
  volume.value = val
  const v = videoEl.value
  if (v) {
    v.volume = val / 100
    if (val > 0) v.muted = false
  }
}

function toggleMute() {
  const v = videoEl.value
  if (!v) return
  v.muted = !v.muted
}

function onSpeed(val) {
  speed.value = Number(val)
  const v = videoEl.value
  if (v) v.playbackRate = speed.value
}

function toggleLoop() {
  looping.value = !looping.value
  const v = videoEl.value
  if (v) v.loop = looping.value
}

function toggleFullscreen() {
  const el = containerEl.value
  if (!el) return
  if (!document.fullscreenElement) {
    el.requestFullscreen?.().catch(() => {})
  } else {
    document.exitFullscreen?.()
  }
}
function onFullscreenChange() {
  isFullscreen.value = !!document.fullscreenElement
}

async function togglePip() {
  const v = videoEl.value
  if (!v) return
  try {
    if (document.pictureInPictureElement) {
      await document.exitPictureInPicture()
    } else if (document.pictureInPictureEnabled) {
      await v.requestPictureInPicture()
    }
  } catch (e) {
    emit('toast', '画中画不可用: ' + String(e), 'error')
  }
}

function screenshot() {
  const v = videoEl.value
  if (!v || !v.videoWidth) return
  const canvas = document.createElement('canvas')
  canvas.width = v.videoWidth
  canvas.height = v.videoHeight
  const ctx = canvas.getContext('2d')
  if (!ctx) return
  ctx.drawImage(v, 0, 0)
  canvas.toBlob((blob) => {
    if (!blob) return
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = (props.file.name || 'screenshot').replace(/\.[^.]+$/, '') + '-' + Math.floor(position.value) + 's.png'
    a.click()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  }, 'image/png')
}

// ---- subtitle ----
function onTracksChange() {
  const v = videoEl.value
  if (!v) return
  const list = []
  for (let i = 0; i < v.textTracks.length; i++) {
    const t = v.textTracks[i]
    list.push({ index: i, label: t.label || '字幕 ' + (i + 1), kind: t.kind })
  }
  subtitleTracks.value = list
}
function selectSubtitle(idx) {
  const v = videoEl.value
  if (!v) return
  for (let i = 0; i < v.textTracks.length; i++) {
    v.textTracks[i].mode = (i === idx) ? 'showing' : 'disabled'
  }
  subtitleEnabled.value = idx >= 0
  currentSubtitle.value = idx >= 0 ? String(idx) : ''
}
function toggleSubtitle() {
  if (subtitleEnabled.value) selectSubtitle(-1)
  else if (subtitleTracks.value.length > 0) selectSubtitle(0)
}

// ---- save cursor ----
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

// ---- keyboard ----
function onKeyDown(e) {
  if (e.target && e.target.tagName === 'INPUT') return
  switch (e.code) {
    case 'Space': e.preventDefault(); togglePlay(); break
    case 'ArrowLeft': seek(-seekStep); break
    case 'ArrowRight': seek(seekStep); break
    case 'ArrowUp': { const v = videoEl.value; if (v) { v.volume = Math.min(1, v.volume + 0.05); volume.value = Math.round(v.volume * 100) } break }
    case 'ArrowDown': { const v = videoEl.value; if (v) { v.volume = Math.max(0, v.volume - 0.05); volume.value = Math.round(v.volume * 100) } break }
    case 'KeyF': toggleFullscreen(); break
    case 'KeyM': toggleMute(); break
    case 'KeyP': togglePip(); break
    case 'KeyL': toggleLoop(); break
    case 'KeyS': screenshot(); break
    case 'KeyC': toggleSubtitle(); break
    case 'Escape': if (!isFullscreen.value) emit('close'); break
  }
  showControls.value = true
  scheduleHideControls()
}

// ---- auto-hide controls ----
function scheduleHideControls() {
  if (controlsTimer) clearTimeout(controlsTimer)
  if (!playing.value) return
  controlsTimer = setTimeout(() => { showControls.value = false }, 3000)
}
function onMouseMove() {
  showControls.value = true
  scheduleHideControls()
}
function onMouseLeave() {
  if (playing.value) showControls.value = false
}

function fmtTime(s) {
  if (!s || !isFinite(s)) return '00:00'
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = Math.floor(s % 60)
  if (h > 0) return `${h}:${String(m).padStart(2, '0')}:${String(sec).padStart(2, '0')}`
  return String(m).padStart(2, '0') + ':' + String(sec).padStart(2, '0')
}

const pct = computed(() => duration.value > 0 ? Math.min(100, (position.value / duration.value) * 100) : 0)
const bufPct = computed(() => duration.value > 0 ? Math.min(100, (buffered.value / duration.value) * 100) : 0)
</script>

<template>
  <div
    ref="containerEl"
    class="player-panel"
    :class="{ 'cursor-hidden': !showControls && playing, fullscreen: isFullscreen }"
    @mousemove="onMouseMove"
    @mouseleave="onMouseLeave"
  >
    <div class="pp-head" :class="{ hidden: !showControls }">
      <div class="pp-title" :title="file.name">{{ file.name }}</div>
      <button class="icon-btn" title="关闭 (Esc)" @click="emit('close')"><UiIcon name="close" :size="14" /></button>
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
        @progress="onProgress"
        @play="onPlay"
        @pause="onPause"
        @ended="onEnded"
        @error="onError"
        @volumechange="onVolumeChange"
        @enterpictureinpicture="onPipEnter"
        @leavepictureinpicture="onPipLeave"
        @click="togglePlay"
        @dblclick="toggleFullscreen"
        preload="auto"
        crossorigin="anonymous"
        playsinline
      ></video>
    </div>

    <!-- 控制条 -->
    <div v-if="!loading && !error" class="pp-controls" :class="{ hidden: !showControls }">
      <button class="pp-btn" @click="togglePlay" :title="playing ? '暂停 (空格)' : '播放 (空格)'">
        <UiIcon :name="playing ? 'pause' : 'play'" :size="16" />
      </button>
      <button class="pp-btn" @click="seek(-seekStep)" title="快退 (←)">
        <UiIcon name="back" :size="14" />
      </button>
      <button class="pp-btn" @click="seek(seekStep)" title="快进 (→)">
        <UiIcon name="forward" :size="14" />
      </button>
      <span class="pp-time">{{ fmtTime(position) }} / {{ fmtTime(duration) }}</span>
      <div class="pp-progress">
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
        :options="speedOptions"
        style="width:80px"
        @update:modelValue="onSpeed"
      />
      <button class="pp-btn" :class="{ active: looping }" @click="toggleLoop" title="循环 (L)">
        <UiIcon name="refresh" :size="14" />
      </button>
      <button class="pp-btn" :class="{ active: subtitleEnabled }" @click="toggleSubtitle" title="字幕 (C)" v-if="subtitleTracks.length > 0">
        <UiIcon name="list" :size="14" />
      </button>
      <button class="pp-btn" @click="screenshot" title="截图 (S)">
        <UiIcon name="copy" :size="14" />
      </button>
      <button class="pp-btn" :class="{ active: pipActive }" @click="togglePip" title="画中画 (P)">
        <UiIcon name="external" :size="14" />
      </button>
      <button class="pp-btn" @click="toggleFullscreen" :title="isFullscreen ? '退出全屏 (F)' : '全屏 (F)'">
        <UiIcon :name="isFullscreen ? 'close' : 'plus'" :size="14" />
      </button>
      <div class="pp-volume">
        <button class="pp-btn" @click="toggleMute" :title="muted ? '取消静音 (M)' : '静音 (M)'">
          <UiIcon :name="muted ? 'close' : 'audio'" :size="14" />
        </button>
        <input type="range" class="pp-vol-input" min="0" max="100" :value="muted ? 0 : volume" @input="onVolume" />
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
  position: relative;
}
.player-panel.fullscreen { border-radius: 0; }
.player-panel.cursor-hidden { cursor: none; }
.pp-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 16px;
  border-bottom: 1px solid var(--border-light);
  flex-shrink: 0;
  transition: opacity var(--motion-fast) var(--motion-ease);
}
.pp-head.hidden { opacity: 0; pointer-events: none; }
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
  gap: 6px;
  padding: 8px 12px;
  background: var(--bg-surface);
  border-top: 1px solid var(--border-light);
  flex-shrink: 0;
  transition: opacity var(--motion-fast) var(--motion-ease);
}
.pp-controls.hidden { opacity: 0; pointer-events: none; }
.pp-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-secondary);
  cursor: pointer;
  transition: background var(--motion-fast) var(--motion-ease);
  flex-shrink: 0;
}
.pp-btn:hover { background: var(--bg-hover); color: var(--text-primary); }
.pp-btn.active { color: var(--color-primary); background: var(--bg-subtle); }
.pp-time {
  font-size: var(--fs-aux);
  color: var(--text-tertiary);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
  flex-shrink: 0;
}
.pp-progress {
  flex: 1;
  height: 6px;
  position: relative;
  background: var(--bg-subtle);
  border-radius: var(--radius-full);
  cursor: pointer;
  overflow: hidden;
  min-width: 80px;
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
  gap: 2px;
  flex-shrink: 0;
}
.pp-vol-input {
  width: 60px;
  cursor: pointer;
  accent-color: var(--color-primary);
}
</style>
