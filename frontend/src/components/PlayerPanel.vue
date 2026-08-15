<script setup>
// 视频播放控制面板：mpv 以独立窗口播放，本面板为控制器。
// 信息层级参考旧版（标题 / 时间 / 控制），布局重新设计为悬浮式控制条。
// 进度为本地估算（ticker + seek 校正），关闭时保存续播位置。
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import {
  PlayVideo, StopPlayer, PausePlayer, SeekPlayer, SetPlayerVolume, SetPlayerSpeed,
  GetPlayCursor, SavePlayCursor, GetVideoPreview, GetSettings,
} from '../../wailsjs/go/app/App'
import { getPrefs } from '../appearance'
import UiIcon from './UiIcon.vue'

const props = defineProps({
  account: { type: Object, required: true },
  file: { type: Object, required: true },
})
const emit = defineEmits(['close', 'toast'])

const loading = ref(true)
const error = ref('')
const playing = ref(false)
const started = ref(false) // mpv 已成功起播
const position = ref(0) // 本地估算秒数
const duration = ref(0)
const prefs = getPrefs()
const volume = ref(prefs.defaultVolume ?? 50)
const speed = ref(prefs.defaultSpeed || 1)
const seekStep = prefs.seekStep || 10
const busy = ref(false)

const SPEEDS = [0.5, 0.75, 1, 1.25, 1.5, 2]

let ticker = null
onMounted(() => {
  ticker = setInterval(() => {
    if (started.value && playing.value && duration.value > 0) {
      position.value = Math.min(duration.value, position.value + 0.5 * speed.value)
      // 播放到结尾自动收起（本地估算到达时长）
      if (getPrefs().autoCloseOnEnd && position.value >= duration.value) close()
    }
  }, 500)
})
onBeforeUnmount(() => {
  clearInterval(ticker)
  // 卸载兜底：页面切换导致组件销毁时也必须停 mpv 并保存续播位置
  if (started.value) {
    if (position.value > 5 && (duration.value <= 0 || position.value < duration.value - 10)) {
      SavePlayCursor(props.account.user_id, props.account.drive_id, props.file.file_id, position.value).catch(() => {})
    }
    StopPlayer().catch(() => {})
  }
})

function fmt(sec) {
  sec = Math.max(0, Math.floor(sec || 0))
  const h = Math.floor(sec / 3600), m = Math.floor((sec % 3600) / 60), s = sec % 60
  return (h ? h + ':' + String(m).padStart(2, '0') : String(m)) + ':' + String(s).padStart(2, '0')
}
const posText = computed(() => fmt(position.value))
const durText = computed(() => fmt(duration.value))
const pct = computed(() => (duration.value > 0 ? Math.min(100, (position.value / duration.value) * 100) : 0))

onMounted(async () => {
  try {
    // 先取元信息（时长），失败不阻塞起播
    GetVideoPreview(props.account.user_id, props.account.drive_id, props.file.file_id)
      .then((p) => { if (p && p.duration) duration.value = p.duration })
      .catch(() => {})
    await PlayVideo(props.account.user_id, props.account.drive_id, props.file.file_id)
    started.value = true
    playing.value = true
    // 应用默认音量 / 倍速（mpv 启动固定为 50 / 1x）
    if (volume.value !== 50) SetPlayerVolume(volume.value).catch(() => {})
    if (speed.value !== 1) SetPlayerSpeed(speed.value).catch(() => {})
    // 续播（遵循后端设置的断点续播开关）
    const st = await GetSettings().catch(() => null)
    if (!st || st.playbackResume !== false) {
      const cursor = await GetPlayCursor(props.account.user_id, props.account.drive_id, props.file.file_id).catch(() => 0)
      if (cursor && cursor > 5) {
        await SeekPlayer(cursor).catch(() => {})
        position.value = cursor
      }
    }
  } catch (e) {
    error.value = String(e)
  }
  loading.value = false
})

async function togglePlay() {
  if (!started.value || busy.value) return
  busy.value = true
  try {
    await PausePlayer(playing.value)
    playing.value = !playing.value
  } catch (e) { emit('toast', String(e), 'error') }
  busy.value = false
}

