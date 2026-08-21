<script setup>
// 图片/SVG 自定义裁剪弹窗：默认使用整张图片，支持平移、缩放与正方形视口裁剪
import { ref, computed, onMounted, onBeforeUnmount, nextTick } from 'vue'
import Modal from './Modal.vue'
import UiIcon from './UiIcon.vue'

const props = defineProps({
  src: { type: String, required: true },
  isSvg: { type: Boolean, default: false },
})
const emit = defineEmits(['confirm', 'cancel'])

const imgEl = ref(null)
const stageEl = ref(null)
const naturalW = ref(0)
const naturalH = ref(0)
const baseScale = ref(1) // 适应视口的基准缩放（使整张图片默认完整包含在裁剪框内）
const userZoom = ref(1)   // 用户缩放倍率 (0.5x - 4x)
const pos = ref({ x: 0, y: 0 })
const isDragging = ref(false)
let dragStart = { x: 0, y: 0, posX: 0, posY: 0 }

const CROP_SIZE = 180 // 视口像素大小

const currentScale = computed(() => baseScale.value * userZoom.value)

function resetToFit() {
  userZoom.value = 1
  pos.value = { x: 0, y: 0 }
}

function onImageLoad(e) {
  const img = e.target
  naturalW.value = img.naturalWidth || CROP_SIZE
  naturalH.value = img.naturalHeight || CROP_SIZE
  // 默认将整张图片完整置于 180x180 裁剪视口内
  const maxDim = Math.max(naturalW.value, naturalH.value)
  baseScale.value = maxDim > 0 ? CROP_SIZE / maxDim : 1
  resetToFit()
}

function onWheel(e) {
  e.preventDefault()
  const factor = e.deltaY < 0 ? 1.12 : 1 / 1.12
  const nextZoom = Math.min(4, Math.max(0.4, Number((userZoom.value * factor).toFixed(3))))
  userZoom.value = nextZoom
}

function onPointerDown(e) {
  if (e.button !== 0) return
  isDragging.value = true
  dragStart = { x: e.clientX, y: e.clientY, posX: pos.value.x, posY: pos.value.y }
  const onMove = (ev) => {
    if (!isDragging.value) return
    pos.value = {
      x: dragStart.posX + (ev.clientX - dragStart.x),
      y: dragStart.posY + (ev.clientY - dragStart.y),
    }
  }
  const onUp = () => {
    isDragging.value = false
    window.removeEventListener('pointermove', onMove)
    window.removeEventListener('pointerup', onUp)
  }
  window.addEventListener('pointermove', onMove)
  window.addEventListener('pointerup', onUp)
}

function cropAndConfirm() {
  // 如果是 SVG 且未做任何平移缩放，直接返回原矢量 Data URL 保留无限精度
  if (props.isSvg && userZoom.value === 1 && pos.value.x === 0 && pos.value.y === 0) {
    emit('confirm', props.src)
    return
  }

  const canvas = document.createElement('canvas')
  const OUT_SIZE = 128
  canvas.width = OUT_SIZE
  canvas.height = OUT_SIZE
  const ctx = canvas.getContext('2d')
  if (!ctx || !imgEl.value) {
    emit('confirm', props.src)
    return
  }

  ctx.imageSmoothingEnabled = true
  ctx.imageSmoothingQuality = 'high'

  // 将裁剪视口内的变换映射到 128x128 Canvas
  const outRatio = OUT_SIZE / CROP_SIZE
  ctx.save()
  ctx.translate(OUT_SIZE / 2 + pos.value.x * outRatio, OUT_SIZE / 2 + pos.value.y * outRatio)
  const drawW = naturalW.value * currentScale.value * outRatio
  const drawH = naturalH.value * currentScale.value * outRatio
  ctx.drawImage(imgEl.value, -drawW / 2, -drawH / 2, drawW, drawH)
  ctx.restore()

  const croppedDataUrl = canvas.toDataURL('image/png')
  emit('confirm', croppedDataUrl)
}
</script>

<template>
  <Modal title="调整与裁剪图标" width="360px" @close="emit('cancel')">
    <div class="crop-container">
      <div
        ref="stageEl"
        class="crop-stage"
        :class="{ grabbing: isDragging }"
        @wheel="onWheel"
        @pointerdown="onPointerDown"
      >
        <!-- 原始图片变换层 -->
        <div
          class="crop-img-layer"
          :style="{
            transform: `translate(${pos.x}px, ${pos.y}px) scale(${currentScale})`,
            width: naturalW ? naturalW + 'px' : 'auto',
            height: naturalH ? naturalH + 'px' : 'auto',
          }"
        >
          <img
            ref="imgEl"
            :src="src"
            alt="待裁剪图片"
            draggable="false"
            @load="onImageLoad"
          />
        </div>

        <!-- 裁剪遮罩层：高亮居中 180x180 裁剪视口 -->
        <div class="crop-mask">
          <div class="crop-hole"></div>
        </div>
      </div>

      <!-- 调节控制条 -->
      <div class="crop-controls">
        <button class="icon-btn sm" title="缩小" :disabled="userZoom <= 0.4" @click="userZoom = Math.max(0.4, Number((userZoom / 1.2).toFixed(2)))">
          <UiIcon name="minimize" :size="13" />
        </button>
        <input
          type="range"
          min="0.4"
          max="4"
          step="0.05"
          v-model.number="userZoom"
          class="crop-slider"
        />
        <button class="icon-btn sm" title="放大" :disabled="userZoom >= 4" @click="userZoom = Math.min(4, Number((userZoom * 1.2).toFixed(2)))">
          <UiIcon name="plus" :size="13" />
        </button>
        <button class="btn sm" title="还原为适应全图" @click="resetToFit">适应全图</button>
      </div>
    </div>

    <template #actions>
      <button class="btn" type="button" @click="emit('cancel')">取消</button>
      <button class="btn primary" type="button" @click="cropAndConfirm">
        <UiIcon name="check" :size="13" />
        <span>应用图标</span>
      </button>
    </template>
  </Modal>
</template>

<style scoped>
.crop-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 14px;
}
.crop-stage {
  position: relative;
  width: 240px;
  height: 240px;
  background: #08080a;
  border-radius: var(--radius-md);
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
  user-select: none;
  touch-action: none;
  cursor: grab;
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.08);
}
.crop-stage.grabbing { cursor: grabbing; }
.crop-img-layer {
  position: absolute;
  display: flex;
  align-items: center;
  justify-content: center;
  transform-origin: center center;
  pointer-events: none;
}
.crop-img-layer img {
  display: block;
  max-width: none;
  max-height: none;
  object-fit: contain;
}
.crop-mask {
  position: absolute;
  inset: 0;
  pointer-events: none;
  display: flex;
  align-items: center;
  justify-content: center;
}
/* 裁剪视口高亮盒：周边暗化 + 虚线高光框 */
.crop-hole {
  width: 180px;
  height: 180px;
  border-radius: 12px;
  box-shadow: 0 0 0 9999px rgba(0, 0, 0, 0.58), inset 0 0 0 1.5px rgba(255, 255, 255, 0.85);
}
.crop-controls {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 0 4px;
}
.crop-slider {
  flex: 1;
  height: 4px;
  accent-color: var(--color-primary);
  cursor: pointer;
}
</style>
