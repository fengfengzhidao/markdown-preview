export interface PreviewMatch {
  marks: HTMLElement[]
  line: number
}

export function clearPreviewSearch(root: HTMLElement) {
  for (const mark of Array.from(root.querySelectorAll('mark.preview-match'))) mark.replaceWith(document.createTextNode(mark.textContent ?? ''))
  root.normalize()
}

// Search each text block as a whole so inline emphasis never splits a match.
export function highlightPreviewSearch(root: HTMLElement, query: string, caseSensitive: boolean): PreviewMatch[] {
  clearPreviewSearch(root)
  if (!query) return []
  const blocks = new Map<Element, Text[]>()
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT)
  while (walker.nextNode()) {
    const node = walker.currentNode as Text
    if (node.parentElement?.closest('button, script, style')) continue
    const block = node.parentElement?.closest('p, li, h1, h2, h3, h4, h5, h6, pre, td, th') ?? root
    const nodes = blocks.get(block) ?? []
    nodes.push(node)
    blocks.set(block, nodes)
  }
  const expression = new RegExp(query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), caseSensitive ? 'gu' : 'giu')
  const matches: PreviewMatch[] = []
  for (const [block, nodes] of blocks) {
    const text = nodes.map(node => node.data).join('')
    const ranges = Array.from(text.matchAll(expression), match => ({ from: match.index!, to: match.index! + match[0].length, marks: [] as HTMLElement[] }))
    let offset = 0
    let rangeIndex = 0
    for (const node of nodes) {
      const end = offset + node.length
      while (rangeIndex < ranges.length && ranges[rangeIndex].to <= offset) rangeIndex++
      const overlapping: typeof ranges = []
      for (let index = rangeIndex; index < ranges.length && ranges[index].from < end; index++) overlapping.push(ranges[index])
      if (overlapping.length) {
        const fragment = document.createDocumentFragment()
        let cursor = 0
        for (const range of overlapping) {
          const from = Math.max(0, range.from - offset)
          const to = Math.min(node.length, range.to - offset)
          fragment.append(node.data.slice(cursor, from))
          const mark = document.createElement('mark')
          mark.className = 'preview-match'
          mark.textContent = node.data.slice(from, to)
          fragment.append(mark)
          range.marks.push(mark)
          cursor = to
        }
        fragment.append(node.data.slice(cursor))
        node.replaceWith(fragment)
      }
      offset = end
    }
    const line = Number(block.closest<HTMLElement>('[data-source-line]')?.dataset.sourceLine ?? 0)
    matches.push(...ranges.map(range => ({ marks: range.marks, line })))
  }
  return matches
}
