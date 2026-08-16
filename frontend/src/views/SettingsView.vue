<script setup>
// 设置页：左侧导航 + 右侧平面行式布局，极简干净无冗余说明
import { ref, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { GetSettings, SaveSettings, PickDirectory, RevealInFolder } from '../api'
import { getPrefs, setPref } from '../appearance'
import SegTabs from '../components/SegTabs.vue'
import UiIcon from '../components/UiIcon.vue'
import UiSelect from '../components/UiSelect.vue'

const emit = defineEmits(['toast', 'theme'])

const defaults = {
  theme: 'system',
  defaultTab: 'pan',
  proxy: '',
  downloadDir: '',
  maxConcurrentDownloads: 3,
  maxDownloadSpeed: 0,
  maxUploadSpeed: 0,
  autoUpdate: true,
  confirmUpdate: true,
  playbackResume: true,
  keepTasks: true,
}

const settings = ref({ ...defaults })
const loaded = ref(false)
const saving = ref(false)
let pendingSave = false
const bodyEl = ref(null)
const activeNav = ref('general')

const groups = [
  { id: 'general', label: '通用', icon: 'settings' },
  { id: 'pan', label: '网盘', icon: 'cloud' },
  { id: 'transfer', label: '传输', icon: 'download' },
  { id: 'player', label: '播放器', icon: 'play' },
  { id: 'network', label: '网络', icon: 'cloud-down' },
  { id: 'about', label: '关于', icon: 'info' },
]

// 纯前端偏好
const prefs = ref(getPrefs())
function onPref(key, value) {
  prefs.value = { ...prefs.value, [key]: value }
  setPref(key, value)
}

let testAudio = null
function playTestSound() {
  try {
    if (!testAudio) {
      testAudio = new Audio(new URL('../assets/audio/download_finished.mp3', import.meta.url).href)
    }
    testAudio.currentTime = 0
    testAudio.play().then(() => emit('toast', '正在试听提示音', 'success')).catch(() => {})
  } catch { /* 忽略 */ }
}

const speedOptions = [
  { value: 0.5, label: '0.5x' },
  { value: 0.75, label: '0.75x' },
  { value: 1, label: '1.0x' },
  { value: 1.25, label: '1.25x' },
  { value: 1.5, label: '1.5x' },
  { value: 2, label: '2.0x' },
]

const seekStepOptions = [
  { value: 5, label: '5 秒' },
  { value: 10, label: '10 秒' },
  { value: 15, label: '15 秒' },
  { value: 30, label: '30 秒' },
]

const tabOptions = [
  { key: 'pan', label: '网盘' },
  { key: 'transfer', label: '传输' },
  { key: 'sync', label: '同步' },
  { key: 'share', label: '分享' },
]

const sortKeyOptions = [
  { value: 'name', label: '名称' },
  { value: 'time', label: '修改时间' },
  { value: 'size', label: '大小' },
]

const hwDecodeOptions = [
  { value: 'auto', label: '自动' },
  { value: 'd3d11va', label: 'D3D11VA 硬解' },
  { value: 'no', label: '软解' },
]

onMounted(async () => {
  try {
    const s = (await GetSettings()) || {}
    settings.value = { ...defaults, ...s }
  } catch {
    settings.value = { ...defaults }
  }
  loaded.value = true
  bodyEl.value?.addEventListener('scroll', onScroll, { passive: true })
})

onBeforeUnmount(() => {
  bodyEl.value?.removeEventListener('scroll', onScroll)
})

async function scrollTo(id) {
  activeNav.value = id
  await nextTick()
  document.getElementById('sg-' + id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

function onScroll() {
  const root = bodyEl.value
  if (!root) return
  let current = groups[0].id
  for (const g of groups) {
    const el = document.getElementById('sg-' + g.id)
    if (el && el.offsetTop <= root.scrollTop + 96) current = g.id
  }
  activeNav.value = current
}

async function save(silent) {
  if (!loaded.value) return
  if (saving.value) { pendingSave = true; return }
  saving.value = true
  try {
    const s = settings.value
    s.maxConcurrentDownloads = Math.min(8, Math.max(1, Number(s.maxConcurrentDownloads) || 1))
    s.maxDownloadSpeed = Math.max(0, Number(s.maxDownloadSpeed) || 0)
    s.maxUploadSpeed = Math.max(0, Number(s.maxUploadSpeed) || 0)
    await SaveSettings(s)
    if (!silent) emit('toast', '设置已保存', 'success')
  } catch (e) {
    emit('toast', '保存失败: ' + String(e), 'error')
  } finally {
    saving.value = false
    if (pendingSave) { pendingSave = false; save(true) }
  }
}

function toggle(key) {
  settings.value[key] = !settings.value[key]
  save(true)
}

function onThemeChange(theme) {
  settings.value.theme = theme
  emit('theme', theme)
  save(true)
}

function onInputCommit() {
  save(true)
}

async function pickDownloadDir() {
  let dir
  try { dir = await PickDirectory('选择下载文件夹', settings.value.downloadDir || '') } catch { return }
  if (!dir) return
  settings.value.downloadDir = dir
  save(true)
}

function openDownloadDir() {
  if (settings.value.downloadDir) {
    RevealInFolder(settings.value.downloadDir)
  } else {
    emit('toast', '当前使用系统默认下载目录', 'info')
  }
}

function setDownloadLimitPreset(kb) {
  settings.value.maxDownloadSpeed = kb
  save(true)
}
function setUploadLimitPreset(kb) {
  settings.value.maxUploadSpeed = kb
  save(true)
}
</script>

<template>
  <div class="settings-layout">
    <aside class="settings-nav">
      <button
        v-for="g in groups"
        :key="g.id"
        type="button"
        class="sn-item"
        :class="{ active: activeNav === g.id }"
        @click="scrollTo(g.id)"
      >
        <UiIcon :name="g.icon" :size="16" />
        <span>{{ g.label }}</span>
      </button>
    </aside>

    <div class="settings-body" ref="bodyEl">
      <div class="settings-column">
        <!-- 1. 通用 -->
        <section class="settings-group" id="sg-general">
          <header class="sg-heading"><h2>通用</h2></header>

          <div class="sg-row">
            <span class="sg-label">外观主题</span>
            <div class="sg-control">
              <div class="theme-cards">
                <button
                  class="theme-card"
                  :class="{ active: settings.theme === 'system' || !settings.theme }"
                  @click="onThemeChange('system')"
                >
                  <UiIcon name="globe" :size="15" />
                  <span>跟随系统</span>
                </button>
                <button
                  class="theme-card"
                  :class="{ active: settings.theme === 'light' }"
                  @click="onThemeChange('light')"
                >
                  <UiIcon name="sun" :size="15" />
                  <span>浅色</span>
                </button>
                <button
                  class="theme-card"
                  :class="{ active: settings.theme === 'dark' }"
                  @click="onThemeChange('dark')"
                >
                  <UiIcon name="moon" :size="15" />
                  <span>深色</span>
                </button>
              </div>
            </div>
          </div>

          <div class="sg-row">
            <span class="sg-label">默认启动页面</span>
            <div class="sg-control">
              <SegTabs
                :options="tabOptions"
                :modelValue="settings.defaultTab || 'pan'"
                @update:modelValue="(v) => { settings.defaultTab = v; save(true) }"
              />
            </div>
          </div>

          <div class="sg-row">
            <span class="sg-label">下载完成提示音</span>
            <div class="sg-control" style="display:flex;gap:10px;align-items:center">
              <div class="switch" :class="{ on: prefs.downloadSound }" @click="onPref('downloadSound', !prefs.downloadSound)"></div>
              <button v-if="prefs.downloadSound" class="tbtn xs" @click="playTestSound">
                <UiIcon name="play" :size="12" />试听
              </button>
            </div>
          </div>

          <div class="sg-row">
            <span class="sg-label">自动检查更新</span>
            <div class="sg-control">
              <div class="switch" :class="{ on: settings.autoUpdate }" @click="toggle('autoUpdate')"></div>
            </div>
          </div>

          <div class="sg-row">
            <span class="sg-label">更新前确认</span>
            <div class="sg-control">
              <div class="switch" :class="{ on: settings.confirmUpdate }" @click="toggle('confirmUpdate')"></div>
            </div>
          </div>
        </section>

        <!-- 2. 网盘 -->
        <section class="settings-group" id="sg-pan">
          <header class="sg-heading"><h2>网盘</h2></header>

          <div class="sg-row">
            <span class="sg-label">默认视图</span>
            <div class="sg-control">
              <SegTabs
                :options="[{ key: 'list', label: '列表' }, { key: 'grid', label: '网格' }]"
                :modelValue="prefs.viewMode"
                @update:modelValue="(v) => onPref('viewMode', v)"
              />
            </div>
          </div>

          <div class="sg-row">
            <span class="sg-label">默认排序</span>
            <div class="sg-control" style="display:flex;gap:8px;align-items:center">
              <UiSelect
                :modelValue="prefs.defaultSortKey || 'name'"
                :options="sortKeyOptions"
                style="width:120px"
                @update:modelValue="(v) => onPref('defaultSortKey', v)"
              />
              <SegTabs
                :options="[{ key: 'asc', label: '升序' }, { key: 'desc', label: '降序' }]"
                :modelValue="prefs.defaultSortAsc ? 'asc' : 'desc'"
                @update:modelValue="(v) => onPref('defaultSortAsc', v === 'asc')"
              />
            </div>
          </div>

          <div class="sg-row">
            <span class="sg-label">目录树悬停预览</span>
            <div class="sg-control">
              <div class="switch" :class="{ on: prefs.hoverPreview }" @click="onPref('hoverPreview', !prefs.hoverPreview)"></div>
            </div>
          </div>
        </section>

        <!-- 3. 传输 -->
        <section class="settings-group" id="sg-transfer">
          <header class="sg-heading"><h2>传输</h2></header>

          <div class="sg-row">
            <span class="sg-label">下载保存目录</span>
            <div class="sg-control" style="display:flex;gap:6px;align-items:center;flex-wrap:wrap">
              <input
                class="input"
                style="min-width:240px;flex:1"
                v-model="settings.downloadDir"
                placeholder="系统默认下载目录"
                @blur="onInputCommit"
                @keydown.enter="onInputCommit"
              />
              <button class="btn sm" @click="pickDownloadDir">选择</button>
              <button v-if="settings.downloadDir" class="btn sm" @click="openDownloadDir">打开</button>
            </div>
          </div>

          <div class="sg-row">
            <span class="sg-label">最大并发下载数</span>
            <div class="sg-control" style="display:flex;gap:8px;align-items:center">
              <input
                class="input input-sm"
                type="number"
                min="1"
                max="8"
                v-model.number="settings.maxConcurrentDownloads"
                @blur="onInputCommit"
                @keydown.enter="onInputCommit"
              />
            </div>
          </div>

          <div class="sg-row">
            <span class="sg-label">下载限速 (KB/s)</span>
            <div class="sg-control" style="display:flex;gap:8px;align-items:center;flex-wrap:wrap">
              <input
                class="input input-sm"
                type="number"
                min="0"
                v-model.number="settings.maxDownloadSpeed"
                placeholder="0 为不限速"
                @blur="onInputCommit"
                @keydown.enter="onInputCommit"
              />
              <div class="preset-pills">
                <button class="pill-btn" :class="{ active: !settings.maxDownloadSpeed }" @click="setDownloadLimitPreset(0)">不限速</button>
                <button class="pill-btn" :class="{ active: settings.maxDownloadSpeed === 5120 }" @click="setDownloadLimitPreset(5120)">5M</button>
                <button class="pill-btn" :class="{ active: settings.maxDownloadSpeed === 10240 }" @click="setDownloadLimitPreset(10240)">10M</button>
                <button class="pill-btn" :class="{ active: settings.maxDownloadSpeed === 20480 }" @click="setDownloadLimitPreset(20480)">20M</button>
              </div>
            </div>
          </div>

          <div class="sg-row">
            <span class="sg-label">上传限速 (KB/s)</span>
            <div class="sg-control" style="display:flex;gap:8px;align-items:center;flex-wrap:wrap">
              <input
                class="input input-sm"
                type="number"
                min="0"
                v-model.number="settings.maxUploadSpeed"
                placeholder="0 为不限速"
                @blur="onInputCommit"
                @keydown.enter="onInputCommit"
              />
              <div class="preset-pills">
                <button class="pill-btn" :class="{ active: !settings.maxUploadSpeed }" @click="setUploadLimitPreset(0)">不限速</button>
                <button class="pill-btn" :class="{ active: settings.maxUploadSpeed === 2048 }" @click="setUploadLimitPreset(2048)">2M</button>
                <button class="pill-btn" :class="{ active: settings.maxUploadSpeed === 5120 }" @click="setUploadLimitPreset(5120)">5M</button>
              </div>
            </div>
          </div>

          <div class="sg-row">
            <span class="sg-label">保留传输记录</span>
            <div class="sg-control">
              <div class="switch" :class="{ on: settings.keepTasks }" @click="toggle('keepTasks')"></div>
            </div>
          </div>

          <div class="sg-row">
            <span class="sg-label">桌面传输悬浮球</span>
            <div class="sg-control">
              <div class="switch" :class="{ on: prefs.transferBall }" @click="onPref('transferBall', !prefs.transferBall)"></div>
            </div>
          </div>
        </section>

        <!-- 4. 播放器 -->
        <section class="settings-group" id="sg-player">
          <header class="sg-heading"><h2>播放器</h2></header>

          <div class="sg-row">
            <span class="sg-label">断点续播</span>
            <div class="sg-control">
              <div class="switch" :class="{ on: settings.playbackResume }" @click="toggle('playbackResume')"></div>
            </div>
          </div>

          <div class="sg-row">
            <span class="sg-label">默认播放音量</span>
            <div class="sg-control" style="display:flex;gap:10px;align-items:center">
              <input
                type="range"
                min="0"
                max="100"
                :value="prefs.defaultVolume"
                @input="(e) => onPref('defaultVolume', Number(e.target.value))"
                style="width:140px;cursor:pointer"
              />
              <span class="sg-value" style="font-variant-numeric:tabular-nums">{{ prefs.defaultVolume }}%</span>
            </div>
          </div>

          <div class="sg-row">
            <span class="sg-label">默认播放倍速</span>
            <div class="sg-control">
              <UiSelect
                :modelValue="prefs.defaultSpeed"
                :options="speedOptions"
                style="width:130px"
                @update:modelValue="(v) => onPref('defaultSpeed', Number(v))"
              />
            </div>
          </div>

          <div class="sg-row">
            <span class="sg-label">快进 / 快退步长</span>
            <div class="sg-control">
              <UiSelect
                :modelValue="prefs.seekStep || 10"
                :options="seekStepOptions"
                style="width:130px"
                @update:modelValue="(v) => onPref('seekStep', Number(v))"
              />
            </div>
          </div>

          <div class="sg-row">
            <span class="sg-label">播放完自动收起</span>
            <div class="sg-control">
              <div class="switch" :class="{ on: prefs.autoCloseOnEnd }" @click="onPref('autoCloseOnEnd', !prefs.autoCloseOnEnd)"></div>
            </div>
          </div>

          <div class="sg-row">
            <span class="sg-label">硬件解码加速</span>
            <div class="sg-control">
              <UiSelect
                :modelValue="prefs.hardwareDecode || 'auto'"
                :options="hwDecodeOptions"
                style="width:150px"
                @update:modelValue="(v) => onPref('hardwareDecode', v)"
              />
            </div>
          </div>

          <div class="sg-row">
            <span class="sg-label">自动挂载同名字幕</span>
            <div class="sg-control">
              <div class="switch" :class="{ on: prefs.autoLoadSubtitles }" @click="onPref('autoLoadSubtitles', !prefs.autoLoadSubtitles)"></div>
            </div>
          </div>
        </section>

        <!-- 5. 网络 -->
        <section class="settings-group" id="sg-network">
          <header class="sg-heading"><h2>网络</h2></header>

          <div class="sg-row">
            <span class="sg-label">代理服务器</span>
            <div class="sg-control">
              <input
                class="input"
                style="width:min(100%, 300px)"
                v-model="settings.proxy"
                placeholder="http://127.0.0.1:7890"
                @blur="onInputCommit"
                @keydown.enter="onInputCommit"
              />
            </div>
          </div>
        </section>

        <!-- 6. 关于 -->
        <section class="settings-group" id="sg-about">
          <header class="sg-heading"><h2>关于</h2></header>

          <div class="sg-row">
            <span class="sg-label">应用版本</span>
            <div class="sg-control"><span class="sg-value" style="font-weight:600">Mnemo-Go 0.1.0-preview</span></div>
          </div>
          <div class="sg-row">
            <span class="sg-label">技术架构</span>
            <div class="sg-control"><span class="sg-value">Go 1.22+ / Wails v2 / Vue 3</span></div>
          </div>
          <div class="sg-row">
            <span class="sg-label">开源协议</span>
            <div class="sg-control"><span class="sg-value">GNU GPL-3.0</span></div>
          </div>
        </section>

        <div class="settings-foot">
          <span style="font-size:12px;color:var(--text-tertiary)">修改即时自动保存</span>
          <button class="btn primary sm" :disabled="saving" @click="save()">
            <span v-if="saving" class="spin spin-on-primary"></span>
            <span>{{ saving ? '保存中…' : '保存设置' }}</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.theme-cards {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.theme-card {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 5px 12px;
  border-radius: var(--radius-sm);
  border: 1px solid var(--control-border);
  background: var(--control-bg);
  color: var(--text-secondary);
  font-size: 13px;
  cursor: pointer;
  transition: all var(--motion-fast) var(--motion-ease);
}
.theme-card:hover {
  border-color: var(--color-primary);
  color: var(--text-primary);
}
.theme-card.active {
  border-color: var(--color-primary);
  background: var(--listselectbg);
  color: var(--color-primary);
  font-weight: 600;
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--color-primary) 30%, transparent);
}

.preset-pills {
  display: inline-flex;
  gap: 4px;
}
.pill-btn {
  padding: 2px 7px;
  border-radius: var(--radius-full);
  border: 1px solid var(--border-light);
  background: var(--bg-subtle);
  font-size: 11.5px;
  color: var(--text-secondary);
  cursor: pointer;
  transition: all var(--motion-fast) var(--motion-ease);
}
.pill-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
.pill-btn.active {
  background: var(--listselectbg);
  color: var(--color-primary);
  border-color: var(--color-primary);
  font-weight: 600;
}
</style>
