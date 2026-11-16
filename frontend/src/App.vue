<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { Events } from '@wailsio/runtime'
import { AppService } from '../bindings/markdownlite'
import DocumentPane from './components/DocumentPane.vue'
import type { DocumentTab, OpenDocument, ViewMode } from './types'

const documents = ref<DocumentTab[]>([])
// Stable pane order preserves browser scroll positions when tabs move.
const documentPanes = computed(() => [...documents.value].sort((first, second) => first.id.localeCompare(second.id)))
const activeID = ref('')
const locked = ref(true)
const opening = ref(false)
const closing = ref(false)
const saving = reactive(new Set<string>())
const active = computed(() => documents.value.find(document => document.id === activeID.value))
const tabs = ref<HTMLElement>()
const moreActions = ref<HTMLElement>()
const menuOpen = ref(false)
const tabDrag = ref<{ id: string; pointerID: number; startX: number; startY: number; x: number; y: number; started: boolean }>()
const dropIndex = ref<number>()
const dropLine = ref({ left: 0, top: 0, height: 0 })
const draggedDocument = computed(() => documents.value.find(document => document.id === tabDrag.value?.id))
const notification = ref('')
const notificationError = ref(false)
const dialog = ref<HTMLDialogElement>()
const question = ref<{ title: string; body: string; kind: 'close' | 'conflict' }>()
let finishQuestion: ((answer: string) => void) | undefined
let noticeTimer: ReturnType<typeof setTimeout> | undefined
let dragHandle: HTMLElement | undefined
let tabScrollFrame: number | undefined
let suppressTabClick = false
const unsubscribe: Array<() => void> = []

function isDirty(document: DocumentTab) {
  return document.content !== document.savedContent
}

function notify(message: string, error = false) {
  clearTimeout(noticeTimer)
  notification.value = message
  notificationError.value = error
  if (!error) noticeTimer = setTimeout(() => { notification.value = '' }, 2400)
}

function showError(error: unknown) {
  notify(error instanceof Error ? error.message : String(error), true)
}

function selectTab(id: string) {
  activeID.value = id
  nextTick(() => tabs.value?.querySelector<HTMLElement>('[aria-selected="true"]')?.scrollIntoView({ block: 'nearest', inline: 'nearest' }))
}

function ingest(result: { documents: OpenDocument[] | null; errors: string[] | null }) {
  for (const document of result.documents ?? []) {
    if (!documents.value.some(existing => existing.id === document.id)) {
      documents.value.push({ ...document, savedContent: document.content, mode: 'split', generation: 0 })
    }
    selectTab(document.id)
  }
  if (result.errors?.length) notify(result.errors.join('\n'), true)
}

async function pickFiles() {
  if (opening.value || closing.value || question.value) return
  opening.value = true
  try { ingest(await AppService.PickFiles()) } catch (error) { showError(error) }
  finally { opening.value = false }
}

async function openFiles(paths: string[]) {
  if (closing.value || question.value) return
  try { ingest(await AppService.OpenFiles(paths)) } catch (error) { showError(error) }
}

function changeContent(id: string, content: string) {
  const document = documents.value.find(document => document.id === id)
  if (document) document.content = content
}

async function ask(title: string, body: string, kind: 'close' | 'conflict'): Promise<string> {
  if (question.value) return 'cancel'
  question.value = { title, body, kind }
  const answer = new Promise<string>(resolve => { finishQuestion = resolve })
  await nextTick()
  dialog.value?.showModal()
  return answer
}

function answerQuestion(answer: string) {
  dialog.value?.close()
  question.value = undefined
  finishQuestion?.(answer)
  finishQuestion = undefined
}

async function saveDocument(document: DocumentTab): Promise<boolean> {
  if (saving.has(document.id)) return false
  if (!isDirty(document)) return true
  saving.add(document.id)
  const content = document.content
  try {
    let result = await AppService.Save(document.id, content, document.revision, false)
    if (result.conflict) {
      const answer = await ask('磁盘上的文件已更新', `“${document.name}”已被外部修改或删除，请选择如何处理当前内容。`, 'conflict')
      if (answer === 'reload') {
        const fresh = await AppService.Reload(document.id)
        Object.assign(document, fresh, { savedContent: fresh.content, generation: document.generation + 1 })
        return true
      }
      if (answer !== 'overwrite') return false
      result = await AppService.Save(document.id, content, document.revision, true)
    }
    document.revision = result.revision
    document.savedContent = content
    notify('已保存')
    return !isDirty(document)
  } catch (error) {
    showError(error)
    return false
  } finally {
    saving.delete(document.id)
  }
}

async function confirmClose(document: DocumentTab): Promise<boolean> {
  if (!isDirty(document)) return true
  selectTab(document.id)
  const answer = await ask('保存修改？', `“${document.name}”有尚未保存的修改。`, 'close')
  if (answer === 'save') return saveDocument(document)
  return answer === 'discard'
}

