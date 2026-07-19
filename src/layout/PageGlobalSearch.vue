<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { ArrowUpRight, ChevronRight, Clock, File, Folder, Search, X } from 'lucide-vue-next'
import { useAppStore } from '../store'
import { humanSize } from '../utils/format'
import { searchAllDrives, searchResultGroupTitle, type GlobalSearchResult } from '../utils/globalSearch'

const appStore = useAppStore()
const HISTORY_KEY = 'global_search_history'
const keyword = ref('')
const inputRef = ref<HTMLInputElement>()
const results = ref<GlobalSearchResult[]>([])
const searching = ref(false)
const selectedIndex = ref(-1)
const searchTimer = ref<ReturnType<typeof setTimeout>>()
const searchId = ref(0)
const history = ref<string[]>([])
const collapsedGroups = ref(new Set<string>())

const groups = computed(() => {
  const map = new Map<string, GlobalSearchResult[]>()
  for (const item of results.value) {
    const key = searchResultGroupTitle(item)
    map.set(key, [...(map.get(key) || []), item])
  }
  return Array.from(map, ([title, items]) => ({ title, items }))
})

const allItems = computed(() => groups.value.flatMap((group, groupIndex) => group.items.map((result, itemIndex) => ({ groupIndex, itemIndex, result }))))
const hasInput = computed(() => keyword.value.trim().length >= 2)

function readHistory() {
  try {
    const value = JSON.parse(localStorage.getItem(HISTORY_KEY) || '[]')
    history.value = Array.isArray(value) ? value.filter((item): item is string => typeof item === 'string') : []
  } catch {
    history.value = []
  }
}

function saveHistory(query: string) {
  const value = query.trim()
  if (value.length < 2) return
  history.value = [value, ...history.value.filter((item) => item !== value)].slice(0, 20)
  localStorage.setItem(HISTORY_KEY, JSON.stringify(history.value))
}

function toggleGroup(title: string) {
  const next = new Set(collapsedGroups.value)
  if (next.has(title)) next.delete(title)
  else next.add(title)
  collapsedGroups.value = next
}

async function runSearch() {
  const query = keyword.value.trim()
  if (query.length < 2) {
    results.value = []
    selectedIndex.value = -1
    return
  }
  const id = ++searchId.value
  searching.value = true
  saveHistory(query)
  try {
    const nextResults = await searchAllDrives(query)
    if (id !== searchId.value) return
    results.value = nextResults
    selectedIndex.value = nextResults.length ? 0 : -1
  } finally {
    if (id === searchId.value) searching.value = false
  }
}

async function openResult(result: GlobalSearchResult) {
  appStore.toggleTab('pan')
  await nextTick()
  const { default: UserDAL } = await import('../user/userdal')
  const { default: PanDAL } = await import('../pan/pandal')
  const { default: usePanTreeStore } = await import('../pan/pantreestore')
  const tree = usePanTreeStore()
  if (tree.user_id !== result.user_id) await UserDAL.UserChange(result.user_id)

  let parentId = result.parent_file_id || result.file_id
  if (parentId === '/' || !parentId) {
    const roots: Record<string, string> = {
      baidu: 'baidu_root', cloud123: 'cloud_root', '115': 'drive115_root', quark: 'quark_root', pikpak: 'pikpak_root', dropbox: 'dropbox_root', onedrive: 'onedrive_root', box: 'box_root'
    }
    parentId = roots[result.provider] || parentId
  }
  PanDAL.aReLoadOneDirToShow(result.drive_id, parentId, true)
}

function select(groupIndex: number, itemIndex: number) {
  selectedIndex.value = allItems.value.findIndex((item) => item.groupIndex === groupIndex && item.itemIndex === itemIndex)
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    inputRef.value?.blur()
    return
  }
  if (!allItems.value.length) return
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
    event.preventDefault()
    const direction = event.key === 'ArrowDown' ? 1 : -1
    selectedIndex.value = (selectedIndex.value + direction + allItems.value.length) % allItems.value.length
    nextTick(() => document.querySelector('.global-search-row.is-selected')?.scrollIntoView({ block: 'nearest' }))
  }
  if (event.key === 'Enter' && selectedIndex.value >= 0) {
    event.preventDefault()
    void openResult(allItems.value[selectedIndex.value].result)
  }
}

watch(keyword, () => {
  if (searchTimer.value) clearTimeout(searchTimer.value)
  if (!hasInput.value) {
    results.value = []
    selectedIndex.value = -1
    searching.value = false
    return
  }
  searching.value = true
  searchTimer.value = setTimeout(() => void runSearch(), 250)
})

onMounted(() => {
  readHistory()
  nextTick(() => inputRef.value?.focus())
})

onUnmounted(() => {
  if (searchTimer.value) clearTimeout(searchTimer.value)
})
</script>

