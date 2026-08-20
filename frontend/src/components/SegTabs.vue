<script setup>
import { computed } from 'vue'

const props = defineProps({
  options: { type: Array, required: true }, // [{ key, label }]
  modelValue: { type: String, required: true },
})
const emit = defineEmits(['update:modelValue'])

const index = computed(() => Math.max(0, props.options.findIndex((o) => o.key === props.modelValue)))

function selectAt(i) {
  const option = props.options[i]
  if (option) emit('update:modelValue', option.key)
}

function onKeydown(e, i) {
  const len = props.options.length
  if (!len) return
  let next = -1
  if (e.key === 'ArrowRight' || e.key === 'ArrowDown') next = (i + 1) % len
  else if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') next = (i - 1 + len) % len
  else if (e.key === 'Home') next = 0
  else if (e.key === 'End') next = len - 1
  else if (e.key === 'Enter' || e.key === ' ') {
    e.preventDefault()
    selectAt(i)
    return
  }
  if (next >= 0) {
    e.preventDefault()
    selectAt(next)
    requestAnimationFrame(() => document.getElementById(`seg-tab-${props.options[next].key}`)?.focus())
  }
}
</script>

<template>
  <div class="seg" role="tablist" aria-label="选项" :style="{ '--seg-count': options.length, '--seg-index': index }">
    <div class="seg-slider"></div>
    <button
      v-for="o in options"
      :key="o.key"
      :id="`seg-tab-${o.key}`"
      type="button"
      role="tab"
      :aria-selected="o.key === modelValue"
      :tabindex="o.key === modelValue ? 0 : -1"
      :class="{ active: o.key === modelValue }"
      @click="selectAt(options.indexOf(o))"
      @keydown="onKeydown($event, options.indexOf(o))"
    >{{ o.label }}</button>
  </div>
</template>
