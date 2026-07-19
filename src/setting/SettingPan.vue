<script setup lang="ts">
import useSettingStore from './settingstore'
import MySwitch from '../layout/MySwitch.vue'
const settingStore = useSettingStore()
const cb = (val: any) => settingStore.updateStore(val)
</script>

<template>
  <div class="ui-plain-list">
    <div class="ui-plain-row">
      <span class="ui-plain-label">独立排序</span>
      <div class="ui-plain-control">
        <a-select class="ui-control-md" size="small" :model-value="settingStore.uiFileOrderDuli" @update:model-value="cb({ uiFileOrderDuli: $event })">
          <a-option value="null">关闭</a-option>
          <a-option value="name asc">文件名升序</a-option>
          <a-option value="name desc">文件名降序</a-option>
          <a-option value="updated_at asc">时间升序</a-option>
          <a-option value="updated_at desc">时间降序</a-option>
          <a-option value="size asc">大小升序</a-option>
          <a-option value="size desc">大小降序</a-option>
        </a-select>
      </div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">显示完整路径</span>
      <div class="ui-plain-control"><MySwitch :value="settingStore.uiShowPanPath" @update:value="cb({ uiShowPanPath: $event })" /></div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">显示媒体信息</span>
      <div class="ui-plain-control"><MySwitch :value="settingStore.uiShowPanMedia" @update:value="cb({ uiShowPanMedia: $event })" /></div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">文件夹预览</span>
      <div class="ui-plain-control"><MySwitch :value="settingStore.uiFolderPreviewEnabled" @update:value="cb({ uiFolderPreviewEnabled: $event })" /></div>
    </div>
    <div v-if="settingStore.uiFolderPreviewEnabled" class="ui-plain-row">
      <span class="ui-plain-label">预览自动关闭</span>
      <div class="ui-plain-control">
        <a-select class="ui-control-sm" size="small" :model-value="settingStore.uiFolderPreviewAutoHide" @update:model-value="cb({ uiFolderPreviewAutoHide: $event })">
          <a-option :value="0">不自动关闭</a-option>
          <a-option :value="3">3 秒</a-option>
          <a-option :value="6">6 秒</a-option>
          <a-option :value="10">10 秒</a-option>
        </a-select>
      </div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">统计文件夹大小</span>
      <div class="ui-plain-control"><MySwitch :value="settingStore.uiFolderSize" @update:value="cb({ uiFolderSize: $event })" /></div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">日期文件夹模板</span>
      <div class="ui-plain-control">
        <a-input class="ui-control-md" size="small" :model-value="settingStore.uiTimeFolderFormate" @update:model-value="cb({ uiTimeFolderFormate: $event })" />
        <a-input-number class="ui-control-xs" size="small" :min="1" :model-value="settingStore.uiTimeFolderIndex" @update:model-value="cb({ uiTimeFolderIndex: $event })" />
      </div>
    </div>
    <div class="ui-plain-row ui-plain-row--top">
      <span class="ui-plain-label">标签</span>
      <div class="ui-plain-control settings-tags">
        <div v-for="item in settingStore.uiFileColorArray" :key="item.key" class="settings-tag">
          <span :style="{ background: item.key }"></span>
          <a-input size="small" :model-value="item.title" @update:model-value="(val: string) => settingStore.updateFileColor(item.key, val)" />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.settings-tags {
  display: grid !important;
  grid-template-columns: repeat(3, 176px);
  justify-content: start;
  gap: 6px 16px !important;
  width: 100%;
  min-width: 0;
  min-height: 0;
  overflow: visible !important;
  white-space: normal !important;
  align-content: start;
}
.settings-tag {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  min-height: 28px;
}
.settings-tag > span {
  width: 12px;
  height: 12px;
  border-radius: 3px;
  flex: 0 0 auto;
}
.settings-tag :deep(.arco-input-wrapper) {
  width: 148px !important;
  min-width: 0;
}
</style>
