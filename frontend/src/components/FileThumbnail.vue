<script setup>
import { computed, ref } from 'vue'
import UiIcon from './UiIcon.vue'

const props = defineProps({ source: String, icon: String, size: Number, failures: Set })
const emit = defineEmits(['failed'])
const loadedSource = ref('')
const candidate = computed(() => props.source && !props.failures?.has(props.source) ? props.source : '')
const ready = computed(() => !!candidate.value && loadedSource.value === candidate.value)

function onLoad(event) {
  if (event.currentTarget.getAttribute('src') === candidate.value) loadedSource.value = candidate.value
}
function onError(event) {
  const failedSource = event.currentTarget.getAttribute('src')
  if (failedSource !== candidate.value) return
  loadedSource.value = ''
  emit('failed', failedSource)
}
</script>

<template>
  <span class="file-thumbnail" :class="'ft-' + icon" aria-hidden="true">
    <UiIcon class="file-thumbnail-placeholder" :class="{ ready }" :name="icon" :size="size" />
    <img v-if="candidate" :key="candidate" :src="candidate" :class="{ ready }" loading="lazy" decoding="async" alt="" @load="onLoad" @error="onError" />
  </span>
</template>

<style scoped>
.file-thumbnail { width: 100%; height: 100%; display: grid; grid-template: minmax(0, 1fr) / minmax(0, 1fr); place-items: center; }
.file-thumbnail > * { grid-area: 1 / 1; }
.file-thumbnail img { opacity: 0; }
.file-thumbnail img.ready { opacity: 1; }
.file-thumbnail-placeholder.ready { opacity: 0; }
@media (prefers-reduced-motion: no-preference) {
  .file-thumbnail :is(img, .file-thumbnail-placeholder) { transition: opacity var(--motion-fast) var(--motion-ease); }
}
</style>
