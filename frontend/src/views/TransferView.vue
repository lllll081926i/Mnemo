<script setup>
import { ref, onMounted } from 'vue'
import { ListDownloads, ListUploads, PauseDownload, ResumeDownload, CancelDownload, CancelUpload, ClearDownloads, ClearUploads, onEvent, formatBytes } from '../api'

const emit = defineEmits(['toast'])
const seg = ref('downloading')
const downloads = ref([])
const uploads = ref([])

function refresh() {
  ListDownloads().then((d) => { downloads.value = d || [] })
  ListUploads().then((u) => { uploads.value = u || [] })
}

const statusText = (s) => ({ queued: '排队', downloading: '下载中', paused: '已暂停', completed: '已完成', failed: '失败', canceled: '已取消', uploading: '上传中', stopped: '已停止' }[s] || s)

onMounted(() => {
  refresh()
  onEvent('transfer:event', refresh)
})

function clear() {
  ClearDownloads(); ClearUploads(); refresh()
}
</script>

<template>
  <div class="panel">
    <div class="panel-row">
      <div class="seg">
        <button :class="{ active: seg === 'downloading' }" @click="seg = 'downloading'">下载</button>
        <button :class="{ active: seg === 'uploading' }" @click="seg = 'uploading'">上传</button>
      </div>
      <div style="flex:1"></div>
      <button class="btn sm" @click="clear">清理完成</button>
    </div>

    <div class="panel-title">{{ seg === 'downloading' ? '下载任务' : '上传任务' }}</div>

    <template v-if="seg === 'downloading'">
      <div v-for="t in downloads" :key="t.id" class="task-item">
        <span class="tname">{{ t.name }}</span>
        <div class="tbar"><div :style="{ width: (t.Progress || 0) + '%' }"></div></div>
        <span class="tmeta">{{ formatBytes(t.Speed) }}/s · {{ formatBytes(t.Downloaded) }}/{{ formatBytes(t.Size) }}</span>
        <span class="tstatus">{{ statusText(t.Status) }}</span>
        <div class="panel-row">
          <button v-if="t.Status === 'downloading' || t.Status === 'queued'" class="btn sm" @click="PauseDownload(t.id)">暂停</button>
          <button v-else-if="t.Status === 'paused'" class="btn sm" @click="ResumeDownload(t.id)">继续</button>
          <button v-if="t.Status !== 'completed'" class="btn sm danger" @click="CancelDownload(t.id)">取消</button>
        </div>
      </div>
      <div v-if="!downloads.length" class="empty">暂无下载任务</div>
    </template>

    <template v-else>
      <div v-for="t in uploads" :key="t.UploadID" class="task-item">
        <span class="tname">{{ t.Info.Name || t.Info.LocalFilePath }}</span>
        <div class="tbar"><div :style="{ width: (t.Upload.DownProcess || 0) + '%' }"></div></div>
        <span class="tmeta">{{ formatBytes(t.Upload.DownSize) }}/{{ formatBytes(t.Info.Size) }}</span>
        <span class="tstatus">{{ statusText(t.Upload.DownState) }}</span>
        <button v-if="!t.Upload.IsCompleted && !t.Upload.IsFailed" class="btn sm danger" @click="CancelUpload(t.UploadID)">取消</button>
      </div>
      <div v-if="!uploads.length" class="empty">暂无上传任务</div>
    </template>
  </div>
</template>