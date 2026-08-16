<template>
  <Modal :title="title" @close="cancel">
    <p class="confirm-body">{{ message }}</p>
    <template #actions>
      <button class="btn" @click="cancel">{{ cancelText }}</button>
      <button class="btn btn-primary" :class="danger ? 'btn-danger' : ''" @click="ok">{{ okText }}</button>
    </template>
  </Modal>
</template>

<script setup>
import { ref } from 'vue'
import Modal from './Modal.vue'

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

<style scoped>
.confirm-body {
  margin: 8px 0 4px;
  line-height: 1.6;
  color: var(--color-text-secondary);
}
.btn-danger {
  background: var(--color-error) !important;
  border-color: var(--color-error) !important;
}
</style>
