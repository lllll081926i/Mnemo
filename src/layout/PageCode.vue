<script setup lang="ts">
import AliFile from '../aliapi/file'
import { KeyboardState, useAppStore, useKeyboardStore } from '../store'
import message from '../utils/message'
import { nextTick, onMounted, onUnmounted, ref } from 'vue'
import { TestAlt, TestKey } from '../utils/keyboardhelper'

interface PrismApi {
  highlightAllUnder(element: Element): void
}

const PRISM_SCRIPT_URL = './prism.js'
const PRISM_STYLE_URL = './prism-vsc-dark-plus.css'
const codeWindow = window as Window & { Prism?: PrismApi }
let prismPromise: Promise<PrismApi> | undefined

const ensurePrismStyles = () => {
  if (document.querySelector('link[data-mnemo-prism-style]')) return
  const link = document.createElement('link')
  link.rel = 'stylesheet'
  link.href = PRISM_STYLE_URL
  link.dataset.mnemoPrismStyle = 'true'
  document.head.append(link)
}

const loadPrism = (): Promise<PrismApi> => {
  ensurePrismStyles()
  if (codeWindow.Prism?.highlightAllUnder) return Promise.resolve(codeWindow.Prism)
  if (prismPromise) return prismPromise

  prismPromise = new Promise<PrismApi>((resolve, reject) => {
    const script = document.createElement('script')
    const timeout = window.setTimeout(() => finish(() => reject(new Error('代码高亮组件加载超时'))), 10000)
    const finish = (callback: () => void) => {
      window.clearTimeout(timeout)
      script.removeEventListener('load', handleLoad)
      script.removeEventListener('error', handleError)
      callback()
    }
    const handleLoad = () => finish(() => codeWindow.Prism?.highlightAllUnder ? resolve(codeWindow.Prism) : reject(new Error('代码高亮组件初始化失败')))
    const handleError = () => finish(() => reject(new Error('代码高亮组件加载失败')))

    script.src = PRISM_SCRIPT_URL
    script.async = true
    script.dataset.mnemoPrism = 'true'
    script.addEventListener('load', handleLoad, { once: true })
    script.addEventListener('error', handleError, { once: true })
    document.head.append(script)
  }).catch((error) => {
    prismPromise = undefined
    throw error
  })

  return prismPromise
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

const appStore = useAppStore()
const codeBlock = ref()
const lang = ref(appStore.pageCode?.code_ext ? 'language-' + appStore.pageCode.code_ext : 'language-plain')
const codeString = ref('')
const format = ref(false)
let titleTimer: number | undefined
let titleRetryTimer: number | undefined

onMounted(() => {
  window.addEventListener('keydown', onKeyDown, true)
  const name = appStore.pageCode?.file_name || '文档在线预览'
  document.title = name
  titleTimer = window.setTimeout(() => {
    document.title = name
  }, 1000)
  titleRetryTimer = window.setTimeout(() => {
    document.title = name
  }, 10000)

  void loadCode()
})

onUnmounted(() => {
  window.removeEventListener('keydown', onKeyDown, true)
  if (titleTimer !== undefined) window.clearTimeout(titleTimer)
  if (titleRetryTimer !== undefined) window.clearTimeout(titleRetryTimer)
})

const handleHideClick = (_e: any) => {
  if (window.WebToWindow) window.WebToWindow({ cmd: 'close' })
}
const handleMinClick = (_e: any) => {
  if (window.WebToWindow) window.WebToWindow({ cmd: 'minsize' })
}
const handleMaxClick = (_e: any) => {
  if (window.WebToWindow) window.WebToWindow({ cmd: 'maxsize' })
}

const loadCode = async () => {
  const pageCode = appStore.pageCode
  if (!pageCode) {
    message.error('缺少文件预览信息')
    return
  }

  try {
    const data = await AliFile.ApiFileDownText(pageCode.user_id, pageCode.drive_id, pageCode.file_id, pageCode.file_size, 512 * 1024, pageCode.encType, pageCode.password)
    if (typeof data !== 'string') throw new Error('文件内容格式无效')
    if (pageCode.file_size > 512 * 1024) {
      message.info('文件较大，只显示了前 512KB 的内容')
    }
    let fext = pageCode.code_ext || 'plain'
    const fsub = data.substring(0, Math.min(200, data.length))
    if (fext == 'plain' && fsub.includes('<?xml')) {
      fext = 'xml'
      lang.value = 'language-xml'
    }
    if (fext == 'plain' && fsub.indexOf('{') >= 0 && fsub.indexOf(':') > 0 && fsub.indexOf('}') > 0 && fsub.indexOf('"') > 0) {
      fext = 'json'
      lang.value = 'language-json'
    }

    const noformat = pageCode.file_size > 512 * 1024 || fext == 'plain'
    codeString.value = data
    format.value = !noformat

    if (noformat) return
    await nextTick()
    try {
      const prism = await loadPrism()
      if (codeBlock.value) prism.highlightAllUnder(codeBlock.value)
    } catch (error: any) {
      message.warning(error?.message || '代码高亮加载失败，已显示原始内容')
    }
  } catch (error: any) {
    message.error(error?.message || '文件内容加载失败')
  }
}
</script>

<template>
  <a-layout style="height: 100vh" draggable="false">
    <a-layout-header id="mnemohead" draggable="false">
      <div id="mnemohead2" class="q-electron-drag">
        <a-button type="text" tabindex="-1">
          <IconFont name="icondebug" v-if="format" />
          <IconFont name="iconfile-txt" v-else />
        </a-button>
        <div class="title">{{ appStore.pageCode?.file_name || '文档在线预览' }}</div>
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
    <a-layout-content style="height: calc(100vh - 42px)">
      <div id="doc-preview" class="doc-preview" style="width: 100%; height: 100%; overflow: auto">
        <div ref="codeBlock" class="fullwidthcode">
          <pre v-if="format" :class="'line-numbers ' + lang + ' format'">
            <code>{{ codeString }}</code>
          </pre>
          <p v-else class="noformat">{{ codeString }}</p>
        </div>
      </div>
    </a-layout-content>
  </a-layout>
</template>

<style>
.fullwidthcode {
  min-height: 100%;
  padding: 8px 0 8px 0;
}

.fullwidthcode .format {
  font-size: 14px;
  background-color: #1e1e1e66;
  user-select: text;
  -webkit-user-drag: auto;
  width: fit-content;
  white-space: pre-wrap;
  min-width: 100%;
}

.fullwidthcode .noformat {
  font-size: 14px;
  color: rgb(217, 217, 217);
  background-color: #1e1e1e66;
  user-select: text;
  -webkit-user-drag: auto;
  width: 100%;
  white-space: normal;
  word-wrap: break-word;
  word-break: break-all;
  display: inline-block;
  min-width: 100%;
}

.fullwidthcode pre:focus {
  outline: none;
}

.fullwidthcode pre * {
  user-select: text;
  -webkit-user-drag: auto;
}
</style>
