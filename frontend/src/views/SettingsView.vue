<script setup>
import { ref, onMounted } from 'vue'
import { GetSettings, SaveSettings, MediaProxy } from '../api'

const emit = defineEmits(['toast'])
const settings = ref({})
const proxyBase = ref('')

onMounted(async () => {
  settings.value = (await GetSettings()) || {}
  proxyBase.value = (await MediaProxy()) || ''
})

function toggle(key) { settings.value[key] = !settings.value[key] }

async function save() {
  await SaveSettings(settings.value)
  emit('toast', '设置已保存', 'success')
}
</script>

<template>
  <div class="panel" style="max-width:640px">
    <div class="panel-title">设置</div>
    <div class="setting-row"><label>下载目录</label>
      <input class="input" v-model="settings.DownloadDir" placeholder="留空使用系统下载目录" style="width:280px" />
    </div>
    <div class="setting-row"><label>主题</label>
      <select class="select" v-model="settings.Theme">
        <option value="dark">深色</option>
        <option value="light">浅色</option>
        <option value="system">跟随系统</option>
      </select>
    </div>
    <div class="setting-row"><label>默认页签</label>
      <select class="select" v-model="settings.DefaultTab">
        <option value="pan">网盘</option>
        <option value="transfer">传输</option>
        <option value="share">分享</option>
        <option value="sync">同步</option>
      </select>
    </div>
    <div class="setting-row"><label>代理地址（HTTP，可选）</label>
      <input class="input" v-model="settings.Proxy" placeholder="http://127.0.0.1:7890" style="width:280px" />
    </div>
    <div class="setting-row"><label>最大并发下载数</label>
      <input class="input" type="number" v-model.number="settings.MaxConcurrentDownloads" style="width:120px" />
    </div>
    <div class="setting-row"><label>下载限速（KB/s，0 不限）</label>
      <input class="input" type="number" v-model.number="settings.MaxDownloadSpeed" style="width:120px" />
    </div>
    <div class="setting-row">
      <label>断点续播</label>
      <div class="switch" :class="{ on: settings.PlaybackResume }" @click="toggle('PlaybackResume')"></div>
    </div>
    <div class="setting-row">
      <label>保留传输记录</label>
      <div class="switch" :class="{ on: settings.KeepTasks }" @click="toggle('KeepTasks')"></div>
    </div>
    <div class="panel-row" style="margin-top:16px">
      <button class="btn primary" @click="save">保存设置</button>
      <span class="fmeta" v-if="proxyBase">媒体代理: {{ proxyBase }}</span>
    </div>
  </div>
</template>