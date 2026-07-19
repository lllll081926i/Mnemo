<script setup lang="ts">
import { computed } from 'vue'
import message from '../utils/message'
import DebugLog from '../utils/debuglog'
import { useLogStore, useWinStore } from '../store'
import { copyToClipboard } from '../utils/electronhelper'

const logStore = useLogStore()
const winStore = useWinStore()

const logHeight = computed(() => winStore.height - 316)

const handleSaveLogRefresh = () => {
  DebugLog.aLoadFromDB()
}

const handleSaveLogClear = () => {
  DebugLog.mSaveLogClear()
}

const handleSaveLogCopy = () => {
  let logstr = ''
  const logList = DebugLog.logList
  for (let i = 0, maxi = logList.length; i < maxi; i++) {
    const item = logList[i]
    logstr += item.logtime + ' : ' + item.logtype + ' : ' + item.logmessage + '\n'
  }
  copyToClipboard(logstr)
  message.success('运行日志已复制到剪切板')
}
</script>

<template>
  <div class="ui-plain-list">
    <div class="ui-plain-row ui-plain-row--top">
      <span class="ui-plain-label">运行日志</span>
      <div class="ui-plain-control">
        <a-list
          :bordered="false"
          :max-height="logHeight"
          :style="{ height: logHeight + 'px' }"
          :data="DebugLog.logList"
          class="loglist"
          :data-refresh="logStore.logTime"
          :virtual-list-props="{
            height: logHeight,
            threshold: 50
          }"
        >
          <template #item="{ item, index }">
            <a-list-item :key="index">
              <a-typography-text :type="item.logtype">[{{ item.logtime }}]</a-typography-text>
              {{ item.logmessage }}
            </a-list-item>
          </template>
        </a-list>
        <a-button type="outline" size="small" @click="handleSaveLogRefresh">刷新</a-button>
        <a-button type="outline" size="small" @click="handleSaveLogClear">清空日志</a-button>
        <a-button type="outline" size="small" @click="handleSaveLogCopy">复制日志</a-button>
      </div>
    </div>
  </div>
</template>

<style>
.settings-page .loglist {
  width: 100%;
  max-height: 420px;
  box-sizing: border-box;
  overflow: hidden;
  border: 1px solid var(--border-light);
  border-radius: 0;
  background: transparent;
}

.settings-page .loglist .arco-list {
  height: 100%;
  overflow-y: hidden;
}

.settings-page .loglist .arco-list-item {
  padding: 6px 10px;
  font-size: 12px;
  color: var(--text-secondary);
}

.settings-page .loglist .arco-list-item-content {
  user-select: text;
  -webkit-user-drag: none;
}
</style>