<template>
  <main class="global-search-page" @keydown="onKeydown">
    <header class="global-search-header">
      <form class="global-search-input" @submit.prevent="runSearch">
        <Search :size="20" :stroke-width="1.8" />
        <input ref="inputRef" v-model="keyword" type="search" placeholder="搜索所有网盘" autocomplete="off" />
        <button v-if="keyword" type="button" aria-label="清除搜索" @click="keyword = ''"><X :size="16" /></button>
      </form>
    </header>

    <section v-if="searching" class="global-search-state">搜索中…</section>
    <section v-else-if="hasInput && results.length === 0" class="global-search-state">未找到与“{{ keyword }}”相关的文件</section>
    <section v-else-if="!hasInput" class="global-search-empty">
      <Search :size="44" :stroke-width="1.2" />
      <p>输入至少 2 个字符开始搜索</p>
      <div v-if="history.length" class="global-search-history">
        <span>最近搜索</span>
        <button v-for="item in history" :key="item" type="button" @click="keyword = item"><Clock :size="14" />{{ item }}</button>
      </div>
    </section>

    <section v-if="!searching && results.length" class="global-search-results">
      <div v-for="(group, groupIndex) in groups" :key="group.title" class="global-search-group">
        <button class="global-search-group-title" type="button" @click="toggleGroup(group.title)">
          <ChevronRight :size="14" :class="{ 'is-open': !collapsedGroups.has(group.title) }" />
          <span>{{ group.title }}</span>
          <small>{{ group.items.length }}</small>
        </button>
        <div v-show="!collapsedGroups.has(group.title)">
          <button
            v-for="(item, itemIndex) in group.items"
            :key="item.id"
            class="global-search-row"
            :class="{ 'is-selected': selectedIndex === allItems.findIndex((entry) => entry.groupIndex === groupIndex && entry.itemIndex === itemIndex) }"
            type="button"
            @mouseenter="select(groupIndex, itemIndex)"
            @click="openResult(item)"
          >
            <Folder v-if="item.isDir" :size="18" />
            <File v-else :size="18" />
            <span class="global-search-name">{{ item.name }}</span>
            <span v-if="item.size" class="global-search-size">{{ humanSize(item.size) }}</span>
            <ArrowUpRight :size="15" />
          </button>
        </div>
      </div>
    </section>
  </main>
</template>

<style scoped>
.global-search-page { display: flex; flex-direction: column; height: 100%; min-height: 0; color: var(--color-text-1); background: var(--color-bg-1); }
.global-search-header { padding: 20px 28px 12px; border-bottom: 1px solid var(--color-border-2); }
.global-search-input { display: flex; align-items: center; gap: 10px; max-width: 720px; margin: 0 auto; padding: 0 10px; border: 1px solid var(--color-border-2); border-radius: 6px; }
.global-search-input:focus-within { border-color: rgb(var(--primary-6)); box-shadow: 0 0 0 2px rgb(var(--primary-1)); }
.global-search-input input { width: 100%; height: 38px; color: inherit; border: 0; outline: 0; background: transparent; font-size: 14px; }
.global-search-input button { display: inline-flex; padding: 4px; color: var(--color-text-3); background: transparent; border: 0; cursor: pointer; }
.global-search-state, .global-search-empty { display: grid; place-items: center; gap: 10px; min-height: 180px; color: var(--color-text-3); font-size: 14px; }
.global-search-empty { align-content: center; }
.global-search-empty p { margin: 0; }
.global-search-history { display: flex; align-items: center; flex-wrap: wrap; justify-content: center; gap: 8px; max-width: 620px; margin-top: 10px; }
.global-search-history > span { margin-right: 4px; font-size: 12px; color: var(--color-text-3); }
.global-search-history button { display: inline-flex; align-items: center; gap: 4px; padding: 3px 7px; color: var(--color-text-2); background: transparent; border: 1px solid var(--color-border-2); border-radius: 4px; cursor: pointer; }
.global-search-results { width: min(920px, calc(100% - 56px)); margin: 14px auto 28px; overflow-y: auto; }
.global-search-group + .global-search-group { margin-top: 16px; }
.global-search-group-title, .global-search-row { display: flex; align-items: center; width: 100%; text-align: left; background: transparent; border: 0; }
.global-search-group-title { gap: 6px; padding: 4px 0 7px; color: var(--color-text-2); cursor: pointer; font-size: 13px; font-weight: 600; }
.global-search-group-title svg { transition: transform .16s ease; }.global-search-group-title svg.is-open { transform: rotate(90deg); }.global-search-group-title small { margin-left: 4px; color: var(--color-text-3); font-weight: 400; }
.global-search-row { gap: 10px; min-height: 42px; padding: 0 8px; color: var(--color-text-2); border-bottom: 1px solid var(--color-border-1); cursor: pointer; }
.global-search-row:hover, .global-search-row.is-selected { color: var(--color-text-1); background: var(--color-fill-1); }.global-search-name { min-width: 0; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }.global-search-size { color: var(--color-text-3); font-size: 12px; }
</style>
