<script setup>
import { ref, onMounted, watch } from 'vue'
import { ListSyncConfigs, SaveSyncConfig, DeleteSyncConfig, RunSync } from '../api'

const props = defineProps({ account: Object })
const emit = defineEmits(['toast'])
const jobs = ref([])
const form = ref({ name: '', localDir: '', remoteDir: '', direction: 'two-way' })
const editing = ref(false)

function refresh() {
  ListSyncConfigs().then((j) => { jobs.value = j || [] })
}

async function save() {
  if (!props.account) { emit('toast', '请先选择账号', 'error'); return }
  const id = editing.value || 'sync-' + Date.now()
  await SaveSyncConfig({
    id, name: form.value.name || id, direction: form.value.direction,
    user_id: props.account.UserID, drive_id: props.account.DriveID,
    local_dir: form.value.localDir, remote_dir: form.value.remoteDir || 'root', enabled: true,
  })
  editing.value = false
  form.value = { name: '', localDir: '', remoteDir: '', direction: 'two-way' }
  refresh()
  emit('toast', '已保存同步任务', 'success')
}

function edit(job) {
  editing.value = job.ID
  form.value = { name: job.Name, localDir: job.LocalDir, remoteDir: job.RemoteDir, direction: job.Direction }
}

async function remove(id) {
  await DeleteSyncConfig(id)
  refresh()
}

async function run(id) {
  emit('toast', '开始同步…', 'success')
  try { await RunSync(id); emit('toast', '同步完成', 'success') }
  catch (e) { emit('toast', String(e), 'error') }
}

watch(() => props.account, () => refresh())
onMounted(refresh)
</script>

<template>
  <div class="panel">
    <div class="panel-title">双向同步（本地 ⇄ 网盘）</div>
    <div class="panel-row">
      <input class="input" style="width:140px" v-model="form.name" placeholder="名称" />
      <input class="input" style="flex:1" v-model="form.localDir" placeholder="本地目录，如 D:\sync" />
      <input class="input" style="width:160px" v-model="form.remoteDir" placeholder="网盘目录 ID (root)" />
      <select class="select" v-model="form.direction">
        <option value="two-way">双向</option>
        <option value="push">仅上传</option>
        <option value="pull">仅下载</option>
      </select>
      <button class="btn primary" @click="save">{{ editing ? '保存修改' : '添加任务' }}</button>
    </div>
    <div v-for="job in jobs" :key="job.ID" class="task-item">
      <span class="tname">{{ job.Name }}</span>
      <span class="tmeta">{{ job.LocalDir }} ⇄ {{ job.RemoteDir }}</span>
      <span class="tstatus">{{ job.Direction }}</span>
      <button class="btn sm" @click="run(job.ID)">同步</button>
      <button class="btn sm" @click="edit(job)">编辑</button>
      <button class="btn sm danger" @click="remove(job.ID)">删除</button>
    </div>
    <div v-if="!jobs.length" class="empty">暂无同步任务</div>
  </div>
</template>