<script setup lang="ts">
import { computed, ref } from 'vue'
import useSettingStore from './settingstore'
import message from '../utils/message'

const CONNECTIVITY_CHECK_URL = 'https://www.msftconnecttest.com/connecttest.txt'

const settingStore = useSettingStore()
const cb = (val: any) => settingStore.updateStore(val)
const isCustomProxy = computed(() => !['system', 'none'].includes(settingStore.proxyType))
const proxyLoading = ref(false)

const handleApplyProxy = async () => {
  if (await settingStore.WebSetProxy()) message.success('代理设置已应用')
  else message.error('请填写完整的代理服务器和端口')
}

const handleProxyConn = async () => {
  proxyLoading.value = true
  const controller = new AbortController()
  const timeout = window.setTimeout(() => controller.abort(), 8000)
  try {
    if (!(await settingStore.WebSetProxy())) {
      message.error('请填写完整的代理服务器和端口')
      return
    }
    await fetch(CONNECTIVITY_CHECK_URL, { cache: 'no-store', signal: controller.signal })
    message.success('网络连接正常')
  } catch (error: any) {
    message.error(error?.name === 'AbortError' ? '网络连接超时' : `网络连接失败：${error?.message || '未知错误'}`)
  } finally {
    window.clearTimeout(timeout)
    proxyLoading.value = false
  }
}
</script>

<template>
  <div class="ui-plain-list">
    <div class="ui-plain-row">
      <span class="ui-plain-label">代理类型</span>
      <div class="ui-plain-control">
        <a-select class="ui-control-sm" size="small" :model-value="settingStore.proxyType" @update:model-value="cb({ proxyType: $event })">
          <a-option value="system">系统代理</a-option>
          <a-option value="none">直连</a-option>
          <a-option value="http">HTTP</a-option>
          <a-option value="https">HTTPS</a-option>
          <a-option value="socks5">SOCKS5</a-option>
          <a-option value="socks5h">SOCKS5H</a-option>
        </a-select>
        <a-button type="outline" size="small" @click="handleApplyProxy">应用</a-button>
        <a-button type="outline" size="small" :loading="proxyLoading" @click="handleProxyConn">测试</a-button>
      </div>
    </div>
    <div v-if="isCustomProxy" class="ui-plain-row">
      <span class="ui-plain-label">代理服务器</span>
      <div class="ui-plain-control ui-inline-fields">
        <a-input class="ui-control-md" size="small" placeholder="地址或域名" :model-value="settingStore.proxyHost" @update:model-value="cb({ proxyHost: $event })" />
        <a-input-number class="ui-control-sm" size="small" hide-button placeholder="端口" :model-value="settingStore.proxyPort" @update:model-value="cb({ proxyPort: $event })" />
      </div>
    </div>
    <div v-if="isCustomProxy" class="ui-plain-row">
      <span class="ui-plain-label">代理认证</span>
      <div class="ui-plain-control ui-inline-fields">
        <a-input class="ui-control-md" size="small" placeholder="用户名" :model-value="settingStore.proxyUserName" @update:model-value="cb({ proxyUserName: $event })" />
        <a-input class="ui-control-md" size="small" type="password" placeholder="密码" :model-value="settingStore.proxyPassword" @update:model-value="cb({ proxyPassword: $event })" />
      </div>
    </div>
  </div>
</template>
