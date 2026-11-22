<script setup>
// 统一自定义下拉选择器（替代原生 <select>）。
// props: modelValue, options: [{ value, label, icon?(UiIcon 名), img?(图片URL), disabled }], placeholder, width
import { ref, computed, nextTick, onBeforeUnmount, useId, watch } from 'vue'
import UiIcon from './UiIcon.vue'

const props = defineProps({
  modelValue: { type: [String, Number], default: '' },
  options: { type: Array, required: true },
  placeholder: { type: String, default: '请选择' },
  ariaLabel: { type: String, default: '' },
  disabled: { type: Boolean, default: false },
  block: { type: Boolean, default: false }, // 宽度 100%
})
const emit = defineEmits(['update:modelValue', 'change'])

const open = ref(false)
const root = ref(null)
const btn = ref(null)
const popup = ref(null)
const dropStyle = ref({})
const dropPlacement = ref('down')
const activeIndex = ref(-1)
const listId = `uiselect-list-${useId()}`
let repositionFrame = null

const current = computed(() => props.options.find((o) => o.value === props.modelValue) || null)
const availableIndexes = computed(() => props.options.flatMap((option, optionIndex) => option.disabled ? [] : [optionIndex]))

function initialIndex() {
  const selected = props.options.findIndex(option => option.value === props.modelValue && !option.disabled)
  return selected >= 0 ? selected : (availableIndexes.value[0] ?? -1)
}

function showActive() {
  nextTick(() => {
    const list = popup.value
    const option = list?.children[activeIndex.value]
    if (!open.value || !option) return
    const top = option.offsetTop
    const bottom = top + option.offsetHeight
    if (top < list.scrollTop) list.scrollTop = top
    else if (bottom > list.scrollTop + list.clientHeight) list.scrollTop = bottom - list.clientHeight
  })
}

function toggle() {
  if (props.disabled) return
  open.value = !open.value
  if (open.value) {
    position()
    activeIndex.value = initialIndex()
    showActive()
  }
}

function position() {
  const rect = btn.value?.getBoundingClientRect()
  if (!rect) return
  const zoom = popup.value?.currentCSSZoom || document.body.currentCSSZoom || 1
  const width = window.innerWidth / zoom, height = window.innerHeight / zoom
  const top = rect.top / zoom, bottom = rect.bottom / zoom
  const left = rect.left / zoom
  const w = Math.min(Math.max(0, width - 16), Math.max(rect.width / zoom, 160))
  const below = Math.max(0, height - bottom - 12)
  const above = Math.max(0, top - 12)
  const list = popup.value
  const desiredHeight = list ? Math.min(280, list.scrollHeight + list.offsetHeight - list.clientHeight) : 120
  const up = below < desiredHeight && above > below
  dropPlacement.value = up ? 'up' : 'down'
  dropStyle.value = {
    left: Math.max(8, Math.min(left, width - w - 8)) + 'px',
    width: w + 'px',
    ...(up
      ? { bottom: height - top + 4 + 'px', top: 'auto' }
      : { top: bottom + 4 + 'px', bottom: 'auto' }),
    maxHeight: Math.min(280, up ? above : below) + 'px',
  }
}

// 弹层固定定位：窗口滚动/缩放/内容变化时跟随触发器重算位置
function onReposition(event) {
  if (!open.value || repositionFrame !== null || event.target === popup.value) return
  repositionFrame = requestAnimationFrame(() => {
    repositionFrame = null
    if (open.value) position()
  })
}

function pick(o) {
  if (!open.value || o.disabled) return
  emit('update:modelValue', o.value)
  emit('change', o.value)
  open.value = false
  activeIndex.value = props.options.findIndex((item) => item.value === o.value)
  nextTick(() => btn.value?.focus({ preventScroll: true }))
}

function onDown(e) {
  if (open.value && !root.value?.contains(e.target) && !popup.value?.contains(e.target)) open.value = false
}

