<script>
// 全局弹窗栈：管理多层弹窗，确保 Esc 按键只关闭最顶层弹窗
const modalStack = []
let globalKeydownBound = false
let pendingRestore = null

function handleGlobalKeydown(event) {
  if (event.key === 'Escape' && !event.defaultPrevented && !event.isComposing && event.keyCode !== 229 && modalStack.length > 0) {
    event.preventDefault()
    if (!event.repeat) modalStack.at(-1).close()
  }
}
</script>

<script setup>
import { nextTick, onMounted, onBeforeUnmount, ref } from 'vue'
import { WindowMinimise, WindowToggleMaximise, WindowIsMaximised } from '../../wailsjs/runtime/runtime'
import UiIcon from './UiIcon.vue'

const props = defineProps({
  title: { type: String, default: '' },
  width: { type: String, default: '' },
  bodyClass: { type: String, default: 'modal-body' },
  dialogClass: { type: String, default: '' },
  maskClass: { type: String, default: '' },
  windowControls: { type: Boolean, default: false },
  initialFocus: { type: String, default: '' },
  closeLabel: { type: String, default: '关闭对话框' },
  dialogStyle: { type: Object, default: () => ({}) },
  // 沉浸式媒体预览在自身舞台中提供关闭/窗口控制，不渲染通用实体标题栏。
  hideHead: { type: Boolean, default: false },
  focusDialog: { type: Boolean, default: false },
})
const emit = defineEmits(['close'])
const dialogEl = ref(null)
const windowMaximized = ref(false)
const handleClose = () => emit('close')
const entry = { close: handleClose, element: null, mask: null, returnTo: null }

function focusableElements() {
  if (!dialogEl.value) return []
  return [...dialogEl.value.querySelectorAll('button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), summary, [tabindex]:not([tabindex="-1"])')]
    .filter((el) => el.tabIndex >= 0 && !el.matches(':disabled') && !el.closest('[inert], [hidden], [aria-hidden="true"]') && (el.getClientRects().length > 0 || el === document.activeElement))
}

function onDialogKeydown(e) {
  if (e.key !== 'Tab') return
  if (modalStack.at(-1)?.close !== handleClose) return
  const items = focusableElements()
  if (!items.length) {
    e.preventDefault()
    dialogEl.value?.focus()
    return
  }
  const first = items[0]
  const last = items[items.length - 1]
  if (document.activeElement === dialogEl.value || !dialogEl.value.contains(document.activeElement)) {
    e.preventDefault()
    const target = e.shiftKey ? last : first
    target.focus()
  } else if (e.shiftKey && document.activeElement === first) {
    e.preventDefault()
    last.focus()
  } else if (!e.shiftKey && document.activeElement === last) {
    e.preventDefault()
    first.focus()
  }
}

function minimiseWindow() {
  try { WindowMinimise() } catch { /* 浏览器预览环境没有 Wails runtime。 */ }
}

function toggleWindowMaximise() {
  try {
    WindowToggleMaximise()
    windowMaximized.value = !windowMaximized.value
    WindowIsMaximised().then((value) => { windowMaximized.value = !!value }).catch(() => {})
  } catch { /* 浏览器预览环境没有 Wails runtime。 */ }
}

onMounted(() => {
  const active = document.activeElement
  entry.element = dialogEl.value
  entry.mask = entry.element?.parentElement
  entry.returnTo = pendingRestore && (active === document.body || pendingRestore.element?.contains(active)) ? pendingRestore.returnTo : active
  if (props.windowControls) {
    try { WindowIsMaximised().then((value) => { windowMaximized.value = !!value }).catch(() => {}) } catch {}
  }
  modalStack.at(-1)?.mask?.setAttribute('inert', '')
  modalStack.push(entry)
  if (!globalKeydownBound) {
    window.addEventListener('keydown', handleGlobalKeydown)
    globalKeydownBound = true
  }
  nextTick(() => {
    if (modalStack.at(-1) !== entry || !dialogEl.value || dialogEl.value.contains(document.activeElement)) return
    if (props.focusDialog) { dialogEl.value.focus({ preventScroll: true }); return }
    const items = focusableElements()
    const preferred = props.initialFocus ? items.find(element => element.matches(props.initialFocus)) : null
    const input = items.find(element => element.matches('input:not([type="checkbox"]):not([type="radio"]), textarea, [role="combobox"]'))
    ;(preferred || input || items.find(element => element.closest('.modal-actions')) || items[0])?.focus({ preventScroll: true })
    if (!dialogEl.value.contains(document.activeElement)) dialogEl.value.focus({ preventScroll: true })
  })
})

onBeforeUnmount(() => {
  const wasTop = modalStack.at(-1) === entry
  entry.mask?.setAttribute('inert', '')
  const idx = modalStack.indexOf(entry)
  if (idx >= 0) modalStack.splice(idx, 1)
  for (const remaining of modalStack) {
    if (entry.element?.contains(remaining.returnTo)) remaining.returnTo = entry.returnTo
  }
  if (modalStack.length === 0 && globalKeydownBound) {
    window.removeEventListener('keydown', handleGlobalKeydown)
    globalKeydownBound = false
  }
  if (!wasTop) return
  const nextTop = modalStack.at(-1)
  nextTop?.mask?.removeAttribute('inert')
  pendingRestore = entry
  nextTick(() => {
    if (pendingRestore !== entry) return
    pendingRestore = null
    if (modalStack.at(-1) !== nextTop) return
    const top = nextTop?.element
    const target = entry.returnTo
    if (target?.isConnected && (!top || top.contains(target))) target.focus?.({ preventScroll: true })
    else if (top && !top.contains(document.activeElement)) top.focus({ preventScroll: true })
  })
})
</script>

<template>
  <teleport to="body">
    <transition name="modal-fade" appear>
      <div class="modal-mask" :class="maskClass" @click.self="emit('close')">
        <div
          ref="dialogEl"
          class="modal"
          :class="dialogClass"
          :style="{ ...(width ? { width } : {}), ...dialogStyle }"
          role="dialog"
          aria-modal="true"
          :aria-label="title || '对话框'"
          tabindex="-1"
          @keydown="onDialogKeydown"
        >
          <div v-if="!hideHead" class="modal-head">
            <slot name="head">
              <h3>{{ title }}</h3>
            </slot>
            <div class="modal-head-actions">
              <div class="modal-head-extra"><slot name="head-extra" /></div>
              <div class="modal-window-controls" aria-label="窗口控制">
                <button v-if="windowControls" type="button" class="modal-window-btn" title="最小化" aria-label="最小化窗口" @click="minimiseWindow">
                  <UiIcon name="window-minimize" :size="14" />
                </button>
                <button v-if="windowControls" type="button" class="modal-window-btn" :title="windowMaximized ? '还原窗口' : '最大化窗口'" :aria-label="windowMaximized ? '还原窗口' : '最大化窗口'" @click="toggleWindowMaximise">
                  <UiIcon :name="windowMaximized ? 'window-restore' : 'window-maximize'" :size="14" />
                </button>
                <button type="button" class="modal-window-btn modal-window-close" title="关闭 (Esc)" :aria-label="closeLabel" @click="emit('close')">
                  <UiIcon name="close" :size="15" />
                </button>
              </div>
            </div>
          </div>
          <div :class="bodyClass">
            <slot />
          </div>
          <div v-if="$slots.actions" class="modal-actions">
            <slot name="actions" />
          </div>
        </div>
      </div>
    </transition>
  </teleport>
</template>
