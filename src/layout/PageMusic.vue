<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ListMusic, Maximize2, Minus, Music2, Pause, Play, SkipBack, SkipForward, Volume2, VolumeX, X } from 'lucide-vue-next'
import { useAppStore } from '../store'
import type { IPageMusicTrack } from '../store/appstore'
import { fetchMusicMetadata, findActiveLineIndex, type LyricLine, type MusicMetadata } from '../utils/musicMetadata'
import { getProxyUrl, getRawUrl, isLocalProxyUrl } from '../utils/proxyhelper'
import message from '../utils/message'

const appStore = useAppStore()
const audioRef = ref<HTMLAudioElement | null>(null)
const lyricListRef = ref<HTMLElement | null>(null)
const playlist = ref<IPageMusicTrack[]>([])
const currentIndex = ref(0)
const currentTime = ref(0)
const duration = ref(0)
const volume = ref(0.8)
const playing = ref(false)
const loading = ref(false)
const errorText = ref('')
const metadata = ref<MusicMetadata | null>(null)
const metadataLoading = ref(false)
const queueVisible = ref(true)
let loadToken = 0
let metadataToken = 0

const currentTrack = computed(() => playlist.value[currentIndex.value])
const title = computed(() => metadata.value?.title || stripExtension(currentTrack.value?.file_name || '音频预览'))
const artist = computed(() => metadata.value?.artist || '')
const album = computed(() => metadata.value?.album || '')
const coverUrl = computed(() => metadata.value?.cover || currentTrack.value?.thumbnail || '')
const lyricLines = computed<LyricLine[]>(() => metadata.value?.lines || [])
const activeLyricIndex = computed(() => findActiveLineIndex(lyricLines.value, currentTime.value))
const progress = computed(() => duration.value > 0 ? Math.min(100, Math.max(0, currentTime.value / duration.value * 100)) : 0)

function stripExtension(name: string) {
  const index = name.lastIndexOf('.')
  return index > 0 ? name.slice(0, index) : name
}

function formatTime(value: number) {
  if (!Number.isFinite(value) || value < 0) return '00:00'
  const minutes = Math.floor(value / 60).toString().padStart(2, '0')
  const seconds = Math.floor(value % 60).toString().padStart(2, '0')
  return `${minutes}:${seconds}`
}

function trackDurationSeconds(track?: IPageMusicTrack) {
  const value = Number(track?.duration_ms || 0)
  return value > 10000 ? value / 1000 : value
}

async function resolveAudioUrl(track: IPageMusicTrack) {
  if (track.local_url) return track.local_url
  const data = await getRawUrl(track.user_id, track.drive_id, track.file_id, track.encType || '', track.password || '', false, 'audio')
  if (typeof data === 'string') throw new Error(data || '获取音频地址失败')
  if (!data.url) throw new Error('未获取到音频地址')
  if (isLocalProxyUrl(data.url)) return data.url
  return getProxyUrl({
    user_id: track.user_id,
    drive_id: track.drive_id,
    file_id: track.file_id,
    file_size: data.size,
    encType: track.encType || '',
    password: track.password || '',
    quality: 'Origin',
    proxy_kind: 'audio',
    proxy_url: data.url,
    proxy_headers: data.headers ? JSON.stringify(data.headers) : undefined
  })
}

async function loadMetadata(track: IPageMusicTrack) {
  const token = ++metadataToken
  metadataLoading.value = true
  try {
    const result = await fetchMusicMetadata({
      filename: track.file_name,
      durationSec: duration.value || trackDurationSeconds(track) || undefined
    })
    if (token === metadataToken) metadata.value = result
  } catch (error) {
    console.warn('加载音乐元数据失败:', error)
    if (token === metadataToken) metadata.value = null
  } finally {
    if (token === metadataToken) metadataLoading.value = false
  }
}

async function loadTrack(index: number, autoplay = true) {
  if (!playlist.value.length) return
  const nextIndex = Math.min(Math.max(index, 0), playlist.value.length - 1)
  const track = playlist.value[nextIndex]
  const audio = audioRef.value
  if (!audio) return

  const token = ++loadToken
  currentIndex.value = nextIndex
  currentTime.value = 0
  duration.value = trackDurationSeconds(track)
  metadata.value = null
  errorText.value = ''
  loading.value = true
  document.title = track.file_name || '音频预览'

  try {
    const url = await resolveAudioUrl(track)
    if (token !== loadToken) return
    audio.src = url
    audio.load()
    void loadMetadata(track)
    if (autoplay) {
      await audio.play()
    }
  } catch (error: any) {
    if (token !== loadToken) return
    errorText.value = error?.message || String(error)
    playing.value = false
    message.error(`加载失败：${errorText.value}`)
  } finally {
    if (token === loadToken) loading.value = false
  }
}