function onKey(e) {
  if (props.disabled || e.isComposing) return
  if (e.key === 'Tab') {
    open.value = false
    return
  }
  if (e.key === 'Escape') {
    if (open.value) {
      e.preventDefault()
      e.stopPropagation()
      open.value = false
      btn.value?.focus()
    }
    return
  }
  if (e.key === 'Enter' || e.key === ' ') {
    e.preventDefault()
    e.stopPropagation()
    if (e.repeat) return
    if (!open.value) {
      toggle()
      return
    }
    const option = props.options[activeIndex.value]
    if (option && !option.disabled) pick(option)
    return
  }
  if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(e.key)) return
  e.preventDefault()
  e.stopPropagation()
  if (!open.value) {
    open.value = true
    position()
    activeIndex.value = props.options.findIndex(option => option.value === props.modelValue && !option.disabled)
  }
  const available = availableIndexes.value
  const currentIndex = available.indexOf(activeIndex.value)
  const next = e.key === 'Home' ? 0
    : e.key === 'End' ? available.length - 1
      : e.key === 'ArrowDown' ? Math.min(available.length - 1, currentIndex + 1)
        : currentIndex < 0 ? available.length - 1 : Math.max(0, currentIndex - 1)
  activeIndex.value = available[next] ?? -1
  btn.value?.focus()
  showActive()
}
function onBlur() { open.value = false }
function onFocus(event) {
  if (!root.value?.contains(event.target) && !popup.value?.contains(event.target)) open.value = false
}

watch(() => props.disabled, disabled => { if (disabled) open.value = false })
watch(() => [props.options, props.modelValue], () => {
  if (open.value) { activeIndex.value = initialIndex(); position(); showActive() }
}, { deep: true, flush: 'post' })

function bindListeners() {
  window.addEventListener('mousedown', onDown, true)
  window.addEventListener('focusin', onFocus)
  window.addEventListener('blur', onBlur)
  window.addEventListener('scroll', onReposition, true)
  window.addEventListener('resize', onReposition)
}
function unbindListeners() {
  window.removeEventListener('mousedown', onDown, true)
  window.removeEventListener('focusin', onFocus)
  window.removeEventListener('blur', onBlur)
  window.removeEventListener('scroll', onReposition, true)
  window.removeEventListener('resize', onReposition)
  if (repositionFrame !== null) cancelAnimationFrame(repositionFrame)
  repositionFrame = null
}
watch(open, value => value ? bindListeners() : unbindListeners(), { flush: 'sync' })
watch(open, value => { if (value) position() }, { flush: 'post' })
onBeforeUnmount(unbindListeners)
</script>

<template>
  <div ref="root" class="uiselect" :class="{ block }" @keydown="onKey">
    <button
      ref="btn"
      type="button"
      class="uiselect-btn"
      :class="{ open, placeholder: !current }"
      :disabled="disabled"
      role="combobox"
      :aria-label="ariaLabel || placeholder"
      aria-haspopup="listbox"
      :aria-expanded="open"
      :aria-controls="listId"
      :aria-activedescendant="open && activeIndex >= 0 ? `${listId}-option-${activeIndex}` : undefined"
      @click="toggle"
    >
      <img v-if="current && current.img" :src="current.img" class="uiselect-ico" alt="" />
      <UiIcon v-else-if="current && current.icon" :name="current.icon" :size="14" />
      <span class="uiselect-label">{{ current ? current.label : placeholder }}</span>
      <UiIcon name="chevron-down" :size="12" class="uiselect-arrow" />
    </button>
    <teleport to="body">
      <transition name="uiselect-popup" @before-leave="element => { element.inert = true }">
        <div v-if="open" ref="popup" :id="listId" class="uiselect-drop" :style="dropStyle" :data-placement="dropPlacement" role="listbox" :aria-label="ariaLabel || placeholder || '选项'" @keydown="onKey">
          <div v-if="!options.length" class="uiselect-empty">无可选项</div>
          <button
            v-for="(o, i) in options"
            :key="o.value"
            type="button"
            class="uiselect-opt"
            :class="{ active: o.value === modelValue, highlighted: i === activeIndex, disabled: o.disabled }"
            :id="`${listId}-option-${i}`"
            role="option"
            :aria-selected="o.value === modelValue"
            :aria-disabled="o.disabled || undefined"
            tabindex="-1"
            :disabled="o.disabled"
            @click="pick(o)"
          >
            <img v-if="o.img" :src="o.img" class="uiselect-ico" alt="" />
            <UiIcon v-else-if="o.icon" :name="o.icon" :size="14" />
            <span class="uiselect-label">{{ o.label }}</span>
            <UiIcon v-if="o.value === modelValue" name="check" :size="13" class="uiselect-check" />
          </button>
        </div>
      </transition>
    </teleport>
  </div>
