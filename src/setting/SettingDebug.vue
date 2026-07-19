<script setup lang="ts">
import { onMounted, ref } from 'vue'
import useSettingStore from './settingstore'
import AppCache from '../utils/appcache'
import { getUserData, openExternal } from '../utils/electronhelper'
import message from '../utils/message'

const settingStore = useSettingStore()
const cb = (val: any) => settingStore.updateStore(val)
const userData = getUserData()
const handleJumpPath = () => openExternal(userData)
const cacheTotalBytes = ref(0)
const cacheLoading = ref(false)
const cacheClearing = ref(false)
const loadCacheStats = async () => {
  if (!(window as any).MsImageCacheStats) return
  cacheLoading.value = true
  try {
    cacheTotalBytes.value = (await (window as any).MsImageCacheStats()).totalBytes ?? 0
  } finally {
    cacheLoading.value = false
  }
}
const handleClearCache = async () => {
  if (!(window as any).MsImageCacheClear) return
  cacheClearing.value = true
  try {
    await (window as any).MsImageCacheClear()
    await loadCacheStats()
    message.success('已清理全部图片缓存')
  } catch {
    message.error('清理失败，请重试')
  } finally {
    cacheClearing.value = false
  }
}
const formatBytes = (bytes: number) => (bytes < 1024 ? `${bytes} B` : bytes < 1048576 ? `${(bytes / 1024).toFixed(1)} KB` : `${(bytes / 1048576).toFixed(1)} MB`)
onMounted(loadCacheStats)
</script>

<template>
  <div class="ui-plain-list">
    <div class="ui-plain-row">
      <span class="ui-plain-label">文件列表上限</span>
      <div class="ui-plain-control"><a-input-number class="ui-control-sm" size="small" :min="3000" :max="10000" :step="100" :model-value="settingStore.debugFileListMax" @update:model-value="cb({ debugFileListMax: $event })" /></div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">收藏列表上限</span>
      <div class="ui-plain-control"><a-input-number class="ui-control-sm" size="small" :min="100" :max="3000" :step="100" :model-value="settingStore.debugFavorListMax" @update:model-value="cb({ debugFavorListMax: $event })" /></div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">传输记录保留</span>
      <div class="ui-plain-control"><a-input-number class="ui-control-sm" size="small" :min="1000" :max="50000" :step="1000" :model-value="settingStore.debugDownedListMax" @update:model-value="cb({ debugDownedListMax: $event })" /></div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">数据位置</span>
      <div class="ui-plain-control">
        <a-input class="ui-control-lg" size="small" :model-value="userData" :readonly="true" />
        <a-button type="outline" size="small" @click="handleJumpPath">打开</a-button>
      </div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">本地数据</span>
      <div class="ui-plain-control">
        <a-popconfirm content="确认要清理数据库？" @ok="AppCache.aClearDir('db')"><a-button type="outline" size="small" status="danger">清理数据库</a-button></a-popconfirm>
        <a-popconfirm content="确认要清理缓存？" @ok="AppCache.aClearDir('cache')"><a-button type="outline" size="small" status="danger">清理缓存</a-button></a-popconfirm>
        <a-popconfirm content="确认要重置？会重启应用" @ok="AppCache.aClearDir('all')"><a-button type="outline" size="small" status="danger">删除全部数据</a-button></a-popconfirm>
      </div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">媒体图片缓存 {{ cacheLoading ? '加载中' : formatBytes(cacheTotalBytes) }}</span>
      <div class="ui-plain-control">
        <a-button type="outline" size="small" :loading="cacheLoading" @click="loadCacheStats">刷新</a-button>
        <a-popconfirm content="确认清理全部媒体图片缓存？" @ok="handleClearCache"><a-button type="outline" size="small" status="danger" :loading="cacheClearing">清理</a-button></a-popconfirm>
      </div>
    </div>
  </div>
</template>
