<script setup lang="ts">
import { computed, ref } from 'vue'
import useSettingStore from './settingstore'
import MySwitch from '../layout/MySwitch.vue'
import { openExternal, getAppNewPath, getResourcesPath } from '../utils/electronhelper'
import ServerHttp from '../aliapi/server'
import { existsSync } from 'fs'
import { getPkgVersion } from '../utils/utils'
import { modalUpdateLog } from '../utils/modal'
import message from '../utils/message'
import { Sleep } from '../utils/format'

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
const handleUpdateLog = () => modalUpdateLog()

const handleCheckVer = async () => {
  verLoading.value = true
  try {
    await ServerHttp.CheckUpgrade(true)
  } catch (e: any) {
    message.error(e?.message || '检查更新失败')
  } finally {
    verLoading.value = false
  }
}

const handleImportAsar = () => {
  if (!window.WebShowOpenDialogSync) return message.error('不支持手动导入')
  window.WebShowOpenDialogSync(
    {
      title: '选择 Asar 文件',
      buttonLabel: '导入',
      filters: [{ name: 'asar', extensions: ['asar'] }],
      properties: ['openFile', 'showHiddenFiles', 'dontAddToRecent']
    },
    async (files: string[] | undefined) => {
      if (!files?.[0] || !existsSync(files[0])) return message.error('文件不存在')
      try {
        getAppNewPath()
        getResourcesPath('app.asar')
        message.info('导入成功，即将重启')
        await Sleep(800)
        window.WebToElectron({ cmd: 'relaunch' })
      } catch (e: any) {
        message.error(e?.message || '导入失败')
      }
    }
  )
}

const openProject = () => openExternal('https://github.com/lllll081926i/Mnemo')
</script>

<template>
  <div class="ui-plain-list">
    <div class="ui-plain-row">
      <span class="ui-plain-label">版本</span>
      <div class="ui-plain-control">
        <span>Mnemo {{ getAppVersion }}</span>
        <a-button type="outline" size="small" @click="handleUpdateLog">更新日志</a-button>
        <a-button type="outline" size="small" :loading="verLoading" @click="handleCheckVer">检查更新</a-button>
        <a-button v-if="platform !== 'linux'" type="outline" size="small" @click="handleImportAsar">导入更新</a-button>
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
      <span class="ui-plain-label">启动时检查更新</span>
      <div class="ui-plain-control"><MySwitch :value="settingStore.uiLaunchAutoCheckUpdate" @update:value="cb({ uiLaunchAutoCheckUpdate: $event })" /></div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">启动时自动签到</span>
      <div class="ui-plain-control"><MySwitch :value="settingStore.uiLaunchAutoSign" @update:value="cb({ uiLaunchAutoSign: $event })" /></div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">关闭即退出</span>
      <div class="ui-plain-control"><MySwitch :value="settingStore.uiExitOnClose" @update:value="cb({ uiExitOnClose: $event })" /></div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">更新代理</span>
      <div class="ui-plain-control"><MySwitch :value="settingStore.uiUpdateProxyEnable" @update:value="cb({ uiUpdateProxyEnable: $event })" /></div>
    </div>
    <div v-if="settingStore.uiUpdateProxyEnable" class="ui-plain-row">
      <span class="ui-plain-label">代理地址</span>
      <div class="ui-plain-control"><a-input class="ui-control-lg" v-model.trim="settingStore.uiUpdateProxyUrl" allow-clear size="small" @update:model-value="cb({ uiUpdateProxyUrl: $event })" /></div>
    </div>
  </div>
</template>
