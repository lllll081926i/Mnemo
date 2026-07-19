<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { Search, X, File, Folder, ArrowUpRight, Film, Tv, Monitor, Clock, ChevronRight } from 'lucide-vue-next'
import { useAppStore } from '../store'
import { humanSize } from '../utils/format'
import { searchAllDrives, searchResultGroupTitle, type GlobalSearchResult } from '../utils/globalSearch'

const appStore = useAppStore()
const HISTORY_KEY = 'global_search_history'
const MAX_HISTORY = 20
const keyword = ref('')
const inputRef = ref<HTMLInputElement>()
const cloudResults = ref<GlobalSearchResult[]>([])
const msResults = ref<GlobalSearchResult[]>([])
const searching = ref(false)
const selectedSection = ref<'cloud' | 'ms' | null>(null)
const selectedIndex = ref(0)
const searchTimer = ref<ReturnType<typeof setTimeout>>()
const searchId = ref(0)
const history = ref<string[]>([])
const collapsedGroups = ref<Set<string>>(new Set())
const historyCollapsed = ref(false)

function toggleGroup(key: string) {
  const next = new Set(collapsedGroups.value)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  collapsedGroups.value = next
}

function loadHistory() {
  try {
    const raw = localStorage.getItem(HISTORY_KEY)
    history.value = raw ? JSON.parse(raw) : []
  } catch {
    history.value = []
  }
}

function saveHistory() {
  localStorage.setItem(HISTORY_KEY, JSON.stringify(history.value))
}

function addToHistory(query: string) {
  const q = query.trim()
  if (q.length < 2) return
  history.value = [q, ...history.value.filter((h) => h !== q)].slice(0, MAX_HISTORY)
  saveHistory()
}

function removeHistoryItem(index: number) {
  history.value.splice(index, 1)
  saveHistory()
}

function clearHistory() {
  history.value = []
  saveHistory()
}

function useHistoryItem(query: string) {
  keyword.value = query
  nextTick(() => {
    if (inputRef.value) inputRef.value.focus()
  })
}

const cloudGroups = computed(() => {
  const groups: { key: string; title: string; items: GlobalSearchResult[] }[] = []
  const map = new Map<string, GlobalSearchResult[]>()
  for (const r of cloudResults.value) {
    const key = searchResultGroupTitle(r)
    if (!map.has(key)) {
      map.set(key, [])
      groups.push({ key, title: key, items: map.get(key)! })
    }
    map.get(key)!.push(r)
  }
  return groups
})

const msGroups = computed(() => {
  const groups: { key: string; title: string; items: GlobalSearchResult[] }[] = []
  const map = new Map<string, GlobalSearchResult[]>()
  for (const r of msResults.value) {
    const key = searchResultGroupTitle(r)
    if (!map.has(key)) {
      map.set(key, [])
      groups.push({ key, title: key, items: map.get(key)! })
    }
    map.get(key)!.push(r)
  }
  return groups
})

const allItems = computed(() => {
  const items: { section: 'cloud' | 'ms'; groupIndex: number; itemIndex: number; result: GlobalSearchResult }[] = []
  cloudGroups.value.forEach((group, gi) => {
    group.items.forEach((result, ii) => {
      items.push({ section: 'cloud', groupIndex: gi, itemIndex: ii, result })
    })
  })
  msGroups.value.forEach((group, gi) => {
    group.items.forEach((result, ii) => {
      items.push({ section: 'ms', groupIndex: gi, itemIndex: ii, result })
    })
  })
  return items
})

const hasInput = computed(() => keyword.value.trim().length >= 2)
const totalCloud = computed(() => cloudResults.value.length)
const totalMs = computed(() => msResults.value.length)
const showHistoryDrop = computed(() => !hasInput.value && history.value.length > 0)
const hasBoth = computed(() => totalCloud.value > 0 && totalMs.value > 0)

function onSearchBoxSubmit() {
  if (searchTimer.value) clearTimeout(searchTimer.value)
  doSearch()
}

function doSearch() {
  const q = keyword.value.trim()
  if (q.length < 2) {
    cloudResults.value = []
    msResults.value = []
    selectedSection.value = null
    selectedIndex.value = 0
    return
  }
  addToHistory(q)
  const id = ++searchId.value
  searching.value = true
  searchAllDrives(q).then((items) => {
    if (id !== searchId.value) return
    cloudResults.value = items.filter((r) => r.source === 'cloud')
    msResults.value = items.filter((r) => r.source === 'media_server')
    if (items.length > 0) {
      selectedSection.value = items[0].source === 'cloud' ? 'cloud' : 'ms'
      selectedIndex.value = 0
    } else {
      selectedSection.value = null
      selectedIndex.value = -1
    }
    searching.value = false
  })
}

