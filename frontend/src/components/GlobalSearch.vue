<script setup>
import { ref, shallowRef, computed, nextTick, watch, onMounted, onBeforeUnmount } from 'vue'
import { motion, MotionConfig } from 'motion-v'
import { search, capsOf, accountName, providerMetaOf, providerIconUrl } from '../api'
import Modal from './Modal.vue'
import UiIcon from './UiIcon.vue'
import { compactReveal, reducedMotion } from '../motion/presets'
const props = defineProps({ accounts: Array, providers: Array, initialQuery: { type: String, default: '' } })
const emit = defineEmits(['close', 'navigate'])
const keyword = ref(props.initialQuery), results = shallowRef([]), errors = ref([]), busy = ref(false), remote = ref(false), stopped = ref(false)
const submittedQuery = ref(''), submittedRemote = ref(false)
const matchesSubmission = computed(() => keyword.value.trim() === submittedQuery.value && remote.value === submittedRemote.value)
const submitDisabled = computed(() => !keyword.value.trim() || (busy.value && matchesSubmission.value))
const accountStates = ref([])
const activeRequests = new Map()
const cancelledSearch = Symbol('cancelled-search')
const searchSession = globalThis.crypto?.randomUUID?.() || `${Date.now()}-${Math.random()}`
let epoch = 0
function cancelRequests(remainingAccounts) {
  for (const [requestId, request] of activeRequests) {
    if (remainingAccounts?.has(request.key)) continue
    activeRequests.delete(requestId)
    request.release(cancelledSearch)
    try { Promise.resolve(window.go?.app?.App?.CancelSearch?.(requestId)).catch(() => {}) } catch {}
  }
}
onBeforeUnmount(() => { ++epoch; cancelRequests() })
const accountKey = (userId, driveId) => `${userId}\u0000${driveId}`
const resultKey = result => `${accountKey(result.userId, result.driveId)}\u0000${result.file.file_id}`
const accountDisplay = computed(() => new Map(props.accounts.map(account => [accountKey(account.user_id, account.drive_id), {
  name: accountName(account), icon: providerIconUrl(providerMetaOf(account, props.providers)),
}])))
watch(accountDisplay, accounts => {
  cancelRequests(accounts)
  const remaining = accountStates.value.filter(state => accounts.has(accountKey(state.userId, state.driveId)))
  if (remaining.length !== accountStates.value.length) accountStates.value = remaining
})
const ordered = computed(() => {
  const buckets = new Map()
  for (const result of results.value) {
    const key = accountKey(result.userId, result.driveId)
    if (!accountDisplay.value.has(key)) continue
    if (!buckets.has(key)) buckets.set(key, [])
    buckets.get(key).push(result)
  }
  return Array.from(accountDisplay.value.keys()).flatMap(key => buckets.get(key) || [])
})
async function run() {
  const query = keyword.value.trim()
  if (!query || (busy.value && matchesSubmission.value)) return
  cancelRequests()
  const seq = ++epoch
  const online = remote.value
  submittedQuery.value = query
  submittedRemote.value = online
  stopped.value = false
  if (focusedKey.value) inputEl.value?.focus()
  results.value = []; errors.value = []; accountStates.value = []
  scrollTop.value = 0
  if (resultsEl.value) resultsEl.value.scrollTop = 0
  busy.value = true
  let cachedResults = []
  const onlineResults = new Map()
  const updateResults = () => {
    if (seq !== epoch) return
    results.value = [
      ...cachedResults.filter(result => !onlineResults.has(resultKey(result))),
      ...onlineResults.values(),
    ]
  }
  const cachedTask = Promise.resolve().then(() => window.go.app.App.SearchCachedFiles(query)).then(cached => {
    if (seq !== epoch) return
    cachedResults = [...new Map((cached || []).map(result => [resultKey(result), { ...result, source: result.partial ? '缓存（部分目录）' : '缓存' }])).values()]
    if ((cached || []).length >= 1000) errors.value.push('缓存结果达到 1000 条显示上限，请缩小搜索范围')
    updateResults()
  }).catch(() => { if (seq === epoch) errors.value.push('缓存搜索失败，请重试') })
  let onlineAccounts = []
  const worker = async () => {
    while (onlineAccounts.length && seq === epoch) {
      const acc = onlineAccounts.shift()
      const key = accountKey(acc.user_id, acc.drive_id)
      if (!accountDisplay.value.has(key)) continue
      const state = accountStates.value.find(item => item.userId === acc.user_id && item.driveId === acc.drive_id)
      const requestId = `${searchSession}:${seq}:${acc.user_id}:${acc.drive_id}`
      const cancel = new Promise(release => activeRequests.set(requestId, { key, release }))
      if (state) state.status = '搜索中'
      try {
        const files = await Promise.race([search(acc.user_id, acc.drive_id, query, requestId), cancel])
        if (seq !== epoch) return
        if (files === cancelledSearch || !activeRequests.has(requestId) || !accountDisplay.value.has(key)) continue
        if (state) state.status = `在线查询完成 · ${(files || []).length} 项`
        const updatedAt = Date.now() / 1000
        for (const file of files || []) {
          const result = { userId: acc.user_id, driveId: acc.drive_id, parentId: file.parent_file_id, file, source: '在线', updatedAt }
          onlineResults.set(resultKey(result), result)
        }
        updateResults()
      } catch {
        if (seq === epoch && activeRequests.has(requestId) && accountDisplay.value.has(key)) {
          if (state) state.status = '失败，结果不完整'
          errors.value.push(`${accountName(acc)}：在线搜索失败`)
        }
      } finally { activeRequests.delete(requestId) }
    }
  }
  try {
    if (online) {
      accountStates.value = props.accounts.map(acc => ({ userId: acc.user_id, driveId: acc.drive_id, name: accountName(acc), status: capsOf(acc, props.providers).search ? '等待查询' : '不支持在线搜索，仅缓存结果' }))
      onlineAccounts = props.accounts.filter(acc => capsOf(acc, props.providers).search)
    }
    await Promise.all([cachedTask, worker(), worker()])
  } catch { if (seq === epoch) errors.value.push('搜索失败，请重试') }
  finally { if (seq === epoch) busy.value = false }
}
function stop() {
  ++epoch
  cancelRequests()
  accountStates.value.forEach(state => { if (['搜索中', '等待查询'].includes(state.status)) state.status = '已停止，结果不完整' })
  busy.value = false
  stopped.value = true
}
function open(result) {
  emit('navigate', { userId: result.userId, driveId: result.driveId, dirId: result.file.isDir ? result.file.file_id : result.parentId, name: result.file.isDir ? result.file.name : '所在目录', file: result.file })
  emit('close')
}

