<script setup>
import { computed, ref } from 'vue'
import { RunDiagnostics, ExportDiagnostics, listAccounts, accountName } from '../api'
import UiSelect from './UiSelect.vue'

const emit = defineEmits(['toast'])
const report = ref(null), busy = ref(false), exporting = ref(false), network = ref(false), accounts = ref([]), selected = ref('')
const options = computed(() => [{ value: '', label: '仅检查本机' }, ...accounts.value.map(account => ({ value: JSON.stringify([account.user_id, account.drive_id]), label: accountName(account), disabled: account.disabled }))])
const labels = { passed: '通过', failed: '异常', skipped: '未检测', info: '说明' }
async function loadAccounts() {
  try { accounts.value = await listAccounts() || [] }
  catch { emit('toast', '本机账号列表读取失败', 'error') }
}
async function run() {
  if (busy.value) return
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
  <div class="diagnostics-panel">
    <h3>诊断中心</h3>
    <p>按需检查本机回调、下载目录和账号只读权限。公共网络检测会请求 GitHub，不代表网盘 API 可用。不会自动重登或恢复任务。</p>
    <div class="diagnostics-actions">
      <button class="btn sm" :disabled="busy" @click="loadAccounts">加载本机账号</button>
      <UiSelect v-model="selected" aria-label="诊断账号" :options="options" :disabled="busy" style="min-width:160px" />
      <label><input v-model="network" type="checkbox" aria-label="公共网络检测" :disabled="busy" />公共网络检测</label>
      <button class="btn sm primary" :disabled="busy" @click="run">{{ busy ? '诊断中…' : '运行诊断' }}</button>
      <button class="btn sm" :disabled="!report || busy || exporting" @click="exportReport">导出脱敏诊断</button>
    </div>
    <p>导出仅包含版本、平台、数量与检测结果，不包含账号标识、文件名、路径、Cookie、令牌、签名链接或原始日志。</p>
    <ul v-if="report" class="diagnostic-checks" aria-live="polite"><li v-for="check in report.checks" :key="check.name" :class="check.status"><strong>{{ check.name }} · {{ labels[check.status] }}</strong><code v-if="check.code">{{ check.code }}</code><span>{{ check.message }}</span></li></ul>
  </div>
</template>

<style scoped>
.diagnostics-panel { padding: 16px; border: 1px solid var(--border-light); border-radius: var(--radius-md); margin-top: 12px; }
h3 { margin: 0 0 8px; font-size: 14px; }
p { color: var(--text-secondary); font-size: 12px; margin: 8px 0; }
.diagnostics-actions { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
label { display: flex; align-items: center; gap: 6px; font-size: 12px; }
.diagnostic-checks { display: grid; gap: 12px; padding: 0; list-style: none; }
.diagnostic-checks li { display: grid; gap: 4px; font-size: 12px; overflow-wrap: anywhere; }
.diagnostic-checks span, code { color: var(--text-secondary); }
.diagnostic-checks .failed strong { color: var(--color-danger); }
</style>
