<script setup>
import { computed, ref, watch } from 'vue'
import { RunDiagnostics, ExportDiagnostics, listAccounts, accountName } from '../api'
import UiSelect from './UiSelect.vue'

const emit = defineEmits(['toast'])
const report = ref(null), busy = ref(false), exporting = ref(false), network = ref(false), accounts = ref([]), selected = ref('')
const expanded = ref(false), accountsLoading = ref(false)
const options = computed(() => [{ value: '', label: '仅检查本机' }, ...accounts.value.map(account => ({ value: JSON.stringify([account.user_id, account.drive_id]), label: accountName(account), disabled: account.disabled }))])
const labels = { passed: '通过', failed: '异常', skipped: '未检测', info: '说明' }
async function loadAccounts() {
  if (accountsLoading.value) return
  accountsLoading.value = true
  try {
    accounts.value = await listAccounts() || []
    if (!options.value.some(option => option.value === selected.value && !option.disabled)) selected.value = ''
  }
  catch { emit('toast', '本机账号列表读取失败', 'error') }
  finally { accountsLoading.value = false }
}
function togglePanel(event) {
  expanded.value = event.target.open
  if (expanded.value) void loadAccounts()
}
watch([selected, network], () => { report.value = null })
async function run() {
  if (busy.value || accountsLoading.value) return
  busy.value = true
  try {
    const [user, drive] = selected.value ? JSON.parse(selected.value) : ['', '']
    report.value = await RunDiagnostics(user, drive, network.value)
  } catch { emit('toast', '诊断未完成，请检查本机存储或稍后重试', 'error') }
  finally { busy.value = false }
}
async function exportReport() {
  if (exporting.value) return
  exporting.value = true
  try { if (await ExportDiagnostics()) emit('toast', '已导出脱敏诊断', 'success') }
  catch { emit('toast', '诊断导出失败，请检查目标目录权限', 'error') }
  finally { exporting.value = false }
}
</script>

<template>
  <details class="diagnostics-panel" @toggle="togglePanel">
    <summary>诊断工具</summary>
    <div v-if="expanded">
      <div class="diagnostics-actions">
        <UiSelect v-model="selected" aria-label="诊断账号" :options="options" :disabled="busy || accountsLoading" style="min-width:160px" />
        <label title="请求 GitHub 检测公共网络，不测试网盘 API"><input v-model="network" type="checkbox" aria-label="公共网络检测" :disabled="busy" />检测 GitHub 连接</label>
        <button class="btn sm primary" :disabled="busy || accountsLoading" @click="run">{{ busy ? '诊断中…' : '运行诊断' }}</button>
        <button class="btn sm" :disabled="!report || busy || exporting" @click="exportReport">{{ exporting ? '导出中…' : '导出诊断' }}</button>
      </div>
      <p>仅执行只读检查，导出自动隐藏敏感信息。</p>
      <ul v-if="report" class="diagnostic-checks" aria-live="polite"><li v-for="check in report.checks" :key="check.name" :class="check.status"><strong>{{ check.name }} · {{ labels[check.status] }}</strong><code v-if="check.code">{{ check.code }}</code><span>{{ check.message }}</span></li></ul>
    </div>
  </details>
</template>

<style scoped>
.diagnostics-panel { padding: 16px; border: 1px solid var(--border-light); border-radius: var(--radius-md); margin-top: 12px; }
summary { cursor: pointer; font-size: 13px; font-weight: 600; color: var(--text-primary); }
p { color: var(--text-secondary); font-size: 12px; margin: 8px 0; }
.diagnostics-actions { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin-top: 14px; }
label { display: flex; align-items: center; gap: 6px; font-size: 12px; }
.diagnostic-checks { display: grid; gap: 12px; padding: 0; list-style: none; }
.diagnostic-checks li { display: grid; gap: 4px; font-size: 12px; overflow-wrap: anywhere; }
.diagnostic-checks span, code { color: var(--text-secondary); }
.diagnostic-checks .failed strong { color: var(--color-danger); }
</style>
