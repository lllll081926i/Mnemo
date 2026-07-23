<script setup lang="ts">
import { computed, defineAsyncComponent, onMounted, onUnmounted, ref, watch } from 'vue'
import { KeyboardState, useAppStore, useFootStore, useKeyboardStore, useMouseStore, useSettingStore, useUserStore, useWinStore } from '../store'
import { Music, Pause, Play, SkipBack, SkipForward, Video } from 'lucide-vue-next'
import { onHideRightMenu, TestAlt, TestCtrl, TestKey, TestShift } from '../utils/keyboardhelper'

import Pan from '../pan/index.vue'

import UserInfo from '../user/UserInfo.vue'
import UserLogin from '../user/UserLogin.vue'
import ShutDown from '../setting/ShutDown.vue'

import MyModal from './MyModal.vue'
import { throttle } from '../utils/debounce'
import { startSyncScheduler } from '../sync/syncengine'

const Down = defineAsyncComponent(() => import('../down/index.vue'))
const Share = defineAsyncComponent(() => import('../share/index.vue'))
const Setting = defineAsyncComponent(() => import('../setting/index.vue'))
const Sync = defineAsyncComponent(() => import('../sync/index.vue'))

const panVisible = ref(true)
const appStore = useAppStore()
const settingStore = useSettingStore()
const winStore = useWinStore()
const keyboardStore = useKeyboardStore()
const mouseStore = useMouseStore()
const footStore = useFootStore()
const transferIndicatorTick = ref(0)
let transferIndicatorTimer: ReturnType<typeof setInterval> | undefined
const transferIndicator = computed(() => {
  if (appStore.appTab === 'down' || appStore.appTab === 'setting') return undefined
  const items = [
    footStore.downloadTotalSpeed ? { key: 'download', icon: '↓', label: '下载', speed: footStore.downloadTotalSpeed } : undefined,
    footStore.uploadTotalSpeed ? { key: 'upload', icon: '↑', label: '上传', speed: footStore.uploadTotalSpeed } : undefined
  ].filter(Boolean) as Array<{ key: string; icon: string; label: string; speed: string }>
  if (!items.length) return undefined
  return items[transferIndicatorTick.value % items.length]
})

const handlePanVisible = () => {
  panVisible.value = !panVisible.value
}

const handleThemeClick = (val: any) => {
  if (appStore.appTheme == 'system') {
    if (appStore.appDark) {
      useSettingStore().updateStore({ uiTheme: 'light' })
    } else {
      useSettingStore().updateStore({ uiTheme: 'dark' })
    }
  } else if (appStore.appTheme === 'dark') {
    useSettingStore().updateStore({ uiTheme: 'light' })
  } else if (appStore.appTheme === 'light') {
    useSettingStore().updateStore({ uiTheme: 'dark' })
  }
}
const themeTitle = computed(() => {
  if (appStore.appTheme == 'system') {
    return '自动'
  } else if (appStore.appTheme === 'light') {
    return '浅色'
  } else if (appStore.appTheme === 'dark') {
    return '黑色'
  }
})

const primaryTabDefinitions = [
  { key: 'pan', title: 'Alt+1', label: '网盘' },
  { key: 'down', title: 'Alt+2', label: '传输' },
  { key: 'sync', title: 'Alt+3', label: '同步' },
  { key: 'share', title: 'Alt+4', label: '分享' }
]

const topNavTabs = computed(() => {
  const preferred = settingStore.uiDefaultTab || 'pan'
  const allowed = new Set(primaryTabDefinitions.map((item) => item.key))
  const safe = allowed.has(preferred) ? preferred : 'pan'
  return [...primaryTabDefinitions].sort((a, b) => {
    if (a.key === safe) return -1
    if (b.key === safe) return 1
    return primaryTabDefinitions.findIndex((item) => item.key === a.key) - primaryTabDefinitions.findIndex((item) => item.key === b.key)
  })
})

const handleHideClick = (_e: any) => {
  if (window.WebToElectron) window.WebToElectron({ cmd: useSettingStore().uiExitOnClose ? 'exit' : 'close' })
}
const handleMinClick = (_e: any) => {
  if (window.WebToElectron) window.WebToElectron({ cmd: 'minsize' })
}
const handleMaxClick = (_e: any) => {
  if (window.WebToElectron) window.WebToElectron({ cmd: 'maxsize' })
}

