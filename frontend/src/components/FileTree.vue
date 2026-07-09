<script setup lang="ts">
defineProps<{ entries: { path: string; type: string }[] }>()
defineEmits<{ select: [path: string] }>()
</script>

<template>
  <div class="p-2">
    <div
      v-for="e in entries"
      :key="e.path"
      class="flex items-center gap-2 px-3 py-2 hover:bg-[#f6f8fa] cursor-pointer text-sm"
      @click="$emit('select', e.path)"
    >
      <span>{{ e.type === 'dir' ? '📁' : '📄' }}</span>
      <span>{{ e.path }}</span>
    </div>
    <p v-if="!entries.length" class="p-4 text-[#656d76] text-sm">Empty repository</p>
  </div>
</template>
