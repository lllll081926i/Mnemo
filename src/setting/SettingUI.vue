<script setup lang="ts">
import { computed, ref } from 'vue'
import useSettingStore from './settingstore'
import MySwitch from '../layout/MySwitch.vue'
import { openExternal } from '../utils/electronhelper'
import { getPkgVersion } from '../utils/utils'
import message from '../utils/message'

const platform = window.platform
const settingStore = useSettingStore()
const verLoading = ref(false)
const getAppVersion = computed(() => {
  try {
    return getPkgVersion()
  } catch {
    return 'dev'
  }
})

const cb = (val: any) => settingStore.updateStore(val)

const handleCheckVer = async () => {
  verLoading.value = true
  try {
    const result = await window.WebCheckUpdate()
    if (result.ok) message.info(`已完成更新检查，最新版本 ${result.version || getAppVersion.value}`)
    else message.warning(result.error || '检查更新失败')
  } catch (e: any) {
    message.error(e?.message || '检查更新失败')
  } finally {
    verLoading.value = false
  }
}

const openProject = () => openExternal('https://github.com/lllll081926i/Mnemo')
</script>

<template>
  <div class="ui-plain-list">
    <div class="ui-plain-row">
      <span class="ui-plain-label">版本</span>
      <div class="ui-plain-control">
        <span>Mnemo {{ getAppVersion }}</span>
        <a-button type="outline" size="small" :loading="verLoading" @click="handleCheckVer">检查更新</a-button>
        <a-button type="text" size="small" @click="openProject">项目地址</a-button>
      </div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">主题</span>
      <div class="ui-plain-control">
        <a-radio-group type="button" size="small" :model-value="settingStore.uiTheme" @update:model-value="cb({ uiTheme: $event })">
          <a-radio value="system">跟随系统</a-radio>
          <a-radio value="light">浅色</a-radio>
          <a-radio value="dark">深色</a-radio>
        </a-radio-group>
      </div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">默认启动页</span>
      <div class="ui-plain-control">
        <a-radio-group type="button" size="small" :model-value="settingStore.uiDefaultTab" @update:model-value="cb({ uiDefaultTab: $event })">
          <a-radio value="pan">网盘</a-radio>
          <a-radio value="down">传输</a-radio>
          <a-radio value="share">分享</a-radio>
          <a-radio value="setting">设置</a-radio>
        </a-radio-group>
      </div>
    </div>
    <div v-if="['win32', 'darwin'].includes(platform)" class="ui-plain-row">
      <span class="ui-plain-label">开机启动</span>
      <div class="ui-plain-control"><MySwitch :value="settingStore.uiLaunchStart" @update:value="cb({ uiLaunchStart: $event })" /></div>
    </div>
    <div v-if="settingStore.uiLaunchStart" class="ui-plain-row">
      <span class="ui-plain-label">启动后显示窗口</span>
      <div class="ui-plain-control"><MySwitch :value="settingStore.uiLaunchStartShow" @update:value="cb({ uiLaunchStartShow: $event })" /></div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">启动时自动签到</span>
      <div class="ui-plain-control"><MySwitch :value="settingStore.uiLaunchAutoSign" @update:value="cb({ uiLaunchAutoSign: $event })" /></div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">关闭即退出</span>
      <div class="ui-plain-control"><MySwitch :value="settingStore.uiExitOnClose" @update:value="cb({ uiExitOnClose: $event })" /></div>
    </div>
  </div>
</template>
