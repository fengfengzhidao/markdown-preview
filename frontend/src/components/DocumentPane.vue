<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { basicSetup } from 'codemirror'
import { Compartment, EditorState, Text } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { markdown } from '@codemirror/lang-markdown'
import { languages } from '@codemirror/language-data'
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language'
import { tags } from '@lezer/highlight'
import { AppService } from '../../bindings/markdownlite'
import { renderMarkdown } from '../markdown'
import type { MarkdownHeading } from '../markdown'
import { clearPreviewSearch, highlightPreviewSearch } from '../previewSearch'
import type { PreviewMatch } from '../previewSearch'
import type { DocumentTab } from '../types'
import OutlinePanel from './OutlinePanel.vue'
import ImageViewer from './ImageViewer.vue'

const props = defineProps<{ document: DocumentTab; locked: boolean; active: boolean; dark: boolean; saveDocument: (document: DocumentTab) => Promise<boolean> }>()
const emit = defineEmits<{ change: [id: string, content: string]; error: [message: string] }>()
const editorHost = ref<HTMLElement>()
const preview = ref<HTMLElement>()
const container = ref<HTMLElement>()
const article = ref<HTMLElement>()
const searchInput = ref<HTMLInputElement>()
const outlineOpen = ref(false)
const syncScroll = ref(true)
const currentHeading = ref('')
const searchOpen = ref(false)
const searchQuery = ref('')
const caseSensitive = ref(false)
const matchCount = ref(0)
const matchIndex = ref(-1)
const selectedImage = ref<{ src: string; alt: string }>()
const pastingImage = ref(false)
const ratio = ref(50)
const resizing = ref(false)
const renderedContent = ref(props.document.content)
const rendered = computed(() => renderMarkdown(renderedContent.value, props.document.id))
const lockConfiguration = new Compartment()
const themeConfiguration = new Compartment()
let editor: EditorView | undefined
let renderTimer: ReturnType<typeof setTimeout> | undefined
let searchTimer: ReturnType<typeof setTimeout> | undefined
let scrollFrame: number | undefined
let layoutFrame: number | undefined
let resizeObserver: ResizeObserver | undefined
let matches: PreviewMatch[] = []
let selectedMarks: HTMLElement[] = []
let scrollLeader: 'source' | 'preview' | 'navigation' = 'source'
let anchors: Array<{ line: number; top: number }> = []
let headingPositions: Array<{ id: string; top: number }> = []
let pendingPaste: { from: number; to: number; generation: number } | undefined
let disposed = false
const copyTimers = new Set<ReturnType<typeof setTimeout>>()

function lockExtensions() {
  return [
    EditorState.readOnly.of(props.locked),
    EditorView.editable.of(!props.locked),
    EditorView.contentAttributes.of({ 'aria-readonly': String(props.locked) }),
  ]
}

function createEditorTheme() {
  const fontSize = 14
  const lineHeight = 1.9
  return EditorView.theme({
    '&': { height: '100%', backgroundColor: 'transparent', color: 'var(--text)', fontSize: `${fontSize}px`, '--editor-line-height': `${fontSize * lineHeight}px` },
    '&.cm-focused': { outline: 'none' },
    '.cm-scroller': { fontFamily: '"Cascadia Code", "Consolas", "Microsoft YaHei UI", monospace', lineHeight: String(lineHeight), overflow: 'auto' },
    '.cm-content': { padding: '30px 24px 80px 12px', caretColor: 'var(--accent-text)' },
    '.cm-cursor, .cm-dropCursor': { borderLeftColor: 'var(--accent-text)' },
    '.cm-gutters': { backgroundColor: 'transparent', border: 'none', color: 'var(--editor-gutter)', padding: '0 4px 0 12px' },
    '.cm-activeLineGutter, .cm-activeLine': { backgroundColor: 'var(--editor-active-line)' },
    '.cm-selectionBackground, &.cm-focused .cm-selectionBackground': { backgroundColor: 'var(--selection)' },
    '.cm-matchingBracket': { backgroundColor: 'var(--selection)' },
    '.cm-panels, .cm-tooltip': { backgroundColor: 'var(--surface-raised)', color: 'var(--text)', borderColor: 'var(--border)' },
    '.cm-searchMatch': { backgroundColor: 'var(--selection)', outline: '1px solid var(--accent)' },
    '.cm-searchMatch.cm-searchMatch-selected': { backgroundColor: 'var(--surface-selected)' },
  }, { dark: props.dark })
}

