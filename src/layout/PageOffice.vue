<script setup lang="ts">
import { KeyboardState, useAppStore, useKeyboardStore } from '../store'
import { onMounted, onUnmounted, ref } from 'vue'
import { TestAlt, TestKey } from '../utils/keyboardhelper'

interface AliyunOfficeSdk {
  config(options: { mount: Element; url: string }): {
    setToken(token: { token: string }): unknown
  }
}

const ALIYUN_OFFICE_SDK_URL = 'https://g.alicdn.com/IMM/office-js/1.1.5/aliyun-web-office-sdk.min.js'
const officeWindow = window as Window & { aliyun?: AliyunOfficeSdk; _Global?: Record<string, unknown>; Global?: Record<string, unknown> }
let officeSdkPromise: Promise<AliyunOfficeSdk> | undefined

const loadAliyunOfficeSdk = (): Promise<AliyunOfficeSdk> => {
  if (officeWindow.aliyun?.config) return Promise.resolve(officeWindow.aliyun)
  if (officeSdkPromise) return officeSdkPromise

  officeSdkPromise = new Promise<AliyunOfficeSdk>((resolve, reject) => {
    const script = document.createElement('script')
    const timeout = window.setTimeout(() => finish(() => reject(new Error('Office SDK 加载超时'))), 15000)

    const finish = (callback: () => void) => {
      window.clearTimeout(timeout)
      script.removeEventListener('load', handleLoad)
      script.removeEventListener('error', handleError)
      callback()
    }
    const handleLoad = () => {
      finish(() => {
        if (!officeWindow.aliyun?.config) {
          reject(new Error('Office SDK 初始化失败'))
          return
        }
        officeWindow.Global = { ...(officeWindow._Global || {}) }
        resolve(officeWindow.aliyun)
      })
    }
    const handleError = () => finish(() => reject(new Error('Office SDK 加载失败')))

    script.src = ALIYUN_OFFICE_SDK_URL
    script.async = true
    script.dataset.mnemoOfficeSdk = 'true'
    script.addEventListener('load', handleLoad, { once: true })
    script.addEventListener('error', handleError, { once: true })
    document.head.append(script)
  }).catch((error) => {
    officeSdkPromise = undefined
    throw error
  })

  return officeSdkPromise
}

const keyboardStore = useKeyboardStore()
keyboardStore.$subscribe((_m: any, state: KeyboardState) => {
  if (TestAlt('f4', state.KeyDownEvent, handleHideClick)) return
  if (TestAlt('m', state.KeyDownEvent, handleMinClick)) return
  if (TestAlt('enter', state.KeyDownEvent, handleMaxClick)) return
  if (TestKey('f11', state.KeyDownEvent, handleMaxClick)) return
})

const onKeyDown = (event: KeyboardEvent) => {
  const ele = (event.srcElement || event.target) as any
  const nodeName = ele && ele.nodeName
  if (document.body.getElementsByClassName('arco-modal-container').length) return
  if (event.key == 'Control' || event.key == 'Shift' || event.key == 'Alt' || event.key == 'Meta') return
  const isInput = nodeName == 'INPUT' || nodeName == 'TEXTAREA' || false
  if (!isInput) {
    keyboardStore.KeyDown(event)
  }
}

const handleHideClick = (_e: any) => {
  if (window.WebToWindow) window.WebToWindow({ cmd: 'close' })
}
const handleMinClick = (_e: any) => {
  if (window.WebToWindow) window.WebToWindow({ cmd: 'minsize' })
}
const handleMaxClick = (_e: any) => {
  if (window.WebToWindow) window.WebToWindow({ cmd: 'maxsize' })
}
const appStore = useAppStore()
const officeState = ref<'idle' | 'loading' | 'ready' | 'error'>(appStore.pageOffice?.access_token ? 'loading' : 'idle')
const officeError = ref('')
let titleTimer: number | undefined
let titleRetryTimer: number | undefined

onMounted(async () => {
  window.addEventListener('keydown', onKeyDown, true)
  if (appStore.pageOffice?.access_token) {
    try {
      const sdk = await loadAliyunOfficeSdk()
      const mount = document.querySelector('#doc-preview-mount')
      if (!mount) throw new Error('Office 预览容器不存在')
      const docOptions = sdk.config({ mount, url: appStore.pageOffice.preview_url || '' })
      docOptions.setToken({ token: appStore.pageOffice.access_token })
      officeState.value = 'ready'
    } catch (error: any) {
      officeState.value = 'error'
      officeError.value = error?.message || 'Office 预览加载失败'
    }
  }
  const name = appStore.pageOffice?.file_name || '文档在线预览'
  document.title = name
  titleTimer = window.setTimeout(() => {
    document.title = name
  }, 1000)
  titleRetryTimer = window.setTimeout(() => {
    document.title = name
  }, 10000)
})

onUnmounted(() => {
  window.removeEventListener('keydown', onKeyDown, true)
  if (titleTimer !== undefined) window.clearTimeout(titleTimer)
  if (titleRetryTimer !== undefined) window.clearTimeout(titleRetryTimer)
})
</script>

<template>
  <a-layout style="height: 100vh; background: #f2f4f7" draggable="false">
    <a-layout-header id="mnemohead" draggable="false">
      <div id="mnemohead2" class="q-electron-drag">
        <a-button type="text" tabindex="-1">
          <IconFont name="iconfile-wps" />
        </a-button>
        <div class="title">{{ appStore.pageOffice?.file_name || '文档在线预览' }}</div>
        <div class="flexauto"></div>
        <a-button type='text' tabindex='-1' title='最小化 Alt+M' @click='handleMinClick'>
          <IconFont name="iconzuixiaohua" />
        </a-button>
        <a-button type='text' tabindex='-1' title='最大化 Alt+Enter' @click='handleMaxClick'>
          <IconFont name="iconfullscreen" />
        </a-button>
        <a-button type='text' tabindex='-1' title='关闭 Alt+F4' @click='handleHideClick'>
          <IconFont name="iconclose" />
        </a-button>
      </div>
    </a-layout-header>
    <a-layout-content style="height: calc(100vh - 42px); padding-top: 8px; background: #f2f4f7">
      <div id="doc-preview" class="doc-preview" style="width: 100%; height: 100%">
        <div v-if="appStore.pageOffice?.access_token" id="doc-preview-mount" class="doc-preview-mount"></div>
        <div v-if="appStore.pageOffice?.access_token && officeState !== 'ready'" class="office-preview-state" :class="{ error: officeState === 'error' }" aria-live="polite">
          {{ officeState === 'error' ? officeError : '正在加载文档...' }}
        </div>
        <iframe
          v-if="!appStore.pageOffice?.access_token"
          id="iframe-preview"
          :src="appStore.pageOffice?.preview_url || ''"
          style="width: 100%; height: 100%; border: none; background: #fff"
        />
      </div>
    </a-layout-content>
  </a-layout>
</template>

<style scoped>
.doc-preview {
  position: relative;
}

.doc-preview-mount {
  width: 100%;
  height: 100%;
}

.office-preview-state {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  color: #4e5969;
  background: #f2f4f7;
  font-size: 14px;
}

.office-preview-state.error {
  color: #c23934;
}
</style>
