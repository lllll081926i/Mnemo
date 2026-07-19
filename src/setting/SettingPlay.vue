<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import useSettingStore from './settingstore'
import MySwitch from '../layout/MySwitch.vue'
import message from '../utils/message'

const settingStore = useSettingStore()
const platform = window.platform
const supportsEmbeddedMpv = platform === 'darwin'
const embeddedMpvCapability = ref<any>()
const cb = (val: any) => settingStore.updateStore(val)
const playerType = computed(() => (settingStore.uiVideoPlayer === 'mpv' ? 'mpv' : settingStore.uiVideoPlayerPath || '').toLowerCase())

async function refreshMpvEmbeddedStatus() {
  if (platform !== 'darwin') return
  const capability = await window.WebMpvEmbeddedCapability?.()
  if (capability) embeddedMpvCapability.value = capability
}

function handleSelectPlayer() {
  if (!window.WebShowOpenDialogSync) return
  window.WebShowOpenDialogSync({ title: '选择播放器', buttonLabel: '选择', properties: ['openFile'], defaultPath: settingStore.uiVideoPlayerPath, filters: [{ name: '应用程序', extensions: ['exe', 'app'] }] }, (result: string[] | undefined) => {
    if (result?.[0]) settingStore.updateStore({ uiVideoPlayerPath: result[0] })
  })
}

async function handleUseMpv() {
  if (!supportsEmbeddedMpv) {
    settingStore.updateStore({ uiVideoPlayer: 'other' })
    message.warning('内置 MPV 仅支持 macOS')
    return
  }
  settingStore.updateStore({ uiVideoPlayer: 'mpv' })
  await refreshMpvEmbeddedStatus()
}

onMounted(() => {
  if (!supportsEmbeddedMpv && settingStore.uiVideoPlayer === 'mpv') settingStore.updateStore({ uiVideoPlayer: 'other' })
  refreshMpvEmbeddedStatus()
})
</script>

<template>
  <div class="ui-plain-list">
    <div class="ui-plain-row">
      <span class="ui-plain-label">播放器</span>
      <div class="ui-plain-control">
        <a-radio-group type="button" size="small" :model-value="settingStore.uiVideoPlayer" @update:model-value="cb({ uiVideoPlayer: $event })">
          <a-radio value="web">内置播放器</a-radio>
          <a-radio v-if="supportsEmbeddedMpv" value="mpv">内置 MPV</a-radio>
          <a-radio value="other">外部播放器</a-radio>
        </a-radio-group>
        <a-button v-if="supportsEmbeddedMpv && settingStore.uiVideoPlayer === 'mpv'" type="outline" size="small" @click="handleUseMpv">{{ embeddedMpvCapability?.enabled ? 'MPV 可用' : '检测' }}</a-button>
      </div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">默认清晰度</span>
      <div class="ui-plain-control">
        <a-select size="small" :model-value="settingStore.uiVideoQuality" @update:model-value="cb({ uiVideoQuality: $event })">
          <a-option value="Origin">原画</a-option>
          <a-option value="QHD">2560p</a-option>
          <a-option value="FHD">1080P</a-option>
          <a-option value="HD">720P</a-option>
          <a-option value="SD">540P</a-option>
          <a-option value="LD">480P</a-option>
        </a-select>
      </div>
    </div>
    <template v-if="settingStore.uiVideoPlayer !== 'web'">
      <div class="ui-plain-row">
        <span class="ui-plain-label">播放前提示清晰度</span>
        <div class="ui-plain-control"><MySwitch :value="settingStore.uiVideoQualityTips" @update:value="cb({ uiVideoQualityTips: $event })" /></div>
      </div>
      <div class="ui-plain-row">
        <span class="ui-plain-label">记住清晰度</span>
        <div class="ui-plain-control"><MySwitch :value="settingStore.uiVideoQualityLastSelect" @update:value="cb({ uiVideoQualityLastSelect: $event })" /></div>
      </div>
      <div class="ui-plain-row">
        <span class="ui-plain-label">字幕</span>
        <div class="ui-plain-control">
          <a-radio-group type="button" size="small" :model-value="settingStore.uiVideoSubtitleMode" @update:model-value="cb({ uiVideoSubtitleMode: $event })">
            <a-radio value="close">关闭</a-radio>
            <a-radio value="auto">自动</a-radio>
            <a-radio value="select">手动</a-radio>
          </a-radio-group>
        </div>
      </div>
      <div class="ui-plain-row">
        <span class="ui-plain-label">播放历史</span>
        <div class="ui-plain-control"><MySwitch :value="settingStore.uiVideoPlayerHistory" @update:value="cb({ uiVideoPlayerHistory: $event })" /></div>
      </div>
    </template>
    <template v-if="settingStore.uiVideoPlayer === 'other'">
      <div class="ui-plain-row">
        <span class="ui-plain-label">外部播放器</span>
        <div class="ui-plain-control">
          <a-auto-complete v-if="platform === 'linux'" size="small" :data="['mpv', 'vlc', 'totem', 'mplayer', 'smplayer']" :model-value="settingStore.uiVideoPlayerPath" @change="cb({ uiVideoPlayerPath: $event })" />
          <a-input-search v-else size="small" :readonly="true" button-text="选择" search-button :model-value="settingStore.uiVideoPlayerPath" @search="handleSelectPlayer" />
        </div>
      </div>
      <div class="ui-plain-row">
        <span class="ui-plain-label">启动参数</span>
        <div class="ui-plain-control"><a-input size="small" v-model.trim="settingStore.uiVideoPlayerParams" allow-clear @update:model-value="cb({ uiVideoPlayerParams: $event })" /></div>
      </div>
      <div v-if="playerType.includes('mpv') || playerType.includes('potplayer')" class="ui-plain-row">
        <span class="ui-plain-label">启用播放列表</span>
        <div class="ui-plain-control"><MySwitch :value="settingStore.uiVideoEnablePlayerList" @update:value="cb({ uiVideoEnablePlayerList: $event })" /></div>
      </div>
      <div v-if="settingStore.uiVideoEnablePlayerList || settingStore.uiVideoPlayerHistory" class="ui-plain-row">
        <span class="ui-plain-label">播放结束退出</span>
        <div class="ui-plain-control"><MySwitch :value="settingStore.uiVideoPlayerExit" @update:value="cb({ uiVideoPlayerExit: $event })" /></div>
      </div>
    </template>
  </div>
</template>
