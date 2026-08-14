<script setup>
import { ref, onMounted, watch } from 'vue'
import { ListShareHistory, CreateShare, OfflineDownload, ListOfflineTasks, onEvent } from '../api'

const props = defineProps({ account: Object })
const emit = defineEmits(['toast'])
const seg = ref('history')
const history = ref([])
const offline = ref([])
const offlineUrl = ref('')

const meta = ref(null)

async function refresh() {
  const uid = props.account ? props.account.UserID : ''
  history.value = (await ListShareHistory(uid)) || []
  offline.value = (await ListOfflineTasks(uid)) || []
}

async function createShareFor(fileIds, name) {
  if (!props.account) { emit('toast', '请先选择账号', 'error'); return }
  try {
    const item = await CreateShare(props.account.UserID, props.account.DriveID, { fileIds, shareName: name })
    emit('toast', (item && item.ShareURL) || '分享已创建', 'success')
    refresh()
  } catch (e) { emit('toast', String(e), 'error') }
}

async function doOffline() {
  if (!props.account) { emit('toast', '请先选择 PikPak 账号', 'error'); return }
  if (!offlineUrl.value) { emit('toast', '请输入链接', 'error'); return }
  try {
    await OfflineDownload(props.account.UserID, props.account.DriveID, offlineUrl.value, '')
    offlineUrl.value = ''
    emit('toast', '已提交云离线', 'success')
    refresh()
  } catch (e) { emit('toast', String(e), 'error') }
}

watch(() => props.account, () => refresh())
onMounted(() => { refresh(); onEvent('account:changed', refresh) })
</script>

<template>
  <div class="panel">
    <div class="panel-row">
      <div class="seg">
        <button :class="{ active: seg === 'history' }" @click="seg = 'history'">我的分享</button>
        <button :class="{ active: seg === 'offline' }" @click="seg = 'offline'">云离线</button>
      </div>
    </div>

    <template v-if="seg === 'history'">
      <div v-if="!history.length" class="empty">暂无分享记录</div>
      <div v-for="h in history" :key="h.ShareID + h.AccountID" class="task-item">
        <span class="tname">{{ h.ShareName }}</span>
        <a class="tmeta" :href="h.ShareURL" target="_blank" style="color:var(--text-link)">{{ h.ShareURL }}</a>
        <span v-if="h.SharePwd" class="tstatus">密码 {{ h.SharePwd }}</span>
      </div>
    </template>

    <template v-else>
      <div class="panel-row">
        <input class="input" style="flex:1" v-model="offlineUrl" placeholder="磁力链接 / HTTP 下载链接（提交到 PikPak 云端离线下载）" />
        <button class="btn primary" @click="doOffline">提交</button>
      </div>
      <div v-for="t in offline" :key="t.ID" class="task-item">
        <span class="tname">{{ t.FileName || t.URL }}</span>
        <span class="tstatus">{{ t.Status }}</span>
      </div>
      <div v-if="!offline.length" class="empty">暂无云离线任务</div>
    </template>
  </div>
</template>