async function closeTab(document: DocumentTab) {
  if (closing.value || question.value) return
  if (saving.size) { notify('正在保存，请稍候'); return }
  closing.value = true
  try {
    if (!(await confirmClose(document))) return
    const index = documents.value.indexOf(document)
    documents.value.splice(index, 1)
    if (activeID.value === document.id) selectTab(documents.value[Math.min(index, documents.value.length - 1)]?.id ?? '')
  } finally { closing.value = false }
}

async function closeApplication() {
  if (closing.value || question.value) return
  if (saving.size) { notify('正在保存，请稍候'); return }
  closing.value = true
  try {
    for (const document of documents.value) {
      if (!(await confirmClose(document))) return
    }
    await AppService.Close()
  } catch (error) { showError(error) }
  finally { closing.value = false }
}

async function copyMarkdown() {
  const document = active.value
  if (!document) return
  closeMenu(true)
  try {
    await AppService.Copy(document.content)
    notify('已复制 Markdown')
  } catch (error) { showError(error) }
}

function toggleLock() {
  closeMenu(true)
  locked.value = !locked.value
  notify(locked.value ? '编辑已锁定' : '已允许编辑')
}

function switchTab(direction: number) {
  if (!documents.value.length) return
  const index = documents.value.findIndex(document => document.id === activeID.value)
  selectTab(documents.value[(index + direction + documents.value.length) % documents.value.length].id)
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    if (tabDrag.value) {
      event.preventDefault()
      finishTabDrag()
    }
    if (menuOpen.value) {
      event.preventDefault()
      closeMenu(true)
    }
    return
  }
  if (!(event.ctrlKey || event.metaKey) || event.altKey || event.isComposing) return
  const key = event.key.toLowerCase()
  const recognized = ['o', 's', 'w', 'tab'].includes(key) || (event.shiftKey && ['l', 'c'].includes(key))
  if (!recognized) return
  event.preventDefault()
  event.stopImmediatePropagation()
  if (question.value || closing.value) return
  if (event.shiftKey && key === 'l') { toggleLock(); return }
  if (event.shiftKey && key === 'c') { void copyMarkdown(); return }
  if (key === 'o') void pickFiles()
  if (key === 's' && active.value) void saveDocument(active.value)
  if (key === 'w' && active.value) void closeTab(active.value)
  if (key === 'tab') switchTab(event.shiftKey ? -1 : 1)
}

function scrollTabs(event: WheelEvent) {
  if (!tabs.value || tabs.value.scrollWidth <= tabs.value.clientWidth) return
  event.preventDefault()
  tabs.value.scrollLeft += event.deltaY || event.deltaX
}

function closeMenu(restoreFocus = false) {
  if (!menuOpen.value) return
  menuOpen.value = false
  if (restoreFocus) nextTick(() => moreActions.value?.querySelector<HTMLButtonElement>('.more-button')?.focus())
}

function toggleMenu() {
  if (menuOpen.value) { closeMenu(true); return }
  menuOpen.value = true
  nextTick(() => moreActions.value?.querySelector<HTMLButtonElement>('.action-menu button:not(:disabled)')?.focus())
}

function onOutsidePointerDown(event: PointerEvent) {
  if (!moreActions.value?.contains(event.target as Node)) closeMenu()
}

function onMenuKeydown(event: KeyboardEvent) {
  if (event.key === 'Tab') { closeMenu(); return }
  if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
  event.preventDefault()
  const items = Array.from(moreActions.value?.querySelectorAll<HTMLButtonElement>('.action-menu button:not(:disabled)') ?? [])
  const current = items.indexOf(window.document.activeElement as HTMLButtonElement)
  let index = (current + (event.key === 'ArrowUp' ? -1 : 1) + items.length) % items.length
  if (event.key === 'Home') index = 0
  if (event.key === 'End') index = items.length - 1
  items[index]?.focus()
}

function startTabDrag(event: PointerEvent, id: string) {
  if (event.button !== 0 || closing.value || question.value) return
  suppressTabClick = false
  dragHandle = event.currentTarget as HTMLElement
  dragHandle.setPointerCapture(event.pointerId)
  tabDrag.value = { id, pointerID: event.pointerId, startX: event.clientX, startY: event.clientY, x: event.clientX, y: event.clientY, started: false }
}

