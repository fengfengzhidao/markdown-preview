<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

const props = defineProps<{ image: { src: string; alt: string } | undefined }>()
const emit = defineEmits<{ close: [] }>()
const dialog = ref<HTMLDialogElement>()
const viewport = ref<HTMLElement>()
const natural = ref({ width: 0, height: 0 })
const zoom = ref(1)
const fitted = ref(true)
const dragging = ref(false)
const dimensions = computed(() => ({ width: `${natural.value.width * zoom.value}px`, height: `${natural.value.height * zoom.value}px` }))
let dragStart = { x: 0, y: 0, left: 0, top: 0 }

function fit() {
  if (!viewport.value || !natural.value.width) return
  zoom.value = Math.min(1, (viewport.value.clientWidth - 40) / natural.value.width, (viewport.value.clientHeight - 40) / natural.value.height)
  fitted.value = true
}
function setZoom(value: number) {
  zoom.value = Math.min(5, Math.max(.1, value))
  fitted.value = false
}
function loaded(event: Event) {
  const image = event.target as HTMLImageElement
  natural.value = { width: image.naturalWidth, height: image.naturalHeight }
  fit()
}
function startDrag(event: PointerEvent) {
  if (event.button !== 0 || !viewport.value) return
  dragging.value = true
  dragStart = { x: event.clientX, y: event.clientY, left: viewport.value.scrollLeft, top: viewport.value.scrollTop }
  viewport.value.setPointerCapture(event.pointerId)
}
function moveDrag(event: PointerEvent) {
  if (!dragging.value || !viewport.value) return
  viewport.value.scrollLeft = dragStart.left - event.clientX + dragStart.x
  viewport.value.scrollTop = dragStart.top - event.clientY + dragStart.y
}
function onKeydown(event: KeyboardEvent) {
  if (event.key === '+' || event.key === '=') setZoom(zoom.value * 1.25)
  else if (event.key === '-') setZoom(zoom.value / 1.25)
  else if (event.key === '0') fit()
  else if (event.key === '1') setZoom(1)
  else return
  event.preventDefault()
}
function resize() { if (fitted.value) fit() }
watch(() => props.image, async image => {
  if (!image) { dialog.value?.close(); return }
  natural.value = { width: 0, height: 0 }
  zoom.value = 1
  fitted.value = true
  await nextTick()
  dialog.value?.showModal()
})
onMounted(() => window.addEventListener('resize', resize))
onBeforeUnmount(() => window.removeEventListener('resize', resize))
</script>

<template>
  <dialog ref="dialog" class="image-viewer" aria-label="图片预览" @cancel.prevent="emit('close')" @keydown.stop="onKeydown" @click="event => event.target === dialog && emit('close')">
    <template v-if="image">
      <header class="image-viewer-toolbar">
        <span class="image-viewer-title">{{ image.alt || '图片预览' }}</span>
        <button aria-label="缩小图片" title="缩小 · -" :disabled="zoom <= .1" @click="setZoom(zoom / 1.25)">−</button>
        <span class="image-zoom" aria-live="polite">{{ Math.round(zoom * 100) }}%</span>
        <button aria-label="放大图片" title="放大 · +" :disabled="zoom >= 5" @click="setZoom(zoom * 1.25)">+</button>
        <button title="适应窗口 · 0" @click="fit">适应</button>
        <button title="原始大小 · 1" @click="setZoom(1)">原图</button>
        <button class="image-viewer-close" aria-label="关闭图片预览" title="关闭 · Esc" @click="emit('close')">×</button>
      </header>
      <div ref="viewport" class="image-viewport" :class="{ dragging }" @wheel.prevent="setZoom(zoom * ($event.deltaY < 0 ? 1.15 : 1 / 1.15))" @pointerdown.prevent="startDrag" @pointermove="moveDrag" @pointerup="dragging = false" @lostpointercapture="dragging = false">
        <div class="image-stage"><img :src="image.src" :alt="image.alt" :style="dimensions" draggable="false" @load="loaded" /></div>
      </div>
    </template>
  </dialog>
</template>
