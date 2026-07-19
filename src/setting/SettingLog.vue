<script setup lang="ts">
import { FileText } from 'lucide-vue-next'
import MySwitch from '../layout/MySwitch.vue'
import DebugLog, { type DebugLogLevel } from '../utils/debuglog'
import { getUserDataPath } from '../utils/electronhelper'
import useSettingStore from './settingstore'

const settingStore = useSettingStore()
const logPath = getUserDataPath('mnemo.log')
const cb = (val: any) => settingStore.updateStore(val)
const setLogLevel = (level: DebugLogLevel) => cb({ debugLogLevel: level })
const openLogFile = () => window.Electron.shell.showItemInFolder(logPath)
</script>

<template>
  <div class="ui-plain-list">
    <div class="ui-plain-row">
      <span class="ui-plain-label">写入日志</span>
      <div class="ui-plain-control"><MySwitch :value="settingStore.debugLogEnabled" @update:value="cb({ debugLogEnabled: $event })" /></div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">日志等级</span>
      <div class="ui-plain-control">
        <a-select class="ui-control-sm" size="small" :model-value="settingStore.debugLogLevel" @update:model-value="setLogLevel">
          <a-option value="debug">Debug</a-option>
          <a-option value="info">Info</a-option>
          <a-option value="warn">Warn</a-option>
          <a-option value="error">Error</a-option>
        </a-select>
      </div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">日志文件上限</span>
      <div class="ui-plain-control">
        <a-input-number class="ui-control-sm" size="small" :min="1" :max="100" :model-value="settingStore.debugLogMaxSizeMB" @update:model-value="cb({ debugLogMaxSizeMB: $event })" />
        <span>MB</span>
      </div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">日志位置</span>
      <div class="ui-plain-control">
        <a-input class="ui-control-lg" size="small" :model-value="DebugLog.logPath" readonly />
        <a-button type="outline" size="small" title="打开日志位置" aria-label="打开日志位置" @click="openLogFile"><FileText :size="15" /></a-button>
      </div>
    </div>
  </div>
</template>
