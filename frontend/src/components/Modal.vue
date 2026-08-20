<script>
// 全局弹窗栈：管理多层弹窗，确保 Esc 按键只关闭最顶层弹窗
const modalStack = []
let globalKeydownBound = false

function handleGlobalKeydown(e) {
  if (e.key === 'Escape' && modalStack.length > 0) {
    const topHandler = modalStack[modalStack.length - 1]
    if (typeof topHandler === 'function') {
      topHandler()
    }
  }
}
</script>

<script setup>
import { nextTick, onMounted, onBeforeUnmount, ref } from 'vue'
import UiIcon from './UiIcon.vue'

const props = defineProps({
  title: { type: String, default: '' },
  width: { type: String, default: '' },
  bodyClass: { type: String, default: 'modal-body' },
  dialogClass: { type: String, default: '' },
  dialogStyle: { type: Object, default: () => ({}) },
})
const emit = defineEmits(['close'])
const dialogEl = ref(null)
let previousActiveElement = null

const handleClose = () => emit('close')

function focusableElements() {
  if (!dialogEl.value) return []
  return [...dialogEl.value.querySelectorAll('button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])')]
    .filter((el) => el.offsetParent !== null || el === document.activeElement)
}

function onDialogKeydown(e) {
  if (e.key !== 'Tab') return
  const items = focusableElements()
  if (!items.length) {
    e.preventDefault()
    dialogEl.value?.focus()
    return
  }
  const first = items[0]
  const last = items[items.length - 1]
  if (e.shiftKey && document.activeElement === first) {
    e.preventDefault()
    last.focus()
  } else if (!e.shiftKey && document.activeElement === last) {
    e.preventDefault()
    first.focus()
  }
}

onMounted(() => {
  previousActiveElement = document.activeElement
  modalStack.push(handleClose)
  if (!globalKeydownBound) {
    window.addEventListener('keydown', handleGlobalKeydown)
    globalKeydownBound = true
  }
  nextTick(() => {
    if (!dialogEl.value || dialogEl.value.contains(document.activeElement)) return
    focusableElements()[0]?.focus()
    if (!dialogEl.value.contains(document.activeElement)) dialogEl.value.focus()
  })
})

onBeforeUnmount(() => {
  const idx = modalStack.lastIndexOf(handleClose)
  if (idx >= 0) modalStack.splice(idx, 1)
  if (modalStack.length === 0 && globalKeydownBound) {
    window.removeEventListener('keydown', handleGlobalKeydown)
    globalKeydownBound = false
  }
  if (previousActiveElement && typeof previousActiveElement.focus === 'function') {
    nextTick(() => previousActiveElement?.focus?.())
  }
})
</script>

<template>
  <teleport to="body">
    <transition name="modal-fade">
      <div class="modal-mask" @click.self="emit('close')">
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
          <div v-if="title || $slots.head" class="modal-head">
            <slot name="head">
              <h3>{{ title }}</h3>
            </slot>
            <div style="display:flex;align-items:center;gap:4px">
              <slot name="head-extra" />
              <button type="button" class="icon-btn" style="width:28px;height:28px" title="关闭 (Esc)" aria-label="关闭对话框" @click="emit('close')">
                <UiIcon name="close" :size="14" />
              </button>
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
