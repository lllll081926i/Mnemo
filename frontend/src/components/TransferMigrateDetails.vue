<script setup>
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { VerifyMigration } from '../api'
import { taskErrorText } from '../task-notices'
import Modal from './Modal.vue'

const props = defineProps({
  job: { type: Object, required: true },
  sourceName: String,
  sourceIcon: String,
  targetName: String,
  targetIcon: String,
  statusLabel: { type: Function, required: true },
})
const emit = defineEmits(['close', 'toast', 'navigate', 'resume'])
const verificationLabels = { size: '传输大小已核对', 'transferred-size': '传输大小已核对', 'remote-size': '目标大小已核对', 'remote-hash': '目标哈希已核对', 'provider-hash': '网盘已校验哈希' }
const pageSize = 50
const mode = ref('files')
const filter = ref('')
const page = ref(1)
const list = ref(null)
const verifying = ref(false)
const verification = shallowRef(null)
let verificationSeq = 0
const canVerify = computed(() => !['running', 'pending'].includes(props.job.status))
const rows = computed(() => mode.value === 'results' ? verification.value || [] : Object.values(props.job.items || {}))
const resultName = item => props.job.items?.[item.id]?.name || item.id
const filtered = computed(() => {
  const term = filter.value.trim().toLowerCase()
  if (!term) return rows.value
  return rows.value.filter(item => String(mode.value === 'results' ? resultName(item) : item.name || item.id).toLowerCase().includes(term))
})
const pageCount = computed(() => Math.max(1, Math.ceil(filtered.value.length / pageSize)))
const currentPage = computed(() => Math.min(page.value, pageCount.value))
const visible = computed(() => filtered.value.slice((currentPage.value - 1) * pageSize, currentPage.value * pageSize))
watch([mode, filter], () => { page.value = 1 })
watch(pageCount, count => { if (page.value > count) page.value = count })
watch([mode, filter, currentPage], () => { if (list.value) list.value.scrollTop = 0 }, { flush: 'post' })
onBeforeUnmount(() => { verificationSeq++ })

async function verify() {
  if (!canVerify.value || verifying.value) return
  const seq = ++verificationSeq
  verifying.value = true
  try {
    const result = await VerifyMigration(props.job.id)
    if (seq !== verificationSeq) return
    verification.value = Array.isArray(result) ? result : []
    filter.value = ''
    page.value = 1
    mode.value = 'results'
  } catch (error) {
    if (seq === verificationSeq) emit('toast', String(error), 'error')
  } finally {
    if (seq === verificationSeq) verifying.value = false
  }
}
</script>

<template>
  <Modal title="迁移详情" width="760px" @close="emit('close')">
    <p class="account-inline"><img :src="sourceIcon" alt="" />{{ sourceName }} → <img :src="targetIcon" alt="" />{{ targetName }}</p>
    <p>目标目录：{{ job.dstParent }} · {{ statusLabel(job.status) }}</p>
    <div class="migration-detail-toolbar">
      <div v-if="verification !== null" role="group" aria-label="迁移详情视图">
        <button type="button" class="btn sm" :aria-pressed="mode === 'files'" @click="mode = 'files'">文件记录</button>
        <button type="button" class="btn sm" :aria-pressed="mode === 'results'" @click="mode = 'results'">校验结果</button>
      </div>
      <input v-model="filter" class="input" aria-label="按文件名筛选" placeholder="按文件名筛选" />
    </div>
    <div ref="list" class="workspace-results migration-detail-list" role="region" :aria-label="mode === 'files' ? '迁移文件记录' : '迁移校验结果'" tabindex="0">
      <div v-if="visible.length" :key="`${mode}:${currentPage}`" class="content-reveal" role="list">
        <div v-for="item in visible" :key="item.id" class="workspace-result" role="listitem">
          <span v-if="mode === 'files'">
            <strong>{{ item.name || item.id }}</strong>
            <small>{{ statusLabel(item.status) }} · {{ verificationLabels[item.verification] || '未校验' }}</small>
            <small v-if="item.error">{{ taskErrorText(item.error, '迁移文件') }}</small>
          </span>
          <span v-else>
            <strong>{{ resultName(item) }}</strong>
            <small :class="{ 'migration-mismatch': item.status === 'mismatch' }">{{ item.detail }}</small>
          </span>
        </div>
      </div>
      <p v-else class="migration-detail-empty">{{ filter.trim() ? '没有匹配的文件' : mode === 'results' ? '没有可校验的文件' : '任务尚未记录子项。' }}</p>
    </div>
    <div class="migration-detail-pagination" aria-label="迁移详情分页">
      <span role="status">{{ filtered.length }} 项 · 第 {{ currentPage }} / {{ pageCount }} 页</span>
      <button type="button" class="btn sm" :aria-disabled="currentPage === 1" @click="page = Math.max(1, currentPage - 1)">上一页</button>
      <button type="button" class="btn sm" :aria-disabled="currentPage === pageCount" @click="page = Math.min(pageCount, currentPage + 1)">下一页</button>
    </div>
    <p class="hint">大小核对仅比对传输字节；网盘校验由服务端直接确认，未二次读取校验。</p>
    <button type="button" class="btn" :disabled="!canVerify" :aria-disabled="!canVerify || verifying" :aria-busy="verifying" @click="verify">{{ verifying ? '校验中…' : '校验文件完整性' }}</button>
    <template #actions>
      <button class="btn" @click="emit('navigate', { userId: job.dstUser, dirId: job.dstParent, name: '迁移目标' })">打开目标目录</button>
      <button v-if="['failed', 'partial', 'canceled'].includes(job.status)" class="btn primary" @click="emit('resume', job)">重试未完成项</button>
    </template>
  </Modal>
</template>

<style scoped>
.migration-detail-toolbar, .migration-detail-pagination { display:flex; flex-wrap:wrap; align-items:center; gap:8px; margin-block:8px; }
.migration-detail-toolbar .input { flex:1; min-width:140px; }
.migration-detail-toolbar [role=group] { display:flex; gap:4px; }
.migration-detail-toolbar [aria-pressed=true] { background:var(--bg-active); }
.migration-detail-list { height:min(42vh, 360px); border:1px solid var(--border-lighter); border-radius:var(--radius-md); scrollbar-gutter:stable; }
.migration-detail-empty { padding:10px; color:var(--text-secondary); }
.migration-detail-pagination span { margin-right:auto; color:var(--text-secondary); font-size:12px; }
.migration-detail-pagination [aria-disabled=true] { opacity:.5; cursor:default; }
.migration-mismatch { color:var(--color-error); }
</style>