keyboardStore.$subscribe((_m: any, state: KeyboardState) => {
  if (TestAlt('1', state.KeyDownEvent, () => appStore.toggleTab('pan'))) return
  if (TestAlt('2', state.KeyDownEvent, () => appStore.toggleTab('down'))) return
  if (TestAlt('3', state.KeyDownEvent, () => appStore.toggleTab('sync'))) return
  if (TestAlt('4', state.KeyDownEvent, () => appStore.toggleTab('share'))) return
  if (TestAlt('5', state.KeyDownEvent, () => appStore.toggleTab('setting'))) return
  if (TestAlt('f4', state.KeyDownEvent, () => handleHideClick(undefined))) return
  if (TestAlt('m', state.KeyDownEvent, () => handleMinClick(undefined))) return
  if (TestAlt('enter', state.KeyDownEvent, () => handleMaxClick(undefined))) return
  if (TestShift('tab', state.KeyDownEvent, () => appStore.toggleTabNext())) return
  if (TestCtrl('tab', state.KeyDownEvent, () => appStore.toggleTabNextMenu())) return
  if (TestAlt('l', state.KeyDownEvent, () => (useUserStore().userShowLogin = true))) return
  const f11 = () => {
    if (window.WebToElectron) window.WebToElectron({ cmd: 'maxsize' })
  }
  if (TestKey('f11', state.KeyDownEvent, f11)) return
})

const onResize = throttle(() => {
  try {
    const width = document.body.offsetWidth || 960
    const height = document.body.offsetHeight || 720
    if (winStore.width != width || winStore.height != height) {
      winStore.updateStore({ width, height })
    }
  } catch (err) {}
  // let ddsound = document.getElementById('ddsound') as { play: any } | undefined
  // if (ddsound) ddsound.play()
}, 50)

const onKeyDown = (event: KeyboardEvent) => {
  const ele = (event.srcElement || event.target) as any
  const nodeName = ele && ele.nodeName
  if (event.key === 'Tab') {
    event.preventDefault()
    event.stopPropagation()
    event.cancelBubble = true
    event.returnValue = false
    if (nodeName && !'BODY|DIV'.includes(nodeName)) ele.blur()
  }
  if (document.body.getElementsByClassName('arco-modal-container').length) return
  if (event.key === 'Escape' && appStore.appTab === 'setting') {
    event.preventDefault()
    event.stopPropagation()
    appStore.closeSettings()
    return
  }
  if (event.key == 'Control' || event.key == 'Shift' || event.key == 'Alt' || event.key == 'Meta') return
  const isInput = nodeName == 'INPUT' || nodeName == 'TEXTAREA' || false
  if (!isInput) {
    onHideRightMenu()
    keyboardStore.KeyDown(event)
  }
}

const onMouseDown = (event: MouseEvent) => {
  const ele = (event.srcElement || event.target) as any
  const nodeName = ele && ele.nodeName
  if (document.body.getElementsByClassName('arco-modal-container').length) return
  const isInput = nodeName == 'INPUT' || nodeName == 'TEXTAREA' || false
  if (!isInput) {
    mouseStore.KeyDown(event)
  }
}
// Apply saved default tab — watch ensures it fires after store + template are ready
watch(
  () => settingStore.uiDefaultTab,
  (tab) => {
    const allowed = new Set(['pan', 'down', 'sync', 'share', 'setting'])
    if (tab && allowed.has(tab) && appStore.appTab !== tab) {
      appStore.toggleTab(tab)
    }
  },
  { immediate: true }
)

onMounted(() => {
  onResize()
  startSyncScheduler()
  window.addEventListener('resize', onResize, { passive: true })
  window.addEventListener('keydown', onKeyDown, true)
  window.addEventListener('mousedown', onMouseDown, true)
  setTimeout(() => {
    onHideRightMenu()
  }, 300)
  window.addEventListener('click', onHideRightMenu, { passive: true })
  transferIndicatorTimer = setInterval(() => {
    transferIndicatorTick.value++
  }, 2800)
  // 空闲时预加载其余工作区标签页，避免首次切换时现拉 chunk 造成卡顿
  const preloadWorkspaceTabs = () => {
    void import('../down/index.vue')
    void import('../share/index.vue')
    void import('../setting/index.vue')
    void import('../sync/index.vue')
  }
  if (typeof window.requestIdleCallback === 'function') window.requestIdleCallback(preloadWorkspaceTabs, { timeout: 4000 })
  else window.setTimeout(preloadWorkspaceTabs, 2000)
})

