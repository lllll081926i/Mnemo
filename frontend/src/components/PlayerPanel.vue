<script setup>
// 视频播放控制面板：所有视频统一交给 mpv 独立窗口播放。
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import {
  PlayVideo, PlayVideoQuality, StopPlayer, PausePlayer, SeekPlayer, SetPlayerVolume, SetPlayerSpeed,
  PinFileSnapshot, GetPlayCursor, SavePlayCursor, GetSettings, GetPlayerState,
} from '../api'
import { getPrefs } from '../appearance'
import UiIcon from './UiIcon.vue'
import UiSelect from './UiSelect.vue'

const props = defineProps({
  account: { type: Object, required: true },
  file: { type: Object, required: true },
})
const emit = defineEmits(['close', 'toast'])

const loading = ref(true)
const error = ref('')
const playing = ref(false)
const started = ref(false)
const position = ref(0)
const duration = ref(0)
const prefs = getPrefs()
const volume = ref(prefs.defaultVolume ?? 50)
const speed = ref(prefs.defaultSpeed || 1)
const seekStep = prefs.seekStep || 10
const busy = ref(false)
const qualities = ref([])
const currentQuality = ref('')
let unmounted = false
let pendingCursor = 0
let progressTimer = null
let syncingProgress = false
let playbackSeq = 0
let stateErrors = 0

const SPEEDS = [0.5, 0.75, 1, 1.25, 1.5, 2]
const speedOptions = SPEEDS.map((s) => ({ value: s, label: s + 'x' }))

onBeforeUnmount(() => {
  unmounted = true
  playbackSeq++
  if (progressTimer) {
    clearInterval(progressTimer)
    progressTimer = null
  }
  if (scrubbing) {
    scrubbing = false
    window.removeEventListener('mousemove', onScrubMove)
    window.removeEventListener('mouseup', onScrubUp)
  }
  window.removeEventListener('keydown', onKey)
  // 保存续播进度并停止播放
  if (started.value && position.value > 5 && (duration.value <= 0 || position.value < duration.value - 10)) {
    SavePlayCursor(props.account.user_id, props.account.drive_id, props.file.file_id, position.value).catch(() => {})
  }
  StopPlayer().catch(() => {})
})

function fmt(sec) {
  sec = Math.max(0, Math.floor(sec || 0))
  const h = Math.floor(sec / 3600), m = Math.floor((sec % 3600) / 60), s = sec % 60
  return (h ? h + ':' + String(m).padStart(2, '0') : String(m)) + ':' + String(s).padStart(2, '0')
}
const posText = computed(() => fmt(position.value))
const durText = computed(() => fmt(duration.value))
const pct = computed(() => (duration.value > 0 ? Math.min(100, (position.value / duration.value) * 100) : 0))
const qualityOptions = computed(() => qualities.value.map((q) => ({
  value: q.value,
  label: q.label,
})))

function qualityValue(q) {
  return String(q.quality || q.value || q.label || '').trim()
}

function qualityLabel(q) {
  return String(q.label || q.quality || q.value || '原画').trim()
}

function setQualityOptions(preview) {
  const seen = new Set()
  qualities.value = (preview && Array.isArray(preview.qualities) ? preview.qualities : [])
    .map((q) => ({ value: qualityValue(q), label: qualityLabel(q) }))
    .filter((q) => q.value && !seen.has(q.value) && seen.add(q.value))
  const origin = qualities.value.find((q) => q.value.toLowerCase() === 'origin' || q.label === '原画')
  currentQuality.value = (origin || qualities.value[0] || {}).value || ''
}

function clearProgressSync() {
  if (progressTimer) {
    clearInterval(progressTimer)
    progressTimer = null
  }
}

