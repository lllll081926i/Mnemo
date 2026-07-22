<script setup lang="ts">
import { computed } from 'vue'
import useSettingStore from './settingstore'
import MySwitch from '../layout/MySwitch.vue'
import { AriaGlobalSpeed } from '../utils/aria2c'
import { getSystemDownloadsPath } from '../utils/electronhelper'

const settingStore = useSettingStore()
const displayDownSavePath = computed(() => settingStore.downSavePath || getSystemDownloadsPath() || '系统默认下载文件夹')
const cb = async (val: any) => {
  await settingStore.updateStore(val)
  if (Object.hasOwn(val, 'downGlobalSpeed') || Object.hasOwn(val, 'downGlobalSpeedM')) await AriaGlobalSpeed()
}
const handleSelectDownSavePath = () => {
  window.WebShowOpenDialogSync?.({ title: '选择下载文件夹', buttonLabel: '选择', properties: ['openDirectory', 'createDirectory'], defaultPath: displayDownSavePath.value }, (result: string[] | undefined) => {
    if (result?.[0]) settingStore.updateStore({ downSavePath: result[0] })
  })
}
</script>

<template>
  <div class="ui-plain-list">
    <div class="ui-plain-row">
      <span class="ui-plain-label">下载位置</span>
      <div class="ui-plain-control"><a-input-search class="ui-control-lg" size="small" :readonly="true" button-text="更改" search-button :model-value="displayDownSavePath" @search="handleSelectDownSavePath" /></div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">默认使用此路径</span>
      <div class="ui-plain-control"><MySwitch :value="settingStore.downSavePathDefault" @update:value="cb({ downSavePathDefault: $event })" /></div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">按网盘路径保存</span>
      <div class="ui-plain-control"><MySwitch :value="settingStore.downSavePathFull" @update:value="cb({ downSavePathFull: $event })" /></div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">最大并行任务</span>
      <div class="ui-plain-control"><a-input-number class="ui-control-xs" size="small" :min="1" :max="5" :model-value="settingStore.downFileMax" @update:model-value="cb({ downFileMax: $event })" /></div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">每文件线程</span>
      <div class="ui-plain-control">
        <a-select class="ui-control-sm" size="small" :model-value="settingStore.downThreadMax" @update:model-value="cb({ downThreadMax: $event })">
          <a-option v-for="item in [1, 2, 4, 8, 16, 24, 32]" :key="item" :value="item">{{ item }} 个线程</a-option>
        </a-select>
      </div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">每服务器连接数</span>
      <div class="ui-plain-control"><a-input-number class="ui-control-xs" size="small" :min="1" :max="64" :model-value="settingStore.ariaMaxConnectionPerServer" @update:model-value="cb({ ariaMaxConnectionPerServer: $event })" /></div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">下载限速</span>
      <div class="ui-plain-control">
        <a-input-number class="ui-control-sm" size="small" :min="0" :model-value="settingStore.downGlobalSpeed" @update:model-value="cb({ downGlobalSpeed: $event })" />
        <a-radio-group type="button" size="small" :model-value="settingStore.downGlobalSpeedM" @update:model-value="cb({ downGlobalSpeedM: $event, downGlobalSpeed: 0 })">
          <a-radio value="MB">MB/s</a-radio>
          <a-radio value="KB">KB/s</a-radio>
        </a-radio-group>
      </div>
    </div>
  </div>
</template>