function createState() {
  const lineEnding = props.document.content.match(/\r\n|\r|\n/)?.[0] ?? '\n'
  return EditorState.create({
    doc: props.document.content,
    extensions: [
      basicSetup,
      markdown({ codeLanguages: languages }),
      EditorView.lineWrapping,
      EditorState.lineSeparator.of(lineEnding),
      lockConfiguration.of(lockExtensions()),
      EditorView.contentAttributes.of({ 'aria-label': 'Markdown 原文', tabindex: '0', spellcheck: 'false' }),
      themeConfiguration.of(createEditorTheme()),
      syntaxHighlighting(HighlightStyle.define([
        { tag: [tags.keyword, tags.modifier, tags.operatorKeyword], color: 'var(--syntax-keyword)' },
        { tag: [tags.name, tags.propertyName], color: 'var(--text)' },
        { tag: [tags.typeName, tags.className, tags.function(tags.variableName)], color: 'var(--syntax-entity)' },
        { tag: [tags.number, tags.bool, tags.null, tags.atom, tags.attributeName], color: 'var(--syntax-constant)' },
        { tag: [tags.string, tags.regexp, tags.url], color: 'var(--syntax-string)' },
        { tag: [tags.comment, tags.meta], color: 'var(--syntax-comment)' },
        { tag: [tags.tagName, tags.heading], color: 'var(--syntax-tag)' },
        { tag: tags.heading, fontWeight: '600' },
        { tag: tags.link, color: 'var(--link)', textDecoration: 'underline' },
        { tag: tags.strong, fontWeight: 'bold' },
        { tag: tags.emphasis, fontStyle: 'italic' },
        { tag: tags.strikethrough, textDecoration: 'line-through' },
        { tag: tags.invalid, color: 'var(--error-text)' },
      ])),
      EditorView.updateListener.of(update => {
        if (pendingPaste && update.docChanged) {
          pendingPaste.from = update.changes.mapPos(pendingPaste.from, 1)
          pendingPaste.to = update.changes.mapPos(pendingPaste.to, 1)
        }
        if (update.docChanged) emit('change', props.document.id, update.state.sliceDoc())
      }),
      EditorView.domEventHandlers({
        scroll: () => { scheduleScroll('source') },
        paste: event => {
          const image = Array.from(event.clipboardData?.items ?? []).find(item => item.kind === 'file' && item.type.startsWith('image/'))?.getAsFile()
          if (!image || props.locked) return false
          event.preventDefault()
          void pasteImage(image)
          return true
        },
      }),
    ],
  })
}

watch(() => props.locked, () => {
  editor?.dispatch({ effects: lockConfiguration.reconfigure(lockExtensions()) })
})

watch(() => props.dark, () => {
  editor?.dispatch({ effects: themeConfiguration.reconfigure(createEditorTheme()) })
})

watch(() => props.document.content, content => {
  clearTimeout(renderTimer)
  renderTimer = setTimeout(() => { renderedContent.value = content }, 90)
})

watch(() => props.document.generation, () => {
  editor?.setState(createState())
  renderedContent.value = props.document.content
})

watch(() => [props.active, props.document.mode], async () => {
  await nextTick()
  editor?.requestMeasure()
  if (props.active) scheduleLayout()
  if (!props.active) selectedImage.value = undefined
})

watch(rendered, async () => {
  await nextTick()
  refreshSearch(false)
  scheduleLayout()
}, { flush: 'post' })

watch([searchQuery, caseSensitive], () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => refreshSearch(true), 100)
})

watch([outlineOpen, searchOpen], async () => {
  await nextTick()
  editor?.requestMeasure()
  scheduleLayout()
})