onUnmounted(() => {
  window.removeEventListener('resize', onResize)
  window.removeEventListener('keydown', onKeyDown)
  window.removeEventListener('mousedown', onMouseDown)
  window.removeEventListener('click', onHideRightMenu)
  if (transferIndicatorTimer) clearInterval(transferIndicatorTimer)
})
</script>
<template>
  <a-layout style="height: 100vh" draggable="false">
    <a-layout-header id="mnemohead" draggable="false">
      <div id="mnemohead2" class="q-electron-drag">
        <a-button v-show="appStore.appTab === 'pan'" type="text" size="small" @click="handlePanVisible">
          <IconFont name="iconmenuon" />
        </a-button>
        <div class="title">Mnemo</div>

        <a-menu mode="horizontal" :selected-keys="[appStore.appTab]" @update:selected-keys="appStore.toggleTab($event[0])">
          <a-menu-item v-for="item in topNavTabs" :key="item.key" :title="item.title">
            {{ item.label }}
          </a-menu-item>
        </a-menu>

        <div class="flexauto"></div>
        <ShutDown />
        <UserInfo />
        <UserLogin />
        <a-button type="text" tabindex="-1" style="margin-right: 5px" :title="themeTitle" @click="handleThemeClick">
          <IconFont name="iconnight" v-if="appStore.appTheme === 'dark' || (appStore.appTheme == 'system' && appStore.appDark)" />
          <IconFont name="iconday" v-else />
        </a-button>
        <a-button
          type="text"
          tabindex="-1"
          :title="appStore.appTab === 'setting' ? '关闭设置，返回上一页 Alt+5' : '打开设置 Alt+5'"
          :aria-pressed="appStore.appTab === 'setting'"
          :class="appStore.appTab == 'setting' ? 'active' : ''"
          @click="appStore.toggleTab('setting')">
          <IconFont name="iconsetting" />
        </a-button>
        <a-button type="text" tabindex="-1" title="最小化 Alt+M" @click="handleMinClick">
          <IconFont name="iconzuixiaohua" />
        </a-button>
        <a-button type="text" tabindex="-1" title="最大化 Alt+Enter" @click="handleMaxClick">
          <IconFont name="iconfullscreen" />
        </a-button>
        <a-button type="text" tabindex="-1" title="关闭 Alt+F4" @click="handleHideClick">
          <IconFont name="iconclose" />
        </a-button>
      </div>
    </a-layout-header>
    <a-layout-content id="mnemobody">
      <a-tabs type="text" :direction="'horizontal'" class="hidetabs" :justify="true" :active-key="appStore.appTab" lazy-load>
        <a-tab-pane key="pan" title="1">
          <Pan :visible="panVisible" />
        </a-tab-pane>
        <a-tab-pane key="down" title="2">
          <Down />
        </a-tab-pane>
        <a-tab-pane key="sync" title="3">
          <Sync />
        </a-tab-pane>
        <a-tab-pane key="share" title="4">
          <Share />
        </a-tab-pane>
        <a-tab-pane key="setting" title="4">
          <Setting />
        </a-tab-pane>
      </a-tabs>
    </a-layout-content>
    <div v-if="transferIndicator" class="transfer-indicator" :title="`${transferIndicator.label}中`">
      <span class="transfer-indicator-label">{{ transferIndicator.label }}</span>
      <span class="transfer-indicator-speed">{{ transferIndicator.speed }}</span>
      <span class="transfer-indicator-arrow" aria-hidden="true">{{ transferIndicator.icon }}</span>
    </div>
    <MyModal />
  </a-layout>
</template>

<style>
.arco-avatar-circle .arco-avatar-image {
  line-height: 100% !important;
}

.transfer-indicator {
  position: fixed;
  right: 16px;
  bottom: 16px;
  z-index: 30;
  display: inline-flex;
  align-items: center;
  min-height: 28px;
  overflow: hidden;
  color: var(--text-primary);
  font-size: 12px;
  line-height: 1;
  background: var(--bg-surface);
  border: 1px solid var(--border-light);
  border-radius: 16px;
  box-shadow: var(--shadow-sm);
  animation: transfer-indicator-enter 180ms cubic-bezier(0.22, 1, 0.36, 1);
}

.transfer-indicator-label,
.transfer-indicator-arrow {
  display: inline-grid;
  width: 28px;
  height: 28px;
  place-items: center;
  color: var(--color-primary);
  background: var(--bg-hover);
  font-weight: 700;
}

.transfer-indicator-speed {
  min-width: 62px;
  padding: 0 8px;
  color: var(--text-primary);
  font-variant-numeric: tabular-nums;
  text-align: center;
}

.transfer-indicator-arrow {
  color: var(--text-secondary);
  background: transparent;
}

@keyframes transfer-indicator-enter {
  from {
    opacity: 0;
    transform: translateY(4px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.hidetabs {
  position: relative;
  z-index: 1;
  height: 100%;
  background: transparent !important;
}

.hidetabs > .ant-tabs-nav {
  height: 0 !important;
  display: none !important;
}

.hidetabs .ant-tabs-content {
  height: 100%;
  background: transparent !important;
}

.hidetabs > .arco-tabs-content {
  padding-top: 0 !important;
  padding-bottom: 1px !important;
  height: 100%;
  background: transparent !important;
}

.hidetabs .arco-tabs-content-list,
.hidetabs .arco-tabs-pane {
  height: 100%;
  background: transparent !important;
}

.hidetabs > .arco-tabs-content > .arco-tabs-content-list > .arco-tabs-content-item-active {
  animation: workspace-page-enter 160ms ease-out;
}

@keyframes workspace-page-enter {
  from {
    opacity: 0;
    transform: translateY(3px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.hidetabs > .arco-tabs-nav {
  width: 0 !important;
  height: 0 !important;
  display: none !important;
}

@media (prefers-reduced-motion: reduce) {
  #mnemobody *,
  #mnemobody *::before,
  #mnemobody *::after {
    transition-duration: 0.01ms !important;
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
  }
}

.arco-upload-list-item-file-icon {
  margin-right: 4px !important;
}
</style>
