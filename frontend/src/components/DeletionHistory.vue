<script setup>
import { ref, onMounted } from 'vue'
import Modal from './Modal.vue'
import ConfirmModal from './ConfirmModal.vue'
import { formatBytes, formatTime } from '../api'

const props = defineProps({ account: { type: Object, required: true } })
const emit = defineEmits(['close', 'toast'])
const records = ref([]), snapshot = ref(null), busy = ref(false), error = ref(''), confirmingClear = ref(false)
const account = { userId: props.account.user_id, driveId: props.account.drive_id }
const bridge = () => window.go.app.App
const statusText = status => ({ delete_pending: '删除中', delete_unknown: '删除状态未知', deleted: '已删除', restore_pending: '恢复中', restore_unknown: '恢复状态未知', restored: '已恢复' }[status] || '状态未知')
async function load() {
  busy.value = true
  try { records.value = await bridge().ListDeletionHistory(account.userId, account.driveId) || []; error.value = '' }
  catch { error.value = '读取本地删除记录失败，已保留原记录' }
  finally { busy.value = false }
}
async function restore(record) {
  busy.value = true
  try {
    await bridge().RestoreDeletionHistory(account.userId, account.driveId, record.fileId)
    emit('toast', '已恢复文件', 'success')
  } catch (failure) { error.value = String(failure) }
  finally {
    const failureMessage = error.value
    await load()
    if (failureMessage) error.value = failureMessage
    busy.value = false
  }
}
async function openWebsite() {
  try { await bridge().OpenTrashWebsite(account.userId, account.driveId) }
  catch (failure) { error.value = String(failure) }
}
async function preview() {
  busy.value = true
  try { snapshot.value = await bridge().PreviewEnterpriseTrash(account.userId, account.driveId); error.value = '' }
  catch (failure) { error.value = `读取失败，当前快照未更新：${String(failure)}` }
  finally { busy.value = false }
}
async function clearLocal() {
  confirmingClear.value = false
  busy.value = true
  try { await bridge().ClearDeletionHistory(account.userId, account.driveId); await load() }
  catch (failure) { error.value = String(failure) }
  finally { busy.value = false }
}
onMounted(load)
</script>

<template>
  <Modal title="本机删除记录" width="780px" @close="emit('close')">
    <div class="deletion-history">
      <p>仅记录在 Mnemo 中执行的删除，不是完整云端回收站。网页端删除及历史文件请前往官网查看。</p>
      <div class="deletion-actions">
        <button class="btn" :disabled="busy" @click="load">刷新记录</button>
        <button class="btn" @click="openWebsite">打开官网回收站</button>
        <button v-if="['business', 'documentLibrary'].includes(props.account.token?.provider_drive_type)" class="btn" :disabled="busy" @click="preview">试验读取一级回收站（只读）</button>
        <button class="btn danger" :disabled="busy || !records.length" @click="confirmingClear = true">清除本机记录</button>
      </div>
      <p v-if="error" role="alert">{{ error }}</p>
      <div v-if="busy" role="status">正在处理…</div>
      <ul class="deletion-list">
        <li v-for="record in records" :key="record.fileId">
          <div><strong>{{ record.name }}</strong><small>{{ formatTime(record.deletedAt) }} · {{ formatBytes(record.size) }} · {{ statusText(record.status) }}</small></div>
          <button v-if="record.providerType === 'personal' && record.status !== 'restored'" class="btn" :disabled="busy" @click="restore(record)">{{ ['restore_pending', 'restore_unknown'].includes(record.status) ? '核对恢复状态' : '恢复' }}</button>
        </li>
      </ul>
      <p v-if="!records.length && !busy">当前账号暂无本地删除记录。</p>
      <section v-if="snapshot">
        <h3>企业回收站快照</h3>
        <p>{{ snapshot.scope }}。当前仅提供只读查看，不支持直接恢复。</p>
        <ul class="deletion-list"><li v-for="item in snapshot.items" :key="item.siteId + item.id"><div><strong>{{ item.name }}</strong><small>{{ item.location }} · {{ item.deletedAt }}</small></div></li></ul>
      </section>
    </div>
    <ConfirmModal v-if="confirmingClear" title="清除本机删除记录" message="仅清除本地记录，不影响云端文件。清除后将无法从本应用直接恢复这些文件。" ok-text="清除记录" danger @ok="clearLocal" @cancel="confirmingClear = false" />
  </Modal>
</template>

<style scoped>
.deletion-history { display: grid; gap: 12px; max-height: 65vh; overflow: auto; }
.deletion-history p { color: var(--text-secondary); font-size: 13px; line-height: 1.6; }
.deletion-actions { display: flex; flex-wrap: wrap; gap: 8px; }
.deletion-list { list-style: none; padding: 0; margin: 0; }
.deletion-list li { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 10px 0; border-bottom: 1px solid var(--border-lighter); }
.deletion-list li div { min-width: 0; overflow-wrap: anywhere; }
.deletion-list small { display: block; color: var(--text-tertiary); margin-top: 4px; }
</style>
