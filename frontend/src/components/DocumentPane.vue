<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { basicSetup } from 'codemirror'
import { Compartment, EditorState } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { markdown } from '@codemirror/lang-markdown'
import { languages } from '@codemirror/language-data'
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language'
import { tags } from '@lezer/highlight'
import { AppService } from '../../bindings/markdownlite'
import { renderMarkdown } from '../markdown'
import type { DocumentTab } from '../types'

const props = defineProps<{ document: DocumentTab; locked: boolean; active: boolean; dark: boolean }>()
const emit = defineEmits<{ change: [id: string, content: string]; error: [message: string] }>()
const editorHost = ref<HTMLElement>()
const preview = ref<HTMLElement>()
const container = ref<HTMLElement>()
const ratio = ref(50)
const resizing = ref(false)
const renderedContent = ref(props.document.content)
const rendered = computed(() => renderMarkdown(renderedContent.value, props.document.id))
const lockConfiguration = new Compartment()
const themeConfiguration = new Compartment()
let editor: EditorView | undefined
let renderTimer: ReturnType<typeof setTimeout> | undefined

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
        if (update.docChanged) emit('change', props.document.id, update.state.sliceDoc())
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
})

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

async function openLink(event: MouseEvent) {
  const anchor = (event.target as HTMLElement).closest('a')
  if (!anchor) return
  event.preventDefault()
  const href = anchor.getAttribute('href') ?? ''
  if (href.startsWith('#')) {
    let id = href.slice(1)
    try { id = decodeURIComponent(id) } catch { /* Use the literal anchor. */ }
    const heading = Array.from(preview.value?.querySelectorAll('[id]') ?? []).find(element => element.id === id)
    heading?.scrollIntoView({ behavior: 'smooth', block: 'start' })
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
  if (props.active && !props.document.path) editor.focus()
})
onBeforeUnmount(() => { clearTimeout(renderTimer); editor?.destroy() })
</script>

<template>
  <section ref="container" class="document-pane" :class="{ resizing }" :aria-label="document.name">
    <div
      v-show="document.mode !== 'preview'"
      ref="editorHost"
      class="source-pane"
      :style="{ width: document.mode === 'split' ? `${ratio}%` : '100%' }"
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
    <div v-show="document.mode !== 'source'" ref="preview" class="preview-pane" @click="openLink">
      <article class="markdown-body" v-html="rendered" />
    </div>
  </section>
</template>