function updateTabDrop() {
  const drag = tabDrag.value
  const tabList = tabs.value
  dropIndex.value = undefined
  if (!drag?.started || !tabList) return
  const bounds = tabList.getBoundingClientRect()
  if (drag.y < bounds.top - 20 || drag.y > bounds.bottom + 20 || drag.x < bounds.left - 24 || drag.x > bounds.right + 24) return
  const remaining = Array.from(tabList.querySelectorAll<HTMLElement>('.tab')).filter(element => element.dataset.tabId !== drag.id)
  const next = remaining.findIndex(element => {
    const rect = element.getBoundingClientRect()
    return drag.x < rect.left + rect.width / 2
  })
  const index = next < 0 ? remaining.length : next
  const boundary = remaining[index]?.getBoundingClientRect().left ?? remaining.at(-1)?.getBoundingClientRect().right ?? bounds.left
  dropIndex.value = index
  dropLine.value = { left: Math.max(bounds.left, Math.min(bounds.right, boundary)), top: bounds.top + 12, height: bounds.height - 24 }
}

function scrollDuringTabDrag() {
  tabScrollFrame = undefined
  const drag = tabDrag.value
  const tabList = tabs.value
  if (!drag?.started || !tabList || dropIndex.value === undefined) return
  const bounds = tabList.getBoundingClientRect()
  const edge = 36
  const leftSpeed = Math.max(0, edge - (drag.x - bounds.left))
  const rightSpeed = Math.max(0, edge - (bounds.right - drag.x))
  const previousScroll = tabList.scrollLeft
  tabList.scrollLeft += Math.max(-12, Math.min(12, (rightSpeed - leftSpeed) / 3))
  if (previousScroll !== tabList.scrollLeft) {
    updateTabDrop()
    tabScrollFrame = requestAnimationFrame(scrollDuringTabDrag)
  }
}

function moveTabDrag(event: PointerEvent) {
  const drag = tabDrag.value
  if (!drag || event.pointerId !== drag.pointerID) return
  drag.x = event.clientX
  drag.y = event.clientY
  if (!drag.started && Math.hypot(drag.x - drag.startX, drag.y - drag.startY) < 6) return
  drag.started = true
  event.preventDefault()
  updateTabDrop()
  if (tabScrollFrame === undefined) tabScrollFrame = requestAnimationFrame(scrollDuringTabDrag)
}

function finishTabDrag() {
  const drag = tabDrag.value
  if (!drag) return
  suppressTabClick = drag.started
  tabDrag.value = undefined
  dropIndex.value = undefined
  if (tabScrollFrame !== undefined) cancelAnimationFrame(tabScrollFrame)
  tabScrollFrame = undefined
  if (dragHandle?.hasPointerCapture(drag.pointerID)) dragHandle.releasePointerCapture(drag.pointerID)
  dragHandle = undefined
}

function dropTab(event: PointerEvent) {
  const drag = tabDrag.value
  if (!drag || event.pointerId !== drag.pointerID) return
  updateTabDrop()
  const index = dropIndex.value
  const from = documents.value.findIndex(document => document.id === drag.id)
  if (drag.started && index !== undefined && from >= 0) {
    const [moved] = documents.value.splice(from, 1)
    documents.value.splice(index, 0, moved)
  }
  finishTabDrag()
}

function clickTab(id: string) {
  if (suppressTabClick) { suppressTabClick = false; return }
  selectTab(id)
}

const modes: Array<{ value: ViewMode; label: string }> = [
  { value: 'source', label: '原文' }, { value: 'split', label: '分栏' }, { value: 'preview', label: '预览' },
]

onMounted(async () => {
  window.addEventListener('keydown', onKeydown, true)
  window.addEventListener('pointerdown', onOutsidePointerDown)
  window.addEventListener('pointermove', moveTabDrag, { passive: false })
  window.addEventListener('pointerup', dropTab)
  window.addEventListener('pointercancel', finishTabDrag)
  window.addEventListener('blur', finishTabDrag)
  unsubscribe.push(Events.On('files:drop', event => { void openFiles(event.data as string[]) }))
  unsubscribe.push(Events.On('app:close-request', () => { void closeApplication() }))
  try { ingest(await AppService.Ready()) } catch (error) { showError(error) }
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown, true)
  window.removeEventListener('pointerdown', onOutsidePointerDown)
  window.removeEventListener('pointermove', moveTabDrag)
  window.removeEventListener('pointerup', dropTab)
  window.removeEventListener('pointercancel', finishTabDrag)
  window.removeEventListener('blur', finishTabDrag)
  finishTabDrag()
  unsubscribe.forEach(off => off())
  clearTimeout(noticeTimer)
})
</script>

