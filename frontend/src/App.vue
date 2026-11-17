<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { Events } from '@wailsio/runtime'
import { AppService } from '../bindings/markdownlite'
import DocumentPane from './components/DocumentPane.vue'
import type { DocumentTab, OpenDocument, ViewMode } from './types'

const documents = ref<DocumentTab[]>([])
// Stable pane order preserves browser scroll positions when tabs move.
const documentPanes = computed(() => [...documents.value].sort((first, second) => first.key.localeCompare(second.key)))
const activeID = ref('')
const darkMode = ref(document.documentElement.dataset.theme === 'dark')
const changingTheme = ref(true)
const opening = ref(false)
const closing = ref(false)
const saving = reactive(new Set<string>())
const active = computed(() => documents.value.find(document => document.id === activeID.value))
const locked = computed(() => active.value?.locked ?? true)
const appRoot = ref<HTMLElement>()
const tabs = ref<HTMLElement>()
const tabNavigation = ref<HTMLElement>()
const tabStrip = ref<HTMLElement>()
const tabOverflow = ref<HTMLElement>()
const hasTabOverflow = ref(false)
const tabMenuOpen = ref(false)
const fileContext = ref<{ document: DocumentTab; x: number; y: number }>()
const fileContextMenu = ref<HTMLElement>()
const renameDialog = ref<HTMLDialogElement>()
const renameInput = ref<HTMLInputElement>()
const renamePrompt = ref<{ document: DocumentTab; name: string; error: string }>()
const renaming = ref(false)
const moreActions = ref<HTMLElement>()
const menuOpen = ref(false)
const tabDrag = ref<{ id: string; pointerID: number; startX: number; startY: number; x: number; y: number; started: boolean }>()
const dropIndex = ref<number>()
const dropLine = ref({ left: 0, top: 0, height: 0 })
const draggedDocument = computed(() => documents.value.find(document => document.id === tabDrag.value?.id))
const notification = ref('')
const notificationError = ref(false)
const dialog = ref<HTMLDialogElement>()
const question = ref<{ title: string; body: string; kind: 'close' | 'conflict' | 'replace' }>()
let finishQuestion: ((answer: string) => void) | undefined
let noticeTimer: ReturnType<typeof setTimeout> | undefined
let dragHandle: HTMLElement | undefined
let tabScrollFrame: number | undefined
let tabOverflowFrame: number | undefined
let tabResizeObserver: ResizeObserver | undefined
let suppressTabClick = false
let newDocumentCount = 0
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

function selectTab(id: string, focus = false) {
  activeID.value = id
  nextTick(() => {
    const selected = tabs.value?.querySelector<HTMLElement>('[aria-selected="true"]')
    selected?.closest('.tab')?.scrollIntoView({ block: 'nearest', inline: 'nearest' })
    if (focus) selected?.focus({ preventScroll: true })
  })
}

function ingest(result: { documents: OpenDocument[] | null; errors: string[] | null }) {
  for (const document of result.documents ?? []) {
    if (!documents.value.some(existing => existing.id === document.id)) {
      documents.value.push({ ...document, key: document.id, locked: true, savedContent: document.content, mode: 'split', generation: 0 })
    }
    selectTab(document.id)
  }
  if (result.errors?.length) notify(result.errors.join('\n'), true)
}

function newDocument() {
  if (opening.value || closing.value || question.value || renamePrompt.value) return
  closeMenu()
  closeTabMenu()
  closeFileContext()
  const id = `new:${crypto.randomUUID()}`
  documents.value.push({
    id, key: id, path: '', name: `未命名${++newDocumentCount}.md`,
    content: '', savedContent: '', revision: '', mode: 'split', generation: 0, locked: false,
  })
  selectTab(id)
}

async function pickFiles() {
  if (opening.value || closing.value || question.value || renamePrompt.value) return
  opening.value = true
  try { ingest(await AppService.PickFiles()) } catch (error) { showError(error) }
  finally { opening.value = false }
}