function measurePreview() {
  if (!preview.value || !article.value || !props.active || props.document.mode === 'source') return
  const top = preview.value.getBoundingClientRect().top
  const byLine = new Map<number, number>([[0, 0]])
  for (const element of Array.from(article.value.querySelectorAll<HTMLElement>('[data-source-line]'))) {
    const line = Number(element.dataset.sourceLine)
    const position = element.getBoundingClientRect().top - top + preview.value.scrollTop
    byLine.set(line, Math.min(byLine.get(line) ?? Infinity, position))
  }
  byLine.set((editor?.state.doc.lines ?? 1), preview.value.scrollHeight)
  anchors = Array.from(byLine, ([line, top]) => ({ line, top })).sort((first, second) => first.line - second.line)
  for (let index = 1; index < anchors.length; index++) anchors[index].top = Math.max(anchors[index - 1].top, anchors[index].top)
  headingPositions = Array.from(article.value.querySelectorAll<HTMLElement>('[data-heading-id]'), element => ({ id: element.dataset.headingId!, top: element.getBoundingClientRect().top - top + preview.value!.scrollTop }))
  updateCurrentHeading()
}

function scheduleLayout() {
  if (layoutFrame !== undefined) return
  layoutFrame = requestAnimationFrame(() => {
    layoutFrame = undefined
    measurePreview()
  })
}

function interpolate(value: number, source: 'line' | 'top', target: 'line' | 'top') {
  let previous = anchors[0]
  if (!previous) return 0
  for (const next of anchors.slice(1)) {
    if (next[source] > value) {
      const fraction = Math.max(0, Math.min(1, (value - previous[source]) / (next[source] - previous[source])))
      return previous[target] + fraction * (next[target] - previous[target])
    }
    previous = next
  }
  return previous[target]
}

function sourceLineAtTop() {
  if (!editor) return 0
  const height = editor.scrollDOM.getBoundingClientRect().top - editor.documentTop
  const block = editor.lineBlockAtHeight(Math.max(0, height))
  const first = editor.state.doc.lineAt(block.from).number - 1
  const last = editor.state.doc.lineAt(block.to).number
  return first + Math.max(0, Math.min(1, (height - block.top) / block.height)) * (last - first)
}

function scrollSourceToLine(line: number) {
  if (!editor) return
  const number = Math.min(editor.state.doc.lines, Math.max(1, Math.floor(line) + 1))
  const block = editor.lineBlockAt(editor.state.doc.line(number).from)
  editor.scrollDOM.scrollTop = block.top + (line - Math.floor(line)) * block.height + editor.documentPadding.top
}

function updateCurrentHeading() {
  if (props.document.mode === 'source') {
    const line = sourceLineAtTop()
    currentHeading.value = rendered.value.headings.findLast(heading => heading.line <= line)?.id ?? rendered.value.headings[0]?.id ?? ''
    return
  }
  const top = (preview.value?.scrollTop ?? 0) + 36
  currentHeading.value = headingPositions.findLast(heading => heading.top <= top)?.id ?? headingPositions[0]?.id ?? ''
}

function scheduleScroll(side: 'source' | 'preview') {
  if (!props.active) return
  if (side !== scrollLeader) { updateCurrentHeading(); return }
  if (scrollFrame !== undefined) cancelAnimationFrame(scrollFrame)
  scrollFrame = requestAnimationFrame(() => {
    scrollFrame = undefined
    if (syncScroll.value && props.document.mode === 'split' && editor && preview.value) {
      const source = editor.scrollDOM
      const target = preview.value
      if (side === 'source') {
        if (source.scrollTop <= 1) target.scrollTop = 0
        else if (source.scrollTop >= source.scrollHeight - source.clientHeight - 1) target.scrollTop = target.scrollHeight
        else target.scrollTop = interpolate(sourceLineAtTop(), 'line', 'top')
      } else {
        if (target.scrollTop <= 1) source.scrollTop = 0
        else if (target.scrollTop >= target.scrollHeight - target.clientHeight - 1) source.scrollTop = source.scrollHeight
        else scrollSourceToLine(interpolate(target.scrollTop, 'top', 'line'))
      }
    }
    updateCurrentHeading()
  })
}

