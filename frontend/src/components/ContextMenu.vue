<script setup>
// 通用右键/下拉菜单。items: [{ icon(UiIcon 名称), label, danger, disabled, sep, action }]
import { computed, nextTick, onMounted, onBeforeUnmount, ref, watch } from 'vue'
import UiIcon from './UiIcon.vue'

const props = defineProps({
  x: { type: Number, required: true },
  y: { type: Number, required: true },
  items: { type: Array, required: true },
  trigger: { type: Object, default: null },
  startLast: Boolean,
})
const emit = defineEmits(['close', 'select'])
const menuEl = ref(null)
const activeIndex = ref(0)
const size = ref({ width: 0, height: 0 })
const viewport = ref({ width: window.innerWidth, height: window.innerHeight, zoom: 1 })
const anchor = ref(null)
let opener = null
const openingScroll = new Map()
let resizeObserver
let restoreFocus = false
let disposed = false

const actionableItems = computed(() => props.items
  .map((item, sourceIndex) => ({ item, sourceIndex }))
  .filter(({ item }) => !item.sep && !item.header && !item.disabled)
  .map(entry => ({ ...entry, identity: entry.item.action ?? entry.item.key ?? entry.item.label })))

const pos = computed(() => {
  const { width: w, height: h } = size.value
  const { width, height, zoom } = viewport.value
  const limits = {
    '--context-menu-max-width': `${Math.max(0, width - 16)}px`,
    maxHeight: `${Math.max(0, height - 16)}px`,
  }
  const bounds = anchor.value
  if (bounds) {
    const below = height - bounds.bottom - 12
    const above = bounds.top - 12
    const upward = h > below && above > below
    const left = Math.max(8, Math.min(bounds.left, width - w - 8))
    return {
      ...limits,
      left: `${left}px`,
      top: `${Math.max(8, Math.min(upward ? bounds.top - h - 4 : bounds.bottom + 4, height - h - 8))}px`,
      transformOrigin: `${Math.max(0, Math.min(w, bounds.left + bounds.width / 2 - left))}px ${upward ? 'bottom' : 'top'}`,
      '--dropdown-offset': upward ? '4px' : '-4px',
    }
  }
  return {
    ...limits,
    left: Math.max(8, Math.min(props.x / zoom, width - w - 8)) + 'px',
    top: Math.max(8, Math.min(props.y / zoom, height - h - 8)) + 'px',
  }
})

function pick(item) {
  if (item.disabled || item.sep || item.header) return
  restoreFocus = true
  emit('select', item.action !== undefined ? item.action : item.key)
  emit('close')
}

function measure() {
  if (disposed || !menuEl.value) return
  const zoom = menuEl.value.currentCSSZoom || 1
  const width = window.innerWidth / zoom, height = window.innerHeight / zoom
  if (viewport.value.width !== width || viewport.value.height !== height || viewport.value.zoom !== zoom) viewport.value = { width, height, zoom }
  if (props.trigger?.isConnected) {
    const bounds = props.trigger.getBoundingClientRect()
    anchor.value = { left: bounds.left / zoom, top: bounds.top / zoom, bottom: bounds.bottom / zoom, width: bounds.width / zoom }
  }
  // offset dimensions avoid the opening transition's scale transform.
  const rect = menuEl.value.getBoundingClientRect()
  const menuWidth = menuEl.value.offsetWidth || rect.width / zoom
  const menuHeight = menuEl.value.offsetHeight || rect.height / zoom
  if (size.value.width !== menuWidth || size.value.height !== menuHeight) size.value = { width: menuWidth, height: menuHeight }
}

function focusActive() {
  nextTick(() => {
    const menu = menuEl.value
    if (disposed || !menu) return
    const sourceIndex = actionableItems.value[activeIndex.value]?.sourceIndex
    const item = sourceIndex === undefined ? menu : menu.querySelector(`[data-menu-index="${sourceIndex}"]`)
    item?.focus({ preventScroll: true })
    if (!item || item === menu || menu.scrollHeight <= menu.clientHeight) return
    const top = item.offsetTop
    const bottom = top + item.offsetHeight
    if (top < menu.scrollTop) menu.scrollTop = top
    else if (bottom > menu.scrollTop + menu.clientHeight) menu.scrollTop = bottom - menu.clientHeight
  })
}

