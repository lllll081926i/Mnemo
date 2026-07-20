<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, type Component, watch } from 'vue'
import { FolderCog, MonitorCog, Network, ShieldCheck, SlidersHorizontal, X } from 'lucide-vue-next'
import { useAppStore } from '../store'
import SettingAccount from './SettingAccount.vue'
import SettingDebug from './SettingDebug.vue'
import SettingDown from './SettingDown.vue'
import SettingLog from './SettingLog.vue'
import SettingPan from './SettingPan.vue'
import SettingPlay from './SettingPlay.vue'
import SettingProxy from './SettingProxy.vue'
import SettingSecurity from './SettingSecurity.vue'
import SettingUI from './SettingUI.vue'
import SettingUpload from './SettingUpload.vue'

type SettingSectionKey = 'general' | 'account-security' | 'files-playback' | 'transfer' | 'advanced'

interface SettingPanel {
  component: Component
}

interface SettingSection {
  key: SettingSectionKey
  label: string
  icon: Component
  panels: SettingPanel[]
}

const appStore = useAppStore()
const sections: SettingSection[] = [
  {
    key: 'general',
    label: '通用',
    icon: MonitorCog,
    panels: [{ component: SettingUI }]
  },
  {
    key: 'account-security',
    label: '账号与安全',
    icon: ShieldCheck,
    panels: [{ component: SettingAccount }, { component: SettingSecurity }]
  },
  {
    key: 'files-playback',
    label: '文件与播放',
    icon: FolderCog,
    panels: [{ component: SettingPan }, { component: SettingPlay }]
  },
  {
    key: 'transfer',
    label: '传输',
    icon: SlidersHorizontal,
    panels: [{ component: SettingDown }, { component: SettingUpload }]
  },
  {
    key: 'advanced',
    label: '网络与高级',
    icon: Network,
    panels: [{ component: SettingProxy }, { component: SettingDebug }, { component: SettingLog }]
  }
]
const visibleSections = computed(() => sections)

const legacySectionMap: Record<string, SettingSectionKey> = {
  SettingUI: 'general',
  SettingAccount: 'account-security',
  SettingSecurity: 'account-security',
  SettingPan: 'files-playback',
  SettingPlay: 'files-playback',
  SettingDown: 'transfer',
  SettingUpload: 'transfer',
  SettingProxy: 'advanced',
  SettingDebug: 'advanced',
  SettingLog: 'advanced'
}

const normalizeSectionKey = (key: string): SettingSectionKey => {
  if (visibleSections.value.some((item) => item.key === key)) return key as SettingSectionKey
  return legacySectionMap[key] || 'general'
}

const activeKey = ref<SettingSectionKey>('general')
const settingsScroll = ref<HTMLElement | null>(null)
let sectionObserver: IntersectionObserver | undefined

const syncActiveSection = (key: SettingSectionKey) => {
  activeKey.value = key
  if (appStore.GetAppTabMenu !== key) appStore.toggleTabMenu('setting', key)
}

const scrollToSection = async (key: SettingSectionKey, behavior: ScrollBehavior = 'smooth') => {
  syncActiveSection(key)
  await nextTick()
  document.getElementById(`setting-${key}`)?.scrollIntoView({ block: 'start', behavior })
}

watch(
  () => [appStore.appTab, appStore.GetAppTabMenu] as const,
  ([tab, menu]) => {
    if (tab !== 'setting') return
    const normalized = normalizeSectionKey(menu || '')
    if (normalized !== menu) appStore.toggleTabMenu('setting', normalized)
    activeKey.value = normalized
  },
  { immediate: true }
)

onMounted(async () => {
  await nextTick()
  const root = settingsScroll.value
  if (!root) return
  sectionObserver = new IntersectionObserver(
    (entries) => {
      const visible = entries.filter((entry) => entry.isIntersecting).sort((a, b) => b.intersectionRatio - a.intersectionRatio)[0]
      const key = visible?.target.getAttribute('data-settings-section') as SettingSectionKey | null
      if (key) syncActiveSection(key)
    },
    { root, rootMargin: '-16% 0px -70% 0px', threshold: [0, 0.1, 0.5] }
  )
  root.querySelectorAll<HTMLElement>('[data-settings-section]').forEach((section) => sectionObserver?.observe(section))
  if (appStore.appTab === 'setting') scrollToSection(activeKey.value, 'auto')
})

onBeforeUnmount(() => sectionObserver?.disconnect())

const closeSettings = () => appStore.closeSettings()
</script>