function togglePlay() {
  const audio = audioRef.value
  if (!audio) return
  if (!audio.src) {
    void loadTrack(currentIndex.value)
    return
  }
  if (audio.paused) void audio.play().catch(() => { playing.value = false })
  else audio.pause()
}

function playPrevious() {
  if (!playlist.value.length) return
  void loadTrack(currentIndex.value > 0 ? currentIndex.value - 1 : playlist.value.length - 1)
}

function playNext() {
  if (!playlist.value.length) return
  void loadTrack(currentIndex.value + 1 < playlist.value.length ? currentIndex.value + 1 : 0)
}

function seekTo(value: number) {
  const audio = audioRef.value
  if (!audio || !Number.isFinite(value)) return
  audio.currentTime = Math.min(Math.max(value, 0), duration.value || value)
  currentTime.value = audio.currentTime
}

function seekFromInput(event: Event) {
  seekTo(Number((event.target as HTMLInputElement).value))
}

function setVolume(event: Event) {
  const nextVolume = Number((event.target as HTMLInputElement).value)
  volume.value = Math.min(1, Math.max(0, nextVolume))
  if (audioRef.value) audioRef.value.volume = volume.value
  localStorage.setItem('mnemo.audio.volume', String(volume.value))
}

function toggleMute() {
  const audio = audioRef.value
  if (!audio) return
  audio.muted = !audio.muted
}

function handleTimeUpdate() {
  const audio = audioRef.value
  if (!audio) return
  currentTime.value = Number.isFinite(audio.currentTime) ? audio.currentTime : 0
  if (Number.isFinite(audio.duration) && audio.duration > 0) duration.value = audio.duration
}

function handleLoadedMetadata() {
  handleTimeUpdate()
  const track = currentTrack.value
  if (track) void loadMetadata(track)
}

function handleClose() {
  audioRef.value?.pause()
  if (window.WebToWindow) window.WebToWindow({ cmd: 'close' })
  else window.close()
}

function handleMinimize() {
  window.WebToWindow?.({ cmd: 'minsize' })
}

function handleMaximize() {
  window.WebToWindow?.({ cmd: 'maxsize' })
}

function handleKeydown(event: KeyboardEvent) {
  const target = event.target as HTMLElement | null
  if (target?.tagName === 'INPUT') return
  if (event.altKey && event.key.toLowerCase() === 'f4') return handleClose()
  if (event.altKey && event.key.toLowerCase() === 'm') return window.WebToWindow?.({ cmd: 'minsize' })
  if (event.altKey && event.key === 'Enter') return window.WebToWindow?.({ cmd: 'maxsize' })
  if (event.code === 'Space') {
    event.preventDefault()
    togglePlay()
  } else if (event.key === 'ArrowLeft') seekTo(currentTime.value - 5)
  else if (event.key === 'ArrowRight') seekTo(currentTime.value + 5)
}

watch(activeLyricIndex, async (index) => {
  await nextTick()
  if (index < 0) return
  lyricListRef.value?.querySelector<HTMLElement>(`[data-lyric-index="${index}"]`)?.scrollIntoView({ block: 'center', behavior: 'smooth' })
})

onMounted(() => {
  const page = appStore.pageMusic
  if (!page) {
    errorText.value = '未提供音频播放参数'
    return
  }
  playlist.value = page.playlist?.length ? [...page.playlist] : [{
    user_id: page.user_id,
    drive_id: page.drive_id,
    file_id: page.file_id,
    parent_file_id: page.parent_file_id,
    file_name: page.file_name,
    encType: page.encType,
    password: page.password
  }]
  currentIndex.value = Math.max(0, playlist.value.findIndex((item) => item.file_id === page.file_id))
  volume.value = Math.min(1, Math.max(0, Number(localStorage.getItem('mnemo.audio.volume') || 0.8)))
  if (audioRef.value) audioRef.value.volume = volume.value
  window.addEventListener('keydown', handleKeydown, true)
  void loadTrack(currentIndex.value)
})

onBeforeUnmount(() => {
  loadToken++
  metadataToken++
  window.removeEventListener('keydown', handleKeydown, true)
  audioRef.value?.pause()
})
</script>

