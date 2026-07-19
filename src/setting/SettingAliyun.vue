<script setup lang="ts">
import MySwitch from '../layout/MySwitch.vue'
import PanDAL from '../pan/pandal'
import useSettingStore from './settingstore'

const settingStore = useSettingStore()
const updateAliNavigation = async (key: 'securityHideBackupDrive' | 'securityHidePicDrive', value: boolean) => {
  await settingStore.updateStore(key === 'securityHideBackupDrive' ? { securityHideBackupDrive: value, securityHideResourceDrive: value } : { [key]: value })
  if (value) PanDAL.aReLoadOneDirToShow('', key === 'securityHidePicDrive' ? 'backup_root' : 'pic_root', true)
}
</script>

<template>
  <div class="ui-plain-list">
    <div class="ui-plain-row">
      <span class="ui-plain-label">网盘分区</span>
      <div class="ui-plain-control">
        <MySwitch :value="settingStore.securityHideBackupDrive" @update:value="updateAliNavigation('securityHideBackupDrive', $event)" />
        显示备份与资源
      </div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">相册入口</span>
      <div class="ui-plain-control">
        <MySwitch :value="settingStore.securityHidePicDrive" @update:value="updateAliNavigation('securityHidePicDrive', $event)" />
        在侧边栏显示相册
      </div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">默认分享有效期</span>
      <div class="ui-plain-control">
        <a-radio-group type="button" size="small" :model-value="settingStore.uiShareDays" @update:model-value="(value: string) => settingStore.updateStore({ uiShareDays: value })">
          <a-radio value="always">永久</a-radio>
          <a-radio value="week">一周</a-radio>
          <a-radio value="month">一月</a-radio>
        </a-radio-group>
      </div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">默认提取码</span>
      <div class="ui-plain-control">
        <a-radio-group type="button" size="small" :model-value="settingStore.uiSharePassword" @update:model-value="(value: string) => settingStore.updateStore({ uiSharePassword: value })">
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