<template>
  <div class="settings-page ui-page-shell">
    <aside class="settings-sidebar ui-page-rail">
      <nav class="settings-nav" aria-label="设置分类">
        <button v-for="item in visibleSections" :key="item.key" type="button" class="settings-nav-item" :class="{ active: activeKey === item.key }" :aria-current="activeKey === item.key ? 'location' : undefined" @click="scrollToSection(item.key)">
          <component :is="item.icon" :size="17" :stroke-width="1.8" class="settings-nav-icon" />
          <span>{{ item.label }}</span>
        </button>
      </nav>
    </aside>

    <main class="settings-main">
      <button type="button" class="settings-close" title="关闭设置" aria-label="关闭设置" @click="closeSettings">
        <X :size="17" :stroke-width="1.9" />
      </button>

      <div ref="settingsScroll" class="settings-scroll ui-page-content">
        <div class="settings-content ui-content-column">
          <section v-for="section in visibleSections" :id="`setting-${section.key}`" :key="section.key" class="settings-section" :data-settings-section="section.key">
            <header class="settings-section-heading">
              <h2>{{ section.label }}</h2>
            </header>
            <component v-for="(panel, index) in section.panels" :is="panel.component" :key="index" />
          </section>
        </div>
      </div>
    </main>
  </div>
</template>

<style>
.settings-page {
  --layout-row-label-width: 150px;
  width: 100%;
  grid-template-columns: var(--layout-rail-width) minmax(0, 1fr);
  color: var(--text-primary);
  background: var(--bg-surface);
}

.settings-sidebar {
  display: flex;
  flex-direction: column;
  background: var(--bg-surface);
  border-right: 1px solid var(--border-light);
}

.settings-close,
.settings-nav-item {
  font: inherit;
  border: 0;
  cursor: pointer;
}

.settings-nav {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  gap: 0;
  min-height: 0;
  padding: 8px 0;
  overflow: auto;
}

.settings-nav-item {
  position: relative;
  display: grid;
  grid-template-columns: 20px minmax(0, 1fr);
  gap: 10px;
  align-items: center;
  width: 100%;
  min-height: 32px;
  padding: 5px 12px;
  color: var(--text-secondary);
  text-align: left;
  background: transparent;
  border-left: 3px solid transparent;
}

.settings-nav-item:hover {
  color: var(--text-primary);
  background: transparent;
}

.settings-nav-item.active {
  color: var(--color-primary);
  background: transparent;
  border-left-color: var(--color-primary);
}

.settings-nav-icon {
  justify-self: center;
}

.settings-nav-item > span {
  overflow: hidden;
  color: inherit;
  font-size: 12px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.settings-main {
  position: relative;
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
  background: var(--bg-surface);
}

.settings-close {
  position: absolute;
  top: 8px;
  right: max(12px, var(--layout-page-gutter));
  z-index: 3;
  display: grid;
  flex-shrink: 0;
  place-items: center;
  width: 28px;
  height: 28px;
  padding: 0;
  color: var(--text-secondary);
  background: transparent;
  border: 0;
  border-radius: var(--ui-control-radius);
}

.settings-close:hover {
  color: var(--text-primary);
  background: var(--bg-hover);
}

.settings-scroll {
  flex: 1 1 auto;
  scrollbar-gutter: stable;
}

.settings-content {
  width: min(100%, var(--layout-content-width));
  min-width: 0;
  max-width: var(--layout-content-width);
  padding-top: 10px;
  padding-right: max(52px, var(--layout-page-gutter));
  padding-bottom: 18px;
}

.settings-page .ui-plain-row {
  grid-template-columns: var(--layout-row-label-width) minmax(0, 1fr);
  gap: var(--layout-row-gap);
}

.settings-section {
  width: 100%;
  scroll-margin-top: 12px;
}

.settings-section + .settings-section {
  margin-top: 10px;
  padding-top: 10px;
  border-top: 1px solid var(--border-light);
}

.settings-section-heading {
  display: flex;
  align-items: center;
  min-height: 24px;
  margin: 0 0 2px;
  color: var(--text-primary);
}

.settings-section-heading h2 {
  margin: 0;
  color: inherit;
  font-size: 13px;
  font-weight: 650;
  line-height: 1.4;
}

.settings-page .arco-input,
.settings-page input,
.settings-page textarea,
.settings-page .arco-select-view-value,
.settings-page .arco-btn {
  color: var(--text-primary);
}

.settings-page .arco-input-wrapper,
.settings-page .arco-select-view {
  background: var(--control-bg);
  border-color: var(--control-border);
}

.settings-page .arco-input-wrapper:focus-within,
.settings-page .arco-select-view:focus-within {
  border-color: var(--border-focus);
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--color-primary) 14%, transparent);
}

</style>