let seeking = false
async function seekBy(delta) {
  if (!started.value || seeking) return
  seeking = true
  const target = Math.max(0, position.value + delta)
  position.value = duration.value > 0 ? Math.min(duration.value, target) : target
  try { await SeekPlayer(position.value) } catch (e) { emit('toast', String(e), 'error') }
  seeking = false
}

function onBarClick(e) {
  if (!started.value || duration.value <= 0) return
  const r = e.currentTarget.getBoundingClientRect()
  const ratio = Math.min(1, Math.max(0, (e.clientX - r.left) / r.width))
  position.value = ratio * duration.value
  SeekPlayer(position.value).catch((e2) => emit('toast', String(e2), 'error'))
}

async function onVolume(e) {
  volume.value = Number(e.target.value)
  await SetPlayerVolume(volume.value).catch(() => {})
}

async function cycleSpeed() {
  const i = SPEEDS.indexOf(speed.value)
  speed.value = SPEEDS[(i + 1) % SPEEDS.length]
  await SetPlayerSpeed(speed.value).catch((e) => emit('toast', String(e), 'error'))
}

async function close() {
  // 保存续播位置（开头/结尾附近不记）
  if (started.value && position.value > 5 && (duration.value <= 0 || position.value < duration.value - 10)) {
    SavePlayCursor(props.account.user_id, props.account.drive_id, props.file.file_id, position.value).catch(() => {})
  }
  await StopPlayer().catch(() => {})
  emit('close')
}
</script>

<template>
  <teleport to="body">
    <div class="player-panel">
      <div class="pp-head">
        <UiIcon name="video" :size="15" />
        <span class="pp-title" :title="file.name">{{ file.name }}</span>
        <button class="icon-btn pp-x" title="停止并关闭" @click="close"><UiIcon name="close" :size="14" /></button>
      </div>

      <div v-if="loading" class="pp-state"><span class="spin"></span>正在获取播放地址…</div>
      <div v-else-if="error" class="pp-state"><div class="form-error" style="margin:0;flex:1"><UiIcon name="warning" :size="14" /><span>{{ error }}</span></div></div>

      <template v-else>
        <div class="pp-bar" :class="{ disabled: duration <= 0 }" title="点击跳转" @click="onBarClick">
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
          <button class="tbtn pp-speed" title="切换倍速" @click="cycleSpeed">{{ speed }}x</button>
          <span class="pp-vol">
            <UiIcon name="volume" :size="14" />
            <input type="range" min="0" max="100" :value="volume" @input="onVolume" />
          </span>
        </div>
      </template>
    </div>
  </teleport>
</template>

<style>
.player-panel {
  position: fixed; left: 50%; bottom: 22px; transform: translateX(-50%);
  width: min(520px, calc(100vw - 48px)); z-index: 260;
  background: var(--bg-elevated); border: 1px solid var(--border-light);
  border-radius: var(--radius-lg); box-shadow: var(--shadow-modal);
  padding: 10px 14px 12px; animation: toast-in .2s cubic-bezier(.22, 1, .36, 1);
}
.pp-head { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; color: var(--text-secondary); }
.pp-title { flex: 1; min-width: 0; font-size: 14px; font-weight: 600; color: var(--text-primary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.pp-x { width: 26px; height: 26px; flex-shrink: 0; }
.pp-state { padding: 14px 0 10px; display: flex; align-items: center; gap: 8px; font-size: 13.5px; color: var(--text-secondary); }
.pp-bar {
  height: 5px; border-radius: 3px; background: var(--bg-subtle);
  cursor: pointer; overflow: hidden; margin-bottom: 10px;
}
.pp-bar.disabled { cursor: default; opacity: .5; }
.pp-bar-fill { height: 100%; background: var(--color-primary); border-radius: 3px; transition: width .5s linear; }
.pp-bar:hover .pp-bar-fill { box-shadow: 0 0 6px color-mix(in srgb, var(--color-primary) 60%, transparent); }
.pp-controls { display: flex; align-items: center; gap: 8px; }
.pp-play { width: 32px; height: 32px; color: var(--color-primary); border-color: color-mix(in srgb, var(--color-primary) 40%, transparent); }
.pp-time { font-size: 13px; color: var(--text-secondary); font-variant-numeric: tabular-nums; margin-left: 4px; }
.pp-spacer { flex: 1; }
.pp-speed { min-width: 42px; justify-content: center; font-variant-numeric: tabular-nums; }
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