const inputEl = ref(null)
const resultsEl = ref(null)
const rowHeight = 56
const overscan = 4
const scrollTop = ref(0)
const viewportHeight = ref(420)
const focusedKey = ref('')
let resultsObserver
const focusedIndex = computed(() => focusedKey.value ? ordered.value.findIndex(result => resultKey(result) === focusedKey.value) : -1)
const firstVisibleIndex = computed(() => Math.min(Math.max(0, ordered.value.length - 1), Math.floor(scrollTop.value / rowHeight)))
const visibleRows = computed(() => {
  const total = ordered.value.length
  const first = firstVisibleIndex.value
  const start = Math.max(0, first - overscan)
  const end = Math.min(total, first + Math.ceil(viewportHeight.value / rowHeight) + overscan + 1)
  const entries = ordered.value.slice(start, end).map((result, offset) => ({ result, index: start + offset }))
  const pinned = focusedIndex.value
  if (pinned >= 0 && (pinned < start || pinned >= end)) {
    const entry = { result: ordered.value[pinned], index: pinned }
    if (pinned < start) entries.unshift(entry)
    else entries.push(entry)
  }
  return entries
})

function syncScroll() {
  scrollTop.value = Math.max(0, resultsEl.value?.scrollTop || 0)
}

watch(ordered, (next, previous, onCleanup) => {
  const viewport = resultsEl.value
  const top = viewport?.scrollTop || 0
  if (top <= 0 || !next.length) return
  const oldIndex = Math.floor(top / rowHeight)
  const anchor = previous[oldIndex]
  if (!anchor) return
  const key = resultKey(anchor)
  if (next[oldIndex] && resultKey(next[oldIndex]) === key) return
  const index = next.findIndex(result => resultKey(result) === key)
  if (index < 0) return
  const anchoredTop = top + (index - oldIndex) * rowHeight
  const seq = epoch
  let cancelled = false
  onCleanup(() => { cancelled = true })
  scrollTop.value = anchoredTop
  nextTick(() => {
    if (cancelled || seq !== epoch || viewport !== resultsEl.value) return
    viewport.scrollTop = anchoredTop
    syncScroll()
  })
})

