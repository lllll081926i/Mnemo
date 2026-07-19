<script setup lang="ts">
import { ref } from 'vue'
import useSettingStore from './settingstore'
import MySwitch from '../layout/MySwitch.vue'
import message from '../utils/message'
/*import HttpsProxyAgent from 'https-proxy-agent'
import { SocksProxyAgent } from 'socks-proxy-agent'*/
import AliHttp from '../aliapi/alihttp'
import nodehttps from 'node:https'

const settingStore = useSettingStore()
const cb = (val: any) => {
  if (!Object.hasOwn(val, 'proxyUseProxy') && settingStore.proxyUseProxy) {
    val.proxyUseProxy = false
  }
  settingStore.updateStore(val)
}
const proxyLoading = ref(false)

const handleProxyConn = async () => {
  proxyLoading.value = true

  let option = {
    strictSSL: false,
    rejectUnauthorized: false,
    timeout: 5000
  }
  const proxy = settingStore.getProxy()
  if (proxy) {
    /*if (settingStore.proxyType.startsWith('http')) {
      const agenth = HttpsProxyAgent(proxy)
      option = Object.assign(option, { agent: agenth })
    } else {
      const agents = new SocksProxyAgent(proxy)
      option = Object.assign(option, { agent: agents })
    }*/

    const result = await new Promise<string>(async (resolve) => {
      nodehttps
        .get(AliHttp.baseApi, option, (res: any) => {
          resolve('success')
        })
        .on('error', (e: any) => {
          let message = e.message || e.code || '网络错误'
          message = message.replace('ERR_SSL_INVALID_LIBRARY_(0)', '不支持双向认证的证书，仅支持单向证书')
          message = message.replace('A "socket" was not created for HTTP request before 5000ms', '网络连接超时失败')
          message = message.replace('Client network socket disconnected before secure TLS connection was established', '无法建立TLS连接')
          resolve(message)
        })
    })
    if (result == 'success') {
      message.success('代理连接成功！可以开启')
    } else {
      message.error('代理设置错误 ' + result)
    }
  } else {
    message.error('代理设置错误')
  }
  proxyLoading.value = false
}
</script>

<template>
  <div class="ui-plain-list">
    <div class="ui-plain-row">
      <span class="ui-plain-label">代理类型</span>
      <div class="ui-plain-control">
        <a-select class="ui-control-sm" size="small" :model-value="settingStore.proxyType" @update:model-value="cb({ proxyType: $event })">
          <a-option value="none">None</a-option>
          <a-option value="http">HTTP</a-option>
          <a-option value="https">HTTPS</a-option>
          <a-option value="socks5">SOCKS5</a-option>
          <a-option value="socks5h">SOCKS5H</a-option>
        </a-select>
      </div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">代理服务器</span>
      <div class="ui-plain-control ui-inline-fields">
        <a-input class="ui-control-md" size="small" placeholder="地址或域名" :model-value="settingStore.proxyHost" @update:model-value="cb({ proxyHost: $event })" />
        <a-input-number class="ui-control-sm" size="small" hide-button placeholder="端口" :model-value="settingStore.proxyPort" @update:model-value="cb({ proxyPort: $event })" />
      </div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">代理认证</span>
      <div class="ui-plain-control ui-inline-fields">
        <a-input class="ui-control-md" size="small" placeholder="用户名" :model-value="settingStore.proxyUserName" @update:model-value="cb({ proxyUserName: $event })" />
        <a-input class="ui-control-md" size="small" type="password" placeholder="密码" :model-value="settingStore.proxyPassword" @update:model-value="cb({ proxyPassword: $event })" />
      </div>
    </div>
    <div class="ui-plain-row">
      <span class="ui-plain-label">启用代理</span>
      <div class="ui-plain-control">
        <MySwitch :value="settingStore.proxyUseProxy" @update:value="cb({ proxyUseProxy: $event })" />
        <a-button type="outline" size="small" :loading="proxyLoading" @click="handleProxyConn">测试</a-button>
      </div>
    </div>
  </div>
</template>