<template>
  <main class="app" data-file-drop-target>
    <header class="toolbar">
      <button class="open-button" title="打开 Markdown 文件 · Ctrl+O" :disabled="opening || closing" @click="pickFiles"><span class="folder-icon" aria-hidden="true" /><span>打开</span></button>
      <div ref="tabs" class="tabs" :class="{ 'dragging-tabs': tabDrag?.started }" role="tablist" aria-label="已打开的文件" @wheel="scrollTabs">
        <div v-for="document in documents" :key="document.id" class="tab" :class="{ selected: activeID === document.id, 'tab-dragging': tabDrag?.started && tabDrag.id === document.id }" :data-tab-id="document.id" :title="document.path" @pointerdown.middle.prevent @auxclick.middle.prevent="closeTab(document)">
          <button
            class="tab-select"
            role="tab"
            :aria-selected="activeID === document.id"
            :tabindex="activeID === document.id ? 0 : -1"
            @click="clickTab(document.id)"
            @pointerdown="startTabDrag($event, document.id)"
            @lostpointercapture="finishTabDrag"
            @dragstart.prevent
            @keydown.left.prevent="switchTab(-1)"
            @keydown.right.prevent="switchTab(1)"
          >
            <span v-if="isDirty(document)" class="dirty-dot" aria-label="有未保存的修改" />
            <span class="tab-name">{{ document.name }}</span>
          </button>
          <button class="tab-close" :aria-label="`关闭 ${document.name}`" title="关闭 · 鼠标中键 / Ctrl+W" :disabled="closing" @click="closeTab(document)">×</button>
        </div>
      </div>
      <div class="toolbar-actions">
        <div class="view-switch" role="group" aria-label="显示模式">
          <button v-for="mode in modes" :key="mode.value" :class="{ chosen: active?.mode === mode.value }" :aria-pressed="active?.mode === mode.value" :disabled="!active" @click="active && (active.mode = mode.value)">{{ mode.label }}</button>
        </div>
        <button v-if="active && isDirty(active)" class="save-button" title="保存 · Ctrl+S" :disabled="saving.has(active.id) || closing" @click="saveDocument(active)">{{ saving.has(active.id) ? '保存中' : '保存' }}</button>
        <div ref="moreActions" class="more-actions">
          <button class="more-button" aria-label="更多操作" aria-haspopup="menu" :aria-expanded="menuOpen" aria-controls="action-menu" title="更多操作" @click="toggleMenu" @keydown.down.prevent="toggleMenu">…</button>
          <div v-if="menuOpen" id="action-menu" class="action-menu" role="menu" aria-label="更多操作" @keydown="onMenuKeydown">
            <button class="menu-copy" role="menuitem" :disabled="!active" @click="copyMarkdown"><span>复制 Markdown</span><kbd>Ctrl+Shift+C</kbd></button>
            <button class="menu-lock" role="menuitemcheckbox" :aria-checked="locked" :disabled="closing" @click="toggleLock">
              <span>编辑锁定</span><span class="menu-meta"><kbd>Ctrl+Shift+L</kbd><span class="switch-track" :class="{ on: locked }" aria-hidden="true"><span /></span></span>
            </button>
          </div>
        </div>
      </div>
    </header>

    <template v-if="tabDrag?.started">
      <div v-if="dropIndex !== undefined" class="tab-drop-line" :style="{ left: `${dropLine.left}px`, top: `${dropLine.top}px`, height: `${dropLine.height}px` }" />
      <div class="tab-drag-preview" :style="{ left: `${tabDrag.x + 12}px`, top: `${tabDrag.y + 14}px` }">{{ draggedDocument?.name }}</div>
    </template>

    <div v-if="!documents.length" class="empty-state">
      <div class="empty-content">
        <h1>打开一篇 Markdown</h1>
        <p>拖入文件，或从电脑中选择</p>
        <button class="empty-open" :disabled="opening" @click="pickFiles">打开文件 <kbd>Ctrl O</kbd></button>
      </div>
    </div>
    <DocumentPane
      v-for="document in documentPanes"
      v-show="document.id === activeID"
      :key="document.id"
      :document="document"
      :locked="locked || closing"
      :active="document.id === activeID"
      @change="changeContent"
      @error="showError"
    />

    <div v-if="notification" class="notification" :class="{ error: notificationError }" :role="notificationError ? 'alert' : 'status'">
      <span>{{ notification }}</span>
      <button v-if="notificationError" aria-label="关闭提示" @click="notification = ''">×</button>
    </div>

    <dialog ref="dialog" class="confirm-dialog" aria-labelledby="dialog-title" aria-describedby="dialog-body" @cancel.prevent="answerQuestion('cancel')">
      <template v-if="question">
        <h2 id="dialog-title">{{ question.title }}</h2>
        <p id="dialog-body">{{ question.body }}</p>
        <div class="dialog-actions">
          <button autofocus @click="answerQuestion('cancel')">取消</button>
          <template v-if="question.kind === 'close'">
            <button @click="answerQuestion('discard')">放弃修改</button>
            <button class="primary" @click="answerQuestion('save')">保存</button>
          </template>
          <template v-else>
            <button @click="answerQuestion('reload')">重新载入</button>
            <button class="primary" @click="answerQuestion('overwrite')">覆盖保存</button>
          </template>
        </div>
      </template>
    </dialog>
  </main>
</template>
