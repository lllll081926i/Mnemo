<script setup>
import { onMounted, onBeforeUnmount } from 'vue'
import UiIcon from './UiIcon.vue'

const props = defineProps({
  title: { type: String, default: '' },
  width: { type: String, default: '' },
  bodyClass: { type: String, default: 'modal-body' },
})
const emit = defineEmits(['close'])

function onKey(e) { if (e.key === 'Escape') emit('close') }
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <teleport to="body">
    <div class="modal-mask" @click.self="emit('close')">
      <div class="modal" :style="width ? { width } : {}">
        <div v-if="title" class="modal-head">
          <h3>{{ title }}</h3>
          <button class="icon-btn" style="width:28px;height:28px" @click="emit('close')"><UiIcon name="close" :size="14" /></button>
        </div>
        <div :class="bodyClass">
          <slot />
        </div>
        <div v-if="$slots.actions" class="modal-actions">
          <slot name="actions" />
        </div>
      </div>
    </div>
  </teleport>
</template>
