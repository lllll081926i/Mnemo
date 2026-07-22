<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import useSettingStore from './settingstore'
import MySwitch from '../layout/MySwitch.vue'
import { openExternal } from '../utils/electronhelper'
import { getPkgVersion } from '../utils/utils'
import message from '../utils/message'

const platform = window.platform
const settingStore = useSettingStore()
const verLoading = ref(false)
const userDataPath = ref('')
const defaultUserDataPath = ref('')
const userDataLoading = ref(false)
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

onMounted(async () => {
  const paths = await window.WebGetAppPaths?.()
  userDataPath.value = paths?.userDataPath || ''
  defaultUserDataPath.value = paths?.defaultUserDataPath || ''
})

const restartAfterUserDataChange = (value: string) => {
  if (!window.WebSetUserDataPath) {
    message.error('当前环境无法更改用户数据位置')
    return
  }
  userDataLoading.value = true
  window.WebSetUserDataPath(value).then((result) => {
    if (!result?.ok) {
      message.error(result?.error || '无法保存用户数据位置')
      userDataLoading.value = false
      return
    }
    if (!result.requiresRestart) {
      userDataPath.value = result.path || userDataPath.value
      userDataLoading.value = false
      message.info('当前已经使用这个用户数据位置')
      return
    }
    message.success('用户数据已移动，应用将重启并使用新位置')
    window.setTimeout(() => window.WebRelaunch?.({}), 450)
  }).catch((error: any) => {
    userDataLoading.value = false
    message.error(error?.message || '无法保存用户数据位置')
  })
}

const handleSelectUserDataPath = () => {
  window.WebShowOpenDialogSync?.({
    title: '选择用户数据目录',
    buttonLabel: '选择',
    properties: ['openDirectory', 'createDirectory'],
    defaultPath: userDataPath.value || defaultUserDataPath.value
  }, (result: string[] | undefined) => {
    const selected = result?.[0]
    if (selected) restartAfterUserDataChange(selected)
  })
}

const handleResetUserDataPath = () => restartAfterUserDataChange('')
const handleOpenUserDataPath = () => {
  const target = userDataPath.value || defaultUserDataPath.value
  if (target) window.WebShowItemInFolder?.(target)
}
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
      <span class="ui-plain-label">用户数据位置</span>
      <div class="ui-plain-control user-data-control">
        <a-input class="ui-control-lg" size="small" :model-value="userDataPath || defaultUserDataPath" readonly />
        <a-button type="outline" size="small" :disabled="!userDataPath && !defaultUserDataPath" @click="handleOpenUserDataPath">打开</a-button>
        <a-button type="outline" size="small" :loading="userDataLoading" @click="handleSelectUserDataPath">更改</a-button>
        <a-button v-if="userDataPath && defaultUserDataPath && userDataPath !== defaultUserDataPath" type="text" size="small" :disabled="userDataLoading" @click="handleResetUserDataPath">恢复默认</a-button>
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
      <span class="ui-plain-label">关闭即退出</span>
      <div class="ui-plain-control"><MySwitch :value="settingStore.uiExitOnClose" @update:value="cb({ uiExitOnClose: $event })" /></div>
    </div>
  </div>
</template>

<style scoped>
.user-data-control {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.user-data-control .ui-control-lg {
  min-width: 0;
  flex: 1 1 auto;
}
</style>