watch(keyword, () => {
  if (searchTimer.value) clearTimeout(searchTimer.value)
  selectedSection.value = null
  selectedIndex.value = 0
  if (keyword.value.trim().length < 2) {
    cloudResults.value = []
    msResults.value = []
    searching.value = false
    return
  }
  searching.value = true
  searchTimer.value = setTimeout(doSearch, 300)
})

async function handleClick(result: GlobalSearchResult) {
  if (result.source === 'media_server') {
    nextTick(async () => {
      const { default: useMediaServerRegistryStore } = await import('../store/mediaServerRegistry')
      const { default: useMediaServerNavigationStore } = await import('../store/mediaServerNavigation')
      const registry = useMediaServerRegistryStore()
      const navigation = useMediaServerNavigationStore()

      if (result.mediaServerId) {
        registry.setCurrentServer(result.mediaServerId)
      }
      navigation.push({ kind: 'item-detail', itemId: result.file_id, title: result.name })
      appStore.toggleTab('media-server')
    })
    return
  }

  nextTick(async () => {
    appStore.toggleTab('pan')
    await nextTick()

    const { default: usePanTreeStore } = await import('../pan/pantreestore')
    const { default: PanDAL } = await import('../pan/pandal')
    const { default: UserDAL } = await import('../user/userdal')
    const panTreeStore = usePanTreeStore()

    if (panTreeStore.user_id !== result.user_id) {
      await UserDAL.UserChange(result.user_id)
    }

    await nextTick()

    let fileId = result.parent_file_id || result.file_id
    if (fileId === '/' || !fileId) {
      if (result.provider === 'baidu') fileId = 'baidu_root'
      else if (result.provider === 'cloud123') fileId = 'cloud_root'
      else if (result.provider === '115') fileId = 'drive115_root'
      else if (result.provider === 'quark') fileId = 'quark_root'
      else if (result.provider === 'pikpak') fileId = 'pikpak_root'
      else if (result.provider === 'dropbox') fileId = 'dropbox_root'
      else if (result.provider === 'onedrive') fileId = 'onedrive_root'
      else if (result.provider === 'box') fileId = 'box_root'
    }

    PanDAL.aReLoadOneDirToShow(result.drive_id, fileId, true)
  })
}

function handleKeyDown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    inputRef.value?.blur()
    return
  }
  const total = allItems.value.length
  if (!total) return
  if (event.key === 'ArrowDown') {
    event.preventDefault()
    selectedIndex.value = (selectedIndex.value + 1) % total
    updateSelectedSection()
    scrollToSelected()
  } else if (event.key === 'ArrowUp') {
    event.preventDefault()
    selectedIndex.value = (selectedIndex.value - 1 + total) % total
    updateSelectedSection()
    scrollToSelected()
  } else if (event.key === 'Enter') {
    event.preventDefault()
    if (document.activeElement === inputRef.value) {
      if (searchTimer.value) clearTimeout(searchTimer.value)
      doSearch()
      return
    }
    const item = allItems.value[selectedIndex.value]
    if (item) handleClick(item.result)
  }
}

function updateSelectedSection() {
  const item = allItems.value[selectedIndex.value]
  if (item) selectedSection.value = item.section
}

function scrollToSelected() {
  nextTick(() => {
    const el = document.querySelector('.gs-page-result-item.selected')
    if (el) el.scrollIntoView({ block: 'nearest' })
  })
}

function selectItem(section: 'cloud' | 'ms', gi: number, ii: number) {
  const idx = allItems.value.findIndex(
    (item) => item.section === section && item.groupIndex === gi && item.itemIndex === ii
  )
  if (idx >= 0) {
    selectedIndex.value = idx
    selectedSection.value = section
  }
}

function getExtBadge(ext: string) {
  return ext.split('/')[0].toLowerCase()
}

function mediaKindIcon(kind: string | undefined) {
  if (kind === 'movie' || kind === 'Movie') return Film
  if (kind === 'series' || kind === 'Series') return Tv
  return Monitor
}

function mediaKindLabel(kind: string | undefined) {
  if (kind === 'movie' || kind === 'Movie') return '电影'
  if (kind === 'series' || kind === 'Series') return '剧集'
  if (kind === 'season' || kind === 'Season') return '季'
  if (kind === 'episode' || kind === 'Episode') return '集'
  if (kind === 'person' || kind === 'Person') return '人物'
  if (kind === 'folder' || kind === 'Folder') return '目录'
  return kind
}

onMounted(() => {
  loadHistory()
  nextTick(() => {
    if (inputRef.value) inputRef.value.focus()
  })
})

onUnmounted(() => {
  if (searchTimer.value) clearTimeout(searchTimer.value)
})
</script>

