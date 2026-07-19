<script lang="ts">
import { computed, defineComponent, type CSSProperties, type PropType } from 'vue'

export default defineComponent({
  props: { tabs: Array as PropType<{ key: string; title: string; alt: string }[]>, name: String, value: String },
  emits: ['update:value'],
  setup(props, ctx) {
    const activeIndex = computed(() => {
      const index = (props.tabs || []).findIndex((item) => item.key === props.value)
      return index >= 0 ? index : 0
    })

    const rootStyle = computed(
      () =>
        ({
          '--segment-count': Math.max(props.tabs?.length || 0, 1),
          '--segment-index': activeIndex.value
        }) as CSSProperties
    )

    const click = (val: string) => ctx.emit('update:value', val)
    return { rootStyle, click }
  }
})
</script>

<template>
  <div class="mantine-root" role="tablist" :style="rootStyle">
    <span class="mantine-SegmentedControl-active mantine-bgblock" aria-hidden="true"></span>
    <button v-for="item in tabs" :key="name + '-' + item.key" type="button" role="tab" :aria-selected="value == item.key" :class="'mantine-item' + (value == item.key ? ' mantine-item-active' : '')" :title="item.alt" @click="click(item.key)">
      <span :id="'mantine-' + name + '-label-' + item.key" :class="'mantine-label' + (value == item.key ? ' mantine-label-active' : '')">{{ item.title }}</span>
    </button>
  </div>
</template>

<style>
.mantine-root {
  --segment-count: 1;
  --segment-index: 0;
  position: relative;
  display: inline-flex;
  flex-direction: row;
  box-sizing: border-box;
  width: 100%;
  background-color: var(--color-fill-2);
  border-radius: 4px;
  overflow: hidden;
  padding: 3px;
  z-index: 2;
}
.mantine-item {
  position: relative;
  box-sizing: border-box;
  flex: 1 1 0%;
  min-width: 0;
  padding: 0;
  z-index: 2;
  color: inherit;
  font: inherit;
  background: transparent;
  border: 0;
  border-radius: 4px;
  cursor: pointer;
}
.mantine-item:not(.mantine-item-active):hover {
  background-color: var(--color-fill-3);
}

.mantine-label {
  border-radius: 4px;
  font-weight: 500;
  font-size: 14px;
  cursor: pointer;
  display: block;
  text-align: center;
  padding: 5px 10px;
  overflow: hidden;
  white-space: nowrap;
  word-break: keep-all;
  text-overflow: ellipsis;
  user-select: none;
  color: var(--color-text-2);
  transition: color 200ms ease 0s;
  line-height: 16px;
}

.mantine-label-active,
.mantine-label-active:hover {
  color: rgb(0, 0, 0);
}

body[arco-theme='dark'] .mantine-label-active,
body[arco-theme='dark'] .mantine-label-active:hover {
  color: rgb(255, 255, 255);
}

.mantine-bgblock {
  box-sizing: border-box;
  border-radius: 4px;
  position: absolute;
  top: 3px;
  bottom: 3px;
  left: 3px;
  z-index: 1;
  box-shadow:
    rgb(0 0 0 / 5%) 0px 1px 3px,
    rgb(0 0 0 / 10%) 0px 1px 2px;
  width: calc((100% - 6px) / var(--segment-count));
  height: auto;
  transform: translateX(calc(var(--segment-index) * 100%));
  transition:
    transform 180ms cubic-bezier(0.22, 1, 0.36, 1),
    width 180ms cubic-bezier(0.22, 1, 0.36, 1);
  background-color: rgb(255, 255, 255);
}

body[arco-theme='dark'] .mantine-bgblock {
  box-shadow:
    rgba(0, 0, 0, 0.25) 0px 1px 3px,
    rgba(0, 0, 0, 0.5) 0px 1px 2px;
  background-color: rgb(var(--primary-6));
}
</style>