async function startPlayback() {
  const seq = ++playbackSeq
  loading.value = true
  error.value = ''
  started.value = false
  playing.value = false
  position.value = 0
  duration.value = 0
  pendingCursor = 0
  qualities.value = []
  currentQuality.value = ''
  stateErrors = 0
  try {
    await StopPlayer().catch(() => {})
    const st = await GetSettings().catch(() => null)
    if (unmounted || seq !== playbackSeq) return
    if (!st || st.playbackResume !== false) {
      pendingCursor = await GetPlayCursor(props.account.user_id, props.account.drive_id, props.file.file_id).catch(() => 0)
    }
    if (unmounted || seq !== playbackSeq) return
    await PinFileSnapshot(props.account.user_id, props.account.drive_id, props.file)
    if (unmounted || seq !== playbackSeq) return
    const preview = await PlayVideo(props.account.user_id, props.account.drive_id, props.file.file_id)
    if (unmounted || seq !== playbackSeq) {
      if (unmounted) await StopPlayer().catch(() => {})
      return
    }
    setQualityOptions(preview)
    if (preview && Number.isFinite(preview.duration) && preview.duration > 0) {
      duration.value = preview.duration
    }
    started.value = true
    playing.value = true
    await SetPlayerVolume(volume.value).catch((e) => emit('toast', String(e), 'error'))
    await SetPlayerSpeed(speed.value).catch((e) => emit('toast', String(e), 'error'))
    if (pendingCursor > 0) {
      await SeekPlayer(pendingCursor).catch((e) => emit('toast', String(e), 'error'))
      position.value = pendingCursor
    }
    if (unmounted || seq !== playbackSeq) {
      if (unmounted) await StopPlayer().catch(() => {})
      return
    }
    startProgressSync()
  } catch (e) {
    if (!unmounted && seq === playbackSeq) error.value = String(e)
  } finally {
    if (!unmounted && seq === playbackSeq) loading.value = false
  }
}

function startProgressSync() {
  clearProgressSync()
  progressTimer = setInterval(syncProgress, 500)
  syncProgress()
}

async function syncProgress() {
  if (unmounted || !started.value || syncingProgress) return
  syncingProgress = true
  try {
    const state = await GetPlayerState()
    if (unmounted || !state) return
    stateErrors = 0
    const nextPosition = Number(state.position)
    const nextDuration = Number(state.duration)
    if (Number.isFinite(nextDuration) && nextDuration > 0) duration.value = nextDuration
    if (Number.isFinite(nextPosition) && nextPosition >= 0) position.value = nextPosition
    if (duration.value > 0 && position.value >= duration.value - 0.5) playing.value = false
  } catch {
    stateErrors++
    // A closed mpv process otherwise leaves the controller looking alive
    // forever because every state poll is silently retried.
    if (stateErrors >= 12 && started.value && !unmounted) {
      started.value = false
      playing.value = false
      error.value = 'mpv 播放窗口已退出，请重试'
      clearProgressSync()
    }
  }
  finally { syncingProgress = false }
}

function onKey(e) {
  if (e.key === 'Escape') {
    e.preventDefault()
    close()
  } else if (e.code === 'Space') {
    e.preventDefault()
    togglePlay()
  }
}

onMounted(() => {
  startPlayback()
  window.addEventListener('keydown', onKey)
})

async function togglePlay() {
  if (!started.value || busy.value) return
  busy.value = true
  try {
    await PausePlayer(playing.value)
    playing.value = !playing.value
  } catch (e) { emit('toast', String(e), 'error') }
  finally { busy.value = false }
}

let seeking = false
async function seekBy(delta) {
  if (!started.value || seeking) return
  seeking = true
  const target = Math.max(0, position.value + delta)
  position.value = duration.value > 0 ? Math.min(duration.value, target) : target
  try {
    await SeekPlayer(position.value)
  } catch (e) { emit('toast', String(e), 'error') }
  finally { seeking = false }
}

let scrubbing = false
let justScrubbed = false

function onBarDown(e) {
  if (!started.value || duration.value <= 0) return
  scrubbing = true
  justScrubbed = true
  updateScrub(e)
  window.addEventListener('mousemove', onScrubMove)
  window.addEventListener('mouseup', onScrubUp)
}

function onScrubMove(e) {
  if (scrubbing) updateScrub(e)
}

