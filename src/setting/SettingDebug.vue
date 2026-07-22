<script setup lang="ts">
import useSettingStore from './settingstore'
import AppCache from '../utils/appcache'

const settingStore = useSettingStore()
const cb = (val: any) => settingStore.updateStore(val)
</script>

<template>
  <div class="ui-plain-list">
    <div class="ui-plain-row">
      <span class="ui-plain-label">文件列表上限</span>
      <div class="ui-plain-control"><a-input-number class="ui-control-sm" size="small" :min="3000" :max="10000" :step="100" :model-value="settingStore.debugFileListMax" @update:model-value="cb({ debugFileListMax: $event })" /></div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">传输记录保留</span>
      <div class="ui-plain-control"><a-input-number class="ui-control-sm" size="small" :min="1000" :max="50000" :step="1000" :model-value="settingStore.debugDownedListMax" @update:model-value="cb({ debugDownedListMax: $event })" /></div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">本地数据</span>
      <div class="ui-plain-control">
        <a-popconfirm content="确认要清理数据库？" @ok="AppCache.aClearDir('db')"><a-button type="outline" size="small" status="danger">清理数据库</a-button></a-popconfirm>
        <a-popconfirm content="确认要清理缓存？" @ok="AppCache.aClearDir('cache')"><a-button type="outline" size="small" status="danger">清理缓存</a-button></a-popconfirm>
        <a-popconfirm content="确认要重置？会重启应用" @ok="AppCache.aClearDir('all')"><a-button type="outline" size="small" status="danger">删除全部数据</a-button></a-popconfirm>
      </div>
    </div>
  </div>
</template>
