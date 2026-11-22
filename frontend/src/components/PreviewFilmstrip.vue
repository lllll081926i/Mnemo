<script setup>
import { computed, onMounted, onBeforeUnmount, ref, useId, watch } from 'vue'
import UiIcon from './UiIcon.vue'

const props = defineProps({ files: { type: Array, required: true }, selectedIndex: { type: Number, required: true } })
const emit = defineEmits(['select'])
const strip = ref(null)
const scrollIndex = ref(0)
const visibleCount = ref(1)
const active = ref(props.selectedIndex)
const prefix = useId()
const stride = 72
const padding = 8
let observer
const width = computed(() => props.files.length * stride + padding)
const first = computed(() => Math.max(0, scrollIndex.value - 3))
const end = computed(() => Math.min(props.files.length, scrollIndex.value + visibleCount.value + 4))
const visible = computed(() => props.files.slice(first.value, end.value))
const optionId = index => `${prefix}-${index}`
const activeId = computed(() => active.value >= first.value && active.value < end.value ? optionId(active.value) : undefined)

function updateViewport() {
  if (!strip.value) return
  scrollIndex.value = Math.floor(Math.max(0, strip.value.scrollLeft) / stride)
  visibleCount.value = Math.max(1, Math.ceil(strip.value.clientWidth / stride))
}

function reveal(index, center = false) {
  const element = strip.value
  if (!element || index < 0 || index >= props.files.length) return
  active.value = index
  const left = index * stride
  if (center) element.scrollLeft = left - (element.clientWidth - stride) / 2
  else if (left < element.scrollLeft) element.scrollLeft = left
  else if (left + stride + padding > element.scrollLeft + element.clientWidth) element.scrollLeft = left + stride + padding - element.clientWidth
  updateViewport()
}

function choose(index) {
  strip.value?.focus({ preventScroll: true })
  reveal(index)
  emit('select', props.files[index])
}

function onKey(event) {
  let index = activeId.value ? active.value : scrollIndex.value
  if (event.key === 'ArrowLeft') index--
  else if (event.key === 'ArrowRight') index++
  else if (event.key === 'Home') index = 0
  else if (event.key === 'End') index = props.files.length - 1
  else if (event.key === 'Enter' || event.key === ' ') {
    event.preventDefault()
    event.stopPropagation()
    if (!event.repeat) choose(index)
    return
  } else return
  event.preventDefault()
  event.stopPropagation()
  reveal(Math.min(props.files.length - 1, Math.max(0, index)))
}

function onWheel(event) {
  if (!strip.value || Math.abs(event.deltaX) >= Math.abs(event.deltaY)) return
  event.preventDefault()
  const scale = event.deltaMode === 1 ? stride : event.deltaMode === 2 ? strip.value.clientWidth : 1
  strip.value.scrollLeft += event.deltaY * scale
  updateViewport()
}

onMounted(() => {
  reveal(props.selectedIndex, true)
  if (typeof ResizeObserver !== 'undefined') {
    observer = new ResizeObserver(updateViewport)
    observer.observe(strip.value)
  }
})
onBeforeUnmount(() => observer?.disconnect())
watch(() => props.selectedIndex, index => reveal(index), { flush: 'post' })
</script>

<template>
  <div ref="strip" class="pv-filmstrip" tabindex="0" role="listbox" aria-label="图片缩略图" aria-orientation="horizontal" :aria-activedescendant="activeId" @scroll.passive="updateViewport" @wheel.stop="onWheel" @keydown="onKey" @pointerdown.stop @dblclick.stop>
    <div class="pv-film-track" :style="{ width: `${width}px` }">
      <div v-for="(file, offset) in visible" :id="optionId(first + offset)" :key="file.file_id" class="pv-film-thumb" :class="{ 'is-focused': active === first + offset }" :style="{ left: `${(first + offset) * stride + padding}px` }" role="option" :aria-label="file.name" :aria-selected="selectedIndex === first + offset" :aria-posinset="first + offset + 1" :aria-setsize="files.length" :title="file.name" @click="choose(first + offset)">
        <img v-if="file.thumbnail" :src="file.thumbnail" alt="" draggable="false" loading="lazy" />
        <UiIcon v-else name="image" :size="16" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.pv-film-track { position:relative; flex:none; height:64px; }
.pv-film-thumb { position:absolute; top:8px; box-sizing:border-box; width:64px; height:48px; border-radius:7px; overflow:hidden; cursor:pointer; border:1.5px solid transparent; display:grid; place-items:center; background:var(--pv-media-surface); color:var(--pv-media-muted); transition:transform .18s var(--motion-spring), border-color .15s ease; }
.pv-film-thumb:hover { transform:translateY(-2px); }
.pv-film-thumb[aria-selected=true] { border-color:var(--pv-media-active); }
.pv-filmstrip:focus-visible .pv-film-thumb.is-focused { outline:1px solid currentColor; outline-offset:2px; }
.pv-film-thumb img { width:100%; height:100%; object-fit:cover; }
</style>