<template>
  <div class="gs-page" @keydown="handleKeyDown">
    <div class="gs-page-header">
      <form class="gs-page-search" @submit.prevent="onSearchBoxSubmit">
        <Search :size="21" :stroke-width="1.7" class="gs-page-search-icon" />
        <input ref="inputRef" v-model="keyword" class="gs-page-input" type="search" placeholder="搜索所有网盘和媒体服务器..." autocomplete="off" />
        <button v-if="keyword" class="gs-page-clear" type="button" title="清除" @click="keyword = ''">
          <X :size="16" :stroke-width="1.8" />
        </button>
      </form>

      <div v-if="showHistoryDrop" class="gs-history-drop">
        <div class="gs-history-drop-header" @click="historyCollapsed = !historyCollapsed">
          <ChevronRight :size="12" :stroke-width="2.5" class="gs-chevron" :class="{ open: !historyCollapsed }" />
          <Clock :size="14" :stroke-width="1.5" />
          <span>历史搜索</span>
          <button class="gs-history-clear" type="button" @click.stop="clearHistory">清除</button>
        </div>
        <div v-show="!historyCollapsed" class="gs-history-list">
          <button
            v-for="(item, idx) in history"
            :key="idx"
            class="gs-history-item"
            type="button"
            @click="useHistoryItem(item)"
          >
            <Clock :size="13" :stroke-width="1.5" class="gs-history-item-icon" />
            <span class="gs-history-item-text">{{ item }}</span>
            <button
              class="gs-history-item-del"
              type="button"
              title="删除"
              @click.stop="removeHistoryItem(idx)"
            >
              <X :size="12" :stroke-width="1.5" />
            </button>
          </button>
        </div>
      </div>
    </div>

    <div class="gs-page-body">
      <div v-if="searching && hasInput" class="gs-page-status">
        <span class="gs-spinner" /> 搜索中...
      </div>
      <div v-else-if="!hasInput && !showHistoryDrop" class="gs-page-empty">
        <Search :size="64" :stroke-width="1" class="gs-page-empty-icon" />
        <div class="gs-page-empty-text">输入关键词开始搜索</div>
        <div class="gs-page-empty-sub">支持搜索文件名、文件夹名、媒体服务器内容</div>
      </div>
      <div
        v-else-if="!searching && totalCloud === 0 && totalMs === 0"
        class="gs-page-status"
      >
        未找到与 "{{ keyword }}" 相关的结果
      </div>

      <div v-if="!searching" class="gs-page-results" :class="{ 'gs-split-mode': hasBoth }">

        <!-- Split layout: both cloud + media server -->
        <div v-if="hasBoth" class="gs-split">
          <div class="gs-split-left">
            <div class="gs-section">
              <div class="gs-section-header">
                <Folder :size="16" :stroke-width="1.5" />
                <span>网盘文件</span>
                <span class="gs-section-count">{{ totalCloud }}</span>
              </div>
              <div v-for="(group, gi) in cloudGroups" :key="group.key" class="gs-page-group">
                <div class="gs-page-group-title" @click="toggleGroup(group.key)">
                  <ChevronRight :size="12" :stroke-width="2.5" class="gs-chevron" :class="{ open: !collapsedGroups.has(group.key) }" />
                  {{ group.title }} ({{ group.items.length }})
                </div>
                <div v-show="!collapsedGroups.has(group.key)">
                  <div v-for="(item, ii) in group.items" :key="item.id" class="gs-page-result-item gs-cloud-item" :class="{ selected: allItems[selectedIndex]?.section === 'cloud' && allItems[selectedIndex]?.groupIndex === gi && allItems[selectedIndex]?.itemIndex === ii }" @click="handleClick(item)" @mouseenter="selectItem('cloud', gi, ii)">
                    <div class="gs-page-result-icon">
                      <Folder v-if="item.isDir" :size="18" :stroke-width="1.5" />
                      <File v-else :size="18" :stroke-width="1.5" />
                    </div>
                    <div class="gs-page-result-body">
                      <div class="gs-page-result-name">{{ item.name }}</div>
                      <div v-if="item.path" class="gs-page-result-path">{{ item.path }}</div>
                      <div class="gs-page-result-meta">
                        <span v-if="item.ext" class="gs-page-ext">{{ getExtBadge(item.ext) }}</span>
                        <span v-if="item.size > 0">{{ humanSize(item.size) }}</span>
                        <span class="gs-page-source">{{ item.providerName }}</span>
                      </div>
                    </div>
                    <ArrowUpRight :size="14" :stroke-width="1.5" class="gs-page-result-go" />
                  </div>
                </div>
              </div>
            </div>
          </div>
          <div class="gs-split-right">
            <div class="gs-section">
              <div class="gs-section-header">
                <Tv :size="16" :stroke-width="1.5" />
                <span>媒体服务器</span>
                <span class="gs-section-count">{{ totalMs }}</span>
              </div>
              <div v-for="(group, gi) in msGroups" :key="group.key" class="gs-ms-group">
                <div class="gs-page-group-title" @click="toggleGroup(group.key)">
                  <ChevronRight :size="12" :stroke-width="2.5" class="gs-chevron" :class="{ open: !collapsedGroups.has(group.key) }" />
                  {{ group.title }} ({{ group.items.length }})
                </div>
                <div v-show="!collapsedGroups.has(group.key)" class="gs-ms-grid">
                  <div v-for="(item, ii) in group.items" :key="item.id" class="gs-ms-card" :class="{ selected: allItems[selectedIndex]?.section === 'ms' && allItems[selectedIndex]?.groupIndex === gi && allItems[selectedIndex]?.itemIndex === ii }" @click="handleClick(item)" @mouseenter="selectItem('ms', gi, ii)">
                    <div class="gs-ms-poster-wrap">
                      <img v-if="item.poster" :src="item.poster" class="gs-ms-poster" loading="lazy" @error="($event.target as HTMLImageElement).style.display='none'" />
                      <div v-else class="gs-ms-poster-fallback">
                        <component :is="mediaKindIcon(item.itemType)" :size="24" :stroke-width="1.3" />
                      </div>
                      <div class="gs-ms-type-badge">{{ mediaKindLabel(item.itemType) }}</div>
                    </div>
                    <div class="gs-ms-title">{{ item.name }}</div>
                    <div class="gs-ms-source">{{ item.providerName }}</div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Single layout: only one type -->
      <template v-else>
          <div v-if="totalCloud > 0" class="gs-section">
            <div class="gs-section-header">
              <Folder :size="18" :stroke-width="1.5" />
              <span>网盘文件</span>
              <span class="gs-section-count">{{ totalCloud }}</span>
            </div>
            <div v-for="(group, gi) in cloudGroups" :key="group.key" class="gs-page-group">
              <div class="gs-page-group-title" @click="toggleGroup(group.key)">
                <ChevronRight :size="12" :stroke-width="2.5" class="gs-chevron" :class="{ open: !collapsedGroups.has(group.key) }" />
                {{ group.title }} ({{ group.items.length }})
              </div>
              <div v-show="!collapsedGroups.has(group.key)">
                <div v-for="(item, ii) in group.items" :key="item.id" class="gs-page-result-item gs-cloud-item" :class="{ selected: allItems[selectedIndex]?.section === 'cloud' && allItems[selectedIndex]?.groupIndex === gi && allItems[selectedIndex]?.itemIndex === ii }" @click="handleClick(item)" @mouseenter="selectItem('cloud', gi, ii)">
                  <div class="gs-page-result-icon">
                    <Folder v-if="item.isDir" :size="20" :stroke-width="1.5" />
                    <File v-else :size="20" :stroke-width="1.5" />
                  </div>
                  <div class="gs-page-result-body">
                    <div class="gs-page-result-name">{{ item.name }}</div>
                    <div v-if="item.path" class="gs-page-result-path">{{ item.path }}</div>
                    <div class="gs-page-result-meta">
                      <span v-if="item.ext" class="gs-page-ext">{{ getExtBadge(item.ext) }}</span>
                      <span v-if="item.size > 0">{{ humanSize(item.size) }}</span>
                      <span class="gs-page-source">{{ item.providerName }}</span>
                    </div>
                  </div>
                  <ArrowUpRight :size="16" :stroke-width="1.5" class="gs-page-result-go" />
                </div>
              </div>
            </div>
          </div>

          <div v-if="totalMs > 0" class="gs-section">
            <div class="gs-section-header">
              <Tv :size="18" :stroke-width="1.5" />
              <span>媒体服务器</span>
              <span class="gs-section-count">{{ totalMs }}</span>
            </div>
            <div v-for="(group, gi) in msGroups" :key="group.key" class="gs-ms-group">
              <div class="gs-page-group-title" @click="toggleGroup(group.key)">
                <ChevronRight :size="12" :stroke-width="2.5" class="gs-chevron" :class="{ open: !collapsedGroups.has(group.key) }" />
                {{ group.title }} ({{ group.items.length }})
              </div>
              <div v-show="!collapsedGroups.has(group.key)" class="gs-ms-grid">
                <div v-for="(item, ii) in group.items" :key="item.id" class="gs-ms-card" :class="{ selected: allItems[selectedIndex]?.section === 'ms' && allItems[selectedIndex]?.groupIndex === gi && allItems[selectedIndex]?.itemIndex === ii }" @click="handleClick(item)" @mouseenter="selectItem('ms', gi, ii)">
                  <div class="gs-ms-poster-wrap">
                    <img v-if="item.poster" :src="item.poster" class="gs-ms-poster" loading="lazy" @error="($event.target as HTMLImageElement).style.display='none'" />
                    <div v-else class="gs-ms-poster-fallback">
                      <component :is="mediaKindIcon(item.itemType)" :size="28" :stroke-width="1.3" />
                    </div>
                    <div class="gs-ms-type-badge">{{ mediaKindLabel(item.itemType) }}</div>
                  </div>
                  <div class="gs-ms-title">{{ item.name }}</div>
                  <div class="gs-ms-source">{{ item.providerName }}</div>
                </div>
              </div>
            </div>
          </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.gs-page {
  display: flex;
  flex-direction: column;
  height: 100%;
  color: var(--color-text-2);
  background: var(--color-bg-1);
  overflow: hidden;
}

