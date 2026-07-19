<script setup lang="ts">
import MySwitch from '../layout/MySwitch.vue'
import PanDAL from '../pan/pandal'
import { copyToClipboard } from '../utils/electronhelper'
import message from '../utils/message'
import useSettingStore from './settingstore'

const settingStore = useSettingStore()
const updateAliNavigation = async (key: 'securityHideBackupDrive' | 'securityHidePicDrive', visible: boolean) => {
  const hidden = !visible
  await settingStore.updateStore(key === 'securityHideBackupDrive' ? { securityHideBackupDrive: hidden, securityHideResourceDrive: hidden } : { [key]: hidden })
  if (hidden) PanDAL.aReLoadOneDirToShow('', key === 'securityHidePicDrive' ? 'backup_root' : 'pic_root', true)
}

const copyCookies = async () => {
  const cookies = (await window.WebGetCookies({ url: 'https://www.aliyundrive.com' })) as Array<{ name: string; value: string }>
  if (!cookies.length) {
    message.error('当前阿里网盘账号没有可复制的 Cookie')
    return
  }
  copyToClipboard(cookies.map((cookie) => `${cookie.name}=${cookie.value};`).join(''))
  message.success('当前阿里网盘账号 Cookie 已复制')
}
</script>

<template>
  <div class="ui-plain-list">
    <div class="ui-plain-row">
      <span class="ui-plain-label">优先显示</span>
      <div class="ui-plain-control">
        <a-select class="ui-control-sm" size="small" :model-value="settingStore.uiShowPanRootFirst" @update:model-value="(value) => settingStore.updateStore({ uiShowPanRootFirst: String(value) })">
          <a-option value="all">全部</a-option>
          <a-option value="backup">备份盘</a-option>
          <a-option value="resource">资源盘</a-option>
        </a-select>
      </div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">网盘分区</span>
      <div class="ui-plain-control">
        <MySwitch :value="!settingStore.securityHideBackupDrive" @update:value="updateAliNavigation('securityHideBackupDrive', $event)" />
        显示备份与资源
      </div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">相册入口</span>
      <div class="ui-plain-control">
        <MySwitch :value="!settingStore.securityHidePicDrive" @update:value="updateAliNavigation('securityHidePicDrive', $event)" />
        在侧边栏显示相册
      </div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">当前账号 Cookie</span>
      <div class="ui-plain-control"><a-button type="outline" size="small" tabindex="-1" @click="copyCookies">复制</a-button></div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">默认分享有效期</span>
      <div class="ui-plain-control">
        <a-radio-group type="button" size="small" :model-value="settingStore.uiShareDays" @update:model-value="(value) => settingStore.updateStore({ uiShareDays: String(value) })">
          <a-radio value="always">永久</a-radio>
          <a-radio value="week">一周</a-radio>
          <a-radio value="month">一月</a-radio>
        </a-radio-group>
      </div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">默认提取码</span>
      <div class="ui-plain-control">
        <a-radio-group type="button" size="small" :model-value="settingStore.uiSharePassword" @update:model-value="(value) => settingStore.updateStore({ uiSharePassword: String(value) })">
          <a-radio value="random">随机</a-radio>
          <a-radio value="last">上次</a-radio>
          <a-radio value="nopassword">无</a-radio>
        </a-radio-group>
      </div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">分享链接模板</span>
      <div class="ui-plain-control"><a-input class="ui-control-lg" size="small" :model-value="settingStore.uiShareFormate" @update:model-value="(value: string) => settingStore.updateStore({ uiShareFormate: value })" /></div>
    </div>
  </div>
</template>