function toggleSync() {
  syncScroll.value = !syncScroll.value
  if (syncScroll.value) {
    scrollLeader = 'source'
    measurePreview()
    scheduleScroll('source')
  }
}

function navigate(heading: MarkdownHeading) {
  scrollLeader = 'navigation'
  const element = article.value?.querySelector<HTMLElement>(`[data-heading-id="${CSS.escape(heading.id)}"]`)
  if (preview.value && element) preview.value.scrollTop += element.getBoundingClientRect().top - preview.value.getBoundingClientRect().top - 16
  if (editor && props.document.mode !== 'preview') {
    const line = editor.state.doc.line(Math.min(editor.state.doc.lines, heading.line + 1))
    editor.dispatch({ effects: EditorView.scrollIntoView(line.from, { y: 'start', yMargin: 16 }) })
  }
  currentHeading.value = heading.id
}

function refreshSearch(jump: boolean) {
  if (!article.value) return
  const previous = matchIndex.value
  matches = highlightPreviewSearch(article.value, searchOpen.value ? searchQuery.value : '', caseSensitive.value)
  matchCount.value = matches.length
  matchIndex.value = matches.length ? Math.min(Math.max(0, previous), matches.length - 1) : -1
  if (jump && matches.length) matchIndex.value = 0
  selectMatch(jump)
}

function selectMatch(jump: boolean) {
  for (const mark of selectedMarks) mark.classList.remove('current')
  const match = matches[matchIndex.value]
  selectedMarks = match?.marks ?? []
  for (const mark of selectedMarks) mark.classList.add('current')
  if (!jump || !match || !preview.value) return
  scrollLeader = 'navigation'
  const mark = match.marks[0]
  const rect = mark.getBoundingClientRect()
  const bounds = preview.value.getBoundingClientRect()
  preview.value.scrollTop += rect.top - bounds.top - preview.value.clientHeight / 3
  const code = mark.closest('pre')
  if (code) code.scrollLeft += rect.left - code.getBoundingClientRect().left - 24
  if (syncScroll.value && props.document.mode === 'split') scrollSourceToLine(match.line)
  updateCurrentHeading()
}

function moveMatch(direction: number) {
  if (!matches.length) return
  matchIndex.value = (matchIndex.value + direction + matches.length) % matches.length
  selectMatch(true)
}

async function openSearch() {
  if (props.document.mode === 'source') props.document.mode = 'split'
  searchOpen.value = true
  await nextTick()
  refreshSearch(false)
  searchInput.value?.focus()
  searchInput.value?.select()
}

function closeSearch() {
  searchOpen.value = false
  clearTimeout(searchTimer)
  if (article.value) clearPreviewSearch(article.value)
  matches = []
  selectedMarks = []
  matchCount.value = 0
  matchIndex.value = -1
  preview.value?.focus({ preventScroll: true })
}

function onKeydown(event: KeyboardEvent) {
  if (!props.active || event.defaultPrevented || event.isComposing || document.querySelector('dialog[open]')) return
  if ((event.ctrlKey || event.metaKey) && !event.altKey && event.key.toLowerCase() === 'f') {
    if (!event.shiftKey && editor?.dom.contains(event.target as Node)) return
    event.preventDefault()
    void openSearch()
  } else if (searchOpen.value && event.key === 'F3') {
    event.preventDefault()
    moveMatch(event.shiftKey ? -1 : 1)
  } else if (searchOpen.value && event.key === 'Escape') {
    event.preventDefault()
    closeSearch()
  }
}

