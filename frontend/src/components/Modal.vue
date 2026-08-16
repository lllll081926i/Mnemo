<script setup>
import { onMounted, onBeforeUnmount } from 'vue'
import UiIcon from './UiIcon.vue'

const props = defineProps({
  title: { type: String, default: '' },
  width: { type: String, default: '' },
  bodyClass: { type: String, default: 'modal-body' },
  dialogClass: { type: String, default: '' },
  dialogStyle: { type: Object, default: () => ({}) },
})
const emit = defineEmits(['close'])

function onKey(e) { if (e.key === 'Escape') emit('close') }
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <teleport to="body">
    <transition name="modal-fade">
      <div class="modal-mask" @click.self="emit('close')">
        <div
          class="modal"
          :class="dialogClass"
          :style="{ ...(width ? { width } : {}), ...dialogStyle }"
        >
          <div v-if="title || $slots.head" class="modal-head">
            <slot name="head">
              <h3>{{ title }}</h3>
            </slot>
            <div style="display:flex;align-items:center;gap:4px">
              <slot name="head-extra" />
              <button class="icon-btn" style="width:28px;height:28px" title="关闭 (Esc)" @click="emit('close')">
                <UiIcon name="close" :size="14" />
              </button>
            </div>
          </div>
          <div :class="bodyClass">
            <slot />
          </div>
          <div v-if="$slots.actions" class="modal-actions">
            <slot name="actions" />
          </div>
        </div>
      </div>
    </transition>
  </teleport>
</template>