.gs-page-header {
  padding: 24px 48px 12px;
  flex-shrink: 0;
}
.gs-page-tabs {
  display: flex;
  gap: 0;
  max-width: 320px;
  margin: 0 auto 16px;
  padding: 3px;
  background: var(--color-fill-2);
  border-radius: 10px;
}
.gs-page-tab {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 5px;
  flex: 1;
  padding: 7px 0;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--color-text-3);
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
  text-align: center;
  line-height: 22px;
}

.gs-pro-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 15px;
  padding: 0 5px;
  border-radius: 999px;
  background: linear-gradient(135deg, #f59e0b, #f97316);
  color: #fff;
  font-size: 9px;
  font-weight: 700;
  line-height: 1;
  letter-spacing: 0;
  box-shadow: 0 2px 6px rgba(245, 158, 11, 0.28);
}
.gs-page-tab:hover:not(.active) {
  color: var(--color-text-2);
}
.gs-page-tab.active {
  background: var(--color-bg-2);
  color: var(--color-text-1);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
}
.gs-page-tab:hover {
  border-color: var(--color-text-4);
}
.gs-page-tab.active {
  background: var(--color-bg-2);
  color: var(--color-text-1);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
}

.gs-page-search {
  display: flex;
  align-items: center;
  gap: 14px;
  max-width: 720px;
  margin: 0 auto;
  padding: 12px 20px;
  background: var(--color-bg-2);
  border: 1px solid var(--color-border-2);
  border-radius: 10px;
  transition: border-color 0.2s;
}

