<script setup>
// 统一自定义下拉选择器（替代原生 <select>）。
// props: modelValue, options: [{ value, label, icon?(UiIcon 名), img?(图片URL), disabled }], placeholder, width
import { ref, computed, nextTick, onMounted, onBeforeUnmount } from 'vue'
import UiIcon from './UiIcon.vue'

let selectSequence = 0

const props = defineProps({
  modelValue: { type: [String, Number], default: '' },
  options: { type: Array, required: true },
  placeholder: { type: String, default: '请选择' },
  disabled: { type: Boolean, default: false },
  block: { type: Boolean, default: false }, // 宽度 100%
})
const emit = defineEmits(['update:modelValue', 'change'])

const open = ref(false)
const root = ref(null)
const btn = ref(null)
const dropStyle = ref({})
const activeIndex = ref(-1)
const listId = `uiselect-list-${++selectSequence}`

const current = computed(() => props.options.find((o) => o.value === props.modelValue) || null)

function toggle() {
  if (props.disabled) return
  open.value = !open.value
  if (open.value) {
    position()
    activeIndex.value = Math.max(0, props.options.findIndex((o) => o.value === props.modelValue))
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
  if (open.value && root.value && !root.value.contains(e.target) && !(e.target.closest && e.target.closest('.uiselect-drop'))) open.value = false
}
function focusOption() {
  nextTick(() => document.getElementById(listId)?.querySelectorAll('.uiselect-opt')[activeIndex.value]?.focus())
}

function onKey(e) {
  const available = props.options.filter((o) => !o.disabled)
  if (e.key === 'Escape') {
    if (open.value) {
      e.preventDefault()
      open.value = false
      btn.value?.focus()
    }
    return
  }
  if (e.key === 'Enter' || e.key === ' ') {
    e.preventDefault()
    if (!open.value) {
      toggle()
      focusOption()
      return
    }
    const option = props.options[activeIndex.value]
    if (option && !option.disabled) pick(option)
    return
  }
  if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(e.key)) return
  e.preventDefault()
  if (!open.value) {
    open.value = true
    position()
  }
  const current = Math.max(0, available.findIndex((o) => o.value === props.options[activeIndex.value]?.value))
  const next = e.key === 'Home' ? 0
    : e.key === 'End' ? available.length - 1
      : e.key === 'ArrowDown' ? Math.min(available.length - 1, current + 1)
        : Math.max(0, current - 1)
  const target = available[next]
  activeIndex.value = target ? props.options.findIndex((o) => o.value === target.value) : -1
  focusOption()
}
function onBlur() { open.value = false }

onMounted(() => {
  window.addEventListener('mousedown', onDown, true)
  window.addEventListener('keydown', onKey)
  window.addEventListener('blur', onBlur)
})
onBeforeUnmount(() => {
  window.removeEventListener('mousedown', onDown, true)
  window.removeEventListener('keydown', onKey)
  window.removeEventListener('blur', onBlur)
})
</script>

<template>
  <div ref="root" class="uiselect" :class="{ block }" @keydown.stop="onKey">
    <button
      ref="btn"
      type="button"
      class="uiselect-btn"
      :class="{ open, placeholder: !current }"
      :disabled="disabled"
      role="combobox"
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
        <div v-if="open" :id="listId" class="uiselect-drop" :style="dropStyle" role="listbox" :aria-label="placeholder || '选项'" @keydown.stop="onKey">
          <div v-if="!options.length" class="uiselect-empty">无可选项</div>
          <button
            v-for="(o, i) in options"
            :key="o.value"
            type="button"
            class="uiselect-opt"
            :class="{ active: o.value === modelValue, disabled: o.disabled }"
            :id="`${listId}-option-${i}`"
            role="option"
            :aria-selected="o.value === modelValue"
            :aria-disabled="o.disabled || undefined"
            :tabindex="o.disabled ? -1 : (i === activeIndex ? 0 : -1)"
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
.uiselect-btn {
  display: inline-flex; align-items: center; gap: 6px;
  width: 100%;
  height: 28px; padding: 0 8px 0 10px; min-width: 0;
  font-size: 13.5px; color: var(--text-primary);
  background: var(--control-bg); border: 1px solid var(--control-border);
  border-radius: var(--radius-sm); cursor: pointer;
  transition: border-color var(--motion-fast) var(--motion-ease), box-shadow var(--motion-fast) var(--motion-ease), background var(--motion-fast) var(--motion-ease);
}
.uiselect-btn:hover:not(:disabled) { border-color: var(--color-primary); }
.uiselect-btn.open {
  border-color: var(--color-primary);
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--color-primary) 18%, transparent);
}
.uiselect-btn:disabled { opacity: .55; cursor: not-allowed; }
.uiselect-btn.placeholder { color: var(--text-tertiary); }
.uiselect-label { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; text-align: left; }
.uiselect-ico { width: 15px; height: 15px; object-fit: contain; border-radius: 3px; flex-shrink: 0; }
.uiselect-arrow { flex-shrink: 0; color: var(--text-tertiary); transition: transform var(--motion-fast) var(--motion-ease); }
.uiselect-btn.open .uiselect-arrow { transform: rotate(180deg); }

.uiselect-drop {
  position: fixed; z-index: 400; overflow-y: auto; padding: 4px;
  background: var(--bg-elevated); border: 1px solid var(--border-light);
  border-radius: var(--radius-md); box-shadow: var(--shadow-modal);
  transform-origin: top center;
}
.uiselect-opt {
  display: flex; align-items: center; gap: 7px; width: 100%;
  padding: 6px 9px; border: none; border-radius: var(--radius-sm);
  background: transparent; font-size: 13.5px; color: var(--text-primary);
  cursor: pointer; text-align: left;
  transition: background var(--motion-fast) var(--motion-ease);
}
.uiselect-opt:hover:not(.disabled) { background: var(--bg-hover); }
.uiselect-opt.active {
  color: var(--color-primary); font-weight: 600;
  background: color-mix(in srgb, var(--color-primary) 11%, transparent);
}
.uiselect-opt.disabled { opacity: .45; cursor: not-allowed; }
.uiselect-check { flex-shrink: 0; margin-left: auto; }
.uiselect-empty { padding: 14px; text-align: center; font-size: 13px; color: var(--text-tertiary); }
</style>