async function pasteImage(image: File) {
  if (!editor || pastingImage.value) return
  pastingImage.value = true
  const selection = editor.state.selection.main
  pendingPaste = { from: selection.from, to: selection.to, generation: props.document.generation }
  try {
    if (!props.document.path && !(await props.saveDocument(props.document))) return
    if (disposed || props.locked || !editor || !pendingPaste) return
    const dataURL = await new Promise<string>((resolve, reject) => {
      const reader = new FileReader()
      reader.onload = () => resolve(String(reader.result))
      reader.onerror = () => reject(new Error('无法读取剪贴板图片'))
      reader.readAsDataURL(image)
    })
    const path = await AppService.SaveImage(props.document.id, dataURL)
    if (disposed || !editor || !pendingPaste || pendingPaste.generation !== props.document.generation) return
    const { from, to } = pendingPaste
    const before = editor.state.doc.sliceString(Math.max(0, from - 2), from)
    const after = editor.state.doc.sliceString(to, Math.min(editor.state.doc.length, to + 2))
    const prefix = before && !before.endsWith('\n\n') ? (before.endsWith('\n') ? '\n' : '\n\n') : ''
    const suffix = after && !after.startsWith('\n\n') ? (after.startsWith('\n') ? '\n' : '\n\n') : ''
    const insert = Text.of(`${prefix}![图片](${path.split('/').map(segment => encodeURIComponent(segment)).join('/')})${suffix}`.split('\n'))
    pendingPaste = undefined
    editor.dispatch({ changes: { from, to, insert }, selection: { anchor: from + insert.length }, scrollIntoView: true })
    editor.focus()
  } catch (error) { if (!disposed) emit('error', String(error)) }
  finally { pastingImage.value = false; pendingPaste = undefined }
}

function startResize(event: PointerEvent) {
  if (event.button !== 0) return
  resizing.value = true
  const handle = event.currentTarget as HTMLElement
  handle.setPointerCapture(event.pointerId)
}

function resize(event: PointerEvent) {
  if (!resizing.value || !container.value) return
  const rect = container.value.getBoundingClientRect()
  ratio.value = Math.min(75, Math.max(25, ((event.clientX - rect.left) / rect.width) * 100))
  editor?.requestMeasure()
}

function resizeByKey(event: KeyboardEvent) {
  if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return
  event.preventDefault()
  ratio.value = Math.min(75, Math.max(25, ratio.value + (event.key === 'ArrowLeft' ? -2 : 2)))
  editor?.requestMeasure()
}

function previewKeydown(event: KeyboardEvent) {
  scrollLeader = 'preview'
  if ((event.key === 'Enter' || event.key === ' ') && (event.target as HTMLElement).matches('img')) {
    event.preventDefault()
    void openLink(event)
  }
}

async function openLink(event: MouseEvent | KeyboardEvent) {
  const target = event.target as HTMLElement
  const copy = target.closest<HTMLButtonElement>('.code-copy')
  if (copy) {
    const content = rendered.value.codeBlocks[Number(copy.dataset.codeIndex)]
    if (content === undefined) return
    try {
      await AppService.Copy(content)
      if (disposed) return
      copy.textContent = '已复制'
      const timer = setTimeout(() => { copy.textContent = '复制'; copyTimers.delete(timer) }, 1600)
      copyTimers.add(timer)
    } catch (error) { emit('error', String(error)) }
    return
  }
  const image = target.closest<HTMLImageElement>('img')
  if (image) {
    event.preventDefault()
    if (image.naturalWidth) selectedImage.value = { src: image.currentSrc || image.src, alt: image.alt }
    return
  }
  const anchor = (event.target as HTMLElement).closest('a')
  if (!anchor) return
  event.preventDefault()
  const href = anchor.getAttribute('href') ?? ''
  if (href.startsWith('#')) {
    let id = href.slice(1)
    try { id = decodeURIComponent(id) } catch { /* Use the literal anchor. */ }
    const heading = rendered.value.headings.find(heading => heading.id === id)
    if (heading) navigate(heading)
    return
  }
  try {
    await AppService.OpenLink(href)
  } catch (error) {
    emit('error', String(error))
  }
}

onMounted(() => {
  editor = new EditorView({ state: createState(), parent: editorHost.value })
  resizeObserver = new ResizeObserver(scheduleLayout)
  if (article.value) resizeObserver.observe(article.value)
  if (preview.value) resizeObserver.observe(preview.value)
  window.addEventListener('keydown', onKeydown)
  scheduleLayout()
  if (props.active && !props.document.path) editor.focus()
})
onBeforeUnmount(() => {
  disposed = true
  clearTimeout(renderTimer)
  clearTimeout(searchTimer)
  for (const timer of copyTimers) clearTimeout(timer)
  if (scrollFrame !== undefined) cancelAnimationFrame(scrollFrame)
  if (layoutFrame !== undefined) cancelAnimationFrame(layoutFrame)
  resizeObserver?.disconnect()
  window.removeEventListener('keydown', onKeydown)
  editor?.destroy()
})
</script>