.gs-page-search:focus-within {
  border-color: rgb(var(--primary-6));
  box-shadow: 0 0 0 2px rgba(var(--primary-6), 0.12);
}

.gs-page-search-icon {
  flex-shrink: 0;
  color: var(--color-text-3);
}

.gs-page-input {
  flex: 1;
  padding: 0;
  color: var(--color-text-1);
  font-size: 20px;
  font-weight: 400;
  line-height: 28px;
  background: transparent;
  border: 0;
  outline: none;
}

.gs-page-input::placeholder {
  color: var(--color-text-4);
}

.gs-page-clear {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  padding: 0;
  color: var(--color-text-3);
  background: transparent;
  border: 0;
  border-radius: 6px;
  cursor: pointer;
}

.gs-page-clear:hover {
  background: var(--color-fill-2);
  color: var(--color-text-2);
}

.gs-page-hint {
  margin-top: 10px;
  font-size: 12px;
  color: var(--color-text-4);
  text-align: center;
}

.gs-page-body {
  flex: 1;
  overflow-y: auto;
}

.gs-page-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 80px 20px;
  text-align: center;
}

.gs-page-empty-icon {
  color: var(--color-text-4);
  opacity: 0.3;
  margin-bottom: 16px;
}

.gs-page-empty-text {
  font-size: 16px;
  font-weight: 500;
  color: var(--color-text-3);
  margin-bottom: 6px;
}

.gs-page-empty-sub {
  font-size: 13px;
  color: var(--color-text-4);
}

.gs-page-status {
  padding: 40px 20px;
  font-size: 14px;
  text-align: center;
  color: var(--color-text-3);
}

.gs-spinner {
  display: inline-block;
  width: 14px;
  height: 14px;
  border: 2px solid var(--color-border-2);
  border-top-color: rgb(var(--primary-6));
  border-radius: 50%;
  vertical-align: middle;
  margin-right: 6px;
  animation: gs-spin 0.6s linear infinite;
}

@keyframes gs-spin {
  to { transform: rotate(360deg); }
}

.gs-page-results {
  max-width: 900px;
  margin: 0 auto;
  padding: 16px 48px 32px;
}

.gs-page-results.gs-split-mode {
  max-width: none;
  padding: 0;
  height: 100%;
  overflow: hidden;
}

/* Split layout */
.gs-split {
  display: flex;
  height: 100%;
  overflow: hidden;
}