async function openFiles(paths: string[]) {
  if (closing.value || question.value || renamePrompt.value) return
  try { ingest(await AppService.OpenFiles(paths)) } catch (error) { showError(error) }
}

function changeContent(id: string, content: string) {
  const document = documents.value.find(document => document.id === id)
  if (document) document.content = content
}

async function ask(title: string, body: string, kind: 'close' | 'conflict' | 'replace'): Promise<string> {
  if (question.value) return 'cancel'
  closeFileContext()
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

async function saveNewDocument(document: DocumentTab, content: string): Promise<boolean> {
  if (opening.value) return false
  opening.value = true
  let target: OpenDocument | null
  try { target = await AppService.PickSaveTarget(document.name) }
  finally { opening.value = false }
  if (!target) return false
  const existing = documents.value.find(other => other !== document && other.id === target.id)
  if (existing && saving.has(existing.id)) { notify('目标文件正在保存，请稍后重试'); return false }
  if (existing && isDirty(existing)) {
    const answer = await ask('覆盖已打开的文件？', `“${existing.name}”有尚未保存的修改，覆盖保存会替换这些内容。`, 'replace')
    if (answer !== 'overwrite') return false
  }
  const existingContent = existing?.content
  let result = await AppService.SaveNew(target.path, content, target.revision, false)
  if (result.conflict) {
    const answer = await ask('磁盘上的文件已更新', `“${target.name}”在选择保存位置后发生了变化，是否覆盖保存？`, 'replace')
    if (answer !== 'overwrite') return false
    result = await AppService.SaveNew(target.path, content, target.revision, true)
  }
  if (!result.document) return false
  const saved = result.document
  const draftID = document.id
  const savedTab = documents.value.find(other => other !== document && other.id === saved.id)
  if (savedTab) {
    const preserveEdits = savedTab === existing ? savedTab.content !== existingContent : isDirty(savedTab)
    Object.assign(savedTab, saved, {
      content: preserveEdits ? savedTab.content : content, savedContent: content,
      generation: savedTab.generation + (preserveEdits ? 0 : 1),
    })
    if (document.content === content) {
      savedTab.locked = document.locked
      documents.value.splice(documents.value.indexOf(document), 1)
      if (activeID.value === draftID) selectTab(savedTab.id)
    }
  } else {
    Object.assign(document, saved, { content: document.content, savedContent: content })
    if (activeID.value === draftID) selectTab(saved.id)
  }
  notify('已保存')
  return document.content === content
}

async function saveDocument(document: DocumentTab): Promise<boolean> {
  if (saving.has(document.id)) return false
  if (document.path && !isDirty(document)) return true
  const saveID = document.id
  saving.add(saveID)
  const content = document.content
  try {
    if (!document.path) return await saveNewDocument(document, content)
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
    saving.delete(saveID)
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
  if (closing.value || question.value || renamePrompt.value) return
  if (saving.size) { notify('正在保存，请稍候'); return }
  closeTabMenu()
  closeFileContext()
  closing.value = true
  try {
    if (!(await confirmClose(document))) return
    const index = documents.value.indexOf(document)
    if (index < 0) return
    documents.value.splice(index, 1)
    if (activeID.value === document.id) selectTab(documents.value[Math.min(index, documents.value.length - 1)]?.id ?? '')
  } finally { closing.value = false }
}

async function closeApplication() {
  if (closing.value || question.value || renamePrompt.value) return
  if (saving.size) { notify('正在保存，请稍候'); return }
  closing.value = true
  try {
    for (const document of [...documents.value]) {
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
  if (!active.value) return
  closeMenu(true)
  active.value.locked = !active.value.locked
  notify(locked.value ? '编辑已锁定' : '已允许编辑')
}

async function toggleDarkMode() {
  changingTheme.value = true
  closeMenu(true)
  darkMode.value = !darkMode.value
  const theme = darkMode.value ? 'dark' : 'light'
  document.documentElement.dataset.theme = theme
  try { localStorage.setItem('moye.theme', theme) } catch { /* Apply the theme for this session. */ }
  try { await AppService.SetDarkMode(darkMode.value) } catch (error) { showError(error) }
  finally { changingTheme.value = false }
}

function switchTab(direction: number) {
  if (!documents.value.length) return
  const index = documents.value.findIndex(document => document.id === activeID.value)
  selectTab(documents.value[(index + direction + documents.value.length) % documents.value.length].id)
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    clearFileDrop()
    if (tabDrag.value) {
      event.preventDefault()
      finishTabDrag()
    }
    if (menuOpen.value) {
      event.preventDefault()
      closeMenu(true)
    }
    if (tabMenuOpen.value) {
      event.preventDefault()
      closeTabMenu(true)
    }
    if (fileContext.value) {
      event.preventDefault()
      closeFileContext()
      selectTab(activeID.value, true)
    }
    return
  }
  if (!(event.ctrlKey || event.metaKey) || event.altKey || event.isComposing) return
  const key = event.key.toLowerCase()
  const recognized = ['n', 'o', 's', 'w', 'tab'].includes(key) || (event.shiftKey && ['l', 'c'].includes(key))
  if (!recognized) return
  event.preventDefault()
  event.stopImmediatePropagation()
  if (question.value || closing.value || renamePrompt.value) return
  if (event.shiftKey && key === 'l') { toggleLock(); return }
  if (event.shiftKey && key === 'c') { void copyMarkdown(); return }
  if (key === 'n') newDocument()
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

function measureTabOverflow() {
  tabOverflowFrame = undefined
  if (!tabNavigation.value || !tabStrip.value) return
  // Measure the full space before reserving the More button to keep the threshold stable.
  hasTabOverflow.value = tabStrip.value.scrollWidth > tabNavigation.value.clientWidth + 1
  if (!hasTabOverflow.value) closeTabMenu(true)
  nextTick(() => tabs.value?.querySelector<HTMLElement>('[aria-selected="true"]')?.closest('.tab')?.scrollIntoView({ block: 'nearest', inline: 'nearest' }))
}

function scheduleTabOverflow() {
  if (tabOverflowFrame === undefined) tabOverflowFrame = requestAnimationFrame(measureTabOverflow)
}

function closeTabMenu(restoreFocus = false) {
  if (!tabMenuOpen.value) return
  tabMenuOpen.value = false
  if (restoreFocus) nextTick(() => {
    const button = hasTabOverflow.value
      ? tabOverflow.value?.querySelector<HTMLButtonElement>('.tabs-more-button')
      : tabs.value?.querySelector<HTMLButtonElement>('[aria-selected="true"]')
    button?.focus({ preventScroll: true })
  })
}

function toggleTabMenu() {
  if (tabMenuOpen.value) { closeTabMenu(true); return }
  closeMenu()
  closeFileContext()
  tabMenuOpen.value = true
  nextTick(() => {
    const item = tabOverflow.value?.querySelector<HTMLButtonElement>('[aria-checked="true"]')
      ?? tabOverflow.value?.querySelector<HTMLButtonElement>('[role="menuitemradio"]')
    item?.focus()
  })
}

function selectOverflowTab(id: string) {
  closeTabMenu()
  selectTab(id, true)
}

function closeMenu(restoreFocus = false) {
  if (!menuOpen.value) return
  menuOpen.value = false
  if (restoreFocus) nextTick(() => moreActions.value?.querySelector<HTMLButtonElement>('.more-button')?.focus())
}

function toggleMenu() {
  if (menuOpen.value) { closeMenu(true); return }
  closeTabMenu()
  closeFileContext()
  menuOpen.value = true
  nextTick(() => moreActions.value?.querySelector<HTMLButtonElement>('.action-menu button:not(:disabled)')?.focus())
}

function onOutsidePointerDown(event: PointerEvent) {
  if (!moreActions.value?.contains(event.target as Node)) closeMenu()
  if (!tabOverflow.value?.contains(event.target as Node)) closeTabMenu()
  if (!fileContextMenu.value?.contains(event.target as Node)) closeFileContext()
}

function onMenuKeydown(event: KeyboardEvent) {
  if (event.key === 'Tab') { closeMenu(); closeTabMenu(); closeFileContext(); return }
  if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
  event.preventDefault()
  const items = Array.from((event.currentTarget as HTMLElement).querySelectorAll<HTMLButtonElement>('button:not(:disabled)'))
  if (!items.length) return
  const current = items.indexOf(window.document.activeElement as HTMLButtonElement)
  let index = (current + (event.key === 'ArrowUp' ? -1 : 1) + items.length) % items.length
  if (event.key === 'Home') index = 0
  if (event.key === 'End') index = items.length - 1
  items[index]?.focus()
}

function closeFileContext() {
  fileContext.value = undefined
}

async function openFileContext(event: MouseEvent, document: DocumentTab) {
  if (closing.value || question.value || renamePrompt.value) return
  closeMenu()
  closeTabMenu()
  finishTabDrag()
  selectTab(document.id)
  const anchor = (event.currentTarget as HTMLElement).getBoundingClientRect()
  const keyboard = event.clientX === 0 && event.clientY === 0
  fileContext.value = { document, x: keyboard ? anchor.left : event.clientX, y: keyboard ? anchor.bottom : event.clientY }
  const context = fileContext.value
  await nextTick()
  if (fileContext.value !== context || !fileContextMenu.value) return
  const bounds = fileContextMenu.value.getBoundingClientRect()
  context.x = Math.max(8, Math.min(context.x, window.innerWidth - bounds.width - 8))
  context.y = Math.max(8, Math.min(context.y, window.innerHeight - bounds.height - 8))
  fileContextMenu.value.querySelector<HTMLButtonElement>('button:not(:disabled)')?.focus({ preventScroll: true })
}

async function openFileFolder(document: DocumentTab) {
  closeFileContext()
  try { await AppService.OpenFolder(document.id) } catch (error) { showError(error) }
}

async function copyFilePath(document: DocumentTab) {
  closeFileContext()
  try {
    await AppService.Copy(document.path)
    notify('已复制文件路径')
  } catch (error) { showError(error) }
}

async function beginRename(document: DocumentTab) {
  if (saving.size || closing.value || question.value || renamePrompt.value) return
  closeFileContext()
  renamePrompt.value = { document, name: document.name, error: '' }
  await nextTick()
  renameDialog.value?.showModal()
  renameInput.value?.focus()
  const extension = document.name.lastIndexOf('.')
  renameInput.value?.setSelectionRange(0, extension > 0 ? extension : document.name.length)
}

function cancelRename() {
  if (renaming.value) return
  renameDialog.value?.close()
  renamePrompt.value = undefined
  selectTab(activeID.value, true)
}

async function confirmRename() {
  const prompt = renamePrompt.value
  if (!prompt || renaming.value) return
  if (prompt.name === prompt.document.name) { cancelRename(); return }
  renaming.value = true
  prompt.error = ''
  const document = prompt.document
  const previousID = document.id
  try {
    const openedIDs = documents.value.filter(other => other !== document && other.path).map(other => other.id)
    const result = await AppService.Rename(document.path ? document.id : '', prompt.name, document.name, openedIDs)
    if (document.path) {
      document.id = result.id
      document.path = result.path
      if (activeID.value === previousID) selectTab(result.id)
    }
    document.name = result.name
    renameDialog.value?.close()
    renamePrompt.value = undefined
    selectTab(activeID.value, true)
    notify('已重命名')
  } catch (error) {
    prompt.error = error instanceof Error ? error.message : String(error)
  } finally { renaming.value = false }
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

function clearFileDrop() {
  appRoot.value?.classList.remove('file-drop-target-active')
}

function onFileDragLeave(event: DragEvent) {
  // Windows reports a null relatedTarget when a drag leaves the WebView.
  if (event.relatedTarget === null) clearFileDrop()
}

const modes: Array<{ value: ViewMode; label: string }> = [
  { value: 'source', label: '原文' }, { value: 'split', label: '分栏' }, { value: 'preview', label: '预览' },
]

onMounted(async () => {
  tabResizeObserver = new ResizeObserver(scheduleTabOverflow)
  for (const element of [tabNavigation.value, tabStrip.value, tabs.value]) {
    if (element) tabResizeObserver.observe(element)
  }
  scheduleTabOverflow()
  window.addEventListener('keydown', onKeydown, true)
  window.addEventListener('pointerdown', onOutsidePointerDown)
  window.addEventListener('pointermove', moveTabDrag, { passive: false })
  window.addEventListener('pointerup', dropTab)
  window.addEventListener('pointercancel', finishTabDrag)
  window.addEventListener('blur', finishTabDrag)
  window.addEventListener('blur', clearFileDrop)
  window.addEventListener('blur', closeFileContext)
  window.addEventListener('resize', closeFileContext)
  unsubscribe.push(Events.On('files:drop', event => { void openFiles(event.data as string[]) }))
  unsubscribe.push(Events.On('app:close-request', () => { void closeApplication() }))
  void AppService.SetDarkMode(darkMode.value).catch(showError).finally(() => { changingTheme.value = false })
  try { ingest(await AppService.Ready()) } catch (error) { showError(error) }
})

onBeforeUnmount(() => {
  tabResizeObserver?.disconnect()
  if (tabOverflowFrame !== undefined) cancelAnimationFrame(tabOverflowFrame)
  window.removeEventListener('keydown', onKeydown, true)
  window.removeEventListener('pointerdown', onOutsidePointerDown)
  window.removeEventListener('pointermove', moveTabDrag)
  window.removeEventListener('pointerup', dropTab)
  window.removeEventListener('pointercancel', finishTabDrag)
  window.removeEventListener('blur', finishTabDrag)
  window.removeEventListener('blur', clearFileDrop)
  window.removeEventListener('blur', closeFileContext)
  window.removeEventListener('resize', closeFileContext)
  finishTabDrag()
  unsubscribe.forEach(off => off())
  clearTimeout(noticeTimer)
})
</script>

<template>
  <main ref="appRoot" class="app" data-file-drop-target @dragleave="onFileDragLeave" @drop.capture="clearFileDrop">
    <header class="toolbar">
      <button class="open-button new-button" title="新建 Markdown 文件 · Ctrl+N" :disabled="opening || closing" @click="newDocument"><span aria-hidden="true">＋</span><span>新建</span></button>
      <button class="open-button" title="打开 Markdown 文件 · Ctrl+O" :disabled="opening || closing" @click="pickFiles"><span class="folder-icon" aria-hidden="true" /><span>打开</span></button>
      <div ref="tabNavigation" class="tab-navigation">
        <div ref="tabs" class="tabs" :class="{ 'dragging-tabs': tabDrag?.started }" role="tablist" aria-label="已打开的文件" @wheel="scrollTabs">
          <div ref="tabStrip" class="tab-strip">
            <div v-for="document in documents" :key="document.key" class="tab" :class="{ selected: activeID === document.id, 'tab-dragging': tabDrag?.started && tabDrag.id === document.id }" :data-tab-id="document.id" :title="document.path || '尚未保存'" @contextmenu.prevent.stop="openFileContext($event, document)" @pointerdown.middle.prevent @auxclick.middle.prevent="closeTab(document)">
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
        </div>
        <div v-if="hasTabOverflow" ref="tabOverflow" class="tab-overflow">
          <button class="tabs-more-button" aria-label="更多文章" aria-haspopup="menu" :aria-expanded="tabMenuOpen" aria-controls="tabs-menu" @click="toggleTabMenu" @keydown.down.prevent="toggleTabMenu">
            <span>更多文章</span><span class="chevron" aria-hidden="true" />
          </button>
          <div v-if="tabMenuOpen" id="tabs-menu" class="action-menu tabs-menu" role="menu" aria-label="已打开的文章" @keydown="onMenuKeydown">
            <button v-for="document in documents" :key="document.key" class="tab-menu-item" :class="{ current: activeID === document.id }" role="menuitemradio" :aria-checked="activeID === document.id" :title="document.path || '尚未保存'" :disabled="closing" @contextmenu.prevent.stop="openFileContext($event, document)" @click="selectOverflowTab(document.id)">
              <span class="tab-menu-check" aria-hidden="true">{{ activeID === document.id ? '✓' : '' }}</span>
              <span class="tab-menu-label"><span class="tab-menu-name">{{ document.name }}</span><span class="tab-menu-path">{{ document.path || '尚未保存' }}</span></span>
              <span v-if="isDirty(document)" class="dirty-dot" aria-label="有未保存的修改" />
            </button>
          </div>
        </div>
      </div>
      <div class="toolbar-actions">
        <div class="view-switch" role="group" aria-label="显示模式">
          <button v-for="mode in modes" :key="mode.value" :class="{ chosen: active?.mode === mode.value }" :aria-pressed="active?.mode === mode.value" :disabled="!active" @click="active && (active.mode = mode.value)">{{ mode.label }}</button>
        </div>
        <button v-if="active && (!active.path || isDirty(active))" class="save-button" title="保存 · Ctrl+S" :disabled="saving.has(active.id) || closing" @click="saveDocument(active)">{{ saving.has(active.id) ? '保存中' : '保存' }}</button>
        <div ref="moreActions" class="more-actions">
          <button class="more-button" aria-label="更多操作" aria-haspopup="menu" :aria-expanded="menuOpen" aria-controls="action-menu" title="更多操作" @click="toggleMenu" @keydown.down.prevent="toggleMenu">…</button>
          <div v-if="menuOpen" id="action-menu" class="action-menu" role="menu" aria-label="更多操作" @keydown="onMenuKeydown">
            <button class="menu-copy" role="menuitem" :disabled="!active" @click="copyMarkdown"><span>复制 Markdown</span><kbd>Ctrl+Shift+C</kbd></button>
            <button class="menu-lock" role="menuitemcheckbox" :aria-checked="locked" :disabled="!active || closing" @click="toggleLock">
              <span>编辑锁定</span><span class="menu-meta"><kbd>Ctrl+Shift+L</kbd><span class="switch-track" :class="{ on: locked }" aria-hidden="true"><span /></span></span>
            </button>
            <button class="menu-theme" role="menuitemcheckbox" :aria-checked="darkMode" :disabled="closing || changingTheme" @click="toggleDarkMode">
              <span>黑夜模式</span><span class="switch-track" :class="{ on: darkMode }" aria-hidden="true"><span /></span>
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
      <section class="welcome" aria-labelledby="welcome-title">
        <header class="welcome-header">
          <h1 id="welcome-title">墨页</h1>
          <p class="welcome-lead">本地 Markdown 阅读与编辑工具</p>
          <p class="welcome-description">阅读项目说明、技术文档和本地笔记，也可以新建文档、编辑文章。</p>
          <div class="welcome-actions">
            <button class="empty-open" :disabled="opening || closing" @click="pickFiles">打开文件 <kbd>Ctrl+O</kbd></button>
            <button class="empty-open welcome-new" :disabled="opening || closing" @click="newDocument">新建文档 <kbd>Ctrl+N</kbd></button>
          </div>
          <p class="welcome-drop-hint">也可以把文件拖入窗口，支持 UTF-8 编码的 .md、.markdown 文件，可一次打开多篇。</p>
        </header>

        <section class="welcome-guides" aria-label="功能与使用方式">
          <div class="welcome-guide">
            <h2>阅读与预览</h2>
            <p>在“原文 / 分栏 / 预览”之间切换。支持代码高亮、表格、任务列表和本地图片。</p>
          </div>
          <div class="welcome-guide">
            <h2>写作与保存</h2>
            <p>新建文档可直接输入。打开已有文件后，在右上角“…”中关闭“编辑锁定”即可修改。新文档首次保存时选择位置。</p>
          </div>
          <div class="welcome-guide">
            <h2>管理多个文件</h2>
            <p>点击标签切换文章，拖动标签调整顺序。标签较多时使用“更多文章”。右键标签可打开文件夹、复制路径或重命名。</p>
          </div>
        </section>

        <section class="welcome-help" aria-labelledby="welcome-shortcuts-title">
          <h2 id="welcome-shortcuts-title">常用快捷键</h2>
          <dl class="welcome-shortcuts">
            <div><dt>保存文档</dt><dd><kbd>Ctrl+S</kbd></dd></div>
            <div><dt>切换编辑锁定</dt><dd><kbd>Ctrl+Shift+L</kbd></dd></div>
            <div><dt>复制完整 Markdown</dt><dd><kbd>Ctrl+Shift+C</kbd></dd></div>
            <div><dt>切换标签</dt><dd><kbd>Ctrl+Tab</kbd></dd></div>
          </dl>
          <p class="welcome-menu-hint">右上角“…”还可以复制完整 Markdown、切换黑夜模式。</p>
        </section>
      </section>
    </div>
    <DocumentPane
      v-for="document in documentPanes"
      v-show="document.id === activeID"
      :key="document.key"
      :document="document"
      :locked="document.locked || closing"
      :dark="darkMode"
      :active="document.id === activeID"
      @change="changeContent"
      @error="showError"
    />

    <div v-if="!question && !closing" class="file-drop-overlay" role="status" aria-live="polite">
      <div class="file-drop-content">
        <span class="file-drop-icon" aria-hidden="true" />
        <h2>松开鼠标，打开文件</h2>
        <p>支持 .md、.markdown，可一次拖入多个文件</p>
      </div>
    </div>

    <div v-if="notification" class="notification" :class="{ error: notificationError }" :role="notificationError ? 'alert' : 'status'">
      <span>{{ notification }}</span>
      <button v-if="notificationError" aria-label="关闭提示" @click="notification = ''">×</button>
    </div>

    <div v-if="fileContext" ref="fileContextMenu" class="action-menu tab-context-menu" role="menu" aria-label="文件操作" :style="{ left: `${fileContext.x}px`, top: `${fileContext.y}px` }" @keydown="onMenuKeydown">
      <button role="menuitem" :disabled="!fileContext.document.path" @click="openFileFolder(fileContext.document)">打开当前文件夹</button>
      <button role="menuitem" :disabled="!fileContext.document.path" @click="copyFilePath(fileContext.document)">复制文件路径</button>
      <button role="menuitem" :disabled="saving.size > 0" @click="beginRename(fileContext.document)">重命名</button>
    </div>

    <dialog ref="renameDialog" class="confirm-dialog rename-dialog" aria-labelledby="rename-title" @cancel.prevent="cancelRename">
      <form v-if="renamePrompt" @submit.prevent="confirmRename">
        <h2 id="rename-title">重命名</h2>
        <label for="rename-name">文件名</label>
        <input id="rename-name" ref="renameInput" v-model="renamePrompt.name" autocomplete="off" spellcheck="false" :disabled="renaming" :aria-invalid="!!renamePrompt.error" :aria-describedby="renamePrompt.error ? 'rename-error' : undefined" />
        <p v-if="renamePrompt.error" id="rename-error" class="rename-error" role="alert">{{ renamePrompt.error }}</p>
        <div class="dialog-actions">
          <button type="button" :disabled="renaming" @click="cancelRename">取消</button>
          <button type="submit" class="primary" :disabled="renaming || !renamePrompt.name.trim()">{{ renaming ? '重命名中' : '重命名' }}</button>
        </div>
      </form>
    </dialog>

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
            <button v-if="question.kind === 'conflict'" @click="answerQuestion('reload')">重新载入</button>
            <button class="primary" @click="answerQuestion('overwrite')">覆盖保存</button>
          </template>
        </div>
      </template>
    </dialog>
  </main>
</template>
