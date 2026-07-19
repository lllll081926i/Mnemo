<script lang="ts">
import { defineComponent, ref, watch } from 'vue'
import { useWinStore, WinState } from '../store'

export default defineComponent({
  props: {
    visible: {
      type: Boolean,
      required: false,
      default: true
    }
  },
  emits: ['splitSize'],
  setup(props, { emit }) {
    const defaultWidth = 220
    const leftMinWidth = 188
    const rightMinWidth = 320
    const winStore = useWinStore()
    const bodyWidth = ref(Math.max(winStore.width, 900))
    const splitMoving = ref(false)
    const expandedSize = ref(defaultWidth)
    const splitSize = ref<string | number>(`${defaultWidth}px`)
    const splitSizeMax = ref(Math.max(leftMinWidth, bodyWidth.value - rightMinWidth))

    const toPixels = (value: string | number) => Number.parseFloat(String(value)) || 0
    const clampExpandedSize = (value: number) => Math.min(Math.max(value, leftMinWidth), splitSizeMax.value)

    winStore.$subscribe((_m: any, state: WinState) => {
      const width = state.width
      if (width > 0 && bodyWidth.value != width) {
        bodyWidth.value = width
        splitSizeMax.value = Math.max(leftMinWidth, width - rightMinWidth)
        expandedSize.value = clampExpandedSize(expandedSize.value)
        if (props.visible) splitSize.value = `${expandedSize.value}px`
      }
    })

    watch(splitSize, (value) => {
      if (!props.visible) return
      const pixels = toPixels(value)
      if (pixels >= leftMinWidth) expandedSize.value = clampExpandedSize(pixels)
    })

    watch(
      () => props.visible,
      (visible) => {
        splitSize.value = visible ? `${clampExpandedSize(expandedSize.value)}px` : '0px'
      },
      { immediate: true }
    )

    const handleMoveEnd = () => {
      splitMoving.value = false
      emit('splitSize', expandedSize.value)
    }

    return { splitSize, expandedSize, leftMinWidth, splitSizeMax, splitMoving, handleMoveEnd }
  }
})
</script>

<template>
  <a-split
    v-model:size="splitSize"
    class="MySplit"
    :class="{ 'is-collapsed': !visible, 'is-resizing': splitMoving }"
    :disabled="!visible"
    :min="visible ? leftMinWidth : 0"
    :max="splitSizeMax"
    tabindex="-1"
    @move-start="splitMoving = true"
    @move-end="handleMoveEnd">
    <template #first>
      <slot name="first">first</slot>
    </template>
    <template #resize-trigger>
      <div class="splitline" :class="{ resize: splitMoving }" draggable="false"></div>
    </template>
    <template #second>
      <slot name="second">second</slot>
    </template>
  </a-split>
</template>