function onScrubUp() {
  if (!scrubbing) return
  scrubbing = false
  window.removeEventListener('mousemove', onScrubMove)
  window.removeEventListener('mouseup', onScrubUp)
  SeekPlayer(position.value).catch((e2) => emit('toast', String(e2), 'error'))
  setTimeout(() => { justScrubbed = false }, 100)
}

function onBarClick(e) {
  if (justScrubbed || !started.value || duration.value <= 0) return
  const r = e.currentTarget.getBoundingClientRect()
  const ratio = Math.min(1, Math.max(0, (e.clientX - r.left) / r.width))
  position.value = ratio * duration.value
  SeekPlayer(position.value).catch((e2) => emit('toast', String(e2), 'error'))
}

function updateScrub(e) {
  const bar = document.querySelector('.pp-bar')
  if (!bar) return
  const r = bar.getBoundingClientRect()
  const ratio = Math.min(1, Math.max(0, (e.clientX - r.left) / r.width))
  position.value = ratio * duration.value
}

async function onVolume(e) {
  volume.value = Number(e.target.value)
  await SetPlayerVolume(volume.value).catch(() => {})
}

async function onSpeedChange(v) {
  speed.value = v
  await SetPlayerSpeed(v).catch((e) => emit('toast', String(e), 'error'))
}

async function onQualityChange(value) {
  if (!value || value === currentQuality.value || !started.value || busy.value || unmounted) return
  const previous = currentQuality.value
  const target = position.value
  busy.value = true
  try {
    const preview = await PlayVideoQuality(
      props.account.user_id,
      props.account.drive_id,
      props.file.file_id,
      value,
    )
    if (unmounted) return
    currentQuality.value = value
    if (preview && Number.isFinite(preview.duration) && preview.duration > 0) {
      duration.value = preview.duration
    }
    const seekTarget = duration.value > 0 ? Math.min(target, duration.value) : target
    if (seekTarget > 0) {
      await SeekPlayer(seekTarget).catch((e) => emit('toast', String(e), 'error'))
      position.value = seekTarget
    }
    stateErrors = 0
  } catch (e) {
    currentQuality.value = previous
    emit('toast', String(e), 'error')
  } finally {
    busy.value = false
  }
}

async function close() {
  playbackSeq++
  clearProgressSync()
  if (started.value && position.value > 5 && (duration.value <= 0 || position.value < duration.value - 10)) {
    SavePlayCursor(props.account.user_id, props.account.drive_id, props.file.file_id, position.value).catch(() => {})
  } else if (started.value && duration.value > 0 && position.value >= duration.value - 10) {
    SavePlayCursor(props.account.user_id, props.account.drive_id, props.file.file_id, 0).catch(() => {})
  }
  await StopPlayer().catch(() => {})
  emit('close')
}
</script>

<template>
  <teleport to="body">
    <transition name="popover-zoom">
      <div class="player-panel">
        <div class="pp-head">
          <UiIcon name="video" :size="15" style="color:var(--color-primary)" />
          <span class="pp-title" :title="file.name">{{ file.name }}</span>
          <button class="icon-btn pp-x" title="停止并关闭 (Esc)" @click="close"><UiIcon name="close" :size="14" /></button>
        </div>

        <div v-if="loading" class="pp-state"><span class="spin"></span>正在获取播放地址并启动 mpv…</div>
        <div v-else-if="error" class="pp-state">
          <div class="form-error" style="margin:0;flex:1">
            <UiIcon name="warning" :size="14" />
            <span>{{ error }}</span>
            <button class="tbtn xs" style="margin-left:auto" @click="startPlayback">重试</button>
          </div>
        </div>

        <template v-else>
          <div class="pp-state pp-mpv-state"><UiIcon name="external" :size="14" />mpv 播放窗口已启动</div>
          <div class="pp-bar" :class="{ disabled: duration <= 0 }" title="点击/拖拽跳转" @click="onBarClick" @mousedown="onBarDown">
            <div class="pp-bar-fill" :style="{ width: pct + '%' }"></div>
          </div>
          <div class="pp-controls">
            <button class="btn-circle" :title="`快退 ${seekStep} 秒`" @click="seekBy(-seekStep)"><UiIcon name="back" :size="14" /></button>
            <button class="btn-circle pp-play" :title="playing ? '暂停' : '播放'" @click="togglePlay">
              <UiIcon :name="playing ? 'pause' : 'play'" :size="16" />
            </button>
            <button class="btn-circle" :title="`快进 ${seekStep} 秒`" @click="seekBy(seekStep)"><UiIcon name="forward" :size="14" /></button>
            <span class="pp-time">{{ posText }}<template v-if="duration > 0"> / {{ durText }}</template></span>
            <div class="pp-spacer"></div>
            <UiSelect
              v-if="qualityOptions.length > 1"
              :model-value="currentQuality"
              :options="qualityOptions"
              :disabled="busy"
              class="pp-quality-sel"
              @update:modelValue="onQualityChange"
            />
            <UiSelect v-model="speed" :options="speedOptions" @update:modelValue="onSpeedChange" class="pp-speed-sel" />
            <span class="pp-vol">
              <UiIcon name="volume" :size="14" />
              <input type="range" min="0" max="100" :value="volume" @input="onVolume" />
            </span>
          </div>
        </template>
      </div>
    </transition>
  </teleport>
