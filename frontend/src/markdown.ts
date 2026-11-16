import MarkdownIt from 'markdown-it'
import taskLists from 'markdown-it-task-lists'
import DOMPurify from 'dompurify'
import hljs from 'highlight.js/lib/common'

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
  return originalImage(tokens, index, options, environment, renderer)
}

markdown.renderer.rules.heading_open = (tokens, index, options, _environment, renderer) => {
  const title = tokens[index + 1]?.content ?? ''
  tokens[index].attrSet('id', title.toLowerCase().replace(/[^\p{L}\p{N}\s_-]/gu, '').replace(/\s+/g, '-'))
  return renderer.renderToken(tokens, index, options)
}

export function renderMarkdown(content: string, id: string): string {
  return DOMPurify.sanitize(markdown.render(content.replace(/^\uFEFF/, ''), { id }))
}