<template>
  <div class="audio-preview">
    <header class="titlebar q-electron-drag">
      <div class="window-actions q-electron-no-drag">
        <button type="button" title="关闭" @click="handleClose"><X :size="15" /></button>
        <button type="button" title="最小化" @click="handleMinimize"><Minus :size="15" /></button>
        <button type="button" title="最大化" @click="handleMaximize"><Maximize2 :size="14" /></button>
      </div>
      <span class="titlebar-name">{{ currentTrack?.file_name || '音频预览' }}</span>
      <button class="queue-toggle q-electron-no-drag" type="button" title="播放列表" @click="queueVisible = !queueVisible"><ListMusic :size="18" /></button>
    </header>

    <main :class="['player-layout', { 'queue-hidden': !queueVisible }]">
      <section class="now-playing">
        <div class="cover">
          <img v-if="coverUrl" :src="coverUrl" alt="" />
          <Music2 v-else :size="72" :stroke-width="1.2" />
        </div>
        <div class="track-info">
          <h1>{{ title }}</h1>
          <p>{{ [artist, album].filter(Boolean).join(' · ') || currentTrack?.file_name }}</p>
        </div>

        <div ref="lyricListRef" class="lyrics" aria-live="polite">
          <div v-if="metadataLoading && !lyricLines.length" class="empty-state">正在匹配歌词...</div>
          <div v-else-if="!lyricLines.length" class="empty-state">暂无歌词</div>
          <button
            v-for="(line, index) in lyricLines"
            :key="`${line.time}-${index}`"
            :data-lyric-index="index"
            :class="['lyric-line', { active: index === activeLyricIndex }]"
            type="button"
            @click="seekTo(line.time)"
          >{{ line.text }}</button>
        </div>

        <div v-if="errorText" class="error-text">{{ errorText }}</div>
        <div class="progress-row">
          <span>{{ formatTime(currentTime) }}</span>
          <input :max="duration || 0" min="0" step="0.1" type="range" :value="currentTime" :style="{ '--progress': `${progress}%` }" @input="seekFromInput" />
          <span>{{ formatTime(duration) }}</span>
        </div>
        <div class="controls">
          <button type="button" title="上一首" @click="playPrevious"><SkipBack :size="23" /></button>
          <button class="play-button" type="button" :title="playing ? '暂停' : '播放'" :disabled="loading" @click="togglePlay">
            <Pause v-if="playing" :size="26" fill="currentColor" />
            <Play v-else :size="26" fill="currentColor" />
          </button>
          <button type="button" title="下一首" @click="playNext"><SkipForward :size="23" /></button>
          <div class="volume-control">
            <button type="button" title="静音" @click="toggleMute">
              <VolumeX v-if="audioRef?.muted || volume === 0" :size="19" />
              <Volume2 v-else :size="19" />
            </button>
            <input max="1" min="0" step="0.01" type="range" :value="volume" @input="setVolume" />
          </div>
        </div>
      </section>

      <aside v-if="queueVisible" class="queue">
        <div class="queue-heading">播放列表 <span>{{ playlist.length }}</span></div>
        <button
          v-for="(track, index) in playlist"
          :key="`${track.drive_id}:${track.file_id}`"
          :class="['queue-item', { active: index === currentIndex }]"
          type="button"
          @click="loadTrack(index)"
        >
          <span class="queue-index">{{ String(index + 1).padStart(2, '0') }}</span>
          <span class="queue-name">{{ stripExtension(track.file_name) }}</span>
          <span class="queue-duration">{{ track.duration_ms ? formatTime(trackDurationSeconds(track)) : '' }}</span>
        </button>
      </aside>
    </main>

    <audio ref="audioRef" preload="metadata" @ended="playNext" @error="errorText = '音频播放失败'" @loadedmetadata="handleLoadedMetadata" @pause="playing = false" @play="playing = true" @timeupdate="handleTimeUpdate" />
  </div>
</template>

<style scoped>
.audio-preview {
  display: flex;
  width: 100vw;
  height: 100vh;
  flex-direction: column;
  overflow: hidden;
  background: #17191c;
  color: #f4f5f6;
}

.titlebar {
  display: grid;
  height: 42px;
  flex: 0 0 42px;
  grid-template-columns: 132px minmax(0, 1fr) 132px;
  align-items: center;
  border-bottom: 1px solid #30343a;
  background: #202328;
}

.window-actions {
  display: flex;
  gap: 4px;
  padding-left: 8px;
}

button {
  border: 0;
  color: inherit;
  cursor: pointer;
  font: inherit;
}

.window-actions button,
.queue-toggle,
.controls > button,
.volume-control button {
  display: inline-flex;
  width: 32px;
  height: 32px;
  align-items: center;
  justify-content: center;
  background: transparent;
}

.window-actions button:hover,
.queue-toggle:hover,
.controls button:hover {
  background: #343940;
}