.gs-split-left,
.gs-split-right {
  flex: 1;
  min-width: 0;
  overflow-y: auto;
  padding: 16px 24px 32px;
}

.gs-split-left {
  border-right: 1px solid var(--color-border-2);
}

.gs-split-right .gs-ms-grid {
  grid-template-columns: repeat(auto-fill, minmax(130px, 1fr));
  gap: 12px;
}

.gs-split-right .gs-ms-card {
  border-radius: 6px;
}

.gs-split-right .gs-ms-poster-wrap {
  border-radius: 4px;
}

.gs-split-right .gs-ms-title {
  font-size: 12px;
}

.gs-split-right .gs-ms-source {
  font-size: 10px;
}

.gs-section {
  margin-bottom: 32px;
}

.gs-section-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 0;
  font-size: 15px;
  font-weight: 600;
  color: var(--color-text-1);
  border-bottom: 2px solid var(--color-border-2);
  margin-bottom: 12px;
}

.gs-section-count {
  margin-left: 4px;
  font-size: 12px;
  font-weight: 500;
  color: var(--color-text-4);
}

/* Cloud drive items */
.gs-page-group {
  margin-bottom: 20px;
}

.gs-page-group-title {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 6px 0 8px;
  font-size: 12px;
  font-weight: 600;
  color: var(--color-text-3);
  border-bottom: 1px solid var(--color-border-2);
  margin-bottom: 4px;
  cursor: pointer;
  user-select: none;
}

.gs-page-group-title:hover {
  color: var(--color-text-2);
}

.gs-chevron {
  flex-shrink: 0;
  color: var(--color-text-4);
  transition: transform 0.15s;
}

.gs-chevron.open {
  transform: rotate(90deg);
}

.gs-page-result-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  cursor: pointer;
  border-radius: 6px;
  transition: background 0.1s;
}

.gs-page-result-item:hover,
.gs-page-result-item.selected {
  background: var(--color-fill-2);
}

.gs-page-result-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  flex-shrink: 0;
  color: var(--color-text-3);
}

.gs-page-result-body {
  flex: 1;
  min-width: 0;
}

.gs-page-result-name {
  font-size: 14px;
  font-weight: 500;
  color: var(--color-text-1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  line-height: 22px;
}

.gs-page-result-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 2px;
  font-size: 12px;
  color: var(--color-text-3);
}

.gs-page-result-path {
  font-size: 12px;
  color: var(--color-text-4);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  line-height: 18px;
  margin-top: 1px;
}

.gs-page-ext {
  padding: 0 5px;
  font-size: 11px;
  font-weight: 600;
  line-height: 18px;
  color: var(--color-text-2);
  background: var(--color-fill-2);
  border-radius: 3px;
  text-transform: uppercase;
}

.gs-page-source {
  padding: 0 6px;
  font-size: 11px;
  line-height: 18px;
  color: rgb(var(--primary-6));
  background: rgba(var(--primary-6), 0.08);
  border-radius: 3px;
}

.gs-page-result-go {
  flex-shrink: 0;
  color: var(--color-text-4);
  opacity: 0;
  transition: opacity 0.15s;
}

.gs-page-result-item:hover .gs-page-result-go {
  opacity: 1;
}

/* Media server cards */
.gs-ms-group {
  margin-bottom: 20px;
}

.gs-ms-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
  gap: 14px;
  margin-top: 4px;
}

.gs-ms-card {
  cursor: pointer;
  border-radius: 8px;
  overflow: hidden;
  transition: background 0.15s, transform 0.15s;
}

.gs-ms-card:hover,
.gs-ms-card.selected {
  background: var(--color-fill-2);
}

.gs-ms-card:hover {
  transform: translateY(-2px);
}

.gs-ms-poster-wrap {
  position: relative;
  width: 100%;
  aspect-ratio: 2 / 3;
  border-radius: 6px;
  overflow: hidden;
  background: var(--color-fill-2);
}

