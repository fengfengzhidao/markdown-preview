import MarkdownIt from 'markdown-it'
import type { Env } from 'markdown-it'
import taskLists from 'markdown-it-task-lists'
import DOMPurify from 'dompurify'
import hljs from 'highlight.js/lib/common'

export interface MarkdownHeading {
  id: string
  title: string
  level: number
  line: number
}

interface MarkdownEnvironment extends Env {
  id: string
  headings: MarkdownHeading[]
  codeBlocks: string[]
  headingIDs: Set<string>
}

const markdown = new MarkdownIt({
  html: false,
  linkify: true,
  highlight(code, language) {
    if (language && hljs.getLanguage(language)) {
      return hljs.highlight(code, { language, ignoreIllegals: true }).value
    }
    return ''
  },
}).use(taskLists, { enabled: false, label: true })

const originalImage = markdown.renderer.rules.image!
markdown.renderer.rules.image = (tokens, index, options, environment, renderer) => {
  const token = tokens[index]
  const source = String(token.attrGet('src') ?? '')
  if (!/^(https?:|data:|\/\/)/i.test(source)) {
    let path = source
    try {
      path = decodeURIComponent(source)
    } catch {
      // Literal percent signs in local filenames remain valid.
    }
    token.attrSet('src', `/local-image/${environment?.id ?? ''}?path=${encodeURIComponent(path)}`)
  }
  token.attrSet('loading', 'lazy')
  token.attrSet('tabindex', '0')
  token.attrSet('role', 'button')
  token.attrSet('aria-label', `查看图片 ${token.content}`)
  return originalImage(tokens, index, options, environment, renderer)
}

markdown.renderer.rules.heading_open = (tokens, index, options, environment, renderer) => {
  const context = environment as MarkdownEnvironment
  const inline = tokens[index + 1]
  const title = inline?.children?.filter(token => ['text', 'code_inline', 'image'].includes(token.type)).map(token => token.content).join('') || inline?.content || '无标题'
  const base = title.toLowerCase().replace(/[^\p{L}\p{N}\s_-]/gu, '').replace(/\s+/g, '-') || 'section'
  let id = base
  let suffix = 1
  while (context.headingIDs.has(id)) id = `${base}-${suffix++}`
  context.headingIDs.add(id)
  tokens[index].attrSet('id', id)
  tokens[index].attrSet('data-heading-id', id)
  context.headings.push({ id, title, level: Number(tokens[index].tag.slice(1)), line: tokens[index].map?.[0] ?? 0 })
  return renderer.renderToken(tokens, index, options)
}

for (const type of ['fence', 'code_block']) {
  const renderCode = markdown.renderer.rules[type]!
  markdown.renderer.rules[type] = (tokens, index, options, environment, renderer) => {
    const codeIndex = (environment as MarkdownEnvironment).codeBlocks.push(tokens[index].content) - 1
    const line = tokens[index].map?.[0] ?? 0
    return `<div class="code-block" data-source-line="${line}"><button type="button" class="code-copy" data-code-index="${codeIndex}" aria-label="复制代码块">复制</button>${renderCode(tokens, index, options, environment, renderer)}</div>`
  }
}

export function renderMarkdown(content: string, id: string) {
  const environment: MarkdownEnvironment = { id, headings: [], codeBlocks: [], headingIDs: new Set() }
  const tokens = markdown.parse(content.replace(/^\uFEFF/, ''), environment)
  for (const token of tokens) {
    if (token.map && token.nesting !== -1 && token.type !== 'inline') token.attrSet('data-source-line', String(token.map[0]))
  }
  const html = DOMPurify.sanitize(markdown.renderer.render(tokens, markdown.options, environment))
  return { html, headings: environment.headings, codeBlocks: environment.codeBlocks }
}
