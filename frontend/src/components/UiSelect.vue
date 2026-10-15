<script setup>
// 统一自定义下拉选择器（替代原生 <select>）。
// props: modelValue, options: [{ value, label, icon?(UiIcon 名), img?(图片URL), disabled }], placeholder, width
import { ref, computed, nextTick, onMounted, onBeforeUnmount, useId, watch } from 'vue'
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
const activeIndex = ref(-1)
const listId = `uiselect-list-${useId()}`

const current = computed(() => props.options.find((o) => o.value === props.modelValue) || null)
const availableIndexes = computed(() => props.options.flatMap((option, optionIndex) => option.disabled ? [] : [optionIndex]))

function initialIndex() {
  const selected = props.options.findIndex(option => option.value === props.modelValue && !option.disabled)
  return selected >= 0 ? selected : (availableIndexes.value[0] ?? -1)
}

function showActive() {
  nextTick(() => {
    if (!open.value) return
    popup.value?.querySelectorAll('.uiselect-opt')[activeIndex.value]?.scrollIntoView?.({ block: 'nearest' })
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
  const r = btn.value && btn.value.getBoundingClientRect()
  if (!r) return
  const w = Math.max(r.width, 160)
  const maxH = 280
  const below = window.innerHeight - r.bottom - 8
  const up = below < 120 && r.top > below
  dropStyle.value = {
    left: Math.max(8, Math.min(r.left, window.innerWidth - w - 8)) + 'px',
    width: w + 'px',
    ...(up
      ? { bottom: window.innerHeight - r.top + 4 + 'px', top: 'auto' }
      : { top: Math.min(r.bottom + 4, window.innerHeight - maxH - 8) + 'px', bottom: 'auto' }),
    maxHeight: maxH + 'px',
  }
}

function pick(o) {
  if (o.disabled) return
  emit('update:modelValue', o.value)
  emit('change', o.value)
  open.value = false
  activeIndex.value = props.options.findIndex((item) => item.value === o.value)
  nextTick(() => btn.value?.focus())
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
  if (open.value) { activeIndex.value = initialIndex(); showActive() }
}, { deep: true })

onMounted(() => {
  window.addEventListener('mousedown', onDown, true)
  window.addEventListener('focusin', onFocus)
  window.addEventListener('blur', onBlur)
})
onBeforeUnmount(() => {
  window.removeEventListener('mousedown', onDown, true)
  window.removeEventListener('focusin', onFocus)
  window.removeEventListener('blur', onBlur)
})
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
      <transition name="popover-zoom">
        <div v-if="open" ref="popup" :id="listId" class="uiselect-drop" :style="dropStyle" role="listbox" :aria-label="ariaLabel || placeholder || '选项'" @keydown="onKey">
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

/* 浮层：毛玻璃卡片，选项逐个弹入，选中项打勾弹跳 */
.uiselect-drop {
  position: fixed; z-index: 400; overflow-y: auto; padding: 5px;
  background: color-mix(in srgb, var(--bg-elevated) 88%, transparent);
  backdrop-filter: blur(18px) saturate(1.5); -webkit-backdrop-filter: blur(18px) saturate(1.5);
  border: 1px solid var(--border-light);
  border-radius: 12px; box-shadow: var(--shadow-modal);
  transform-origin: top center;
}
.uiselect-opt {
  display: flex; align-items: center; gap: 7px; width: 100%;
  padding: 6px 9px; border: none; border-radius: 8px;
  background: transparent; font-size: 13.5px; color: var(--text-primary);
  cursor: pointer; text-align: left;
  animation: uiselect-opt-in .24s var(--motion-ease) both;
  transition: background var(--motion-fast) var(--motion-ease), transform var(--motion-fast) var(--motion-spring);
}
/* 前 10 项依次弹入（波纹入场） */
.uiselect-opt:nth-child(2) { animation-delay: 20ms; }
.uiselect-opt:nth-child(3) { animation-delay: 40ms; }
.uiselect-opt:nth-child(4) { animation-delay: 58ms; }
.uiselect-opt:nth-child(5) { animation-delay: 74ms; }
.uiselect-opt:nth-child(6) { animation-delay: 88ms; }
.uiselect-opt:nth-child(7) { animation-delay: 100ms; }
.uiselect-opt:nth-child(8) { animation-delay: 110ms; }
.uiselect-opt:nth-child(9) { animation-delay: 118ms; }
.uiselect-opt:nth-child(n+10) { animation-delay: 124ms; }
@keyframes uiselect-opt-in {
  from { opacity: 0; transform: translateY(4px) scale(.98); }
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
