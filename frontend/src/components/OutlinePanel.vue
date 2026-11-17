<script setup lang="ts">
import { computed, ref } from 'vue'
import type { MarkdownHeading } from '../markdown'

const props = defineProps<{ headings: MarkdownHeading[]; current: string }>()
defineEmits<{ navigate: [heading: MarkdownHeading]; close: [] }>()
const collapsed = ref(new Set<string>())
const minimumLevel = computed(() => Math.min(...props.headings.map(heading => heading.level)))
const visible = computed(() => {
  let hiddenBelow = 7
  return props.headings.filter(heading => {
    if (heading.level > hiddenBelow) return false
    hiddenBelow = collapsed.value.has(heading.id) ? heading.level : 7
    return true
  })
})
function hasChildren(heading: MarkdownHeading) {
  const index = props.headings.indexOf(heading)
  return (props.headings[index + 1]?.level ?? 0) > heading.level
}
function toggle(id: string) {
  if (collapsed.value.has(id)) collapsed.value.delete(id)
  else collapsed.value.add(id)
}
</script>

<template>
  <aside class="outline-panel" aria-label="文章大纲">
    <header><span>文章大纲</span><button aria-label="收起文章大纲" title="收起文章大纲" @click="$emit('close')">×</button></header>
    <nav aria-label="章节导航">
      <p v-if="!headings.length" class="outline-empty">文档中的标题会显示在这里</p>
      <div v-for="heading in visible" :key="heading.id" class="outline-row" :style="{ paddingLeft: `${8 + (heading.level - minimumLevel) * 12}px` }">
        <button v-if="hasChildren(heading)" class="outline-fold" :aria-label="`${collapsed.has(heading.id) ? '展开' : '折叠'} ${heading.title}`" :aria-expanded="!collapsed.has(heading.id)" @click="toggle(heading.id)">{{ collapsed.has(heading.id) ? '›' : '⌄' }}</button>
        <span v-else class="outline-fold-space" />
        <button class="outline-link" :class="{ current: current === heading.id }" :aria-current="current === heading.id ? 'location' : undefined" :title="heading.title" @click="$emit('navigate', heading)">{{ heading.title }}</button>
      </div>
    </nav>
  </aside>
</template>
