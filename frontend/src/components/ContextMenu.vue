<script setup>
// 通用右键/下拉菜单。items: [{ icon(UiIcon 名称), label, danger, disabled, sep, action }]
import { computed, nextTick, onMounted, onBeforeUnmount, ref } from 'vue'
import UiIcon from './UiIcon.vue'

const props = defineProps({
  x: { type: Number, required: true },
  y: { type: Number, required: true },
  items: { type: Array, required: true },
})
const emit = defineEmits(['close', 'select'])
const menuEl = ref(null)
const activeIndex = ref(0)

const actionableItems = computed(() => props.items
  .map((item, sourceIndex) => ({ item, sourceIndex }))
  .filter(({ item }) => !item.sep && !item.header && !item.disabled))

const pos = computed(() => {
  const w = 200
  const h = props.items.length * 34 + 12
  return {
    left: Math.max(8, Math.min(props.x, window.innerWidth - w - 8)) + 'px',
    top: Math.max(8, Math.min(props.y, window.innerHeight - h - 8)) + 'px',
  }
})

function pick(item) {
  if (item.disabled || item.sep) return
  emit('select', item.action !== undefined ? item.action : item.key)
  emit('close')
}

function focusActive() {
  nextTick(() => {
    const sourceIndex = actionableItems.value[activeIndex.value]?.sourceIndex
    if (sourceIndex === undefined) return
    menuEl.value?.querySelector(`[data-menu-index="${sourceIndex}"]`)?.focus()
  })
}

function onPointerDown(e) {
  // Run after the menu has had a chance to stop events from its own items.
  const target = e.target
  if (!target || !target.closest || !target.closest('.ctx-menu')) emit('close')
}
function onKey(e) {
  if (e.key === 'Escape') {
    e.preventDefault()
    emit('close')
    return
  }
  const len = actionableItems.value.length
  if (!len) return
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    e.preventDefault()
    activeIndex.value = e.key === 'ArrowDown'
      ? (activeIndex.value + 1) % len
      : (activeIndex.value - 1 + len) % len
    focusActive()
  } else if (e.key === 'Home' || e.key === 'End') {
    e.preventDefault()
    activeIndex.value = e.key === 'Home' ? 0 : len - 1
    focusActive()
  } else if (e.key === 'Enter' || e.key === ' ') {
    e.preventDefault()
    const entry = actionableItems.value[activeIndex.value]
    if (entry) pick(entry.item)
  }
}

function onBlur() { emit('close') }

onMounted(() => {
  window.addEventListener('pointerdown', onPointerDown)
  window.addEventListener('keydown', onKey)
  window.addEventListener('blur', onBlur)
  focusActive()
})
onBeforeUnmount(() => {
  window.removeEventListener('pointerdown', onPointerDown)
  window.removeEventListener('keydown', onKey)
  window.removeEventListener('blur', onBlur)
})
</script>

<template>
  <teleport to="body">
    <transition name="popover-zoom">
      <div ref="menuEl" class="ctx-menu" :style="pos" role="menu" aria-label="操作菜单" tabindex="-1" @pointerdown.stop @contextmenu.prevent.stop>
        <template v-for="(item, i) in items" :key="i">
          <div v-if="item.sep" class="ctx-sep"></div>
          <div v-else-if="item.header" class="ctx-header">{{ item.header }}</div>
          <button
            v-else
            type="button"
            class="ctx-item"
            :class="{ danger: item.danger, disabled: item.disabled }"
            :data-menu-index="i"
            role="menuitem"
            :disabled="item.disabled"
            :tabindex="item.disabled ? -1 : (actionableItems.findIndex((entry) => entry.sourceIndex === i) === activeIndex ? 0 : -1)"
            @click="pick(item)"
          >
            <span class="ci"><UiIcon v-if="item.icon" :name="item.icon" :size="15" /></span>
            <span>{{ item.label }}</span>
          </button>
        </template>
      </div>
    </transition>
  </teleport>
</template>