.titlebar-name {
  overflow: hidden;
  color: #c8cbd0;
  font-size: 12px;
  text-align: center;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.queue-toggle {
  justify-self: end;
  margin-right: 8px;
}

.player-layout {
  display: grid;
  min-height: 0;
  flex: 1;
  grid-template-columns: minmax(0, 1fr) minmax(240px, 30%);
}

.player-layout.queue-hidden {
  grid-template-columns: 1fr;
}

.now-playing {
  display: grid;
  min-width: 0;
  min-height: 0;
  padding: 28px clamp(24px, 5vw, 72px) 24px;
  grid-template-columns: 136px minmax(0, 1fr);
  grid-template-rows: auto minmax(120px, 1fr) auto auto auto;
  gap: 0 24px;
}

.cover {
  display: flex;
  width: 136px;
  aspect-ratio: 1;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  border: 1px solid #3b4047;
  border-radius: 6px;
  background: #25292e;
  color: #747b84;
}

.cover img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.track-info {
  min-width: 0;
  align-self: center;
}

.track-info h1 {
  overflow: hidden;
  margin: 0 0 8px;
  font-size: 24px;
  font-weight: 650;
  letter-spacing: 0;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.track-info p {
  overflow: hidden;
  margin: 0;
  color: #9ba1a9;
  font-size: 14px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.lyrics {
  min-height: 0;
  margin-top: 24px;
  grid-column: 1 / -1;
  overflow: auto;
  scrollbar-width: thin;
  mask-image: linear-gradient(transparent, #000 8%, #000 92%, transparent);
}

.lyric-line {
  display: block;
  width: 100%;
  padding: 9px 12px;
  background: transparent;
  color: #7f868e;
  font-size: 16px;
  line-height: 1.45;
  text-align: center;
  transition: color .18s ease, transform .18s ease;
}

.lyric-line:hover {
  color: #c9cdd2;
}

.lyric-line.active {
  color: #fff;
  font-weight: 600;
  transform: scale(1.04);
}

.empty-state {
  display: flex;
  height: 100%;
  align-items: center;
  justify-content: center;
  color: #777e87;
  font-size: 14px;
}

.error-text {
  margin-top: 8px;
  grid-column: 1 / -1;
  color: #ff8b82;
  font-size: 13px;
  text-align: center;
}

.progress-row {
  display: grid;
  margin-top: 12px;
  grid-column: 1 / -1;
  grid-template-columns: 42px minmax(0, 1fr) 42px;
  align-items: center;
  gap: 10px;
  color: #90969e;
  font-size: 11px;
}

input[type='range'] {
  width: 100%;
  accent-color: #e6e8eb;
}

.controls {
  position: relative;
  display: flex;
  height: 62px;
  grid-column: 1 / -1;
  align-items: center;
  justify-content: center;
  gap: 8px;
}

.controls .play-button {
  width: 46px;
  height: 46px;
  border-radius: 50%;
  background: #f4f5f6;
  color: #17191c;
}

.controls .play-button:hover {
  background: #fff;
}

.controls button:disabled {
  cursor: wait;
  opacity: .5;
}

.volume-control {
  position: absolute;
  right: 0;
  display: flex;
  width: 130px;
  align-items: center;
  gap: 4px;
}

.volume-control input {
  min-width: 0;
}

.queue {
  min-width: 0;
  overflow: auto;
  border-left: 1px solid #30343a;
  background: #1d2024;
}

.queue-heading {
  position: sticky;
  top: 0;
  z-index: 1;
  padding: 18px 16px 12px;
  background: #1d2024;
  color: #e1e3e6;
  font-size: 14px;
  font-weight: 650;
}

.queue-heading span {
  margin-left: 5px;
  color: #777e87;
  font-size: 12px;
  font-weight: 400;
}

.queue-item {
  display: grid;
  width: 100%;
  min-height: 46px;
  padding: 8px 14px;
  grid-template-columns: 28px minmax(0, 1fr) auto;
  align-items: center;
  gap: 8px;
  background: transparent;
  color: #a8adb4;
  text-align: left;
}

.queue-item:hover {
  background: #292d32;
}

.queue-item.active {
  background: #30353b;
  color: #fff;
}

.queue-index,
.queue-duration {
  color: #737a83;
  font-size: 11px;
}

.queue-name {
  overflow: hidden;
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 720px) {
  .player-layout {
    grid-template-columns: 1fr;
  }

  .now-playing {
    padding: 20px 18px 14px;
    grid-template-columns: 92px minmax(0, 1fr);
  }

  .cover {
    width: 92px;
  }

  .track-info h1 {
    font-size: 19px;
  }

  .queue {
    position: absolute;
    inset: 42px 0 0 28%;
    z-index: 3;
    border-left: 1px solid #30343a;
    box-shadow: -12px 0 32px rgba(0, 0, 0, .35);
  }

  .volume-control {
    display: none;
  }
}
</style>
