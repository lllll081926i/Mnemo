<script setup>
import { computed, ref, watch, onMounted, onBeforeUnmount } from 'vue'
import { listShareImportReceipts, refreshShareImportReceipt, accountName, onEvent, copyText } from '../api'

const props = defineProps({ accounts: { type: Array, default: () => [] }, latest: { type: Object, default: null } })
const emit = defineEmits(['toast', 'navigate'])
const receipts = ref([])
const loadError = ref('')
const loading = ref(false)
const checking = ref(new Set())
const checkedLatest = ref(null)
let sequence = 0
const keyOf = receipt => JSON.stringify([receipt.accountId, receipt.driveId, receipt.id])
const visible = computed(() => {
  const scoped = receipts.value.filter(receipt => props.accounts.some(account => account.user_id === receipt.accountId && (account.drive_id || '') === receipt.driveId))
  const entries = new Map(scoped.map(receipt => [keyOf(receipt), receipt]))
  if (props.latest && checkedLatest.value !== props.latest && props.accounts.some(account => account.user_id === props.latest.accountId && (account.drive_id || '') === props.latest.driveId)) {
    const previous = entries.get(keyOf(props.latest))
    if (!previous || previous.updatedAt <= props.latest.updatedAt || !props.latest.historySaved) entries.set(keyOf(props.latest), props.latest)
  }
  return [...entries.values()].sort((left, right) => right.updatedAt - left.updatedAt)
})

function statusOf(receipt) {
  const items = receipt.items || []
  const count = status => items.filter(item => item.status === status).length
  const completed = count('completed')
  const failed = count('failed')
  const submitted = count('submitted')
  return `已完成 ${completed} · 已受理 ${submitted} · 失败 ${failed} · 待核对 ${items.length - completed - failed - submitted}`
}

async function load() {
  const current = ++sequence
  loading.value = true
  loadError.value = ''
  const scopes = props.accounts.map(account => ({ userId: account.user_id, driveId: account.drive_id || '' }))
  const results = await Promise.allSettled(scopes.map(scope => listShareImportReceipts(scope.userId, scope.driveId)))
  if (current !== sequence) return
  const entries = new Map(receipts.value.map(receipt => [keyOf(receipt), receipt]))
  results.forEach((result, index) => {
    if (result.status === 'rejected') {
      loadError.value = '部分转存回执加载失败，已保留原记录；可重新加载'
      return
    }
    const scope = scopes[index]
    for (const [key, receipt] of entries) if (receipt.accountId === scope.userId && receipt.driveId === scope.driveId) entries.delete(key)
    for (const receipt of result.value || []) entries.set(keyOf(receipt), receipt)
  })
  receipts.value = [...entries.values()]
  loading.value = false
}

async function check(receipt) {
  const key = keyOf(receipt)
  if (checking.value.has(key)) return
  checking.value = new Set([...checking.value, key])
  try {
    const updated = await refreshShareImportReceipt(receipt.accountId, receipt.driveId, receipt.id)
    receipts.value = [...receipts.value.filter(item => keyOf(item) !== key), updated]
    if (props.latest && keyOf(props.latest) === key) checkedLatest.value = props.latest
    if (updated.message) emit('toast', updated.message, 'warn')
  } catch (error) {
    emit('toast', String(error?.message || error), 'error')
  } finally {
    checking.value = new Set([...checking.value].filter(item => item !== key))
  }
}

async function copyReceipt(receipt) {
  try {
    const copied = await copyText(JSON.stringify(receipt, null, 2))
    emit('toast', copied ? '已复制转存回执' : '复制失败', copied ? 'success' : 'error')
  } catch (error) {
    emit('toast', '复制失败', 'error')
  }
}

watch(() => props.accounts.map(account => [account.user_id, account.drive_id]), load, { immediate: true, deep: true })
const offs = []
onMounted(() => offs.push(onEvent('share:imports-changed', load)))
onBeforeUnmount(() => { ++sequence; offs.forEach(off => off?.()) })
</script>

<template>
  <details v-if="visible.length || loadError" class="share-import-history" open>
    <summary>本机转存回执 · {{ visible.length }} 批</summary>
    <p class="hint">仅记录本机提交，不代表完整云端历史。未知结果不会自动重传；目标目录可能仍在处理。</p>
    <p v-if="loadError" role="alert">{{ loadError }} <button class="tbtn" :disabled="loading" @click="load">重新加载</button></p>
    <div v-for="receipt in visible" :key="keyOf(receipt)" class="share-record">
      <div class="share-record-main">
        <strong>{{ accountName(accounts.find(account => account.user_id === receipt.accountId)) }} · {{ receipt.items?.length || 0 }} 项</strong>
        <p role="status">{{ statusOf(receipt) }}</p>
        <p v-if="receipt.message" class="hint">{{ receipt.message }}</p>
        <p v-if="!receipt.historySaved" role="alert">最终回执未保存，请先保留此窗口并核对目标目录。</p>
      </div>
      <div class="share-record-actions">
        <button class="tbtn" :disabled="checking.has(keyOf(receipt))" @click="check(receipt)">核对状态</button>
        <button class="tbtn" @click="copyReceipt(receipt)">复制回执</button>
        <button class="tbtn" @click="emit('navigate', { userId: receipt.accountId, dirId: receipt.parentId, name: '转存目录' })">打开目录</button>
      </div>
    </div>
  </details>
</template>

<style scoped>
.share-import-history { padding: 14px 16px; margin-bottom: 12px; border: 1px solid var(--border); border-radius: 10px; }
.share-import-history summary { cursor: pointer; font-weight: 600; }
.share-import-history p { margin: 6px 0; }
</style>
