<script setup>
import { computed } from 'vue'

const props = defineProps({
  options: { type: Array, required: true }, // [{ key, label }]
  modelValue: { type: String, required: true },
})
const emit = defineEmits(['update:modelValue'])

const index = computed(() => Math.max(0, props.options.findIndex((o) => o.key === props.modelValue)))
</script>

<template>
  <div class="seg" :style="{ '--seg-count': options.length, '--seg-index': index }">
    <div class="seg-slider"></div>
    <button
      v-for="o in options"
      :key="o.key"
      :class="{ active: o.key === modelValue }"
      @click="emit('update:modelValue', o.key)"
    >{{ o.label }}</button>
  </div>
</template>