</template>

<style scoped>
.player-panel {
  position: fixed; left: 50%; bottom: 22px; transform: translateX(-50%);
  width: min(540px, calc(100vw - 48px)); z-index: 260;
  background: var(--bg-elevated); border: 1px solid var(--border-light);
  border-radius: var(--radius-lg); box-shadow: var(--shadow-modal);
  padding: 12px 16px 14px;
}
.pp-head { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; color: var(--text-secondary); }
.pp-title { flex: 1; min-width: 0; font-size: 14px; font-weight: 600; color: var(--text-primary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.pp-x { width: 26px; height: 26px; flex-shrink: 0; }
.pp-state { padding: 14px 0 10px; display: flex; align-items: center; gap: 8px; font-size: 13.5px; color: var(--text-secondary); }
.pp-mpv-state { padding-bottom: 12px; }
.pp-bar {
  height: 6px; border-radius: 3px; background: var(--bg-subtle);
  cursor: pointer; overflow: hidden; margin-bottom: 12px;
  transition: height var(--motion-fast) var(--motion-ease);
}
.pp-bar:hover { height: 8px; }
.pp-bar.disabled { cursor: default; opacity: .5; }
.pp-bar-fill { height: 100%; background: var(--color-primary); border-radius: 3px; transition: width .4s linear; }
.pp-bar:hover .pp-bar-fill { box-shadow: 0 0 8px color-mix(in srgb, var(--color-primary) 60%, transparent); }
.pp-controls { display: flex; align-items: center; gap: 8px; }
.pp-play { width: 34px; height: 34px; color: var(--color-primary); background: var(--listselectbg); }
.pp-play:hover { background: color-mix(in srgb, var(--color-primary) 20%, transparent); }
.pp-time { font-size: 13px; color: var(--text-secondary); font-variant-numeric: tabular-nums; margin-left: 4px; }
.pp-spacer { flex: 1; }
.pp-quality-sel { max-width: 112px; }
.pp-quality-sel :deep(.uiselect-btn) { max-width: 112px; }
.pp-vol { display: inline-flex; align-items: center; gap: 6px; color: var(--text-tertiary); }
.pp-vol input[type="range"] {
  -webkit-appearance: none; appearance: none; width: 80px; height: 4px;
  border-radius: 2px; background: var(--bg-subtle); outline: none; cursor: pointer;
}
.pp-vol input[type="range"]::-webkit-slider-thumb {
  -webkit-appearance: none; appearance: none; width: 12px; height: 12px; border-radius: 50%;
  background: var(--color-primary); border: none; cursor: grab;
}
.pp-vol input[type="range"]::-moz-range-thumb {
  width: 12px; height: 12px; border-radius: 50%; background: var(--color-primary); border: none; cursor: grab;
}
</style>
