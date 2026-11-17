<script setup lang="ts">
import type { RecentFile } from '../types'

defineProps<{ files: RecentFile[]; activeId: string; busy: boolean }>()
defineEmits<{ create: []; pick: []; open: [file: RecentFile]; remove: [id: string] }>()

function parentFolder(path: string) {
  const parts = path.replace(/\\/g, '/').split('/')
  return parts.slice(0, -1).slice(-2).join(' / ')
}
</script>

<template>
  <aside id="file-sidebar" class="file-sidebar" aria-label="文件列表">
    <div class="sidebar-actions" role="group" aria-label="文件操作">
      <button class="open-button new-button" title="新建 Markdown 文件 · Ctrl+N" :disabled="busy" @click="$emit('create')"><span aria-hidden="true">＋</span><span>新建</span></button>
      <button class="open-button" title="打开 Markdown 文件 · Ctrl+O" :disabled="busy" @click="$emit('pick')"><span class="folder-icon" aria-hidden="true" /><span>打开</span></button>
    </div>
    <h2 class="sidebar-heading">文件列表</h2>
    <p v-if="!files.length" class="sidebar-empty">打开过的文件会显示在这里</p>
    <nav v-else class="sidebar-navigation" aria-label="打开过的文件">
      <ul class="sidebar-files">
        <li v-for="file in files" :key="file.id" class="sidebar-row" :class="{ current: file.id === activeId }">
          <button class="sidebar-file" :title="file.path" :aria-current="file.id === activeId ? 'page' : undefined" :disabled="busy" @click="$emit('open', file)">
            <span class="sidebar-document-icon" aria-hidden="true" />
            <span class="sidebar-file-text"><span class="sidebar-file-name">{{ file.name }}</span><span class="sidebar-directory">{{ parentFolder(file.path) }}</span></span>
          </button>
          <button class="sidebar-remove" :aria-label="`移除记录 ${file.name}`" title="移除记录" :disabled="busy" @click="$emit('remove', file.id)">×</button>
        </li>
      </ul>
    </nav>
  </aside>
</template>