async function focusResult(index) {
  const viewport = resultsEl.value
  if (!viewport || !ordered.value.length) return
  const next = Math.max(0, Math.min(ordered.value.length - 1, index))
  const top = next * rowHeight
  const height = viewport.clientHeight || viewportHeight.value
  if (top < viewport.scrollTop) viewport.scrollTop = top
  else if (top + rowHeight > viewport.scrollTop + height) viewport.scrollTop = top + rowHeight - height
  syncScroll()
  await nextTick()
  viewport.querySelector(`[data-result-index="${next}"]`)?.focus({ preventScroll: true })
}

function onSearchKeydown(event) {
  if (event.key === 'ArrowDown' && !event.isComposing && ordered.value.length) {
    event.preventDefault()
    focusResult(0)
  }
}

function onResultKeydown(event, index) {
  if (event.isComposing || !['ArrowDown', 'ArrowUp', 'Home', 'End', 'PageUp', 'PageDown'].includes(event.key)) return
  event.preventDefault()
  const page = Math.max(1, Math.floor(viewportHeight.value / rowHeight))
  if (event.key === 'ArrowDown') focusResult(index + 1)
  else if (event.key === 'ArrowUp' && index === 0) inputEl.value?.focus()
  else if (event.key === 'ArrowUp') focusResult(index - 1)
  else if (event.key === 'Home') focusResult(0)
  else if (event.key === 'End') focusResult(ordered.value.length - 1)
  else focusResult(index + (event.key === 'PageUp' ? -page : page))
}

watch(focusedIndex, index => { if (index < 0 && focusedKey.value) inputEl.value?.focus() })
onMounted(() => {
  if (typeof ResizeObserver !== 'undefined' && resultsEl.value) {
    resultsObserver = new ResizeObserver(() => { if (resultsEl.value?.clientHeight) viewportHeight.value = resultsEl.value.clientHeight })
    resultsObserver.observe(resultsEl.value)
  }
  if (keyword.value.trim()) void run()
})
onBeforeUnmount(() => resultsObserver?.disconnect())
</script>
<template>
  <Modal title="搜索" width="760px" @close="emit('close')">
    <MotionConfig :reduced-motion="reducedMotion">
      <motion.div class="global-search" :initial="compactReveal.initial" :animate="compactReveal.animate" :transition="compactReveal.transition">
        <form class="global-search-form" @submit.prevent="run">
          <UiIcon name="search" :size="17" class="global-search-icon" />
          <input ref="inputEl" class="input" v-model="keyword" aria-label="跨盘搜索名称" :placeholder="remote ? '搜索网盘文件' : '搜索已缓存文件'" autofocus @keydown="onSearchKeydown" />
          <button class="btn primary" :disabled="submitDisabled">{{ busy && matchesSubmission ? '搜索中' : '搜索' }}</button>
        </form>
        <div class="global-search-options">
          <label class="global-search-toggle"><input type="checkbox" v-model="remote" /><span>同时在线搜索</span></label>
          <span v-if="!remote">仅搜索本机已缓存的文件</span>
        </div>
        <div v-if="busy" class="global-search-status" role="status"><span class="spin" aria-hidden="true"></span><span>正在搜索「{{ submittedQuery }}」…</span><button class="tbtn" type="button" @click="stop">停止</button></div>
        <div v-else-if="submittedQuery && !matchesSubmission" class="global-search-status">当前显示「{{ submittedQuery }}」的{{ submittedRemote ? '缓存和在线' : '缓存' }}结果</div>
        <div v-for="error in errors" :key="error" class="global-search-error" role="alert"><UiIcon name="warning" :size="14" /><span>{{ error }}</span></div>
        <details v-if="accountStates.length" class="global-search-accounts">
          <summary>网盘查询状态（{{ accountStates.length }} 个账号）</summary>
          <div class="global-search-account-list">
            <div v-for="state in accountStates" :key="accountKey(state.userId, state.driveId)" class="global-search-status">{{ state.name }}：{{ state.status }}</div>
          </div>
        </details>
        <div v-if="ordered.length" class="global-search-count">{{ ordered.length.toLocaleString() }} 项结果</div>
        <div ref="resultsEl" class="workspace-results global-search-results" :class="{ 'has-results': ordered.length }" role="listbox" aria-label="搜索结果" @scroll.passive="syncScroll">
          <div v-if="ordered.length" class="global-search-track" :style="{ height: `${ordered.length * rowHeight}px` }">
            <button v-for="{ result, index } in visibleRows" :key="resultKey(result)" class="workspace-result" role="option" :data-result-index="index" :style="{ top: `${index * rowHeight}px`, height: `${rowHeight}px` }" :tabindex="index === (focusedIndex >= 0 ? focusedIndex : firstVisibleIndex) ? 0 : -1" :aria-selected="false" :aria-posinset="index + 1" :aria-setsize="ordered.length" :title="result.file.name" @focus="focusedKey = resultKey(result)" @blur="focusedKey = ''" @click="open(result)" @keydown="onResultKeydown($event, index)">
              <img :src="accountDisplay.get(accountKey(result.userId, result.driveId)).icon" alt="" />
              <span><strong>{{ result.file.name }}</strong><small :title="result.parentId">{{ accountDisplay.get(accountKey(result.userId, result.driveId)).name }} · {{ result.parentId || '路径由网盘提供' }} · {{ result.source }} · {{ new Date(result.updatedAt*1000).toLocaleString() }}</small></span><UiIcon name="chevron-right" :size="14" />
            </button>
          </div>
          <div v-if="!busy && !ordered.length && !errors.length" class="workspace-empty-state global-search-empty">
            <UiIcon name="search" :size="22" />
            <span>{{ stopped ? '搜索已停止，可重新搜索' : submittedQuery ? '没有匹配的文件' : '输入关键词搜索文件' }}</span>
          </div>
        </div>
      </motion.div>
    </MotionConfig>
  </Modal>
