export type ViewMode = 'source' | 'split' | 'preview'

export interface OpenDocument {
  id: string
  path: string
  name: string
  content: string
  revision: string
}

export interface RecentFile {
  id: string
  path: string
  name: string
}

export interface RecentFileList {
  revision: number
  files: RecentFile[] | null
}

export interface DocumentTab extends OpenDocument {
  key: string
  locked: boolean
  savedContent: string
  mode: ViewMode
  generation: number
}
