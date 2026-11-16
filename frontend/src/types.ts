export type ViewMode = 'source' | 'split' | 'preview'

export interface OpenDocument {
  id: string
  path: string
  name: string
  content: string
  revision: string
}

export interface DocumentTab extends OpenDocument {
  savedContent: string
  mode: ViewMode
  generation: number
}
