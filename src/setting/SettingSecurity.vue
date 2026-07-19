<script setup lang="ts">
import { computed } from 'vue'
import MySwitch from '../layout/MySwitch.vue'
import useSettingStore from './settingstore'
import { modalPassword } from '../utils/modal'

const settingStore = useSettingStore()
const cb = async (val: any) => settingStore.updateStore(val)
const disabled = computed(() => !settingStore.securityPassword)
const handlerPassword = (optType: string, event: any) => {
  modalPassword(optType, (success) => {
    if (optType !== 'confirm' || !success) return
    cb(event)
  })
}
</script>

<template>
  <div class="ui-plain-list">
    <div class="ui-plain-row">
      <span class="ui-plain-label">默认加密算法</span>
      <div class="ui-plain-control">
        <a-radio-group type="button" size="small" :model-value="settingStore.securityEncType" @update:model-value="cb({ securityEncType: $event })">
          <a-radio value="aesctr">AES-CTR</a-radio>
          <a-radio value="rc4md5">RC4-MD5</a-radio>
        </a-radio-group>
      </div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">默认解密密码</span>
      <div class="ui-plain-control">
        <a-button v-if="!settingStore.securityPassword" type="outline" size="small" @click="handlerPassword('new', '')">设置</a-button>
        <a-button v-else type="outline" size="small" @click="handlerPassword('modify', '')">修改</a-button>
        <a-popconfirm v-if="settingStore.securityPassword" content="确认要删除密码？" @ok="handlerPassword('del', '')"><a-button type="outline" size="small" status="danger">删除</a-button></a-popconfirm>
      </div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">加密文件名</span>
      <div class="ui-plain-control"><MySwitch :value="settingStore.securityEncFileName" :disabled="disabled" @update:value="cb({ securityEncFileName: $event })" /></div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">隐藏扩展名</span>
      <div class="ui-plain-control"><MySwitch :value="settingStore.securityEncFileNameHideExt" :disabled="disabled" @update:value="cb({ securityEncFileNameHideExt: $event })" /></div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">敏感操作确认</span>
      <div class="ui-plain-control"><MySwitch :value="settingStore.securityPasswordConfirm" :disabled="disabled" @change="handlerPassword('confirm', { securityPasswordConfirm: $event })" /></div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">自动解密文件名</span>
      <div class="ui-plain-control"><MySwitch :value="settingStore.securityFileNameAutoDecrypt" :disabled="disabled" @change="handlerPassword('confirm', { securityFileNameAutoDecrypt: $event })" /></div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">预览和下载自动解密</span>
      <div class="ui-plain-control"><MySwitch :value="settingStore.securityPreviewAutoDecrypt" :disabled="disabled" @change="handlerPassword('confirm', { securityPreviewAutoDecrypt: $event })" /></div>
    </div>
  </div>
</template>