</template>

<style scoped>
.global-search { display: grid; gap: 12px; min-height: 0; }
.global-search-form { display: flex; align-items: center; gap: 9px; padding: 6px 7px 6px 12px; border: 1px solid var(--control-border); border-radius: var(--radius-md); background: var(--control-bg); }
.global-search-form:focus-within { border-color: var(--border-focus); box-shadow: var(--ring-focus); }
.global-search-form .input { min-width: 0; flex: 1; height: 30px; border: 0; box-shadow: none; background: transparent; }
.global-search-form .input:focus { box-shadow: none; }
.global-search-form .btn { min-width: 72px; }
.global-search-icon { flex: 0 0 auto; color: var(--color-primary); }
.global-search-options { display: flex; align-items: center; justify-content: space-between; gap: 10px; color: var(--text-tertiary); font-size: 12px; }
.global-search-toggle { display: inline-flex; align-items: center; gap: 7px; color: var(--text-secondary); cursor: pointer; }
.global-search-toggle input { accent-color: var(--color-primary); }
.global-search-status, .global-search-error { display: flex; align-items: center; gap: 8px; min-height: 32px; padding: 7px 9px; border-radius: var(--radius-sm); background: var(--bg-subtle); color: var(--text-secondary); font-size: 12.5px; }
.global-search-status .tbtn { margin-left: auto; }
.global-search-error { color: var(--color-error); background: color-mix(in srgb, var(--color-error) 8%, transparent); }
.global-search-accounts { font-size: 12px; color: var(--text-secondary); }
.global-search-accounts summary { cursor: pointer; }
.global-search-account-list { max-height: 132px; overflow: auto; display: grid; gap: 4px; margin-top: 6px; }
.global-search-count { font-size: 12px; color: var(--text-secondary); font-variant-numeric: tabular-nums; }
.global-search-results { max-height: min(420px, 46vh); min-width: 0; overflow: auto; overflow-anchor: none; border-top: 1px solid var(--border-lighter); }
.global-search-results.has-results { animation: search-results-in var(--motion-normal) var(--motion-ease); }
.global-search-track { position: relative; }
@keyframes search-results-in { from { opacity: .65; } }
.global-search-results .workspace-result { position: absolute; left: 0; box-sizing: border-box; padding-block: 6px; transition: background var(--motion-fast) var(--motion-ease); }
.global-search-results .workspace-result strong, .global-search-results .workspace-result small { display: block; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
.global-search-results .workspace-result img, .global-search-results .workspace-result svg { flex-shrink: 0; }
.global-search-results .workspace-result:focus-visible {
  outline: none;
  background: var(--listselectbg);
  box-shadow: inset 3px 0 0 color-mix(in srgb, var(--color-primary) 72%, transparent);
}
.global-search-empty { display: grid; justify-items: center; gap: 9px; min-height: 160px; color: var(--text-tertiary); }
.global-search-empty svg { color: var(--color-primary); opacity: .7; }
@media (prefers-reduced-motion: reduce) { .global-search-results.has-results { animation: none; } }
@media (max-width: 560px) { .global-search-options > span { display: none; } .global-search-form .btn { min-width: 64px; } }
</style>