<template>
  <section class="document-pane" :class="{ resizing }" :aria-label="document.name">
    <div class="document-tools" role="toolbar" aria-label="阅读工具">
      <button :class="{ chosen: outlineOpen }" :aria-pressed="outlineOpen" @click="outlineOpen = !outlineOpen">大纲</button>
      <button :class="{ chosen: syncScroll }" :aria-pressed="syncScroll" :disabled="document.mode !== 'split'" title="原文与预览双向同步滚动" @click="toggleSync">同步滚动</button>
      <button :class="{ chosen: searchOpen }" :aria-expanded="searchOpen" title="搜索预览 · Ctrl+Shift+F" @click="openSearch">搜索预览</button>
      <span v-if="pastingImage" class="image-paste-status" role="status">正在保存图片…</span>
    </div>
    <form v-if="searchOpen" class="preview-search" role="search" aria-label="搜索预览内容" @submit.prevent="moveMatch(1)">
      <input ref="searchInput" v-model="searchQuery" type="search" aria-label="预览搜索关键词" placeholder="搜索预览内容" autocomplete="off" spellcheck="false" @keydown.shift.enter.prevent="moveMatch(-1)" />
      <span class="search-count" role="status" aria-live="polite">{{ matchCount ? `${matchIndex + 1} / ${matchCount}` : searchQuery ? '无匹配' : '0 / 0' }}</span>
      <button type="button" :class="{ chosen: caseSensitive }" :aria-pressed="caseSensitive" aria-label="区分大小写" title="区分大小写" @click="caseSensitive = !caseSensitive">Aa</button>
      <button type="button" aria-label="上一个匹配" title="上一个 · Shift+Enter / Shift+F3" :disabled="!matchCount" @click="moveMatch(-1)">↑</button>
      <button type="submit" aria-label="下一个匹配" title="下一个 · Enter / F3" :disabled="!matchCount">↓</button>
      <button type="button" aria-label="关闭预览搜索" title="关闭 · Esc" @click="closeSearch">×</button>
    </form>
    <div class="document-layout">
      <div ref="container" class="document-content">
        <div
          v-show="document.mode !== 'preview'"
          ref="editorHost"
          class="source-pane"
          :style="{ width: document.mode === 'split' ? `${ratio}%` : '100%' }"
          @wheel.passive="scrollLeader = 'source'"
          @pointerdown="scrollLeader = 'source'"
          @keydown="scrollLeader = 'source'"
        />
        <div
          v-show="document.mode === 'split'"
          class="resize-handle"
          role="separator"
          tabindex="0"
          aria-label="调整原文与预览宽度"
          aria-orientation="vertical"
          :aria-valuenow="Math.round(ratio)"
          :aria-valuemin="25"
          :aria-valuemax="75"
          @pointerdown="startResize"
          @pointermove="resize"
          @pointerup="resizing = false"
          @lostpointercapture="resizing = false"
          @keydown="resizeByKey"
        />
        <div
          v-show="document.mode !== 'source'"
          ref="preview"
          class="preview-pane"
          tabindex="0"
          aria-label="Markdown 预览"
          @click="openLink"
          @scroll.passive="scheduleScroll('preview')"
          @wheel.passive="scrollLeader = 'preview'"
          @pointerdown="scrollLeader = 'preview'"
          @keydown="previewKeydown"
          @load.capture="scheduleLayout"
        >
          <article ref="article" class="markdown-body" v-html="rendered.html" />
        </div>
      </div>
      <OutlinePanel v-if="outlineOpen" :headings="rendered.headings" :current="currentHeading" @navigate="navigate" @close="outlineOpen = false" />
    </div>
    <ImageViewer :image="selectedImage" @close="selectedImage = undefined" />
  </section>
</template>