</template>

<style>
.uiselect { display: inline-block; }
.uiselect.block, .uiselect.block .uiselect-btn { width: 100%; }
/* 触发器：细描边柔底，悬停描边加深，展开时主题色晕环 */
.uiselect-btn {
  display: inline-flex; align-items: center; gap: 6px;
  width: 100%;
  height: 30px; padding: 0 8px 0 11px; min-width: 0;
  font-size: 13.5px; color: var(--text-primary);
  background: var(--control-bg); border: 1px solid var(--control-border);
  border-radius: var(--radius-md); cursor: pointer;
  transition: background var(--motion-fast) var(--motion-ease), box-shadow var(--motion-fast) var(--motion-ease), transform var(--motion-fast) var(--motion-spring);
}
.uiselect-btn:hover:not(:disabled) { background: var(--bg-hover); border-color: var(--border-focus); }
.uiselect-btn:active:not(:disabled) { transform: scale(.98); }
.uiselect-btn.open {
  background: var(--bg-hover);
  border-color: var(--color-primary);
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--color-primary) 22%, transparent);
}
.uiselect-btn:disabled { opacity: .55; cursor: not-allowed; }
.uiselect-btn.placeholder { color: var(--text-tertiary); }
.uiselect-label { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; text-align: left; }
.uiselect-ico { width: 15px; height: 15px; object-fit: contain; border-radius: 3px; flex-shrink: 0; }
.uiselect-arrow { flex-shrink: 0; color: var(--text-tertiary); transition: transform 240ms var(--motion-spring); }
.uiselect-btn.open .uiselect-arrow { transform: rotate(180deg); }

.uiselect-drop {
  position: fixed; z-index: 400; overflow-y: auto; padding: 5px;
  background: var(--bg-elevated); overscroll-behavior: contain;
  border: 1px solid var(--border-light);
  border-radius: 12px; box-shadow: var(--shadow-modal);
  transform-origin: top center;
  --uiselect-offset: -4px;
}
.uiselect-drop[data-placement="up"] { transform-origin: bottom center; --uiselect-offset: 4px; }
.uiselect-popup-enter-active { transition: opacity 100ms var(--motion-ease), transform 150ms var(--motion-glide); }
.uiselect-popup-leave-active { transition: opacity 90ms var(--motion-ease), transform 100ms var(--motion-glide); pointer-events: none; }
.uiselect-popup-enter-from, .uiselect-popup-leave-to { opacity: 0; transform: translateY(var(--uiselect-offset)) scale(.99); }
@media (prefers-reduced-motion: reduce) {
  .uiselect-popup-enter-active, .uiselect-popup-leave-active { transition: none; }
  .uiselect-popup-enter-from, .uiselect-popup-leave-to { transform: none; }
}
.uiselect-opt {
  display: flex; align-items: center; gap: 7px; width: 100%;
  padding: 6px 9px; border: none; border-radius: 8px;
  background: transparent; font-size: 13.5px; color: var(--text-primary);
  cursor: pointer; text-align: left;
  transition: background var(--motion-fast) var(--motion-ease), transform var(--motion-fast) var(--motion-spring);
}
.uiselect-opt:hover:not(.disabled) { background: var(--bg-hover); }
.uiselect-opt.highlighted:not(.disabled) { outline: 2px solid var(--border-focus); outline-offset: -2px; }
.uiselect-opt:active:not(.disabled) { transform: scale(.98); }
.uiselect-opt.active {
  color: var(--color-primary); font-weight: 600;
  background: color-mix(in srgb, var(--color-primary) 12%, transparent);
}
.uiselect-opt.disabled { opacity: .45; cursor: not-allowed; }
.uiselect-check { flex-shrink: 0; margin-left: auto; animation: check-pop 260ms var(--motion-spring); }
.uiselect-empty { padding: 14px; text-align: center; font-size: 13px; color: var(--text-tertiary); }
</style>
