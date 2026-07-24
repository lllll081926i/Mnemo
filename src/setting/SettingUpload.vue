<script setup lang="ts">
import useSettingStore from './settingstore'
import MySwitch from '../layout/MySwitch.vue'
import MyTags from '../layout/MyTags.vue'
const settingStore = useSettingStore()
const cb = (val: any) => settingStore.updateStore(val)
</script>

<template>
  <div class="ui-plain-list">
    <div class="ui-plain-row">
      <span class="ui-plain-label">最大并行任务</span>
      <div class="ui-plain-control">
        <a-select class="ui-control-sm" size="small" :model-value="settingStore.uploadFileMax" @update:model-value="cb({ uploadFileMax: $event })">
          <a-option v-for="item in [1, 3, 5, 10, 20, 30, 50]" :key="item" :value="item">{{ item }} 个文件</a-option>
        </a-select>
      </div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">上传限速</span>
      <div class="ui-plain-control">
        <a-input-number class="ui-control-sm" size="small" :min="0" :model-value="settingStore.uploadGlobalSpeed" @update:model-value="cb({ uploadGlobalSpeed: $event })" />
        <a-radio-group type="button" size="small" :model-value="settingStore.uploadGlobalSpeedM" @update:model-value="cb({ uploadGlobalSpeedM: $event, uploadGlobalSpeed: 0 })">
          <a-radio value="MB">MB/s</a-radio>
          <a-radio value="KB">KB/s</a-radio>
        </a-radio-group>
      </div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">完成后自动关机</span>
      <div class="ui-plain-control"><MySwitch :value="settingStore.downAutoShutDown > 0" @update:value="cb({ downAutoShutDown: $event ? 1 : 0 })" /></div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">优先小文件</span>
      <div class="ui-plain-control"><MySwitch :value="settingStore.downSmallFileFirst" @update:value="cb({ downSmallFileFirst: $event })" /></div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">任务栏总进度</span>
      <div class="ui-plain-control"><MySwitch :value="settingStore.downSaveShowPro" @update:value="cb({ downSaveShowPro: $event })" /></div>
    </div>
    <div class="ui-plain-row upload-filter-row">
      <span class="ui-plain-label">过滤文件</span>
      <div class="ui-plain-control upload-filter-control"><MyTags class="upload-filter-tags" :value="settingStore.downIngoredList" :maxlen="20" @update:value="cb({ downIngoredList: $event })" /></div>
    </div>
  </div>
</template>

<style scoped>
.upload-filter-row {
  align-items: start;
  padding-block: 6px;
}
.upload-filter-control {
  overflow: visible;
  white-space: normal;
}
.upload-filter-tags {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  min-width: 0;
  width: 100%;
  gap: 6px;
}
.upload-filter-tags :deep(.arco-tag) {
  margin: 0;
  flex: 0 0 auto;
  color: var(--text-primary);
  background: var(--bg-subtle);
  border-color: var(--border-light);
}
.upload-filter-tags :deep(.arco-input-search) {
  width: 180px !important;
  min-width: 180px;
  margin: 0;
}
</style>
