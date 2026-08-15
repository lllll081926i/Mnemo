<script setup>
// 通用右键/下拉菜单。items: [{ icon(UiIcon 名称), label, danger, disabled, sep, action }]
import { computed, onMounted, onBeforeUnmount } from 'vue'
import UiIcon from './UiIcon.vue'

const props = defineProps({
  x: { type: Number, required: true },
  y: { type: Number, required: true },
  items: { type: Array, required: true },
})
const emit = defineEmits(['close', 'select'])

const pos = computed(() => {
  const w = 200
  const h = props.items.length * 34 + 12
  return {
    left: Math.min(props.x, window.innerWidth - w - 8) + 'px',
    top: Math.min(props.y, window.innerHeight - h - 8) + 'px',
  }
})

function pick(item) {
  if (item.disabled || item.sep) return
  emit('select', item.action !== undefined ? item.action : item.key)
  emit('close')
}

function onDown(e) {
  if (!e.target.closest || !e.target.closest('.ctx-menu')) emit('close')
}
function onKey(e) { if (e.key === 'Escape') emit('close') }

function onBlur() { emit('close') }

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
  <teleport to="body">
    <div class="ctx-menu" :style="pos" @mousedown.stop>
      <template v-for="(item, i) in items" :key="i">
        <div v-if="item.sep" class="ctx-sep"></div>
        <div v-else-if="item.header" class="ctx-header">{{ item.header }}</div>
        <div
          v-else
          class="ctx-item"
          :class="{ danger: item.danger, disabled: item.disabled }"
          @click="pick(item)"
        >
          <span class="ci"><UiIcon v-if="item.icon" :name="item.icon" :size="15" /></span>
          <span>{{ item.label }}</span>
        </div>
      </template>
    </div>
  </teleport>
</template>