.gs-ms-poster {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.gs-ms-poster-fallback {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 100%;
  color: var(--color-text-4);
  background: var(--color-fill-2);
}

.gs-ms-type-badge {
  position: absolute;
  bottom: 6px;
  left: 6px;
  padding: 1px 6px;
  font-size: 10px;
  font-weight: 600;
  line-height: 16px;
  color: #fff;
  background: rgba(0, 0, 0, 0.65);
  border-radius: 3px;
  backdrop-filter: blur(4px);
}

.gs-ms-title {
  padding: 6px 2px 1px;
  font-size: 13px;
  font-weight: 500;
  color: var(--color-text-1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  line-height: 18px;
}

.gs-ms-source {
  padding: 0 2px 4px;
  font-size: 11px;
  color: var(--color-text-4);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* Search history drop */
.gs-history-drop {
  max-width: 720px;
  margin: 8px auto 0;
  padding: 4px 0;
  background: var(--color-bg-2);
  border: 1px solid var(--color-border-2);
  border-radius: 8px;
}

.gs-history-drop-header {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 4px;
  padding: 8px 12px 0;
  font-size: 12px;
  font-weight: 600;
  color: var(--color-text-3);
  cursor: pointer;
}

.gs-history-clear {
  margin-left: auto;
  padding: 1px 6px;
  font-size: 12px;
  font-weight: 400;
  color: var(--color-text-4);
  background: transparent;
  border: 0;
  border-radius: 4px;
  cursor: pointer;
}

.gs-history-clear:hover {
  color: rgb(var(--danger-6));
  background: var(--color-fill-2);
}

.gs-history-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  padding: 4px 12px 8px;
}

.gs-history-item {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 5px 10px;
  text-align: left;
  color: var(--color-text-2);
  font-size: 13px;
  background: var(--color-fill-2);
  border: 0;
  border-radius: 16px;
  cursor: pointer;
  transition: background 0.1s;
}

.gs-history-item:hover {
  background: var(--color-fill-3);
}

.gs-history-item-icon {
  flex-shrink: 0;
  color: var(--color-text-4);
}

.gs-history-item-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 120px;
}

.gs-history-item-del {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  padding: 0;
  flex-shrink: 0;
  color: var(--color-text-4);
  background: transparent;
  border: 0;
  border-radius: 10px;
  cursor: pointer;
  opacity: 0;
  transition: opacity 0.1s, color 0.1s;
}

.gs-history-item:hover .gs-history-item-del {
  opacity: 1;
}

.gs-history-item-del:hover {
  color: rgb(var(--danger-6));
  background: var(--color-fill-2);
}

@media (max-width: 720px) {
  .gs-page-header {
    padding: 20px 16px 12px;
  }

  .gs-page-results {
    padding: 12px 16px 24px;
  }

  .gs-ms-grid {
    grid-template-columns: repeat(auto-fill, minmax(110px, 1fr));
    gap: 10px;
  }
}
</style>

<style>
/* ===== Mineradio dark-theme alignment for Global Search ===== */
body[arco-theme='dark'] .gs-page {
  color: var(--app-mineradio-ink);
  background: transparent;
}

body[arco-theme='dark'] .gs-page-tabs {
  background: rgba(255,255,255,.055);
  border: 1px solid rgba(255,255,255,.075);
  border-radius: 14px;
  backdrop-filter: blur(18px) saturate(1.2);
}

body[arco-theme='dark'] .gs-page-tab {
  color: rgba(255,255,255,.60);
  border-radius: 10px;
  background: transparent;
}

body[arco-theme='dark'] .gs-page-tab:hover:not(.active) {
  color: rgba(255,255,255,.82);
}

body[arco-theme='dark'] .gs-page-tab.active {
  color: #fff;
  background: rgba(255,255,255,.085);
  box-shadow: inset 0 0 0 1px rgba(0,245,212,.22), 0 10px 28px rgba(0,245,212,.055);
}

body[arco-theme='dark'] .gs-page-search {
  background: rgba(255,255,255,.055);
  border: 1px solid rgba(255,255,255,.075);
  border-radius: 16px;
  backdrop-filter: blur(18px) saturate(1.2);
}

body[arco-theme='dark'] .gs-page-search:focus-within {
  border-color: rgba(0,245,212,.22);
  box-shadow: 0 0 0 3px rgba(0,245,212,.08);
}

body[arco-theme='dark'] .gs-page-input {
  color: #fff;
}

body[arco-theme='dark'] .gs-page-input::placeholder {
  color: rgba(255,255,255,.36);
}

body[arco-theme='dark'] .gs-page-search-icon,
body[arco-theme='dark'] .gs-page-clear {
  color: rgba(255,255,255,.42);
}

body[arco-theme='dark'] .gs-page-clear:hover {
  background: rgba(255,255,255,.08);
  color: #fff;
}

body[arco-theme='dark'] .gs-page-empty-icon {
  color: rgba(255,255,255,.25);
}

body[arco-theme='dark'] .gs-page-empty-text {
  color: rgba(255,255,255,.72);
}

body[arco-theme='dark'] .gs-page-empty-sub {
  color: rgba(255,255,255,.40);
}

body[arco-theme='dark'] .gs-page-status {
  color: rgba(255,255,255,.55);
}

body[arco-theme='dark'] .gs-spinner {
  border-color: rgba(255,255,255,.15);
  border-top-color: var(--app-mineradio-accent);
}

body[arco-theme='dark'] .gs-section-header {
  color: rgba(255,255,255,.92);
  border-bottom-color: rgba(255,255,255,.07);
}

body[arco-theme='dark'] .gs-section-count {
  color: var(--app-mineradio-accent);
}

body[arco-theme='dark'] .gs-page-group-title {
  color: rgba(255,255,255,.60);
  border-bottom-color: rgba(255,255,255,.06);
}

body[arco-theme='dark'] .gs-page-group-title:hover {
  color: rgba(255,255,255,.85);
}

body[arco-theme='dark'] .gs-page-result-item {
  border: 1px solid transparent;
  border-radius: 13px;
  background: transparent;
  transition: background .15s, border-color .15s;
}

body[arco-theme='dark'] .gs-page-result-item:hover,
body[arco-theme='dark'] .gs-page-result-item.selected {
  background: rgba(255,255,255,.075);
  border-color: rgba(244,210,138,.18);
  box-shadow: 0 10px 28px rgba(0,0,0,.18);
}

body[arco-theme='dark'] .gs-page-result-name {
  color: rgba(255,255,255,.92);
}

body[arco-theme='dark'] .gs-page-result-meta,
body[arco-theme='dark'] .gs-page-result-path {
  color: rgba(255,255,255,.42);
}

body[arco-theme='dark'] .gs-page-ext {
  color: rgba(255,255,255,.72);
  background: rgba(255,255,255,.06);
}

body[arco-theme='dark'] .gs-page-source {
  color: var(--app-mineradio-accent);
  background: rgba(0,245,212,.10);
}

body[arco-theme='dark'] .gs-page-result-go {
  color: rgba(255,255,255,.42);
}

body[arco-theme='dark'] .gs-ms-card {
  border: 1px solid rgba(255,255,255,.075);
  border-radius: 14px;
  background: rgba(255,255,255,.042);
  transition: background .15s, transform .15s, border-color .15s, box-shadow .15s;
}

body[arco-theme='dark'] .gs-ms-card:hover {
  transform: translateY(-2px);
  background: rgba(255,255,255,.068);
  border-color: rgba(0,245,212,.20);
  box-shadow: 0 14px 38px rgba(0,0,0,.26);
}

body[arco-theme='dark'] .gs-ms-poster-wrap {
  background: rgba(255,255,255,.055);
  border: 1px solid rgba(255,255,255,.06);
}

body[arco-theme='dark'] .gs-ms-poster-fallback {
  color: rgba(255,255,255,.30);
  background: rgba(255,255,255,.04);
}

body[arco-theme='dark'] .gs-ms-type-badge {
  background: rgba(0,0,0,.65);
  backdrop-filter: blur(4px);
}

body[arco-theme='dark'] .gs-ms-title {
  color: rgba(255,255,255,.90);
}

body[arco-theme='dark'] .gs-ms-source {
  color: rgba(255,255,255,.40);
}

body[arco-theme='dark'] .gs-split-left {
  border-right-color: rgba(255,255,255,.07);
}

body[arco-theme='dark'] .gs-history-drop {
  background: rgba(255,255,255,.055);
  border: 1px solid rgba(255,255,255,.075);
  border-radius: 14px;
  backdrop-filter: blur(18px) saturate(1.2);
}

body[arco-theme='dark'] .gs-history-drop-header {
  color: rgba(255,255,255,.60);
}

body[arco-theme='dark'] .gs-history-item {
  color: rgba(255,255,255,.72);
  background: rgba(255,255,255,.055);
  border: 1px solid rgba(255,255,255,.06);
}

body[arco-theme='dark'] .gs-history-item:hover {
  background: rgba(255,255,255,.085);
}

body[arco-theme='dark'] .gs-history-item-icon {
  color: rgba(255,255,255,.40);
}

body[arco-theme='dark'] .gs-history-item-del:hover {
  color: #ff8f9d;
  background: rgba(255,143,157,.12);
}

body[arco-theme='dark'] .gs-chevron {
  color: rgba(255,255,255,.40);
}

body[arco-theme='dark'] .gs-pro-badge {
  box-shadow: 0 2px 8px rgba(245,158,11,.32);
}

body[arco-theme='dark'] .ph-results-section {
  padding-top: 18px;
}

body[arco-theme='dark'] .ph-stats-bar {
  background: rgba(255,255,255,.045);
  border-color: rgba(255,255,255,.085);
  box-shadow: inset 0 1px 0 rgba(255,255,255,.035);
}

body[arco-theme='dark'] .ph-stat-item,
body[arco-theme='dark'] .ph-filter-pill,
body[arco-theme='dark'] .ph-sort-select {
  background: rgba(255,255,255,.055);
  border-color: rgba(255,255,255,.075);
}

body[arco-theme='dark'] .ph-filter-pill.active {
  background: rgba(105,121,255,.92);
  border-color: rgba(158,169,255,.45);
}
</style>
