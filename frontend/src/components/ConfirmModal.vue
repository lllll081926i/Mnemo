<script setup>
import Modal from './Modal.vue'
import UiIcon from './UiIcon.vue'

const props = defineProps({
  title: { type: String, default: '确认操作' },
  message: { type: String, required: true },
  okText: { type: String, default: '确定' },
  cancelText: { type: String, default: '取消' },
  danger: { type: Boolean, default: false },
})
const emit = defineEmits(['ok', 'cancel'])

const ok = () => emit('ok')
const cancel = () => emit('cancel')
</script>

<template>
  <Modal :title="title" width="400px" @close="cancel">
    <div class="confirm-body">
      <div class="confirm-icon-wrap" :class="{ danger }">
        <UiIcon :name="danger ? 'warning' : 'info'" :size="20" />
      </div>
      <div class="confirm-message">{{ message }}</div>
    </div>
    <template #actions>
      <button class="btn" @click="cancel">{{ cancelText }}</button>
      <button class="btn primary" :class="{ danger }" @click="ok">{{ okText }}</button>
    </template>
  </Modal>
</template>

<style scoped>
.confirm-body {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 8px 0 4px;
}
.confirm-icon-wrap {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  background: color-mix(in srgb, var(--color-primary) 12%, transparent);
  color: var(--color-primary);
  flex-shrink: 0;
}
.confirm-icon-wrap.danger {
  background: color-mix(in srgb, var(--color-error) 12%, transparent);
  color: var(--color-error);
}
.confirm-message {
  flex: 1;
  min-width: 0;
  line-height: 1.6;
  font-size: 14px;
  color: var(--text-primary);
  word-break: break-word;
  padding-top: 6px;
}
</style>
