<script setup>
import { ref, computed, onBeforeUnmount } from 'vue'
import { motion, MotionConfig } from 'motion-v'
import { search, capsOf, accountName, providerMetaOf, providerIconUrl } from '../api'
import Modal from './Modal.vue'
import UiIcon from './UiIcon.vue'
import { compactReveal, reducedMotion } from '../motion/presets'
const props = defineProps({ accounts: Array, providers: Array })
const emit = defineEmits(['close', 'navigate'])
const keyword = ref(''), results = ref([]), errors = ref([]), busy = ref(false), remote = ref(false), stopped = ref(false)
const accountStates = ref([]), visibleCount = ref(200)
const activeRequests = new Set()
const searchSession = globalThis.crypto?.randomUUID?.() || `${Date.now()}-${Math.random()}`
let epoch = 0
function cancelRequests() {
  for (const requestId of activeRequests) {
    Promise.resolve(window.go?.app?.App?.CancelSearch?.(requestId)).catch(() => {})
  }
  activeRequests.clear()
}
onBeforeUnmount(() => { ++epoch; cancelRequests() })
const ordered = computed(() => {
  const rank = new Map(props.accounts.map((a, i) => [a.user_id, i]))
  return results.value.filter(r => rank.has(r.userId)).sort((a,b) => rank.get(a.userId) - rank.get(b.userId))
})
function account(id) { return props.accounts.find(a => a.user_id === id) }
async function run() {
  cancelRequests()
  const query = keyword.value.trim()
  const seq = ++epoch
  stopped.value = false
  results.value = []; errors.value = []; accountStates.value = []; visibleCount.value = 200
  if (!query) { busy.value = false; return }
  busy.value = true
  let cachedResults = []
  const onlineResults = []
  const updateResults = () => {
    if (seq !== epoch) return
    const onlineKeys = new Set(onlineResults.map(result => `${result.userId}\u0000${result.driveId}\u0000${result.file.file_id}`))
    results.value = [
      ...cachedResults.filter(result => !onlineKeys.has(`${result.userId}\u0000${result.driveId}\u0000${result.file.file_id}`)),
      ...onlineResults,
    ]
  }
  const cachedTask = Promise.resolve().then(() => window.go.app.App.SearchCachedFiles(query)).then(cached => {
    if (seq !== epoch) return
    cachedResults = (cached || []).map(result => ({ ...result, source: result.partial ? '缓存（部分目录）' : '缓存' }))
    if ((cached || []).length >= 1000) errors.value.push('缓存结果达到 1000 条显示上限，请缩小搜索范围')
    updateResults()
  }).catch(() => { if (seq === epoch) errors.value.push('缓存搜索失败，请重试') })
  let onlineAccounts = []
  const worker = async () => {
    while (onlineAccounts.length && seq === epoch) {
      const acc = onlineAccounts.shift()
      if (!props.accounts.some(current => current.user_id === acc.user_id && current.drive_id === acc.drive_id)) continue
      const state = accountStates.value.find(item => item.userId === acc.user_id && item.driveId === acc.drive_id)
      const requestId = `${searchSession}:${seq}:${acc.user_id}:${acc.drive_id}`
      activeRequests.add(requestId)
      if (state) state.status = '搜索中'
      try {
        const files = await search(acc.user_id, acc.drive_id, query, requestId)
        if (seq !== epoch) return
        if (state) state.status = `在线查询完成 · ${(files || []).length} 项`
        onlineResults.push(...(files || []).map(file => ({ userId: acc.user_id, driveId: acc.drive_id, parentId: file.parent_file_id, file, source: '在线', updatedAt: Date.now() / 1000 })))
        updateResults()
      } catch {
        if (seq === epoch) {
          if (state) state.status = '失败，结果不完整'
          errors.value.push(`${accountName(acc)}：在线搜索失败`)
        }
      } finally { activeRequests.delete(requestId) }
    }
  }
  try {
    if (remote.value) {
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
  emit('navigate', { userId: result.userId, dirId: result.file.isDir ? result.file.file_id : result.parentId, name: result.file.isDir ? result.file.name : '所在目录', file: result.file })
  emit('close')
}
</script>
<template>
  <Modal title="搜索" width="760px" @close="emit('close')">
    <MotionConfig :reduced-motion="reducedMotion">
      <motion.div class="global-search" :initial="compactReveal.initial" :animate="compactReveal.animate" :transition="compactReveal.transition">
        <form class="global-search-form" @submit.prevent="run">
          <UiIcon name="search" :size="17" class="global-search-icon" />
          <input class="input" v-model="keyword" aria-label="跨盘搜索名称" placeholder="搜索已缓存文件，或开启在线搜索" autofocus />
          <button class="btn primary" :disabled="busy">{{ busy ? '搜索中' : '搜索' }}</button>
        </form>
        <div class="global-search-options">
          <label class="global-search-toggle"><input type="checkbox" v-model="remote" /><span>同时在线搜索</span></label>
          <span v-if="!remote">当前仅搜索本机缓存，未浏览过的目录可能缺失</span>
          <span v-else>在线搜索范围由网盘提供；结果分批显示，不截断查询结果</span>
        </div>
        <div v-if="busy" class="global-search-status" role="status"><span class="spin"></span><span>正在搜索…</span><button class="tbtn" type="button" @click="stop">停止</button></div>
        <div v-for="error in errors" :key="error" class="global-search-error" role="alert"><UiIcon name="warning" :size="14" /><span>{{ error }}</span></div>
        <div v-for="state in accountStates" :key="state.userId + state.driveId" class="global-search-status" role="status">{{ state.name }}：{{ state.status }}</div>
        <div class="workspace-results global-search-results">
          <button v-for="r in ordered.slice(0, visibleCount)" :key="r.userId + r.driveId + r.file.file_id" class="workspace-result" @click="open(r)">
            <img :src="providerIconUrl(providerMetaOf(account(r.userId), providers))" alt="" />
            <span><strong>{{ r.file.name }}</strong><small>{{ accountName(account(r.userId)) }} · {{ r.parentId || '路径由网盘提供' }} · {{ r.source }} · {{ new Date(r.updatedAt*1000).toLocaleString() }}</small></span><UiIcon name="chevron-right" :size="14" />
          </button>
          <button v-if="ordered.length > visibleCount" class="btn" type="button" @click="visibleCount += 200">显示更多（已显示 {{ visibleCount }} / {{ ordered.length }} 项）</button>
          <div v-if="!busy && !ordered.length && !errors.length" class="workspace-empty-state global-search-empty">
            <UiIcon name="search" :size="22" />
            <span>{{ stopped ? '搜索已停止，可重新搜索' : keyword ? '没有匹配的文件' : '输入名称，查找散落在各个网盘的文件' }}</span>
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
.global-search-icon { flex: 0 0 auto; color: var(--color-primary); }
.global-search-options { display: flex; align-items: center; justify-content: space-between; gap: 10px; color: var(--text-tertiary); font-size: 12px; }
.global-search-toggle { display: inline-flex; align-items: center; gap: 7px; color: var(--text-secondary); cursor: pointer; }
.global-search-toggle input { accent-color: var(--color-primary); }
.global-search-status, .global-search-error { display: flex; align-items: center; gap: 8px; min-height: 32px; padding: 7px 9px; border-radius: var(--radius-sm); background: var(--bg-subtle); color: var(--text-secondary); font-size: 12.5px; }
.global-search-status .tbtn { margin-left: auto; }
.global-search-error { color: var(--color-error); background: color-mix(in srgb, var(--color-error) 8%, transparent); }
.global-search-results { max-height: min(420px, 46vh); overflow-y: auto; border-top: 1px solid var(--border-lighter); }
.global-search-results .workspace-result { transition: background var(--motion-fast) var(--motion-ease), transform var(--motion-fast) var(--motion-spring); }
.global-search-results .workspace-result:hover { transform: translateX(2px); }
.global-search-empty { display: grid; justify-items: center; gap: 9px; min-height: 160px; color: var(--text-tertiary); }
.global-search-empty svg { color: var(--color-primary); opacity: .7; }
@media (max-width: 560px) { .global-search-options > span { display: none; } .global-search-form .btn { min-width: 64px; } }
</style>
