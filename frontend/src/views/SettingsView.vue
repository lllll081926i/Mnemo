<script setup>
// 设置页：左侧图标导航（流动色块选中）+ 右侧分节滚动，sg-row 平面行式布局（对齐旧版 SettingUI 风格）
import { ref, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { GetSettings, SaveSettings, PickDirectory } from '../api'
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
  keepTasks: true
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
  { id: 'about', label: '关于', icon: 'info' }
]

// 纯前端偏好（localStorage，即时生效）
const prefs = ref(getPrefs())
function onPref(key, value) {
  prefs.value = { ...prefs.value, [key]: value }
  setPref(key, value)
}
const speedOptions = [
  { value: '0.5', label: '0.5x' },
  { value: '0.75', label: '0.75x' },
  { value: '1', label: '1x' },
  { value: '1.25', label: '1.25x' },
  { value: '1.5', label: '1.5x' },
  { value: '2', label: '2x' },
]
const seekStepOptions = [
  { value: '5', label: '5 秒' },
  { value: '10', label: '10 秒' },
  { value: '15', label: '15 秒' },
  { value: '30', label: '30 秒' },
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
  if (saving.value) { pendingSave = true; return } // 在途保存完成后用最新值重存，避免静默丢失
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

function onSelect(key, value) {
  settings.value[key] = value
  save(true)
}

function onInputCommit() {
  save(true)
}

// 选择本地下载目录（原生目录选择对话框）
async function pickDownloadDir() {
  let dir
  try { dir = await PickDirectory('选择下载文件夹', settings.value.downloadDir || '') } catch { return }
  if (!dir) return
  settings.value.downloadDir = dir
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
        <!-- 通用 -->
        <section class="settings-group" id="sg-general">
          <header class="sg-heading"><h2>通用</h2></header>

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

        <!-- 网盘 -->
        <section class="settings-group" id="sg-pan">
          <header class="sg-heading"><h2>网盘</h2></header>

          <div class="sg-row">
            <span class="sg-label">默认文件视图</span>
            <div class="sg-control">
              <SegTabs
                :options="[{ key: 'list', label: '列表' }, { key: 'grid', label: '网格' }]"
                :modelValue="prefs.viewMode"
                @update:modelValue="(v) => onPref('viewMode', v)"
              />
            </div>
          </div>

          <div class="sg-row">
            <span class="sg-label">文件夹悬停预览<span class="sg-hint">目录树悬停时预览内容</span></span>
            <div class="sg-control">
              <div class="switch" :class="{ on: prefs.hoverPreview }" @click="onPref('hoverPreview', !prefs.hoverPreview)"></div>
            </div>
          </div>
        </section>

        <!-- 传输 -->
        <section class="settings-group" id="sg-transfer">
          <header class="sg-heading"><h2>传输</h2></header>

          <div class="sg-row">
            <span class="sg-label">下载目录<span class="sg-hint">留空自动使用系统下载目录</span></span>
            <div class="sg-control" style="display:flex;gap:6px;align-items:center">
              <input
                class="input"
                style="flex:1"
                v-model="settings.downloadDir"
                placeholder="系统下载目录（自动检测）"
                @blur="onInputCommit"
                @keydown.enter="onInputCommit"
              />
              <button class="btn sm" @click="pickDownloadDir">选择目录</button>
            </div>
          </div>

          <div class="sg-row">
            <span class="sg-label">最大并发下载数<span class="sg-hint">1 - 8</span></span>
            <div class="sg-control">
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
            <span class="sg-label">下载限速<span class="sg-hint">KB/s，0 不限</span></span>
            <div class="sg-control">
              <input
                class="input input-sm"
                type="number"
                min="0"
                v-model.number="settings.maxDownloadSpeed"
                @blur="onInputCommit"
                @keydown.enter="onInputCommit"
              />
            </div>
          </div>

          <div class="sg-row">
            <span class="sg-label">上传限速<span class="sg-hint">KB/s，0 不限</span></span>
            <div class="sg-control">
              <input
                class="input input-sm"
                type="number"
                min="0"
                v-model.number="settings.maxUploadSpeed"
                @blur="onInputCommit"
                @keydown.enter="onInputCommit"
              />
            </div>
          </div>

          <div class="sg-row">
            <span class="sg-label">保留传输记录</span>
            <div class="sg-control">
              <div class="switch" :class="{ on: settings.keepTasks }" @click="toggle('keepTasks')"></div>
            </div>
          </div>

          <div class="sg-row">
            <span class="sg-label">传输悬浮球<span class="sg-hint">传输时右下角显示速度球</span></span>
            <div class="sg-control">
              <div class="switch" :class="{ on: prefs.transferBall }" @click="onPref('transferBall', !prefs.transferBall)"></div>
            </div>
          </div>
        </section>

        <!-- 播放器 (mpv) -->
        <section class="settings-group" id="sg-player">
          <header class="sg-heading"><h2>播放器（mpv）</h2></header>

          <div class="sg-row">
            <span class="sg-label">断点续播<span class="sg-hint">重新打开视频时从上次位置继续</span></span>
            <div class="sg-control">
              <div class="switch" :class="{ on: settings.playbackResume }" @click="toggle('playbackResume')"></div>
            </div>
          </div>

          <div class="sg-row">
            <span class="sg-label">默认音量<span class="sg-hint">0 - 100</span></span>
            <div class="sg-control">
              <input
                class="input input-sm"
                type="number"
                min="0"
                max="100"
                :value="prefs.defaultVolume"
                @change="(e) => onPref('defaultVolume', Math.min(100, Math.max(0, Number(e.target.value) || 0)))"
              />
            </div>
          </div>

          <div class="sg-row">
            <span class="sg-label">默认倍速</span>
            <div class="sg-control">
              <UiSelect
                :modelValue="String(prefs.defaultSpeed)"
                :options="speedOptions"
                style="width:120px"
                @update:modelValue="(v) => onPref('defaultSpeed', Number(v))"
              />
            </div>
          </div>

          <div class="sg-row">
            <span class="sg-label">快进 / 快退步长</span>
            <div class="sg-control">
              <UiSelect
                :modelValue="String(prefs.seekStep || 10)"
                :options="seekStepOptions"
                style="width:120px"
                @update:modelValue="(v) => onPref('seekStep', Number(v))"
              />
            </div>
          </div>

          <div class="sg-row">
            <span class="sg-label">播放完自动关闭<span class="sg-hint">播放到结尾后收起控制条</span></span>
            <div class="sg-control">
              <div class="switch" :class="{ on: prefs.autoCloseOnEnd }" @click="onPref('autoCloseOnEnd', !prefs.autoCloseOnEnd)"></div>
            </div>
          </div>

          <div class="sg-row">
            <span class="sg-label">高级配置<span class="sg-hint">在数据目录 mpv-config/ 下放 mpv.conf 生效</span></span>
            <div class="sg-control"><span class="sg-value">mpv --config-dir 已挂载</span></div>
          </div>
        </section>

        <!-- 网络 -->
        <section class="settings-group" id="sg-network">
          <header class="sg-heading"><h2>网络</h2></header>

          <div class="sg-row">
            <span class="sg-label">代理地址<span class="sg-hint">留空直连</span></span>
            <div class="sg-control">
              <input
                class="input"
                v-model="settings.proxy"
                placeholder="http://127.0.0.1:7890"
                @blur="onInputCommit"
                @keydown.enter="onInputCommit"
              />
            </div>
          </div>
        </section>

        <!-- 关于 -->
        <section class="settings-group" id="sg-about">
          <header class="sg-heading"><h2>关于</h2></header>

          <div class="sg-row">
            <span class="sg-label">Mnemo-Go</span>
            <div class="sg-control"><span class="sg-value">多网盘桌面文件管理器 · 0.1.0-preview</span></div>
          </div>
          <div class="sg-row">
            <span class="sg-label">技术栈</span>
            <div class="sg-control"><span class="sg-value">Go + Wails v2 / Vue 3</span></div>
          </div>
          <div class="sg-row">
            <span class="sg-label">开源协议</span>
            <div class="sg-control"><span class="sg-value">GPL-3.0</span></div>
          </div>
        </section>

        <div class="settings-foot">
          <span class="sg-hint">更改即时自动保存</span>
          <button class="btn primary sm" :disabled="saving" @click="save()">立即保存</button>
        </div>
      </div>
    </div>
  </div>
</template>
