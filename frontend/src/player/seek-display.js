import { computed, ref } from 'vue'

export function useSeekDisplay(position) {
  const preview = ref(null)
  const pending = ref(null)
  const display = computed(() => preview.value ?? pending.value ?? position.value)
  let timeout = null

  function clearPending() {
    if (timeout) clearTimeout(timeout)
    timeout = null
    pending.value = null
  }

  function reset() {
    preview.value = null
    clearPending()
  }

  function showPreview(value) {
    if (Number.isFinite(value)) preview.value = value
  }

  function commit(value) {
    if (!Number.isFinite(value)) return
    preview.value = null
    clearPending()
    pending.value = value
    timeout = setTimeout(clearPending, 15000)
  }

  function sync(value) {
    if (!Number.isFinite(value)) return
    position.value = value
    if (pending.value !== null && Math.abs(value - pending.value) <= 0.75) clearPending()
  }

  return { display, showPreview, commit, sync, reset }
}