function onPointerDown(e) {
  if (!menuEl.value?.contains(e.target) && !props.trigger?.contains(e.target)) emit('close')
}
function activate(sourceIndex) {
  const index = actionableItems.value.findIndex(entry => entry.sourceIndex === sourceIndex)
  if (index >= 0) activeIndex.value = index
}
function onPointerMove(event) {
  if (event.currentTarget.disabled || event.currentTarget === document.activeElement) return
  event.currentTarget.focus({ preventScroll: true })
}
function onKey(e) {
  e.stopPropagation()
  if (e.isComposing) return
  if (e.key === 'Tab') {
    emit('close')
    return
  }
  if (e.key === 'Escape') {
    e.preventDefault()
    restoreFocus = true
    emit('close')
    return
  }
  const len = actionableItems.value.length
  if (!len) return
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    e.preventDefault()
    activeIndex.value = e.key === 'ArrowDown'
      ? (activeIndex.value + 1) % len
      : (activeIndex.value - 1 + len) % len
    focusActive()
  } else if (e.key === 'Home' || e.key === 'End') {
    e.preventDefault()
    activeIndex.value = e.key === 'Home' ? 0 : len - 1
    focusActive()
  } else if (e.key === 'Enter' || e.key === ' ') {
    e.preventDefault()
    const entry = actionableItems.value[activeIndex.value]
    if (entry) pick(entry.item)
  }
}

function onBlur() { emit('close') }
function onScroll(event) {
  const target = event.target
  if (target instanceof Node && menuEl.value?.contains(target)) return
  const scroller = target === document ? document.scrollingElement : target
  const initial = openingScroll.get(scroller)
  if (initial && initial.top === scroller.scrollTop && initial.left === scroller.scrollLeft) return
  emit('close')
}
function onResize() { measure() }
function onFocusIn(e) {
  if (!menuEl.value?.contains(e.target) && !props.trigger?.contains(e.target)) emit('close')
}

watch([() => props.x, () => props.y, actionableItems], ([x, y, items], [previousX, previousY, previousItems]) => {
  if (x !== previousX || y !== previousY) {
    activeIndex.value = 0
  } else {
    const previous = previousItems[activeIndex.value]
    const match = previous ? items.findIndex(entry => entry.identity === previous.identity) : -1
    activeIndex.value = match >= 0 ? match : Math.max(0, Math.min(activeIndex.value, items.length - 1))
  }
  nextTick(measure)
  focusActive()
})

onMounted(() => {
  opener = document.activeElement
  for (let element = props.trigger || opener; element; element = element.parentElement) {
    openingScroll.set(element, { top: element.scrollTop, left: element.scrollLeft })
  }
  if (props.startLast) activeIndex.value = Math.max(0, actionableItems.value.length - 1)
  window.addEventListener('pointerdown', onPointerDown, true)
  window.addEventListener('blur', onBlur)
  window.addEventListener('scroll', onScroll, true)
  window.addEventListener('resize', onResize)
  window.addEventListener('focusin', onFocusIn)
  if (typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(measure)
    resizeObserver.observe(menuEl.value)
  }
  measure()
  focusActive()
})
onBeforeUnmount(() => {
  disposed = true
  window.removeEventListener('pointerdown', onPointerDown, true)
  window.removeEventListener('blur', onBlur)
  window.removeEventListener('scroll', onScroll, true)
  window.removeEventListener('resize', onResize)
  window.removeEventListener('focusin', onFocusIn)
  resizeObserver?.disconnect()
  // 焦点恢复：优先回到显式触发器（下拉按钮），否则回到打开前的焦点元素。
  // 鼠标点击菜单外空白导致焦点落在 body 时同样收回，避免键盘焦点丢失。
  const target = (props.trigger && props.trigger.isConnected) ? props.trigger : (opener?.isConnected ? opener : null)
  const activeInMenu = menuEl.value?.contains(document.activeElement)
  if (target && (restoreFocus || activeInMenu || document.activeElement === document.body)) {
    target.focus({ preventScroll: true })
  }
})
</script>

<template>
  <teleport to="body">
    <transition :name="trigger ? 'toolbar-dropdown' : 'popover-zoom'" :appear="!!trigger">
      <div ref="menuEl" class="ctx-menu" :style="pos" role="menu" aria-label="操作菜单" tabindex="-1" @pointerdown.stop @contextmenu.prevent.stop @keydown="onKey">
        <template v-for="(item, i) in items" :key="i">
          <div v-if="item.sep" class="ctx-sep"></div>
          <div v-else-if="item.header" class="ctx-header">{{ item.header }}</div>
          <button
            v-else
            type="button"
            class="ctx-item"
            :class="{ danger: item.danger, disabled: item.disabled }"
            :data-menu-index="i"
            role="menuitem"
            :disabled="item.disabled"
            :tabindex="actionableItems[activeIndex]?.sourceIndex === i ? 0 : -1"
            @click="pick(item)"
            @focus="activate(i)"
            @pointermove="onPointerMove($event)"
          >
            <span class="ci"><UiIcon v-if="item.icon" :name="item.icon" :size="15" /></span>
            <span>{{ item.label }}</span>
          </button>
        </template>
      </div>
    </transition>
  </teleport>
</template>
