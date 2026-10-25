<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ListDownloads, ListUploads, ListMigrateJobs, accountName, formatBytes } from '../api'
import { accountHealth, healthLabels } from '../workspace'
import { recentItems, recentEnabled } from '../recent-items'
import Modal from './Modal.vue'

const props = defineProps({ accounts: { type: Array, default: () => [] } })
const emit = defineEmits(['close', 'select', 'navigate', 'tasks'])
const downloads = ref([]), uploads = ref([]), migrations = ref([]), loading = ref(false), failedSources = ref([])
let disposed = false
const spaces = computed(() => new Set(props.accounts.map(account => JSON.stringify([account.user_id, account.drive_id]))))
function available(user, drive) { return spaces.value.has(JSON.stringify([user, drive])) }
const localDownloads = computed(() => downloads.value.filter(task => available(task.user_id, task.drive_id)))
const localUploads = computed(() => uploads.value.filter(task => available(task.user_id, task.Info?.drive_id)))
const localMigrations = computed(() => migrations.value.filter(task => available(task.srcUser, task.srcDrive) || available(task.dstUser, task.dstDrive)))
const pending = computed(() => localDownloads.value.filter(task => !['completed', 'canceled'].includes(task.status)).length + localUploads.value.filter(task => !task.Upload?.IsCompleted).length + localMigrations.value.filter(task => !['completed', 'canceled'].includes(task.status)).length)
const failures = computed(() => [
  ...localDownloads.value.filter(task => task.status === 'failed').map(task => ({ key: `download:${task.id}`, name: task.name, message: task.issue?.message || '下载失败，请查看任务详情' })),
  ...localUploads.value.filter(task => task.Upload?.IsFailed || task.Upload?.issue?.code === 'UPLOAD_RESULT_UNKNOWN').map(task => ({ key: `upload:${task.UploadID}`, name: task.Info?.name, message: task.Upload?.issue?.message || '上传失败，请核对目标目录' })),
  ...localMigrations.value.filter(task => ['failed', 'partial'].includes(task.status)).map(task => ({ key: `migration:${task.id}`, name: '跨盘迁移', message: '迁移未全部完成，请查看任务详情' })),
])
const recent = computed(() => recentEnabled.value ? recentItems.value.filter(item => available(item.userId, item.driveId)).slice(0, 8) : [])
function status(account) {
  if (account.disabled) return '已停用'
  if (account.auth_state === 'reauth_required') return '需要重新登录'
  return healthLabels[accountHealth[account.user_id]?.status] || '尚未检查'
}
function capacity(account) {
  const usage = account.usage
  if (usage?.status !== 'available' || !Number.isFinite(usage.size) || usage.size <= 0 || !Number.isFinite(usage.used)) return '容量未知'
  return `${formatBytes(usage.used)} / ${formatBytes(usage.size)}`
}
async function refresh() {
  if (loading.value) return
  loading.value = true
  const names = ['下载记录', '上传记录', '迁移记录']
  const results = await Promise.allSettled([Promise.resolve().then(ListDownloads), Promise.resolve().then(ListUploads), Promise.resolve().then(ListMigrateJobs)])
  if (disposed) return
  const targets = [downloads, uploads, migrations]
  failedSources.value = []
  results.forEach((result, index) => {
    if (result.status === 'fulfilled') targets[index].value = Array.isArray(result.value) ? result.value : []
    else failedSources.value.push(names[index])
  })
  loading.value = false
}
onMounted(refresh)
onBeforeUnmount(() => { disposed = true })
</script>

<template>
  <Modal title="我的空间" width="760px" @close="emit('close')">
    <p class="overview-note" title="账号状态与任务统计来自本机记录，非实时检测">本机概览</p>
    <p v-if="failedSources.length" role="alert">{{ failedSources.join('、') }}读取失败，统计不完整。</p>
    <div class="overview-summary"><span>{{ accounts.length }} 个账号</span><span>{{ pending }} 项待处理</span><span>{{ failures.length }} 项异常</span></div>
    <div class="overview-accounts">
      <button v-for="account in accounts" :key="account.user_id + '/' + account.drive_id" class="overview-account" @click="emit('select', account)"><strong>{{ accountName(account) }}</strong><span>{{ status(account) }}</span><span>{{ capacity(account) }}</span></button>
      <p v-if="!accounts.length">尚未添加网盘账号。</p>
    </div>
    <h3>最近使用</h3>
    <div class="overview-recent"><button v-for="item in recent" :key="JSON.stringify([item.userId, item.driveId, item.fileId])" class="btn sm" @click="emit('navigate', item)">{{ item.name }}</button><span v-if="!recent.length">{{ recentEnabled ? '暂无本机记录' : '最近使用记录已暂停' }}</span></div>
    <h3>待处理异常</h3>
    <ul class="overview-failures"><li v-for="failure in failures.slice(0, 6)" :key="failure.key"><strong>{{ failure.name || '未命名任务' }}</strong> · {{ failure.message }}</li><li v-if="!failures.length">暂无异常任务</li></ul>
    <template #actions><button class="btn" :disabled="loading" @click="refresh">{{ loading ? '读取中…' : '刷新' }}</button><button class="btn primary" @click="emit('tasks')">查看任务</button></template>
  </Modal>
</template>

<style scoped>
.overview-note { color: var(--text-secondary); font-size: 12px; margin-bottom: 14px; }
.overview-summary { display: flex; flex-wrap: wrap; gap: 20px; margin-bottom: 14px; }
.overview-accounts { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 8px; max-height: 240px; overflow: auto; }
.overview-account { display: grid; gap: 4px; padding: 12px; border: 1px solid var(--border-light); border-radius: var(--radius-md); background: var(--bg-base); color: var(--text-primary); text-align: left; cursor: pointer; overflow-wrap: anywhere; }
.overview-account span { color: var(--text-secondary); font-size: 12px; }
h3 { font-size: 13px; margin: 18px 0 8px; }
.overview-recent { display: flex; flex-wrap: wrap; gap: 8px; }
.overview-recent button { max-width: 100%; overflow-wrap: anywhere; white-space: normal; }
.overview-failures { padding-left: 18px; font-size: 12px; color: var(--text-secondary); }
.overview-failures li { margin-bottom: 6px; overflow-wrap: anywhere; }
</style>
