<script setup>
import { computed, onBeforeUnmount, onMounted, ref, useId, watch } from 'vue'

const props = defineProps({ files: { type: Array, required: true }, selectedIndex: { type: Number, required: true } })
const emit = defineEmits(['select', 'dismiss'])
const list = ref(null)
const active = ref(Math.max(0, props.selectedIndex))
const firstVisible = ref(0)
const visibleCount = ref(1)
const prefix = useId()
const rowHeight = 36
const overscan = 3
let observer

const start = computed(() => Math.max(0, firstVisible.value - overscan))
const end = computed(() => Math.min(props.files.length, firstVisible.value + visibleCount.value + overscan + 1))
const rows = computed(() => props.files.slice(start.value, end.value))
const activeId = computed(() => active.value >= start.value && active.value < end.value ? `${prefix}-${active.value}` : undefined)

function updateViewport() {
  const element = list.value
  if (!element) return
  firstVisible.value = Math.floor(Math.max(0, element.scrollTop) / rowHeight)
  visibleCount.value = Math.max(1, Math.ceil(element.clientHeight / rowHeight))
}

function reveal(index, center = false) {
  const element = list.value
  if (!element || !props.files.length) return
  active.value = Math.max(0, Math.min(props.files.length - 1, index))
  const top = active.value * rowHeight
  if (center) element.scrollTop = top - (element.clientHeight - rowHeight) / 2
  else if (top < element.scrollTop) element.scrollTop = top
  else if (top + rowHeight > element.scrollTop + element.clientHeight) element.scrollTop = top + rowHeight - element.clientHeight
  updateViewport()
}

function choose(index) {
  const file = props.files[index]
  if (file) emit('select', file)
}

function onKey(event) {
  if (event.isComposing || event.ctrlKey || event.altKey || event.metaKey) return
  const index = activeId.value ? active.value : firstVisible.value
  const steps = { ArrowUp: -1, ArrowDown: 1, PageUp: -visibleCount.value, PageDown: visibleCount.value }
  if (!Object.hasOwn(steps, event.key) && !['Home', 'End', 'Enter', ' ', 'Escape'].includes(event.key)) return
  event.preventDefault()
  event.stopPropagation()
  if (event.key === 'Escape') emit('dismiss')
  else if (event.key === 'Enter' || event.key === ' ') { if (!event.repeat) choose(index) }
  else if (event.key === 'Home') reveal(0)
  else if (event.key === 'End') reveal(props.files.length - 1)
  else reveal(index + steps[event.key])
}

onMounted(() => {
  reveal(props.selectedIndex, true)
  list.value?.focus({ preventScroll: true })
  if (typeof ResizeObserver !== 'undefined') {
    observer = new ResizeObserver(updateViewport)
    observer.observe(list.value)
  }
})
watch(() => props.selectedIndex, index => reveal(index, true), { flush: 'post' })
watch(() => props.files.length, () => reveal(active.value), { flush: 'post' })
onBeforeUnmount(() => observer?.disconnect())
</script>

<template>
  <div ref="list" class="media-playlist" role="listbox" aria-label="播放列表" tabindex="0" :aria-activedescendant="activeId" @keydown="onKey" @scroll.passive="updateViewport">
    <div class="media-playlist-track" :style="{ height: `${files.length * rowHeight}px` }">
      <div v-for="(file, offset) in rows" :id="`${prefix}-${start + offset}`" :key="file.file_id" class="media-playlist-row" :class="{ 'keyboard-active': start + offset === active }" :style="{ top: `${(start + offset) * rowHeight}px`, height: `${rowHeight}px` }" role="option" :aria-label="file.name" :aria-selected="start + offset === selectedIndex" :aria-posinset="start + offset + 1" :aria-setsize="files.length" :title="file.name" @click="choose(start + offset)">
        <span class="media-playlist-number">{{ start + offset + 1 }}</span>
        <span class="media-playlist-name">{{ file.name }}</span>
        <slot v-if="start + offset === selectedIndex" name="selected" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.media-playlist { max-height: min(36vh, 288px); min-height: 0; overflow: auto; overflow-anchor: none; overscroll-behavior: contain; scrollbar-width: thin; }
.media-playlist-track { position: relative; }
.media-playlist-row { box-sizing: border-box; position: absolute; inset-inline: 0; display: flex; align-items: center; gap: 8px; padding: 0 10px; border-radius: var(--radius-sm); color: var(--text-secondary); font-size: 12px; cursor: pointer; }
.media-playlist-row:hover { background: var(--bg-hover); color: var(--text-primary); }
.media-playlist-row[aria-selected=true] { background: var(--listselectbg); color: var(--color-primary); }
.media-playlist:focus-visible { outline: none; }
.media-playlist:focus-visible .keyboard-active { outline: 1px solid var(--border-focus); outline-offset: -2px; }
.media-playlist-number { min-width: 24px; color: var(--text-tertiary); text-align: right; font-size: 11px; font-variant-numeric: tabular-nums; }
.media-playlist-name { flex: 1; min-width: 0; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
</style>
